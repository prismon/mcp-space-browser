# Resource-Set Refactoring Implementation Plan

This document provides a detailed implementation plan for the resource-centric refactoring.

## Key Changes

1. **DAG Structure**: Resource-sets form a Directed Acyclic Graph (multiple parents allowed)
2. **Resource-Centric Tools**: New operations that work on abstract resources
3. **Plans Own Indexing**: `disk-index` moves to plan execution
4. **MCP Resource Templates**: Dual access via tools and resource URIs

## Implementation Phases

### Phase 1: Database Schema Changes

**Objective**: Create new DAG-based schema

#### 1.1 Create Migration Script

**File**: `pkg/database/migrations/002_resource_sets.sql`

```sql
-- Create resource_sets table
CREATE TABLE resource_sets (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
);

-- DAG edges (supports multiple parents)
CREATE TABLE resource_set_edges (
    parent_id INTEGER NOT NULL,
    child_id INTEGER NOT NULL,
    added_at INTEGER DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (parent_id, child_id),
    FOREIGN KEY (parent_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (child_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    CHECK (parent_id != child_id)
);

CREATE INDEX idx_edges_parent ON resource_set_edges(parent_id);
CREATE INDEX idx_edges_child ON resource_set_edges(child_id);

-- File entries within resource-sets
CREATE TABLE resource_set_entries (
    set_id INTEGER NOT NULL,
    entry_path TEXT NOT NULL,
    added_at INTEGER DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (set_id, entry_path),
    FOREIGN KEY (set_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
);

-- Unified sources table
CREATE TABLE sources (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('filesystem.index', 'filesystem.watch', 'query', 'resource-set')),
    target_set_name TEXT NOT NULL,
    update_mode TEXT DEFAULT 'append' CHECK(update_mode IN ('replace', 'append', 'merge')),
    config_json TEXT NOT NULL,
    status TEXT DEFAULT 'stopped',
    enabled INTEGER DEFAULT 1,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER,
    last_error TEXT
);
```

**Tasks**:
- [ ] Create migration file
- [ ] Rename `resource_set_children` → `resource_set_edges` (reflects DAG, not tree)
- [ ] Add migration runner
- [ ] Test on copy of production data

#### 1.2 DAG Operations

**File**: `pkg/database/resource_sets.go`

**New functions**:
```go
// DAG edge operations
func (db *DiskDB) AddResourceSetEdge(parentName, childName string) error
func (db *DiskDB) RemoveResourceSetEdge(parentName, childName string) error
func (db *DiskDB) GetResourceSetChildren(name string) ([]*ResourceSet, error)
func (db *DiskDB) GetResourceSetParents(name string) ([]*ResourceSet, error)

// Cycle detection
func (db *DiskDB) isAncestor(potentialAncestor, node int64) bool
func (db *DiskDB) GetAncestors(name string) ([]*ResourceSet, error)
func (db *DiskDB) GetDescendants(name string) ([]*ResourceSet, error)

// Aggregation
func (db *DiskDB) GetResourceSetMetricSum(name, metric string, includeChildren bool) (int64, error)
```

**Tasks**:
- [ ] Implement `AddResourceSetEdge` with cycle detection
- [ ] Implement `GetResourceSetParents` (upstream navigation)
- [ ] Implement `GetResourceSetChildren` (downstream navigation)
- [ ] Implement `isAncestor` using BFS/DFS
- [ ] Implement metric aggregation through DAG

---

### Phase 2: Model Updates

**Objective**: Update Go types for DAG structure

#### 2.1 Resource-Set Models

**File**: `internal/models/models.go`

