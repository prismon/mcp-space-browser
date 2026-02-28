package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBatchTestDB(t *testing.T) *database.DiskDB {
	t.Helper()
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)

	now := time.Now().Unix()
	root := "/"
	testDir := "/test"
	entries := []*models.Entry{
		{Path: "/test", Parent: &root, Size: 0, Kind: "directory", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/test/a.txt", Parent: &testDir, Size: 100, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/test/b.txt", Parent: &testDir, Size: 200, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/test/c.txt", Parent: &testDir, Size: 300, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
	}
	for _, e := range entries {
		require.NoError(t, db.InsertOrUpdate(e))
	}
	return db
}

func TestBatchTool_Attributes(t *testing.T) {
	db := setupBatchTestDB(t)
	defer db.Close()

	request := makeRequest("batch", map[string]interface{}{
		"operation": "attributes",
		"paths":     []interface{}{"/test/a.txt", "/test/b.txt"},
		"keys":      []interface{}{"kind", "size"},
	})

	result, err := handleBatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "attributes", response["operation"])
	assert.Equal(t, float64(2), response["processed"])

	results := response["results"].([]interface{})
	assert.Len(t, results, 2)

	// Verify metadata were stored in DB
	m, err := db.GetMetadataByKey("/test/a.txt", "kind")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "file", *m.Value)
}

func TestBatchTool_Duplicates_Exact(t *testing.T) {
	db := setupBatchTestDB(t)
	defer db.Close()

	// Set same hash for a.txt and b.txt
	hashAbc := "abc123"
	hashDef := "def456"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{EntryPath: "/test/a.txt", Key: "hash.md5", Value: &hashAbc, Source: "scan"}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{EntryPath: "/test/b.txt", Key: "hash.md5", Value: &hashAbc, Source: "scan"}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{EntryPath: "/test/c.txt", Key: "hash.md5", Value: &hashDef, Source: "scan"}))

	request := makeRequest("batch", map[string]interface{}{
		"operation": "duplicates",
		"paths":     []interface{}{"/test/a.txt", "/test/b.txt", "/test/c.txt"},
		"method":    "exact",
	})

	result, err := handleBatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "duplicates", response["operation"])
	assert.Equal(t, float64(1), response["duplicate_groups"])

	groups := response["groups"].([]interface{})
	assert.Len(t, groups, 1)
	group := groups[0].(map[string]interface{})
	assert.Equal(t, "abc123", group["hash"])
	assert.Equal(t, float64(2), group["count"])
}

func TestBatchTool_Move(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.Mkdir(srcDir, 0755))
	require.NoError(t, os.Mkdir(dstDir, 0755))

	// Create actual files
	filePath := filepath.Join(srcDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0644))

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	require.NoError(t, db.InsertOrUpdate(&models.Entry{
		Path: filePath, Parent: &srcDir, Size: 5, Kind: "file",
		Ctime: now, Mtime: now, LastScanned: now,
	}))

	request := makeRequest("batch", map[string]interface{}{
		"operation":   "move",
		"paths":       []interface{}{filePath},
		"destination": dstDir,
	})

	result, err := handleBatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, float64(1), response["moved"])

	// Verify file moved
	_, err = os.Stat(filepath.Join(dstDir, "test.txt"))
	assert.NoError(t, err)
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestBatchTool_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doomed.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("bye"), 0644))

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	parent := tmpDir
	require.NoError(t, db.InsertOrUpdate(&models.Entry{
		Path: filePath, Parent: &parent, Size: 3, Kind: "file",
		Ctime: now, Mtime: now, LastScanned: now,
	}))

	request := makeRequest("batch", map[string]interface{}{
		"operation": "delete",
		"paths":     []interface{}{filePath},
	})

	result, err := handleBatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, float64(1), response["deleted"])

	// Verify file gone
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))

	// Verify DB entry gone
	entry, err := db.Get(filePath)
	assert.NoError(t, err)
	assert.Nil(t, entry)
}

func TestBatchTool_InvalidOperation(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("batch", map[string]interface{}{
		"operation": "explode",
		"paths":     []interface{}{"/test.txt"},
	})

	result, err := handleBatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestBatchTool_NoPaths(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	request := makeRequest("batch", map[string]interface{}{
		"operation": "attributes",
	})

	result, err := handleBatch(context.Background(), request, db)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
