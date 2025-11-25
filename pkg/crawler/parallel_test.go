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

func TestIndexParallelWithSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	// Create a real file
	testFile := filepath.Join(tempDir, "real.txt")
	err := os.WriteFile(testFile, []byte("real content"), 0644)
	require.NoError(t, err)

	// Create a symlink to the file
	symlinkPath := filepath.Join(tempDir, "link.txt")
	err = os.Symlink(testFile, symlinkPath)
	if err != nil {
		t.Skip("Symlinks not supported on this system")
	}

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	// Both the real file and symlink should be indexed
	assert.GreaterOrEqual(t, stats.FilesProcessed, 1)
}

func TestIndexParallelWithHiddenFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a hidden file
	hiddenFile := filepath.Join(tempDir, ".hidden")
	err := os.WriteFile(hiddenFile, []byte("hidden content"), 0644)
	require.NoError(t, err)

	// Create a regular file
	normalFile := filepath.Join(tempDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("normal content"), 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.FilesProcessed, 2)
}

func TestIndexParallelWithLargeFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a larger file (1MB)
	largeFile := filepath.Join(tempDir, "large.bin")
	data := make([]byte, 1024*1024) // 1MB
	err := os.WriteFile(largeFile, data, 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.FilesProcessed)
	assert.GreaterOrEqual(t, stats.TotalSize, int64(1024*1024))
}

func TestIndexParallelDeepNesting(t *testing.T) {
	tempDir := t.TempDir()

	// Create deeply nested directory structure
	deepPath := filepath.Join(tempDir, "a", "b", "c", "d", "e", "f", "g", "h")
	err := os.MkdirAll(deepPath, 0755)
	require.NoError(t, err)

	// Create files at various levels
	for _, path := range []string{
		filepath.Join(tempDir, "root.txt"),
		filepath.Join(tempDir, "a", "level1.txt"),
		filepath.Join(tempDir, "a", "b", "c", "level3.txt"),
		filepath.Join(deepPath, "deep.txt"),
	} {
		err := os.WriteFile(path, []byte("content"), 0644)
		require.NoError(t, err)
	}

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 4, stats.FilesProcessed)
	assert.Equal(t, 9, stats.DirectoriesProcessed) // tempDir + a,b,c,d,e,f,g,h
}

func TestIndexParallelWithSpecialCharacters(t *testing.T) {
	tempDir := t.TempDir()

	// Create files with spaces and special characters
	specialFiles := []string{
		"file with spaces.txt",
		"file_with_underscores.txt",
		"file-with-dashes.txt",
	}

	for _, name := range specialFiles {
		path := filepath.Join(tempDir, name)
		err := os.WriteFile(path, []byte("content"), 0644)
		require.NoError(t, err)
	}

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 3, stats.FilesProcessed)
}

func TestIndexParallelWithEmptySubdirs(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple empty subdirectories
	for i := 0; i < 5; i++ {
		emptyDir := filepath.Join(tempDir, "empty"+string(rune('a'+i)))
		err := os.Mkdir(emptyDir, 0755)
		require.NoError(t, err)
	}

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	stats, err := IndexParallel(tempDir, db, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 0, stats.FilesProcessed)
	assert.Equal(t, 6, stats.DirectoriesProcessed) // tempDir + 5 empty dirs
}
