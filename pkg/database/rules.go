package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// Rule Operations

// CreateRule creates a new rule in the database
func (d *DiskDB) CreateRule(rule *models.Rule) (int64, error) {
	log.WithFields(logrus.Fields{
		"name":     rule.Name,
		"enabled":  rule.Enabled,
		"priority": rule.Priority,
	}).Info("Creating rule")

	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	result, err := d.db.Exec(`
		INSERT INTO rules (name, description, enabled, priority, condition_json, outcome_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, rule.Name, rule.Description, enabled, rule.Priority, rule.ConditionJSON, rule.OutcomeJSON)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetRule retrieves a rule by name
func (d *DiskDB) GetRule(name string) (*models.Rule, error) {
	var rule models.Rule
	var description sql.NullString
	var enabled int

	err := d.db.QueryRow(`
		SELECT id, name, description, enabled, priority, condition_json, outcome_json, created_at, updated_at
		FROM rules WHERE name = ?
	`, name).Scan(
		&rule.ID, &rule.Name, &description, &enabled, &rule.Priority,
		&rule.ConditionJSON, &rule.OutcomeJSON, &rule.CreatedAt, &rule.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		rule.Description = &description.String
	}
	rule.Enabled = enabled == 1

	return &rule, nil
}

// GetRuleByID retrieves a rule by ID
func (d *DiskDB) GetRuleByID(id int64) (*models.Rule, error) {
	var rule models.Rule
	var description sql.NullString
	var enabled int

	err := d.db.QueryRow(`
		SELECT id, name, description, enabled, priority, condition_json, outcome_json, created_at, updated_at
		FROM rules WHERE id = ?
	`, id).Scan(
		&rule.ID, &rule.Name, &description, &enabled, &rule.Priority,
		&rule.ConditionJSON, &rule.OutcomeJSON, &rule.CreatedAt, &rule.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		rule.Description = &description.String
	}
	rule.Enabled = enabled == 1

	return &rule, nil
}

// ListRules retrieves all rules, ordered by priority (highest first)
func (d *DiskDB) ListRules(enabledOnly bool) ([]*models.Rule, error) {
	query := `
		SELECT id, name, description, enabled, priority, condition_json, outcome_json, created_at, updated_at
		FROM rules
	`
	args := []interface{}{}

	if enabledOnly {
		query += " WHERE enabled = 1"
	}

	query += " ORDER BY priority DESC, name ASC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*models.Rule
	for rows.Next() {
		var rule models.Rule
		var description sql.NullString
		var enabled int

		if err := rows.Scan(
			&rule.ID, &rule.Name, &description, &enabled, &rule.Priority,
			&rule.ConditionJSON, &rule.OutcomeJSON, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if description.Valid {
			rule.Description = &description.String
		}
		rule.Enabled = enabled == 1

		rules = append(rules, &rule)
	}

	return rules, rows.Err()
}

// UpdateRule updates an existing rule
func (d *DiskDB) UpdateRule(rule *models.Rule) error {
	log.WithField("name", rule.Name).Info("Updating rule")

	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	_, err := d.db.Exec(`
		UPDATE rules
		SET description = ?, enabled = ?, priority = ?, condition_json = ?, outcome_json = ?, updated_at = strftime('%s', 'now')
		WHERE name = ?
	`, rule.Description, enabled, rule.Priority, rule.ConditionJSON, rule.OutcomeJSON, rule.Name)

	return err
}

// DeleteRule deletes a rule by name
func (d *DiskDB) DeleteRule(name string) error {
	log.WithField("name", name).Info("Deleting rule")
	_, err := d.db.Exec(`DELETE FROM rules WHERE name = ?`, name)
	return err
}

// Rule Execution Operations

// CreateRuleExecution creates a new rule execution record
// IMPORTANT: selection_set_id is REQUIRED - every rule execution must be associated with a selection set
func (d *DiskDB) CreateRuleExecution(execution *models.RuleExecution) (int64, error) {
	if execution.ResourceSetID == 0 {
		return 0, fmt.Errorf("selection_set_id is required for rule execution")
	}

	log.WithFields(logrus.Fields{
		"ruleID":         execution.RuleID,
		"resourceSetID": execution.ResourceSetID,
	}).Debug("Creating rule execution record")

	result, err := d.db.Exec(`
		INSERT INTO rule_executions (rule_id, selection_set_id, entries_matched, entries_processed, status, error_message, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, execution.RuleID, execution.ResourceSetID, execution.EntriesMatched, execution.EntriesProcessed,
		execution.Status, execution.ErrorMessage, execution.DurationMs)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetRuleExecution retrieves a rule execution by ID
func (d *DiskDB) GetRuleExecution(id int64) (*models.RuleExecution, error) {
	var execution models.RuleExecution
	var errorMessage sql.NullString
	var durationMs sql.NullInt64

	err := d.db.QueryRow(`
		SELECT id, rule_id, selection_set_id, executed_at, entries_matched, entries_processed, status, error_message, duration_ms
		FROM rule_executions WHERE id = ?
	`, id).Scan(
		&execution.ID, &execution.RuleID, &execution.ResourceSetID, &execution.ExecutedAt,
		&execution.EntriesMatched, &execution.EntriesProcessed, &execution.Status,
		&errorMessage, &durationMs,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if errorMessage.Valid {
		execution.ErrorMessage = &errorMessage.String
	}
	if durationMs.Valid {
		ms := int(durationMs.Int64)
		execution.DurationMs = &ms
	}

	return &execution, nil
}

// ListRuleExecutions retrieves executions for a rule
func (d *DiskDB) ListRuleExecutions(ruleID int64, limit int) ([]*models.RuleExecution, error) {
	query := `
		SELECT id, rule_id, selection_set_id, executed_at, entries_matched, entries_processed, status, error_message, duration_ms
		FROM rule_executions
		WHERE rule_id = ?
		ORDER BY executed_at DESC
	`
	args := []interface{}{ruleID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*models.RuleExecution
	for rows.Next() {
		var execution models.RuleExecution
		var errorMessage sql.NullString
		var durationMs sql.NullInt64

		if err := rows.Scan(
			&execution.ID, &execution.RuleID, &execution.ResourceSetID, &execution.ExecutedAt,
			&execution.EntriesMatched, &execution.EntriesProcessed, &execution.Status,
			&errorMessage, &durationMs,
		); err != nil {
			return nil, err
		}

		if errorMessage.Valid {
			execution.ErrorMessage = &errorMessage.String
		}
		if durationMs.Valid {
			ms := int(durationMs.Int64)
			execution.DurationMs = &ms
		}

		executions = append(executions, &execution)
	}

	return executions, rows.Err()
}

// ListRuleExecutionsByResourceSet retrieves executions associated with a selection set
func (d *DiskDB) ListRuleExecutionsByResourceSet(resourceSetID int64, limit int) ([]*models.RuleExecution, error) {
	query := `
		SELECT id, rule_id, selection_set_id, executed_at, entries_matched, entries_processed, status, error_message, duration_ms
		FROM rule_executions
		WHERE selection_set_id = ?
		ORDER BY executed_at DESC
	`
	args := []interface{}{resourceSetID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*models.RuleExecution
	for rows.Next() {
		var execution models.RuleExecution
		var errorMessage sql.NullString
		var durationMs sql.NullInt64

		if err := rows.Scan(
			&execution.ID, &execution.RuleID, &execution.ResourceSetID, &execution.ExecutedAt,
			&execution.EntriesMatched, &execution.EntriesProcessed, &execution.Status,
			&errorMessage, &durationMs,
		); err != nil {
			return nil, err
		}

		if errorMessage.Valid {
			execution.ErrorMessage = &errorMessage.String
		}
		if durationMs.Valid {
			ms := int(durationMs.Int64)
			execution.DurationMs = &ms
		}

		executions = append(executions, &execution)
	}

	return executions, rows.Err()
}

// Rule Outcome Operations

// CreateRuleOutcome creates a new rule outcome record
// IMPORTANT: selection_set_id is REQUIRED - every outcome must be associated with a selection set
func (d *DiskDB) CreateRuleOutcome(outcome *models.RuleOutcomeRecord) (int64, error) {
	if outcome.ResourceSetID == 0 {
		return 0, fmt.Errorf("selection_set_id is required for rule outcome")
	}

	log.WithFields(logrus.Fields{
		"executionID":    outcome.ExecutionID,
		"resourceSetID": outcome.ResourceSetID,
		"entryPath":      outcome.EntryPath,
		"outcomeType":    outcome.OutcomeType,
	}).Trace("Creating rule outcome record")

	result, err := d.db.Exec(`
		INSERT INTO rule_outcomes (execution_id, selection_set_id, entry_path, outcome_type, outcome_data, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, outcome.ExecutionID, outcome.ResourceSetID, outcome.EntryPath, outcome.OutcomeType,
		outcome.OutcomeData, outcome.Status, outcome.ErrorMessage)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// ListRuleOutcomes retrieves outcomes for a rule execution
func (d *DiskDB) ListRuleOutcomes(executionID int64) ([]*models.RuleOutcomeRecord, error) {
	rows, err := d.db.Query(`
		SELECT id, execution_id, selection_set_id, entry_path, outcome_type, outcome_data, status, error_message, created_at
		FROM rule_outcomes
		WHERE execution_id = ?
		ORDER BY created_at ASC
	`, executionID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []*models.RuleOutcomeRecord
	for rows.Next() {
		var outcome models.RuleOutcomeRecord
		var outcomeData, errorMessage sql.NullString

		if err := rows.Scan(
			&outcome.ID, &outcome.ExecutionID, &outcome.ResourceSetID, &outcome.EntryPath,
			&outcome.OutcomeType, &outcomeData, &outcome.Status, &errorMessage, &outcome.CreatedAt,
		); err != nil {
			return nil, err
		}

		if outcomeData.Valid {
			outcome.OutcomeData = &outcomeData.String
		}
		if errorMessage.Valid {
			outcome.ErrorMessage = &errorMessage.String
		}

		outcomes = append(outcomes, &outcome)
	}

	return outcomes, rows.Err()
}

// ListRuleOutcomesByResourceSet retrieves outcomes associated with a selection set
func (d *DiskDB) ListRuleOutcomesByResourceSet(resourceSetID int64, limit int) ([]*models.RuleOutcomeRecord, error) {
	query := `
		SELECT id, execution_id, selection_set_id, entry_path, outcome_type, outcome_data, status, error_message, created_at
		FROM rule_outcomes
		WHERE selection_set_id = ?
		ORDER BY created_at DESC
	`
	args := []interface{}{resourceSetID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []*models.RuleOutcomeRecord
	for rows.Next() {
		var outcome models.RuleOutcomeRecord
		var outcomeData, errorMessage sql.NullString

		if err := rows.Scan(
			&outcome.ID, &outcome.ExecutionID, &outcome.ResourceSetID, &outcome.EntryPath,
			&outcome.OutcomeType, &outcomeData, &outcome.Status, &errorMessage, &outcome.CreatedAt,
		); err != nil {
			return nil, err
		}

		if outcomeData.Valid {
			outcome.OutcomeData = &outcomeData.String
		}
		if errorMessage.Valid {
			outcome.ErrorMessage = &errorMessage.String
		}

		outcomes = append(outcomes, &outcome)
	}

	return outcomes, rows.Err()
}

// Helper Functions

// ValidateRuleOutcome validates that an outcome has a required selection set
func ValidateRuleOutcome(outcome *models.RuleOutcome) error {
	if outcome.ResourceSetName == "" {
		return fmt.Errorf("resourceSetName is required for all rule outcomes")
	}

	// Validate chained outcomes recursively
	if outcome.Type == "chained" {
		for i, childOutcome := range outcome.Outcomes {
			if err := ValidateRuleOutcome(childOutcome); err != nil {
				return fmt.Errorf("chained outcome %d: %w", i, err)
			}
		}
	}

	return nil
}

// EnsureResourceSetForOutcome ensures a selection set exists for the given outcome
// If it doesn't exist, it creates one
func (d *DiskDB) EnsureResourceSetForOutcome(outcome *models.RuleOutcome, ruleName string) (int64, error) {
	if outcome.ResourceSetName == "" {
		return 0, fmt.Errorf("resourceSetName is required")
	}

	// Check if selection set exists
	set, err := d.GetResourceSet(outcome.ResourceSetName)
	if err != nil {
		return 0, fmt.Errorf("failed to check selection set: %w", err)
	}

	if set != nil {
		return set.ID, nil
	}

	// Create new selection set
	description := fmt.Sprintf("Auto-created for rule: %s", ruleName)
	newSet := &models.ResourceSet{
		Name:        outcome.ResourceSetName,
		Description: &description,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	setID, err := d.CreateResourceSet(newSet)
	if err != nil {
		return 0, fmt.Errorf("failed to create selection set: %w", err)
	}

	log.WithFields(logrus.Fields{
		"setName": outcome.ResourceSetName,
		"setID":   setID,
		"rule":    ruleName,
	}).Info("Auto-created selection set for rule outcome")

	return setID, nil
}

// ParseRuleCondition parses a JSON condition string into a RuleCondition
func ParseRuleCondition(conditionJSON string) (*models.RuleCondition, error) {
	var condition models.RuleCondition
	if err := json.Unmarshal([]byte(conditionJSON), &condition); err != nil {
		return nil, fmt.Errorf("failed to parse condition: %w", err)
	}
	return &condition, nil
}

// ParseRuleOutcome parses a JSON outcome string into a RuleOutcome
func ParseRuleOutcome(outcomeJSON string) (*models.RuleOutcome, error) {
	var outcome models.RuleOutcome
	if err := json.Unmarshal([]byte(outcomeJSON), &outcome); err != nil {
		return nil, fmt.Errorf("failed to parse outcome: %w", err)
	}

	// Validate that outcome has required selection set
	if err := ValidateRuleOutcome(&outcome); err != nil {
		return nil, err
	}

	return &outcome, nil
}
