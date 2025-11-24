package rules

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/sirupsen/logrus"
)

// Engine is responsible for evaluating rules and applying outcomes
type Engine struct {
	db         *sql.DB
	classifier classifier.Classifier
	log        *logrus.Entry
}

// NewEngine creates a new rule execution engine
func NewEngine(db *sql.DB, clf classifier.Classifier) *Engine {
	return &Engine{
		db:         db,
		classifier: clf,
		log:        logrus.WithField("component", "rules-engine"),
	}
}

// ExecuteRulesForPath evaluates and executes all enabled rules for a given path
func (e *Engine) ExecuteRulesForPath(ctx context.Context, path string) error {
	// Get the entry from database
	entry, err := e.getEntry(path)
	if err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}
	if entry == nil {
		return fmt.Errorf("entry not found: %s", path)
	}

	// Get all enabled rules ordered by priority
	rules, err := e.getEnabledRules()
	if err != nil {
		return fmt.Errorf("failed to get enabled rules: %w", err)
	}

	e.log.WithFields(logrus.Fields{
		"path":       path,
		"ruleCount":  len(rules),
	}).Debug("Executing rules for path")

	// Execute each rule
	for _, rule := range rules {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := e.executeRule(ctx, rule, entry); err != nil {
			e.log.WithError(err).WithFields(logrus.Fields{
				"rule": rule.Name,
				"path": path,
			}).Error("Failed to execute rule")
		}
	}

	return nil
}

// executeRule executes a single rule for an entry
func (e *Engine) executeRule(ctx context.Context, rule *models.Rule, entry *models.Entry) error {
	startTime := time.Now()

	// Parse condition
	var condition models.RuleCondition
	if err := json.Unmarshal([]byte(rule.ConditionJSON), &condition); err != nil {
		return fmt.Errorf("failed to parse condition: %w", err)
	}

	// Evaluate condition
	matches, err := e.evaluateCondition(&condition, entry)
	if err != nil {
		return fmt.Errorf("failed to evaluate condition: %w", err)
	}

	if !matches {
		e.log.WithFields(logrus.Fields{
			"rule": rule.Name,
			"path": entry.Path,
		}).Trace("Rule condition not matched")
		return nil
	}

	e.log.WithFields(logrus.Fields{
		"rule": rule.Name,
		"path": entry.Path,
	}).Debug("Rule condition matched, applying outcome")

	// Parse outcome
	var outcome models.RuleOutcome
	if err := json.Unmarshal([]byte(rule.OutcomeJSON), &outcome); err != nil {
		return fmt.Errorf("failed to parse outcome: %w", err)
	}

	// Validate outcome has selection set name
	if outcome.SelectionSetName == "" {
		return fmt.Errorf("rule outcome missing required SelectionSetName")
	}

	// Ensure selection set exists
	selectionSetID, err := e.ensureSelectionSet(outcome.SelectionSetName)
	if err != nil {
		return fmt.Errorf("failed to ensure selection set: %w", err)
	}

	// Create execution record
	executionID, err := e.createExecution(rule.ID, selectionSetID)
	if err != nil {
		return fmt.Errorf("failed to create execution record: %w", err)
	}

	// Apply outcome
	err = e.applyOutcome(ctx, &outcome, entry, executionID, selectionSetID)

	// Update execution record
	duration := time.Since(startTime).Milliseconds()
	status := "success"
	var errorMsg *string
	if err != nil {
		status = "error"
		msg := err.Error()
		errorMsg = &msg
	}

	e.updateExecution(executionID, status, int(duration), errorMsg)

	return err
}

// evaluateCondition evaluates a rule condition against an entry
func (e *Engine) evaluateCondition(cond *models.RuleCondition, entry *models.Entry) (bool, error) {
	switch cond.Type {
	case "all":
		// All conditions must match
		for _, subCond := range cond.Conditions {
			matches, err := e.evaluateCondition(subCond, entry)
			if err != nil {
				return false, err
			}
			if !matches {
				return false, nil
			}
		}
		return true, nil

	case "any":
		// At least one condition must match
		for _, subCond := range cond.Conditions {
			matches, err := e.evaluateCondition(subCond, entry)
			if err != nil {
				return false, err
			}
			if matches {
				return true, nil
			}
		}
		return false, nil

	case "none":
		// No conditions should match
		for _, subCond := range cond.Conditions {
			matches, err := e.evaluateCondition(subCond, entry)
			if err != nil {
				return false, err
			}
			if matches {
				return false, nil
			}
		}
		return true, nil

	case "media_type":
		return e.matchMediaType(cond, entry), nil

	case "size":
		return e.matchSize(cond, entry), nil

	case "time":
		return e.matchTime(cond, entry), nil

	case "path":
		return e.matchPath(cond, entry)

	default:
		return false, fmt.Errorf("unknown condition type: %s", cond.Type)
	}
}

