# Plans Architecture Design

## Executive Summary

This document outlines the architectural refactoring to introduce **Plans** as a first-class concept, decoupling eligibility logic from Resource Sets. Plans define what to process, how to process it, and what outcomes to produce, while Resource Sets become pure data containers.

## Core Principles

1. **Separation of Concerns**: Plans define logic; Resource Sets store results
2. **Reusability**: Plans can target multiple resource sets and run multiple times
3. **Extensibility**: Plans support future long-running modes with filesystem notifications
4. **Backward Compatibility**: Resource Sets work standalone without Plans
5. **Clean Boundaries**: Plan Definition → Plan Executor → Outcomes → Resource Sets

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│                         PLAN LAYER                            │
│  ┌────────────┐   ┌──────────────┐   ┌──────────────┐       │
│  │   Plan     │──>│ Plan Executor│──>│   Outcomes   │       │
│  │ Definition │   │   Engine     │   │   (Actions)  │       │
│  └────────────┘   └──────────────┘   └──────────────┘       │
│         │                 │                    │              │
│         │                 │                    ▼              │
│         │                 │           ┌─────────────────┐    │
│         │                 │           │ Resource Sets  │    │
│         │                 │           │   Add/Remove    │    │
│         │                 │           └─────────────────┘    │
│         │                 │                                   │
│         │                 ▼                                   │
│         │        ┌──────────────────┐                        │
│         └───────>│  Filesystem      │                        │
│                  │  Source Specs    │                        │
│                  └──────────────────┘                        │
└──────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                    SELECTION SET LAYER                        │
│  ┌────────────────────────────────────────────────┐          │
│  │          Resource Sets (Pure Storage)         │          │
│  │  - No eligibility logic                        │          │
│  │  - Manual add/remove still supported           │          │
│  │  - Can be targets of Plan outcomes             │          │
│  └────────────────────────────────────────────────┘          │
└──────────────────────────────────────────────────────────────┘
```

---

## Data Models

### 1. Plan Definition

```go
// Plan defines what to process, how to filter, and what outcomes to produce
type Plan struct {
    ID          int64     `db:"id" json:"id"`
    Name        string    `db:"name" json:"name"`                    // Unique plan name
    Description *string   `db:"description" json:"description"`

    // Execution mode
    Mode        string    `db:"mode" json:"mode"`                    // "oneshot", "continuous"
    Status      string    `db:"status" json:"status"`                // "active", "paused", "disabled"

    // Source configuration
    SourcesJSON string    `db:"sources_json" json:"-"`               // JSON array of PlanSource

    // Filtering/eligibility logic (reuses RuleCondition)
    ConditionsJSON *string `db:"conditions_json" json:"-"`           // JSON of RuleCondition (tree)

    // What to do with matching entries (reuses RuleOutcome)
    OutcomesJSON   string `db:"outcomes_json" json:"-"`              // JSON array of RuleOutcome

    // Metadata
    CreatedAt   int64     `db:"created_at" json:"created_at"`
    UpdatedAt   int64     `db:"updated_at" json:"updated_at"`
    LastRunAt   *int64    `db:"last_run_at" json:"last_run_at"`

    // Parsed fields (not stored in DB)
    Sources     []PlanSource     `db:"-" json:"sources"`
    Conditions  *RuleCondition   `db:"-" json:"conditions"`    // Reuses RuleCondition
    Outcomes    []RuleOutcome    `db:"-" json:"outcomes"`      // Reuses RuleOutcome
}
```

### 2. Plan Source (Filesystem Specification)

```go
// PlanSource defines where to get files and what metadata to generate
type PlanSource struct {
    // Filesystem specification
    Type      string   `json:"type"`           // "filesystem", "selection_set", "query"
    Paths     []string `json:"paths"`          // Root paths to scan (for filesystem)

    // Source-specific parameters
    SourceRef *string  `json:"source_ref"`     // Reference to selection_set or query name

    // Characteristic generators (metadata to compute)
    Characteristics []CharacteristicGenerator `json:"characteristics"`

    // Scan options
    FollowSymlinks  bool  `json:"follow_symlinks"`
    MaxDepth        *int  `json:"max_depth"`
    IncludeHidden   bool  `json:"include_hidden"`
}

