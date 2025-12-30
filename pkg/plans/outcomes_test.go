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

func setupOutcomeTest(t *testing.T) (*OutcomeApplier, *database.DiskDB) {
	os.Setenv("GO_ENV", "test")

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)

	logger := logrus.New().WithField("test", "outcomes")
	oa := NewOutcomeApplier(db, logger)

	return oa, db
}

func TestOutcomeApplier_Apply_UnknownTool(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{Tool: "unknown-tool"}

	// Unknown tools result in 0 entries processed (logged as warnings)
	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOutcomeApplier_ApplyAll_Empty(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcomes := []models.RuleOutcome{}

	count, err := oa.ApplyAll(entries, outcomes, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOutcomeApplier_ApplyAll_ConfigurationError(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcomes := []models.RuleOutcome{
		{Tool: "unknown-tool"}, // Unknown tool results in 0 processed
	}

	// Unknown tools don't return errors - they just process 0 entries
	count, err := oa.ApplyAll(entries, outcomes, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOutcomeApplier_Apply_ResourceSetModify_MissingName(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			// Missing "name"
			"operation": "add",
		},
	}

	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires 'name'")
}

func TestOutcomeApplier_Apply_ResourceSetModify_InvalidOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Create a resource set first
	_, err := db.CreateResourceSet(&models.ResourceSet{Name: "test-set"})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name":      "test-set",
			"operation": "invalid_op",
		},
	}

	_, err = oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid operation")
}

func TestOutcomeApplier_Apply_ResourceSetModify_AddOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name":      "test-add-set",
			"operation": "add",
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify entry was added
	setEntries, err := db.GetResourceSetEntries("test-add-set")
	require.NoError(t, err)
	assert.Len(t, setEntries, 1)
}

func TestOutcomeApplier_Apply_ResourceSetModify_RemoveOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry and add to resource set first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "test-remove-set"})
	require.NoError(t, err)

	err = db.AddToResourceSet("test-remove-set", []string{"/test/file.txt"})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name":      "test-remove-set",
			"operation": "remove",
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOutcomeApplier_Apply_ResourceSetModify_DefaultOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "test-default-set",
			// No operation specified - should default to "add"
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOutcomeApplier_Apply_ClassifierProcess(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "classifier-process",
		Arguments: map[string]interface{}{
			"operation": "thumbnail",
		},
	}

	// Classifier outcomes without a processor should return 0 without error
	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOutcomeApplier_Apply_Chained_Empty(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Outcomes: []*models.RuleOutcome{}, // Empty outcomes - not treated as chained
	}

	// An empty Outcomes array doesn't satisfy IsChained() (len must be > 0)
	// so it's validated as a regular outcome and fails because no tool is specified
	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must specify 'tool'")
}

func TestOutcomeApplier_Apply_Chained_Success(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Outcomes: []*models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "chained-set-1",
					"operation": "add",
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "chained-set-2",
					"operation": "add",
				},
			},
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, count) // 1 entry added to 2 sets
}

func TestOutcomeApplier_Apply_Chained_StopOnError(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	stopOnError := true
	outcome := models.RuleOutcome{
		StopOnError: &stopOnError,
		Outcomes: []*models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test-set",
					"operation": "invalid_op", // This will fail
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test-set-2",
					"operation": "add", // Should not be reached
				},
			},
		},
	}

	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err) // Should error because stopOnError is true
}

func TestOutcomeApplier_Apply_Chained_ContinueOnError(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	stopOnError := false
	outcome := models.RuleOutcome{
		StopOnError: &stopOnError,
		Outcomes: []*models.RuleOutcome{
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test-set-1",
					"operation": "invalid_op", // This will fail
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      "test-set-2",
					"operation": "add", // Should still be executed
				},
			},
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err) // Should not error because stopOnError is false
	assert.Equal(t, 1, count) // Only the second outcome succeeded
}

func TestOutcomeApplier_ApplyAll_MultipleOutcomes(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcomes := []models.RuleOutcome{
		{
			Tool: "resource-set-modify",
			Arguments: map[string]interface{}{
				"name":      "set-1",
				"operation": "add",
			},
		},
		{
			Tool: "resource-set-modify",
			Arguments: map[string]interface{}{
				"name":      "set-2",
				"operation": "add",
			},
		},
	}

	count, err := oa.ApplyAll(entries, outcomes, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestOutcomeApplier_Apply_ResourceSetCreate(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-create",
		Arguments: map[string]interface{}{
			"name":        "new-set",
			"description": "A test resource set",
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify set was created
	set, err := db.GetResourceSet("new-set")
	require.NoError(t, err)
	assert.NotNil(t, set)
	assert.Equal(t, "new-set", set.Name)
}

func TestOutcomeApplier_Apply_ResourceSetAddChild(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Create parent and child sets
	_, err := db.CreateResourceSet(&models.ResourceSet{Name: "parent-set"})
	require.NoError(t, err)
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "child-set"})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-add-child",
		Arguments: map[string]interface{}{
			"parent": "parent-set",
			"child":  "child-set",
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify edge was created
	children, err := db.GetResourceSetChildren("parent-set")
	require.NoError(t, err)
	assert.Len(t, children, 1)
	assert.Equal(t, "child-set", children[0].Name)
}

func TestOutcomeApplier_Apply_ResourceSetRemoveChild(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Create parent and child sets with edge
	_, err := db.CreateResourceSet(&models.ResourceSet{Name: "parent-set"})
	require.NoError(t, err)
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "child-set"})
	require.NoError(t, err)
	err = db.AddResourceSetEdge("parent-set", "child-set")
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Tool: "resource-set-remove-child",
		Arguments: map[string]interface{}{
			"parent": "parent-set",
			"child":  "child-set",
		},
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify edge was removed
	children, err := db.GetResourceSetChildren("parent-set")
	require.NoError(t, err)
	assert.Len(t, children, 0)
}

func TestOutcomeApplier_Apply_MissingTool(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		// No tool specified, not chained
		Arguments: map[string]interface{}{
			"name": "test-set",
		},
	}

	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must specify 'tool'")
}
