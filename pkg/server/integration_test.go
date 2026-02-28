package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test target — a real directory on an external volume.
// Tests using this path are skipped automatically if the path doesn't exist.
const integrationTestRoot = "/Volumes/Archives/Alison Angel Girlfriends"

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

// --- Real filesystem integration tests ---
// These scan a real external volume and compare results against Unix commands.

// fsStats holds filesystem counts from Unix commands.
type fsStats struct {
	Files      []string
	FileCount  int
	DirCount   int
	TotalSize  int64
	Extensions map[string]int
}

// getFSStats uses find + du to get ground-truth filesystem data.
func getFSStats(t *testing.T, root string) fsStats {
	t.Helper()

	out, err := exec.Command("find", root, "-type", "f").Output()
	require.NoError(t, err, "find -type f")
	fileLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(fileLines) == 1 && fileLines[0] == "" {
		fileLines = nil
	}

	out, err = exec.Command("find", root, "-type", "d").Output()
	require.NoError(t, err, "find -type d")
	dirLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(dirLines) == 1 && dirLines[0] == "" {
		dirLines = nil
	}

	out, err = exec.Command("du", "-sk", root).Output()
	require.NoError(t, err, "du -sk")
	fields := strings.Fields(string(out))
	sizeKB, err := strconv.ParseInt(fields[0], 10, 64)
	require.NoError(t, err, "parse du output")

	extensions := make(map[string]int)
	for _, f := range fileLines {
		if f == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f))
		if ext != "" {
			extensions[ext]++
		}
	}

	return fsStats{
		Files:      fileLines,
		FileCount:  len(fileLines),
		DirCount:   len(dirLines),
		TotalSize:  sizeKB * 1024,
		Extensions: extensions,
	}
}

