# Disk Space Browser MCP Agent Design

This document outlines a design for a new **MCP agent** that indexes local disk
storage, records file sizes, and provides tools for exploring disk utilization.

## Overview

- The agent will be implemented in **TypeScript** using the Bun runtime.
- It will traverse a filesystem using a depth‑first search (DFS), store
  folder/file metadata and sizes in a Bun database, and expose commands to query
  and visualize this data.
- The approach is inspired by tools such as *Baobab* and *WinDirStat*.

## Components

1. **MCP Server**: a Bun native HTTP service built with the
   [Model Context Protocol TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk).
   This server exposes endpoints and MCP resources for indexing and querying
   disk space information.  Following the SDK examples, the service can be
   launched via `McpServer` with a simple CLI entry point.
2. **Filesystem Crawler**: a TypeScript module that performs DFS on a given
   directory, gathering path, size, and type (file or folder).
3. **Database**: the built‑in Bun SQLite wrapper will store metadata (path,
   size, parent, isDirectory). Indices will support efficient queries such as
   listing contents or computing folder sizes.
4. **MCP Tools**: CLI commands integrated with the MCP framework to trigger
   indexing, fetch summaries, and generate visualizations (e.g., treemaps).

## Data Model

A single SQLite table `entries` will store the *current* state of each path.
Previous scan results are overwritten so the database always reflects the latest
view of the filesystem.  Each entry also records when it was last scanned and
whether the record's aggregated size needs to be recomputed.

```
id INTEGER PRIMARY KEY,
path TEXT UNIQUE NOT NULL,
parent INTEGER,
size INTEGER,
kind TEXT CHECK(kind IN ('file', 'directory')),
ctime INTEGER,
mtime INTEGER,
last_scanned INTEGER,
dirty INTEGER DEFAULT 0
```

- `path` is the absolute or root‑relative path and is unique per entry.
- `parent` references the parent directory entry.
- `size` stores file size in bytes (for directories it may store total size).
- `ctime` and `mtime` keep creation and modification timestamps for change
  tracking.
- `last_scanned` indicates when the crawler last processed this path.
- `dirty` is a boolean flag marking whether aggregates need recomputation.

## Filesystem Traversal

The crawler will:

1. Start from a root directory and push it onto a stack.
2. Pop a path, gather file stats (using Bun's `fs` module), insert the record
   into the database, and push child paths onto the stack if it is a directory.
3. Optionally, compute directory sizes by summing child file sizes after
   traversal.

Pseudocode:

```ts
async function index(root: string, db: Database) {
  const stack = [root];
  while (stack.length) {
    const current = stack.pop()!;
    const info = await fs.stat(current);
    const isDir = info.isDirectory();
    const now = Date.now();
    db.run(
      `INSERT INTO entries
        (path, parent, size, kind, ctime, mtime, last_scanned, dirty)
       VALUES (?, ?, ?, ?, ?, ?, ?, 0)
       ON CONFLICT(path) DO UPDATE SET
         parent=excluded.parent,
         size=excluded.size,
         kind=excluded.kind,
         ctime=excluded.ctime,
         mtime=excluded.mtime,
         last_scanned=excluded.last_scanned,
         dirty=0`,
      current,
      path.dirname(current),
      info.size,
      isDir ? 'directory' : 'file',
      info.ctimeMs,
      info.mtimeMs,
      now
    );
    if (isDir) {
      const children = await fs.readdir(current);
      stack.push(...children.map(c => path.join(current, c)));
    }
  }
}
```

Each scan updates `last_scanned` for every processed entry and resets the
`dirty` flag to `0`.  If metadata becomes stale (e.g., external changes are
detected), entries can be marked as `dirty = 1` so the crawler or other tools
know to recompute aggregated statistics.

## MCP Commands

We will provide CLI commands using the MCP tool interface:

- `index <path>`: run the crawler to index the specified path.
- `cd <path>`: navigate into a directory and return a lightweight listing slice.
- `inspect <path>`: fetch metadata for a file or directory.
- `job-progress <jobId>`: check on an indexing run.

These commands will query the SQLite database or trigger new indexing runs.

## Visualization

For a visualization similar to *Baobab* or *WinDirStat*, we can create a
web-based frontend served by the Bun service:

1. The backend will expose an endpoint like `/api/tree?path=/home/user` to
   retrieve hierarchical JSON of directories and sizes.
2. The frontend (JavaScript/TypeScript) can render this data as a treemap or
   sunburst using D3.js or a similar library.

## Future Work

- Implement deduplication detection by storing file hashes and reporting files
  with identical contents.
- Add incremental updates: detect filesystem changes and update the database
  without full reindexing.
- Provide summary statistics (total disk usage, largest directories/files).

