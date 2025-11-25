package database

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAndGetRule(t *testing.T) {
	// Set test environment
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a test rule
	condition := models.RuleCondition{
		Type:    "size",
		MinSize: int64Ptr(1048576),
	}
	conditionJSON, err := json.Marshal(condition)
	require.NoError(t, err)

	outcome := models.RuleOutcome{
		Type:                "classifier",
		ResourceSetName:     "test-thumbnails",
		ClassifierOperation: strPtr("generate_thumbnail"),
	}
	outcomeJSON, err := json.Marshal(outcome)
	require.NoError(t, err)

	desc := "Test rule"
	rule := &models.Rule{
		Name:          "test-rule",
		Description:   &desc,
		Enabled:       true,
		Priority:      10,
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
	}

	// Create the rule
	id, err := db.CreateRule(rule)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	// Retrieve the rule
	retrieved, err := db.GetRule("test-rule")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-rule", retrieved.Name)
	assert.Equal(t, "Test rule", *retrieved.Description)
	assert.True(t, retrieved.Enabled)
	assert.Equal(t, 10, retrieved.Priority)

	// Parse and verify condition
	parsedCondition, err := ParseRuleCondition(retrieved.ConditionJSON)
	require.NoError(t, err)
	assert.Equal(t, "size", parsedCondition.Type)
	assert.Equal(t, int64(1048576), *parsedCondition.MinSize)

	// Parse and verify outcome
	parsedOutcome, err := ParseRuleOutcome(retrieved.OutcomeJSON)
	require.NoError(t, err)
	assert.Equal(t, "classifier", parsedOutcome.Type)
	assert.Equal(t, "test-thumbnails", parsedOutcome.ResourceSetName)
}

func TestListRules(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple rules with different priorities
	rules := []struct {
		name     string
		enabled  bool
		priority int
	}{
		{"rule-high", true, 100},
		{"rule-medium", true, 50},
		{"rule-low", false, 10},
	}

	for _, r := range rules {
		condition := models.RuleCondition{Type: "size", MinSize: int64Ptr(100)}
		conditionJSON, _ := json.Marshal(condition)

		outcome := models.RuleOutcome{
			Type:            "resource_set",
			ResourceSetName: "test-set",
			Operation:       strPtr("add"),
		}
		outcomeJSON, _ := json.Marshal(outcome)

		rule := &models.Rule{
			Name:          r.name,
			Enabled:       r.enabled,
			Priority:      r.priority,
			ConditionJSON: string(conditionJSON),
			OutcomeJSON:   string(outcomeJSON),
		}
		_, err := db.CreateRule(rule)
		require.NoError(t, err)
	}

	// List all rules
	allRules, err := db.ListRules(false)
	require.NoError(t, err)
	assert.Len(t, allRules, 3)
	// Should be sorted by priority DESC
	assert.Equal(t, "rule-high", allRules[0].Name)
	assert.Equal(t, "rule-medium", allRules[1].Name)
	assert.Equal(t, "rule-low", allRules[2].Name)

	// List only enabled rules
	enabledRules, err := db.ListRules(true)
	require.NoError(t, err)
	assert.Len(t, enabledRules, 2)
}

func TestRuleExecutionWithResourceSet(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a rule
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Type:            "resource_set",
		ResourceSetName: "test-set",
		Operation:       strPtr("add"),
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "test-rule",
		Enabled:       true,
		Priority:      1,
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
	}
	ruleID, err := db.CreateRule(rule)
	require.NoError(t, err)

	// Create a resource set
	resourceSet := &models.ResourceSet{
		Name:      "test-set",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	setID, err := db.CreateResourceSet(resourceSet)
	require.NoError(t, err)

	// Create a rule execution - MUST have resource_set_id
	execution := &models.RuleExecution{
		RuleID:           ruleID,
		ResourceSetID:    setID,
		EntriesMatched:   10,
		EntriesProcessed: 10,
		Status:           "success",
	}
	executionID, err := db.CreateRuleExecution(execution)
	require.NoError(t, err)
	assert.Greater(t, executionID, int64(0))

	// Retrieve the execution
	retrieved, err := db.GetRuleExecution(executionID)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, ruleID, retrieved.RuleID)
	assert.Equal(t, setID, retrieved.ResourceSetID)
	assert.Equal(t, 10, retrieved.EntriesMatched)
	assert.Equal(t, "success", retrieved.Status)

	// List executions for the rule
	executions, err := db.ListRuleExecutions(ruleID, 0)
	require.NoError(t, err)
	assert.Len(t, executions, 1)

	// List executions for the resource set
	setExecutions, err := db.ListRuleExecutionsBySelectionSet(setID, 0)
	require.NoError(t, err)
	assert.Len(t, setExecutions, 1)
}

