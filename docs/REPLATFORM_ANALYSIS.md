# MCP Space Browser - Go Replatforming Analysis

## Executive Summary

**mcp-space-browser** is a disk space indexing agent that crawls filesystems, stores metadata in SQLite, and exposes tools through the Model Context Protocol (MCP). The current implementation is written in Bun/TypeScript (~3,471 lines) and needs to be replatformed to Go.

---

## Project Overview

### Purpose
A disk space indexing agent similar to Baobab/WinDirStat that:
- Recursively indexes directory trees with DFS traversal
- Stores file/directory metadata in SQLite
- Provides query and analysis tools
- Exposes capabilities through the Model Context Protocol (MCP)
- Supports saved queries and selection sets (file groupings)

### Current Runtime
- **Runtime**: Bun (JavaScript runtime with native SQLite support)
- **Language**: TypeScript
- **Total Code**: ~3,471 lines across 8 main modules + 11 test files
- **Version**: 0.1.0

---

## Module Breakdown & Replatforming Requirements

### 1. **Logger Module** (`src/logger.ts`) - 19 lines
**Purpose**: Structured logging with Pino

**Current Implementation**:
```typescript
- Pino logger with pino-pretty formatter
- Log levels: trace, debug, info, warn, error
- Silent during tests (NODE_ENV=test)
- Child loggers with contextual names
- Pretty printing with colors and timestamps
```

**Go Equivalent**:
- Use `github.com/sirupsen/logrus` or `github.com/rs/zerolog` for structured logging
- Support log level configuration via `LOG_LEVEL` env var
- Implement pretty printing for development

---

### 2. **Database Layer** (`src/db.ts`) - 635 lines
**Purpose**: SQLite abstraction and data persistence

**Database Schema**:
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
CREATE INDEX idx_parent ON entries(parent)
CREATE INDEX idx_mtime ON entries(mtime)

CREATE TABLE selection_sets (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  criteria_type TEXT CHECK(criteria_type IN ('user_selected', 'tool_query')),
  criteria_json TEXT,
  created_at INTEGER DEFAULT (strftime('%s', 'now')),
  updated_at INTEGER DEFAULT (strftime('%s', 'now'))
)

CREATE TABLE selection_set_entries (
  set_id INTEGER NOT NULL,
  entry_path TEXT NOT NULL,
  added_at INTEGER DEFAULT (strftime('%s', 'now')),
  PRIMARY KEY (set_id, entry_path),
  FOREIGN KEY (set_id) REFERENCES selection_sets(id) ON DELETE CASCADE,
  FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
)
CREATE INDEX idx_set_entries ON selection_set_entries(set_id)

CREATE TABLE queries (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  query_type TEXT CHECK(query_type IN ('file_filter', 'custom_script')),
  query_json TEXT NOT NULL,
  target_selection_set TEXT,
  update_mode TEXT CHECK(update_mode IN ('replace', 'append', 'merge')) DEFAULT 'replace',
  created_at INTEGER DEFAULT (strftime('%s', 'now')),
  updated_at INTEGER DEFAULT (strftime('%s', 'now')),
  last_executed INTEGER,
  execution_count INTEGER DEFAULT 0
)

CREATE TABLE query_executions (
  id INTEGER PRIMARY KEY,
  query_id INTEGER NOT NULL,
  executed_at INTEGER DEFAULT (strftime('%s', 'now')),
  duration_ms INTEGER,
  files_matched INTEGER,
  status TEXT CHECK(status IN ('success', 'error')),
  error_message TEXT,
  FOREIGN KEY (query_id) REFERENCES queries(id) ON DELETE CASCADE
)
CREATE INDEX idx_query_executions ON query_executions(query_id, executed_at DESC)
```

**Data Structures**:
```typescript
interface Entry {
  id?: number
  path: string
  parent: string | null
  size: number
  kind: 'file' | 'directory'
  ctime: number
  mtime: number
  last_scanned: number
}

interface SelectionSet {
  id?: number
  name: string
  description?: string
  criteria_type: 'user_selected' | 'tool_query'
  criteria_json?: string
  created_at?: number
  updated_at?: number
}

interface Query {
  id?: number
  name: string
  description?: string
  query_type: 'file_filter' | 'custom_script'
  query_json: string
  target_selection_set?: string
  update_mode?: 'replace' | 'append' | 'merge'
  created_at?: number
  updated_at?: number
  last_executed?: number
  execution_count?: number
}

