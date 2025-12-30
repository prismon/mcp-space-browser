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
		Tool: "classifier-process",
		Arguments: map[string]interface{}{
			"operation": "generate_thumbnail",
		},
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
	assert.Equal(t, "classifier-process", parsedOutcome.Tool)
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
			Tool: "resource-set-modify",
			Arguments: map[string]interface{}{
				"name":      "test-set",
				"operation": "add",
			},
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
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name":      "test-set",
			"operation": "add",
		},
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

	// Create a selection set
	resourceSet := &models.ResourceSet{
		Name:      "test-set",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	setID, err := db.CreateResourceSet(resourceSet)
	require.NoError(t, err)

	// Create a rule execution - MUST have selection_set_id
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

	// List executions for the selection set
	setExecutions, err := db.ListRuleExecutionsByResourceSet(setID, 0)
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
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "test-set",
		},
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

	// Try to create execution WITHOUT selection_set_id - should fail
	execution := &models.RuleExecution{
		RuleID:           ruleID,
		ResourceSetID:    0, // Missing!
		EntriesMatched:   5,
		EntriesProcessed: 5,
		Status:           "success",
	}
	_, err = db.CreateRuleExecution(execution)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "selection_set_id is required")
}

func TestRuleOutcomeRecord(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Setup: Create rule, selection set, and execution
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Tool: "classifier-process",
		Arguments: map[string]interface{}{
			"operation": "thumbnail",
		},
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

	// Create outcome record - MUST have selection_set_id
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

	// List outcomes by selection set
	setOutcomes, err := db.ListRuleOutcomesByResourceSet(setID, 0)
	require.NoError(t, err)
	assert.Len(t, setOutcomes, 1)
}

func TestRuleOutcomeRequiresResourceSet(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Try to create outcome WITHOUT selection_set_id - should fail
	outcomeRecord := &models.RuleOutcomeRecord{
		ExecutionID:   1,
		ResourceSetID: 0, // Missing!
		EntryPath:     "/test/file.jpg",
		OutcomeType:   "test",
		Status:        "success",
	}
	_, err = db.CreateRuleOutcome(outcomeRecord)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "selection_set_id is required")
}

func TestValidateRuleOutcome(t *testing.T) {
	tests := []struct {
		name      string
		outcome   *models.RuleOutcome
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid tool-based outcome",
			outcome: &models.RuleOutcome{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name": "test-set",
				},
			},
			shouldErr: false,
		},
		{
			name: "missing tool",
			outcome: &models.RuleOutcome{
				Arguments: map[string]interface{}{
					"name": "test-set",
				},
			},
			shouldErr: true,
			errMsg:    "must specify 'tool'",
		},
		{
			name: "valid chained outcome",
			outcome: &models.RuleOutcome{
				Outcomes: []*models.RuleOutcome{
					{
						Tool: "resource-set-modify",
						Arguments: map[string]interface{}{
							"name": "child-set",
						},
					},
				},
			},
			shouldErr: false,
		},
		{
			name: "chained outcome with invalid child",
			outcome: &models.RuleOutcome{
				Outcomes: []*models.RuleOutcome{
					{
						// Missing Tool!
						Arguments: map[string]interface{}{
							"name": "test",
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "must specify 'tool'",
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

func TestEnsureResourceSetExists(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Should auto-create the selection set
	setID, err := db.EnsureResourceSetExists("auto-created-set", "test-rule")
	require.NoError(t, err)
	assert.Greater(t, setID, int64(0))

	// Verify it was created
	set, err := db.GetResourceSet("auto-created-set")
	require.NoError(t, err)
	assert.NotNil(t, set)
	assert.Equal(t, "auto-created-set", set.Name)
	assert.Contains(t, *set.Description, "test-rule")

	// Calling again should return the same ID
	setID2, err := db.EnsureResourceSetExists("auto-created-set", "test-rule")
	require.NoError(t, err)
	assert.Equal(t, setID, setID2)
}

func TestGetRuleByID(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a rule
	condition := models.RuleCondition{Type: "size", MinSize: int64Ptr(100)}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "test-set",
		},
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "id-test-rule",
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
		Enabled:       true,
		Priority:      5,
	}
	id, err := db.CreateRule(rule)
	require.NoError(t, err)

	// Get by ID
	retrieved, err := db.GetRuleByID(id)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "id-test-rule", retrieved.Name)
	assert.Equal(t, 5, retrieved.Priority)

	// Test non-existent ID
	notFound, err := db.GetRuleByID(9999)
	assert.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestUpdateRule(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a rule
	condition := models.RuleCondition{Type: "size", MinSize: int64Ptr(100)}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "test-set",
		},
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "update-test-rule",
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
		Enabled:       true,
		Priority:      5,
	}
	id, err := db.CreateRule(rule)
	require.NoError(t, err)

	// Update the rule
	rule.ID = id
	rule.Enabled = false
	rule.Priority = 20
	desc := "Updated description"
	rule.Description = &desc

	err = db.UpdateRule(rule)
	assert.NoError(t, err)

	// Verify update
	retrieved, _ := db.GetRuleByID(id)
	assert.False(t, retrieved.Enabled)
	assert.Equal(t, 20, retrieved.Priority)
	assert.Equal(t, "Updated description", *retrieved.Description)
}

func TestDeleteRule(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a rule
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "test-set",
		},
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "delete-test-rule",
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
	}
	id, err := db.CreateRule(rule)
	require.NoError(t, err)

	// Verify exists
	exists, _ := db.GetRuleByID(id)
	assert.NotNil(t, exists)

	// Delete by name
	err = db.DeleteRule("delete-test-rule")
	assert.NoError(t, err)

	// Verify deleted
	deleted, _ := db.GetRuleByID(id)
	assert.Nil(t, deleted)

	// Delete non-existent should not error
	err = db.DeleteRule("nonexistent-rule")
	assert.NoError(t, err)
}

