package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ScanCreateQueryVerify(t *testing.T) {
	// 1. Create temp directory with known file structure
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("hello world"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("foo bar baz"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir", "file3.txt"), []byte("nested content here"), 0644))

	// 2. Create in-memory DiskDB
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// 3. Create a resource-set via handleManage
	createSetReq := makeRequest("manage", map[string]interface{}{
		"entity": "resource-set",
		"action": "create",
		"name":   "test-files",
	})
	result, err := handleManage(ctx, createSetReq, db)
	require.NoError(t, err)
	require.False(t, result.IsError, "create resource-set should succeed")
	resp := resultJSON(t, result)
	assert.Equal(t, "created", resp["status"])

	// 4. Scan the temp directory synchronously
	scanReq := makeRequest("scan", map[string]interface{}{
		"paths": []interface{}{tmpDir},
		"async": false,
		"force": true,
	})
	result, err = handleScan(ctx, scanReq, db, "")
	require.NoError(t, err)
	require.False(t, result.IsError, "scan should succeed")
	resp = resultJSON(t, result)
	assert.Equal(t, "completed", resp["status"])

	// 5. Query all files → verify count = 3
	queryFilesReq := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{
			"kind": "file",
		},
	})
	result, err = handleQuery(ctx, queryFilesReq, db)
	require.NoError(t, err)
	require.False(t, result.IsError, "query files should succeed")
	resp = resultJSON(t, result)
	assert.Equal(t, float64(3), resp["total"], "should find exactly 3 files")

	// 6. Aggregate sum of file sizes
	aggReq := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{
			"kind": "file",
		},
		"aggregate": "sum",
		"field":     "size",
	})
	result, err = handleQuery(ctx, aggReq, db)
	require.NoError(t, err)
	require.False(t, result.IsError, "aggregate query should succeed")
	resp = resultJSON(t, result)
	// file1.txt=11, file2.txt=11, file3.txt=19 → total=41
	assert.Equal(t, float64(41), resp["value"], "total file size should be 41 bytes")

	// 7. Query directories → verify count = 1 (subdir only, not tmpDir root)
	queryDirsReq := makeRequest("query", map[string]interface{}{
		"where": map[string]interface{}{
			"kind":   "directory",
			"parent": tmpDir,
		},
	})
	result, err = handleQuery(ctx, queryDirsReq, db)
	require.NoError(t, err)
	require.False(t, result.IsError, "query directories should succeed")
	resp = resultJSON(t, result)
	assert.Equal(t, float64(1), resp["total"], "should find exactly 1 subdirectory under tmpDir")

	// 8. Verify entries exist in DB directly
	entry, err := db.Get(filepath.Join(tmpDir, "file1.txt"))
	require.NoError(t, err)
	require.NotNil(t, entry, "file1.txt should exist in DB")
	assert.Equal(t, int64(11), entry.Size)
	assert.Equal(t, "file", entry.Kind)

	entry, err = db.Get(filepath.Join(tmpDir, "subdir"))
	require.NoError(t, err)
	require.NotNil(t, entry, "subdir should exist in DB")
	assert.Equal(t, "directory", entry.Kind)

	entry, err = db.Get(filepath.Join(tmpDir, "subdir", "file3.txt"))
	require.NoError(t, err)
	require.NotNil(t, entry, "file3.txt should exist in DB")
	assert.Equal(t, int64(19), entry.Size)
}