```go
type ResourceSet struct {
    ID          int64   `db:"id" json:"id"`
    Name        string  `db:"name" json:"name"`
    Description *string `db:"description" json:"description,omitempty"`
    CreatedAt   int64   `db:"created_at" json:"created_at"`
    UpdatedAt   int64   `db:"updated_at" json:"updated_at"`
}

// DAG edge (replaces ResourceSetChild)
type ResourceSetEdge struct {
    ParentID int64 `db:"parent_id" json:"parent_id"`
    ChildID  int64 `db:"child_id" json:"child_id"`
    AddedAt  int64 `db:"added_at" json:"added_at"`
}

type ResourceSetEntry struct {
    SetID     int64  `db:"set_id" json:"set_id"`
    EntryPath string `db:"entry_path" json:"entry_path"`
    AddedAt   int64  `db:"added_at" json:"added_at"`
}
```

**Tasks**:
- [ ] Rename `SelectionSet` → `ResourceSet`
- [ ] Rename `ResourceSetChild` → `ResourceSetEdge`
- [ ] Update all references

---

### Phase 3: Resource-Centric Operations

**Objective**: Implement new resource query tools

#### 3.1 Resource Sum

**File**: `pkg/database/resource_metrics.go`

```go
func (db *DiskDB) ResourceSum(name, metric string, includeChildren bool) (*MetricResult, error) {
    // 1. Get resource-set
    // 2. If includeChildren, get all descendants
    // 3. Aggregate metric across all entries
    // 4. Return breakdown by child
}

type MetricResult struct {
    ResourceSet string        `json:"resource_set"`
    Metric      string        `json:"metric"`
    Value       int64         `json:"value"`
    Breakdown   []MetricPart  `json:"breakdown,omitempty"`
}

type MetricPart struct {
    Name  string `json:"name"`
    Value int64  `json:"value"`
}
```

**Tasks**:
- [ ] Implement `ResourceSum` with DAG traversal
- [ ] Support metrics: `size`, `count`, `duration`
- [ ] Return breakdown by child set

#### 3.2 Resource Time Range

**File**: `pkg/database/resource_queries.go`

```go
func (db *DiskDB) ResourceTimeRange(name, field string, min, max *time.Time, includeChildren bool) ([]*Entry, error) {
    // 1. Get entries from resource-set (and children if requested)
    // 2. Filter by time field (mtime, ctime, added_at)
    // 3. Return matching entries
}
```

**Tasks**:
- [ ] Implement time range filtering
- [ ] Support fields: `mtime`, `ctime`, `added_at`
- [ ] Handle nil min/max (open-ended ranges)

#### 3.3 Resource Metric Range

**File**: `pkg/database/resource_queries.go`

```go
func (db *DiskDB) ResourceMetricRange(name, metric string, min, max *int64, includeChildren bool) ([]*Entry, error) {
    // 1. Get entries from resource-set (and children if requested)
    // 2. Filter by metric range
    // 3. Return matching entries
}
```

**Tasks**:
- [ ] Implement metric range filtering
- [ ] Support metrics: `size`, custom metrics
- [ ] Handle nil min/max

---

### Phase 4: MCP Tool Updates

**Objective**: Replace disk-* tools with resource-* tools

#### 4.1 Remove Old Tools

**File**: `pkg/server/mcp_tools.go`

**Removed**:
- [ ] `disk-index` (moves to plan-execute)
- [ ] `disk-du` (replaced by resource-sum)
- [ ] `disk-time-range` (replaced by resource-time-range)
- [ ] `disk-tree` (replaced by resource-children)
- [ ] `navigate` (replaced by resource-children + resource-parent)

#### 4.2 Add Resource Navigation Tools

**File**: `pkg/server/mcp_resource_tools.go`

```go
// resource-children - Get child nodes in DAG
{
    Name: "resource-children",
    InputSchema: {
        "name": string,           // Required: resource-set name
        "depth": int,             // Optional: 1=immediate, null=all
        "include_entries": bool   // Optional: include file entries
    }
}

// resource-parent - Get parent nodes in DAG
{
    Name: "resource-parent",
    InputSchema: {
        "name": string,           // Required: resource-set name
        "depth": int              // Optional: 1=immediate, null=all ancestors
    }
}
```

