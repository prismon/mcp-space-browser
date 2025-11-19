package crawler

import (
	"os"
	"path/filepath"
	"testing"

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
	stats, err := Index(tempDir, db, 0)
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
	stats, err := Index(tempDir, db, 0)
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

	// Second index (incremental)
	_, err = Index(tempDir, db, 0)
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
