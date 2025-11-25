package plans

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/sirupsen/logrus"
)

// OutcomeApplier applies RuleOutcome to matched entries
type OutcomeApplier struct {
	db     *database.DiskDB
	logger *logrus.Entry
}

// NewOutcomeApplier creates a new outcome applier
func NewOutcomeApplier(db *database.DiskDB, logger *logrus.Entry) *OutcomeApplier {
	return &OutcomeApplier{
		db:     db,
		logger: logger.WithField("component", "outcome_applier"),
	}
}

// ApplyAll applies all outcomes to matched entries
func (oa *OutcomeApplier) ApplyAll(entries []*models.Entry, outcomes []models.RuleOutcome, execID, planID int64) (int, error) {
	totalApplied := 0

	for i, outcome := range outcomes {
		count, err := oa.Apply(entries, outcome, execID, planID)
		if err != nil {
			// Check if this is a configuration error (invalid type/operation)
			errStr := err.Error()
			if strings.Contains(errStr, "unknown outcome type") || strings.Contains(errStr, "invalid operation") {
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

// Apply executes a single outcome on matched entries
func (oa *OutcomeApplier) Apply(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	switch outcome.Type {
	case "resource_set":
		return oa.applyResourceSet(entries, outcome, execID, planID)
	case "classifier":
		return oa.applyClassifier(entries, outcome, execID, planID)
	case "chained":
		return oa.applyChained(entries, outcome, execID, planID)
	default:
		return 0, fmt.Errorf("unknown outcome type: %s", outcome.Type)
	}
}

func (oa *OutcomeApplier) applyResourceSet(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	if outcome.ResourceSetName == "" {
		return 0, fmt.Errorf("resourceSetName is required for resource_set outcome")
	}

	setName := outcome.ResourceSetName
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}

	// Ensure resource set exists
	set, err := oa.db.GetResourceSet(setName)
	if err != nil || set == nil {
		// Create if doesn't exist
		desc := "Auto-created by plan"
		_, createErr := oa.db.CreateResourceSet(&models.ResourceSet{
			Name:        setName,
			Description: &desc,
		})
		if createErr != nil {
			return 0, fmt.Errorf("failed to create resource set: %w", createErr)
		}
		oa.logger.Infof("Created resource set: %s", setName)
	}

	// Apply operation
	var opErr error
	operation := "add" // Default operation
	if outcome.Operation != nil {
		operation = *outcome.Operation
	}

	switch operation {
	case "add":
		opErr = oa.db.AddToResourceSet(setName, paths)
	case "remove":
		opErr = oa.db.RemoveFromResourceSet(setName, paths)
	default:
		return 0, fmt.Errorf("invalid operation: %s", operation)
	}

	if opErr != nil {
		return 0, fmt.Errorf("failed to %s entries: %w", operation, opErr)
	}

	// Record outcomes for audit
	for _, entry := range entries {
		outcomeData, _ := json.Marshal(map[string]string{
			"resource_set": setName,
			"operation":    operation,
		})

		record := &models.PlanOutcomeRecord{
			ExecutionID: execID,
			PlanID:      planID,
			EntryPath:   entry.Path,
			OutcomeType: "resource_set",
			OutcomeData: string(outcomeData),
			Status:      "success",
		}

		if err := oa.db.RecordPlanOutcome(record); err != nil {
			oa.logger.Warnf("Failed to record outcome for %s: %v", entry.Path, err)
		}
	}

	oa.logger.WithFields(logrus.Fields{
		"operation": operation,
		"set":       setName,
		"count":     len(entries),
	}).Info("Applied resource_set outcome")

	return len(entries), nil
}

func (oa *OutcomeApplier) applyClassifier(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	// TODO: Implement classifier outcomes (thumbnail generation, metadata extraction, etc.)
	if outcome.ClassifierOperation != nil {
		oa.logger.Warnf("Classifier outcomes not yet implemented: %s", *outcome.ClassifierOperation)
	}
	return 0, nil
}

func (oa *OutcomeApplier) applyChained(entries []*models.Entry, outcome models.RuleOutcome, execID, planID int64) (int, error) {
	if outcome.Outcomes == nil || len(outcome.Outcomes) == 0 {
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