interface FileFilter {
  path?: string
  pattern?: string
  extensions?: string[]
  minSize?: number
  maxSize?: number
  minDate?: string
  maxDate?: string
  nameContains?: string
  pathContains?: string
  sortBy?: 'size' | 'name' | 'mtime'
  descendingSort?: boolean
  limit?: number
}
```

**Key Methods** (200+ methods across CRUD, aggregation, filtering):
- Entry management: `get()`, `insertOrUpdate()`, `children()`, `deleteStale()`, `computeAggregates()`
- SelectionSet management: `createSelectionSet()`, `listSelectionSets()`, `addToSelectionSet()`, `removeFromSelectionSet()`, `getSelectionSetEntries()`, `getSelectionSetStats()`
- Query management: `createQuery()`, `listQueries()`, `updateQuery()`, `executeQuery()`, `getQueryExecutions()`
- File filtering: `executeFileFilter()` with regex, size, date, extension, and name filtering
- Transaction support: `beginTransaction()`, `commitTransaction()`, `rollbackTransaction()`

**Go Equivalent**:
- Use `github.com/mattn/go-sqlite3` or `github.com/glebarez/sqlite` for database access
- Implement equivalent GORM models or raw SQL with prepared statements
- Type definitions via Go structs
- Proper connection pooling and transaction handling

---

### 3. **Filesystem Crawler** (`src/crawler.ts`) - 105 lines
**Purpose**: Recursive filesystem indexing with DFS traversal

**Algorithm**:
1. Uses stack-based DFS traversal (not recursive, to handle deep trees)
2. For each path:
   - Call `fs.stat()` to get metadata (size, times, type)
   - Insert/update database entry
   - If directory, push children onto stack
3. After traversal:
   - Delete stale entries (files not seen in this scan)
   - Compute aggregate sizes (directory sizes = sum of children)

**Key Functions**:
- `index(root: string, db: DiskDB)`: Main async indexing function
  - Tracks: filesProcessed, directoriesProcessed, totalSize, errors
  - Progress logging every 5 seconds
  - Transaction-based operations for performance
  - Runs in ~O(n) time where n = number of entries

**Go Equivalent**:
- Use `os.Stat()` or `filepath.Walk()` for traversal
- Implement similar progress tracking
- Consider parallel workers for performance improvements
- Batch database operations

---

### 4. **CLI Interface** (`src/cli.ts`) - 176 lines
**Purpose**: Command-line interface with three main commands

**Commands**:
1. `disk-index <path>` - Index a directory tree
   - Trigger the crawler to scan and store filesystem metadata
   - Non-blocking operation with progress logging

2. `disk-du <path>` - Show disk usage
   - Query database for total size of a path
   - Returns single integer (bytes)

3. `disk-tree <path> [options]` - Display tree view
   - Options:
     - `--sort-by=<size|mtime|name>`: Sort criteria
     - `--ascending`: Sort ascending (default: descending)
     - `--min-date=<YYYY-MM-DD>`: Filter by date
     - `--max-date=<YYYY-MM-DD>`: Filter by date
   - Recursive tree display with indentation
   - Format: `name (size) [date]`

**Go Equivalent**:
- Use `github.com/spf13/cobra` or `urfave/cli` for CLI
- Implement same command structure
- Support flags/options parsing

---

### 5. **HTTP Server** (`src/server.ts`) - 61 lines
**Purpose**: REST API for disk space tools

**Endpoints**:
1. `GET /api/index?path=...`
   - Triggers filesystem indexing
   - Returns "OK" immediately (async operation)
   - Logs completion/errors in background

2. `GET /api/tree?path=...`
   - Returns JSON tree structure
   - Format: `{path, size, children: []}`
   - Recursive hierarchical representation

**Go Equivalent**:
- Use `gin-gonic/gin` or `gorilla/mux` for routing
- Implement same endpoints
- Support JSON response marshaling

---

### 6. **MCP Server Implementation** (`src/mcp.ts`) - 825 lines
**Purpose**: Main MCP (Model Context Protocol) server with 20+ tools

**MCP Framework**: FastMCP (Bun TypeScript SDK wrapper)

**Tools Exposed** (20+ total):

**Core Indexing Tools**:
- `disk-index`: Index filesystem at path
- `disk-du`: Get disk usage for path
- `disk-tree`: Get tree view with pagination, grouping, sorting, filtering
- `disk-time-range`: Get oldest/newest files

**Selection Set Management** (file groupings):
- `selection-set-create`: Create named selection set, optionally from tool results
- `selection-set-list`: List all selection sets with stats
- `selection-set-get`: Get entries in a selection set
- `selection-set-modify`: Add/remove paths from selection set
- `selection-set-delete`: Delete selection set

**Query Management** (saved filters):
- `query-create`: Create persistent file filter query
- `query-execute`: Execute saved query by name
- `query-list`: List available queries
- `query-get`: Get query details and execution history
- `query-update`: Update query criteria
- `query-delete`: Delete query

**Session Management** (optional):
- `session-info`: Get current session info
- `session-set-preferences`: Set session preferences (maxResults, sortBy)

**Advanced Features**:
- Tree grouping by extension (extension map with statistics)
- Pagination support (offset, pageSize, hasMore)
- Multiple filtering criteria:
  - Size ranges (minSize, maxSize)
  - Date ranges (minDate, maxDate)
  - Name/path contains
  - Regex patterns
  - File extensions
- Multiple sort options: size, name, mtime
- Limit parameter for result capping
- Depth control for tree traversal

**MCP Tool Parameter Schema** (using Zod):
- All tools use Zod schemas for validation
- Parameters are type-checked and described
- Error handling with try-catch in tool execution

**Go Equivalent**:
- Consider `go-echarts/go-echarts` or similar for alternative implementations
- Use `go-chi/chi` or `gorilla/mux` with JSON-RPC 2.0 for MCP
- Implement FastMCP equivalent or use MCP Go SDK when available
- Parameter validation with `go-playground/validator` or similar

---

### 7. **MCP Simple** (`src/mcp-simple.ts`) - 51 lines
**Purpose**: Minimal MCP server with basic tools

**Tools**: 
- `disk-index`
- `disk-du`

**Note**: Appears to be a test/reference implementation

---

## Dependencies & External Integrations

### NPM Dependencies:
```json
{
  "fastmcp": "^3.8.5",      // MCP server framework
  "pino": "^9.7.0",         // Structured logging
  "pino-pretty": "^13.0.0",  // Log formatting
  "zod": "^3.25.76"         // Schema validation
}
```

### Built-in Bun APIs Used:
- `bun:sqlite` - SQLite database (Bun's native binding)
- `fs` and `fs/promises` - Filesystem operations
- `path` - Path utilities
- Native HTTP server (`Bun.serve()`)

### Go Equivalents:
- `github.com/glebarez/sqlite` or `github.com/mattn/go-sqlite3` - SQLite
- `os`, `io/ioutil`, `path/filepath` - Filesystem
- Built-in `net/http` - HTTP server
- Optional: `github.com/sirupsen/logrus` or `github.com/rs/zerolog` - Logging
- Optional: `github.com/go-playground/validator` - Validation

---

## Test Coverage

### Test Files (11 total, ~750+ lines):
1. **agent.test.ts** - Basic crawler and aggregation tests
2. **mcp.test.ts** - MCP tool execution tests
3. **mcp-server.test.ts** - HTTP server tests
4. **test-pagination.ts** - Pagination logic tests
5. **test-session.ts** - Session state tests
6. **test-queries.ts** - Query creation and execution tests
7. **test-selection-sets.ts** - Selection set CRUD tests
8. **test-mcp-selection.ts** - MCP selection set tools tests
9. **check-sets.ts** - Selection set verification script
10. **create-log-query.ts** - Query creation helper
11. **create-log-selection.ts** - Selection set creation helper

### Test Patterns:
- Use temporary directories (`withTempDir` helper)
- In-memory SQLite databases (`:memory:`)
- Bun's `bun:test` test runner
- Full lifecycle testing (create → modify → delete)

### Go Testing Approach:
- Use `testing.T` and subtests
- Temporary directories with `t.TempDir()`
- In-memory SQLite or temporary file-based databases
- Table-driven tests for comprehensive coverage

---

## Key Design Patterns to Preserve

### 1. **Single Table Design**
- All entries in one `entries` table with parent references
- Prevents joins and improves query simplicity
- Not normalized, but efficient for this use case

### 2. **Last Scanned Tracking**
- Every scan updates `last_scanned` timestamp
- Enables incremental updates and stale entry detection
- Allows multiple roots in same database

### 3. **Post-Processing Aggregation**
- Directory sizes computed AFTER crawling completes
- Uses `computeAggregates()` to sum child sizes
- Transactional update for consistency

### 4. **Transaction-Based Operations**
- Batch inserts use `BEGIN TRANSACTION`
- Improves performance (10-100x faster than individual inserts)
- Includes rollback on error

### 5. **Stack-Based DFS (Not Recursive)**
- Uses array/stack to avoid stack overflow on deep trees
- O(n) time complexity, O(h) space (h = height)
- Allows processing extremely deep directory structures

### 6. **In-Memory Database Support**
- Tests use `:memory:` SQLite databases
- Enables fast, isolated testing without filesystem pollution

---

## Configuration & Environment Variables

### Current Environment Variables:
- `NODE_ENV=test` - Silences logging during tests
- `LOG_LEVEL` - Sets log level (default: info)
- `PORT` - HTTP server port (default: 3000 for server.ts, 8080 for MCP)
- `HOST` - HTTP server host (default: 0.0.0.0)

---

## Data Flow & Component Interactions

```
┌─────────────────────┐
│   CLI / HTTP / MCP  │ (Entry points)
└──────────┬──────────┘
           │
