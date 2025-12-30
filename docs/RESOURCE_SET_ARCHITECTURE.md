# Resource-Set Architecture

## Overview

This document describes the architectural refactoring to a **resource-centric model** where:

1. **Resource-Sets form a DAG** (Directed Acyclic Graph) with bidirectional navigation
2. **Operations are resource-centric** with neutral naming (not file-specific)
3. **Indexing moves to Plans** rather than being a standalone operation
4. **MCP Resources + Tools** provide dual access patterns for all resources

## Design Principles

1. **DAG over Trees**: Resource-sets can have multiple parents and children (no cycles)
2. **Resource-Neutral Operations**: Tools work on abstract resources, not just files
3. **Plans Own Indexing**: All data ingestion happens through Plans with Sources
4. **Dual Access**: Resources accessible via MCP tools AND resource templates
5. **Bidirectional Navigation**: Navigate both to children and parents
6. **Metric Aggregation**: Hierarchical rollup of any metric through the DAG

---

## Core Concepts

### 1. Resource-Set as DAG Node

A **Resource-Set** is a node in a Directed Acyclic Graph that can:
- Contain resources (file/directory entries)
- Have multiple parent resource-sets
- Have multiple child resource-sets
- Be traversed in both directions

```
                    ┌─────────────┐
                    │  all-media  │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │   photos    │ │   videos    │ │    audio    │
    └──────┬──────┘ └──────┬──────┘ └─────────────┘
           │               │
           ▼               ▼
    ┌─────────────┐ ┌─────────────┐
    │  vacation   │ │  tutorials  │  ← Can have multiple parents
    └─────────────┘ └─────────────┘
           │
           ▼
    ┌─────────────┐
    │  shared     │  ← Multiple parents: vacation + tutorials
    └─────────────┘
```

**DAG Properties:**
- Nodes can have 0..N parents (roots have 0)
- Nodes can have 0..N children (leaves have 0)
- **No cycles allowed** (enforced at add-child time)
- Bidirectional traversal: `resource-children` and `resource-parent`

### 2. Resource-Centric Operations

Operations are designed to work on abstract resources with metrics, not tied to files:

| Operation | Description | Replaces |
|-----------|-------------|----------|
| `resource-sum` | Hierarchical aggregation of a metric | `disk-du` |
| `resource-time-range` | Filter resources by time field in range | `disk-time-range` |
| `resource-metric-range` | Filter resources by metric in range | (new) |
| `resource-children` | Get child nodes in DAG | `disk-tree` (partial) |
| `resource-parent` | Get parent nodes (".."-like) | (new) |

### 3. Plans Own Indexing

**Key Change**: `disk-index` is no longer a standalone tool. Indexing is an operation that:
- Belongs to a **Plan**
- Uses a **Source** (type: `filesystem.index`)
- Populates a **Resource-Set**

```
┌─────────────────────────────────────────────────────────────┐
│                          PLAN                                │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Sources (what to ingest)                             │   │
│  │  - filesystem.index: /home/user/Photos               │   │
│  │  - filesystem.watch: /home/user/Downloads            │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                  │
│                           ▼                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Resource-Sets (where to store)                       │   │
│  │  - photos, videos, downloads                          │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
│  Execution: plan-execute "my-plan"                           │
└─────────────────────────────────────────────────────────────┘
```

### 4. MCP Resource Templates

Resources are accessible via **two patterns**:

**1. Tools (imperative):**
```json
{"tool": "resource-children", "params": {"name": "photos"}}
```

**2. Resource Templates (declarative):**
```
synthesis://resource-set/photos
synthesis://resource-set/photos/children
synthesis://resource-set/photos/parents
synthesis://resource-set/photos/entries
synthesis://resource-set/photos/metrics/size
```

---

## Resource Operations

### resource-sum

Hierarchical aggregation of a metric across the DAG.

```json
{
  "tool": "resource-sum",
  "params": {
    "name": "all-media",
    "metric": "size",           // "size", "count", "duration", custom
    "include_children": true,   // Aggregate through DAG
    "depth": null               // null = unlimited
  }
}
```

