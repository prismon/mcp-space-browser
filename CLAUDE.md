# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Principles

### MCP-Only Interface

This system is **ONLY** exposed through the Model Context Protocol (MCP). There are:
- **NO REST APIs**
- **NO gRPC interfaces**
- **NO other external access methods**

All interaction happens via MCP tools and resource templates over JSON-RPC 2.0 at `POST /mcp`.

### Universal MCP Access

MCP tools are designed to be usable by:
- **Human Users** via MCP client applications
- **AI Models** (Claude, ChatGPT, etc.) via MCP integration
- **Automated Systems** implementing MCP client protocol

When designing new tools, ensure they work seamlessly for all three audiences.

### Documentation Requirements

**Every change must update documentation.** When making changes:
1. Update relevant docs in `docs/` directory
2. Update `CLAUDE.md` if architecture or guidelines change
3. Update inline code comments for non-obvious logic
4. Ensure MCP tool/resource changes are reflected in `docs/MCP_REFERENCE.md`

### Code Coverage

**Minimum passing code coverage is 80%.** All new code must:
1. Include comprehensive unit tests
2. Maintain or improve overall coverage
3. Test edge cases and error conditions
4. Use in-memory SQLite for database tests

Run coverage: `go test -v -cover ./...`

---

## Project Overview

**mcp-space-browser** is a disk space indexing agent that crawls filesystems, stores metadata in SQLite, and provides tools for exploring disk utilization (similar to Baobab/WinDirStat).

This is a **Go implementation** providing:
- **3-5x faster** filesystem indexing
- **Single static binary** deployment (no runtime dependencies)
- **Better resource management** and lower memory usage
- **MCP server** with streamable HTTP transport
- **Live filesystem monitoring** with real-time updates using fsnotify
- **Rule-based automation** for automatic file classification and processing

## Essential Commands

#### Build
```bash
# Build CLI and MCP server
go build -o mcp-space-browser ./cmd/mcp-space-browser
```

#### Development
```bash
# Run MCP server with streamable HTTP transport
go run ./cmd/mcp-space-browser server --port=3000
# MCP endpoint: http://localhost:3000/mcp

# Run tests
go test ./...                        # All tests
go test ./pkg/crawler/...            # Specific package
go test -v -cover ./...              # With coverage
```

Note: Indexing is performed via Plans. Create a plan with a `filesystem.index` source and execute it using the `plan-execute` MCP tool.

## Architecture

### Project Structure

```
.
├── cmd/
│   └── mcp-space-browser/    # CLI application and MCP server
├── pkg/
│   ├── logger/                # Structured logging (logrus)
│   ├── database/              # SQLite abstraction
│   ├── crawler/               # Filesystem traversal
│   ├── sources/               # Source abstraction and live filesystem monitoring
│   ├── rules/                 # Rule execution engine for automation
│   ├── classifier/            # Media file classification and thumbnail generation
│   └── server/                # MCP server with streamable HTTP transport
└── internal/
    └── models/                # Shared data structures
```

### Core Components
1. **CLI Entry Point**: Command-line interface for server and utility commands
2. **Filesystem Crawler**: Stack-based DFS traversal, metadata collection, database updates
   - **Performance optimization**: Skips indexing if a path was recently scanned (within configurable `maxAge`, default 1 hour)
   - Use `force=true` in the MCP `index` tool to override and force re-indexing
3. **Database Layer**: SQLite abstraction with multiple tables (entries, resource_sets, sources, rules, plans, etc.)
4. **Resource-Set Management** (DAG-based):
   - Named collections of file/directory entries
   - **DAG structure**: Resource-sets form a Directed Acyclic Graph (multiple parents allowed)
   - **Bidirectional navigation**: `resource-children` and `resource-parent`
   - Pure storage containers (no logic about HOW items got there)
5. **Unified Source Abstraction**:
   - `filesystem.index`: One-time filesystem scans (triggered via Plans)
   - `filesystem.watch`: Real-time monitoring using fsnotify
   - `query`: File filter queries against indexed data
   - `resource-set`: Copy entries from another resource-set
   - All sources target a resource-set and track execution history
