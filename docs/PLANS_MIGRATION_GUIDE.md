# Plans Migration Guide

This document shows the transformation from the old bespoke selection-set approach to the new Plans-based architecture.

## Before vs. After Comparison

### Old Approach: Bespoke Selection Sets

```go
// Selection sets had embedded criteria
type SelectionSet struct {
    ID           int64
    Name         string
    Description  *string
    CriteriaType string  // "user_selected" or "tool_query"
    CriteriaJSON *string // Embedded filtering logic
    CreatedAt    int64
    UpdatedAt    int64
}

// Creating a selection set with embedded criteria
selectionSet := &SelectionSet{
    Name:         "large-videos",
    CriteriaType: "tool_query",
    CriteriaJSON: `{"tool": "disk-du", "params": {"minSize": 1073741824, "extensions": ["mp4"]}}`,
}
db.CreateSelectionSet(selectionSet)

// Problem: No execution mechanism!
// The criteria was stored but never evaluated automatically
```

**Issues:**
- ❌ Selection sets knew WHAT to filter but not HOW to execute
- ❌ Eligibility logic tightly coupled to storage
- ❌ No execution history or audit trail
- ❌ No way to reuse criteria across multiple sets
- ❌ CriteriaJSON was just documentation, not executable

### New Approach: Plans + Selection Sets

```go
// Plans define the logic
type Plan struct {
    ID             int64
    Name           string
    Mode           string  // "oneshot" or "continuous"
    Sources        []PlanSource     // WHERE to get files
    Conditions     *PlanCondition   // HOW to filter
    Outcomes       []PlanOutcome    // WHAT to do with matches
}

// Selection sets are pure storage
type SelectionSet struct {
    ID          int64
    Name        string
    Description *string
    // NO criteria fields!
}

// Creating a plan that populates a selection set
plan := &Plan{
    Name: "large-videos",
    Mode: "oneshot",
    Sources: []PlanSource{
        {Type: "filesystem", Paths: []string{"/home/videos"}},
    },
    Conditions: &PlanCondition{
        Type: "all",
        Conditions: []*PlanCondition{
            {MediaType: stringPtr("video")},
            {MinSize: int64Ptr(1073741824)},
        },
    },
    Outcomes: []PlanOutcome{
        {
            Type:             "selection_set",
            SelectionSetName: stringPtr("large-videos"),
            Operation:        stringPtr("replace"),
        },
    },
}

// Execute the plan
executor.Execute(plan)
// Result: Selection set is populated with matching files
```

**Benefits:**
- ✅ Clear separation: Plans (logic) vs. Selection Sets (storage)
- ✅ Plans are executable and reusable
- ✅ Full execution history and audit trail
- ✅ One plan can target multiple selection sets
- ✅ Plans can run multiple times with different results

---

## Migration Path

### Phase 1: Add Plans Infrastructure (Non-Breaking)

1. **Add new tables** (no changes to existing tables)
   ```sql
   CREATE TABLE plans (...);
   CREATE TABLE plan_executions (...);
   CREATE TABLE plan_outcome_records (...);
   ```

2. **Add new models** in `internal/models/plan.go`

3. **Add database methods** in `pkg/database/plans.go`

4. **Implement execution engine** in `pkg/plans/`

**Status:** Existing code continues to work unchanged

### Phase 2: Migrate Existing Selection Sets (Optional)

For selection sets with `criteria_type = 'tool_query'`:

```go
// Migration script: convert selection sets to plans
func MigrateSelectionSetToPlan(db *database.DiskDB, setName string) error {
    // 1. Get existing selection set
    selSet, err := db.GetSelectionSet(setName)
    if err != nil {
        return err
    }

    // 2. Parse criteria JSON
    var criteria SelectionCriteria
    if selSet.CriteriaJSON != nil {
        json.Unmarshal([]byte(*selSet.CriteriaJSON), &criteria)
    }

    // 3. Convert to plan
    plan := &models.Plan{
        Name:   setName + "-plan",
        Mode:   "oneshot",
        Status: "active",
        Sources: []models.PlanSource{
            // Derive from criteria.Tool and criteria.Params
        },
        Conditions: criteriaToConditions(criteria),
        Outcomes: []models.PlanOutcome{
            {
                Type:             "selection_set",
                SelectionSetName: &setName,
                Operation:        stringPtr("replace"),
            },
        },
    }

    // 4. Create plan
    return db.CreatePlan(plan)
}
```

**Result:** Selection set data preserved, but logic moved to plan

### Phase 3: Simplify Selection Sets Schema

```sql
-- Remove criteria fields from selection_sets
ALTER TABLE selection_sets DROP COLUMN criteria_type;
ALTER TABLE selection_sets DROP COLUMN criteria_json;
```

Update model:
```go
type SelectionSet struct {
    ID          int64
    Name        string
    Description *string
    CreatedAt   int64
    UpdatedAt   int64
    // REMOVED: CriteriaType, CriteriaJSON
}
```

**Impact:**
- Existing selection sets continue to work for manual curation
- Automated population now done via Plans

### Phase 4: Deprecate Rules System