func TestRuleExecutionRequiresResourceSet(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a rule
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Type:            "resource_set",
		ResourceSetName: "test-set",
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "test-rule",
		Enabled:       true,
		Priority:      1,
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
	}
	ruleID, err := db.CreateRule(rule)
	require.NoError(t, err)

	// Try to create execution WITHOUT resource_set_id - should fail
	execution := &models.RuleExecution{
		RuleID:           ruleID,
		ResourceSetID:    0, // Missing!
		EntriesMatched:   5,
		EntriesProcessed: 5,
		Status:           "success",
	}
	_, err = db.CreateRuleExecution(execution)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource_set_id is required")
}

func TestRuleOutcomeRecord(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Setup: Create rule, resource set, and execution
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Type:            "classifier",
		ResourceSetName: "test-thumbnails",
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "test-rule",
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
		Enabled:       true,
	}
	ruleID, _ := db.CreateRule(rule)

	resourceSet := &models.ResourceSet{
		Name:      "test-thumbnails",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	setID, _ := db.CreateResourceSet(resourceSet)

	execution := &models.RuleExecution{
		RuleID:        ruleID,
		ResourceSetID: setID,
		Status:        "success",
	}
	executionID, _ := db.CreateRuleExecution(execution)

	// Create an entry
	entry := &models.Entry{
		Path: "/test/file.jpg",
		Size: 1000000,
		Kind: "file",
	}
	_ = db.InsertOrUpdate(entry)

	// Create outcome record - MUST have resource_set_id
	outcomeRecord := &models.RuleOutcomeRecord{
		ExecutionID:   executionID,
		ResourceSetID: setID,
		EntryPath:     "/test/file.jpg",
		OutcomeType:   "generate_thumbnail",
		Status:        "success",
	}
	outcomeID, err := db.CreateRuleOutcome(outcomeRecord)
	require.NoError(t, err)
	assert.Greater(t, outcomeID, int64(0))

	// List outcomes for execution
	outcomes, err := db.ListRuleOutcomes(executionID)
	require.NoError(t, err)
	assert.Len(t, outcomes, 1)
	assert.Equal(t, "/test/file.jpg", outcomes[0].EntryPath)
	assert.Equal(t, "generate_thumbnail", outcomes[0].OutcomeType)
	assert.Equal(t, setID, outcomes[0].ResourceSetID)

	// List outcomes by resource set
	setOutcomes, err := db.ListRuleOutcomesBySelectionSet(setID, 0)
	require.NoError(t, err)
	assert.Len(t, setOutcomes, 1)
}

func TestRuleOutcomeRequiresResourceSet(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Try to create outcome WITHOUT resource_set_id - should fail
	outcomeRecord := &models.RuleOutcomeRecord{
		ExecutionID:   1,
		ResourceSetID: 0, // Missing!
		EntryPath:     "/test/file.jpg",
		OutcomeType:   "test",
		Status:        "success",
	}
	_, err = db.CreateRuleOutcome(outcomeRecord)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource_set_id is required")
}

func TestValidateRuleOutcome(t *testing.T) {
	tests := []struct {
		name      string
		outcome   *models.RuleOutcome
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid resource_set outcome",
			outcome: &models.RuleOutcome{
				Type:            "resource_set",
				ResourceSetName: "test-set",
			},
			shouldErr: false,
		},
		{
			name: "missing resourceSetName",
			outcome: &models.RuleOutcome{
				Type: "resource_set",
			},
			shouldErr: true,
			errMsg:    "resourceSetName is required",
		},
		{
			name: "valid chained outcome",
			outcome: &models.RuleOutcome{
				Type:            "chained",
				ResourceSetName: "parent-set",
				Outcomes: []*models.RuleOutcome{
					{
						Type:            "classifier",
						ResourceSetName: "child-set",
					},
				},
			},
			shouldErr: false,
		},
		{
			name: "chained outcome with missing child resourceSetName",
			outcome: &models.RuleOutcome{
				Type:            "chained",
				ResourceSetName: "parent-set",
				Outcomes: []*models.RuleOutcome{
					{
						Type: "classifier",
						// Missing ResourceSetName!
					},
				},
			},
			shouldErr: true,
			errMsg:    "chained outcome 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuleOutcome(tt.outcome)
			if tt.shouldErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnsureResourceSetForOutcome(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	outcome := &models.RuleOutcome{
		Type:            "classifier",
		ResourceSetName: "auto-created-set",
	}

	// Should auto-create the resource set
	setID, err := db.EnsureResourceSetForOutcome(outcome, "test-rule")
	require.NoError(t, err)
	assert.Greater(t, setID, int64(0))

	// Verify it was created
	set, err := db.GetResourceSet("auto-created-set")
	require.NoError(t, err)
	assert.NotNil(t, set)
	assert.Equal(t, "auto-created-set", set.Name)
	assert.Contains(t, *set.Description, "test-rule")

	// Calling again should return the same ID
	setID2, err := db.EnsureResourceSetForOutcome(outcome, "test-rule")
	require.NoError(t, err)
	assert.Equal(t, setID, setID2)
}

// Helper function
func strPtr(s string) *string {
	return &s
}
