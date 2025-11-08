# MCP Space Browser - Module Architecture

## Directory Structure

```
mcp-space-browser/
├── src/
│   ├── cli.ts              # CLI entry point (176 lines)
│   ├── crawler.ts          # Filesystem DFS traversal (105 lines)
│   ├── db.ts               # SQLite abstraction layer (635 lines)
│   ├── logger.ts           # Logging infrastructure (19 lines)
│   ├── server.ts           # HTTP REST API server (61 lines)
│   ├── mcp.ts              # MCP server implementation (825 lines)
│   └── mcp-simple.ts       # Minimal MCP reference (51 lines)
├── test/
│   ├── agent.test.ts       # Crawler and aggregation tests
│   ├── mcp.test.ts         # MCP tool tests
│   ├── mcp-server.test.ts  # HTTP server tests
│   ├── test-pagination.ts
│   ├── test-session.ts
│   ├── test-queries.ts
│   ├── test-selection-sets.ts
│   ├── test-mcp-selection.ts
│   └── helpers/
├── docs/
│   └── disk_agent_design.md
├── package.json            # Dependencies
├── tsconfig.json           # TypeScript config
├── bun.lock               # Lock file
└── disk.db                # SQLite database (generated)
```

## Module Dependency Graph

```
┌─────────────────────────────────────────────────────────┐
│                   ENTRY POINTS                          │
├─────────────────────────────────────────────────────────┤
│  CLI (cli.ts)  │  HTTP Server (server.ts)  │  MCP (mcp.ts)  │
└────────┬─────────────────┬─────────────────┬──────────────┘
         │                 │                 │
         └─────────────────┼─────────────────┘
                           │
                    ┌──────▼──────────┐
                    │  LOGGER         │
                    │  (logger.ts)    │
                    └─────────────────┘
                           ▲
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
    ┌────▼────────┐  ┌─────▼──────┐  ┌────▼────────┐
    │  CRAWLER    │  │  DATABASE  │  │ VALIDATION  │
    │(crawler.ts)│  │  (db.ts)   │  │  (Zod)      │
    └────┬────────┘  └─────┬──────┘  └─────────────┘
         │                 │
         │          ┌──────▼──────────┐
         │          │  SQLITE         │
         │          │  (disk.db)      │
         │          └─────────────────┘
         │
    ┌────▼──────────────────────────────┐
    │  OS / FILESYSTEM APIs             │
    │  (fs, path, stat)                 │
    └───────────────────────────────────┘
```

## Component Interactions

### Crawler Module Flow
```
index(root, db)
  ├─→ Create stack with [root]
  ├─→ While stack not empty:
  │   ├─→ Pop path
  │   ├─→ fs.stat(path) → get metadata
  │   ├─→ db.insertOrUpdate(entry)
  │   └─→ If directory: push children
  ├─→ db.deleteStale(root, runId) → remove old entries
  └─→ db.computeAggregates(root) → sum directory sizes
```

### Database Module Structure
```
DiskDB (class)
  ├─ Constructor(dbPath: string = 'disk.db')
  │   ├─ Open/create SQLite connection
  │   ├─ Initialize schema
  │   └─ Prepare statements
  │
  ├─ Entry Management
  │   ├─ insertOrUpdate(entry: Entry)
  │   ├─ get(path: string): Entry | undefined
  │   ├─ children(parent: string): Entry[]
  │   ├─ deleteStale(root: string, runId: number)
  │   └─ computeAggregates(root: string)
  │
  ├─ SelectionSet Management
  │   ├─ createSelectionSet(set: SelectionSet): number
  │   ├─ listSelectionSets(): SelectionSet[]
  │   ├─ getSelectionSet(name: string): SelectionSet | undefined
  │   ├─ deleteSelectionSet(name: string)
  │   ├─ addToSelectionSet(setName: string, paths: string[])
  │   ├─ removeFromSelectionSet(setName: string, paths: string[])
  │   ├─ getSelectionSetEntries(setName: string): Entry[]
  │   └─ getSelectionSetStats(setName: string): {count, totalSize}
  │
  ├─ Query Management
  │   ├─ createQuery(query: Query): number
  │   ├─ listQueries(): Query[]
  │   ├─ getQuery(name: string): Query | undefined
  │   ├─ updateQuery(name: string, updates: Partial<Query>)
  │   ├─ deleteQuery(name: string)
  │   ├─ executeQuery(name: string): {selectionSet, filesMatched}
  │   └─ getQueryExecutions(queryName: string, limit: number): QueryExecution[]
  │
  ├─ File Filtering
  │   └─ executeFileFilter(filter: FileFilter): string[] (paths)
  │
  └─ Transaction Support
      ├─ beginTransaction()
      ├─ commitTransaction()
      └─ rollbackTransaction()
```

