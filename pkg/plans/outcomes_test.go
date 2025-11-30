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

func TestOutcomeApplier_Apply_UnknownType(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{Type: "unknown_type"}

	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown outcome type")
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
		{Type: "unknown_type"}, // Configuration error
	}

	_, err := oa.ApplyAll(entries, outcomes, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown outcome type")
}

func TestOutcomeApplier_Apply_SelectionSet_MissingName(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Type:             "selection_set",
		SelectionSetName: "", // Missing name
	}

	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "selectionSetName is required")
}

func TestOutcomeApplier_Apply_SelectionSet_InvalidOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Create a selection set first
	_, err := db.CreateSelectionSet(&models.SelectionSet{Name: "test-set"})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	invalidOp := "invalid_op"
	outcome := models.RuleOutcome{
		Type:             "selection_set",
		SelectionSetName: "test-set",
		Operation:        &invalidOp,
	}

	_, err = oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid operation")
}

func TestOutcomeApplier_Apply_SelectionSet_AddOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	addOp := "add"
	outcome := models.RuleOutcome{
		Type:             "selection_set",
		SelectionSetName: "test-add-set",
		Operation:        &addOp,
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify entry was added
	setEntries, err := db.GetSelectionSetEntries("test-add-set")
	require.NoError(t, err)
	assert.Len(t, setEntries, 1)
}

func TestOutcomeApplier_Apply_SelectionSet_RemoveOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry and add to selection set first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	_, err = db.CreateSelectionSet(&models.SelectionSet{Name: "test-remove-set"})
	require.NoError(t, err)

	err = db.AddToSelectionSet("test-remove-set", []string{"/test/file.txt"})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	removeOp := "remove"
	outcome := models.RuleOutcome{
		Type:             "selection_set",
		SelectionSetName: "test-remove-set",
		Operation:        &removeOp,
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOutcomeApplier_Apply_SelectionSet_DefaultOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Type:             "selection_set",
		SelectionSetName: "test-default-set",
		// No operation specified - should default to "add"
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOutcomeApplier_Apply_Classifier(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	classifierOp := "thumbnail"
	outcome := models.RuleOutcome{
		Type:                "classifier",
		ClassifierOperation: &classifierOp,
	}

	// Classifier outcomes are not yet implemented, should return 0 without error
	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOutcomeApplier_Apply_Classifier_NoOperation(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Type: "classifier",
		// No operation specified
	}

	count, err := oa.Apply(entries, outcome, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOutcomeApplier_Apply_Chained_Empty(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	outcome := models.RuleOutcome{
		Type:     "chained",
		Outcomes: nil, // Empty/nil outcomes
	}

	_, err := oa.Apply(entries, outcome, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chained outcome requires sub-outcomes")
}

func TestOutcomeApplier_Apply_Chained_Success(t *testing.T) {
	oa, db := setupOutcomeTest(t)
	defer db.Close()

	// Insert test entry in database first
	err := db.InsertOrUpdate(&models.Entry{Path: "/test/file.txt", Kind: "file", Size: 100})
	require.NoError(t, err)

	entries := []*models.Entry{{Path: "/test/file.txt"}}
	addOp := "add"
	outcome := models.RuleOutcome{
		Type: "chained",
		Outcomes: []*models.RuleOutcome{
			{
				Type:             "selection_set",
				SelectionSetName: "chained-set-1",
				Operation:        &addOp,
			},
			{
				Type:             "selection_set",
				SelectionSetName: "chained-set-2",
				Operation:        &addOp,
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
	addOp := "add"
	invalidOp := "invalid_op"
	outcome := models.RuleOutcome{
		Type:        "chained",
		StopOnError: &stopOnError,
		Outcomes: []*models.RuleOutcome{
			{
				Type:             "selection_set",
				SelectionSetName: "test-set",
				Operation:        &invalidOp, // This will fail
			},
			{
				Type:             "selection_set",
				SelectionSetName: "test-set-2",
				Operation:        &addOp, // Should not be reached
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
	addOp := "add"
	invalidOp := "invalid_op"
	outcome := models.RuleOutcome{
		Type:        "chained",
		StopOnError: &stopOnError,
		Outcomes: []*models.RuleOutcome{
			{
				Type:             "selection_set",
				SelectionSetName: "test-set-1",
				Operation:        &invalidOp, // This will fail
			},
			{
				Type:             "selection_set",
				SelectionSetName: "test-set-2",
				Operation:        &addOp, // Should still be executed
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
	addOp := "add"
	outcomes := []models.RuleOutcome{
		{
			Type:             "selection_set",
			SelectionSetName: "set-1",
			Operation:        &addOp,
		},
		{
			Type:             "selection_set",
			SelectionSetName: "set-2",
			Operation:        &addOp,
		},
	}

	count, err := oa.ApplyAll(entries, outcomes, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
