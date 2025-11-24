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
3. **Database Layer**: SQLite abstraction with multiple tables (entries, selection_sets, queries, sources, rules, etc.)
4. **Source Management**:
   - **Manual Sources**: One-time filesystem scans
   - **Live Sources**: Real-time monitoring using fsnotify (watches for Create, Modify, Delete, Rename events)
   - **Source Manager**: Manages multiple sources, lifecycle, and persistence
5. **Rule Engine**:
   - Evaluates conditions (media type, size, time, path patterns)
   - Applies outcomes (add to selection sets, generate thumbnails, chain actions)
   - Automatic execution on file changes for live sources
6. **MCP Server**: Streamable HTTP server providing MCP endpoint (`/mcp`) with 24+ tools for disk space analysis and source management

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

-- Filesystem sources (manual or live monitoring)
CREATE TABLE sources (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  type TEXT CHECK(type IN ('manual', 'live', 'scheduled')) NOT NULL,
  root_path TEXT NOT NULL,
  config_json TEXT,
  status TEXT CHECK(status IN ('stopped', 'starting', 'running', 'stopping', 'error')),
  enabled INTEGER DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
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
**Selection Sets**: create, list, get, modify, delete
**Queries**: create, execute, list, get, update, delete
**Source Management**: source-create, source-start, source-stop, source-list, source-get, source-delete, source-stats
**Session**: info, set-preferences

The MCP server exposes tools and resources at `http://localhost:3000/mcp` when running:
```bash
go run ./cmd/mcp-space-browser server --port=3000
```

### Live Filesystem Monitoring

Create a live source to monitor a directory for changes:
```json
{
  "name": "my-photos",
  "type": "live",
  "path": "/home/user/Photos",
  "watch_recursive": true,
  "debounce_ms": 500
}
```

Live sources automatically:
- Index new files as they're created
- Update metadata when files are modified
- Remove entries when files are deleted
- Execute rules for automatic classification and processing

Note: Use rules to filter which files to process rather than ignore patterns. This provides more flexibility for conditional processing based on file type, size, path patterns, and other criteria.

See `README.go.md` for complete tool documentation.