### MCP Tool Hierarchy
```
MCP Server (FastMCP)
  │
  ├─ Core Tools (Filesystem Operations)
  │  ├─ disk-index
  │  ├─ disk-du
  │  ├─ disk-tree
  │  └─ disk-time-range
  │
  ├─ Selection Set Management
  │  ├─ selection-set-create
  │  ├─ selection-set-list
  │  ├─ selection-set-get
  │  ├─ selection-set-modify
  │  └─ selection-set-delete
  │
  ├─ Query Management
  │  ├─ query-create
  │  ├─ query-execute
  │  ├─ query-list
  │  ├─ query-get
  │  ├─ query-update
  │  └─ query-delete
  │
  └─ Session Management
     ├─ session-info
     └─ session-set-preferences
```

## Data Model & Schema

### Entry Table (Core)
```
entries
├─ id: INTEGER PRIMARY KEY
├─ path: TEXT UNIQUE NOT NULL
├─ parent: TEXT                    → References entries.path
├─ size: INTEGER                   → Bytes (aggregated for directories)
├─ kind: TEXT ('file' | 'directory')
├─ ctime: INTEGER                  → Creation time (unix seconds)
├─ mtime: INTEGER                  → Modification time (unix seconds)
├─ last_scanned: INTEGER          → Last index run timestamp
└─ dirty: INTEGER DEFAULT 0        → Recomputation flag

Indexes:
├─ PRIMARY KEY (id)
├─ UNIQUE (path)
├─ idx_parent (parent)
└─ idx_mtime (mtime)
```

### SelectionSet Tables
```
selection_sets
├─ id: INTEGER PRIMARY KEY
├─ name: TEXT UNIQUE NOT NULL
├─ description: TEXT
├─ criteria_type: TEXT ('user_selected' | 'tool_query')
├─ criteria_json: TEXT            → Serialized {tool, params}
├─ created_at: INTEGER
└─ updated_at: INTEGER

selection_set_entries (junction table)
├─ set_id: INTEGER PRIMARY KEY    → FK to selection_sets.id
├─ entry_path: TEXT PRIMARY KEY   → FK to entries.path
├─ added_at: INTEGER

Index:
└─ idx_set_entries (set_id)
```

### Query Tables
```
queries
├─ id: INTEGER PRIMARY KEY
├─ name: TEXT UNIQUE NOT NULL
├─ description: TEXT
├─ query_type: TEXT ('file_filter' | 'custom_script')
├─ query_json: TEXT               → Serialized FileFilter
├─ target_selection_set: TEXT
├─ update_mode: TEXT ('replace' | 'append' | 'merge')
├─ created_at: INTEGER
├─ updated_at: INTEGER
├─ last_executed: INTEGER
└─ execution_count: INTEGER

query_executions
├─ id: INTEGER PRIMARY KEY
├─ query_id: INTEGER FK
├─ executed_at: INTEGER
├─ duration_ms: INTEGER
├─ files_matched: INTEGER
├─ status: TEXT ('success' | 'error')
└─ error_message: TEXT

Index:
└─ idx_query_executions (query_id, executed_at DESC)
```

## Type Definitions

### Core Types
```typescript
interface Entry {
  id?: number
  path: string                    // Absolute path
  parent: string | null           // Parent directory path
  size: number                    // Bytes (0 for directories until aggregated)
  kind: 'file' | 'directory'
  ctime: number                   // Unix seconds
  mtime: number                   // Unix seconds
  last_scanned: number            // Unix milliseconds (scan run ID)
}

interface FileFilter {
  path?: string                   // Root directory
  pattern?: string                // Regex pattern for paths
  extensions?: string[]           // File extensions (e.g., ['ts', 'js'])
  minSize?: number                // Bytes
  maxSize?: number                // Bytes
  minDate?: string                // YYYY-MM-DD
  maxDate?: string                // YYYY-MM-DD
  nameContains?: string           // Substring in filename
  pathContains?: string           // Substring in full path
  sortBy?: 'size' | 'name' | 'mtime'
  descendingSort?: boolean        // Default: true
  limit?: number                  // Max results
}

interface SelectionSet {
  id?: number
  name: string
  description?: string
  criteria_type: 'user_selected' | 'tool_query'
  criteria_json?: string          // Serialized {tool, params}
  created_at?: number
  updated_at?: number
}

interface Query {
  id?: number
  name: string
  description?: string
  query_type: 'file_filter' | 'custom_script'
  query_json: string              // Serialized FileFilter
  target_selection_set?: string
  update_mode?: 'replace' | 'append' | 'merge'
  created_at?: number
  updated_at?: number
  last_executed?: number
  execution_count?: number
}
```

## API Endpoints

### HTTP Server (port 3000)
```
GET /api/index?path=/foo
  → Trigger async indexing
  → Response: "OK"
  → Side effects: Updates database

GET /api/tree?path=/foo
  → Get hierarchical JSON tree
  → Response: {path, size, children: [{...}]}
  → Tree structure of entries
```