**Response:**
```json
{
  "resource_set": "all-media",
  "metric": "size",
  "value": 1073741824,
  "breakdown": [
    {"name": "photos", "value": 524288000},
    {"name": "videos", "value": 536870912},
    {"name": "audio", "value": 12582912}
  ]
}
```

### resource-time-range

Filter resources by a time field within a range.

```json
{
  "tool": "resource-time-range",
  "params": {
    "name": "photos",
    "field": "mtime",           // "mtime", "ctime", "added_at"
    "min": "2024-01-01",
    "max": "2024-12-31",
    "include_children": true
  }
}
```

### resource-metric-range

Filter resources by a metric within a numeric range.

```json
{
  "tool": "resource-metric-range",
  "params": {
    "name": "videos",
    "metric": "size",
    "min": 1073741824,          // 1GB minimum
    "max": null,                // No maximum
    "include_children": true
  }
}
```

### resource-is

Exact match on a field value.

```json
{
  "tool": "resource-is",
  "params": {
    "name": "photos",
    "field": "kind",            // Any entry field: kind, path, parent, etc.
    "value": "file",            // Exact value to match
    "include_children": true
  }
}
```

**Use cases:**
- `field: "kind", value: "directory"` - Find all directories
- `field: "parent", value: "/home/user/Photos"` - Find immediate children
- `field: "path", value: "/home/user/file.txt"` - Find exact path

### resource-fuzzy-match

Fuzzy/pattern matching on text fields.

```json
{
  "tool": "resource-fuzzy-match",
  "params": {
    "name": "photos",
    "field": "path",            // Text field to search
    "pattern": "vacation",      // Pattern to match
    "mode": "contains",         // "contains", "prefix", "suffix", "regex", "glob"
    "case_sensitive": false,
    "include_children": true
  }
}
```

**Match modes:**
- `contains`: Path contains substring (SQL LIKE %pattern%)
- `prefix`: Path starts with pattern (SQL LIKE pattern%)
- `suffix`: Path ends with pattern (SQL LIKE %pattern)
- `regex`: Full regex matching (SQLite REGEXP)
- `glob`: Shell-style globbing (*, ?, [])

**Examples:**
```json
// Find all JPEG files
{"tool": "resource-fuzzy-match", "params": {"name": "photos", "field": "path", "pattern": ".jpg", "mode": "suffix"}}

// Find files in any "backup" directory
{"tool": "resource-fuzzy-match", "params": {"name": "all-files", "field": "path", "pattern": "*/backup/*", "mode": "glob"}}

// Find files matching regex
{"tool": "resource-fuzzy-match", "params": {"name": "docs", "field": "path", "pattern": "report_\\d{4}\\.pdf", "mode": "regex"}}
```

### resource-children

Get child nodes in the DAG (downstream navigation).

```json
{
  "tool": "resource-children",
  "params": {
    "name": "all-media",
    "depth": 1,                 // 1 = immediate, null = all
    "include_entries": false    // Include file entries?
  }
}
```

**Response:**
```json
{
  "resource_set": "all-media",
  "children": [
    {"name": "photos", "entry_count": 1234, "child_count": 2},
    {"name": "videos", "entry_count": 56, "child_count": 1},
    {"name": "audio", "entry_count": 789, "child_count": 0}
  ]
}
```

### resource-parent

Get parent nodes in the DAG (upstream navigation, like "..").

```json
{
  "tool": "resource-parent",
  "params": {
    "name": "vacation",
    "depth": 1                  // 1 = immediate, null = all ancestors
  }
}
```

**Response:**
```json
{
  "resource_set": "vacation",
  "parents": [
    {"name": "photos"},
    {"name": "videos"}          // Multiple parents allowed in DAG
  ]
}
```

---

## MCP Resource Templates

Resource-sets are exposed as MCP resources for declarative access:

### Resource URIs

| URI Pattern | Description |
|-------------|-------------|
| `synthesis://resource-set/{name}` | Resource-set metadata |
| `synthesis://resource-set/{name}/entries` | File entries in set |
| `synthesis://resource-set/{name}/children` | Child resource-sets |
| `synthesis://resource-set/{name}/parents` | Parent resource-sets |
| `synthesis://resource-set/{name}/metrics/{metric}` | Aggregated metric |
| `synthesis://resource-set/{name}/tree` | Full subtree (entries + children) |

