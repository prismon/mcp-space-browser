package crawler

import (
	"fmt"
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

// TestIndexSpaceUsageTreeCalculation creates a realistic directory structure with files
// of varying sizes and verifies that the size aggregation works correctly for
// treemap/radial visualizations. This is an end-to-end test that validates:
// 1. Files are indexed with correct sizes
// 2. Directory sizes are computed as the sum of their children
// 3. Nested directories propagate sizes correctly up the tree
// 4. Mixed content (files + subdirectories) in the same directory works correctly
func TestIndexSpaceUsageTreeCalculation(t *testing.T) {
	tempDir := t.TempDir()

	// Create a realistic directory structure for space visualization:
	//
	// tempDir/                        (expected total: 14000 bytes)
	// ├── documents/                  (expected: 6000 bytes)
	// │   ├── work/                   (expected: 3500 bytes)
	// │   │   ├── report.pdf          (2000 bytes)
	// │   │   └── notes.txt           (1500 bytes)
	// │   └── personal/               (expected: 2500 bytes)
	// │       ├── budget.xlsx         (1000 bytes)
	// │       └── todo.txt            (1500 bytes)
	// ├── media/                      (expected: 8000 bytes)
	// │   ├── photos/                 (expected: 5000 bytes)
	// │   │   ├── vacation/           (expected: 3000 bytes)
	// │   │   │   ├── beach.jpg       (1500 bytes)
	// │   │   │   └── mountain.jpg    (1500 bytes)
	// │   │   └── family.jpg          (2000 bytes)
	// │   └── videos/                 (expected: 3000 bytes)
	// │       └── clip.mp4            (3000 bytes)
	// └── empty_folder/               (expected: 0 bytes)

	// Structure definition: path -> size (0 for directories)
	structure := []struct {
		path string
		size int // 0 for directories
	}{
		// Directories first (to ensure parents exist)
		{"documents", 0},
		{"documents/work", 0},
		{"documents/personal", 0},
		{"media", 0},
		{"media/photos", 0},
		{"media/photos/vacation", 0},
		{"media/videos", 0},
		{"empty_folder", 0},

		// Files with specific sizes
		{"documents/work/report.pdf", 2000},
		{"documents/work/notes.txt", 1500},
		{"documents/personal/budget.xlsx", 1000},
		{"documents/personal/todo.txt", 1500},
		{"media/photos/vacation/beach.jpg", 1500},
		{"media/photos/vacation/mountain.jpg", 1500},
		{"media/photos/family.jpg", 2000},
		{"media/videos/clip.mp4", 3000},
	}

	// Create the directory structure
	for _, item := range structure {
		fullPath := filepath.Join(tempDir, item.path)
		if item.size == 0 {
			// Create directory
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				t.Fatalf("Failed to create directory %s: %v", item.path, err)
			}
		} else {
			// Create file with exact size
			data := make([]byte, item.size)
			// Fill with recognizable pattern
			for i := range data {
				data[i] = byte(i % 256)
			}
			err := os.WriteFile(fullPath, data, 0644)
			if err != nil {
				t.Fatalf("Failed to create file %s: %v", item.path, err)
			}
		}
	}

	// Create database and index
	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts := &IndexOptions{Force: true, MaxAge: DefaultMaxAge}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Verify file counts
	assert.Equal(t, 8, stats.FilesProcessed, "Should have processed 8 files")
	assert.Equal(t, 9, stats.DirectoriesProcessed, "Should have processed 9 directories (including root)")

	// Verify total size reported during indexing
	expectedTotalSize := int64(2000 + 1500 + 1000 + 1500 + 1500 + 1500 + 2000 + 3000)
	assert.Equal(t, expectedTotalSize, stats.TotalSize, "Total size from stats should match")

	// Now verify the tree structure sizes after aggregation

	// Expected sizes for each node in the tree (for treemap/radial)
	expectedSizes := map[string]int64{
		// Root (2000+1500+1000+1500+1500+1500+2000+3000 = 14000)
		tempDir: 14000,

		// Level 1 directories
		filepath.Join(tempDir, "documents"):    6000,
		filepath.Join(tempDir, "media"):        8000,
		filepath.Join(tempDir, "empty_folder"): 0,

		// Level 2 directories
		filepath.Join(tempDir, "documents/work"):      3500,
		filepath.Join(tempDir, "documents/personal"):  2500,
		filepath.Join(tempDir, "media/photos"):        5000,
		filepath.Join(tempDir, "media/videos"):        3000,

		// Level 3 directories
		filepath.Join(tempDir, "media/photos/vacation"): 3000,

		// Files
		filepath.Join(tempDir, "documents/work/report.pdf"):       2000,
		filepath.Join(tempDir, "documents/work/notes.txt"):        1500,
		filepath.Join(tempDir, "documents/personal/budget.xlsx"):  1000,
		filepath.Join(tempDir, "documents/personal/todo.txt"):     1500,
		filepath.Join(tempDir, "media/photos/vacation/beach.jpg"): 1500,
		filepath.Join(tempDir, "media/photos/vacation/mountain.jpg"): 1500,
		filepath.Join(tempDir, "media/photos/family.jpg"):         2000,
		filepath.Join(tempDir, "media/videos/clip.mp4"):           3000,
	}

	// Verify each path has the expected size
	for path, expectedSize := range expectedSizes {
		entry, err := db.Get(path)
		assert.NoError(t, err, "Should be able to get entry for %s", path)
		assert.NotNil(t, entry, "Entry should exist for %s", path)
		assert.Equal(t, expectedSize, entry.Size, "Size mismatch for %s (expected %d, got %d)", path, expectedSize, entry.Size)
	}

	// Verify parent-child relationships are consistent
	// Each parent's size should equal the sum of its direct children's sizes
	parentChildSums := map[string][]string{
		tempDir: {
			filepath.Join(tempDir, "documents"),
			filepath.Join(tempDir, "media"),
			filepath.Join(tempDir, "empty_folder"),
		},
		filepath.Join(tempDir, "documents"): {
			filepath.Join(tempDir, "documents/work"),
			filepath.Join(tempDir, "documents/personal"),
		},
		filepath.Join(tempDir, "documents/work"): {
			filepath.Join(tempDir, "documents/work/report.pdf"),
			filepath.Join(tempDir, "documents/work/notes.txt"),
		},
		filepath.Join(tempDir, "documents/personal"): {
			filepath.Join(tempDir, "documents/personal/budget.xlsx"),
			filepath.Join(tempDir, "documents/personal/todo.txt"),
		},
		filepath.Join(tempDir, "media"): {
			filepath.Join(tempDir, "media/photos"),
			filepath.Join(tempDir, "media/videos"),
		},
		filepath.Join(tempDir, "media/photos"): {
			filepath.Join(tempDir, "media/photos/vacation"),
			filepath.Join(tempDir, "media/photos/family.jpg"),
		},
		filepath.Join(tempDir, "media/photos/vacation"): {
			filepath.Join(tempDir, "media/photos/vacation/beach.jpg"),
			filepath.Join(tempDir, "media/photos/vacation/mountain.jpg"),
		},
		filepath.Join(tempDir, "media/videos"): {
			filepath.Join(tempDir, "media/videos/clip.mp4"),
		},
	}

	for parent, children := range parentChildSums {
		parentEntry, err := db.Get(parent)
		assert.NoError(t, err)

		var childSum int64
		for _, child := range children {
			childEntry, err := db.Get(child)
			assert.NoError(t, err)
			childSum += childEntry.Size
		}

		assert.Equal(t, childSum, parentEntry.Size,
			"Parent %s size (%d) should equal sum of children (%d)", parent, parentEntry.Size, childSum)
	}
}