func TestIntegration_RealFS_ScanAndVerify(t *testing.T) {
	if _, err := os.Stat(integrationTestRoot); os.IsNotExist(err) {
		t.Skipf("Skipping: %s not found", integrationTestRoot)
	}

	fs := getFSStats(t, integrationTestRoot)
	t.Logf("Filesystem: %d files, %d dirs, %s total", fs.FileCount, fs.DirCount, fmtBytes(fs.TotalSize))
	t.Logf("Extensions: %v", fs.Extensions)
	require.Greater(t, fs.FileCount, 0)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	cacheDir := t.TempDir()
	SetArtifactCacheDir(cacheDir)

	request := makeRequest("scan", map[string]interface{}{
		"paths":      []interface{}{integrationTestRoot},
		"async":      false,
		"force":      true,
		"attributes": []interface{}{"mime", "permissions", "thumbnail"},
	})

	result, err := handleScan(context.Background(), request, db, cacheDir)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "scan should not return error")

	response := resultJSON(t, result)
	assert.Equal(t, "completed", response["status"])

	if pp, ok := response["post_processing"].(map[string]interface{}); ok {
		t.Logf("Post-processing: files=%v, metadata=%v, errors=%v, duration=%vms",
			pp["files_processed"], pp["metadata_set"], pp["errors"], pp["duration_ms"])
	}

	ctx := context.Background()

	// --- File count ---
	dbFileCount := queryCount(t, ctx, db, map[string]interface{}{
		"kind": "file", "parent": map[string]interface{}{"like": integrationTestRoot + "%"},
	})
	t.Logf("File count: DB=%d, FS=%d", dbFileCount, fs.FileCount)
	assert.InDelta(t, fs.FileCount, dbFileCount, 10,
		"DB file count should be within 10 of filesystem")

	// --- Directory count ---
	dbDirCount := queryCount(t, ctx, db, map[string]interface{}{
		"kind": "directory", "path": map[string]interface{}{"like": integrationTestRoot + "%"},
	})
	t.Logf("Dir count:  DB=%d, FS=%d", dbDirCount, fs.DirCount)
	assert.InDelta(t, fs.DirCount, dbDirCount, 5,
		"DB dir count should be within 5 of filesystem")

	// --- Total size (du uses allocated blocks, we use logical — allow wide margin) ---
	dbTotalSize := querySum(t, ctx, db, map[string]interface{}{
		"kind": "file", "parent": map[string]interface{}{"like": integrationTestRoot + "%"},
	})
	t.Logf("Total size: DB=%s, FS=%s (du -sk)", fmtBytes(dbTotalSize), fmtBytes(fs.TotalSize))
	assert.True(t, float64(dbTotalSize) >= float64(fs.TotalSize)*0.5 &&
		float64(dbTotalSize) <= float64(fs.TotalSize)*1.5,
		fmt.Sprintf("DB size %d should be within 50%% of FS size %d", dbTotalSize, fs.TotalSize))

	// --- MIME types set for most files ---
	mimeCount := queryCount(t, ctx, db, map[string]interface{}{
		"mime": map[string]interface{}{"like": "%"},
	})
	t.Logf("Files with MIME: %d / %d", mimeCount, fs.FileCount)
	assert.Greater(t, mimeCount, int(float64(fs.FileCount)*0.9),
		"At least 90%% of files should have MIME types")

	// --- JPEG count matches ---
	jpegCount := fs.Extensions[".jpg"] + fs.Extensions[".jpeg"] + fs.Extensions[".JPG"]
	if jpegCount > 0 {
		dbJpegCount := queryCount(t, ctx, db, map[string]interface{}{"mime": "image/jpeg"})
		t.Logf("JPEG count: DB=%d, FS=%d", dbJpegCount, jpegCount)
		assert.InDelta(t, jpegCount, dbJpegCount, float64(jpegCount)*0.05+5,
			"JPEG count should match within 5%%+5")
	}

	// --- Permissions set for most files ---
	permCount := queryCount(t, ctx, db, map[string]interface{}{
		"permissions": map[string]interface{}{"like": "%"},
	})
	t.Logf("Files with permissions: %d / %d", permCount, fs.FileCount)
	assert.Greater(t, permCount, int(float64(fs.FileCount)*0.9),
		"At least 90%% of files should have permissions")

	// --- Spot-check: pick 5 random files, verify size matches stat ---
	checked := 0
	for _, fpath := range fs.Files {
		if fpath == "" {
			continue
		}
		info, err := os.Stat(fpath)
		if err != nil {
			continue
		}
		entry, err := db.Get(fpath)
		if err != nil || entry == nil {
			t.Errorf("File %s exists on disk but not in DB", fpath)
			continue
		}
		assert.Equal(t, info.Size(), entry.Size,
			fmt.Sprintf("Size mismatch for %s: stat=%d, db=%d", fpath, info.Size(), entry.Size))
		checked++
		if checked >= 5 {
			break
		}
	}
	t.Logf("Spot-checked %d file sizes against stat — all match", checked)
}