### Example Resource Access

```json
// MCP resources/read request
{
  "method": "resources/read",
  "params": {
    "uri": "synthesis://resource-set/photos/metrics/size"
  }
}

// Response
{
  "contents": [
    {
      "uri": "synthesis://resource-set/photos/metrics/size",
      "mimeType": "application/json",
      "text": "{\"value\": 524288000, \"unit\": \"bytes\"}"
    }
  ]
}
```

### Resource Templates Definition

```json
{
  "resourceTemplates": [
    {
      "uriTemplate": "synthesis://resource-set/{name}",
      "name": "Resource Set",
      "description": "Access a resource-set by name",
      "mimeType": "application/json"
    },
    {
      "uriTemplate": "synthesis://resource-set/{name}/children",
      "name": "Resource Set Children",
      "description": "Child resource-sets in the DAG"
    },
    {
      "uriTemplate": "synthesis://resource-set/{name}/parents",
      "name": "Resource Set Parents",
      "description": "Parent resource-sets in the DAG"
    },
    {
      "uriTemplate": "synthesis://resource-set/{name}/entries?limit={limit}&offset={offset}",
      "name": "Resource Set Entries",
      "description": "File entries with pagination"
    },
    {
      "uriTemplate": "synthesis://resource-set/{name}/metrics/{metric}",
      "name": "Resource Metric",
      "description": "Aggregated metric value"
    }
  ]
}
```

---

## Unified Search Engine

All resource query operations (`resource-sum`, `resource-time-range`, `resource-metric-range`, `resource-is`, `resource-fuzzy-match`) are translated into a unified search mechanism that generates optimized SQL queries.

### Search Query Structure

```go
// ResourceQuery represents a unified query against resources
type ResourceQuery struct {
    ResourceSetName  string           `json:"resource_set"`
    IncludeChildren  bool             `json:"include_children"`
    Filters          []QueryFilter    `json:"filters"`
    Aggregation      *QueryAggregation `json:"aggregation,omitempty"`
    Pagination       *QueryPagination  `json:"pagination,omitempty"`
}

// QueryFilter represents a single filter condition
type QueryFilter struct {
    Field     string      `json:"field"`     // "path", "size", "mtime", "kind", etc.
    Operator  string      `json:"operator"`  // "eq", "ne", "gt", "gte", "lt", "lte", "in", "like", "regex", "glob"
    Value     interface{} `json:"value"`
    CaseSensitive bool    `json:"case_sensitive,omitempty"`
}

// QueryAggregation for metrics
type QueryAggregation struct {
    Function string `json:"function"` // "sum", "count", "avg", "min", "max"
    Field    string `json:"field"`    // Field to aggregate
    GroupBy  string `json:"group_by,omitempty"` // Optional grouping
}
```

### Tool to SQL Translation

| Tool | Filter Operator | SQL Translation |
|------|-----------------|-----------------|
| `resource-is` | `eq` | `field = ?` |
| `resource-fuzzy-match` (contains) | `like` | `field LIKE '%' \|\| ? \|\| '%'` |
| `resource-fuzzy-match` (prefix) | `like_prefix` | `field LIKE ? \|\| '%'` |
| `resource-fuzzy-match` (suffix) | `like_suffix` | `field LIKE '%' \|\| ?` |
| `resource-fuzzy-match` (regex) | `regex` | `field REGEXP ?` |
| `resource-fuzzy-match` (glob) | `glob` | `field GLOB ?` |
| `resource-time-range` | `gte` + `lte` | `field >= ? AND field <= ?` |
| `resource-metric-range` | `gte` + `lte` | `field >= ? AND field <= ?` |
| `resource-sum` | (aggregation) | `SELECT SUM(field) FROM ...` |