// CharacteristicGenerator specifies metadata/analysis to perform
type CharacteristicGenerator struct {
    Type   string                 `json:"type"`    // "media_type", "thumbnail", "exif", "hash"
    Params map[string]interface{} `json:"params"`  // Type-specific configuration
}
```

### 3. Plan Condition (Reuses RuleCondition)

Plans use the existing `RuleCondition` type from the rules system:

```go
// RuleCondition represents the condition for a rule (from internal/models/models.go)
type RuleCondition struct {
    Type       string           `json:"type"` // "all", "any", "none", "media_type", "size", "time", "path"
    Conditions []*RuleCondition `json:"conditions,omitempty"` // For composite conditions

    // Media type condition
    MediaType *string `json:"mediaType,omitempty"` // "image", "video", "audio", "document"

    // Size condition
    MinSize *int64 `json:"minSize,omitempty"` // Bytes
    MaxSize *int64 `json:"maxSize,omitempty"` // Bytes

    // Time condition
    MinMtime *int64 `json:"minMtime,omitempty"` // Unix timestamp
    MaxMtime *int64 `json:"maxMtime,omitempty"` // Unix timestamp
    MinCtime *int64 `json:"minCtime,omitempty"` // Unix timestamp
    MaxCtime *int64 `json:"maxCtime,omitempty"` // Unix timestamp

    // Path condition
    PathContains   *string `json:"pathContains,omitempty"`
    PathPrefix     *string `json:"pathPrefix,omitempty"`
    PathSuffix     *string `json:"pathSuffix,omitempty"`
    PathPattern    *string `json:"pathPattern,omitempty"` // Regex
}
```

**Note:** Plans reuse the existing condition logic - no duplication.

### 4. Plan Outcome (Reuses RuleOutcome)

Plans use the existing `RuleOutcome` type from the rules system:

```go
// RuleOutcome represents the outcome of a rule (from internal/models/models.go)
// IMPORTANT: All outcomes must have a ResourceSetName to ensure traceability
type RuleOutcome struct {
    Type             string         `json:"type"` // "selection_set", "classifier", "chained"
    ResourceSetName string         `json:"selectionSetName"` // REQUIRED for all outcome types

    // For selection_set outcome
    Operation *string `json:"operation,omitempty"` // "add", "remove"

    // For classifier outcome
    ClassifierOperation *string `json:"classifierOperation,omitempty"` // "generate_thumbnail", "extract_metadata"
    MaxWidth            *int    `json:"maxWidth,omitempty"`
    MaxHeight           *int    `json:"maxHeight,omitempty"`
    Quality             *int    `json:"quality,omitempty"`

    // For chained outcome
    Outcomes     []*RuleOutcome `json:"outcomes,omitempty"`
    StopOnError  *bool          `json:"stopOnError,omitempty"`
}
```

**Note:** Plans reuse the existing outcome logic - no duplication.

### 5. Plan Execution Record

```go
// PlanExecution tracks individual plan runs
type PlanExecution struct {
    ID               int64   `db:"id" json:"id"`
    PlanID           int64   `db:"plan_id" json:"plan_id"`
    PlanName         string  `db:"plan_name" json:"plan_name"`

    // Execution metadata
    StartedAt        int64   `db:"started_at" json:"started_at"`
    CompletedAt      *int64  `db:"completed_at" json:"completed_at"`
    DurationMs       *int    `db:"duration_ms" json:"duration_ms"`

    // Results
    EntriesProcessed int     `db:"entries_processed" json:"entries_processed"`
    EntriesMatched   int     `db:"entries_matched" json:"entries_matched"`
    OutcomesApplied  int     `db:"outcomes_applied" json:"outcomes_applied"`

    Status           string  `db:"status" json:"status"`        // "running", "success", "partial", "error"
    ErrorMessage     *string `db:"error_message" json:"error_message"`
}
```

### 6. Plan Outcome Record (Audit Trail)

```go
// PlanOutcomeRecord tracks individual outcome applications
type PlanOutcomeRecord struct {
    ID            int64   `db:"id" json:"id"`
    ExecutionID   int64   `db:"execution_id" json:"execution_id"`
    PlanID        int64   `db:"plan_id" json:"plan_id"`

    EntryPath     string  `db:"entry_path" json:"entry_path"`
    OutcomeType   string  `db:"outcome_type" json:"outcome_type"`   // "selection_set", "classifier"
    OutcomeData   string  `db:"outcome_data" json:"outcome_data"`   // JSON details

    Status        string  `db:"status" json:"status"`               // "success", "error"
    ErrorMessage  *string `db:"error_message" json:"error_message"`
    CreatedAt     int64   `db:"created_at" json:"created_at"`
}
```

### 7. Simplified Resource Set (Pure Item Storage)

```go
// ResourceSet is a pure data container - just stores items
// It doesn't care HOW items got selected, only WHAT items are in it and WHEN they were added
type ResourceSet struct {
    ID          int64   `db:"id" json:"id"`
    Name        string  `db:"name" json:"name"`
    Description *string `db:"description" json:"description"`
    CreatedAt   int64   `db:"created_at" json:"created_at"`
    UpdatedAt   int64   `db:"updated_at" json:"updated_at"`

    // REMOVED: All criteria/logic fields (CriteriaType, CriteriaJSON)
    // Plans handle eligibility logic; resource sets just store results
}

