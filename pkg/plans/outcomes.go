package plans

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/sirupsen/logrus"
)

// OutcomeApplier applies RuleOutcome to matched entries by invoking MCP tools
type OutcomeApplier struct {
	db               *database.DiskDB
	processor        *classifier.Processor
	toolInvoker      *ToolInvoker
	evaluator        *ConditionEvaluator
	templateResolver *TemplateResolver
	logger           *logrus.Entry
}

// NewOutcomeApplier creates a new outcome applier
func NewOutcomeApplier(db *database.DiskDB, logger *logrus.Entry) *OutcomeApplier {
	oa := &OutcomeApplier{
		db:               db,
		evaluator:        NewConditionEvaluator(logger),
		templateResolver: NewTemplateResolver(),
		logger:           logger.WithField("component", "outcome_applier"),
	}
	oa.toolInvoker = NewToolInvoker(db, nil, logger)
	return oa
}

// SetProcessor sets the classifier processor for generating thumbnails.
// It also sets the database on the processor for storing artifact metadata.
func (oa *OutcomeApplier) SetProcessor(processor *classifier.Processor) {
	oa.processor = processor
	oa.toolInvoker.SetProcessor(processor)
	// Set the database so the processor can store artifact metadata
	if processor != nil && oa.db != nil {
		processor.SetDatabase(oa.db)
	}
}

// ApplyAll applies all outcomes to matched entries
func (oa *OutcomeApplier) ApplyAll(entries []*models.Entry, outcomes []models.RuleOutcome, execID, planID int64) (int, error) {
	return oa.ApplyAllWithPreferences(entries, outcomes, execID, planID, nil)
}

// ApplyAllWithPreferences applies all outcomes with plan preferences
func (oa *OutcomeApplier) ApplyAllWithPreferences(entries []*models.Entry, outcomes []models.RuleOutcome, execID, planID int64, preferences map[string]interface{}) (int, error) {
	totalApplied := 0

	// Set preferences for template resolution
	if preferences != nil {
		oa.templateResolver.SetPreferences(preferences)
	}

	for i, outcome := range outcomes {
		count, err := oa.applyWithConditions(entries, outcome, execID, planID)
		if err != nil {
			// Check if this is a configuration error
			errStr := err.Error()
			if strings.Contains(errStr, "not supported") || strings.Contains(errStr, "must specify") {
				// Configuration errors - fail fast
				return totalApplied, fmt.Errorf("outcome[%d]: %w", i, err)
			}
			// Transient errors - log and continue
			oa.logger.Errorf("Failed to apply outcome[%d]: %v", i, err)
			continue
		}
		totalApplied += count
	}

	return totalApplied, nil
}

// applyWithConditions applies an outcome after filtering by per-outcome conditions
func (oa *OutcomeApplier) applyWithConditions(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	// Filter entries by per-outcome conditions if present
	filteredEntries := entries
	if outcome.Conditions != nil {
		var err error
		filteredEntries, err = oa.filterByConditions(entries, outcome.Conditions)
		if err != nil {
			return 0, fmt.Errorf("failed to filter by outcome conditions: %w", err)
		}
		oa.logger.Debugf("Per-outcome conditions filtered %d -> %d entries", len(entries), len(filteredEntries))

		if len(filteredEntries) == 0 {
			return 0, nil
		}
	}

	return oa.Apply(filteredEntries, outcome, execID, planID)
}

// filterByConditions filters entries by a condition
func (oa *OutcomeApplier) filterByConditions(entries []*models.Entry, condition *models.RuleCondition) ([]*models.Entry, error) {
	var matched []*models.Entry

	for _, entry := range entries {
		matches, err := oa.evaluator.Evaluate(entry, condition)
		if err != nil {
			oa.logger.Warnf("Failed to evaluate per-outcome condition for %s: %v", entry.Path, err)
			continue
		}
		if matches {
			matched = append(matched, entry)
		}
	}

	return matched, nil
}

// Apply executes a single outcome on matched entries by invoking the specified tool
func (oa *OutcomeApplier) Apply(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	// Handle chained outcomes
	if outcome.IsChained() {
		return oa.applyChained(entries, outcome, execID, planID)
	}

	// Validate outcome
	if err := outcome.Validate(); err != nil {
		return 0, err
	}

	// Use batch invocation for efficiency
	ctx := context.Background()
	count, err := oa.toolInvoker.InvokeToolBatch(ctx, outcome.Tool, outcome.Arguments, entries)
	if err != nil {
		return 0, fmt.Errorf("tool %s failed: %w", outcome.Tool, err)
	}

	// Record outcomes for audit
	oa.recordOutcomes(entries, outcome, execID, planID, count)

	oa.logger.WithFields(logrus.Fields{
		"tool":    outcome.Tool,
		"total":   len(entries),
		"applied": count,
	}).Info("Applied tool outcome")

	return count, nil
}

func (oa *OutcomeApplier) applyChained(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	if len(outcome.Outcomes) == 0 {
		return 0, fmt.Errorf("chained outcome requires sub-outcomes")
	}

	totalApplied := 0
	stopOnError := outcome.StopOnError != nil && *outcome.StopOnError

	for i, subOutcome := range outcome.Outcomes {
		count, err := oa.Apply(entries, *subOutcome, execID, planID)
		if err != nil {
			oa.logger.Errorf("Failed to apply chained outcome[%d]: %v", i, err)
			if stopOnError {
				return totalApplied, err
			}
			continue
		}
		totalApplied += count
	}

	return totalApplied, nil
}

func (oa *OutcomeApplier) recordOutcomes(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64, successCount int) {
	outcomeData, _ := json.Marshal(map[string]interface{}{
		"tool":      outcome.Tool,
		"arguments": outcome.Arguments,
	})

	for _, entry := range entries {
		record := &models.PlanOutcomeRecord{
			ExecutionID: execID,
			PlanID:      planID,
			EntryPath:   entry.Path,
			OutcomeType: outcome.Tool,
			OutcomeData: string(outcomeData),
			Status:      "success",
		}

		if err := oa.db.RecordPlanOutcome(record); err != nil {
			oa.logger.Warnf("Failed to record outcome for %s: %v", entry.Path, err)
		}
	}
}