### Query Execution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    MCP Tool Request                              │
│  resource-fuzzy-match(name="photos", field="path",              │
│                       pattern="vacation", mode="contains")       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Query Builder                                 │
│  ResourceQuery{                                                  │
│    ResourceSetName: "photos",                                    │
│    IncludeChildren: true,                                        │
│    Filters: [{Field: "path", Operator: "like", Value: "vacation"}]│
│  }                                                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    SQL Generator                                 │
│  WITH RECURSIVE descendants AS (                                 │
│    SELECT id FROM resource_sets WHERE name = 'photos'            │
│    UNION ALL                                                     │
│    SELECT e.child_id FROM resource_set_edges e                   │
│    JOIN descendants d ON e.parent_id = d.id                      │
│  )                                                               │
│  SELECT e.* FROM entries e                                       │
│  JOIN resource_set_entries rse ON e.path = rse.entry_path        │
│  WHERE rse.set_id IN (SELECT id FROM descendants)                │
│    AND e.path LIKE '%vacation%'                                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Result Set                                    │
│  [Entry{path: "/photos/vacation/beach.jpg", ...},               │
│   Entry{path: "/photos/vacation/hotel.png", ...}]               │
└─────────────────────────────────────────────────────────────────┘
```

### Composable Filters

Multiple filters can be combined using AND/OR logic:

```go
// Multiple filters (AND by default)
ResourceQuery{
    Filters: []QueryFilter{
        {Field: "kind", Operator: "eq", Value: "file"},
        {Field: "size", Operator: "gte", Value: 1048576},
        {Field: "path", Operator: "like_suffix", Value: ".jpg"},
    }
}

// Translates to:
// WHERE kind = 'file' AND size >= 1048576 AND path LIKE '%.jpg'
```

### DAG Traversal

When `include_children: true`, the query uses a recursive CTE to collect all descendant resource-sets:

```sql
WITH RECURSIVE descendants(id) AS (
    -- Base case: the starting resource-set
    SELECT id FROM resource_sets WHERE name = ?

    UNION ALL

    -- Recursive case: all children
    SELECT e.child_id
    FROM resource_set_edges e
    JOIN descendants d ON e.parent_id = d.id
)
SELECT DISTINCT e.*
FROM entries e
JOIN resource_set_entries rse ON e.path = rse.entry_path
WHERE rse.set_id IN (SELECT id FROM descendants)
  AND [filters...]
```

### Performance Optimizations

1. **Index Usage**: Queries leverage indexes on `path`, `mtime`, `size`, `kind`
2. **CTE Materialization**: Recursive CTEs are materialized once for DAG traversal
3. **Lazy Loading**: Large result sets use cursor-based pagination
4. **Query Caching**: Identical queries within a time window can be cached

---

## Data Model

### ResourceSet (DAG Node)

```go
type ResourceSet struct {
    ID          int64   `db:"id" json:"id"`
    Name        string  `db:"name" json:"name"`
    Description *string `db:"description" json:"description,omitempty"`
    CreatedAt   int64   `db:"created_at" json:"created_at"`
    UpdatedAt   int64   `db:"updated_at" json:"updated_at"`
}

// DAG edges (supports multiple parents)
type ResourceSetEdge struct {
    ParentID  int64 `db:"parent_id" json:"parent_id"`
    ChildID   int64 `db:"child_id" json:"child_id"`
    AddedAt   int64 `db:"added_at" json:"added_at"`
}

// File entries within a resource-set
type ResourceSetEntry struct {
    SetID     int64  `db:"set_id" json:"set_id"`
    EntryPath string `db:"entry_path" json:"entry_path"`
    AddedAt   int64  `db:"added_at" json:"added_at"`
}
```

### Source (Unified)

```go
type SourceType string

const (
    SourceTypeFilesystemIndex SourceType = "filesystem.index"
    SourceTypeFilesystemWatch SourceType = "filesystem.watch"
    SourceTypeQuery           SourceType = "query"
    SourceTypeResourceSet     SourceType = "resource-set"
)

