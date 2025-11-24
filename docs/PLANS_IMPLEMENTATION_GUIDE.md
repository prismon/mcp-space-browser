# Plans Implementation Guide

This guide provides practical implementation examples for the Plans architecture.

## Table of Contents

1. [Data Model Implementation](#data-model-implementation)
2. [Database Layer](#database-layer)
3. [Plan Executor](#plan-executor)
4. [Condition Evaluator](#condition-evaluator)
5. [Outcome Applier](#outcome-applier)
6. [Testing Strategy](#testing-strategy)

---

## Data Model Implementation

### File: `internal/models/plan.go`

```go
package models

import (
    "encoding/json"
    "fmt"
    "time"
)

// Plan defines what to process, how to filter, and what outcomes to produce
type Plan struct {
    ID             int64   `db:"id" json:"id"`
    Name           string  `db:"name" json:"name"`
    Description    *string `db:"description" json:"description,omitempty"`
    Mode           string  `db:"mode" json:"mode"`                    // "oneshot", "continuous"
    Status         string  `db:"status" json:"status"`                // "active", "paused", "disabled"
    SourcesJSON    string  `db:"sources_json" json:"-"`
    ConditionsJSON *string `db:"conditions_json" json:"-"`
    OutcomesJSON   string  `db:"outcomes_json" json:"-"`
    CreatedAt      int64   `db:"created_at" json:"created_at"`
    UpdatedAt      int64   `db:"updated_at" json:"updated_at"`
    LastRunAt      *int64  `db:"last_run_at" json:"last_run_at,omitempty"`

    // Parsed fields (populated after JSON unmarshaling)
    Sources    []PlanSource   `db:"-" json:"sources"`
    Conditions *PlanCondition `db:"-" json:"conditions,omitempty"`
    Outcomes   []PlanOutcome  `db:"-" json:"outcomes"`
}

// PlanSource defines where to get files
type PlanSource struct {
    Type            string                     `json:"type"`               // "filesystem", "selection_set", "query"
    Paths           []string                   `json:"paths,omitempty"`    // For filesystem sources
    SourceRef       *string                    `json:"source_ref,omitempty"` // For selection_set or query
    Characteristics []CharacteristicGenerator  `json:"characteristics,omitempty"`
    FollowSymlinks  bool                       `json:"follow_symlinks,omitempty"`
    MaxDepth        *int                       `json:"max_depth,omitempty"`
    IncludeHidden   bool                       `json:"include_hidden,omitempty"`
}

// CharacteristicGenerator specifies metadata to compute
type CharacteristicGenerator struct {
    Type   string                 `json:"type"`    // "media_type", "thumbnail", "exif", "hash"
    Params map[string]interface{} `json:"params,omitempty"`
}

// PlanCondition defines filtering logic (tree structure)
type PlanCondition struct {
    // Logical operators (branch nodes)
    Type       string            `json:"type"` // "all", "any", "none"
    Conditions []*PlanCondition  `json:"conditions,omitempty"`

    // File attribute filters (leaf nodes)
    MediaType    *string  `json:"media_type,omitempty"`
    MinSize      *int64   `json:"min_size,omitempty"`
    MaxSize      *int64   `json:"max_size,omitempty"`
    MinMtime     *int64   `json:"min_mtime,omitempty"`
    MaxMtime     *int64   `json:"max_mtime,omitempty"`
    PathContains *string  `json:"path_contains,omitempty"`
    PathPrefix   *string  `json:"path_prefix,omitempty"`
    PathSuffix   *string  `json:"path_suffix,omitempty"`
    PathPattern  *string  `json:"path_pattern,omitempty"` // Regex
    Extensions   []string `json:"extensions,omitempty"`

    // Custom characteristic filters (future)
    CharacteristicType  *string `json:"characteristic_type,omitempty"`
    CharacteristicValue *string `json:"characteristic_value,omitempty"`
}

// PlanOutcome defines actions to take
type PlanOutcome struct {
    Type             string                 `json:"type"` // "selection_set", "classifier", "export"
    SelectionSetName *string                `json:"selection_set_name,omitempty"`
    Operation        *string                `json:"operation,omitempty"` // "add", "remove", "replace"
    ClassifierType   *string                `json:"classifier_type,omitempty"`
    ClassifierParams map[string]interface{} `json:"classifier_params,omitempty"`
    ExportPath       *string                `json:"export_path,omitempty"`
    ExportFormat     *string                `json:"export_format,omitempty"`
}

// PlanExecution tracks individual plan runs
type PlanExecution struct {
    ID               int64   `db:"id" json:"id"`
    PlanID           int64   `db:"plan_id" json:"plan_id"`
    PlanName         string  `db:"plan_name" json:"plan_name"`
    StartedAt        int64   `db:"started_at" json:"started_at"`
    CompletedAt      *int64  `db:"completed_at" json:"completed_at,omitempty"`
    DurationMs       *int    `db:"duration_ms" json:"duration_ms,omitempty"`
    EntriesProcessed int     `db:"entries_processed" json:"entries_processed"`
    EntriesMatched   int     `db:"entries_matched" json:"entries_matched"`
    OutcomesApplied  int     `db:"outcomes_applied" json:"outcomes_applied"`
    Status           string  `db:"status" json:"status"` // "running", "success", "partial", "error"
    ErrorMessage     *string `db:"error_message" json:"error_message,omitempty"`
}

// PlanOutcomeRecord tracks individual outcome applications
type PlanOutcomeRecord struct {
    ID           int64   `db:"id" json:"id"`
    ExecutionID  int64   `db:"execution_id" json:"execution_id"`
    PlanID       int64   `db:"plan_id" json:"plan_id"`
    EntryPath    string  `db:"entry_path" json:"entry_path"`
    OutcomeType  string  `db:"outcome_type" json:"outcome_type"`
    OutcomeData  string  `db:"outcome_data" json:"outcome_data"`
    Status       string  `db:"status" json:"status"`
    ErrorMessage *string `db:"error_message" json:"error_message,omitempty"`
    CreatedAt    int64   `db:"created_at" json:"created_at"`
}

// MarshalForDB serializes plan fields to JSON for database storage
func (p *Plan) MarshalForDB() error {
    sourcesJSON, err := json.Marshal(p.Sources)
    if err != nil {
        return fmt.Errorf("failed to marshal sources: %w", err)
    }
    p.SourcesJSON = string(sourcesJSON)

    if p.Conditions != nil {
        conditionsJSON, err := json.Marshal(p.Conditions)
        if err != nil {
            return fmt.Errorf("failed to marshal conditions: %w", err)
        }
        condStr := string(conditionsJSON)
        p.ConditionsJSON = &condStr
    }

    outcomesJSON, err := json.Marshal(p.Outcomes)
    if err != nil {
        return fmt.Errorf("failed to marshal outcomes: %w", err)
    }
    p.OutcomesJSON = string(outcomesJSON)

    return nil
}

// UnmarshalFromDB deserializes JSON fields from database
func (p *Plan) UnmarshalFromDB() error {
    if err := json.Unmarshal([]byte(p.SourcesJSON), &p.Sources); err != nil {
        return fmt.Errorf("failed to unmarshal sources: %w", err)
    }

    if p.ConditionsJSON != nil && *p.ConditionsJSON != "" {
        if err := json.Unmarshal([]byte(*p.ConditionsJSON), &p.Conditions); err != nil {
            return fmt.Errorf("failed to unmarshal conditions: %w", err)
        }
    }

    if err := json.Unmarshal([]byte(p.OutcomesJSON), &p.Outcomes); err != nil {
        return fmt.Errorf("failed to unmarshal outcomes: %w", err)
    }

    return nil
}

// Validate ensures plan has required fields and valid values
func (p *Plan) Validate() error {
    if p.Name == "" {
        return fmt.Errorf("plan name is required")
    }

    if p.Mode != "oneshot" && p.Mode != "continuous" {
        return fmt.Errorf("mode must be 'oneshot' or 'continuous'")
    }

    if p.Status != "active" && p.Status != "paused" && p.Status != "disabled" {
        return fmt.Errorf("status must be 'active', 'paused', or 'disabled'")
    }

    if len(p.Sources) == 0 {
        return fmt.Errorf("at least one source is required")
    }

    if len(p.Outcomes) == 0 {
        return fmt.Errorf("at least one outcome is required")
    }

    // Validate sources
    for i, src := range p.Sources {
        if err := src.Validate(); err != nil {
            return fmt.Errorf("source[%d]: %w", i, err)
        }
    }

    // Validate outcomes
    for i, outcome := range p.Outcomes {
        if err := outcome.Validate(); err != nil {
            return fmt.Errorf("outcome[%d]: %w", i, err)
        }
    }

    return nil
}

// Validate checks if PlanSource is properly configured
func (ps *PlanSource) Validate() error {
    switch ps.Type {
    case "filesystem":
        if len(ps.Paths) == 0 {
            return fmt.Errorf("filesystem source requires at least one path")
        }
    case "selection_set", "query":
        if ps.SourceRef == nil || *ps.SourceRef == "" {
            return fmt.Errorf("%s source requires source_ref", ps.Type)
        }
    default:
        return fmt.Errorf("invalid source type: %s", ps.Type)
    }
    return nil
}

// Validate checks if PlanOutcome is properly configured
func (po *PlanOutcome) Validate() error {
    switch po.Type {
    case "selection_set":
        if po.SelectionSetName == nil || *po.SelectionSetName == "" {
            return fmt.Errorf("selection_set outcome requires selection_set_name")
        }
        if po.Operation == nil {
            return fmt.Errorf("selection_set outcome requires operation")
        }
        if *po.Operation != "add" && *po.Operation != "remove" && *po.Operation != "replace" {
            return fmt.Errorf("operation must be 'add', 'remove', or 'replace'")
        }
    case "classifier":
        if po.ClassifierType == nil || *po.ClassifierType == "" {
            return fmt.Errorf("classifier outcome requires classifier_type")
        }
    case "export":
        if po.ExportPath == nil || *po.ExportPath == "" {
            return fmt.Errorf("export outcome requires export_path")
        }
    default:
        return fmt.Errorf("invalid outcome type: %s", po.Type)
    }
    return nil
}

// IsLeaf returns true if this is a leaf condition (not a logical operator)
func (pc *PlanCondition) IsLeaf() bool {
    return pc.Type != "all" && pc.Type != "any" && pc.Type != "none"
}
```

---

## Database Layer

### File: `pkg/database/plans.go`

```go
package database

import (
    "database/sql"
    "fmt"
    "time"

    "github.com/prismon/mcp-space-browser/internal/models"
)

// CreatePlan creates a new plan
func (d *DiskDB) CreatePlan(plan *models.Plan) error {
    if err := plan.Validate(); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    if err := plan.MarshalForDB(); err != nil {
        return fmt.Errorf("failed to marshal plan: %w", err)
    }

    now := time.Now().Unix()
    plan.CreatedAt = now
    plan.UpdatedAt = now

    query := `
        INSERT INTO plans (name, description, mode, status, sources_json, conditions_json, outcomes_json, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `

    result, err := d.db.Exec(query,
        plan.Name,
        plan.Description,
        plan.Mode,
        plan.Status,
        plan.SourcesJSON,
        plan.ConditionsJSON,
        plan.OutcomesJSON,
        plan.CreatedAt,
        plan.UpdatedAt,
    )
    if err != nil {
        return fmt.Errorf("failed to insert plan: %w", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return fmt.Errorf("failed to get plan ID: %w", err)
    }
    plan.ID = id

    return nil
}

// GetPlan retrieves a plan by name
func (d *DiskDB) GetPlan(name string) (*models.Plan, error) {
    query := `SELECT id, name, description, mode, status, sources_json, conditions_json, outcomes_json, created_at, updated_at, last_run_at
              FROM plans WHERE name = ?`

    var plan models.Plan
    err := d.db.QueryRow(query, name).Scan(
        &plan.ID,
        &plan.Name,
        &plan.Description,
        &plan.Mode,
        &plan.Status,
        &plan.SourcesJSON,
        &plan.ConditionsJSON,
        &plan.OutcomesJSON,
        &plan.CreatedAt,
        &plan.UpdatedAt,
        &plan.LastRunAt,
    )
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("plan not found: %s", name)
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get plan: %w", err)
    }

    if err := plan.UnmarshalFromDB(); err != nil {
        return nil, err
    }

    return &plan, nil
}

// ListPlans returns all plans
func (d *DiskDB) ListPlans() ([]*models.Plan, error) {
    query := `SELECT id, name, description, mode, status, sources_json, conditions_json, outcomes_json, created_at, updated_at, last_run_at
              FROM plans ORDER BY created_at DESC`

    rows, err := d.db.Query(query)
    if err != nil {
        return nil, fmt.Errorf("failed to list plans: %w", err)
    }
    defer rows.Close()

    var plans []*models.Plan
    for rows.Next() {
        var plan models.Plan
        if err := rows.Scan(
            &plan.ID,
            &plan.Name,
            &plan.Description,
            &plan.Mode,
            &plan.Status,
            &plan.SourcesJSON,
            &plan.ConditionsJSON,
            &plan.OutcomesJSON,
            &plan.CreatedAt,
            &plan.UpdatedAt,
            &plan.LastRunAt,
        ); err != nil {
            return nil, fmt.Errorf("failed to scan plan: %w", err)
        }

        if err := plan.UnmarshalFromDB(); err != nil {
            return nil, err
        }

        plans = append(plans, &plan)
    }

    return plans, nil
}

// UpdatePlan updates an existing plan
func (d *DiskDB) UpdatePlan(plan *models.Plan) error {
    if err := plan.Validate(); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    if err := plan.MarshalForDB(); err != nil {
        return fmt.Errorf("failed to marshal plan: %w", err)
    }

    plan.UpdatedAt = time.Now().Unix()

    query := `
        UPDATE plans
        SET description = ?, mode = ?, status = ?, sources_json = ?, conditions_json = ?, outcomes_json = ?, updated_at = ?
        WHERE name = ?
    `

    result, err := d.db.Exec(query,
        plan.Description,
        plan.Mode,
        plan.Status,
        plan.SourcesJSON,
        plan.ConditionsJSON,
        plan.OutcomesJSON,
        plan.UpdatedAt,
        plan.Name,
    )
    if err != nil {
        return fmt.Errorf("failed to update plan: %w", err)
    }

    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %w", err)
    }
    if rows == 0 {
        return fmt.Errorf("plan not found: %s", plan.Name)
    }

    return nil
}

// DeletePlan deletes a plan by name
func (d *DiskDB) DeletePlan(name string) error {
    query := `DELETE FROM plans WHERE name = ?`
    result, err := d.db.Exec(query, name)
    if err != nil {
        return fmt.Errorf("failed to delete plan: %w", err)
    }

    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %w", err)
    }
    if rows == 0 {
        return fmt.Errorf("plan not found: %s", name)
    }

    return nil
}

// CreatePlanExecution creates a new execution record
func (d *DiskDB) CreatePlanExecution(planID int64, planName string) (*models.PlanExecution, error) {
    exec := &models.PlanExecution{
        PlanID:    planID,
        PlanName:  planName,
        StartedAt: time.Now().Unix(),
        Status:    "running",
    }

    query := `INSERT INTO plan_executions (plan_id, plan_name, started_at, status)
              VALUES (?, ?, ?, ?)`

    result, err := d.db.Exec(query, exec.PlanID, exec.PlanName, exec.StartedAt, exec.Status)
    if err != nil {
        return nil, fmt.Errorf("failed to create execution: %w", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return nil, fmt.Errorf("failed to get execution ID: %w", err)
    }
    exec.ID = id

    return exec, nil
}

// UpdatePlanExecution updates execution record with results
func (d *DiskDB) UpdatePlanExecution(exec *models.PlanExecution) error {
    query := `
        UPDATE plan_executions
        SET completed_at = ?, duration_ms = ?, entries_processed = ?, entries_matched = ?, outcomes_applied = ?, status = ?, error_message = ?
        WHERE id = ?
    `

    _, err := d.db.Exec(query,
        exec.CompletedAt,
        exec.DurationMs,
        exec.EntriesProcessed,
        exec.EntriesMatched,
        exec.OutcomesApplied,
        exec.Status,
        exec.ErrorMessage,
        exec.ID,
    )
    return err
}

// UpdatePlanLastRun updates the last_run_at timestamp
func (d *DiskDB) UpdatePlanLastRun(planID int64) error {
    now := time.Now().Unix()
    query := `UPDATE plans SET last_run_at = ? WHERE id = ?`
    _, err := d.db.Exec(query, now, planID)
    return err
}

// GetPlanExecutions retrieves execution history for a plan
func (d *DiskDB) GetPlanExecutions(planName string, limit int) ([]*models.PlanExecution, error) {
    query := `SELECT id, plan_id, plan_name, started_at, completed_at, duration_ms, entries_processed, entries_matched, outcomes_applied, status, error_message
              FROM plan_executions WHERE plan_name = ? ORDER BY started_at DESC LIMIT ?`

    rows, err := d.db.Query(query, planName, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get executions: %w", err)
    }
    defer rows.Close()

    var executions []*models.PlanExecution
    for rows.Next() {
        var exec models.PlanExecution
        if err := rows.Scan(
            &exec.ID,
            &exec.PlanID,
            &exec.PlanName,
            &exec.StartedAt,
            &exec.CompletedAt,
            &exec.DurationMs,
            &exec.EntriesProcessed,
            &exec.EntriesMatched,
            &exec.OutcomesApplied,
            &exec.Status,
            &exec.ErrorMessage,
        ); err != nil {
            return nil, fmt.Errorf("failed to scan execution: %w", err)
        }
        executions = append(executions, &exec)
    }

    return executions, nil
}

// RecordPlanOutcome creates an outcome record
func (d *DiskDB) RecordPlanOutcome(record *models.PlanOutcomeRecord) error {
    record.CreatedAt = time.Now().Unix()

    query := `INSERT INTO plan_outcome_records (execution_id, plan_id, entry_path, outcome_type, outcome_data, status, error_message, created_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

    result, err := d.db.Exec(query,
        record.ExecutionID,
        record.PlanID,
        record.EntryPath,
        record.OutcomeType,
        record.OutcomeData,
        record.Status,
        record.ErrorMessage,
        record.CreatedAt,
    )
    if err != nil {
        return fmt.Errorf("failed to record outcome: %w", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return fmt.Errorf("failed to get outcome ID: %w", err)
    }
    record.ID = id

    return nil
}
```

### Schema Migration

Add to `pkg/database/database.go` in the `InitDatabase` function:

```go
// Plans tables
_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS plans (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    mode TEXT CHECK(mode IN ('oneshot', 'continuous')) DEFAULT 'oneshot',
    status TEXT CHECK(status IN ('active', 'paused', 'disabled')) DEFAULT 'active',
    sources_json TEXT NOT NULL,
    conditions_json TEXT,
    outcomes_json TEXT NOT NULL,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now')),
    last_run_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);
CREATE INDEX IF NOT EXISTS idx_plans_mode ON plans(mode);

CREATE TABLE IF NOT EXISTS plan_executions (
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
CREATE INDEX IF NOT EXISTS idx_plan_executions_plan ON plan_executions(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_executions_status ON plan_executions(status);

CREATE TABLE IF NOT EXISTS plan_outcome_records (
    id INTEGER PRIMARY KEY,
    execution_id INTEGER NOT NULL,
    plan_id INTEGER NOT NULL,
    entry_path TEXT NOT NULL,
    outcome_type TEXT NOT NULL,
    outcome_data TEXT,
    status TEXT CHECK(status IN ('success', 'error')) DEFAULT 'success',
    error_message TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    FOREIGN KEY (execution_id) REFERENCES plan_executions(id) ON DELETE CASCADE,
    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE,
    FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_plan_outcomes_execution ON plan_outcome_records(execution_id);
CREATE INDEX IF NOT EXISTS idx_plan_outcomes_plan ON plan_outcome_records(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_outcomes_entry ON plan_outcome_records(entry_path);
`)
if err != nil {
    return nil, fmt.Errorf("failed to create plans tables: %w", err)
}
```

---

## Plan Executor

### File: `pkg/plans/executor.go`

```go
package plans

import (
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
    "time"

    "github.com/prismon/mcp-space-browser/internal/models"
    "github.com/prismon/mcp-space-browser/pkg/database"
    "github.com/sirupsen/logrus"
)

type Executor struct {
    db        *database.DiskDB
    evaluator *ConditionEvaluator
    applier   *OutcomeApplier
    logger    *logrus.Entry
}

func NewExecutor(db *database.DiskDB, logger *logrus.Entry) *Executor {
    return &Executor{
        db:        db,
        evaluator: NewConditionEvaluator(logger),
        applier:   NewOutcomeApplier(db, logger),
        logger:    logger.WithField("component", "plan_executor"),
    }
}

// Execute runs a plan and returns the execution record
func (e *Executor) Execute(plan *models.Plan) (*models.PlanExecution, error) {
    e.logger.Infof("Starting execution of plan: %s", plan.Name)
    startTime := time.Now()

    // Create execution record
    exec, err := e.db.CreatePlanExecution(plan.ID, plan.Name)
    if err != nil {
        return nil, fmt.Errorf("failed to create execution: %w", err)
    }

    // Execute plan (capture any errors)
    err = e.executePlan(plan, exec)

    // Update execution record
    completedAt := time.Now().Unix()
    durationMs := int(time.Since(startTime).Milliseconds())
    exec.CompletedAt = &completedAt
    exec.DurationMs = &durationMs

    if err != nil {
        exec.Status = "error"
        errMsg := err.Error()
        exec.ErrorMessage = &errMsg
    } else if exec.EntriesMatched > 0 {
        exec.Status = "success"
    } else {
        exec.Status = "partial" // No matches
    }

    if updateErr := e.db.UpdatePlanExecution(exec); updateErr != nil {
        e.logger.Errorf("Failed to update execution: %v", updateErr)
    }

    // Update plan's last run time
    if updateErr := e.db.UpdatePlanLastRun(plan.ID); updateErr != nil {
        e.logger.Errorf("Failed to update plan last run: %v", updateErr)
    }

    e.logger.Infof("Plan execution completed: %s (status=%s, matched=%d/%d)",
        plan.Name, exec.Status, exec.EntriesMatched, exec.EntriesProcessed)

    return exec, err
}

func (e *Executor) executePlan(plan *models.Plan, exec *models.PlanExecution) error {
    // Step 1: Resolve sources to get candidate entries
    entries, err := e.resolveSources(plan.Sources)
    if err != nil {
        return fmt.Errorf("failed to resolve sources: %w", err)
    }
    exec.EntriesProcessed = len(entries)
    e.logger.Debugf("Resolved %d entries from sources", len(entries))

    // Step 2: Filter entries based on conditions
    var matchedEntries []*models.Entry
    if plan.Conditions != nil {
        matchedEntries, err = e.filterEntries(entries, plan.Conditions)
        if err != nil {
            return fmt.Errorf("failed to filter entries: %w", err)
        }
    } else {
        matchedEntries = entries // No conditions = all match
    }
    exec.EntriesMatched = len(matchedEntries)
    e.logger.Debugf("Matched %d entries after filtering", len(matchedEntries))

    if len(matchedEntries) == 0 {
        e.logger.Info("No entries matched conditions")
        return nil
    }

    // Step 3: Apply outcomes
    outcomesApplied, err := e.applier.ApplyAll(matchedEntries, plan.Outcomes, exec.ID, plan.ID)
    if err != nil {
        return fmt.Errorf("failed to apply outcomes: %w", err)
    }
    exec.OutcomesApplied = outcomesApplied

    return nil
}

// resolveSources gets all entries from plan sources
func (e *Executor) resolveSources(sources []models.PlanSource) ([]*models.Entry, error) {
    var allEntries []*models.Entry
    seenPaths := make(map[string]bool)

    for i, source := range sources {
        entries, err := e.resolveSource(source)
        if err != nil {
            return nil, fmt.Errorf("source[%d]: %w", i, err)
        }

        // Deduplicate entries by path
        for _, entry := range entries {
            if !seenPaths[entry.Path] {
                allEntries = append(allEntries, entry)
                seenPaths[entry.Path] = true
            }
        }
    }

    return allEntries, nil
}

func (e *Executor) resolveSource(source models.PlanSource) ([]*models.Entry, error) {
    switch source.Type {
    case "filesystem":
        return e.resolveFilesystemSource(source)
    case "selection_set":
        return e.resolveSelectionSetSource(source)
    case "query":
        return e.resolveQuerySource(source)
    default:
        return nil, fmt.Errorf("unknown source type: %s", source.Type)
    }
}

func (e *Executor) resolveFilesystemSource(source models.PlanSource) ([]*models.Entry, error) {
    var allEntries []*models.Entry

    for _, path := range source.Paths {
        entries, err := e.db.GetEntriesByPath(path)
        if err != nil {
            e.logger.Warnf("Failed to get entries for path %s: %v", path, err)
            continue
        }
        allEntries = append(allEntries, entries...)
    }

    return allEntries, nil
}

func (e *Executor) resolveSelectionSetSource(source models.PlanSource) ([]*models.Entry, error) {
    if source.SourceRef == nil {
        return nil, fmt.Errorf("selection_set source requires source_ref")
    }

    return e.db.GetSelectionSetEntries(*source.SourceRef)
}

func (e *Executor) resolveQuerySource(source models.PlanSource) ([]*models.Entry, error) {
    if source.SourceRef == nil {
        return nil, fmt.Errorf("query source requires source_ref")
    }

    return e.db.ExecuteQuery(*source.SourceRef)
}

// filterEntries evaluates conditions and returns matching entries
func (e *Executor) filterEntries(entries []*models.Entry, condition *models.PlanCondition) ([]*models.Entry, error) {
    var matched []*models.Entry

    for _, entry := range entries {
        matches, err := e.evaluator.Evaluate(entry, condition)
        if err != nil {
            e.logger.Warnf("Failed to evaluate condition for %s: %v", entry.Path, err)
            continue
        }
        if matches {
            matched = append(matched, entry)
        }
    }

    return matched, nil
}
```

---

## Condition Evaluator

### File: `pkg/plans/evaluator.go`

```go
package plans

import (
    "fmt"
    "regexp"
    "strings"

    "github.com/prismon/mcp-space-browser/internal/models"
    "github.com/sirupsen/logrus"
)

type ConditionEvaluator struct {
    logger *logrus.Entry
}

func NewConditionEvaluator(logger *logrus.Entry) *ConditionEvaluator {
    return &ConditionEvaluator{
        logger: logger.WithField("component", "condition_evaluator"),
    }
}

// Evaluate checks if an entry matches the condition tree
func (ce *ConditionEvaluator) Evaluate(entry *models.Entry, condition *models.PlanCondition) (bool, error) {
    if condition == nil {
        return true, nil // No condition = always match
    }

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

func (ce *ConditionEvaluator) evaluateAll(entry *models.Entry, conditions []*models.PlanCondition) (bool, error) {
    for _, cond := range conditions {
        match, err := ce.Evaluate(entry, cond)
        if err != nil {
            return false, err
        }
        if !match {
            return false, nil
        }
    }
    return true, nil
}

func (ce *ConditionEvaluator) evaluateAny(entry *models.Entry, conditions []*models.PlanCondition) (bool, error) {
    for _, cond := range conditions {
        match, err := ce.Evaluate(entry, cond)
        if err != nil {
            return false, err
        }
        if match {
            return true, nil
        }
    }
    return false, nil
}

func (ce *ConditionEvaluator) evaluateNone(entry *models.Entry, conditions []*models.PlanCondition) (bool, error) {
    for _, cond := range conditions {
        match, err := ce.Evaluate(entry, cond)
        if err != nil {
            return false, err
        }
        if match {
            return false, nil
        }
    }
    return true, nil
}

func (ce *ConditionEvaluator) evaluateLeaf(entry *models.Entry, condition *models.PlanCondition) (bool, error) {
    // Size filters
    if condition.MinSize != nil && entry.Size < *condition.MinSize {
        return false, nil
    }
    if condition.MaxSize != nil && entry.Size > *condition.MaxSize {
        return false, nil
    }

    // Time filters
    if condition.MinMtime != nil && entry.MTime < *condition.MinMtime {
        return false, nil
    }
    if condition.MaxMtime != nil && entry.MTime > *condition.MaxMtime {
        return false, nil
    }

    // Path filters
    if condition.PathContains != nil && !strings.Contains(entry.Path, *condition.PathContains) {
        return false, nil
    }
    if condition.PathPrefix != nil && !strings.HasPrefix(entry.Path, *condition.PathPrefix) {
        return false, nil
    }
    if condition.PathSuffix != nil && !strings.HasSuffix(entry.Path, *condition.PathSuffix) {
        return false, nil
    }
    if condition.PathPattern != nil {
        matched, err := regexp.MatchString(*condition.PathPattern, entry.Path)
        if err != nil {
            return false, fmt.Errorf("invalid regex pattern: %w", err)
        }
        if !matched {
            return false, nil
        }
    }

    // Extension filter
    if len(condition.Extensions) > 0 {
        matched := false
        for _, ext := range condition.Extensions {
            if strings.HasSuffix(strings.ToLower(entry.Path), "."+strings.ToLower(ext)) {
                matched = true
                break
            }
        }
        if !matched {
            return false, nil
        }
    }

    // Media type filter (would require media_type field in Entry)
    // TODO: Implement when characteristic generators are added

    return true, nil
}
```

---

## Outcome Applier

### File: `pkg/plans/outcomes.go`

```go
package plans

import (
    "encoding/json"
    "fmt"

    "github.com/prismon/mcp-space-browser/internal/models"
    "github.com/prismon/mcp-space-browser/pkg/database"
    "github.com/sirupsen/logrus"
)

type OutcomeApplier struct {
    db     *database.DiskDB
    logger *logrus.Entry
}

func NewOutcomeApplier(db *database.DiskDB, logger *logrus.Entry) *OutcomeApplier {
    return &OutcomeApplier{
        db:     db,
        logger: logger.WithField("component", "outcome_applier"),
    }
}

// ApplyAll applies all outcomes to matched entries
func (oa *OutcomeApplier) ApplyAll(entries []*models.Entry, outcomes []models.PlanOutcome, execID, planID int64) (int, error) {
    totalApplied := 0

    for i, outcome := range outcomes {
        count, err := oa.Apply(entries, outcome, execID, planID)
        if err != nil {
            oa.logger.Errorf("Failed to apply outcome[%d]: %v", i, err)
            continue
        }
        totalApplied += count
    }

    return totalApplied, nil
}

// Apply executes a single outcome on matched entries
func (oa *OutcomeApplier) Apply(entries []*models.Entry, outcome models.PlanOutcome, execID, planID int64) (int, error) {
    switch outcome.Type {
    case "selection_set":
        return oa.applySelectionSet(entries, outcome, execID, planID)
    case "classifier":
        return oa.applyClassifier(entries, outcome, execID, planID)
    default:
        return 0, fmt.Errorf("unknown outcome type: %s", outcome.Type)
    }
}

func (oa *OutcomeApplier) applySelectionSet(entries []*models.Entry, outcome models.PlanOutcome, execID, planID int64) (int, error) {
    if outcome.SelectionSetName == nil {
        return 0, fmt.Errorf("selection_set_name is required")
    }

    setName := *outcome.SelectionSetName
    paths := make([]string, len(entries))
    for i, entry := range entries {
        paths[i] = entry.Path
    }

    // Ensure selection set exists
    _, err := oa.db.GetSelectionSet(setName)
    if err != nil {
        // Create if doesn't exist
        if err := oa.db.CreateSelectionSet(&models.SelectionSet{
            Name:        setName,
            Description: stringPtr(fmt.Sprintf("Auto-created by plan")),
        }); err != nil {
            return 0, fmt.Errorf("failed to create selection set: %w", err)
        }
        oa.logger.Infof("Created selection set: %s", setName)
    }

    // Apply operation
    var opErr error
    switch *outcome.Operation {
    case "add":
        opErr = oa.db.AddToSelectionSet(setName, paths)
    case "remove":
        opErr = oa.db.RemoveFromSelectionSet(setName, paths)
    case "replace":
        // Clear and re-add
        existing, err := oa.db.GetSelectionSetEntries(setName)
        if err != nil {
            return 0, fmt.Errorf("failed to get existing entries: %w", err)
        }
        existingPaths := make([]string, len(existing))
        for i, e := range existing {
            existingPaths[i] = e.Path
        }
        if err := oa.db.RemoveFromSelectionSet(setName, existingPaths); err != nil {
            return 0, fmt.Errorf("failed to clear set: %w", err)
        }
        opErr = oa.db.AddToSelectionSet(setName, paths)
    default:
        return 0, fmt.Errorf("invalid operation: %s", *outcome.Operation)
    }

    if opErr != nil {
        return 0, fmt.Errorf("failed to %s entries: %w", *outcome.Operation, opErr)
    }

    // Record outcomes for audit
    for _, entry := range entries {
        outcomeData, _ := json.Marshal(map[string]string{
            "selection_set": setName,
            "operation":     *outcome.Operation,
        })

        record := &models.PlanOutcomeRecord{
            ExecutionID: execID,
            PlanID:      planID,
            EntryPath:   entry.Path,
            OutcomeType: "selection_set",
            OutcomeData: string(outcomeData),
            Status:      "success",
        }

        if err := oa.db.RecordPlanOutcome(record); err != nil {
            oa.logger.Warnf("Failed to record outcome for %s: %v", entry.Path, err)
        }
    }

    oa.logger.Infof("Applied selection_set outcome: %s %d entries to %s",
        *outcome.Operation, len(entries), setName)

    return len(entries), nil
}

func (oa *OutcomeApplier) applyClassifier(entries []*models.Entry, outcome models.PlanOutcome, execID, planID int64) (int, error) {
    // TODO: Implement classifier outcomes (thumbnail generation, metadata extraction, etc.)
    oa.logger.Warnf("Classifier outcomes not yet implemented: %s", *outcome.ClassifierType)
    return 0, nil
}

func stringPtr(s string) *string {
    return &s
}
```

---

## Testing Strategy

### Unit Tests for Condition Evaluator

```go
// pkg/plans/evaluator_test.go
package plans

import (
    "testing"

    "github.com/prismon/mcp-space-browser/internal/models"
    "github.com/sirupsen/logrus"
    "github.com/stretchr/testify/assert"
)

func TestEvaluateLeaf_SizeFilter(t *testing.T) {
    logger := logrus.New().WithField("test", "evaluator")
    evaluator := NewConditionEvaluator(logger)

    entry := &models.Entry{
        Path: "/test/file.mp4",
        Size: 1024 * 1024 * 100, // 100MB
    }

    minSize := int64(1024 * 1024 * 50) // 50MB
    condition := &models.PlanCondition{
        Type:    "size",
        MinSize: &minSize,
    }

    match, err := evaluator.Evaluate(entry, condition)
    assert.NoError(t, err)
    assert.True(t, match)
}

func TestEvaluateAll(t *testing.T) {
    logger := logrus.New().WithField("test", "evaluator")
    evaluator := NewConditionEvaluator(logger)

    entry := &models.Entry{
        Path:  "/videos/movie.mp4",
        Size:  1024 * 1024 * 200,
        MTime: 1700000000,
    }

    minSize := int64(1024 * 1024 * 100)
    pathPrefix := "/videos"
    condition := &models.PlanCondition{
        Type: "all",
        Conditions: []*models.PlanCondition{
            {MinSize: &minSize},
            {PathPrefix: &pathPrefix},
        },
    }

    match, err := evaluator.Evaluate(entry, condition)
    assert.NoError(t, err)
    assert.True(t, match)
}
```

### Integration Tests

```go
// pkg/plans/executor_test.go
package plans

import (
    "os"
    "testing"

    "github.com/prismon/mcp-space-browser/internal/models"
    "github.com/prismon/mcp-space-browser/pkg/database"
    "github.com/sirupsen/logrus"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExecutePlan_OneShot(t *testing.T) {
    // Setup
    os.Setenv("GO_ENV", "test")
    logger := logrus.New().WithField("test", "executor")

    db, err := database.NewDiskDB(":memory:", logger)
    require.NoError(t, err)
    defer db.Close()

    // Create test entries
    tmpDir := t.TempDir()
    createTestFile(t, tmpDir+"/large.mp4", 1024*1024*200)
    createTestFile(t, tmpDir+"/small.txt", 100)

    // Index filesystem
    // (assume crawler has indexed tmpDir)

    // Create plan
    minSize := int64(1024 * 1024 * 100)
    setName := "large-files"
    operation := "add"

    plan := &models.Plan{
        Name: "test-plan",
        Mode: "oneshot",
        Status: "active",
        Sources: []models.PlanSource{
            {
                Type:  "filesystem",
                Paths: []string{tmpDir},
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

    err = db.CreatePlan(plan)
    require.NoError(t, err)

    // Execute
    executor := NewExecutor(db, logger)
    exec, err := executor.Execute(plan)
    require.NoError(t, err)

    // Verify
    assert.Equal(t, "success", exec.Status)
    assert.Equal(t, 1, exec.EntriesMatched)
    assert.Equal(t, 1, exec.OutcomesApplied)

    // Check selection set
    entries, err := db.GetSelectionSetEntries(setName)
    require.NoError(t, err)
    assert.Len(t, entries, 1)
    assert.Contains(t, entries[0].Path, "large.mp4")
}

func createTestFile(t *testing.T, path string, size int64) {
    f, err := os.Create(path)
    require.NoError(t, err)
    defer f.Close()

    if size > 0 {
        require.NoError(t, f.Truncate(size))
    }
}
```

---

## Summary

This implementation provides:

1. **Clean data models** with JSON marshaling and validation
2. **Database layer** with full CRUD operations
3. **Execution engine** with source resolution, condition evaluation, and outcome application
4. **Testable components** with clear interfaces
5. **Audit trail** via execution and outcome records

Next steps:
- Implement MCP tools for plan management
- Add CLI commands for plan operations
- Build continuous plan mode with filesystem watching
- Add characteristic generators (media type, thumbnails, etc.)
