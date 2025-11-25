# Resource-Set Refactoring Implementation Plan

This document provides a detailed implementation plan for refactoring selection-sets to resource-sets with nesting support and unified sources.

## Implementation Phases

### Phase 1: Database Schema Changes

**Objective**: Create new tables and migrate existing data

#### 1.1 Create Migration Script

**File**: `pkg/database/migrations/002_resource_sets.sql`

```sql
-- Create resource_sets table (copy from selection_sets)
-- Create resource_set_entries table (copy from selection_set_entries)
-- Create resource_set_children table (new)
-- Update sources table to unified schema
-- Migrate queries to sources
-- Create source_executions table
-- Update plans table schema
```

**Tasks**:
- [ ] Create migration file with up/down scripts
- [ ] Add migration runner to database initialization
- [ ] Test migration on copy of production data

#### 1.2 Update Database Package

**Files to modify**:
- `pkg/database/database.go` - Update schema creation
- `pkg/database/selection_sets.go` → `pkg/database/resource_sets.go`
- `pkg/database/queries.go` - Merge into sources

**New files**:
- `pkg/database/resource_sets.go` - ResourceSet CRUD
- `pkg/database/resource_set_nesting.go` - Nesting operations

**Tasks**:
- [ ] Rename `CreateSelectionSet` → `CreateResourceSet`
- [ ] Rename `GetSelectionSet` → `GetResourceSet`
- [ ] Rename `ListSelectionSets` → `ListResourceSets`
- [ ] Rename `DeleteSelectionSet` → `DeleteResourceSet`
- [ ] Rename `AddToSelectionSet` → `AddToResourceSet`
- [ ] Rename `RemoveFromSelectionSet` → `RemoveFromResourceSet`
- [ ] Rename `GetSelectionSetEntries` → `GetResourceSetEntries`
- [ ] Add `AddChildResourceSet(parentName, childName string) error`
- [ ] Add `RemoveChildResourceSet(parentName, childName string) error`
- [ ] Add `GetResourceSetChildren(name string) ([]*ResourceSet, error)`
- [ ] Add `GetResourceSetAllEntries(name string) ([]*Entry, error)` - flattened
- [ ] Add cycle detection for nesting

---

### Phase 2: Model Updates

**Objective**: Update Go types to reflect new architecture

#### 2.1 Rename SelectionSet Types

**File**: `internal/models/models.go`

**Tasks**:
- [ ] Rename `SelectionSet` → `ResourceSet`
- [ ] Rename `SelectionSetEntry` → `ResourceSetEntry`
- [ ] Add `ResourceSetChild` struct

```go
type ResourceSet struct {
    ID          int64   `db:"id" json:"id"`
    Name        string  `db:"name" json:"name"`
    Description *string `db:"description" json:"description,omitempty"`
    CreatedAt   int64   `db:"created_at" json:"created_at"`
    UpdatedAt   int64   `db:"updated_at" json:"updated_at"`
}

type ResourceSetChild struct {
    ParentID int64 `db:"parent_id" json:"parent_id"`
    ChildID  int64 `db:"child_id" json:"child_id"`
    AddedAt  int64 `db:"added_at" json:"added_at"`
}
```

#### 2.2 Update Source Types

**File**: `pkg/sources/source.go`

**Tasks**:
- [ ] Update `SourceType` constants:
  - `SourceTypeManual` → `SourceTypeFilesystemIndex`
  - `SourceTypeLive` → `SourceTypeFilesystemWatch`
  - Add `SourceTypeQuery`
  - Add `SourceTypeResourceSet`
- [ ] Add `SourceStatusCompleted`
- [ ] Add `TargetSetName` and `UpdateMode` to `SourceConfig`
- [ ] Add type-specific config structs

```go
const (
    SourceTypeFilesystemIndex SourceType = "filesystem.index"
    SourceTypeFilesystemWatch SourceType = "filesystem.watch"
    SourceTypeQuery           SourceType = "query"
    SourceTypeResourceSet     SourceType = "resource-set"
)

type SourceConfig struct {
    ID            int64        `json:"id"`
    Name          string       `json:"name"`
    Type          SourceType   `json:"type"`
    TargetSetName string       `json:"target_set_name"`  // NEW
    UpdateMode    string       `json:"update_mode"`       // NEW: replace, append, merge
    ConfigJSON    string       `json:"config_json"`
    Status        SourceStatus `json:"status"`
    Enabled       bool         `json:"enabled"`
    CreatedAt     time.Time    `json:"created_at"`
    UpdatedAt     time.Time    `json:"updated_at"`
    LastRunAt     *time.Time   `json:"last_run_at,omitempty"`  // NEW
    LastError     string       `json:"last_error,omitempty"`
}
```

#### 2.3 Update Plan Types

**File**: `internal/models/plan.go`