func TestUpdateResourceSet(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a selection set
	set := &models.ResourceSet{
		Name: "update-test-set",
	}
	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	// Update the set by name
	desc := "Updated description"
	err = db.UpdateResourceSet("update-test-set", &desc)
	assert.NoError(t, err)

	// Verify update
	retrieved, _ := db.GetResourceSet("update-test-set")
	assert.NotNil(t, retrieved.Description)
	assert.Equal(t, "Updated description", *retrieved.Description)
}

func TestGetRuleExecutionNonexistent(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get non-existent execution
	exec, err := db.GetRuleExecution(9999)
	assert.NoError(t, err)
	assert.Nil(t, exec)
}

func TestListRuleExecutionsWithLimit(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a rule
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "test-set",
		},
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "exec-limit-rule",
		Enabled:       true,
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
	}
	ruleID, err := db.CreateRule(rule)
	require.NoError(t, err)

	// Create selection set
	set := &models.ResourceSet{Name: "test-set"}
	setID, _ := db.CreateResourceSet(set)

	// Create multiple executions
	for i := 0; i < 5; i++ {
		exec := &models.RuleExecution{
			RuleID:        ruleID,
			ResourceSetID: setID,
			Status:        "success",
		}
		_, err := db.CreateRuleExecution(exec)
		require.NoError(t, err)
	}

	// List with limit
	execs, err := db.ListRuleExecutions(ruleID, 3)
	assert.NoError(t, err)
	assert.Len(t, execs, 3)

	// List all (limit 0)
	allExecs, err := db.ListRuleExecutions(ruleID, 0)
	assert.NoError(t, err)
	assert.Len(t, allExecs, 5)
}

func TestListRuleOutcomesEmpty(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// List outcomes for non-existent execution
	outcomes, err := db.ListRuleOutcomes(9999)
	assert.NoError(t, err)
	assert.Len(t, outcomes, 0)
}

func TestParseRuleConditionInvalid(t *testing.T) {
	_, err := ParseRuleCondition("invalid json")
	assert.Error(t, err)
}

func TestParseRuleOutcomeInvalid(t *testing.T) {
	_, err := ParseRuleOutcome("invalid json")
	assert.Error(t, err)
}

func TestUpdateResourceSetNonexistent(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	desc := "New desc"
	err = db.UpdateResourceSet("nonexistent-set", &desc)
	assert.Error(t, err)
}

func TestGetResourceSetStatsFromRules(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a selection set
	set := &models.ResourceSet{Name: "rule-stats-set"}
	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	// Create entries
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path:  "/rulestats/file" + string(rune('a'+i)) + ".txt",
			Size:  int64(i * 100),
			Kind:  "file",
			Mtime: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Add entries to set
	paths := []string{"/rulestats/filea.txt", "/rulestats/fileb.txt", "/rulestats/filec.txt"}
	db.AddToResourceSet("rule-stats-set", paths)

	// Get stats
	stats, err := db.GetResourceSetStats("rule-stats-set")
	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

func TestListRuleOutcomesByResourceSetWithLimit(t *testing.T) {
	os.Setenv("GO_ENV", "test")
	defer os.Unsetenv("GO_ENV")

	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create rule
	condition := models.RuleCondition{Type: "size"}
	conditionJSON, _ := json.Marshal(condition)
	outcome := models.RuleOutcome{
		Tool: "resource-set-modify",
		Arguments: map[string]interface{}{
			"name": "limit-test-set",
		},
	}
	outcomeJSON, _ := json.Marshal(outcome)

	rule := &models.Rule{
		Name:          "limit-outcome-rule",
		Enabled:       true,
		ConditionJSON: string(conditionJSON),
		OutcomeJSON:   string(outcomeJSON),
	}
	ruleID, _ := db.CreateRule(rule)

	// Create selection set
	set := &models.ResourceSet{Name: "limit-test-set"}
	setID, _ := db.CreateResourceSet(set)

	// Create execution
	exec := &models.RuleExecution{
		RuleID:        ruleID,
		ResourceSetID: setID,
		Status:        "success",
	}
	execID, _ := db.CreateRuleExecution(exec)

	// Create multiple entries and outcomes
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path: "/limit/file" + string(rune('a'+i)) + ".txt",
			Size: int64(i * 100),
			Kind: "file",
		}
		db.InsertOrUpdate(entry)

		outcomeRecord := &models.RuleOutcomeRecord{
			ExecutionID:   execID,
			ResourceSetID: setID,
			EntryPath:     entry.Path,
			OutcomeType:   "test",
			Status:        "success",
		}
		db.CreateRuleOutcome(outcomeRecord)
	}

	// List with limit
	outcomes, err := db.ListRuleOutcomesByResourceSet(setID, 3)
	assert.NoError(t, err)
	assert.Len(t, outcomes, 3)

	// List all
	allOutcomes, err := db.ListRuleOutcomesByResourceSet(setID, 0)
	assert.NoError(t, err)
	assert.Len(t, allOutcomes, 5)
}

// Helper function
func strPtr(s string) *string {
	return &s
}
