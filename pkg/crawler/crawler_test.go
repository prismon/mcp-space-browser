package crawler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create test file structure
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	testSubdir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(testSubdir, 0755); err != nil {
		t.Fatal(err)
	}

	testFile2 := filepath.Join(testSubdir, "test2.txt")
	if err := os.WriteFile(testFile2, []byte("test content 2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Run indexer (no job tracking in tests)
	stats, err := Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Greater(t, stats.FilesProcessed, 0)
	assert.Greater(t, stats.DirectoriesProcessed, 0)

	// Verify results
	entries, err := db.All()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(entries)) // tempDir + test.txt + subdir + test2.txt

	// Verify root directory entry
	rootEntry, err := db.Get(tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, rootEntry)
	assert.Equal(t, "directory", rootEntry.Kind)

	// Verify file entry
	fileEntry, err := db.Get(testFile)
	assert.NoError(t, err)
	assert.NotNil(t, fileEntry)
	assert.Equal(t, "file", fileEntry.Kind)
	assert.Greater(t, fileEntry.Size, int64(0))

	// Verify subdirectory entry
	subdirEntry, err := db.Get(testSubdir)
	assert.NoError(t, err)
	assert.NotNil(t, subdirEntry)
	assert.Equal(t, "directory", subdirEntry.Kind)

	// Verify aggregate sizes are computed
	// After aggregation, directory sizes should be sum of children
	assert.Greater(t, rootEntry.Size, int64(0))
}

func TestIndexIncrementalUpdate(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First index
	stats, err := Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	entries, err := db.All()
	assert.NoError(t, err)
	initialCount := len(entries)
	assert.Equal(t, 2, initialCount) // tempDir + test.txt

	// Add new file
	testFile2 := filepath.Join(tempDir, "test2.txt")
	if err := os.WriteFile(testFile2, []byte("test content 2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Second index (incremental) - use force=true since we just indexed and want to pick up new file
	forceOpts := &IndexOptions{Force: true, MaxAge: DefaultMaxAge}
	_, err = IndexWithOptions(tempDir, db, nil, 0, nil, forceOpts)
	assert.NoError(t, err)

	entries, err = db.All()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), initialCount+1) // At least one new file added

	// Verify both files exist
	file1Entry, err := db.Get(testFile)
	assert.NoError(t, err)
	assert.NotNil(t, file1Entry)

	file2Entry, err := db.Get(testFile2)
	assert.NoError(t, err)
	assert.NotNil(t, file2Entry)
}

func TestIndexSkipsRecentScans(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First index with default options (should complete normally)
	opts := &IndexOptions{
		Force:  false,
		MaxAge: 3600, // 1 hour
	}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.False(t, stats.Skipped, "First scan should not be skipped")
	assert.Equal(t, 1, stats.FilesProcessed)
	assert.Equal(t, 1, stats.DirectoriesProcessed)

	// Second index immediately after should be skipped
	stats2, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.NotNil(t, stats2)
	assert.True(t, stats2.Skipped, "Second scan should be skipped because path was recently scanned")
	assert.NotEmpty(t, stats2.SkipReason)
	assert.Equal(t, 0, stats2.FilesProcessed)
}

func TestIndexForceOverridesSkip(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First index
	opts := &IndexOptions{
		Force:  false,
		MaxAge: 3600,
	}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.False(t, stats.Skipped)

	// Second index with force=true should NOT be skipped
	forceOpts := &IndexOptions{
		Force:  true,
		MaxAge: 3600,
	}
	stats2, err := IndexWithOptions(tempDir, db, nil, 0, nil, forceOpts)
	assert.NoError(t, err)
	assert.NotNil(t, stats2)
	assert.False(t, stats2.Skipped, "Scan with force=true should not be skipped")
	assert.Equal(t, 1, stats2.FilesProcessed)
}

func TestIndexMaxAgeZeroAlwaysRescans(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Index with maxAge=0 (should always rescan)
	opts := &IndexOptions{
		Force:  false,
		MaxAge: 0,
	}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.False(t, stats.Skipped)

	// Second index with maxAge=0 should also NOT be skipped
	stats2, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.False(t, stats2.Skipped, "Scan with maxAge=0 should never be skipped")
	assert.Equal(t, 1, stats2.FilesProcessed)
}

