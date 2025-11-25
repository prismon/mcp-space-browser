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

	// Create a plan
	minSize := int64(1024 * 100) // 100KB
	operation := "add"
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
				Type:            "resource_set",
				ResourceSetName: "test-results",
				Operation:       &operation,
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

	operation := "add"
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
				Type:            "resource_set",
				ResourceSetName: "all-files",
				Operation:       &operation,
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

	operation := "add"
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
				Type:            "resource_set",
				ResourceSetName: "auto-created", // Doesn't exist yet
				Operation:       &operation,
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

// Test Negative Cases

func TestExecutePlan_InvalidSource(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	operation := "add"
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
				Type:            "resource_set",
				ResourceSetName: "test",
				Operation:       &operation,
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

	operation := "add"
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
				Type:            "resource_set",
				ResourceSetName: "test",
				Operation:       &operation,
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
	operation := "add"
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
				Type:            "resource_set",
				ResourceSetName: "test",
				Operation:       &operation,
			},
		},
	}

	execution, err := executor.Execute(plan)
	require.NoError(t, err)
	assert.Equal(t, "partial", execution.Status) // Partial because no matches
	assert.Equal(t, 0, execution.EntriesMatched)
	assert.Equal(t, 0, execution.OutcomesApplied)
}

func TestExecutePlan_InvalidOperation(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	logger := logrus.New().WithField("test", "executor")
	executor := NewExecutor(db, logger)

	tmpDir := t.TempDir()
	createTestEntries(t, db, tmpDir)

	invalidOp := "invalid_operation"
	plan := &models.Plan{
		ID:     1,
		Name:   "invalid-op-plan",
		Mode:   "oneshot",
		Status: "active",
		Sources: []models.PlanSource{
			{Type: "filesystem", Paths: []string{tmpDir}},
		},
		Outcomes: []models.RuleOutcome{
			{
				Type:            "resource_set",
				ResourceSetName: "test",
				Operation:       &invalidOp, // Invalid operation
			},
		},
	}

	execution, err := executor.Execute(plan)
	// Configuration errors should return error and execution record
	require.Error(t, err) // Invalid operation is a configuration error
	assert.NotNil(t, execution)
	assert.Equal(t, "error", execution.Status)
	assert.NotNil(t, execution.ErrorMessage)
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