6. **Rule Engine**:
   - Evaluates conditions (media type, size, time, path patterns)
   - Applies outcomes (add to resource-sets, generate thumbnails, chain actions)
   - Automatic execution on file changes for live sources
7. **Plans**: Orchestration layer that owns indexing operations
8. **MCP Server**: Streamable HTTP server with tools AND resource templates

### Key Design Patterns
- **Single Table Design**: All filesystem entries in one table with parent references
- **Incremental Updates**: Uses `last_scanned` timestamp to detect/remove stale entries
- **Post-Processing Aggregation**: Directory sizes computed after crawling completes
- **In-Memory Testing**: Tests use temporary directories and `:memory:` SQLite

### Database Schema

**Core Tables:**
```sql
-- Filesystem entries
CREATE TABLE entries (
  id INTEGER PRIMARY KEY,
  path TEXT UNIQUE NOT NULL,
  parent TEXT,
  size INTEGER,
  kind TEXT CHECK(kind IN ('file', 'directory')),
  ctime INTEGER,
  mtime INTEGER,
  last_scanned INTEGER,
  dirty INTEGER DEFAULT 0
)

-- Resource-sets (named collections, supports nesting)
CREATE TABLE resource_sets (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  created_at INTEGER,
  updated_at INTEGER
)

-- Resource-set entries (links sets to filesystem entries)
CREATE TABLE resource_set_entries (
  set_id INTEGER NOT NULL,
  entry_path TEXT NOT NULL,
  added_at INTEGER,
  PRIMARY KEY (set_id, entry_path)
)

-- Resource-set edges (DAG structure, supports multiple parents)
CREATE TABLE resource_set_edges (
  parent_id INTEGER NOT NULL,
  child_id INTEGER NOT NULL,
  added_at INTEGER,
  PRIMARY KEY (parent_id, child_id),
  CHECK (parent_id != child_id)
)

-- Unified sources (filesystem, query, resource-set)
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
)

-- Rules for automatic file processing
CREATE TABLE rules (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  enabled INTEGER DEFAULT 1,
  priority INTEGER DEFAULT 0,
  condition_json TEXT NOT NULL,
  outcome_json TEXT NOT NULL,
  created_at INTEGER,
  updated_at INTEGER
)

-- Plans (orchestration of resource-sets and sources)
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
)
```

### Server Endpoints

> **Note**: This system exposes functionality **exclusively via MCP**. No REST or gRPC APIs.

- `POST /mcp` - MCP streamable HTTP transport endpoint (JSON-RPC 2.0)
- `/web/*` - Static web component microfrontend (uses MCP for data)

## Development Guidelines

#### Runtime Environment
- Go 1.21 or later
- SQLite support via mattn/go-sqlite3
- Single static binary deployment

#### Testing Approach
- Uses Go's built-in testing framework
- Create temporary directories with `t.TempDir()`
- Use in-memory SQLite databases (`:memory:`)
- Run tests with: `GO_ENV=test go test ./...`
- **Minimum 80% code coverage required**: `go test -v -cover ./...`

#### Dependencies
- `github.com/mark3labs/mcp-go@v0.43.0`: MCP server
- `github.com/mattn/go-sqlite3`: SQLite driver
- `github.com/fsnotify/fsnotify`: Filesystem monitoring for live sources
- `github.com/sirupsen/logrus`: Structured logging
- `github.com/spf13/cobra`: CLI framework
- `github.com/gin-gonic/gin`: HTTP server
- `github.com/stretchr/testify`: Testing utilities

#### Logging
- Uses logrus for structured logging with colors
- Logger configuration in `pkg/logger/logger.go`
- Logging levels: trace, debug, info, warn, error
- Silent during tests (GO_ENV=test)
- Set custom log level: `LOG_LEVEL=debug ./mcp-space-browser disk-index /path`
- Each module has its own child logger with contextual name

