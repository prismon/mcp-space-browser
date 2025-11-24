package plans

import (
	"fmt"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/sirupsen/logrus"
)

// Executor runs plans and coordinates execution
type Executor struct {
	db        *database.DiskDB
	evaluator *ConditionEvaluator
	applier   *OutcomeApplier
	logger    *logrus.Entry
}

// NewExecutor creates a new plan executor
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
	e.logger.WithFields(logrus.Fields{
		"plan": plan.Name,
		"mode": plan.Mode,
	}).Info("Starting plan execution")

	startTime := time.Now()

	// Create execution record
	exec, err := e.db.CreatePlanExecution(plan.ID, plan.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Execute plan (capture any errors)
	execErr := e.executePlan(plan, exec)

	// Update execution record
	completedAt := time.Now().Unix()
	durationMs := int(time.Since(startTime).Milliseconds())
	exec.CompletedAt = &completedAt
	exec.DurationMs = &durationMs

	if execErr != nil {
		exec.Status = "error"
		errMsg := execErr.Error()
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

	e.logger.WithFields(logrus.Fields{
		"plan":      plan.Name,
		"status":    exec.Status,
		"matched":   exec.EntriesMatched,
		"processed": exec.EntriesProcessed,
		"duration":  durationMs,
	}).Info("Plan execution completed")

	return exec, execErr
}

func (e *Executor) executePlan(plan *models.Plan, exec *models.PlanExecution) error {
	// Step 1: Resolve sources to get candidate entries
	entries, err := e.resolveSources(plan.Sources)
	if err != nil {
		return fmt.Errorf("failed to resolve sources: %w", err)
	}
	exec.EntriesProcessed = len(entries)
	e.logger.Debugf("Resolved %d entries from sources", len(entries))

	if len(entries) == 0 {
		e.logger.Info("No entries found from sources")
		return nil
	}

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
			// Check if this is a configuration error (invalid type) vs transient error
			if source.Type != "filesystem" && source.Type != "selection_set" && source.Type != "query" {
				// Invalid source type is a configuration error - fail fast
				return nil, fmt.Errorf("source[%d]: %w", i, err)
			}
			// Transient errors (path not found, etc.) - log and continue
			e.logger.Warnf("Failed to resolve source[%d]: %v", i, err)
			continue
		}

		// Deduplicate entries by path
		for _, entry := range entries {
			if entry == nil {
				continue
			}
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
		entries, err := e.getEntriesUnderPath(path)
		if err != nil {
			e.logger.Warnf("Failed to get entries for path %s: %v", path, err)
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	return allEntries, nil
}

// getEntriesUnderPath returns all entries at or under the given path
func (e *Executor) getEntriesUnderPath(path string) ([]*models.Entry, error) {
	// Get the root entry
	rootEntry, err := e.db.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get entry for path %s: %w", path, err)
	}

	// Start with the root entry
	allEntries := []*models.Entry{rootEntry}

	// Get all descendants using tree traversal
	tree, err := e.db.GetTree(path)
	if err != nil {
		// If GetTree fails, just return the root entry
		e.logger.Debugf("Could not get tree for %s, returning single entry: %v", path, err)
		return allEntries, nil
	}

	// Recursively collect all entries from the tree
	e.collectEntriesFromTree(tree, &allEntries)

	return allEntries, nil
}

// collectEntriesFromTree recursively collects entries from a tree node
func (e *Executor) collectEntriesFromTree(node *models.TreeNode, entries *[]*models.Entry) {
	if node == nil {
		return
	}

	// Add current node as an entry (skip if path is empty)
	if node.Path != "" {
		*entries = append(*entries, &models.Entry{
			Path: node.Path,
			Size: node.Size,
			Kind: node.Kind,
		})
	}

	// Recursively add children
	if node.Children != nil {
		for _, child := range node.Children {
			e.collectEntriesFromTree(child, entries)
		}
	}
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
func (e *Executor) filterEntries(entries []*models.Entry, condition *models.RuleCondition) ([]*models.Entry, error) {
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
