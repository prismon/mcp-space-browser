package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateQuery(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	desc := "Find large files"
	filter := models.FileFilter{
		MinSize: int64Ptr(1000000),
	}
	filterJSON, _ := json.Marshal(filter)

	query := &models.Query{
		Name:        "large-files",
		Description: &desc,
		QueryType:   "file_filter",
		QueryJSON:   string(filterJSON),
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	id, err := db.CreateQuery(query)
	assert.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestGetQuery(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test getting non-existent query
	query, err := db.GetQuery("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, query)

	// Create and retrieve
	desc := "Test query"
	filter := models.FileFilter{
		MinSize: int64Ptr(500),
	}
	filterJSON, _ := json.Marshal(filter)

	newQuery := &models.Query{
		Name:        "test-query",
		Description: &desc,
		QueryType:   "file_filter",
		QueryJSON:   string(filterJSON),
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	id, err := db.CreateQuery(newQuery)
	require.NoError(t, err)

	retrieved, err := db.GetQuery("test-query")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, id, retrieved.ID)
	assert.Equal(t, "test-query", retrieved.Name)
	assert.Equal(t, "file_filter", retrieved.QueryType)
}

func TestListQueries(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple queries
	for i := 0; i < 3; i++ {
		name := "query-" + string(rune('a'+i))
		filter := models.FileFilter{}
		filterJSON, _ := json.Marshal(filter)

		query := &models.Query{
			Name:        name,
			QueryType:   "file_filter",
			QueryJSON:   string(filterJSON),
			CreatedAt:   time.Now().Unix(),
			UpdatedAt:   time.Now().Unix(),
		}
		_, err := db.CreateQuery(query)
		require.NoError(t, err)
	}

	queries, err := db.ListQueries()
	assert.NoError(t, err)
	assert.Len(t, queries, 3)
}

func TestDeleteQuery(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	filter := models.FileFilter{}
	filterJSON, _ := json.Marshal(filter)

	query := &models.Query{
		Name:        "to-delete",
		QueryType:   "file_filter",
		QueryJSON:   string(filterJSON),
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	_, err = db.CreateQuery(query)
	require.NoError(t, err)

	// Delete it
	err = db.DeleteQuery("to-delete")
	assert.NoError(t, err)

	// Verify it's gone
	retrieved, err := db.GetQuery("to-delete")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestExecuteQuery(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Insert test files
	now := time.Now().Unix()
	files := []*models.Entry{
		{
			Path:        "/test/small.txt",
			Size:        100,
			Kind:        "file",
			Ctime:       now,
			Mtime:       now,
			LastScanned: now,
		},
		{
			Path:        "/test/large.txt",
			Size:        5000,
			Kind:        "file",
			Ctime:       now,
			Mtime:       now,
			LastScanned: now,
		},
	}

	for _, file := range files {
		db.InsertOrUpdate(file)
	}

	// Create query for large files
	minSize := int64(1000)
	filter := models.FileFilter{
		MinSize: &minSize,
	}
	filterJSON, _ := json.Marshal(filter)

	query := &models.Query{
		Name:        "find-large",
		QueryType:   "file_filter",
		QueryJSON:   string(filterJSON),
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	_, err = db.CreateQuery(query)
	require.NoError(t, err)

	// Execute query
	entries, err := db.ExecuteQuery("find-large")
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/test/large.txt", entries[0].Path)

	// Verify execution was recorded
	retrieved, err := db.GetQuery("find-large")
	assert.NoError(t, err)
	assert.Equal(t, 1, retrieved.ExecutionCount)
	assert.NotNil(t, retrieved.LastExecuted)
}

func TestRecordQueryExecution(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a query first
	filter := models.FileFilter{}
	filterJSON, _ := json.Marshal(filter)

	query := &models.Query{
		Name:        "exec-test",
		QueryType:   "file_filter",
		QueryJSON:   string(filterJSON),
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	queryID, err := db.CreateQuery(query)
	require.NoError(t, err)

	// Record execution
	duration := 150
	filesMatched := 5
	exec := &models.QueryExecution{
		QueryID:      queryID,
		ExecutedAt:   time.Now().Unix(),
		DurationMs:   &duration,
		FilesMatched: &filesMatched,
		Status:       "success",
	}

	err = db.RecordQueryExecution(exec)
	assert.NoError(t, err)
}

func TestGetQueryExecutions(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create query
	filter := models.FileFilter{}
	filterJSON, _ := json.Marshal(filter)

	query := &models.Query{
		Name:        "exec-history",
		QueryType:   "file_filter",
		QueryJSON:   string(filterJSON),
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	queryID, err := db.CreateQuery(query)
	require.NoError(t, err)

	// Record multiple executions
	for i := 0; i < 5; i++ {
		duration := i * 100
		filesMatched := i * 10
		exec := &models.QueryExecution{
			QueryID:      queryID,
			ExecutedAt:   time.Now().Unix(),
			DurationMs:   &duration,
			FilesMatched: &filesMatched,
			Status:       "success",
		}
		db.RecordQueryExecution(exec)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Get executions
	executions, err := db.GetQueryExecutions(queryID, 3)
	assert.NoError(t, err)
	assert.Len(t, executions, 3)

	// Verify most recent first
	assert.True(t, executions[0].ExecutedAt >= executions[1].ExecutedAt)
}

func TestExecuteQueryWithError(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create query with invalid JSON
	query := &models.Query{
		Name:        "bad-query",
		QueryType:   "file_filter",
		QueryJSON:   "invalid json",
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	_, err = db.CreateQuery(query)
	require.NoError(t, err)

	// Execute should fail
	_, err = db.ExecuteQuery("bad-query")
	assert.Error(t, err)

	// When JSON unmarshal fails, execution is not recorded
	// This is expected behavior - we only record executions for valid queries
	retrieved, err := db.GetQuery("bad-query")
	assert.NoError(t, err)
	assert.Equal(t, 0, retrieved.ExecutionCount)
}

// Helper function
func int64Ptr(i int64) *int64 {
	return &i
}