## MCP Integration

Provides full MCP (Model Context Protocol) integration via **tools** and **resource templates**:

### MCP Tools

**Resource Navigation (DAG):**
- `resource-children`: Get child nodes in DAG (downstream)
- `resource-parent`: Get parent nodes in DAG (upstream, like "..")

**Resource Queries:**
- `resource-sum`: Hierarchical aggregation of a metric (replaces disk-du)
- `resource-time-range`: Filter by time field range (replaces disk-time-range)
- `resource-metric-range`: Filter by metric value range
- `resource-is`: Exact match on a field value (e.g., kind="file")
- `resource-fuzzy-match`: Fuzzy/pattern matching on text fields (contains, prefix, suffix, regex, glob)

**Resource-Set Management:**
- `resource-set-create`, `resource-set-list`, `resource-set-get`, `resource-set-modify`, `resource-set-delete`
- `resource-set-add-child`, `resource-set-remove-child` (DAG edge operations)

**Unified Sources:**
- `source-create`, `source-start`, `source-stop`, `source-list`, `source-get`, `source-delete`, `source-execute`

**Plans (own indexing):**
- `plan-create`, `plan-execute`, `plan-list`, `plan-get`, `plan-update`, `plan-delete`

### MCP Resource Templates

Resources are also accessible via declarative URIs:
```
synthesis://resource-set/{name}
synthesis://resource-set/{name}/children
synthesis://resource-set/{name}/parents
synthesis://resource-set/{name}/entries
synthesis://resource-set/{name}/metrics/{metric}
```

The MCP server exposes tools and resources at `http://localhost:3000/mcp` when running:
```bash
go run ./cmd/mcp-space-browser server --port=3000
```

### Example: Index and Query via Plans

```json
// Step 1: Create resource-sets (DAG nodes)
{"tool": "resource-set-create", "params": {"name": "photos"}}
{"tool": "resource-set-create", "params": {"name": "all-media"}}
{"tool": "resource-set-add-child", "params": {"parent": "all-media", "child": "photos"}}

// Step 2: Create and execute a plan to index
{
  "tool": "plan-create",
  "params": {
    "name": "index-photos",
    "sources": [
      {"name": "photos-source", "type": "filesystem.index", "target_set": "photos",
       "config": {"path": "/home/user/Photos", "recursive": true}}
    ]
  }
}
{"tool": "plan-execute", "params": {"name": "index-photos"}}

// Step 3: Query resources
{"tool": "resource-sum", "params": {"name": "all-media", "metric": "size"}}
{"tool": "resource-time-range", "params": {"name": "photos", "field": "mtime", "min": "2024-01-01"}}
{"tool": "resource-children", "params": {"name": "all-media"}}
```

### Live Filesystem Monitoring

For real-time monitoring, use a `filesystem.watch` source in a plan:
```json
{
  "tool": "plan-create",
  "params": {
    "name": "watch-downloads",
    "mode": "continuous",
    "sources": [
      {"name": "downloads-watch", "type": "filesystem.watch", "target_set": "downloads",
       "config": {"path": "/home/user/Downloads", "recursive": true, "debounce_ms": 500}}
    ]
  }
}
```

Live sources automatically:
- Index new files as they're created
- Update metadata when files are modified
- Remove entries when files are deleted
- Execute rules for automatic classification
- Populate the target resource-set

### Architecture Documentation

- `docs/ARCHITECTURE_C3.md` - C3 Context/Container views of the system
- `docs/ENTITY_RELATIONSHIP.md` - Entity-Relationship Diagram of the data model
- `docs/MCP_REFERENCE.md` - Complete MCP tool and resource template reference
- `docs/RESOURCE_SET_ARCHITECTURE.md` - Detailed architecture for resource-sets, unified sources, and plans
- `docs/RESOURCE_SET_IMPLEMENTATION_PLAN.md` - Implementation roadmap with detailed tasks
- `docs/PLANS_ARCHITECTURE.md` - Plans system architecture