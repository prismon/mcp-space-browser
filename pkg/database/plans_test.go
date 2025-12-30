package database

import (
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a valid outcome
func createTestOutcome() models.RuleOutcome {
	return models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name":      "test-set",
			"operation": "add",
		},
	}
}

func TestCreatePlan(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	t.Run("valid plan", func(t *testing.T) {
		plan := &models.Plan{
			Name:   "test-plan",
			Mode:   "oneshot",
			Status: "active",
			Sources: []models.PlanSource{
				{Type: "filesystem", Paths: []string{"/test/path"}},
			},
			Outcomes: []models.RuleOutcome{createTestOutcome()},
		}
		err := db.CreatePlan(plan)
		assert.NoError(t, err)
		assert.Greater(t, plan.ID, int64(0))
	})

	t.Run("duplicate name fails", func(t *testing.T) {
		plan := &models.Plan{
			Name:   "test-plan",
			Mode:   "oneshot",
			Status: "active",
			Sources: []models.PlanSource{
				{Type: "filesystem", Paths: []string{"/test/path"}},
			},
			Outcomes: []models.RuleOutcome{createTestOutcome()},
		}
		err := db.CreatePlan(plan)
		assert.Error(t, err)
	})

	t.Run("invalid plan fails validation", func(t *testing.T) {
		plan := &models.Plan{
			Name:   "",
			Mode:   "oneshot",
			Status: "active",
		}
		err := db.CreatePlan(plan)
		assert.Error(t, err)
	})
}

func TestGetPlan(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a plan
	desc := "Test description"
	plan := &models.Plan{
		Name:        "get-plan",
		Description: &desc,
		Mode:        "continuous",
		Status:      "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/test"}},
		},
		Outcomes: []models.RuleOutcome{createTestOutcome()},
	}
	err = db.CreatePlan(plan)
	require.NoError(t, err)

	t.Run("existing plan", func(t *testing.T) {
		retrieved, err := db.GetPlan("get-plan")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "get-plan", retrieved.Name)
		assert.Equal(t, "continuous", retrieved.Mode)
		assert.NotNil(t, retrieved.Description)
		assert.Equal(t, desc, *retrieved.Description)
	})

	t.Run("non-existent plan", func(t *testing.T) {
		_, err := db.GetPlan("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plan not found")
	})
}

func TestListPlans(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple plans
	for i := 0; i < 3; i++ {
		plan := &models.Plan{
			Name:   "list-plan-" + string(rune('a'+i)),
			Mode:   "oneshot",
			Status: "active",
			Sources: []models.PlanSource{
				{Type: "filesystem", Paths: []string{"/test"}},
			},
			Outcomes: []models.RuleOutcome{createTestOutcome()},
		}
		err := db.CreatePlan(plan)
		require.NoError(t, err)
	}

	plans, err := db.ListPlans()
	assert.NoError(t, err)
	assert.Len(t, plans, 3)
}

func TestUpdatePlan(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a plan first
	plan := &models.Plan{
		Name:   "update-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/test"}},
		},
		Outcomes: []models.RuleOutcome{createTestOutcome()},
	}
	err = db.CreatePlan(plan)
	require.NoError(t, err)

	t.Run("update existing plan", func(t *testing.T) {
		plan.Status = "paused"
		plan.Mode = "continuous"
		err := db.UpdatePlan(plan)
		assert.NoError(t, err)

		// Verify update
		retrieved, err := db.GetPlan("update-plan")
		assert.NoError(t, err)
		assert.Equal(t, "paused", retrieved.Status)
		assert.Equal(t, "continuous", retrieved.Mode)
	})

	t.Run("update non-existent plan", func(t *testing.T) {
		nonExistent := &models.Plan{
			Name:   "does-not-exist",
			Mode:   "oneshot",
			Status: "active",
			Sources: []models.PlanSource{
				{Type: "filesystem", Paths: []string{"/test"}},
			},
			Outcomes: []models.RuleOutcome{createTestOutcome()},
		}
		err := db.UpdatePlan(nonExistent)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plan not found")
	})
}

func TestDeletePlan(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a plan first
	plan := &models.Plan{
		Name:   "delete-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/test"}},
		},
		Outcomes: []models.RuleOutcome{createTestOutcome()},
	}
	err = db.CreatePlan(plan)
	require.NoError(t, err)

	t.Run("delete existing plan", func(t *testing.T) {
		err := db.DeletePlan("delete-plan")
		assert.NoError(t, err)

		// Verify deleted
		_, err = db.GetPlan("delete-plan")
		assert.Error(t, err)
	})

	t.Run("delete non-existent plan", func(t *testing.T) {
		err := db.DeletePlan("does-not-exist")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plan not found")
	})
}

func TestPlanExecution(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a plan first
	plan := &models.Plan{
		Name:   "exec-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/test"}},
		},
		Outcomes: []models.RuleOutcome{createTestOutcome()},
	}
	err = db.CreatePlan(plan)
	require.NoError(t, err)

	t.Run("create execution", func(t *testing.T) {
		exec, err := db.CreatePlanExecution(plan.ID, plan.Name)
		assert.NoError(t, err)
		assert.NotNil(t, exec)
		assert.Greater(t, exec.ID, int64(0))
		assert.Equal(t, plan.ID, exec.PlanID)
		assert.Equal(t, "running", exec.Status)
	})

	t.Run("update execution", func(t *testing.T) {
		exec, _ := db.CreatePlanExecution(plan.ID, plan.Name)

		now := time.Now().Unix()
		duration := 1000
		errorMsg := "test error"
		exec.CompletedAt = &now
		exec.DurationMs = &duration
		exec.EntriesProcessed = 100
		exec.EntriesMatched = 50
		exec.OutcomesApplied = 25
		exec.Status = "error"
		exec.ErrorMessage = &errorMsg

		err := db.UpdatePlanExecution(exec)
		assert.NoError(t, err)
	})

	t.Run("get executions", func(t *testing.T) {
		executions, err := db.GetPlanExecutions(plan.Name, 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(executions), 2)
	})
}

func TestUpdatePlanLastRun(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a plan first
	plan := &models.Plan{
		Name:   "lastrun-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/test"}},
		},
		Outcomes: []models.RuleOutcome{createTestOutcome()},
	}
	err = db.CreatePlan(plan)
	require.NoError(t, err)

	err = db.UpdatePlanLastRun(plan.ID)
	assert.NoError(t, err)

	// Verify last_run_at was updated
	retrieved, err := db.GetPlan(plan.Name)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved.LastRunAt)
}

func TestRecordPlanOutcome(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a plan and execution
	plan := &models.Plan{
		Name:   "outcome-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/test"}},
		},
		Outcomes: []models.RuleOutcome{createTestOutcome()},
	}
	err = db.CreatePlan(plan)
	require.NoError(t, err)

	exec, err := db.CreatePlanExecution(plan.ID, plan.Name)
	require.NoError(t, err)

	// Create an entry for the outcome
	entry := &models.Entry{
		Path: "/test/file.txt",
		Size: 100,
		Kind: "file",
	}
	db.InsertOrUpdate(entry)

	record := &models.PlanOutcomeRecord{
		ExecutionID: exec.ID,
		PlanID:      plan.ID,
		EntryPath:   "/test/file.txt",
		OutcomeType: "selection_set",
		Status:      "success",
	}

	err = db.RecordPlanOutcome(record)
	assert.NoError(t, err)
	assert.Greater(t, record.ID, int64(0))
}