The existing `rules` tables can be deprecated:

```sql
-- Mark rules tables as deprecated
-- Keep for backward compatibility but recommend plans

-- Migration: rules → plans
-- Similar structure, different execution model
```

**Comparison:**

| Feature | Rules | Plans |
|---------|-------|-------|
| Scope | Always tied to selection sets | Independent, reusable |
| Execution | No built-in executor | Full execution engine |
| Sources | Implicit (all entries) | Explicit (filesystem, sets, queries) |
| Audit trail | Basic (rule_executions) | Comprehensive (executions + outcomes) |
| Modes | One-shot only | One-shot + continuous |

---

## Code Examples: Old vs. New

### Example 1: Find Large Files

#### Old Way (Bespoke Selection Set)

```go
// Step 1: Create selection set with criteria
selSet := &models.SelectionSet{
    Name:         "large-files",
    Description:  stringPtr("Files larger than 1GB"),
    CriteriaType: "tool_query",
    CriteriaJSON: stringPtr(`{
        "tool": "disk-du",
        "params": {"minSize": 1073741824}
    }`),
}
db.CreateSelectionSet(selSet)

// Step 2: ???
// The criteria is stored but there's no execution mechanism!
// Users would have to manually run disk-du and add files
```

#### New Way (Plan)

```go
// Step 1: Create plan
minSize := int64(1073741824)
setName := "large-files"
operation := "replace"

plan := &models.Plan{
    Name:        "find-large-files",
    Description: stringPtr("Find files larger than 1GB"),
    Mode:        "oneshot",
    Status:      "active",
    Sources: []models.PlanSource{
        {
            Type:  "filesystem",
            Paths: []string{"/"},
        },
    },
    Conditions: &models.PlanCondition{
        Type:    "size",
        MinSize: &minSize,
    },
    Outcomes: []models.PlanOutcome{
        {
            Type:             "selection_set",
            SelectionSetName: &setName,
            Operation:        &operation,
        },
    },
}
db.CreatePlan(plan)

// Step 2: Execute plan
executor := plans.NewExecutor(db, logger)
execution, err := executor.Execute(plan)

// Result: Selection set "large-files" now contains all files > 1GB
// Execution record shows what was processed
```

### Example 2: Complex Filtering

#### Old Way (Not Possible)

```go
// Can't express complex AND/OR logic in CriteriaJSON
// Would need custom code for each use case
```

#### New Way (Nested Conditions)

```go
// Find: (videos > 1GB) OR (images > 100MB)
plan := &models.Plan{
    Name: "media-to-archive",
    Mode: "oneshot",
    Sources: []models.PlanSource{
        {Type: "filesystem", Paths: []string{"/media"}},
    },
    Conditions: &models.PlanCondition{
        Type: "any",
        Conditions: []*models.PlanCondition{
            {
                Type: "all",
                Conditions: []*models.PlanCondition{
                    {MediaType: stringPtr("video")},
                    {MinSize: int64Ptr(1073741824)},
                },
            },
            {
                Type: "all",
                Conditions: []*models.PlanCondition{
                    {MediaType: stringPtr("image")},
                    {MinSize: int64Ptr(104857600)},
                },
            },
        },
    },
    Outcomes: []models.PlanOutcome{
        {
            Type:             "selection_set",
            SelectionSetName: stringPtr("archive-queue"),
            Operation:        stringPtr("add"),
        },
    },
}
```

### Example 3: Multiple Outcomes

#### Old Way (Not Possible)

```go
// Selection sets could only represent ONE target
// Can't do "add to set A AND remove from set B"
```

#### New Way (Multiple Outcomes)

```go
// Find processed files and:
// 1. Add to "processed" set
// 2. Remove from "pending" set
plan := &models.Plan{
    Name: "mark-processed",
    Sources: []models.PlanSource{
        {
            Type:      "selection_set",
            SourceRef: stringPtr("pending"),
        },
    },
    Conditions: &models.PlanCondition{
        // Check if file has .processed marker
        PathSuffix: stringPtr(".processed"),
    },
    Outcomes: []models.PlanOutcome{
        {
            Type:             "selection_set",
            SelectionSetName: stringPtr("processed"),
            Operation:        stringPtr("add"),
        },
        {
            Type:             "selection_set",
            SelectionSetName: stringPtr("pending"),
            Operation:        stringPtr("remove"),
        },
    },
}
```

---

## MCP Tools Migration

### Old MCP Tools (Keep for Manual Curation)

```
selection-set-create    - Create empty set
selection-set-modify    - Manually add/remove paths
selection-set-get       - Get entries
selection-set-list      - List all sets
selection-set-delete    - Delete set
```

**Usage:** Continue to work for manual curation

### New MCP Tools (Automated Population)

```
plan-create             - Create a new plan
plan-execute            - Run a plan (populate selection sets)
plan-get                - Get plan definition + execution history
plan-list               - List all plans
plan-update             - Modify plan definition
plan-delete             - Delete plan
plan-stop               - Stop continuous plan (future)
```

**Example:**