**Tasks**:
- [ ] Implement `resource-children` tool
- [ ] Implement `resource-parent` tool
- [ ] Add depth parameter for controlled traversal

#### 4.3 Add Resource Query Tools

**File**: `pkg/server/mcp_resource_tools.go`

```go
// resource-sum - Hierarchical metric aggregation
{
    Name: "resource-sum",
    InputSchema: {
        "name": string,            // Required: resource-set name
        "metric": string,          // Required: "size", "count", etc.
        "include_children": bool,  // Optional: aggregate through DAG
        "depth": int               // Optional: limit depth
    }
}

// resource-time-range - Filter by time field
{
    Name: "resource-time-range",
    InputSchema: {
        "name": string,            // Required: resource-set name
        "field": string,           // Required: "mtime", "ctime", "added_at"
        "min": string,             // Optional: ISO date
        "max": string,             // Optional: ISO date
        "include_children": bool
    }
}

// resource-metric-range - Filter by metric range
{
    Name: "resource-metric-range",
    InputSchema: {
        "name": string,
        "metric": string,          // Required: "size", etc.
        "min": int,                // Optional
        "max": int,                // Optional
        "include_children": bool
    }
}
```

**Tasks**:
- [ ] Implement `resource-sum` tool
- [ ] Implement `resource-time-range` tool
- [ ] Implement `resource-metric-range` tool

---

### Phase 5: MCP Resource Templates

**Objective**: Add declarative resource access via MCP resources

#### 5.1 Register Resource Templates

**File**: `pkg/server/mcp_resources.go`

```go
func (s *MCPServer) registerResourceTemplates() {
    s.mcp.AddResourceTemplate(mcp.ResourceTemplate{
        URITemplate: "resource://resource-set/{name}",
        Name:        "Resource Set",
        Description: "Access a resource-set by name",
        MimeType:    "application/json",
    })

    s.mcp.AddResourceTemplate(mcp.ResourceTemplate{
        URITemplate: "resource://resource-set/{name}/children",
        Name:        "Resource Set Children",
        Description: "Child resource-sets in the DAG",
    })

    s.mcp.AddResourceTemplate(mcp.ResourceTemplate{
        URITemplate: "resource://resource-set/{name}/parents",
        Name:        "Resource Set Parents",
        Description: "Parent resource-sets in the DAG",
    })

    s.mcp.AddResourceTemplate(mcp.ResourceTemplate{
        URITemplate: "resource://resource-set/{name}/entries",
        Name:        "Resource Set Entries",
        Description: "File entries with pagination",
    })

    s.mcp.AddResourceTemplate(mcp.ResourceTemplate{
        URITemplate: "resource://resource-set/{name}/metrics/{metric}",
        Name:        "Resource Metric",
        Description: "Aggregated metric value",
    })
}
```

#### 5.2 Implement Resource Handlers

**File**: `pkg/server/mcp_resources.go`

```go
func (s *MCPServer) handleResourceRead(uri string) (*mcp.ResourceContent, error) {
    // Parse URI: resource://resource-set/{name}/...
    // Route to appropriate handler
    // Return JSON content
}
```

**Tasks**:
- [ ] Parse resource URIs
- [ ] Implement handlers for each template
- [ ] Support pagination for entries
- [ ] Return appropriate MIME types

---

### Phase 6: Plan System Updates

**Objective**: Move indexing to plan execution

#### 6.1 Plan Creates Sources Inline

**File**: `pkg/plans/executor.go`

When a plan is executed:
1. Create any inline sources that don't exist
2. Execute sources in dependency order
3. Record execution history

```go
type PlanSourceDef struct {
    Name      string      `json:"name"`
    Type      SourceType  `json:"type"`
    TargetSet string      `json:"target_set"`
    Config    interface{} `json:"config"`
}
```