// ResourceSetEntry represents a single item in a resource set
// This tracks WHEN each item was added (but not HOW or WHY)
type ResourceSetEntry struct {
    SetID     int64  `db:"set_id" json:"set_id"`
    EntryPath string `db:"entry_path" json:"entry_path"`
    AddedAt   int64  `db:"added_at" json:"added_at"`     // Timestamp when added
}
```

**Key Principle:** Selection sets are dumb storage. They know:
- What items are in the set (paths)
- When each item was added (timestamp)
- That's it. No logic about WHY items are there.

---

## Database Schema

### New Tables

```sql
-- Plans table
CREATE TABLE plans (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    mode TEXT CHECK(mode IN ('oneshot', 'continuous')) DEFAULT 'oneshot',
    status TEXT CHECK(status IN ('active', 'paused', 'disabled')) DEFAULT 'active',

    sources_json TEXT NOT NULL,         -- JSON array of PlanSource
    conditions_json TEXT,               -- JSON of PlanCondition tree
    outcomes_json TEXT NOT NULL,        -- JSON array of PlanOutcome

    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER
);

CREATE INDEX idx_plans_status ON plans(status);
CREATE INDEX idx_plans_mode ON plans(mode);

-- Plan execution tracking
CREATE TABLE plan_executions (
    id INTEGER PRIMARY KEY,
    plan_id INTEGER NOT NULL,
    plan_name TEXT NOT NULL,

    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    duration_ms INTEGER,

    entries_processed INTEGER DEFAULT 0,
    entries_matched INTEGER DEFAULT 0,
    outcomes_applied INTEGER DEFAULT 0,

    status TEXT CHECK(status IN ('running', 'success', 'partial', 'error')) DEFAULT 'running',
    error_message TEXT,

    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE
);

CREATE INDEX idx_plan_executions_plan ON plan_executions(plan_id);
CREATE INDEX idx_plan_executions_status ON plan_executions(status);

-- Plan outcome audit trail
CREATE TABLE plan_outcome_records (
    id INTEGER PRIMARY KEY,
    execution_id INTEGER NOT NULL,
    plan_id INTEGER NOT NULL,

    entry_path TEXT NOT NULL,
    outcome_type TEXT NOT NULL,         -- "selection_set", "classifier", etc.
    outcome_data TEXT,                  -- JSON details

    status TEXT CHECK(status IN ('success', 'error')) DEFAULT 'success',
    error_message TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),

    FOREIGN KEY (execution_id) REFERENCES plan_executions(id) ON DELETE CASCADE,
    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE,
    FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
);

CREATE INDEX idx_plan_outcomes_execution ON plan_outcome_records(execution_id);
CREATE INDEX idx_plan_outcomes_plan ON plan_outcome_records(plan_id);
CREATE INDEX idx_plan_outcomes_entry ON plan_outcome_records(entry_path);
```

### Modified Tables

```sql
-- Selection sets table (SIMPLIFIED - pure item storage)
-- Only cares about WHAT items and WHEN added, not HOW they got there
CREATE TABLE resource_sets (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
    -- REMOVED: criteria_type, criteria_json (logic moved to Plans)
);