```javascript
// Old way: Manual curation
await client.callTool("selection-set-create", {
    name: "favorites",
    criteriaType: "user_selected"
});
await client.callTool("selection-set-modify", {
    name: "favorites",
    operation: "add",
    paths: "/path/to/file1,/path/to/file2"
});

// New way: Automated via plan
await client.callTool("plan-create", {
    name: "auto-favorites",
    plan: JSON.stringify({
        mode: "oneshot",
        sources: [{type: "filesystem", paths: ["/media"]}],
        conditions: {
            type: "all",
            conditions: [
                {media_type: "video"},
                {min_size: 1073741824}
            ]
        },
        outcomes: [{
            type: "selection_set",
            selection_set_name: "favorites",
            operation: "replace"
        }]
    })
});
await client.callTool("plan-execute", {name: "auto-favorites"});
```

---

## API Endpoints Migration

### Old REST API (Keep)

```
GET  /api/selection-sets              - List all sets
GET  /api/selection-sets/:name        - Get set entries
POST /api/selection-sets              - Create set
PUT  /api/selection-sets/:name        - Modify set
DELETE /api/selection-sets/:name      - Delete set
```

### New REST API (Add)

```
GET  /api/plans                       - List all plans
GET  /api/plans/:name                 - Get plan definition
POST /api/plans                       - Create plan
PUT  /api/plans/:name                 - Update plan
DELETE /api/plans/:name               - Delete plan
POST /api/plans/:name/execute         - Execute plan
GET  /api/plans/:name/executions      - Get execution history
```

---

## Testing Strategy During Migration

### Phase 1: Parallel Testing

```go
func TestMigration_SelectionSetVsPlan(t *testing.T) {
    // Create selection set the old way (manual)
    db.CreateSelectionSet(&models.SelectionSet{
        Name: "manual-test",
    })
    db.AddToSelectionSet("manual-test", []string{"/file1", "/file2"})

    // Create plan that does the same thing
    plan := &models.Plan{
        Name: "auto-test",
        Sources: []models.PlanSource{
            {Type: "filesystem", Paths: []string{"/testdir"}},
        },
        Outcomes: []models.PlanOutcome{
            {
                Type:             "selection_set",
                SelectionSetName: stringPtr("auto-test"),
                Operation:        stringPtr("add"),
            },
        },
    }
    db.CreatePlan(plan)
    executor.Execute(plan)

    // Both methods should work
    manualEntries, _ := db.GetSelectionSetEntries("manual-test")
    autoEntries, _ := db.GetSelectionSetEntries("auto-test")

    assert.NotNil(t, manualEntries)
    assert.NotNil(t, autoEntries)
}
```

### Phase 2: Deprecation Warnings

```go
func (d *DiskDB) CreateSelectionSet(set *models.SelectionSet) error {
    if set.CriteriaType == "tool_query" {
        d.logger.Warn("Creating selection sets with criteria_type='tool_query' is deprecated. Use Plans instead.")
    }
    // Continue with creation...
}
```

---

## Rollback Plan

If issues arise during migration:

### Step 1: Keep Old Code Intact

Plans are additive - they don't modify existing selection set functionality.

### Step 2: Feature Flag

```go
const USE_PLANS = os.Getenv("USE_PLANS") == "true"

if USE_PLANS {
    // New plan-based workflow
    executor.Execute(plan)
} else {
    // Old manual workflow
    db.AddToSelectionSet(name, paths)
}
```

### Step 3: Database Rollback

```sql
-- If needed, drop plan tables
DROP TABLE IF EXISTS plan_outcome_records;
DROP TABLE IF EXISTS plan_executions;
DROP TABLE IF EXISTS plans;

-- Selection sets table remains unchanged
-- No data loss
```

---

## Timeline Recommendation

### Week 1-2: Foundation
- ✅ Implement Plan models and database layer
- ✅ Write unit tests for models
- ✅ Add plan tables to schema

### Week 3-4: Execution Engine
- ✅ Implement PlanExecutor, ConditionEvaluator, OutcomeApplier
- ✅ Write integration tests
- ✅ Test with real filesystem data

### Week 5: Integration
- ✅ Add MCP tools for Plans
- ✅ Add REST API endpoints
- ✅ Update CLI commands

### Week 6: Migration
- ✅ Create migration scripts for existing selection sets
- ✅ Update documentation
- ✅ Run migration in staging environment

### Week 7: Cleanup (Optional)
- ✅ Remove `criteria_type` and `criteria_json` from selection_sets table
- ✅ Mark rules system as deprecated
- ✅ Update all references

---

## Conclusion

The migration from bespoke selection sets to Plans provides:

1. **Separation of Concerns**: Logic (Plans) vs. Storage (Selection Sets)
2. **Executability**: Plans are not just documentation - they DO things
3. **Reusability**: One plan can run multiple times, target multiple sets
4. **Auditability**: Full execution history and outcome tracking
5. **Extensibility**: Easy to add new source types, conditions, and outcomes
6. **Backward Compatibility**: Existing selection set operations continue to work

The migration is **non-breaking** and can be done incrementally, with the option to rollback at any point.
