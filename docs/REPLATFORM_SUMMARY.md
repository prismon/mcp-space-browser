# MCP Space Browser - Go Replatforming Summary

## Quick Reference

This document provides a high-level overview of the replatforming effort from Bun/TypeScript to Go.

**For detailed analysis, see:**
- `REPLATFORM_ANALYSIS.md` - Comprehensive module breakdown and requirements
- `MODULE_ARCHITECTURE.md` - Data structures, schemas, and component interactions

---

## Executive Summary

**mcp-space-browser** is a 3,471-line Bun/TypeScript application that needs to be replatformed to Go.

### What This System Does
1. **Indexes filesystems** - Recursively crawls directories with DFS traversal
2. **Stores metadata** - Persists file/directory info in SQLite (5 tables, 4 indexes)
3. **Provides tools** - Exposes 20+ tools via Model Context Protocol (MCP)
4. **Manages queries** - Saved file filters with execution history
5. **Groups files** - Named selection sets for batch operations

### Why Replatform
- **Performance**: Go's native compilation, goroutines for parallelization
- **Deployment**: Single static binary vs. Bun runtime dependency
- **Maintainability**: Explicit error handling, no async/await complexity

---

## Codebase Overview

### Current Structure (Bun/TypeScript)
```
Total: 3,471 lines across 8 modules

Core Modules:
├─ db.ts (635 lines)      - SQLite abstraction, all database logic
├─ mcp.ts (825 lines)     - MCP server with 20+ tools
├─ cli.ts (176 lines)     - Command-line interface
├─ crawler.ts (105 lines) - Filesystem DFS traversal
├─ server.ts (61 lines)   - HTTP REST API
├─ logger.ts (19 lines)   - Structured logging
└─ mcp-simple.ts (51 lines) - Minimal MCP reference

Tests: 11 files, ~750+ lines
```

### Go Replatforming Estimate
```
Expected: 2,200-2,450 lines of Go code

Why more?
- Go is more verbose (explicit error handling, nil checks)
- Less built-in framework integration
- More boilerplate for similar functionality

Why less?
- No async/await complexity
- Simpler transaction handling
- Potential for parallel optimization
```

---

## Module Mapping: TypeScript → Go

| TypeScript | Go Equivalent | Notes |
|-----------|--------------|-------|
| `logger.ts` | `pkg/logger` | Use logrus or zerolog |
| `db.ts` | `pkg/database` | Use glebarez/sqlite or mattn/go-sqlite3 |
| `crawler.ts` | `pkg/crawler` | Use os.Stat, filepath.Walk |
| `cli.ts` | `cmd/cli` | Use spf13/cobra or urfave/cli |
| `server.ts` | `cmd/server` | Use gin or gorilla/mux |
| `mcp.ts` | `cmd/mcp` | Await Go MCP SDK or build JSON-RPC 2.0 |
| Bun:sqlite | go-sqlite3 | Different API, similar functionality |
| Zod validation | validator tags | Go struct tags + validation library |

---

## Critical Implementation Points

### 1. Database Schema (Must Preserve)
```sql
CREATE TABLE entries (                    -- Entries indexed
  id INTEGER PRIMARY KEY,
  path TEXT UNIQUE NOT NULL,              -- Absolute filesystem path
  parent TEXT,                            -- Parent directory path
  size INTEGER,                           -- Bytes (aggregated for dirs)
  kind TEXT CHECK(...),                   -- 'file' or 'directory'
  ctime INTEGER, mtime INTEGER,           -- Unix seconds
  last_scanned INTEGER,                   -- Scan run ID (for incremental updates)
  dirty INTEGER DEFAULT 0                 -- Recomputation flag
)
CREATE INDEX idx_parent ON entries(parent)
CREATE INDEX idx_mtime ON entries(mtime)

-- Plus 4 more tables for:
-- - selection_sets (named file groupings)
-- - selection_set_entries (many-to-many)
-- - queries (saved file filters)
-- - query_executions (execution history)
```

### 2. Core Algorithms

**Filesystem Indexing** (crawler):
```
Stack-based DFS traversal:
1. Push root onto stack
2. While stack not empty:
   - Pop path
   - Call stat() to get metadata
   - Insert/update entry in DB
   - If directory, push children
3. Delete stale entries (not seen in this run)
4. Compute aggregate sizes (sum children, deepest first)
```

**File Filtering** (db.executeFileFilter):
```
Recursive tree traversal with multiple criteria:
- Path pattern (regex)
- File extensions
- Size range (minSize, maxSize)
- Date range (minDate, maxDate)
- Name/path contains
- Sorting (size, name, mtime)
- Pagination (offset, limit)
```

### 3. MCP Tool Hierarchy (20+ tools)