// matchMediaType checks if entry matches media type condition
func (e *Engine) matchMediaType(cond *models.RuleCondition, entry *models.Entry) bool {
	if cond.MediaType == nil {
		return true
	}

	ext := strings.ToLower(filepath.Ext(entry.Path))
	mediaType := *cond.MediaType

	switch mediaType {
	case "image":
		imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".heic"}
		return contains(imageExts, ext)
	case "video":
		videoExts := []string{".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v"}
		return contains(videoExts, ext)
	case "audio":
		audioExts := []string{".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a"}
		return contains(audioExts, ext)
	case "document":
		docExts := []string{".pdf", ".doc", ".docx", ".txt", ".rtf", ".odt"}
		return contains(docExts, ext)
	default:
		return false
	}
}

// matchSize checks if entry matches size condition
func (e *Engine) matchSize(cond *models.RuleCondition, entry *models.Entry) bool {
	if cond.MinSize != nil && entry.Size < *cond.MinSize {
		return false
	}
	if cond.MaxSize != nil && entry.Size > *cond.MaxSize {
		return false
	}
	return true
}

// matchTime checks if entry matches time condition
func (e *Engine) matchTime(cond *models.RuleCondition, entry *models.Entry) bool {
	if cond.MinMtime != nil && entry.Mtime < *cond.MinMtime {
		return false
	}
	if cond.MaxMtime != nil && entry.Mtime > *cond.MaxMtime {
		return false
	}
	if cond.MinCtime != nil && entry.Ctime < *cond.MinCtime {
		return false
	}
	if cond.MaxCtime != nil && entry.Ctime > *cond.MaxCtime {
		return false
	}
	return true
}

