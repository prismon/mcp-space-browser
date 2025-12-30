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

// Test Positive Cases

func TestExecutePlan_Success(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	// Create a test directory structure in DB
	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	// Create a resource set for outcomes
	desc := "Test resource set"
	_, err = db.CreateResourceSet(&models.ResourceSet{
		Name:        "test-results",
		Description: &desc,
	})
	require.NoError(t, err)

	// Create a plan with tool-based outcome
	minSize := int64(1024 * 100) // 100KB
	plan := &models.Plan{
		ID:     1,
		Name:   "test-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{
				Type:  "filesystem",
				Paths: []string{tmpDir},
			},
		},
		Conditions: &models.RuleCondition{
			Type:    "size",
			MinSize: &minSize,
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test-results",
					"operation": "add",
				},
			},
		},
	}

	// Execute plan
	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, "success", execution.Status)
	assert.Greater(t, execution.EntriesProcessed, 0)
	assert.Greater(t, execution.EntriesMatched, 0)
	assert.Greater(t, execution.OutcomesApplied, 0)
}

func TestExecutePlan_NoConditions(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	plan := &models.Plan{
		ID:         1,
		Name:       "all-files-plan",
		Mode:       "oneshot",
		Status:     "active",
		Conditions: nil, // No conditions = match all
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{tmpDir}},
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "all-files",
					"operation": "add",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	assert.Equal(t, "success", execution.Status)
	// All entries should match when no conditions
	assert.Equal(t, execution.EntriesProcessed, execution.EntriesMatched)
}

func TestExecutePlan_AutoCreateResourceSet(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	plan := &models.Plan{
		ID:     1,
		Name:   "auto-create-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{tmpDir}},
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "auto-created", // Doesn't exist yet
					"operation": "add",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	assert.Equal(t, "success", execution.Status)

	// Verify resource set was auto-created
	set, err := db.GetResourceSet("auto-created")
	require.NoError(t, err)
	assert.NotNil(t, set)
}

func TestExecutePlan_ProjectSource(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	// Plan with project source - gets ALL entries
	plan := &models.Plan{
		ID:     1,
		Name:   "project-source-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "project"}, // Project source - all entries
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "all-project-files",
					"operation": "add",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	assert.Equal(t, "success", execution.Status)
	// Should match all entries in the database
	assert.Equal(t, 4, execution.EntriesProcessed) // 1 dir + 3 files
}

// Test Negative Cases

func TestExecutePlan_InvalidSource(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	plan := &models.Plan{
		ID:     1,
		Name:   "invalid-source-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "invalid_type"}, // Invalid source type
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test",
					"operation": "add",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	// Configuration errors should return error and execution record
	require.Error(t, err) // Invalid source type is a configuration error
	assert.NotNil(t, execution)
	assert.Equal(t, "error", execution.Status)
	assert.NotNil(t, execution.ErrorMessage)
}

func TestExecutePlan_NonexistentPath(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	plan := &models.Plan{
		ID:     1,
		Name:   "nonexistent-path-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{"/nonexistent/path"}},
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test",
					"operation": "add",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	// Should be partial (no matches) or error
	assert.Contains(t, []string{"partial", "error"}, execution.Status)
}

func TestExecutePlan_NoMatchingEntries(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	// Condition that won't match anything
	minSize := int64(1024 * 1024 * 1024 * 100) // 100GB - way too large
	plan := &models.Plan{
		ID:     1,
		Name:   "no-match-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{tmpDir}},
		},
		Conditions: &models.RuleCondition{
			Type:    "size",
			MinSize: &minSize,
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test",
					"operation": "add",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	assert.Equal(t, "partial", execution.Status) // Partial because no matches
	assert.Equal(t, 0, execution.EntriesMatched)
	assert.Equal(t, 0, execution.OutcomesApplied)
}

func TestExecutePlan_InvalidTool(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	plan := &models.Plan{
		ID:     1,
		Name:   "invalid-tool-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{tmpDir}},
		},
		Outcomes: []models.RuleOutcome{
			{
				Tool: "nonexistent-tool", // Invalid tool
				Arguments: map[string]interface{}{
					"param": "value",
				},
			},
		},
	}

	execution, err := executor.Execute(plan)
	// Invalid tools are logged as warnings but don't fail the plan
	// The plan succeeds but with 0 outcomes applied
	require.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, "success", execution.Status) // Still "success" because entries were matched
	assert.Equal(t, 0, execution.OutcomesApplied) // But no outcomes could be applied
}

// Helper functions

func createTestEntries(t *testing.T, db *database.DiskDB, baseDir string) {
	// Insert the base directory itself
	err := db.InsertOrUpdate(&models.Entry{
		Path: baseDir,
		Size: 0,
		Kind: "directory",
	})
	require.NoError(t, err)

	// Create test files
	files := []struct {
		name string
		size int64
	}{
		{"small.txt", 100},
		{"medium.dat", 1024 * 200}, // 200KB
		{"large.bin", 1024 * 1024}, // 1MB
	}

	for _, f := range files {
		path := baseDir + "/" + f.name

		// Create the file
		file, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, file.Truncate(f.size))
		file.Close()

		// Insert into database
		stat, err := os.Stat(path)
		require.NoError(t, err)

		parent := baseDir
		entry := &models.Entry{
			Path:   path,
			Parent: &parent,
			Size:   stat.Size(),
			Kind:   "file",
		}
		err = db.InsertOrUpdate(entry)
		require.NoError(t, err)
	}
}