**Core**: disk-index, disk-du, disk-tree, disk-time-range
**SelectionSets**: create, list, get, modify, delete
**Queries**: create, execute, list, get, update, delete
**Session**: info, set-preferences

### 4. Data Structures

```go
// Go equivalent for TypeScript interfaces:

type Entry struct {
    ID         int64      // null in TS (optional)
    Path       string     // required
    Parent     *string    // string | null in TS
    Size       int64
    Kind       string     // 'file' | 'directory'
    Ctime      int64      // unix seconds
    Mtime      int64      // unix seconds
    LastScanned int64     // scan run timestamp
}

type FileFilter struct {
    Path           *string    // optional filtering root
    Pattern        *string    // regex
    Extensions     []string   // e.g., ["ts", "js"]
    MinSize        *int64
    MaxSize        *int64
    MinDate        *string    // YYYY-MM-DD format
    MaxDate        *string
    NameContains   *string
    PathContains   *string
    SortBy         *string    // "size", "name", "mtime"
    DescendingSort bool       // default true
    Limit          *int
}

type SelectionSet struct {
    ID          int64
    Name        string     // unique
    Description *string
    CriteriaType string    // "user_selected" or "tool_query"
    CriteriaJSON *string
    CreatedAt   int64
    UpdatedAt   int64
}

type Query struct {
    ID                int64
    Name              string     // unique
    Description       *string
    QueryType         string     // "file_filter" or "custom_script"
    QueryJSON         string     // serialized FileFilter
    TargetSelectionSet *string
    UpdateMode        *string    // "replace", "append", "merge"
    CreatedAt         int64
    UpdatedAt         int64
    LastExecuted      *int64
    ExecutionCount    int
}
```

---

## Key Design Decisions to Preserve

### 1. Single-Table Design Philosophy
- All filesystem entries in ONE `entries` table with parent references
- Not normalized, but highly efficient for this use case
- Enables simple queries and fast traversal

### 2. Last Scanned Tracking
- Every scan updates `last_scanned` timestamp
- Enables incremental updates without full reindexing
- Allows multiple root directories in same database
- Stale entries auto-detected and removed

### 3. Post-Processing Aggregation
- Directory sizes computed AFTER crawling (not during)
- Single pass through directories (deepest first)
- Atomic transaction for consistency

### 4. Stack-Based DFS (Not Recursive)
- Critical for deep directory trees (avoids stack overflow)
- O(d) space where d = tree depth, not call stack depth

### 5. Transaction-Batching for Performance
- Bulk inserts wrapped in transactions
- 10-100x faster than individual inserts
- Rollback on error

---

## Development Roadmap

### Phase 1: Foundation (Est. 5-7 days)
- [ ] Set up Go project structure and module layout
- [ ] Implement logger with parity to Pino
- [ ] Create database layer with SQLite wrapper
- [ ] Define all Go structs matching TypeScript types
- [ ] Implement Entry CRUD operations

**Deliverable**: In-memory SQLite database working with basic CRUD

### Phase 2: Core Functionality (Est. 5-7 days)
- [ ] Implement filesystem crawler (DFS traversal)
- [ ] Aggregate size computation
- [ ] Stale entry deletion
- [ ] SelectionSet management
- [ ] Query creation and execution

**Deliverable**: Full database layer tested with temp directories

### Phase 3: Interfaces (Est. 3-4 days)
- [ ] CLI with cobra/urfave
- [ ] HTTP server with gin/gorilla
- [ ] File filtering with all options

**Deliverable**: All three CLI commands and HTTP endpoints working

### Phase 4: MCP Integration (Est. 4-5 days)
- [ ] Research/evaluate MCP Go SDK options
- [ ] Implement MCP server with JSON-RPC 2.0
- [ ] Tool definitions with parameter schemas
- [ ] Session management (if keeping)

**Deliverable**: All 20+ tools accessible via MCP

### Phase 5: Testing & Polish (Est. 3-4 days)
- [ ] Unit tests for each module
- [ ] Integration tests with temp directories
- [ ] Error handling parity
- [ ] Performance optimization
- [ ] Documentation and examples

**Deliverable**: Production-ready Go binary

**Total Estimate: 3-4 weeks** for experienced Go developer

---

## Go Dependencies to Evaluate

```
Core:
├─ github.com/glebarez/sqlite      -- SQLite driver (or mattn/go-sqlite3)
├─ database/sql                    -- Go stdlib DB abstraction

Logging:
├─ github.com/sirupsen/logrus      -- Structured logging with colors
└─ github.com/rs/zerolog            -- Alternative: faster structured logging

CLI:
├─ github.com/spf13/cobra           -- Complete CLI framework
└─ github.com/urfave/cli             -- Lighter alternative

HTTP Server:
├─ github.com/gin-gonic/gin         -- Full-featured HTTP framework
├─ github.com/gorilla/mux           -- Lighter router
└─ net/http                          -- Stdlib HTTP

Validation:
├─ github.com/go-playground/validator -- Struct tag validation
└─ Custom validation                 -- Lightweight option

MCP:
├─ github.com/modelcontextprotocol/sdk-go -- Official (if available)
└─ Custom JSON-RPC 2.0               -- Fallback implementation

Testing:
├─ testing                           -- Stdlib testing
├─ github.com/stretchr/testify      -- Assertions and mocking
└─ testify/assert                    -- Better assertion helpers
```