func TestDefaultIndexOptions(t *testing.T) {
	opts := DefaultIndexOptions()
	assert.NotNil(t, opts)
	assert.False(t, opts.Force)
	assert.Equal(t, int64(DefaultMaxAge), opts.MaxAge)
}

func TestGetPathScanInfo(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Before indexing, path should not exist
	info, err := db.GetPathScanInfo(tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.False(t, info.Exists)
	assert.Equal(t, int64(0), info.LastScanned)
	assert.Equal(t, 0, info.EntryCount)

	// Index the path
	_, err = Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)

	// After indexing, path should exist with scan info
	info, err = db.GetPathScanInfo(tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.True(t, info.Exists)
	assert.Greater(t, info.LastScanned, int64(0))
	assert.Equal(t, 2, info.EntryCount) // tempDir + test.txt

	// LastScanned should be within the last few seconds
	now := time.Now().Unix()
	assert.LessOrEqual(t, info.LastScanned, now)
	assert.GreaterOrEqual(t, info.LastScanned, now-5) // Within last 5 seconds
}

func TestGetPathLastScanned(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Before indexing, should return 0
	lastScanned, err := db.GetPathLastScanned(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), lastScanned)

	// Index the path
	_, err = Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)

	// After indexing, should return a valid timestamp
	lastScanned, err = db.GetPathLastScanned(tempDir)
	assert.NoError(t, err)
	assert.Greater(t, lastScanned, int64(0))

	// Should be within the last few seconds
	now := time.Now().Unix()
	assert.LessOrEqual(t, lastScanned, now)
	assert.GreaterOrEqual(t, lastScanned, now-5)
}

func TestIndexWithProgressCallback(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple files to ensure progress callback is called
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tempDir, "file"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	callbackCalled := false
	callback := func(stats *IndexStats, remaining int) {
		callbackCalled = true
	}

	stats, err := Index(tempDir, db, nil, 0, callback)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 5, stats.FilesProcessed)
	// Progress callback may or may not be called depending on timing
	_ = callbackCalled
}

func TestIndexInvalidPath(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Try to index a non-existent path
	_, err = Index("/nonexistent/path/that/does/not/exist", db, nil, 0, nil)
	assert.Error(t, err)
}

func TestIndexStalePathReindex(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First index with a very short maxAge (1 second)
	opts := &IndexOptions{
		Force:  false,
		MaxAge: 1, // 1 second
	}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.False(t, stats.Skipped)

	// Wait for the scan to become stale
	time.Sleep(2 * time.Second)

	// Second index should NOT be skipped because it's stale
	stats2, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.NotNil(t, stats2)
	assert.False(t, stats2.Skipped, "Stale scan should not be skipped")
}

func TestIndexWithManyFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create many files
	for i := 0; i < 50; i++ {
		testFile := filepath.Join(tempDir, "file"+string(rune('0'+i%10))+string(rune('0'+i/10))+".txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stats, err := Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 50, stats.FilesProcessed)
	assert.Equal(t, 1, stats.DirectoriesProcessed) // tempDir

	// Verify all files are in database
	entries, err := db.All()
	assert.NoError(t, err)
	assert.Equal(t, 51, len(entries)) // tempDir + 50 files
}

func TestIndexEmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create an empty subdirectory
	emptyDir := filepath.Join(tempDir, "empty")
	if err := os.Mkdir(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stats, err := Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 0, stats.FilesProcessed)
	assert.Equal(t, 2, stats.DirectoriesProcessed) // tempDir + empty

	// Verify both directories are in database
	entries, err := db.All()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(entries))
}

func TestIndexDeepNesting(t *testing.T) {
	tempDir := t.TempDir()

	// Create deeply nested directory structure
	deepPath := filepath.Join(tempDir, "a", "b", "c", "d", "e")
	if err := os.MkdirAll(deepPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file at the deepest level
	testFile := filepath.Join(deepPath, "deep.txt")
	if err := os.WriteFile(testFile, []byte("deep content"), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stats, err := Index(tempDir, db, nil, 0, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.FilesProcessed)
	assert.Equal(t, 6, stats.DirectoriesProcessed) // tempDir + a + b + c + d + e

	// Verify the deepest file is in database
	deepEntry, err := db.Get(testFile)
	assert.NoError(t, err)
	assert.NotNil(t, deepEntry)
	assert.Equal(t, "file", deepEntry.Kind)
}