// TestIndexSpaceUsageWithVaryingSizes tests with files of dramatically different sizes
// to ensure the aggregation handles edge cases correctly
func TestIndexSpaceUsageWithVaryingSizes(t *testing.T) {
	tempDir := t.TempDir()

	// Create structure with extreme size variations:
	// - Very small files (1 byte)
	// - Medium files (1KB)
	// - Larger files (100KB)
	// - Empty files (0 bytes)

	structure := []struct {
		path string
		size int
	}{
		{"small", 0},
		{"medium", 0},
		{"large", 0},
		{"mixed", 0},

		// Small files (1 byte each)
		{"small/tiny1.txt", 1},
		{"small/tiny2.txt", 1},
		{"small/tiny3.txt", 1},

		// Medium files (1KB each)
		{"medium/file1.dat", 1024},
		{"medium/file2.dat", 1024},

		// Large files (100KB each)
		{"large/big1.bin", 102400},
		{"large/big2.bin", 102400},

		// Mixed sizes including empty file
		{"mixed/empty.txt", 0},
		{"mixed/small.txt", 10},
		{"mixed/medium.dat", 5000},
	}

	for _, item := range structure {
		fullPath := filepath.Join(tempDir, item.path)
		// Directories have no extension and size 0
		if item.size == 0 && filepath.Ext(item.path) == "" {
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				t.Fatalf("Failed to create directory %s: %v", item.path, err)
			}
			continue
		}
		// Ensure parent directory exists
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create parent directory for %s: %v", item.path, err)
		}
		// Create file
		data := make([]byte, item.size)
		err = os.WriteFile(fullPath, data, 0644)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", item.path, err)
		}
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts := &IndexOptions{Force: true, MaxAge: DefaultMaxAge}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)

	// Expected sizes
	expectedSizes := map[string]int64{
		filepath.Join(tempDir, "small"):  3,                          // 1+1+1
		filepath.Join(tempDir, "medium"): 2048,                       // 1024+1024
		filepath.Join(tempDir, "large"):  204800,                     // 102400+102400
		filepath.Join(tempDir, "mixed"):  5010,                       // 0+10+5000
		tempDir:                          3 + 2048 + 204800 + 5010,   // sum of all
	}

	for path, expectedSize := range expectedSizes {
		entry, err := db.Get(path)
		assert.NoError(t, err)
		assert.Equal(t, expectedSize, entry.Size, "Size mismatch for %s", path)
	}

	// Verify empty file is tracked correctly
	emptyEntry, err := db.Get(filepath.Join(tempDir, "mixed/empty.txt"))
	assert.NoError(t, err)
	assert.Equal(t, int64(0), emptyEntry.Size, "Empty file should have size 0")
	assert.Equal(t, "file", emptyEntry.Kind)

	// Verify total matches stats
	assert.Equal(t, stats.TotalSize, expectedSizes[tempDir])
}

