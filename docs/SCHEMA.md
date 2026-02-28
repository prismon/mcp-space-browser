# Database Schema

mcp-space-browser uses SQLite for all persistent storage. The schema is initialized in `pkg/database/database.go`.

## Core Tables

### entries

One row per file or directory in the indexed filesystem.

```sql
CREATE TABLE entries (
  id INTEGER PRIMARY KEY,
  path TEXT UNIQUE NOT NULL,
  parent TEXT,
  size INTEGER,
  blocks INTEGER DEFAULT 0,
  kind TEXT CHECK(kind IN ('file', 'directory')),
  ctime INTEGER,
  mtime INTEGER,
  last_scanned INTEGER,
  dirty INTEGER DEFAULT 0
);
CREATE INDEX idx_parent ON entries(parent);
CREATE INDEX idx_mtime ON entries(mtime);
```

- `size`: For files, actual file size. For directories, sum of direct children (computed by aggregation).
- `last_scanned`: Unix timestamp of last scan. Used to skip re-indexing recent paths.
- `dirty`: Flag for incremental update tracking.

### attributes

Extensible key-value metadata per entry. Multiple attributes per entry, identified by `(entry_path, key)`.

```sql
CREATE TABLE attributes (
  entry_path TEXT NOT NULL,
  key TEXT NOT NULL,
  value TEXT,
  source TEXT NOT NULL,
  computed_at INTEGER,
  PRIMARY KEY (entry_path, key)
);
CREATE INDEX idx_attributes_key ON attributes(key);
CREATE INDEX idx_attributes_source ON attributes(source);
```

- `key`: Attribute name (e.g. `mime`, `hash.md5`, `exif.camera`, `permissions`).
- `source`: How the attribute was computed (e.g. `scan`, `classifier`, `rule`).
- Queryable via the `query` tool's `where` clause. Non-entry-column keys automatically JOIN on this table.

### resource_sets

Named collections of entries, forming a DAG (directed acyclic graph).

```sql
CREATE TABLE resource_sets (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  created_at INTEGER,
  updated_at INTEGER
);

CREATE TABLE resource_set_entries (
  set_id INTEGER NOT NULL,
  entry_path TEXT NOT NULL,
  added_at INTEGER,
  PRIMARY KEY (set_id, entry_path),
  FOREIGN KEY (set_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
  FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
);

CREATE TABLE resource_set_edges (
  parent_id INTEGER NOT NULL,
  child_id INTEGER NOT NULL,
  added_at INTEGER,
  PRIMARY KEY (parent_id, child_id),
  FOREIGN KEY (parent_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
  FOREIGN KEY (child_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
  CHECK (parent_id != child_id)
);
```

## Orchestration Tables

### sources

Unified source abstraction for data ingestion.

```sql
CREATE TABLE sources (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  type TEXT CHECK(type IN ('filesystem.index', 'filesystem.watch', 'query', 'resource-set')) NOT NULL,
  target_set_name TEXT NOT NULL,
  update_mode TEXT CHECK(update_mode IN ('replace', 'append', 'merge')),
  config_json TEXT,
  status TEXT CHECK(status IN ('stopped', 'starting', 'running', 'stopping', 'completed', 'error')),
  enabled INTEGER DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_run_at INTEGER,
  last_error TEXT
);
```

### plans

Orchestration of resource-sets and sources.

```sql
CREATE TABLE plans (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  mode TEXT CHECK(mode IN ('oneshot', 'continuous')),
  status TEXT CHECK(status IN ('active', 'paused', 'disabled')),
  resource_sets_json TEXT NOT NULL,
  sources_json TEXT NOT NULL,
  conditions_json TEXT,
  outcomes_json TEXT,
  created_at INTEGER,
  updated_at INTEGER,
  last_run_at INTEGER
);
```

### rules

Automation rules for file processing.

```sql
CREATE TABLE rules (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  enabled INTEGER DEFAULT 1,
  priority INTEGER DEFAULT 0,
  condition_json TEXT NOT NULL,
  outcome_json TEXT NOT NULL,
  created_at INTEGER,
  updated_at INTEGER
);
```

### index_jobs

Tracking for scan operations.

```sql
CREATE TABLE index_jobs (
  id INTEGER PRIMARY KEY,
  root_path TEXT NOT NULL,
  status TEXT CHECK(status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
  files_found INTEGER DEFAULT 0,
  files_indexed INTEGER DEFAULT 0,
  bytes_total INTEGER DEFAULT 0,
  error_message TEXT,
  created_at TEXT,
  started_at TEXT,
  completed_at TEXT,
  scan_config_json TEXT
);
```
