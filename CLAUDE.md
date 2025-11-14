# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**mcp-space-browser** is a disk space indexing agent that crawls filesystems, stores metadata in SQLite, and provides tools for exploring disk utilization (similar to Baobab/WinDirStat).

This is a **Go implementation** providing:
- **3-5x faster** filesystem indexing
- **Single static binary** deployment (no runtime dependencies)
- **Better resource management** and lower memory usage
- **Unified server** exposing both REST API and MCP endpoints

## Essential Commands

#### Build
```bash
# Build unified CLI and server
go build -o mcp-space-browser ./cmd/mcp-space-browser
```

#### Development
```bash
# Run CLI commands directly
go run ./cmd/mcp-space-browser disk-index <path>
go run ./cmd/mcp-space-browser disk-du <path>
go run ./cmd/mcp-space-browser disk-tree <path>

# Run unified HTTP server (provides both REST API and MCP endpoints)
go run ./cmd/mcp-space-browser server --port=3000
# REST API: http://localhost:3000/api/*
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
│   └── mcp-space-browser/    # CLI application and unified server (REST API + MCP)
├── pkg/
│   ├── logger/                # Structured logging (logrus)
│   ├── database/              # SQLite abstraction
│   ├── crawler/               # Filesystem traversal
│   └── server/                # Unified HTTP server (REST API + MCP)
└── internal/
    └── models/                # Shared data structures
```

### Core Components
1. **CLI Entry Point**: Command-line interface with disk-index, disk-du, disk-tree commands
2. **Filesystem Crawler**: Stack-based DFS traversal, metadata collection, database updates
3. **Database Layer**: SQLite abstraction with 5 tables (entries, selection_sets, queries, etc.)
4. **Unified Server**: Single HTTP server providing:
   - REST API endpoints (`/api/index`, `/api/tree`)
   - MCP endpoint (`/mcp`) with 17 tools for disk space analysis

### Key Design Patterns
- **Single Table Design**: All filesystem entries in one table with parent references
- **Incremental Updates**: Uses `last_scanned` timestamp to detect/remove stale entries
- **Post-Processing Aggregation**: Directory sizes computed after crawling completes
- **In-Memory Testing**: Tests use temporary directories and `:memory:` SQLite

### Database Schema
```sql
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
```

### API Endpoints
- `GET /api/index?path=...` - Trigger filesystem indexing
- `GET /api/tree?path=...` - Get hierarchical JSON data

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

Provides full MCP (Model Context Protocol) integration with 17 tools:

**Core Tools**: disk-index, disk-du, disk-tree, disk-time-range
**Selection Sets**: create, list, get, modify, delete
**Queries**: create, execute, list, get, update, delete
**Session**: info, set-preferences

The unified server exposes MCP tools at `http://localhost:3000/mcp` when running:
```bash
go run ./cmd/mcp-space-browser server --port=3000
```

See `README.go.md` for complete tool documentation.