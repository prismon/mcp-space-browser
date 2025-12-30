package plans

import (
	"context"
	"fmt"
	"sync"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var triggerLog *logrus.Entry

func init() {
	triggerLog = logger.WithName("lifecycle_trigger")
}

// LifecycleTrigger handles triggering lifecycle plans based on events
type LifecycleTrigger struct {
	db       *database.DiskDB
	executor *Executor
	mu       sync.RWMutex
	enabled  bool
	logger   *logrus.Entry
}

// NewLifecycleTrigger creates a new lifecycle trigger
func NewLifecycleTrigger(db *database.DiskDB, executor *Executor) *LifecycleTrigger {
	return &LifecycleTrigger{
		db:       db,
		executor: executor,
		enabled:  true,
		logger:   triggerLog,
	}
}

// SetEnabled enables or disables the trigger
func (lt *LifecycleTrigger) SetEnabled(enabled bool) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.enabled = enabled
	lt.logger.WithField("enabled", enabled).Info("Lifecycle trigger state changed")
}

// IsEnabled returns whether the trigger is enabled
func (lt *LifecycleTrigger) IsEnabled() bool {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	return lt.enabled
}

// TriggerOnAdd triggers the "on_add" plans for the given entries
func (lt *LifecycleTrigger) TriggerOnAdd(ctx context.Context, entries []*models.Entry) error {
	return lt.triggerPlans(ctx, "on_add", entries)
}

// TriggerOnRemove triggers the "on_remove" plans for the given entries
func (lt *LifecycleTrigger) TriggerOnRemove(ctx context.Context, entries []*models.Entry) error {
	return lt.triggerPlans(ctx, "on_remove", entries)
}

// TriggerOnRefresh triggers the "on_refresh" plans for the given entries
func (lt *LifecycleTrigger) TriggerOnRefresh(ctx context.Context, entries []*models.Entry) error {
	return lt.triggerPlans(ctx, "on_refresh", entries)
}

// triggerPlans finds and executes all plans with the given trigger type
func (lt *LifecycleTrigger) triggerPlans(ctx context.Context, triggerType string, entries []*models.Entry) error {
	if !lt.IsEnabled() {
		lt.logger.Debug("Lifecycle trigger is disabled, skipping")
		return nil
	}

	if len(entries) == 0 {
		return nil
	}

	lt.logger.WithFields(logrus.Fields{
		"trigger": triggerType,
		"entries": len(entries),
	}).Debug("Triggering lifecycle plans")

	// Get all active plans with this trigger
	plans, err := lt.db.GetPlansByTrigger(triggerType)
	if err != nil {
		return fmt.Errorf("failed to get plans for trigger %s: %w", triggerType, err)
	}

	if len(plans) == 0 {
		lt.logger.WithField("trigger", triggerType).Debug("No active plans found for trigger")
		return nil
	}

	// Execute each plan
	var execErrors []error
	for _, plan := range plans {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lt.logger.WithFields(logrus.Fields{
			"plan":    plan.Name,
			"trigger": triggerType,
			"entries": len(entries),
		}).Info("Executing lifecycle plan")

		exec, err := lt.executor.ExecuteForEntries(plan, entries)
		if err != nil {
			lt.logger.WithError(err).WithField("plan", plan.Name).Error("Failed to execute lifecycle plan")
			execErrors = append(execErrors, fmt.Errorf("plan %s: %w", plan.Name, err))
			continue
		}

		if exec != nil {
			lt.logger.WithFields(logrus.Fields{
				"plan":     plan.Name,
				"status":   exec.Status,
				"matched":  exec.EntriesMatched,
				"applied":  exec.OutcomesApplied,
				"duration": exec.DurationMs,
			}).Info("Lifecycle plan execution completed")
		}
	}

	if len(execErrors) > 0 {
		return fmt.Errorf("some plans failed: %v", execErrors)
	}

	return nil
}

// TriggerManually allows manual triggering of a specific plan for entries
func (lt *LifecycleTrigger) TriggerManually(ctx context.Context, planName string, entries []*models.Entry) (*models.PlanExecution, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries provided")
	}

	plan, err := lt.db.GetPlan(planName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	lt.logger.WithFields(logrus.Fields{
		"plan":    planName,
		"entries": len(entries),
	}).Info("Manually triggering plan")

	return lt.executor.ExecuteForEntries(plan, entries)
}

// TriggerForPaths triggers a plan for entries at the given paths
// This is useful for MCP tools that receive paths instead of entries
func (lt *LifecycleTrigger) TriggerForPaths(ctx context.Context, planName string, paths []string) (*models.PlanExecution, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths provided")
	}

	// Resolve paths to entries
	var entries []*models.Entry
	for _, path := range paths {
		entry, err := lt.db.Get(path)
		if err != nil {
			lt.logger.WithError(err).WithField("path", path).Warn("Failed to get entry for path")
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid entries found for paths")
	}

	return lt.TriggerManually(ctx, planName, entries)
}