┌──────────▼──────────┐
│     Crawler         │ (index() function)
│  - DFS traversal    │
│  - fs.stat() calls  │
└──────────┬──────────┘
           │
┌──────────▼──────────┐
│  Database Layer     │ (DiskDB class)
│  - Entry CRUD       │
│  - Aggregation      │
│  - Query execution  │
│  - Selection sets   │
└──────────┬──────────┘
           │
┌──────────▼──────────┐
│   SQLite Database   │ (disk.db file)
│  - entries table    │
│  - selection_sets   │
│  - queries          │
└─────────────────────┘
```

### Typical Operation Flow:
1. User invokes CLI/HTTP/MCP endpoint
2. Call `DiskDB.index(path)` → triggers `crawler.index()`
3. Crawler walks filesystem, inserting entries via `insertOrUpdate()`
4. After crawl completes:
   - `deleteStale()` removes old entries
   - `computeAggregates()` updates directory sizes
5. Query operations filter/sort the database entries
6. Results returned to user

---

## Performance Considerations

### Current Optimizations:
- Batch transactions for inserts (10-100x faster)
- Prepared statements for repeated queries
- Indexes on `parent` and `mtime` columns
- Post-processing aggregation (single pass)
- Stack-based traversal (avoids stack overflow)

### Potential Go Improvements:
- Parallel filesystem scanning with goroutines
- Better connection pooling
- Memory-mapped SQLite files
- Caching of frequently accessed paths
- Concurrent query execution

---

## Replatforming Checklist

### Phase 1: Core Infrastructure
- [ ] Database layer with SQLite and equivalent schema
- [ ] Logger implementation with parity
- [ ] Filesystem crawler with DFS traversal
- [ ] Entry/SelectionSet/Query data structures

### Phase 2: CLI & HTTP
- [ ] CLI commands (disk-index, disk-du, disk-tree)
- [ ] HTTP server with /api/index and /api/tree endpoints
- [ ] Options/flags parsing

### Phase 3: MCP Server
- [ ] MCP server framework integration
- [ ] Tool definitions and schemas
- [ ] All 20+ tool implementations
- [ ] Session management

### Phase 4: Testing
- [ ] Unit tests for crawler
- [ ] Database CRUD tests
- [ ] MCP tool tests
- [ ] Integration tests
- [ ] Test helpers for temp dirs and in-memory DBs

### Phase 5: Polish
- [ ] Error handling parity
- [ ] Logging output parity
- [ ] Performance testing
- [ ] Documentation

---

## Summary of Modules to Implement

| Module | Lines | Type | Complexity | Dependencies |
|--------|-------|------|-----------|---|
| Logger | 19 | Core | Low | Logging library |
| Database | 635 | Core | High | SQLite, JSON |
| Crawler | 105 | Core | Medium | OS, Path utilities |
| CLI | 176 | Interface | Medium | CLI framework |
| Server | 61 | Interface | Low | HTTP framework |
| MCP | 825 | Integration | High | MCP SDK, All above |
| **Total** | **1,821** | - | - | - |

### Estimated Go Implementation Lines:
- Database: 700-800 (slightly more verbose)
- Crawler: 150-200 (cleaner parallel potential)
- CLI: 200-250 (more boilerplate)
- Server: 80-100 (slightly more)
- MCP: 900-1000 (depends on MCP Go SDK)
- **Total: ~2,200-2,450 lines of Go code**

---

## Notes for Replatforming

1. **No External File-based Configs**: The system uses environment variables and command-line arguments. Keep this pattern.

2. **Database is Mutable**: The SQLite database is created and modified during operation. Ensure proper file handling in Go.

3. **Async Patterns**: Bun uses async/await heavily. Go will use goroutines and channels instead.

4. **Validation**: Zod schemas should be replaced with Go struct tags + validation library.

5. **Error Handling**: Go requires explicit error handling. Ensure comprehensive error chains.

6. **Null/Optional**: TypeScript's `?` must become Go's `*Type` or `sql.NullType`.

7. **JSON Marshaling**: JSON in database and API responses. Use Go's `json` tags.

8. **Session State**: Optional session management currently uses a Map. Can be simplified or kept with sync.Map.

---

