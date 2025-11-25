# Resource-Set Architecture

## Overview

This document describes the architectural refactoring from "Selection Sets" to "Resource Sets" with three major enhancements:

1. **Rename**: selection-set → resource-set
2. **Nesting**: Resource-sets can contain other resource-sets
3. **Unified Sources**: Rationalize query, source, and plan systems into a single "Sources" abstraction

## Design Principles

1. **Composition over Configuration**: Resource-sets compose through nesting rather than complex configuration
2. **Single Abstraction**: "Source" is the unified concept for anything that populates a resource-set
3. **Plans as Orchestrators**: Plans combine resource-sets with sources and define execution flow
4. **Audit Trail**: All operations are tracked for debugging and compliance

---

## Core Concepts

### 1. Resource-Set (formerly Selection-Set)

A **Resource-Set** is a named collection that can contain:
- File/directory entries (paths from the `entries` table)
- References to other resource-sets (enabling composition)

```
┌─────────────────────────────────────────────────────────────┐
│                      Resource-Set                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Entries (files/directories)                          │   │
│  │  - /home/user/photos/vacation.jpg                    │   │
│  │  - /home/user/photos/birthday.mp4                    │   │
│  └─────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Child Resource-Sets (nested)                         │   │
│  │  - "archived-photos" (resource-set reference)         │   │
│  │  - "shared-photos" (resource-set reference)           │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Key Properties:**
- Pure storage container (no logic about HOW items got there)
- Tracks when each item was added
- Can be nested arbitrarily deep (cycles prevented by validation)
- Can be used as input/output for Sources

### 2. Source (Unified Abstraction)

A **Source** is anything that populates a resource-set with entries. This unifies:
- Current `sources` table (filesystem.index, filesystem.watch)
- Current `queries` table (query filters)
- Plan source resolution logic

```
┌─────────────────────────────────────────────────────────────┐
│                         SOURCES                              │
├─────────────────────────────────────────────────────────────┤
│  filesystem.index     │  One-time filesystem scan           │
│  filesystem.watch     │  Real-time filesystem monitoring    │
│  query                │  File filter against indexed data   │
│  resource-set         │  Copy from another resource-set     │
└─────────────────────────────────────────────────────────────┘
```

**Source Types:**

| Type | Description | Config |
|------|-------------|--------|
| `filesystem.index` | One-time scan of directory tree | `path`, `recursive`, `max_depth` |
| `filesystem.watch` | Live monitoring with fsnotify | `path`, `recursive`, `debounce_ms` |
| `query` | Filter entries using FileFilter | `filter` (path, size, date, pattern) |
| `resource-set` | Reference another resource-set | `source_set_name`, `operation` |

**Source Lifecycle:**
1. **Created**: Source definition stored in database
2. **Started**: Source begins populating its target resource-set
3. **Running**: Actively processing (continuous for watch, finite for others)
4. **Stopped**: Source stops populating
5. **Completed**: For finite sources, indicates successful completion

### 3. Plan

A **Plan** orchestrates resource-sets and sources to define complete data processing workflows.

```
┌─────────────────────────────────────────────────────────────┐
│                          PLAN                                │
│                                                              │
│  ┌───────────────┐      ┌───────────────┐                  │
│  │ Resource-Sets │      │    Sources    │                  │
│  │               │←─────│               │                  │
│  │ - photos      │      │ - fs.watch    │                  │
│  │ - videos      │      │ - query       │                  │
│  │ - media (nest)│      │               │                  │
│  └───────────────┘      └───────────────┘                  │
│                                                              │
│  Execution: oneshot | continuous                             │
│  Status: active | paused | disabled                          │
└─────────────────────────────────────────────────────────────┘
```

**Plan Components:**
- **Resource-Sets**: The containers being managed
- **Sources**: The mechanisms populating those containers
- **Conditions**: Optional filters applied during source execution
- **Outcomes**: Actions taken on matched entries

---

## Data Model

### Resource-Set

```go
// ResourceSet is a named collection of entries and/or other resource-sets
type ResourceSet struct {
    ID          int64   `db:"id" json:"id"`
    Name        string  `db:"name" json:"name"`
    Description *string `db:"description" json:"description,omitempty"`
    CreatedAt   int64   `db:"created_at" json:"created_at"`
    UpdatedAt   int64   `db:"updated_at" json:"updated_at"`
}

