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
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/plans"
	"github.com/sirupsen/logrus"
)

// Engine is responsible for evaluating rules and applying outcomes
type Engine struct {
	db          *sql.DB
	diskDB      *database.DiskDB
	classifier  classifier.Classifier
	toolInvoker *plans.ToolInvoker
	log         *logrus.Entry
}

// NewEngine creates a new rule execution engine
func NewEngine(db *sql.DB, diskDB *database.DiskDB, clf classifier.Classifier) *Engine {
	log := logrus.WithField("component", "rules-engine")
	return &Engine{
		db:          db,
		diskDB:      diskDB,
		classifier:  clf,
		toolInvoker: plans.NewToolInvoker(diskDB, nil, log),
		log:         log,
	}
}

// SetProcessor sets the classifier processor on the tool invoker
func (e *Engine) SetProcessor(processor *classifier.Processor) {
	e.toolInvoker.SetProcessor(processor)
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
		"path":      path,
		"ruleCount": len(rules),
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

	// Validate outcome
	if err := outcome.Validate(); err != nil {
		return fmt.Errorf("invalid outcome: %w", err)
	}

	// Apply outcome using tool invoker
	err = e.applyOutcome(ctx, &outcome, entry)

	// Log result
	duration := time.Since(startTime).Milliseconds()
	if err != nil {
		e.log.WithError(err).WithFields(logrus.Fields{
			"rule":     rule.Name,
			"path":     entry.Path,
			"duration": duration,
		}).Error("Rule outcome failed")
	} else {
		e.log.WithFields(logrus.Fields{
			"rule":     rule.Name,
			"path":     entry.Path,
			"tool":     outcome.Tool,
			"duration": duration,
		}).Info("Rule outcome applied successfully")
	}

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

// applyOutcome applies a rule outcome using the tool invoker
func (e *Engine) applyOutcome(ctx context.Context, outcome *models.RuleOutcome, entry *models.Entry) error {
	// Handle chained outcomes
	if outcome.IsChained() {
		return e.applyChainedOutcome(ctx, outcome, entry)
	}

	// Invoke the tool
	return e.toolInvoker.InvokeTool(ctx, outcome.Tool, outcome.Arguments, entry)
}

// applyChainedOutcome applies a chained outcome
func (e *Engine) applyChainedOutcome(ctx context.Context, outcome *models.RuleOutcome, entry *models.Entry) error {
	stopOnError := false
	if outcome.StopOnError != nil {
		stopOnError = *outcome.StopOnError
	}

	var errors []error
	for _, subOutcome := range outcome.Outcomes {
		err := e.applyOutcome(ctx, subOutcome, entry)
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

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