// TestIndexSpaceUsageDeepHierarchy tests size calculation in a deeply nested structure
func TestIndexSpaceUsageDeepHierarchy(t *testing.T) {
	tempDir := t.TempDir()

	// Create a 10-level deep hierarchy with files at each level
	// Each file is 100 bytes
	levels := 10
	fileSize := 100

	currentPath := tempDir
	for i := 0; i < levels; i++ {
		// Create directory at this level
		levelDir := filepath.Join(currentPath, fmt.Sprintf("level%d", i))
		err := os.MkdirAll(levelDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory at level %d: %v", i, err)
		}

		// Create a file at this level
		filePath := filepath.Join(levelDir, fmt.Sprintf("file%d.txt", i))
		data := make([]byte, fileSize)
		err = os.WriteFile(filePath, data, 0644)
		if err != nil {
			t.Fatalf("Failed to create file at level %d: %v", i, err)
		}

		currentPath = levelDir
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts := &IndexOptions{Force: true, MaxAge: DefaultMaxAge}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)

	// Verify we processed all files and directories
	assert.Equal(t, levels, stats.FilesProcessed)
	assert.Equal(t, levels+1, stats.DirectoriesProcessed) // levels + root

	// Total size should be levels * fileSize
	expectedTotal := int64(levels * fileSize)
	assert.Equal(t, expectedTotal, stats.TotalSize)

	// Verify root has accumulated all sizes
	rootEntry, err := db.Get(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, expectedTotal, rootEntry.Size)

	// Verify each level has correct accumulated size
	// level0 should have all 10 files (1000 bytes)
	// level9 should have just 1 file (100 bytes)
	currentPath = tempDir
	for i := 0; i < levels; i++ {
		levelDir := filepath.Join(currentPath, fmt.Sprintf("level%d", i))
		entry, err := db.Get(levelDir)
		assert.NoError(t, err)

		// This level and all levels below it have files
		expectedSize := int64((levels - i) * fileSize)
		assert.Equal(t, expectedSize, entry.Size,
			"Level %d should have size %d (contains %d files below)", i, expectedSize, levels-i)

		currentPath = levelDir
	}
}