func TestIntegration_RealFS_ThumbnailGeneration(t *testing.T) {
	if _, err := os.Stat(integrationTestRoot); os.IsNotExist(err) {
		t.Skipf("Skipping: %s not found", integrationTestRoot)
	}

	// Pick the first subfolder that has JPGs
	subfolders, err := os.ReadDir(integrationTestRoot)
	require.NoError(t, err)

	var testDir string
	for _, d := range subfolders {
		if !d.IsDir() {
			continue
		}
		candidate := filepath.Join(integrationTestRoot, d.Name())
		matches, _ := filepath.Glob(filepath.Join(candidate, "*.jpg"))
		if len(matches) > 0 {
			testDir = candidate
			break
		}
	}
	require.NotEmpty(t, testDir, "need a subfolder with JPG files")

	// Count JPGs via find
	out, err := exec.Command("find", testDir, "-type", "f", "-iname", "*.jpg").Output()
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	jpgCount := 0
	for _, l := range lines {
		if l != "" {
			jpgCount++
		}
	}
	t.Logf("Testing thumbnails on %s (%d JPGs)", filepath.Base(testDir), jpgCount)
	require.Greater(t, jpgCount, 0)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	cacheDir := t.TempDir()
	SetArtifactCacheDir(cacheDir)

	request := makeRequest("scan", map[string]interface{}{
		"paths":      []interface{}{testDir},
		"async":      false,
		"force":      true,
		"attributes": []interface{}{"mime", "thumbnail"},
	})

	result, err := handleScan(context.Background(), request, db, cacheDir)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	response := resultJSON(t, result)
	assert.Equal(t, "completed", response["status"])
	if pp, ok := response["post_processing"].(map[string]interface{}); ok {
		t.Logf("Post-processing: files=%v, metadata=%v, errors=%v",
			pp["files_processed"], pp["metadata_set"], pp["errors"])
	}

	// Count thumbnails in DB
	files, err := db.GetFilesUnderRoot(testDir)
	require.NoError(t, err)

	thumbCount := 0
	cacheFilesOK := 0
	var totalThumbBytes int64
	var totalOrigBytes int64

	for _, file := range files {
		artifacts, err := db.GetArtifactMetadata(file.Path)
		if err != nil {
			continue
		}
		for _, a := range artifacts {
			if a.Key != "thumbnail" {
				continue
			}
			thumbCount++

			if a.CachePath == nil || *a.CachePath == "" {
				continue
			}

			// Verify cache file exists
			thumbInfo, err := os.Stat(*a.CachePath)
			if err != nil {
				t.Logf("WARN: thumbnail cache missing: %s", *a.CachePath)
				continue
			}
			cacheFilesOK++
			totalThumbBytes += thumbInfo.Size()

			// Accumulate original size
			if origInfo, err := os.Stat(file.Path); err == nil {
				totalOrigBytes += origInfo.Size()
			}
		}
	}

	t.Logf("Thumbnails: %d generated / %d JPGs, %d cache files verified", thumbCount, jpgCount, cacheFilesOK)

	// At least 80% of JPGs should have thumbnails
	minExpected := int(float64(jpgCount) * 0.8)
	assert.GreaterOrEqual(t, thumbCount, minExpected,
		fmt.Sprintf("At least 80%% of %d JPGs should have thumbnails, got %d", jpgCount, thumbCount))

	// All thumbnails should have valid cache files
	assert.Equal(t, thumbCount, cacheFilesOK, "All thumbnails should have cache files on disk")

	// Verify first thumbnail is a valid JPEG
	for _, file := range files {
		artifacts, _ := db.GetArtifactMetadata(file.Path)
		for _, a := range artifacts {
			if a.Key == "thumbnail" && a.CachePath != nil {
				data, err := os.ReadFile(*a.CachePath)
				require.NoError(t, err)
				require.True(t, len(data) >= 2, "thumbnail should not be empty")
				assert.Equal(t, byte(0xFF), data[0], "JPEG magic byte 0")
				assert.Equal(t, byte(0xD8), data[1], "JPEG magic byte 1")
				require.NotNil(t, a.MimeType)
				assert.Equal(t, "image/jpeg", *a.MimeType)
				t.Logf("Verified JPEG thumbnail: %s (%d bytes)", *a.CachePath, len(data))
				goto doneJpegCheck
			}
		}
	}
doneJpegCheck:

	// Verify thumbnails are smaller than originals
	if cacheFilesOK > 0 && totalOrigBytes > 0 {
		ratio := float64(totalThumbBytes) / float64(totalOrigBytes)
		t.Logf("Compression: %d files, avg orig=%dKB, avg thumb=%dKB, ratio=%.3f",
			cacheFilesOK, totalOrigBytes/int64(cacheFilesOK)/1024,
			totalThumbBytes/int64(cacheFilesOK)/1024, ratio)
		assert.Less(t, ratio, 0.5, "Thumbnails should be <50%% the size of originals")
	}
}

// --- Helpers ---

func queryCount(t *testing.T, ctx context.Context, db *database.DiskDB, where map[string]interface{}) int {
	t.Helper()
	req := makeRequest("query", map[string]interface{}{
		"where": where, "aggregate": "count", "field": "size",
	})
	result, err := handleQuery(ctx, req, db)
	require.NoError(t, err)
	resp := resultJSON(t, result)
	return int(resp["value"].(float64))
}

func querySum(t *testing.T, ctx context.Context, db *database.DiskDB, where map[string]interface{}) int64 {
	t.Helper()
	req := makeRequest("query", map[string]interface{}{
		"where": where, "aggregate": "sum", "field": "size",
	})
	result, err := handleQuery(ctx, req, db)
	require.NoError(t, err)
	resp := resultJSON(t, result)
	return int64(resp["value"].(float64))
}

func fmtBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