---

## Testing Strategy

### Unit Test Coverage
```
database_test.go:
  ├─ TestInsertOrUpdate
  ├─ TestGet
  ├─ TestChildren
  ├─ TestComputeAggregates
  ├─ TestDeleteStale
  ├─ TestSelectionSetCRUD
  └─ TestQueryExecution

crawler_test.go:
  ├─ TestIndexBasic
  ├─ TestIndexAggregates
  ├─ TestIncrementalUpdate
  └─ TestErrorHandling

cli_test.go / server_test.go / mcp_test.go:
  └─ Full tool/command test cases
```

### Test Patterns
```go
func TestIndexing(t *testing.T) {
    // Use t.TempDir() for filesystem operations
    tempDir := t.TempDir()
    
    // Use in-memory SQLite for speed
    db := setupTestDB(":memory:")
    defer db.Close()
    
    // Run indexer
    err := crawler.Index(tempDir, db)
    assert.NoError(t, err)
    
    // Verify results
    entries, err := db.All()
    assert.Equal(t, 5, len(entries))
}
```

---

## Performance Considerations for Go

### Potential Improvements (vs. Bun version)
1. **Parallel filesystem traversal** - Use goroutines for concurrent stat() calls
2. **Connection pooling** - Better database connection management
3. **Memory efficiency** - Streaming large result sets instead of building in-memory
4. **Binary size** - Single static binary vs. runtime dependency

### Optimization Points
- Consider adding result caching for frequently accessed paths
- Implement streaming for large tree traversals (pagination already there)
- Use context.Context for cancellation in long-running operations
- Consider goroutine workers for filesystem scans

---

## Compatibility Notes

### Exact Parity Required
- Same CLI command names and flags
- Same HTTP endpoint paths and response formats
- Same MCP tool names and parameter types
- Same database schema and behavior

### Can Improve
- Add context.Context support
- Better error messages with wrapping
- More efficient parallel operations
- Smaller binary size

### Known Differences (Acceptable)
- Go syntax instead of TypeScript
- `nil` instead of `undefined`
- `error` instead of exceptions
- Goroutines instead of async/await

---

## Files to Review for Reference

1. **REPLATFORM_ANALYSIS.md** (18 KB)
   - Complete module breakdown
   - Data structures and schemas
   - All 20+ MCP tools documented
   - Dependencies and equivalents

2. **MODULE_ARCHITECTURE.md** (15 KB)
   - Detailed component interactions
   - Data flow examples
   - Type definitions
   - Performance characteristics

3. **Source Code**
   - `src/db.ts` - Database layer (study the executeFileFilter logic)
   - `src/crawler.ts` - Simple example of DFS
   - `src/mcp.ts` - Tool definitions and schemas

---

## Success Criteria

The replatformed Go version is successful when:

1. ✅ All 20+ MCP tools function identically
2. ✅ Database schema and operations are identical
3. ✅ CLI commands produce same output
4. ✅ HTTP server endpoints work identically
5. ✅ All tests pass (with Go equivalents)
6. ✅ Performance is comparable or better
7. ✅ Error handling is comprehensive
8. ✅ Code is maintainable and idiomatic Go

---

## Quick Start for Go Developer

1. Read `REPLATFORM_ANALYSIS.md` sections:
   - Module Breakdown (modules 2, 3, 6)
   - Database Schema
   - Data Structures
   - Dependencies

2. Study the TypeScript code in this order:
   - `src/logger.ts` (trivial, understand log levels)
   - `src/db.ts` lines 1-100 (Entry management)
   - `src/crawler.ts` (understand DFS algorithm)
   - `src/mcp.ts` lines 1-100 (understand tool pattern)

3. Start implementation:
   - Create project structure: `cmd/`, `pkg/`, `test/`
   - Implement logger first
   - Then database layer (most complex)
   - Then crawler
   - Then CLI/HTTP/MCP in parallel

---

## Questions to Clarify

Before starting, confirm:

1. **MCP SDK**: Is Go SDK available from modelcontextprotocol? If not, implement custom JSON-RPC 2.0?
2. **Session State**: Keep optional session management or simplify?
3. **Logging Format**: Must parity match Pino pretty output exactly?
4. **Performance**: Is parallelization expected/desired in crawler?
5. **Testing**: Full test suite parity required?

---