## CLI Commands

```bash
disk-index /path              # Index a directory tree
disk-du /path                 # Get disk usage (bytes)
disk-tree /path [options]     # Display tree with options:
  --sort-by=<size|mtime|name>
  --ascending
  --min-date=YYYY-MM-DD
  --max-date=YYYY-MM-DD
```

## MCP Tool Signatures

### Indexing Tools
```typescript
disk-index({path: string})
  → Promise<string>  // "OK"

disk-du({path: string})
  → Promise<string>  // Size in bytes

disk-tree({
  path: string
  maxDepth?: number
  minSize?: number
  limit?: number
  sortBy?: 'size' | 'name' | 'mtime'
  descendingSort?: boolean
  groupBy?: 'extension' | 'none'
  minDate?: string
  maxDate?: string
  offset?: number
  pageSize?: number
})
  → Promise<string>  // JSON with pagination info

disk-time-range({
  path: string
  count?: number     // Number of oldest/newest
  minSize?: number
})
  → Promise<string>  // JSON with oldest/newest files
```

### Selection Set Tools
```typescript
selection-set-create({
  name: string
  description?: string
  fromTool?: {tool: string, params: {}}
})
  → Promise<string>

selection-set-list()
  → Promise<string>  // Array of sets with stats

selection-set-get({name: string})
  → Promise<string>  // Set with entries and stats

selection-set-modify({
  name: string
  paths: string[]
  operation: 'add' | 'remove'
})
  → Promise<string>

selection-set-delete({name: string})
  → Promise<string>
```

### Query Tools
```typescript
query-create({
  name: string
  description?: string
  filter: FileFilter
  targetSelectionSet?: string
  updateMode?: 'replace' | 'append' | 'merge'
})
  → Promise<string>

query-execute({name: string})
  → Promise<string>  // Execution result with counts

query-list()
  → Promise<string>  // Array of queries

query-get({name: string})
  → Promise<string>  // Query with execution history

query-update({name: string, ...updates})
  → Promise<string>

query-delete({name: string})
  → Promise<string>
```

### Session Tools
```typescript
session-info()
  → Promise<string>  // Current session state

session-set-preferences({
  maxResults?: number
  sortBy?: 'size' | 'name' | 'mtime'
})
  → Promise<string>
```

## Data Flow Example: Indexing a Directory

```
User Input:
  disk-index /home/user/projects

Execution Flow:
1. CLI parses: cmd='disk-index', target='/home/user/projects'
2. CLI calls: DiskDB().index(target)
3. Crawler.index(target, db):
   a) Stack = ['/home/user/projects']
   b) Pop '/home/user/projects'
   c) fs.stat() → size=4096, isDir=true
   d) db.insertOrUpdate({path, size, kind: 'directory', ...})
   e) fs.readdir() → ['src', 'test', 'package.json']
   f) Push to stack: ['/home/user/projects/src', ...]
   g) Repeat (b-f) for each entry
4. db.deleteStale(target, runId):
   a) Delete entries in [target, target/%] with older last_scanned
5. db.computeAggregates(target):
   a) Get all directories in subtree (ORDER BY length DESC)
   b) For each directory (deepest first):
      - SUM(size) of children
      - UPDATE entries SET size = SUM WHERE path = dir
6. Return "OK" to user

Database After:
  entries table has:
  ├─ /home/user/projects (size=12345, aggregated)
  ├─ /home/user/projects/src (size=8000)
  ├─ /home/user/projects/src/main.ts (size=2000)
  ├─ /home/user/projects/test (size=3000)
  └─ /home/user/projects/package.json (size=345)
```

## Performance Characteristics

### Time Complexity
- **Indexing**: O(n) where n = total entries traversed
- **Aggregation**: O(n) single pass with sorted directories
- **Query execution**: O(n) with early termination at limit

### Space Complexity
- **Stack**: O(d) where d = max directory depth
- **Database**: O(n) entries stored
- **Memory**: Minimal, streaming results for large trees

### Optimizations
- Batch transactions: 10-100x faster inserts
- Prepared statements: Reuse compiled SQL
- Indexes on parent/mtime: Fast lookups
- Stack-based DFS: Avoids recursion limits

## Test Structure

### Test Patterns
```
withTempDir(async (dir) => {
  // Create temporary test directory
  // Create test files
  const db = new DiskDB(':memory:')  // In-memory DB
  await index(dir, db)
  // Assertions
  // Temp dir auto-cleanup
})
```

### Test Coverage Areas
1. **Crawler**: Basic traversal, aggregation updates, stale cleanup
2. **Database**: CRUD operations, filtering, pagination
3. **MCP Tools**: Parameter validation, tool execution, result formatting
4. **Integration**: Full workflows with temporary directories

