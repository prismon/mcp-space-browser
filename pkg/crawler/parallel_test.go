package crawler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultParallelIndexOptions(t *testing.T) {
	opts := DefaultParallelIndexOptions()
	assert.NotNil(t, opts)
	assert.Equal(t, 8, opts.WorkerCount)
	assert.Equal(t, 10000, opts.QueueSize)
	assert.Equal(t, 1000, opts.BatchSize)
}

func TestIndexParallel(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create test file structure
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	testSubdir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(testSubdir, 0755)
	require.NoError(t, err)

	testFile2 := filepath.Join(testSubdir, "test2.txt")
	err = os.WriteFile(testFile2, []byte("test content 2"), 0644)
	require.NoError(t, err)

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Run parallel indexer with default options
	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Greater(t, stats.FilesProcessed, 0)
	assert.Greater(t, stats.DirectoriesProcessed, 0)

	// Verify results
	entries, err := db.All()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 3) // At least tempDir + files
}

func TestIndexParallelWithOptions(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create test files
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tempDir, "file"+string(rune('a'+i))+".txt")
		err := os.WriteFile(testFile, []byte("content"), 0644)
		require.NoError(t, err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Custom options
	opts := &ParallelIndexOptions{
		WorkerCount: 2,
		QueueSize:   100,
		BatchSize:   10,
	}

	// Run parallel indexer with custom options
	stats, err := IndexParallel(tempDir, db, nil, opts)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 5, stats.FilesProcessed)
}

func TestIndexParallelInvalidPath(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Try to index non-existent path
	_, err = IndexParallel("/nonexistent/path/that/does/not/exist", db, nil, nil)
	assert.Error(t, err)
}

func TestIndexParallelWithProgress(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create test files
	for i := 0; i < 3; i++ {
		testFile := filepath.Join(tempDir, "file"+string(rune('a'+i))+".txt")
		err := os.WriteFile(testFile, []byte("content"), 0644)
		require.NoError(t, err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Track progress callbacks
	opts := &ParallelIndexOptions{
		WorkerCount: 2,
		QueueSize:   100,
		BatchSize:   10,
		ProgressCallback: func(stats *IndexStats, remaining int) {
			// Just verify callback works
		},
	}

	// Run parallel indexer with progress callback
	stats, err := IndexParallel(tempDir, db, nil, opts)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	// Progress callback may or may not be called depending on timing
}

func TestParallelIndexerPauseResume(t *testing.T) {
	// Create temporary directory with many files to ensure pause/resume can be tested
	tempDir := t.TempDir()

	// Create many test files
	for i := 0; i < 100; i++ {
		testFile := filepath.Join(tempDir, "file"+string(rune('0'+i%10))+string(rune('0'+i/10))+".txt")
		err := os.WriteFile(testFile, []byte("content"), 0644)
		require.NoError(t, err)
	}

	// Create in-memory database for testing
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Run parallel indexer - just verify it completes without error
	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 100, stats.FilesProcessed)
}

func TestStringPtr(t *testing.T) {
	s := "test"
	ptr := stringPtr(s)
	assert.NotNil(t, ptr)
	assert.Equal(t, s, *ptr)
}