**Tasks**:
- [ ] Update `PlanSource` to reference sources by name
- [ ] Add `PlanResourceSet` struct for nesting definition
- [ ] Update JSON field names

---

### Phase 3: Source Unification

**Objective**: Merge query execution into unified source system

#### 3.1 Create Query Source Implementation

**File**: `pkg/sources/query.go`

**Tasks**:
- [ ] Implement `QuerySource` struct implementing `Source` interface
- [ ] Move `ExecuteFileFilter` logic from database package
- [ ] Add `Start()` that executes filter and populates target set
- [ ] Add execution history recording

```go
type QuerySource struct {
    config     *SourceConfig
    db         *database.DiskDB
    log        *logrus.Entry
    filter     *models.FileFilter
    targetSet  string
    updateMode string
}

func (s *QuerySource) Start(ctx context.Context) error {
    // 1. Parse filter from config
    // 2. Execute filter against entries
    // 3. Update target resource-set based on updateMode
    // 4. Record execution
    return nil
}
```

#### 3.2 Create ResourceSet Source Implementation

**File**: `pkg/sources/resource_set_source.go`

**Tasks**:
- [ ] Implement source that copies from one resource-set to another
- [ ] Support flatten option for nested sets
- [ ] Add execution history recording

#### 3.3 Update Source Manager

**File**: `pkg/sources/manager.go`

**Tasks**:
- [ ] Update `CreateSource` to handle all source types
- [ ] Add `ExecuteSource` for one-time execution (query, index, resource-set)
- [ ] Update factory pattern for source instantiation

```go
func (m *SourceManager) createSourceInstance(config *SourceConfig) (Source, error) {
    switch config.Type {
    case SourceTypeFilesystemIndex:
        return NewFilesystemIndexSource(config, m.db, m.log)
    case SourceTypeFilesystemWatch:
        return NewLiveFilesystemSource(config, m.db, m.log)
    case SourceTypeQuery:
        return NewQuerySource(config, m.db, m.log)
    case SourceTypeResourceSet:
        return NewResourceSetSource(config, m.db, m.log)
    default:
        return nil, fmt.Errorf("unknown source type: %s", config.Type)
    }
}
```

#### 3.4 Rename Existing Source Files

**Tasks**:
- [ ] Rename `pkg/sources/live.go` → `pkg/sources/filesystem_watch.go`
- [ ] Create `pkg/sources/filesystem_index.go` from manual source logic
- [ ] Update imports and references

---

### Phase 4: MCP Tool Updates

**Objective**: Update all MCP tools to use new naming and functionality

#### 4.1 Rename Selection-Set Tools

**File**: `pkg/server/mcp_tools.go`

**Tasks**:
- [ ] Rename `selection-set-create` → `resource-set-create`
- [ ] Rename `selection-set-list` → `resource-set-list`
- [ ] Rename `selection-set-get` → `resource-set-get`
- [ ] Rename `selection-set-modify` → `resource-set-modify`
- [ ] Rename `selection-set-delete` → `resource-set-delete`

#### 4.2 Add Nesting Tools

**File**: `pkg/server/mcp_tools.go`

**Tasks**:
- [ ] Add `resource-set-add-child` tool
- [ ] Add `resource-set-remove-child` tool
- [ ] Add `resource-set-get-all` tool (flattened entries)
- [ ] Add `resource-set-get-tree` tool (hierarchical view)

#### 4.3 Update Source Tools

**File**: `pkg/server/mcp_source_tools.go`

**Tasks**:
- [ ] Update `source-create` to accept all source types
- [ ] Add `source-execute` for one-time execution
- [ ] Update `source-list` with type filtering
- [ ] Remove query-specific tools (redirect to source-*)

#### 4.4 Deprecate Query Tools

**Tasks**:
- [ ] Add deprecation warning to `query-*` tools
- [ ] Redirect to equivalent `source-*` tools
- [ ] Document migration path

---

### Phase 5: Plan System Updates

**Objective**: Update plans to work with new resource-set and source abstractions

#### 5.1 Update Plan Executor

**File**: `pkg/plans/executor.go`

**Tasks**:
- [ ] Update source resolution to use unified Source abstraction
- [ ] Add support for resource-set nesting in outcomes
- [ ] Update condition evaluation to work with ResourceSet

#### 5.2 Update Plan Sources

**File**: `pkg/plans/executor.go`

**Tasks**:
- [ ] Change `PlanSource` to reference Source by name
- [ ] Add dependency resolution between sources
- [ ] Update execution flow for source orchestration

```go
func (e *Executor) executeSources(plan *models.Plan) error {
    // Build dependency graph
    // Execute sources in dependency order
    // Handle failures and partial execution
}
```

---

### Phase 6: CLI Updates

**Objective**: Update CLI commands to reflect new terminology

#### 6.1 Update Commands

**File**: `cmd/mcp-space-browser/main.go` and related