// matchPath checks if entry matches path condition
func (e *Engine) matchPath(cond *models.RuleCondition, entry *models.Entry) (bool, error) {
	if cond.PathContains != nil {
		if !strings.Contains(entry.Path, *cond.PathContains) {
			return false, nil
		}
	}
	if cond.PathPrefix != nil {
		if !strings.HasPrefix(entry.Path, *cond.PathPrefix) {
			return false, nil
		}
	}
	if cond.PathSuffix != nil {
		if !strings.HasSuffix(entry.Path, *cond.PathSuffix) {
			return false, nil
		}
	}
	if cond.PathPattern != nil {
		matched, err := regexp.MatchString(*cond.PathPattern, entry.Path)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %w", err)
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

// applyOutcome applies a rule outcome to an entry
func (e *Engine) applyOutcome(ctx context.Context, outcome *models.RuleOutcome, entry *models.Entry, executionID, selectionSetID int64) error {
	switch outcome.Type {
	case "selection_set":
		return e.applySelectionSetOutcome(outcome, entry, executionID, selectionSetID)

	case "classifier":
		return e.applyClassifierOutcome(ctx, outcome, entry, executionID, selectionSetID)

	case "chained":
		return e.applyChainedOutcome(ctx, outcome, entry, executionID, selectionSetID)

	default:
		return fmt.Errorf("unknown outcome type: %s", outcome.Type)
	}
}

// applySelectionSetOutcome applies a selection set outcome
func (e *Engine) applySelectionSetOutcome(outcome *models.RuleOutcome, entry *models.Entry, executionID, selectionSetID int64) error {
	operation := "add"
	if outcome.Operation != nil {
		operation = *outcome.Operation
	}

	var err error
	switch operation {
	case "add":
		err = e.addToSelectionSet(selectionSetID, entry.Path)
	case "remove":
		err = e.removeFromSelectionSet(selectionSetID, entry.Path)
	default:
		err = fmt.Errorf("unknown operation: %s", operation)
	}

	// Record outcome
	status := "success"
	var errorMsg *string
	if err != nil {
		status = "error"
		msg := err.Error()
		errorMsg = &msg
	}

	e.recordOutcome(executionID, selectionSetID, entry.Path, outcome.Type, nil, status, errorMsg)

	return err
}

// applyClassifierOutcome applies a classifier outcome
func (e *Engine) applyClassifierOutcome(ctx context.Context, outcome *models.RuleOutcome, entry *models.Entry, executionID, selectionSetID int64) error {
	if e.classifier == nil {
		return fmt.Errorf("classifier not available")
	}

	operation := "generate_thumbnail"
	if outcome.ClassifierOperation != nil {
		operation = *outcome.ClassifierOperation
	}

	var err error
	var outcomeData string

	switch operation {
	case "generate_thumbnail":
		// Detect media type
		mediaType := classifier.DetectMediaType(entry.Path)

		// Create temporary output path
		outputPath := fmt.Sprintf("/tmp/thumb_%d.jpg", time.Now().UnixNano())

		req := &classifier.ArtifactRequest{
			SourcePath:   entry.Path,
			OutputPath:   outputPath,
			MediaType:    mediaType,
			ArtifactType: classifier.ArtifactTypeThumbnail,
		}
		if outcome.MaxWidth != nil {
			req.MaxWidth = *outcome.MaxWidth
		} else {
			req.MaxWidth = 320
		}
		if outcome.MaxHeight != nil {
			req.MaxHeight = *outcome.MaxHeight
		} else {
			req.MaxHeight = 320
		}

		result := e.classifier.GenerateThumbnail(req)
		if result.Error != nil {
			err = result.Error
		} else if result.OutputPath != "" {
			data, _ := json.Marshal(result)
			outcomeData = string(data)
		}

	default:
		err = fmt.Errorf("unknown classifier operation: %s", operation)
	}

	// Record outcome
	status := "success"
	var errorMsg *string
	var outcomeDataPtr *string
	if err != nil {
		status = "error"
		msg := err.Error()
		errorMsg = &msg
	}
	if outcomeData != "" {
		outcomeDataPtr = &outcomeData
	}

	e.recordOutcome(executionID, selectionSetID, entry.Path, outcome.Type, outcomeDataPtr, status, errorMsg)

	// Also add to selection set for traceability
	if err == nil {
		e.addToSelectionSet(selectionSetID, entry.Path)
	}

	return err
}

// applyChainedOutcome applies a chained outcome
func (e *Engine) applyChainedOutcome(ctx context.Context, outcome *models.RuleOutcome, entry *models.Entry, executionID, selectionSetID int64) error {
	stopOnError := false
	if outcome.StopOnError != nil {
		stopOnError = *outcome.StopOnError
	}

	var errors []error
	for _, subOutcome := range outcome.Outcomes {
		// Use the parent's selection set name if sub-outcome doesn't have one
		if subOutcome.SelectionSetName == "" {
			subOutcome.SelectionSetName = outcome.SelectionSetName
		}

		err := e.applyOutcome(ctx, subOutcome, entry, executionID, selectionSetID)
		if err != nil {
			errors = append(errors, err)
			if stopOnError {
				return err
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("chained outcome had %d errors: %v", len(errors), errors)
	}

	return nil
}

// Database helper methods

func (e *Engine) getEntry(path string) (*models.Entry, error) {
	var entry models.Entry
	var parent sql.NullString

	err := e.db.QueryRow(`
		SELECT id, path, parent, size, kind, ctime, mtime, last_scanned
		FROM entries WHERE path = ?
	`, path).Scan(
		&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind,
		&entry.Ctime, &entry.Mtime, &entry.LastScanned,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if parent.Valid {
		entry.Parent = &parent.String
	}

	return &entry, nil
}

func (e *Engine) getEnabledRules() ([]*models.Rule, error) {
	rows, err := e.db.Query(`
		SELECT id, name, description, enabled, priority, condition_json, outcome_json, created_at, updated_at
		FROM rules
		WHERE enabled = 1
		ORDER BY priority DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*models.Rule
	for rows.Next() {
		var rule models.Rule
		var description sql.NullString

		if err := rows.Scan(
			&rule.ID, &rule.Name, &description, &rule.Enabled, &rule.Priority,
			&rule.ConditionJSON, &rule.OutcomeJSON, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if description.Valid {
			rule.Description = &description.String
		}

		rules = append(rules, &rule)
	}

	return rules, rows.Err()
}

func (e *Engine) ensureSelectionSet(name string) (int64, error) {
	// Check if selection set exists
	var id int64
	err := e.db.QueryRow(`SELECT id FROM selection_sets WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	// Create selection set
	result, err := e.db.Exec(`
		INSERT INTO selection_sets (name, description, criteria_type)
		VALUES (?, ?, ?)
	`, name, "Auto-created by rule execution", "tool_query")
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (e *Engine) createExecution(ruleID, selectionSetID int64) (int64, error) {
	result, err := e.db.Exec(`
		INSERT INTO rule_executions (rule_id, selection_set_id, entries_matched, entries_processed, status)
		VALUES (?, ?, 1, 0, 'running')
	`, ruleID, selectionSetID)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (e *Engine) updateExecution(executionID int64, status string, durationMs int, errorMsg *string) error {
	_, err := e.db.Exec(`
		UPDATE rule_executions
		SET status = ?, entries_processed = 1, duration_ms = ?, error_message = ?
		WHERE id = ?
	`, status, durationMs, errorMsg, executionID)
	return err
}

func (e *Engine) recordOutcome(executionID, selectionSetID int64, entryPath, outcomeType string, outcomeData *string, status string, errorMsg *string) error {
	_, err := e.db.Exec(`
		INSERT INTO rule_outcomes (execution_id, selection_set_id, entry_path, outcome_type, outcome_data, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, executionID, selectionSetID, entryPath, outcomeType, outcomeData, status, errorMsg)
	return err
}

func (e *Engine) addToSelectionSet(setID int64, path string) error {
	_, err := e.db.Exec(`
		INSERT OR IGNORE INTO selection_set_entries (set_id, entry_path)
		VALUES (?, ?)
	`, setID, path)
	return err
}

func (e *Engine) removeFromSelectionSet(setID int64, path string) error {
	_, err := e.db.Exec(`
		DELETE FROM selection_set_entries
		WHERE set_id = ? AND entry_path = ?
	`, setID, path)
	return err
}

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
