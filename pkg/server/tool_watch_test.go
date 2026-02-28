package server

import (
	"context"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchTool_List_Empty(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// With no sourceManager, list should return empty
	oldManager := sourceManager
	sourceManager = nil
	defer func() { sourceManager = oldManager }()

	request := makeRequest("watch", map[string]interface{}{
		"action": "list",
	})
	result, err := handleWatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, float64(0), response["total"])
	watchers := response["watchers"].([]interface{})
	assert.Len(t, watchers, 0)
}

func TestWatchTool_StartMissingPath(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("watch", map[string]interface{}{
		"action": "start",
		"name":   "test-watch",
	})
	result, err := handleWatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestWatchTool_StartMissingName(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("watch", map[string]interface{}{
		"action": "start",
		"path":   "/tmp",
	})
	result, err := handleWatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestWatchTool_StartNoManager(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	oldManager := sourceManager
	sourceManager = nil
	defer func() { sourceManager = oldManager }()

	request := makeRequest("watch", map[string]interface{}{
		"action": "start",
		"path":   "/tmp",
		"name":   "test-watch",
	})
	result, err := handleWatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestWatchTool_StopNoManager(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	oldManager := sourceManager
	sourceManager = nil
	defer func() { sourceManager = oldManager }()

	request := makeRequest("watch", map[string]interface{}{
		"action": "stop",
		"name":   "test-watch",
	})
	result, err := handleWatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestWatchTool_InvalidAction(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("watch", map[string]interface{}{
		"action": "explode",
	})
	result, err := handleWatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