**Tasks**:
- [ ] Support inline source definitions in plans
- [ ] Create sources on plan execution
- [ ] Track source execution within plan execution

#### 6.2 Remove Standalone disk-index

**File**: `pkg/server/mcp_tools.go`

Remove the `disk-index` tool. Users must now:
1. Create a resource-set
2. Create a plan with a filesystem.index source
3. Execute the plan

**Tasks**:
- [ ] Remove `disk-index` tool definition
- [ ] Update documentation
- [ ] Add migration notes for existing users

---

### Phase 7: Testing

**Objective**: Comprehensive test coverage

#### 7.1 DAG Tests

**File**: `pkg/database/resource_sets_test.go`

**Tests**:
- [ ] Test multiple parents (DAG structure)
- [ ] Test cycle detection (A→B→C→A should fail)
- [ ] Test ancestor/descendant queries
- [ ] Test bidirectional navigation

#### 7.2 Metric Aggregation Tests

**File**: `pkg/database/resource_metrics_test.go`

**Tests**:
- [ ] Test `resource-sum` with single set
- [ ] Test `resource-sum` with children
- [ ] Test `resource-sum` with diamond pattern (node with 2 parents)
- [ ] Test breakdown accuracy

#### 7.3 MCP Resource Tests

**File**: `pkg/server/mcp_resources_test.go`

**Tests**:
- [ ] Test resource URI parsing
- [ ] Test each resource template
- [ ] Test pagination
- [ ] Test error handling

---

### Phase 8: Documentation

**Objective**: Update all documentation

#### 8.1 Update CLAUDE.md

**Tasks**:
- [ ] Update tool list with resource-* tools
- [ ] Remove disk-* tools
- [ ] Update database schema
- [ ] Add MCP resource template examples

#### 8.2 Update README.go.md

**Tasks**:
- [ ] Update MCP tools list
- [ ] Add resource template documentation
- [ ] Update workflow examples

---

## File Change Summary

### New Files
- `pkg/database/resource_sets.go` - ResourceSet CRUD and DAG operations
- `pkg/database/resource_metrics.go` - Metric aggregation
- `pkg/database/resource_queries.go` - Time/metric range queries
- `pkg/server/mcp_resource_tools.go` - Resource-centric MCP tools
- `pkg/server/mcp_resources.go` - MCP resource template handlers

### Renamed Files
- `pkg/database/selection_sets.go` → removed (merged into resource_sets.go)
- `resource_set_children` table → `resource_set_edges` table

### Modified Files
- `pkg/database/database.go` - Schema updates
- `internal/models/models.go` - Type renames
- `pkg/server/mcp_tools.go` - Remove disk-* tools
- `pkg/plans/executor.go` - Support inline sources
- `CLAUDE.md` - Documentation updates
- `README.go.md` - Documentation updates

## Tool Migration Matrix

| Old Tool | New Tool(s) | Notes |
|----------|-------------|-------|
| `disk-index` | `plan-execute` | Index via plan with filesystem.index source |
| `disk-du` | `resource-sum` | Use metric: "size" |
| `disk-time-range` | `resource-time-range` | Same functionality |
| `disk-tree` | `resource-children` | DAG navigation |
| `navigate` | `resource-children` + `resource-parent` | Bidirectional |
| `selection-set-*` | `resource-set-*` | Renamed |
| `query-*` | `source-*` | Unified into sources |

## Breaking Changes

1. **`disk-index` removed**: Must use plan-execute
2. **DAG structure**: Multiple parents now allowed
3. **MCP tool names**: All selection-set-* → resource-set-*
4. **Database schema**: New edges table, renamed tables

## Success Criteria

1. All DAG operations work correctly
2. Cycle detection prevents invalid graphs
3. Metric aggregation traverses DAG correctly
4. MCP resource templates respond correctly
5. Plans can create and execute sources
6. All tests pass
7. Documentation is complete
