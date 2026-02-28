package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeRequest(name string, args map[string]interface{}) mcp.CallToolRequest {
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = name
	request.Params.Arguments = argsMap
	return request
}

func resultJSON(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	var response map[string]interface{}
	textContent := result.Content[0].(mcp.TextContent)
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &response))
	return response
}

func TestScanTool_BasicIndex(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir", "nested.txt"), []byte("world"), 0644))

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("scan", map[string]interface{}{
		"paths": []interface{}{tmpDir},
		"depth": -1,
		"async": false,
		"force": true,
	})

	result, err := handleScan(context.Background(), request, db)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "scan should not return error")

	response := resultJSON(t, result)
	assert.Equal(t, "completed", response["status"])

	entries, err := db.All()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 3)
}

func TestScanTool_MultiplePaths(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "a.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "b.txt"), []byte("b"), 0644))

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("scan", map[string]interface{}{
		"paths": []interface{}{tmpDir1, tmpDir2},
		"async": false,
		"force": true,
	})

	result, err := handleScan(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	results := response["results"].([]interface{})
	assert.Len(t, results, 2)

	entry1, err := db.Get(tmpDir1)
	assert.NoError(t, err)
	assert.NotNil(t, entry1)

	entry2, err := db.Get(tmpDir2)
	assert.NoError(t, err)
	assert.NotNil(t, entry2)
}

func TestScanTool_AsyncReturnsJobID(t *testing.T) {
	tmpDir := t.TempDir()

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("scan", map[string]interface{}{
		"paths": []interface{}{tmpDir},
		"async": true,
		"force": true,
	})

	result, err := handleScan(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "started", response["status"])
	assert.Contains(t, response, "jobs")
	jobs := response["jobs"].([]interface{})
	assert.Len(t, jobs, 1)
	job := jobs[0].(map[string]interface{})
	assert.Contains(t, job, "job_id")
	assert.Contains(t, job, "status_url")
}

func TestScanTool_AsyncCompletesAndUpdatesJobStatus(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644))

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("scan", map[string]interface{}{
		"paths": []interface{}{tmpDir},
		"async": true,
		"force": true,
	})

	result, err := handleScan(context.Background(), request, db)
	require.NoError(t, err)

	response := resultJSON(t, result)
	jobs := response["jobs"].([]interface{})
	job := jobs[0].(map[string]interface{})
	jobID := int64(job["job_id"].(float64))

	// Wait for the async scan to complete (poll job status)
	var finalStatus string
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		jobRecord, err := db.GetIndexJob(jobID)
		require.NoError(t, err)
		require.NotNil(t, jobRecord)
		if jobRecord.Status == "completed" || jobRecord.Status == "failed" {
			finalStatus = jobRecord.Status
			break
		}
	}

	assert.Equal(t, "completed", finalStatus, "Async scan job should reach 'completed' status")

	// Verify the job has completedAt set
	jobRecord, err := db.GetIndexJob(jobID)
	require.NoError(t, err)
	assert.NotNil(t, jobRecord.CompletedAt, "Completed job should have completedAt timestamp")
}

func TestScanTool_MissingPaths(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("scan", map[string]interface{}{})

	result, err := handleScan(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError, "scan without paths should error")
}

func TestScanTool_InvalidPath(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("scan", map[string]interface{}{
		"paths": []interface{}{"/nonexistent/path/that/doesnt/exist"},
		"async": false,
	})

	result, err := handleScan(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
