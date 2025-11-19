package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiskDB(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify tables were created
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	assert.NoError(t, err)
	assert.Greater(t, count, 0)
}

func TestInsertOrUpdateEntry(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	entry := &models.Entry{
		Path:        "/test/file.txt",
		Parent:      stringPtr("/test"),
		Size:        1024,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}

	err = db.InsertOrUpdate(entry)
	assert.NoError(t, err)

	// Verify entry was inserted
	retrieved, err := db.Get("/test/file.txt")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "/test/file.txt", retrieved.Path)
	assert.Equal(t, int64(1024), retrieved.Size)
	assert.Equal(t, "file", retrieved.Kind)
}

func TestGetEntry(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test getting non-existent entry
	entry, err := db.Get("/nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, entry)

	// Insert and retrieve
	now := time.Now().Unix()
	testEntry := &models.Entry{
		Path:        "/test/dir",
		Size:        0,
		Kind:        "directory",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}

	err = db.InsertOrUpdate(testEntry)
	require.NoError(t, err)

	retrieved, err := db.Get("/test/dir")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "directory", retrieved.Kind)
}

func TestChildren(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create parent directory
	parent := &models.Entry{
		Path:        "/parent",
		Size:        0,
		Kind:        "directory",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(parent)

	// Create children
	child1 := &models.Entry{
		Path:        "/parent/child1.txt",
		Parent:      stringPtr("/parent"),
		Size:        100,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	child2 := &models.Entry{
		Path:        "/parent/child2.txt",
		Parent:      stringPtr("/parent"),
		Size:        200,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}

	db.InsertOrUpdate(child1)
	db.InsertOrUpdate(child2)

	// Get children
	children, err := db.Children("/parent")
	assert.NoError(t, err)
	assert.Len(t, children, 2)
}

func TestDeleteStale(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	oldTime := time.Now().Unix() - 1000
	newTime := time.Now().Unix()

	// Insert old entry
	oldEntry := &models.Entry{
		Path:        "/root/old.txt",
		Parent:      stringPtr("/root"),
		Size:        100,
		Kind:        "file",
		Ctime:       oldTime,
		Mtime:       oldTime,
		LastScanned: oldTime,
	}
	db.InsertOrUpdate(oldEntry)

	// Insert new entry
	newEntry := &models.Entry{
		Path:        "/root/new.txt",
		Parent:      stringPtr("/root"),
		Size:        200,
		Kind:        "file",
		Ctime:       newTime,
		Mtime:       newTime,
		LastScanned: newTime,
	}
	db.InsertOrUpdate(newEntry)

	// Delete stale entries (older than newTime)
	err = db.DeleteStale("/root", newTime)
	assert.NoError(t, err)

	// Old entry should be deleted
	oldRetrieved, err := db.Get("/root/old.txt")
	assert.NoError(t, err)
	assert.Nil(t, oldRetrieved)

	// New entry should still exist
	newRetrieved, err := db.Get("/root/new.txt")
	assert.NoError(t, err)
	assert.NotNil(t, newRetrieved)
}

func TestComputeAggregates(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create directory structure
	// /root (should sum to 300)
	//   /root/subdir (should sum to 300)
	//     /root/subdir/file1.txt (100)
	//     /root/subdir/file2.txt (200)

	root := &models.Entry{
		Path:        "/root",
		Size:        0,
		Kind:        "directory",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(root)

	subdir := &models.Entry{
		Path:        "/root/subdir",
		Parent:      stringPtr("/root"),
		Size:        0,
		Kind:        "directory",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(subdir)

	file1 := &models.Entry{
		Path:        "/root/subdir/file1.txt",
		Parent:      stringPtr("/root/subdir"),
		Size:        100,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(file1)

	file2 := &models.Entry{
		Path:        "/root/subdir/file2.txt",
		Parent:      stringPtr("/root/subdir"),
		Size:        200,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(file2)

	// Compute aggregates
	err = db.ComputeAggregates("/root")
	assert.NoError(t, err)

	// Verify subdirectory size
	subdirEntry, err := db.Get("/root/subdir")
	assert.NoError(t, err)
	assert.Equal(t, int64(300), subdirEntry.Size)

	// Verify root directory size
	rootEntry, err := db.Get("/root")
	assert.NoError(t, err)
	assert.Equal(t, int64(300), rootEntry.Size)
}

func TestAll(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Insert some entries
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path:        filepath.Join("/test", string(rune('a'+i))),
			Size:        int64(i * 100),
			Kind:        "file",
			Ctime:       now,
			Mtime:       now,
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	entries, err := db.All()
	assert.NoError(t, err)
	assert.Len(t, entries, 5)
}

func TestGetTree(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create a simple tree structure
	root := &models.Entry{
		Path:        "/tree",
		Size:        0,
		Kind:        "directory",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(root)

	file := &models.Entry{
		Path:        "/tree/file.txt",
		Parent:      stringPtr("/tree"),
		Size:        100,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(file)

	tree, err := db.GetTree("/tree")
	assert.NoError(t, err)
	assert.NotNil(t, tree)
	assert.Equal(t, "/tree", tree.Path)
	assert.Len(t, tree.Children, 1)
	assert.Equal(t, "/tree/file.txt", tree.Children[0].Path)
}

func TestGetDiskUsageSummary(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	oldTime := now - 86400 // 1 day ago

	// Create test structure
	root := &models.Entry{
		Path:        "/usage",
		Size:        0,
		Kind:        "directory",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(root)

	file1 := &models.Entry{
		Path:        "/usage/large.txt",
		Parent:      stringPtr("/usage"),
		Size:        1000,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(file1)

	file2 := &models.Entry{
		Path:        "/usage/old.txt",
		Parent:      stringPtr("/usage"),
		Size:        500,
		Kind:        "file",
		Ctime:       oldTime,
		Mtime:       oldTime,
		LastScanned: now,
	}
	db.InsertOrUpdate(file2)

	summary, err := db.GetDiskUsageSummary("/usage")
	assert.NoError(t, err)
	assert.NotNil(t, summary)
	assert.Equal(t, int64(1500), summary.TotalSize)
	assert.Equal(t, 2, summary.FileCount)
	assert.Equal(t, 1, summary.DirectoryCount)
	assert.Equal(t, "/usage/large.txt", summary.LargestFile)
	assert.Equal(t, int64(1000), summary.LargestFileSize)
	assert.Equal(t, "/usage/old.txt", summary.OldestFile)
}

func TestTransactions(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test begin, commit
	err = db.BeginTransaction()
	assert.NoError(t, err)

	err = db.CommitTransaction()
	assert.NoError(t, err)

	// Test rollback
	err = db.BeginTransaction()
	assert.NoError(t, err)

	err = db.RollbackTransaction()
	assert.NoError(t, err)
}

func TestExecuteFileFilter(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Insert test files
	file1 := &models.Entry{
		Path:        "/test/large.txt",
		Size:        5000,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(file1)

	file2 := &models.Entry{
		Path:        "/test/small.txt",
		Size:        100,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(file2)

	// Test filter by minimum size
	minSize := int64(1000)
	filter := &models.FileFilter{
		MinSize: &minSize,
	}

	entries, err := db.ExecuteFileFilter(filter)
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/test/large.txt", entries[0].Path)
}

func TestGetEntriesByTimeRange(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create entries with different dates
	date1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	date2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).Unix()
	date3 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC).Unix()

	entry1 := &models.Entry{
		Path:        "/test/jan.txt",
		Size:        100,
		Kind:        "file",
		Ctime:       date1,
		Mtime:       date1,
		LastScanned: time.Now().Unix(),
	}
	db.InsertOrUpdate(entry1)

	entry2 := &models.Entry{
		Path:        "/test/jun.txt",
		Size:        200,
		Kind:        "file",
		Ctime:       date2,
		Mtime:       date2,
		LastScanned: time.Now().Unix(),
	}
	db.InsertOrUpdate(entry2)

	entry3 := &models.Entry{
		Path:        "/test/dec.txt",
		Size:        300,
		Kind:        "file",
		Ctime:       date3,
		Mtime:       date3,
		LastScanned: time.Now().Unix(),
	}
	db.InsertOrUpdate(entry3)

	// Query for entries in middle of year
	entries, err := db.GetEntriesByTimeRange("2024-05-01", "2024-07-01", nil)
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/test/jun.txt", entries[0].Path)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
