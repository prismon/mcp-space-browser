package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupQueryTestDB(t *testing.T) *database.DiskDB {
	t.Helper()
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)

	now := time.Now().Unix()
	root := "/"
	testDir := "/photos"
	entries := []*models.Entry{
		{Path: "/photos", Parent: &root, Size: 0, Kind: "directory", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/photos/a.jpg", Parent: &testDir, Size: 5000, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/photos/b.png", Parent: &testDir, Size: 10000, Kind: "file", Ctime: now, Mtime: now - 86400, LastScanned: now},
		{Path: "/photos/c.txt", Parent: &testDir, Size: 100, Kind: "file", Ctime: now, Mtime: now - 172800, LastScanned: now},
	}
	for _, e := range entries {
		require.NoError(t, db.InsertOrUpdate(e))
	}

	mimeJpeg := "image/jpeg"
	mimePng := "image/png"
	mimeText := "text/plain"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{EntryPath: "/photos/a.jpg", Key: "mime", Value: &mimeJpeg, Source: "scan"}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{EntryPath: "/photos/b.png", Key: "mime", Value: &mimePng, Source: "scan"}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{EntryPath: "/photos/c.txt", Key: "mime", Value: &mimeText, Source: "scan"}))

	return db
}

func TestQueryTool_BasicFilter(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{"kind": "file"},
		"limit": 100,
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 3)
}

func TestQueryTool_SizeFilter(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{
			"kind": "file",
			"size": map[string]interface{}{">": 1000},
		},
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2)
}

func TestQueryTool_AttributeFilter(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{
			"mime": map[string]interface{}{"like": "image/%"},
		},
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2) // a.jpg and b.png
}

func TestQueryTool_Aggregate_Sum(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where":     map[string]interface{}{"kind": "file"},
		"aggregate": "sum",
		"field":     "size",
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, float64(15100), response["value"])
}

func TestQueryTool_Aggregate_Count(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where":     map[string]interface{}{"kind": "file"},
		"aggregate": "count",
		"field":     "size",
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, float64(3), response["value"])
}

func TestQueryTool_OrderBy_Descending(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where":    map[string]interface{}{"kind": "file"},
		"order_by": "-size",
		"limit":    2,
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2)
	first := entries[0].(map[string]interface{})
	assert.Equal(t, "/photos/b.png", first["path"])
}

func TestQueryTool_CursorPagination(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	// Page 1: limit=2
	request := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{"kind": "file"},
		"limit": 2,
	})
	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2)
	assert.Equal(t, float64(3), response["total"])
	assert.NotNil(t, response["next_cursor"])

	// Page 2: use cursor
	cursor := response["next_cursor"].(string)
	request2 := makeRequest("query", map[string]interface{}{
		"where":  map[string]interface{}{"kind": "file"},
		"limit":  2,
		"cursor": cursor,
	})
	result2, err := handleQuery(context.Background(), request2, db)
	require.NoError(t, err)

	response2 := resultJSON(t, result2)
	entries2 := response2["entries"].([]interface{})
	assert.Len(t, entries2, 1) // Only 1 left
	assert.Nil(t, response2["next_cursor"])
}

func TestQueryTool_GroupedAggregate(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{
		"where":     map[string]interface{}{"kind": "file"},
		"aggregate": "sum",
		"field":     "size",
		"group_by":  "mime",
	})

	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	groups := response["groups"].([]interface{})
	assert.Len(t, groups, 3) // image/jpeg, image/png, text/plain
}

func TestQueryTool_EmptyWhere(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	request := makeRequest("query", map[string]interface{}{})
	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 4) // All entries
}

func TestQueryTool_ResourceSetFilter(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	// Create a resource set and add entries
	_, err := db.CreateResourceSet(&models.ResourceSet{Name: "images"})
	require.NoError(t, err)
	require.NoError(t, db.AddToResourceSet("images", []string{"/photos/a.jpg", "/photos/b.png"}))

	request := makeRequest("query", map[string]interface{}{
		"from": "images",
	})
	result, err := handleQuery(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2)
}

// Helper already defined in tool_scan_test.go, but we need it visible in same package.
// makeRequest and resultJSON are defined in tool_scan_test.go.
// Go test files in the same package share scope, so they're available here.
func init() {
	// Suppress unused import
	_ = mcp.TextContent{}
	_ = json.Marshal
}