type Source struct {
    ID            int64        `db:"id" json:"id"`
    Name          string       `db:"name" json:"name"`
    Type          SourceType   `db:"type" json:"type"`
    TargetSetName string       `db:"target_set_name" json:"target_set_name"`
    UpdateMode    string       `db:"update_mode" json:"update_mode"`
    ConfigJSON    string       `db:"config_json" json:"-"`
    Status        SourceStatus `db:"status" json:"status"`
    Enabled       bool         `db:"enabled" json:"enabled"`
    CreatedAt     int64        `db:"created_at" json:"created_at"`
    UpdatedAt     int64        `db:"updated_at" json:"updated_at"`
    LastRunAt     *int64       `db:"last_run_at" json:"last_run_at,omitempty"`
    LastError     *string      `db:"last_error" json:"last_error,omitempty"`
}
```

### Plan (Orchestrator)

```go
type Plan struct {
    ID               int64   `db:"id" json:"id"`
    Name             string  `db:"name" json:"name"`
    Description      *string `db:"description" json:"description,omitempty"`
    Mode             string  `db:"mode" json:"mode"`     // "oneshot", "continuous"
    Status           string  `db:"status" json:"status"` // "active", "paused", "disabled"
    ResourceSetsJSON string  `db:"resource_sets_json" json:"-"`
    SourcesJSON      string  `db:"sources_json" json:"-"`
    ConditionsJSON   *string `db:"conditions_json" json:"-"`
    OutcomesJSON     *string `db:"outcomes_json" json:"-"`
    CreatedAt        int64   `db:"created_at" json:"created_at"`
    UpdatedAt        int64   `db:"updated_at" json:"updated_at"`
    LastRunAt        *int64  `db:"last_run_at" json:"last_run_at,omitempty"`
}
```

---

## Database Schema

```sql
-- Resource-sets (DAG nodes)
CREATE TABLE resource_sets (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
);

-- DAG edges (parent-child relationships, supports multiple parents)
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

CREATE INDEX idx_set_entries ON resource_set_entries(set_id);

-- Unified sources (all ingestion mechanisms)
CREATE TABLE sources (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('filesystem.index', 'filesystem.watch', 'query', 'resource-set')),
    target_set_name TEXT NOT NULL,
    update_mode TEXT DEFAULT 'append' CHECK(update_mode IN ('replace', 'append', 'merge')),
    config_json TEXT NOT NULL,
    status TEXT DEFAULT 'stopped' CHECK(status IN ('stopped', 'starting', 'running', 'stopping', 'completed', 'error')),
    enabled INTEGER DEFAULT 1,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER,
    last_error TEXT,
    FOREIGN KEY (target_set_name) REFERENCES resource_sets(name) ON DELETE CASCADE
);

-- Plans (orchestration of resource-sets and sources)
CREATE TABLE plans (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    mode TEXT DEFAULT 'oneshot' CHECK(mode IN ('oneshot', 'continuous')),
    status TEXT DEFAULT 'active' CHECK(status IN ('active', 'paused', 'disabled')),
    resource_sets_json TEXT NOT NULL,
    sources_json TEXT NOT NULL,
    conditions_json TEXT,
    outcomes_json TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER
);
```

---

## DAG Cycle Prevention

When adding an edge (parent → child), verify no cycle would be created:

```go
func (db *DiskDB) AddResourceSetChild(parentName, childName string) error {
    // 1. Get IDs
    parent, child := getByName(parentName), getByName(childName)

    // 2. Check for self-reference
    if parent.ID == child.ID {
        return ErrSelfReference
    }

    // 3. Check if adding this edge would create a cycle
    // (child cannot be an ancestor of parent)
    if db.isAncestor(child.ID, parent.ID) {
        return ErrCycleDetected
    }

    // 4. Add edge
    return db.insertEdge(parent.ID, child.ID)
}

