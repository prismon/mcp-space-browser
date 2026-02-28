package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupManageTestDB(t *testing.T) *database.DiskDB {
	t.Helper()
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	return db
}

func TestManageTool_ResourceSetCreate(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	request := makeRequest("manage", map[string]interface{}{
		"entity":      "resource-set",
		"action":      "create",
		"name":        "test-set",
		"description": "A test set",
	})

	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "test-set", response["name"])
	assert.Equal(t, "created", response["status"])

	// Verify it exists
	rs, err := db.GetResourceSet("test-set")
	require.NoError(t, err)
	assert.Equal(t, "test-set", rs.Name)
}

func TestManageTool_ResourceSetList(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	// Create 2 sets
	for _, name := range []string{"set-a", "set-b"} {
		req := makeRequest("manage", map[string]interface{}{
			"entity": "resource-set",
			"action": "create",
			"name":   name,
		})
		result, err := handleManage(context.Background(), req, db)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	}

	// List
	request := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "list",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	items := response["items"].([]interface{})
	assert.Len(t, items, 2)
	assert.Equal(t, float64(2), response["total"])
}

func TestManageTool_ResourceSetGet(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	// Create
	req := makeRequest("manage", map[string]interface{}{
		"entity":      "resource-set",
		"action":      "create",
		"name":        "my-set",
		"description": "My description",
	})
	_, err := handleManage(context.Background(), req, db)
	require.NoError(t, err)

	// Get
	request := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "get",
		"name":   "my-set",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "my-set", response["name"])
}

func TestManageTool_ResourceSetDelete(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	// Create
	req := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "create",
		"name":   "doomed",
	})
	_, err := handleManage(context.Background(), req, db)
	require.NoError(t, err)

	// Delete
	request := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "delete",
		"name":   "doomed",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "deleted", response["status"])

	// Verify gone
	rs, err := db.GetResourceSet("doomed")
	assert.NoError(t, err)
	assert.Nil(t, rs)
}

func TestManageTool_PlanCreate(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	request := makeRequest("manage", map[string]interface{}{
		"entity":      "plan",
		"action":      "create",
		"name":        "my-plan",
		"description": "Test plan",
		"mode":        "oneshot",
		"sources": []interface{}{
			map[string]interface{}{"type": "project"},
		},
		"outcomes": []interface{}{
			map[string]interface{}{"tool": "resource-set-modify", "arguments": map[string]interface{}{"name": "test"}},
		},
	})

	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "my-plan", response["name"])
	assert.Equal(t, "created", response["status"])

	// Verify
	plan, err := db.GetPlan("my-plan")
	require.NoError(t, err)
	assert.Equal(t, "my-plan", plan.Name)
	assert.Equal(t, "oneshot", plan.Mode)
}

func TestManageTool_PlanList(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	// Create 2 plans
	for _, name := range []string{"plan-1", "plan-2"} {
		req := makeRequest("manage", map[string]interface{}{
			"entity":   "plan",
			"action":   "create",
			"name":     name,
			"sources":  []interface{}{map[string]interface{}{"type": "project"}},
			"outcomes": []interface{}{map[string]interface{}{"tool": "resource-set-modify", "arguments": map[string]interface{}{"name": "test"}}},
		})
		result, err := handleManage(context.Background(), req, db)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	}

	// List
	request := makeRequest("manage", map[string]interface{}{
		"entity": "plan",
		"action": "list",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	items := response["items"].([]interface{})
	assert.Len(t, items, 2)
}

func TestManageTool_JobList(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	// Job list should start empty
	request := makeRequest("manage", map[string]interface{}{
		"entity": "job",
		"action": "list",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, float64(0), response["total"])
}

func TestManageTool_InvalidEntity(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	request := makeRequest("manage", map[string]interface{}{
		"entity": "unknown",
		"action": "list",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestManageTool_InvalidAction(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	request := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "explode",
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestManageTool_ResourceSetListPagination(t *testing.T) {
	db := setupManageTestDB(t)
	defer db.Close()

	// Create 5 sets
	for i := 0; i < 5; i++ {
		req := makeRequest("manage", map[string]interface{}{
			"entity": "resource-set",
			"action": "create",
			"name":   fmt.Sprintf("set-%d", i),
		})
		_, err := handleManage(context.Background(), req, db)
		require.NoError(t, err)
	}

	// Page 1: limit=2
	request := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "list",
		"limit":  2,
	})
	result, err := handleManage(context.Background(), request, db)
	require.NoError(t, err)

	response := resultJSON(t, result)
	items := response["items"].([]interface{})
	assert.Len(t, items, 2)
	assert.Equal(t, float64(5), response["total"])
	assert.NotNil(t, response["next_cursor"])

	// Page 2
	cursor := response["next_cursor"].(string)
	request2 := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "list",
		"limit":  2,
		"cursor": cursor,
	})
	result2, err := handleManage(context.Background(), request2, db)
	require.NoError(t, err)

	response2 := resultJSON(t, result2)
	items2 := response2["items"].([]interface{})
	assert.Len(t, items2, 2)
	assert.NotNil(t, response2["next_cursor"])

	// Page 3 (last)
	cursor3 := response2["next_cursor"].(string)
	request3 := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "list",
		"limit":  2,
		"cursor": cursor3,
	})
	result3, err := handleManage(context.Background(), request3, db)
	require.NoError(t, err)

	response3 := resultJSON(t, result3)
	items3 := response3["items"].([]interface{})
	assert.Len(t, items3, 1)
	assert.Nil(t, response3["next_cursor"])
}