-- resource_set_entries: The join table that tracks items and when they were added
-- This is the only place that knows "what's in the set" and "when it was added"
CREATE TABLE resource_set_entries (
    set_id INTEGER NOT NULL,
    entry_path TEXT NOT NULL,
    added_at INTEGER DEFAULT (strftime('%s', 'now')),  -- Tracks WHEN item was added
    PRIMARY KEY (set_id, entry_path),
    FOREIGN KEY (set_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
);
CREATE INDEX idx_set_entries ON resource_set_entries(set_id);
```

**Migration Note:** To migrate existing resource_sets:
```sql
-- Drop the criteria columns (one-way migration)
ALTER TABLE resource_sets DROP COLUMN criteria_type;
ALTER TABLE resource_sets DROP COLUMN criteria_json;
```

---

## Component Architecture

### 1. Plan Manager (`pkg/plans/manager.go`)

```go
type PlanManager struct {
    db       *database.DiskDB
    executor *PlanExecutor
    logger   *logrus.Entry
}

// CRUD operations
func (pm *PlanManager) CreatePlan(plan *models.Plan) error
func (pm *PlanManager) GetPlan(name string) (*models.Plan, error)
func (pm *PlanManager) ListPlans() ([]*models.Plan, error)
func (pm *PlanManager) UpdatePlan(plan *models.Plan) error
func (pm *PlanManager) DeletePlan(name string) error

// Execution
func (pm *PlanManager) ExecutePlan(name string) (*models.PlanExecution, error)
func (pm *PlanManager) StopPlan(name string) error  // For continuous plans

// Status
func (pm *PlanManager) GetPlanExecutions(name string, limit int) ([]*models.PlanExecution, error)
```

### 2. Plan Executor (`pkg/plans/executor.go`)

```go
type PlanExecutor struct {
    db     *database.DiskDB
    logger *logrus.Entry
}

// Core execution flow
func (pe *PlanExecutor) Execute(plan *models.Plan) (*models.PlanExecution, error) {
    // 1. Create execution record
    // 2. Resolve sources (get entry list)
    // 3. Evaluate conditions (filter entries)
    // 4. Apply outcomes (modify resource sets, trigger classifiers)
    // 5. Record results
}

// Source resolution
func (pe *PlanExecutor) resolveSources(sources []PlanSource) ([]string, error)
func (pe *PlanExecutor) resolveFilesystemSource(src PlanSource) ([]string, error)
func (pe *PlanExecutor) resolveResourceSetSource(src PlanSource) ([]string, error)

// Condition evaluation (reuses RuleCondition)
func (pe *PlanExecutor) evaluateConditions(entries []*models.Entry, cond *models.RuleCondition) ([]*models.Entry, error)
func (pe *PlanExecutor) matchCondition(entry *models.Entry, cond *models.RuleCondition) (bool, error)

// Outcome application (reuses RuleOutcome)
func (pe *PlanExecutor) applyOutcomes(entries []*models.Entry, outcomes []models.RuleOutcome, execID int64) error
func (pe *PlanExecutor) applyResourceSetOutcome(entries []*models.Entry, outcome models.RuleOutcome, execID int64) error
```

### 3. Condition Evaluator (`pkg/plans/evaluator.go`)

**Note:** This component evaluates `RuleCondition` (reused from existing rules system).

```go
type ConditionEvaluator struct {
    logger *logrus.Entry
}

// Tree-based condition evaluation (uses RuleCondition)
func (ce *ConditionEvaluator) Evaluate(entry *models.Entry, condition *models.RuleCondition) (bool, error) {
    switch condition.Type {
    case "all":
        return ce.evaluateAll(entry, condition.Conditions)
    case "any":
        return ce.evaluateAny(entry, condition.Conditions)
    case "none":
        return ce.evaluateNone(entry, condition.Conditions)
    default:
        return ce.evaluateLeaf(entry, condition)
    }
}

// Leaf condition evaluation
func (ce *ConditionEvaluator) evaluateLeaf(entry *models.Entry, condition *models.RuleCondition) (bool, error) {
    // Check MediaType, MinSize/MaxSize, MinMtime/MaxMtime, path filters, etc.
    // Uses fields from RuleCondition (not duplicated)
}
```

### 4. Outcome Applier (`pkg/plans/outcomes.go`)

**Note:** This component applies `RuleOutcome` (reused from existing rules system).

```go
type OutcomeApplier struct {
    db     *database.DiskDB
    logger *logrus.Entry
}

func (oa *OutcomeApplier) Apply(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) error {
    switch outcome.Type {
    case "selection_set":
        return oa.applyResourceSet(entries, outcome, execID, planID)
    case "classifier":
        return oa.applyClassifier(entries, outcome, execID, planID)
    case "chained":
        return oa.applyChained(entries, outcome, execID, planID)
    default:
        return fmt.Errorf("unknown outcome type: %s", outcome.Type)
    }
}

func (oa *OutcomeApplier) applyResourceSet(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) error {
    paths := extractPaths(entries)

    // RuleOutcome.ResourceSetName is always required (string, not *string)
    setName := outcome.ResourceSetName

    switch *outcome.Operation {
    case "add":
        return oa.db.AddToResourceSet(setName, paths)
    case "remove":
        return oa.db.RemoveFromResourceSet(setName, paths)
    }

    // Record outcome for audit trail
    oa.recordOutcomes(entries, outcome, execID, planID)
}
```

---

## Execution Flow

### One-Shot Plan Execution

```
1. User triggers plan execution
   └─> PlanManager.ExecutePlan("my-plan")

2. Load plan definition
   └─> db.GetPlan("my-plan") → Plan object
   └─> Parse JSON fields (sources, conditions, outcomes)

3. Create execution record
   └─> db.CreatePlanExecution(planID) → executionID

4. Resolve sources (get entry candidates)
   ├─> For each PlanSource:
   │   ├─> filesystem: db.GetEntriesByPath(paths)
   │   ├─> selection_set: db.GetResourceSetEntries(name)
   │   └─> query: db.ExecuteQuery(name)
   └─> Combine into []Entry

5. Evaluate conditions (filter entries)
   ├─> For each entry:
   │   └─> ConditionEvaluator.Evaluate(entry, plan.Conditions)
   └─> Filter to matched entries

6. Apply outcomes
   ├─> For each PlanOutcome:
   │   ├─> OutcomeApplier.Apply(matchedEntries, outcome, execID)
   │   └─> Record PlanOutcomeRecord for each entry
   └─> Update execution stats

7. Complete execution
   └─> db.CompletePlanExecution(executionID, stats)
```

### Continuous Plan (Future)

```
1. PlanManager starts continuous plan
   └─> Creates inotify watcher for source paths

2. On filesystem event:
   ├─> Scan changed path
   ├─> Update entries in DB
   └─> Trigger plan evaluation for new/modified entries

3. Plan remains active until stopped
   └─> PlanManager.StopPlan("my-plan")
```

---

## Backward Compatibility

### Resource Sets Continue to Work Standalone

```go
// Direct manipulation (no plan required)
db.CreateResourceSet(&models.ResourceSet{
    Name: "my-favorites",
    Description: "Manually curated files",
})

db.AddToResourceSet("my-favorites", []string{
    "/path/to/file1.mp4",
    "/path/to/file2.jpg",
})

// MCP tools continue to work
// resource-set-create, resource-set-modify, etc.
```

### Migration Path for Existing Code

1. **Phase 1: Add Plans infrastructure**
   - Create new tables (plans, plan_executions, plan_outcome_records)
   - Implement Plan models and database methods
   - Build PlanExecutor, ConditionEvaluator, OutcomeApplier

2. **Phase 2: Simplify Resource Sets**
   - Remove `criteria_type` and `criteria_json` from resource_sets table
   - Update ResourceSet model (remove fields)
   - Update tests

3. **Phase 3: Deprecate Rules system**
   - Map existing rules to Plans
   - Create migration script: `rules → plans`
   - Mark rules tables as deprecated (but keep for backward compat)

4. **Phase 4: Add MCP tools for Plans**
   - `plan-create`: Create new plan
   - `plan-execute`: Run a plan
   - `plan-list`: List all plans
   - `plan-get`: Get plan definition and execution history
   - `plan-update`: Modify plan
   - `plan-delete`: Remove plan

---

## Example Use Cases

### Example 1: Find Large Video Files

```json
{
  "name": "large-videos",
  "description": "Find video files larger than 1GB",
  "mode": "oneshot",
  "sources": [
    {
      "type": "filesystem",
      "paths": ["/home/user/Videos"],
      "characteristics": [
        {"type": "media_type"}
      ]
    }
  ],
  "conditions": {
    "type": "all",
    "conditions": [
      {
        "type": "media_type",
        "media_type": "video"
      },
      {
        "type": "size",
        "min_size": 1073741824
      }
    ]
  },
  "outcomes": [
    {
      "type": "selection_set",
      "selection_set_name": "large-videos",
      "operation": "replace"
    }
  ]
}
```

### Example 2: Recent Documents for Backup

```json
{
  "name": "recent-docs",
  "description": "Documents modified in last 7 days",
  "mode": "oneshot",
  "sources": [
    {
      "type": "filesystem",
      "paths": ["/home/user/Documents"]
    }
  ],
  "conditions": {
    "type": "all",
    "conditions": [
      {
        "type": "extensions",
        "extensions": ["pdf", "docx", "xlsx", "txt"]
      },
      {
        "type": "time",
        "min_mtime": 1732320000
      }
    ]
  },
  "outcomes": [
    {
      "type": "selection_set",
      "selection_set_name": "backup-queue",
      "operation": "add"
    }
  ]
}
```

### Example 3: Continuous Photo Organizer (Future)

```json
{
  "name": "organize-photos",
  "description": "Automatically organize new photos",
  "mode": "continuous",
  "sources": [
    {
      "type": "filesystem",
      "paths": ["/home/user/Pictures/Inbox"],
      "characteristics": [
        {"type": "exif"}
      ]
    }
  ],
  "conditions": {
    "type": "media_type",
    "media_type": "image"
  },
  "outcomes": [
    {
      "type": "selection_set",
      "selection_set_name": "unprocessed-photos",
      "operation": "add"
    },
    {
      "type": "classifier",
      "classifier_type": "thumbnail",
      "classifier_params": {"size": 256}
    }
  ]
}
```

---

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Create `internal/models/plan.go` with Plan-related structs
- [ ] Add plan tables to `pkg/database/schema.go`
- [ ] Implement `pkg/database/plans.go` (CRUD operations)
- [ ] Write tests for database layer

### Phase 2: Execution Engine
- [ ] Implement `pkg/plans/manager.go`
- [ ] Implement `pkg/plans/executor.go`
- [ ] Implement `pkg/plans/evaluator.go` (condition matching)
- [ ] Implement `pkg/plans/outcomes.go` (outcome application)
- [ ] Write tests for execution engine

### Phase 3: Integration
- [ ] Add MCP tools for Plans
- [ ] Update CLI commands (`plan create`, `plan run`)
- [ ] Add REST API endpoints (`/api/plans`)
- [ ] Update documentation

### Phase 4: Migration & Cleanup
- [ ] Create migration script (rules → plans)
- [ ] Remove `criteria_type`/`criteria_json` from resource_sets
- [ ] Update existing tests
- [ ] Mark rules system as deprecated

### Phase 5: Future Enhancements
- [ ] Implement continuous plan mode
- [ ] Add filesystem notify (inotify/FSEvents)
- [ ] Add more outcome types (export, delete)
- [ ] Add characteristic generators (thumbnails, EXIF)

---

## Design Decisions & Rationale

### Why Plans are Separate from Resource Sets

**Before**: Selection sets had embedded eligibility logic (`criteria_type`, `criteria_json`), tightly coupling storage and logic.

**After**: Plans are independent entities that can:
- Target multiple resource sets
- Be reused and modified without affecting stored data
- Support complex execution modes (one-shot, continuous)
- Be version-controlled and shared

### Why JSON for Plan Configuration

- **Flexibility**: Supports arbitrary nesting of conditions
- **Extensibility**: Easy to add new fields without schema changes
- **Portability**: Plans can be exported/imported as JSON
- **MCP Compatibility**: JSON aligns with MCP tool parameter formats

### Why Keep Execution Records

- **Audit Trail**: Track what was processed and when
- **Debugging**: Diagnose why entries were/weren't matched
- **Analytics**: Understand plan effectiveness over time
- **Idempotency**: Avoid re-processing unchanged entries (future optimization)

### Why Separate Outcome Types

Different outcome types have different execution semantics:
- **selection_set**: Immediate database write
- **classifier**: Trigger background job (future)
- **export**: Write to filesystem
- **delete**: Requires confirmation/safety checks

Keeping them separate allows for:
- Type-specific validation
- Different error handling strategies
- Future extensibility

---

## Migration Guide

### For Users

1. **Existing resource sets continue to work** - no changes required
2. **New workflow**: Instead of creating resource sets with criteria, create a Plan
3. **Run plans** using `plan-execute` MCP tool or `disk-plan-run` CLI command

### For Developers

1. **Use PlanManager for automated workflows** instead of directly manipulating resource sets
2. **Deprecate RuleCondition/RuleOutcome** in favor of PlanCondition/PlanOutcome
3. **New imports**:
   ```go
   import "github.com/prismon/mcp-space-browser/pkg/plans"
   ```

---

## Conclusion

This architecture provides:
- ✅ **Clean separation** between logic (Plans) and storage (Resource Sets)
- ✅ **Backward compatibility** for existing code
- ✅ **Extensibility** for future features (continuous mode, notifications)
- ✅ **Testability** through clear component boundaries
- ✅ **Auditability** via execution and outcome records
- ✅ **Reusability** of plan definitions across multiple runs

The design follows Go best practices, maintains consistency with the existing codebase, and provides a clear migration path.