func (db *DiskDB) isAncestor(potentialAncestor, node int64) bool {
    // BFS/DFS from node upward through parents
    // Return true if potentialAncestor is found
}
```

---

## API Summary

### MCP Tools

**Resource Navigation:**
| Tool | Description |
|------|-------------|
| `resource-set-create` | Create a resource-set node |
| `resource-set-list` | List all resource-sets |
| `resource-set-get` | Get resource-set metadata |
| `resource-set-modify` | Add/remove entries |
| `resource-set-delete` | Delete a resource-set |
| `resource-set-add-child` | Create parent→child edge |
| `resource-set-remove-child` | Remove parent→child edge |
| `resource-children` | Get child nodes in DAG |
| `resource-parent` | Get parent nodes in DAG |

**Resource Queries:**
| Tool | Description |
|------|-------------|
| `resource-sum` | Hierarchical metric aggregation |
| `resource-time-range` | Filter by time field range |
| `resource-metric-range` | Filter by metric range |
| `resource-is` | Exact match on a field value |
| `resource-fuzzy-match` | Fuzzy/pattern matching on text fields |

**Sources (Data Ingestion):**
| Tool | Description |
|------|-------------|
| `source-create` | Create a source |
| `source-start` | Start a source |
| `source-stop` | Stop a source |
| `source-list` | List all sources |
| `source-get` | Get source details |
| `source-delete` | Delete a source |
| `source-execute` | Execute source once |

**Plans (Orchestration):**
| Tool | Description |
|------|-------------|
| `plan-create` | Create a plan with sources |
| `plan-execute` | Run a plan (triggers indexing) |
| `plan-list` | List all plans |
| `plan-get` | Get plan details |
| `plan-update` | Modify plan |
| `plan-delete` | Remove plan |

**Removed Tools:**
| Old Tool | Replacement |
|----------|-------------|
| `disk-index` | `plan-execute` with filesystem.index source |
| `disk-du` | `resource-sum` with metric: "size" |
| `disk-time-range` | `resource-time-range` |
| `disk-tree` | `resource-children` + `resource-set-get` |
| `navigate` | `resource-children` + `resource-parent` |

### MCP Resources

| URI Template | Description |
|--------------|-------------|
| `synthesis://resource-set/{name}` | Resource-set metadata |
| `synthesis://resource-set/{name}/entries` | File entries |
| `synthesis://resource-set/{name}/children` | Child sets |
| `synthesis://resource-set/{name}/parents` | Parent sets |
| `synthesis://resource-set/{name}/metrics/{metric}` | Aggregated metric |

---

## Example Workflows

### 1. Initial Setup (Index Files via Plan)

```json
// Step 1: Create resource-sets
{"tool": "resource-set-create", "params": {"name": "photos"}}
{"tool": "resource-set-create", "params": {"name": "videos"}}
{"tool": "resource-set-create", "params": {"name": "all-media"}}

// Step 2: Build DAG structure
{"tool": "resource-set-add-child", "params": {"parent": "all-media", "child": "photos"}}
{"tool": "resource-set-add-child", "params": {"parent": "all-media", "child": "videos"}}

// Step 3: Create plan with sources
{
  "tool": "plan-create",
  "params": {
    "name": "index-media",
    "sources": [
      {"name": "index-photos", "type": "filesystem.index", "target_set": "photos",
       "config": {"path": "/home/user/Photos", "recursive": true}},
      {"name": "index-videos", "type": "filesystem.index", "target_set": "videos",
       "config": {"path": "/home/user/Videos", "recursive": true}}
    ]
  }
}

// Step 4: Execute plan (this does the indexing!)
{"tool": "plan-execute", "params": {"name": "index-media"}}
```

### 2. Query Resources

```json
// Get total size of all media (aggregated through DAG)
{"tool": "resource-sum", "params": {"name": "all-media", "metric": "size"}}

// Find large videos
{"tool": "resource-metric-range", "params": {"name": "videos", "metric": "size", "min": 1073741824}}

// Find recent photos
{"tool": "resource-time-range", "params": {"name": "photos", "field": "mtime", "min": "2024-01-01"}}
```

### 3. Navigate DAG

```json
// What's in all-media?
{"tool": "resource-children", "params": {"name": "all-media"}}

// What sets contain "vacation"?
{"tool": "resource-parent", "params": {"name": "vacation"}}
```

---

## Resource-Set Layering

Resource-sets can be organized in hierarchical layers to represent different organizational perspectives. The DAG structure allows flexible categorization where resources can belong to multiple hierarchies simultaneously.

### Example: Media Organization

```
all-resources (root)
├── media
│   ├── images
│   │   ├── photos
│   │   │   ├── vacation-2024
│   │   │   └── family-portraits
│   │   └── screenshots
│   ├── videos
│   │   ├── movies
│   │   └── tutorials
│   └── audio
│       ├── music
│       └── podcasts
├── documents
│   ├── work
│   └── personal
└── by-year
    ├── 2023
    └── 2024
```

### Creating a Layered Structure