// TestIndexSpaceUsageWideBranching tests size calculation with many siblings
func TestIndexSpaceUsageWideBranching(t *testing.T) {
	tempDir := t.TempDir()

	// Create a wide structure: 1 parent with 100 children (files)
	// Each file is 50 bytes
	numChildren := 100
	fileSize := 50

	for i := 0; i < numChildren; i++ {
		filePath := filepath.Join(tempDir, fmt.Sprintf("file_%03d.txt", i))
		data := make([]byte, fileSize)
		err := os.WriteFile(filePath, data, 0644)
		if err != nil {
			t.Fatalf("Failed to create file %d: %v", i, err)
		}
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts := &IndexOptions{Force: true, MaxAge: DefaultMaxAge}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)

	assert.Equal(t, numChildren, stats.FilesProcessed)
	assert.Equal(t, 1, stats.DirectoriesProcessed) // just root

	// Verify root size
	expectedTotal := int64(numChildren * fileSize)
	assert.Equal(t, expectedTotal, stats.TotalSize)

	rootEntry, err := db.Get(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, expectedTotal, rootEntry.Size)
}

// TestIndexSpaceUsageMultipleSubdirectoriesWithFiles tests size calculation
// with multiple subdirectories each containing multiple files
func TestIndexSpaceUsageMultipleSubdirectoriesWithFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create structure:
	// root/
	//   subdir1/ (3 files: 100, 200, 300 = 600 bytes)
	//   subdir2/ (2 files: 400, 500 = 900 bytes)
	//   subdir3/ (4 files: 50, 50, 50, 50 = 200 bytes)
	// Total: 1700 bytes

	subdirs := []struct {
		name  string
		files []int // sizes
	}{
		{"subdir1", []int{100, 200, 300}},
		{"subdir2", []int{400, 500}},
		{"subdir3", []int{50, 50, 50, 50}},
	}

	var totalExpected int64
	subdirExpected := make(map[string]int64)

	for _, subdir := range subdirs {
		subdirPath := filepath.Join(tempDir, subdir.name)
		err := os.MkdirAll(subdirPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", subdir.name, err)
		}

		var subdirTotal int64
		for i, size := range subdir.files {
			filePath := filepath.Join(subdirPath, fmt.Sprintf("file%d.dat", i))
			data := make([]byte, size)
			err := os.WriteFile(filePath, data, 0644)
			if err != nil {
				t.Fatalf("Failed to create file in %s: %v", subdir.name, err)
			}
			subdirTotal += int64(size)
		}
		subdirExpected[subdirPath] = subdirTotal
		totalExpected += subdirTotal
	}

	db, err := database.NewDiskDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts := &IndexOptions{Force: true, MaxAge: DefaultMaxAge}
	stats, err := IndexWithOptions(tempDir, db, nil, 0, nil, opts)
	assert.NoError(t, err)

	// Verify subdirectory sizes
	for path, expected := range subdirExpected {
		entry, err := db.Get(path)
		assert.NoError(t, err)
		assert.Equal(t, expected, entry.Size, "Subdirectory %s size mismatch", path)
	}

	// Verify root total
	rootEntry, err := db.Get(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, totalExpected, rootEntry.Size)
	assert.Equal(t, totalExpected, stats.TotalSize)
}