**Tasks**:
- [ ] Rename `selection-set` subcommand → `resource-set`
- [ ] Add `resource-set add-child` command
- [ ] Add `resource-set remove-child` command
- [ ] Update `source` commands for unified handling
- [ ] Add backward-compatible aliases

---

### Phase 7: Testing

**Objective**: Comprehensive test coverage for all changes

#### 7.1 Database Tests

**File**: `pkg/database/resource_sets_test.go`

**Tasks**:
- [ ] Test ResourceSet CRUD
- [ ] Test nesting operations
- [ ] Test cycle detection
- [ ] Test flattened entry retrieval
- [ ] Test migration from selection_sets

#### 7.2 Source Tests

**File**: `pkg/sources/query_test.go`, etc.

**Tasks**:
- [ ] Test QuerySource execution
- [ ] Test ResourceSetSource execution
- [ ] Test source manager with all types
- [ ] Test execution history recording

#### 7.3 MCP Tool Tests

**File**: `pkg/server/mcp_tools_test.go`

**Tasks**:
- [ ] Test renamed tools
- [ ] Test new nesting tools
- [ ] Test unified source tools
- [ ] Test deprecation warnings

#### 7.4 Integration Tests

**Tasks**:
- [ ] Test full workflow: create sets → create sources → execute plan
- [ ] Test nested resource-set flattening
- [ ] Test source dependencies in plans

---

### Phase 8: Documentation

**Objective**: Update all documentation

#### 8.1 Update Existing Docs

**Tasks**:
- [ ] Update `CLAUDE.md`
- [ ] Update `README.go.md`
- [ ] Update `MODULE_ARCHITECTURE.md`
- [ ] Update `docs/PLANS_ARCHITECTURE.md`

#### 8.2 Create Migration Guide

**File**: `docs/RESOURCE_SET_MIGRATION_GUIDE.md`

**Tasks**:
- [ ] Document API changes
- [ ] Provide migration examples
- [ ] List breaking changes

---

## Implementation Order

```
Phase 1: Database Schema        [~2 days]
    ↓
Phase 2: Model Updates          [~1 day]
    ↓
Phase 3: Source Unification     [~3 days]
    ↓
Phase 4: MCP Tool Updates       [~2 days]
    ↓
Phase 5: Plan System Updates    [~2 days]
    ↓
Phase 6: CLI Updates            [~1 day]
    ↓
Phase 7: Testing                [~2 days]
    ↓
Phase 8: Documentation          [~1 day]
```

## File Change Summary

### New Files
- `pkg/database/migrations/002_resource_sets.sql`
- `pkg/database/resource_sets.go`
- `pkg/database/resource_set_nesting.go`
- `pkg/sources/query.go`
- `pkg/sources/resource_set_source.go`
- `pkg/sources/filesystem_index.go`
- `docs/RESOURCE_SET_ARCHITECTURE.md`
- `docs/RESOURCE_SET_IMPLEMENTATION_PLAN.md`
- `docs/RESOURCE_SET_MIGRATION_GUIDE.md`

### Renamed Files
- `pkg/database/selection_sets.go` → removed (merged into resource_sets.go)
- `pkg/sources/live.go` → `pkg/sources/filesystem_watch.go`

### Modified Files
- `pkg/database/database.go` - Schema updates
- `pkg/database/queries.go` - Deprecation, redirect to sources
- `internal/models/models.go` - Type renames
- `internal/models/plan.go` - Plan type updates
- `pkg/sources/source.go` - Type updates
- `pkg/sources/manager.go` - Unified handling
- `pkg/server/mcp_tools.go` - Tool renames and additions
- `pkg/server/mcp_source_tools.go` - Unified source tools
- `pkg/plans/executor.go` - Source reference updates
- `cmd/mcp-space-browser/main.go` - CLI updates
- `CLAUDE.md` - Documentation updates
- `README.go.md` - Documentation updates

## Breaking Changes

1. **MCP Tool Names**: `selection-set-*` → `resource-set-*`
2. **Query Tools**: `query-*` deprecated, use `source-*` instead
3. **Source Types**: `manual` → `filesystem.index`, `live` → `filesystem.watch`
4. **Database Schema**: New tables, renamed tables
5. **Plan Sources**: Now reference Source by name instead of inline definition

## Backward Compatibility

- Provide tool aliases for 2 releases
- Migration script for database
- Clear deprecation warnings
- Documentation of migration path

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Data loss during migration | Test migration on copy first, backup before migration |
| Breaking existing clients | Provide aliases, deprecation period |
| Cycle in resource-set nesting | Implement cycle detection in AddChild |
| Performance with deep nesting | Limit nesting depth, cache flattened results |
| Source execution failures | Robust error handling, partial execution support |

## Success Criteria

1. All existing tests pass with renamed types
2. New nesting tests pass
3. Migration script successfully converts test data
4. MCP tools work with new names
5. Backward-compatible aliases work
6. Documentation is complete and accurate
