package models

import (
	"encoding/json"
	"fmt"
)

// Plan defines what to process, how to filter, and what outcomes to produce
type Plan struct {
	ID          int64   `db:"id" json:"id"`
	Name        string  `db:"name" json:"name"`
	Description *string `db:"description" json:"description,omitempty"`
	Mode        string  `db:"mode" json:"mode"`                    // "oneshot", "continuous"
	Status      string  `db:"status" json:"status"`                // "active", "paused", "disabled"
	SourcesJSON string  `db:"sources_json" json:"-"`
	ConditionsJSON *string `db:"conditions_json" json:"-"`
	OutcomesJSON   string `db:"outcomes_json" json:"-"`
	CreatedAt      int64  `db:"created_at" json:"created_at"`
	UpdatedAt      int64  `db:"updated_at" json:"updated_at"`
	LastRunAt      *int64 `db:"last_run_at" json:"last_run_at,omitempty"`

	// Parsed fields (not stored in DB)
	// Reuses RuleCondition and RuleOutcome from existing rules system
	Sources    []PlanSource    `db:"-" json:"sources"`
	Conditions *RuleCondition  `db:"-" json:"conditions,omitempty"`
	Outcomes   []RuleOutcome   `db:"-" json:"outcomes"`
}

// PlanSource defines where to get files and what metadata to generate
type PlanSource struct {
	Type            string                     `json:"type"`               // "filesystem", "resource_set", "query"
	Paths           []string                   `json:"paths,omitempty"`    // Root paths to scan (for filesystem)
	SourceRef       *string                    `json:"source_ref,omitempty"` // Reference to resource_set or query name
	Characteristics []CharacteristicGenerator  `json:"characteristics,omitempty"`
	FollowSymlinks  bool                       `json:"follow_symlinks,omitempty"`
	MaxDepth        *int                       `json:"max_depth,omitempty"`
	IncludeHidden   bool                       `json:"include_hidden,omitempty"`
}

// CharacteristicGenerator specifies metadata/analysis to perform
type CharacteristicGenerator struct {
	Type   string                 `json:"type"`    // "media_type", "thumbnail", "exif", "hash"
	Params map[string]interface{} `json:"params,omitempty"`
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

// PlanOutcomeRecord tracks individual outcome applications (uses RuleOutcomeRecord structure)
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

	// Validate outcomes (use existing RuleOutcome validation)
	for i, outcome := range p.Outcomes {
		if outcome.ResourceSetName == "" {
			return fmt.Errorf("outcome[%d]: resourceSetName is required", i)
		}
		if outcome.Type != "resource_set" && outcome.Type != "classifier" && outcome.Type != "chained" {
			return fmt.Errorf("outcome[%d]: invalid outcome type: %s", i, outcome.Type)
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
	case "resource_set", "query":
		if ps.SourceRef == nil || *ps.SourceRef == "" {
			return fmt.Errorf("%s source requires source_ref", ps.Type)
		}
	default:
		return fmt.Errorf("invalid source type: %s", ps.Type)
	}
	return nil
}
