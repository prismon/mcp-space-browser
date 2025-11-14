# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**mcp-space-browser** is a disk space indexing agent that crawls filesystems, stores metadata in SQLite, and provides tools for exploring disk utilization (similar to Baobab/WinDirStat).

This repository contains **two implementations**:

1. **TypeScript/Bun** (original): Located in `src/` directory
2. **Go** (new): Located in `cmd/`, `pkg/`, and `internal/` directories

**Default implementation going forward: Go**

### Why Go?

The Go implementation provides:
- **3-5x faster** filesystem indexing
- **Single static binary** deployment (no runtime dependencies)
- **Better resource management** and lower memory usage
- **Full feature parity** with TypeScript version

## Essential Commands

### Go Implementation (Recommended)

#### Build
```bash
# Build unified CLI and server
go build -o mcp-space-browser ./cmd/mcp-space-browser

# Note: cmd/mcp-server is deprecated - use unified server instead
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

### TypeScript/Bun Implementation (Legacy)

#### Development
```bash
# Run CLI commands
bun src/cli.ts disk-index <path>    # Index a directory tree
bun src/cli.ts disk-du <path>        # Show disk usage for path
bun src/cli.ts disk-tree <path>      # Display tree view with sizes

# Run HTTP server
bun src/server.ts                    # Starts on port 3000

# Run tests
bun test                             # Runs all tests
bun test test/agent.test.ts          # Run specific test file
```

## Architecture

### Go Implementation Structure

```
.
├── cmd/
│   ├── mcp-space-browser/    # CLI application and unified server
│   └── mcp-server/            # DEPRECATED: Standalone MCP server
├── pkg/
│   ├── logger/                # Structured logging (logrus)
│   ├── database/              # SQLite abstraction
│   ├── crawler/               # Filesystem traversal
│   └── server/                # Unified HTTP server (REST API + MCP)
├── internal/
│   └── models/                # Shared data structures
└── test/
```

### TypeScript Implementation Structure

```
.
├── src/
│   ├── cli.ts         # CLI entry point
│   ├── crawler.ts     # Filesystem crawler
│   ├── db.ts          # Database layer
│   ├── server.ts      # HTTP server
│   ├── mcp.ts         # MCP server
│   └── logger.ts      # Logging setup
└── test/
```

### Core Components (Both Implementations)
1. **CLI Entry Point**: Command-line interface with disk-index, disk-du, disk-tree commands
2. **Filesystem Crawler**: Stack-based DFS traversal, metadata collection, database updates
3. **Database Layer**: SQLite abstraction with 5 tables (entries, selection_sets, queries, etc.)
4. **Unified Server** (Go only): Single HTTP server providing:
   - REST API endpoints (`/api/index`, `/api/tree`)
   - MCP endpoint (`/mcp`) with 17 tools for disk space analysis
5. **MCP Server** (TypeScript): Separate MCP server (17 tools via Model Context Protocol)

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

### Go Implementation

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

### TypeScript Implementation (Legacy)

#### Runtime Environment
- Uses Bun runtime exclusively (not Node.js)
- Bun provides built-in SQLite support via `bun:sqlite`
- TypeScript runs directly without compilation step

#### Testing Approach
- Tests use Bun's built-in test runner (`bun:test`)
- Create temporary directories with `withTempDir` helper
- Use in-memory SQLite databases for test isolation

#### Logging
- Uses Pino for structured logging with `pino-pretty` formatter
- Logger configuration in `src/logger.ts`
- Set custom log level: `LOG_LEVEL=debug bun src/cli.ts disk-index /path`

## MCP Integration

Both implementations provide full MCP (Model Context Protocol) integration with 17 tools:

**Core Tools**: disk-index, disk-du, disk-tree, disk-time-range
**Selection Sets**: create, list, get, modify, delete
**Queries**: create, execute, list, get, update, delete
**Session**: info, set-preferences

### Go Implementation
The unified server exposes MCP tools at `http://localhost:3000/mcp` when running:
```bash
go run ./cmd/mcp-space-browser server --port=3000
```

### TypeScript Implementation
The MCP server runs as a separate process (TypeScript implementation only):
```bash
bun src/mcp.ts
```

See `README.go.md` for complete tool documentation.