// ResourceSetEntry links a resource-set to a file/directory entry
type ResourceSetEntry struct {
    SetID     int64  `db:"set_id" json:"set_id"`
    EntryPath string `db:"entry_path" json:"entry_path"`
    AddedAt   int64  `db:"added_at" json:"added_at"`
}

// ResourceSetChild links a parent resource-set to a child resource-set
type ResourceSetChild struct {
    ParentID  int64  `db:"parent_id" json:"parent_id"`
    ChildID   int64  `db:"child_id" json:"child_id"`
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

type SourceStatus string

const (
    SourceStatusStopped   SourceStatus = "stopped"
    SourceStatusStarting  SourceStatus = "starting"
    SourceStatusRunning   SourceStatus = "running"
    SourceStatusStopping  SourceStatus = "stopping"
    SourceStatusCompleted SourceStatus = "completed"
    SourceStatusError     SourceStatus = "error"
)

// Source represents any mechanism that populates a resource-set
type Source struct {
    ID              int64        `db:"id" json:"id"`
    Name            string       `db:"name" json:"name"`
    Type            SourceType   `db:"type" json:"type"`

    // Target resource-set this source populates
    TargetSetName   string       `db:"target_set_name" json:"target_set_name"`
    UpdateMode      string       `db:"update_mode" json:"update_mode"` // "replace", "append", "merge"

    // Type-specific configuration (JSON)
    ConfigJSON      string       `db:"config_json" json:"-"`

    // Runtime state
    Status          SourceStatus `db:"status" json:"status"`
    Enabled         bool         `db:"enabled" json:"enabled"`

    // Metadata
    CreatedAt       int64        `db:"created_at" json:"created_at"`
    UpdatedAt       int64        `db:"updated_at" json:"updated_at"`
    LastRunAt       *int64       `db:"last_run_at" json:"last_run_at,omitempty"`
    LastError       *string      `db:"last_error" json:"last_error,omitempty"`

    // Parsed config (not stored)
    Config          interface{}  `db:"-" json:"config,omitempty"`
}

// FilesystemIndexConfig for filesystem.index sources
type FilesystemIndexConfig struct {
    Path       string `json:"path"`
    Recursive  bool   `json:"recursive"`
    MaxDepth   *int   `json:"max_depth,omitempty"`
    IncludeHidden bool `json:"include_hidden"`
}

// FilesystemWatchConfig for filesystem.watch sources
type FilesystemWatchConfig struct {
    Path           string `json:"path"`
    Recursive      bool   `json:"recursive"`
    DebounceMs     int    `json:"debounce_ms"`
    BatchSize      int    `json:"batch_size"`
}

// QueryConfig for query sources
type QueryConfig struct {
    Filter FileFilter `json:"filter"`
}

// ResourceSetSourceConfig for resource-set sources
type ResourceSetSourceConfig struct {
    SourceSetName string `json:"source_set_name"`
    IncludeNested bool   `json:"include_nested"` // Flatten nested sets
}
```

### Plan

```go
// Plan orchestrates resource-sets and sources
type Plan struct {
    ID             int64   `db:"id" json:"id"`
    Name           string  `db:"name" json:"name"`
    Description    *string `db:"description" json:"description,omitempty"`

    // Execution mode
    Mode           string  `db:"mode" json:"mode"`     // "oneshot", "continuous"
    Status         string  `db:"status" json:"status"` // "active", "paused", "disabled"

    // Configuration (JSON)
    ResourceSetsJSON string  `db:"resource_sets_json" json:"-"` // Resource-sets managed by this plan
    SourcesJSON      string  `db:"sources_json" json:"-"`       // Sources that populate the sets
    ConditionsJSON   *string `db:"conditions_json" json:"-"`    // Optional filtering
    OutcomesJSON     *string `db:"outcomes_json" json:"-"`      // Optional actions

    // Metadata
    CreatedAt      int64  `db:"created_at" json:"created_at"`
    UpdatedAt      int64  `db:"updated_at" json:"updated_at"`
    LastRunAt      *int64 `db:"last_run_at" json:"last_run_at,omitempty"`

    // Parsed fields (not stored)
    ResourceSets   []PlanResourceSet `db:"-" json:"resource_sets,omitempty"`
    Sources        []PlanSource      `db:"-" json:"sources,omitempty"`
    Conditions     *RuleCondition    `db:"-" json:"conditions,omitempty"`
    Outcomes       []RuleOutcome     `db:"-" json:"outcomes,omitempty"`
}

// PlanResourceSet defines a resource-set within a plan
type PlanResourceSet struct {
    Name        string   `json:"name"`
    Description *string  `json:"description,omitempty"`
    Children    []string `json:"children,omitempty"` // Nested resource-set names
}

// PlanSource references a source by name and defines execution order
type PlanSource struct {
    SourceName  string   `json:"source_name"`
    DependsOn   []string `json:"depends_on,omitempty"` // Source names this depends on
    Conditions  *RuleCondition `json:"conditions,omitempty"` // Override plan-level conditions
}
```

---

## Database Schema

### New Tables

```sql
-- Resource sets (renamed from selection_sets)
CREATE TABLE resource_sets (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
);

-- Resource set entries (links to filesystem entries)
CREATE TABLE resource_set_entries (
    set_id INTEGER NOT NULL,
    entry_path TEXT NOT NULL,
    added_at INTEGER DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (set_id, entry_path),
    FOREIGN KEY (set_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
);

CREATE INDEX idx_resource_set_entries ON resource_set_entries(set_id);

-- Resource set children (nesting support)
CREATE TABLE resource_set_children (
    parent_id INTEGER NOT NULL,
    child_id INTEGER NOT NULL,
    added_at INTEGER DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (parent_id, child_id),
    FOREIGN KEY (parent_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (child_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    CHECK (parent_id != child_id)
);

CREATE INDEX idx_resource_set_children_parent ON resource_set_children(parent_id);
CREATE INDEX idx_resource_set_children_child ON resource_set_children(child_id);

-- Unified sources table (replaces both sources and queries tables)
CREATE TABLE sources (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('filesystem.index', 'filesystem.watch', 'query', 'resource-set')),

    -- Target resource-set
    target_set_name TEXT NOT NULL,
    update_mode TEXT DEFAULT 'append' CHECK(update_mode IN ('replace', 'append', 'merge')),

    -- Type-specific configuration
    config_json TEXT NOT NULL,

    -- Runtime state
    status TEXT DEFAULT 'stopped' CHECK(status IN ('stopped', 'starting', 'running', 'stopping', 'completed', 'error')),
    enabled INTEGER DEFAULT 1,

    -- Metadata
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER,
    last_error TEXT,

    FOREIGN KEY (target_set_name) REFERENCES resource_sets(name) ON DELETE CASCADE
);

CREATE INDEX idx_sources_type ON sources(type);
CREATE INDEX idx_sources_target ON sources(target_set_name);
CREATE INDEX idx_sources_status ON sources(status);

-- Source execution history
CREATE TABLE source_executions (
    id INTEGER PRIMARY KEY,
    source_id INTEGER NOT NULL,
    source_name TEXT NOT NULL,

    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    duration_ms INTEGER,

    entries_processed INTEGER DEFAULT 0,
    entries_added INTEGER DEFAULT 0,
    entries_removed INTEGER DEFAULT 0,

    status TEXT DEFAULT 'running' CHECK(status IN ('running', 'success', 'partial', 'error')),
    error_message TEXT,

    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

CREATE INDEX idx_source_executions_source ON source_executions(source_id);
CREATE INDEX idx_source_executions_status ON source_executions(status);

-- Plans table (orchestration layer)
CREATE TABLE plans (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    mode TEXT DEFAULT 'oneshot' CHECK(mode IN ('oneshot', 'continuous')),
    status TEXT DEFAULT 'active' CHECK(status IN ('active', 'paused', 'disabled')),

    resource_sets_json TEXT NOT NULL,  -- JSON array of PlanResourceSet
    sources_json TEXT NOT NULL,        -- JSON array of PlanSource
    conditions_json TEXT,              -- Optional RuleCondition
    outcomes_json TEXT,                -- Optional RuleOutcome array

    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER
);

CREATE INDEX idx_plans_status ON plans(status);
CREATE INDEX idx_plans_mode ON plans(mode);

-- Plan execution history
CREATE TABLE plan_executions (
    id INTEGER PRIMARY KEY,
    plan_id INTEGER NOT NULL,
    plan_name TEXT NOT NULL,

    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    duration_ms INTEGER,

    sources_executed INTEGER DEFAULT 0,
    entries_processed INTEGER DEFAULT 0,
    entries_matched INTEGER DEFAULT 0,
    outcomes_applied INTEGER DEFAULT 0,

    status TEXT DEFAULT 'running' CHECK(status IN ('running', 'success', 'partial', 'error')),
    error_message TEXT,

    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE
);

CREATE INDEX idx_plan_executions_plan ON plan_executions(plan_id);
```

---

## Migration Strategy

### Phase 1: Schema Migration

```sql
-- 1. Create new resource_sets table
CREATE TABLE resource_sets AS SELECT id, name, description, created_at, updated_at FROM selection_sets;

-- 2. Create new resource_set_entries table
CREATE TABLE resource_set_entries AS SELECT set_id, entry_path, added_at FROM selection_set_entries;

-- 3. Create resource_set_children table (new)
CREATE TABLE resource_set_children (...);

-- 4. Migrate queries to sources
INSERT INTO sources (name, type, target_set_name, update_mode, config_json, status, enabled, created_at, updated_at, last_run_at)
SELECT
    name,
    'query',
    COALESCE(target_selection_set, name || '-results'),
    COALESCE(update_mode, 'replace'),
    query_json,
    'stopped',
    1,
    created_at,
    updated_at,
    last_executed
FROM queries;

-- 5. Migrate existing sources (filesystem only)
UPDATE sources SET type =
    CASE
        WHEN type = 'manual' THEN 'filesystem.index'
        WHEN type = 'live' THEN 'filesystem.watch'
        ELSE type
    END;

-- 6. Drop old tables (after verification)
DROP TABLE selection_sets;
DROP TABLE selection_set_entries;
DROP TABLE queries;
DROP TABLE query_executions;
```

### Phase 2: Code Migration

1. **Rename all Go types**: `SelectionSet` → `ResourceSet`
2. **Update database methods**: `pkg/database/selection_sets.go` → `pkg/database/resource_sets.go`
3. **Unify source handling**: Merge `pkg/sources/` with query execution
4. **Update MCP tools**: `selection-set-*` → `resource-set-*`
5. **Update tests**

### Phase 3: Backward Compatibility

- Provide MCP tool aliases: `selection-set-*` redirects to `resource-set-*` with deprecation warning
- Migration CLI command: `mcp-space-browser migrate --from=v1 --to=v2`

---

## API Changes

### MCP Tools

**Renamed Tools:**
| Old Name | New Name |
|----------|----------|
| `selection-set-create` | `resource-set-create` |
| `selection-set-list` | `resource-set-list` |
| `selection-set-get` | `resource-set-get` |
| `selection-set-modify` | `resource-set-modify` |
| `selection-set-delete` | `resource-set-delete` |

**New Tools for Nesting:**
| Tool | Description |
|------|-------------|
| `resource-set-add-child` | Add a child resource-set to a parent |
| `resource-set-remove-child` | Remove a child resource-set |
| `resource-set-get-all` | Get all entries including nested sets (flattened) |

**Unified Source Tools:**
| Tool | Description |
|------|-------------|
| `source-create` | Create any source type |
| `source-start` | Start a source |
| `source-stop` | Stop a source |
| `source-list` | List all sources (all types) |
| `source-get` | Get source details |
| `source-delete` | Delete a source |
| `source-execute` | Execute a source once (for query/index types) |

**Removed Tools (absorbed into source-*):**
| Old Tool | Replacement |
|----------|-------------|
| `query-create` | `source-create` with `type: "query"` |
| `query-execute` | `source-execute` |
| `query-list` | `source-list` with `type: "query"` filter |
| `query-get` | `source-get` |
| `query-update` | `source-update` |
| `query-delete` | `source-delete` |

---

## Example Usage

### Creating a Nested Resource-Set Structure

```json
// Create parent resource-set
{
  "tool": "resource-set-create",
  "params": {
    "name": "all-media",
    "description": "All media files"
  }
}

// Create child resource-sets
{
  "tool": "resource-set-create",
  "params": {
    "name": "photos",
    "description": "Photo files"
  }
}

{
  "tool": "resource-set-create",
  "params": {
    "name": "videos",
    "description": "Video files"
  }
}

// Add children to parent
{
  "tool": "resource-set-add-child",
  "params": {
    "parent": "all-media",
    "child": "photos"
  }
}

{
  "tool": "resource-set-add-child",
  "params": {
    "parent": "all-media",
    "child": "videos"
  }
}

// Get all entries (flattened from all nested sets)
{
  "tool": "resource-set-get-all",
  "params": {
    "name": "all-media"
  }
}
```

### Creating Sources to Populate Resource-Sets

```json
// Filesystem watch source for photos
{
  "tool": "source-create",
  "params": {
    "name": "watch-photos",
    "type": "filesystem.watch",
    "target_set": "photos",
    "config": {
      "path": "/home/user/Photos",
      "recursive": true,
      "debounce_ms": 500
    }
  }
}

// Query source for large videos
{
  "tool": "source-create",
  "params": {
    "name": "large-videos-query",
    "type": "query",
    "target_set": "videos",
    "update_mode": "append",
    "config": {
      "filter": {
        "path": "/home/user/Videos",
        "extensions": ["mp4", "mkv", "avi"],
        "minSize": 1073741824
      }
    }
  }
}

// Start the watch source
{
  "tool": "source-start",
  "params": {
    "name": "watch-photos"
  }
}

// Execute the query source
{
  "tool": "source-execute",
  "params": {
    "name": "large-videos-query"
  }
}
```

### Creating a Plan

```json
{
  "tool": "plan-create",
  "params": {
    "name": "media-organizer",
    "description": "Organize all media files",
    "mode": "continuous",
    "resource_sets": [
      {
        "name": "all-media",
        "children": ["photos", "videos", "audio"]
      }
    ],
    "sources": [
      {"source_name": "watch-photos"},
      {"source_name": "watch-videos"},
      {"source_name": "large-videos-query", "depends_on": ["watch-videos"]}
    ],
    "conditions": {
      "type": "any",
      "conditions": [
        {"type": "media_type", "mediaType": "image"},
        {"type": "media_type", "mediaType": "video"},
        {"type": "media_type", "mediaType": "audio"}
      ]
    },
    "outcomes": [
      {
        "type": "selection_set",
        "selectionSetName": "processed-media",
        "operation": "add"
      }
    ]
  }
}
```

---

## Benefits of This Architecture

1. **Simplified Mental Model**: One abstraction (Source) for all data population
2. **Composability**: Resource-sets compose through nesting
3. **Flexibility**: Any source type can target any resource-set
4. **Consistency**: Uniform API for all source operations
5. **Extensibility**: Easy to add new source types (S3, HTTP, etc.)
6. **Auditability**: Complete execution history for all sources

---

## Comparison: Before vs After

| Aspect | Before | After |
|--------|--------|-------|
| Collections | SelectionSet (flat) | ResourceSet (nestable) |
| Filesystem Indexing | Source (manual/live) | Source (filesystem.index/watch) |
| Query Filtering | Query (separate system) | Source (type: query) |
| Plan Sources | 3 types (filesystem, selection_set, query) | References Source by name |
| APIs | 3 separate tool groups | 2 unified tool groups |
| Database Tables | 4 tables (selection_sets, queries, sources, plans) | 3 tables (resource_sets, sources, plans) |