```json
// Step 1: Create leaf-level resource-sets
{"tool": "resource-set-create", "params": {"name": "vacation-2024"}}
{"tool": "resource-set-create", "params": {"name": "family-portraits"}}
{"tool": "resource-set-create", "params": {"name": "screenshots"}}
{"tool": "resource-set-create", "params": {"name": "movies"}}
{"tool": "resource-set-create", "params": {"name": "tutorials"}}

// Step 2: Create intermediate layers
{"tool": "resource-set-create", "params": {"name": "photos"}}
{"tool": "resource-set-create", "params": {"name": "videos"}}
{"tool": "resource-set-create", "params": {"name": "images"}}
{"tool": "resource-set-create", "params": {"name": "media"}}
{"tool": "resource-set-create", "params": {"name": "all-resources"}}

// Step 3: Build the hierarchy bottom-up
{"tool": "resource-set-add-child", "params": {"parent": "photos", "child": "vacation-2024"}}
{"tool": "resource-set-add-child", "params": {"parent": "photos", "child": "family-portraits"}}
{"tool": "resource-set-add-child", "params": {"parent": "images", "child": "photos"}}
{"tool": "resource-set-add-child", "params": {"parent": "images", "child": "screenshots"}}
{"tool": "resource-set-add-child", "params": {"parent": "videos", "child": "movies"}}
{"tool": "resource-set-add-child", "params": {"parent": "videos", "child": "tutorials"}}
{"tool": "resource-set-add-child", "params": {"parent": "media", "child": "images"}}
{"tool": "resource-set-add-child", "params": {"parent": "media", "child": "videos"}}
{"tool": "resource-set-add-child", "params": {"parent": "all-resources", "child": "media"}}
```

### Cross-Cutting Hierarchies (Multiple Parents)

Resources can belong to multiple hierarchies. For example, vacation photos from 2024 can be in both the content-type hierarchy AND the temporal hierarchy:

```json
// vacation-2024 already in: photos → images → media → all-resources
// Add to temporal hierarchy as well:
{"tool": "resource-set-add-child", "params": {"parent": "2024", "child": "vacation-2024"}}
{"tool": "resource-set-add-child", "params": {"parent": "by-year", "child": "2024"}}
{"tool": "resource-set-add-child", "params": {"parent": "all-resources", "child": "by-year"}}
```

Now `vacation-2024` has two parents: `photos` and `2024`. Queries against either path will find the same resources.

### Aggregation Through Layers

When aggregating metrics, values roll up through the DAG:

```json
// Get total size of all media
{"tool": "resource-sum", "params": {"name": "media", "metric": "size"}}
// Response: Aggregates size from images + videos + audio (and all children)

// Get total size of just photos
{"tool": "resource-sum", "params": {"name": "photos", "metric": "size"}}
// Response: Aggregates size from vacation-2024 + family-portraits

// Get total size by year
{"tool": "resource-sum", "params": {"name": "2024", "metric": "size"}}
// Response: Aggregates all resources from 2024 (regardless of content type)
```

### Automating Layer Population with Plans

Use plans to automatically populate resource-sets based on conditions:

```json
{
  "tool": "plan-create",
  "params": {
    "name": "organize-photos-by-year",
    "mode": "oneshot",
    "sources": [
      {"type": "filesystem", "paths": ["/home/user/Photos"]}
    ],
    "conditions": {
      "type": "all",
      "conditions": [
        {"type": "media_type", "media_type": "image"},
        {"type": "time", "min_mtime": 1704067200, "max_mtime": 1735689599}
      ]
    },
    "outcomes": [
      {
        "tool": "resource-set-modify",
        "arguments": {
          "name": "2024",
          "operation": "add"
        }
      }
    ]
  }
}
```

---

## Benefits

1. **Resource-Neutral**: Operations work on any resources, not tied to files
2. **DAG Flexibility**: Multiple inheritance paths, no rigid hierarchy
3. **Bidirectional Navigation**: Easy to traverse up and down
4. **Metric Aggregation**: Roll up any metric through the graph
5. **Dual Access**: Tools for actions, Resources for state
6. **Clean Separation**: Plans own ingestion, Sets own storage
7. **Extensible**: Easy to add new source types, metrics, operations
