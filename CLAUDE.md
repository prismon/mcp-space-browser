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

**mcp-space-browser** is a disk space indexing agent that crawls filesystems, stores metadata in SQLite, and provides **5 composable MCP tools** for exploring disk utilization (similar to Baobab/WinDirStat).

This is a **Go implementation** providing:
- **5 composable MCP tools** replacing 59 specialized ones (scan, query, manage, batch, watch)
- **Unified metadata system** with key-value pairs and classifier artifacts per entry
- **Single static binary** deployment (no runtime dependencies)
- **MCP server** with streamable HTTP transport
- **Live filesystem monitoring** with real-time updates using fsnotify
- **Cursor-based pagination** for LLM-friendly result navigation

## Essential Commands

#### Build
```bash
go build -o mcp-space-browser ./cmd/mcp-space-browser
```

#### Development
```bash
# Run MCP server
go run ./cmd/mcp-space-browser server --port=3000
# MCP endpoint: http://localhost:3000/mcp

# Run tests
go test ./...                        # All tests
go test ./pkg/server/...             # Server package (tools + resources)
go test -v -cover ./...              # With coverage
```

## Architecture

### Project Structure

```
.
├── cmd/
│   └── mcp-space-browser/    # CLI application and MCP server
├── pkg/
│   ├── server/                # MCP server: 5 tools + 8 resource templates
│   ├── database/              # SQLite abstraction (entries, metadata, sets, plans)
│   ├── crawler/               # Filesystem traversal (stack-based DFS)
│   ├── sources/               # Source abstraction and live monitoring (fsnotify)
│   ├── rules/                 # Rule execution engine for automation
│   ├── classifier/            # Media file classification and thumbnails
│   └── logger/                # Structured logging (logrus)
└── internal/
    └── models/                # Shared data structures (Entry, MetadataRecord, Plan, etc.)
```

### Core Components
1. **5 MCP Tools** (`pkg/server/tool_*.go`):
   - `scan` — Index filesystem paths, extract metadata, auto-generate thumbnails
   - `query` — Search, filter, aggregate with dynamic SQL and cursor pagination
   - `manage` — CRUD for resource-sets, plans, and jobs
   - `batch` — Multi-file operations (attributes, duplicates, move, delete)
   - `watch` — Real-time filesystem monitoring via fsnotify
2. **Metadata System** (`pkg/database/metadata.go`, `internal/models/metadata.go`):
   - Unified table for simple key-value pairs and classifier artifacts
   - Simple metadata keys: `mime`, `hash.md5`, `hash.sha256`, `permissions`, etc.
   - Artifact metadata: `thumbnail`, `video-timeline`, `metadata` (with hash-based dedup)
   - Queryable via the `query` tool's `where` clause (auto-JOINs on non-entry columns)
3. **Post-Processor** (`pkg/server/post_processor.go`):
   - Runs automatically after scan completes (both sync and async)
   - Default extractions: `mime`, `permissions`, `thumbnail`, `video.thumbnails`, `metadata`
   - Opt-in: `hash.md5`, `hash.sha256` (slow for large files)
   - Concurrent worker pool (capped at 8), error-tolerant per file
   - `attributes` parameter acts as filter (omit for all defaults)
4. **Filesystem Crawler**: Stack-based DFS with bottom-up size aggregation
   - Skips re-indexing recent paths (configurable `maxAge`, default 1 hour)
   - Use `force=true` in `scan` tool to override
5. **Resource-Set DAG**: Named collections with parent-child edges (multiple parents allowed)
6. **Plans**: Orchestration layer for indexing operations
7. **8 Resource Templates**: Declarative `synthesis://` URIs for entries, sets, jobs, projects

### Key Design Patterns
- **Composable Tool Interface**: 5 tools with structured parameters replace 59 specialized tools
- **Metadata-Based Filtering**: `query` WHERE clause handles both entry columns and metadata transparently
- **Cursor Pagination**: Base64-encoded offset tokens for stateless pagination
- **Entries + Metadata**: Entries table for core data, unified metadata table for key-value pairs and classifier artifacts
- **Bottom-Up Aggregation**: Directory sizes computed after crawling (deepest-first)
- **In-Memory Testing**: All tests use `:memory:` SQLite (with `MaxOpenConns(1)` for connection safety)

### Database Schema

See `docs/SCHEMA.md` for complete schema. Key tables:

```sql
-- Core data
entries (path, parent, size, kind, ctime, mtime, last_scanned)
metadata (entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash)  -- unified metadata

-- Organization
resource_sets (name, description)
resource_set_entries (set_id, entry_path)
resource_set_edges (parent_id, child_id)  -- DAG structure

-- Orchestration
plans (name, mode, sources_json, outcomes_json)
sources (name, type, target_set_name, config_json, status)
rules (name, condition_json, outcome_json)
index_jobs (root_path, status, files_found, files_indexed)
```

### Server Endpoints

> **Note**: This system exposes functionality **exclusively via MCP**. No REST or gRPC APIs.

- `POST /mcp` — MCP streamable HTTP transport endpoint (JSON-RPC 2.0)
- `/web/*` — Static web component microfrontend (uses MCP for data)

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
- Tool tests call handler functions directly (MCPServer doesn't expose CallTool)
- Shared test helpers: `makeRequest()` and `resultJSON()` in `tool_scan_test.go`

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
- Set custom log level: `LOG_LEVEL=debug ./mcp-space-browser server`
- Each module has its own child logger with contextual name

## MCP Integration

### 5 MCP Tools

| Tool | Description |
|------|-------------|
| `scan` | Index filesystem paths and extract attributes |
| `query` | Search, filter, aggregate with composable WHERE clause |
| `manage` | CRUD for resource-sets, plans, and jobs |
| `batch` | Multi-file operations: attributes, duplicates, move, delete |
| `watch` | Real-time filesystem monitoring (start, stop, status, list) |

See `docs/MCP_REFERENCE.md` for complete parameter reference and examples.

### 8 Resource Templates

| URI | Description |
|-----|-------------|
| `synthesis://entries/{path}` | Entry with attributes |
| `synthesis://entries/{path}/attributes` | Attributes only |
| `synthesis://sets` | List resource sets |
| `synthesis://sets/{name}` | Resource set details |
| `synthesis://sets/{name}/entries` | Entries in a set |
| `synthesis://jobs` | List indexing jobs |
| `synthesis://jobs/{id}` | Job details |
| `synthesis://projects` | List projects |

### Example: Index and Query

```json
// Scan a directory
{"tool": "scan", "params": {"paths": ["/home/user/Photos"], "target": "photos"}}

// Find large files
{"tool": "query", "params": {"where": {"kind": "file", "size": {">": 100000000}}, "order_by": "-size", "limit": 10}}

// Aggregate by type
{"tool": "query", "params": {"from": "photos", "aggregate": "count", "group_by": "mime"}}

// Organize with DAG
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "name": "media"}}
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "parent": "media", "child": "photos"}}
```

### Live Filesystem Monitoring

```json
{"tool": "watch", "params": {"action": "start", "path": "/home/user/Downloads", "name": "dl-watch"}}
{"tool": "watch", "params": {"action": "list"}}
{"tool": "watch", "params": {"action": "stop", "name": "dl-watch"}}
```

### Architecture Documentation

- `docs/ARCHITECTURE.md` — System architecture and component diagram
- `docs/MCP_REFERENCE.md` — Complete MCP tool and resource template reference
- `docs/SCHEMA.md` — Database schema reference
