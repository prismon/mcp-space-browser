# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
# Run CLI commands directly
go run ./cmd/mcp-space-browser disk-index <path>
go run ./cmd/mcp-space-browser disk-du <path>
go run ./cmd/mcp-space-browser disk-tree <path>

# Run MCP server with streamable HTTP transport
go run ./cmd/mcp-space-browser server --port=3000
# MCP endpoint: http://localhost:3000/mcp

# Run tests
go test ./...                        # All tests
go test ./pkg/crawler/...            # Specific package
go test -v -cover ./...              # With coverage
```

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
1. **CLI Entry Point**: Command-line interface with disk-index, disk-du, disk-tree commands
2. **Filesystem Crawler**: Stack-based DFS traversal, metadata collection, database updates
3. **Database Layer**: SQLite abstraction with multiple tables (entries, resource_sets, sources, rules, plans, etc.)
4. **Resource-Set Management** (formerly Selection Sets):
   - Named collections of file/directory entries
   - **Nesting support**: Resource-sets can contain other resource-sets
   - Pure storage containers (no logic about HOW items got there)
5. **Unified Source Abstraction**:
   - `filesystem.index`: One-time filesystem scans
   - `filesystem.watch`: Real-time monitoring using fsnotify
   - `query`: File filter queries against indexed data
   - `resource-set`: Copy entries from another resource-set
   - All sources target a resource-set and track execution history
6. **Rule Engine**:
   - Evaluates conditions (media type, size, time, path patterns)
   - Applies outcomes (add to resource-sets, generate thumbnails, chain actions)
   - Automatic execution on file changes for live sources
7. **Plans**: Orchestration layer combining resource-sets with sources
8. **MCP Server**: Streamable HTTP server providing MCP endpoint (`/mcp`) with 24+ tools for disk space analysis and source management

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

-- Resource-set children (nesting support)
CREATE TABLE resource_set_children (
  parent_id INTEGER NOT NULL,
  child_id INTEGER NOT NULL,
  added_at INTEGER,
  PRIMARY KEY (parent_id, child_id)
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
- `POST /mcp` - MCP streamable HTTP transport endpoint
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

Provides full MCP (Model Context Protocol) integration with 24+ tools:

**Core Tools**: disk-index, disk-du, disk-tree, disk-time-range, navigate
**Resource-Sets**: resource-set-create, resource-set-list, resource-set-get, resource-set-modify, resource-set-delete, resource-set-add-child, resource-set-remove-child, resource-set-get-all
**Unified Sources**: source-create, source-start, source-stop, source-list, source-get, source-delete, source-execute, source-stats
**Plans**: plan-create, plan-execute, plan-list, plan-get, plan-update, plan-delete
**Session**: info, set-preferences

The MCP server exposes tools and resources at `http://localhost:3000/mcp` when running:
```bash
go run ./cmd/mcp-space-browser server --port=3000
```

### Live Filesystem Monitoring

Create a filesystem.watch source to monitor a directory for changes:
```json
{
  "name": "watch-photos",
  "type": "filesystem.watch",
  "target_set": "photos",
  "config": {
    "path": "/home/user/Photos",
    "recursive": true,
    "debounce_ms": 500
  }
}
```

Live sources automatically:
- Index new files as they're created
- Update metadata when files are modified
- Remove entries when files are deleted
- Execute rules for automatic classification and processing
- Populate the target resource-set with matched entries

Note: Use rules to filter which files to process rather than ignore patterns. This provides more flexibility for conditional processing based on file type, size, path patterns, and other criteria.

### Architecture Documentation

- `docs/RESOURCE_SET_ARCHITECTURE.md` - Detailed architecture for resource-sets, unified sources, and plans
- `docs/RESOURCE_SET_IMPLEMENTATION_PLAN.md` - Implementation roadmap with detailed tasks
- `docs/PLANS_ARCHITECTURE.md` - Plans system architecture

See `README.go.md` for complete tool documentation.