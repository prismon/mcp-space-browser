package database

import (
	"context"
	"encoding/json"
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

func TestDB(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test that DB() returns the underlying database
	rawDB := db.DB()
	assert.NotNil(t, rawDB)

	// Verify we can use it to query
	var count int
	err = rawDB.QueryRow("SELECT COUNT(*) FROM entries").Scan(&count)
	assert.NoError(t, err)
}

func TestLockUnlockIndexing(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Lock indexing
	err = db.LockIndexing()
	assert.NoError(t, err, "should acquire lock on first attempt")

	// Try to lock again - should fail
	err = db.LockIndexing()
	assert.Error(t, err, "should not acquire lock when already locked")

	// Unlock
	db.UnlockIndexing()

	// Should be able to lock again
	err = db.LockIndexing()
	assert.NoError(t, err, "should acquire lock after unlock")
	db.UnlockIndexing()
}

func TestMetadataOperations(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// First create an entry for the metadata
	entry := &models.Entry{
		Path: "/test/file.txt",
		Size: 100,
		Kind: "file",
	}
	err = db.InsertOrUpdate(entry)
	require.NoError(t, err)

	testHash := "abc123hash"

	t.Run("CreateOrUpdateMetadata", func(t *testing.T) {
		meta := &models.Metadata{
			Hash:         testHash,
			SourcePath:   "/test/file.txt",
			MetadataType: "thumbnail",
			MimeType:     "image/jpeg",
			CachePath:    "/cache/abc123.jpg",
			FileSize:     1024,
			CreatedAt:    time.Now().Unix(),
		}
		err := db.CreateOrUpdateMetadata(meta)
		assert.NoError(t, err)
	})

	t.Run("GetMetadata", func(t *testing.T) {
		retrieved, err := db.GetMetadata(testHash)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "thumbnail", retrieved.MetadataType)
	})

	t.Run("GetMetadataByPath", func(t *testing.T) {
		metas, err := db.GetMetadataByPath("/test/file.txt")
		assert.NoError(t, err)
		assert.NotEmpty(t, metas)
		assert.Equal(t, "/test/file.txt", metas[0].SourcePath)
	})

	t.Run("ListMetadata", func(t *testing.T) {
		// Add another entry and metadata
		entry2 := &models.Entry{
			Path: "/test/file2.txt",
			Size: 200,
			Kind: "file",
		}
		db.InsertOrUpdate(entry2)

		meta2 := &models.Metadata{
			Hash:         "xyz789hash",
			SourcePath:   "/test/file2.txt",
			MetadataType: "thumbnail",
			MimeType:     "image/jpeg",
			CachePath:    "/cache/xyz789.jpg",
			FileSize:     2048,
			CreatedAt:    time.Now().Unix(),
		}
		db.CreateOrUpdateMetadata(meta2)

		metaType := "thumbnail"
		list, err := db.ListMetadata(&metaType)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(list), 2)
	})

	t.Run("DeleteMetadata", func(t *testing.T) {
		err := db.DeleteMetadata(testHash)
		assert.NoError(t, err)

		// Verify deleted
		retrieved, err := db.GetMetadata(testHash)
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestDeleteEntry(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create an entry
	entry := &models.Entry{
		Path: "/test/delete.txt",
		Size: 100,
		Kind: "file",
	}
	err = db.InsertOrUpdate(entry)
	require.NoError(t, err)

	// Delete it
	err = db.DeleteEntry("/test/delete.txt")
	assert.NoError(t, err)

	// Verify deleted
	retrieved, err := db.Get("/test/delete.txt")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestDeleteEntryRecursive(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a directory structure
	parent := &models.Entry{
		Path: "/parent",
		Size: 0,
		Kind: "directory",
	}
	db.InsertOrUpdate(parent)

	child := &models.Entry{
		Path:   "/parent/child.txt",
		Parent: stringPtr("/parent"),
		Size:   100,
		Kind:   "file",
	}
	db.InsertOrUpdate(child)

	// Delete recursively
	err = db.DeleteEntryRecursive("/parent")
	assert.NoError(t, err)

	// Verify both deleted
	parentEntry, _ := db.Get("/parent")
	assert.Nil(t, parentEntry)

	childEntry, _ := db.Get("/parent/child.txt")
	assert.Nil(t, childEntry)
}

func TestUpdateEntryPath(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create an entry
	entry := &models.Entry{
		Path: "/old/path.txt",
		Size: 100,
		Kind: "file",
	}
	err = db.InsertOrUpdate(entry)
	require.NoError(t, err)

	// Update path
	err = db.UpdateEntryPath("/old/path.txt", "/new/path.txt")
	assert.NoError(t, err)

	// Verify old path doesn't exist
	old, _ := db.Get("/old/path.txt")
	assert.Nil(t, old)

	// Verify new path exists
	new, err := db.Get("/new/path.txt")
	assert.NoError(t, err)
	assert.NotNil(t, new)
	assert.Equal(t, "/new/path.txt", new.Path)
}

func TestUpdatePathsRecursive(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a directory structure
	parent := &models.Entry{
		Path: "/old",
		Size: 0,
		Kind: "directory",
	}
	db.InsertOrUpdate(parent)

	child := &models.Entry{
		Path:   "/old/file.txt",
		Parent: stringPtr("/old"),
		Size:   100,
		Kind:   "file",
	}
	db.InsertOrUpdate(child)

	// Update paths recursively
	err = db.UpdatePathsRecursive("/old", "/new")
	assert.NoError(t, err)

	// Verify old paths don't exist
	oldParent, _ := db.Get("/old")
	assert.Nil(t, oldParent)

	// Verify new paths exist
	newParent, _ := db.Get("/new")
	assert.NotNil(t, newParent)

	newChild, _ := db.Get("/new/file.txt")
	assert.NotNil(t, newChild)
}

func TestGetPathLastScanned(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	scanTime := time.Now().Unix()

	// Create an entry with scan time
	entry := &models.Entry{
		Path:        "/test/scanned.txt",
		Size:        100,
		Kind:        "file",
		LastScanned: scanTime,
	}
	err = db.InsertOrUpdate(entry)
	require.NoError(t, err)

	// Get last scanned time
	lastScanned, err := db.GetPathLastScanned("/test/scanned.txt")
	assert.NoError(t, err)
	assert.Equal(t, scanTime, lastScanned)

	// Test non-existent path
	lastScanned2, err := db.GetPathLastScanned("/nonexistent")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), lastScanned2)
}

func TestGetPathScanInfo(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	scanTime := time.Now().Unix()

	// Create a directory with entries
	parent := &models.Entry{
		Path:        "/scantest",
		Size:        0,
		Kind:        "directory",
		LastScanned: scanTime,
	}
	db.InsertOrUpdate(parent)

	child := &models.Entry{
		Path:        "/scantest/file.txt",
		Parent:      stringPtr("/scantest"),
		Size:        100,
		Kind:        "file",
		LastScanned: scanTime,
	}
	db.InsertOrUpdate(child)

	// Get scan info
	scanInfo, err := db.GetPathScanInfo("/scantest")
	assert.NoError(t, err)
	assert.NotNil(t, scanInfo)
	assert.Equal(t, 2, scanInfo.EntryCount) // parent + child

	// Test non-existent path - returns struct with Exists=false
	scanInfo2, err := db.GetPathScanInfo("/nonexistent")
	assert.NoError(t, err)
	assert.NotNil(t, scanInfo2)
	assert.False(t, scanInfo2.Exists)
	assert.Equal(t, 0, scanInfo2.EntryCount)
}

func TestGetTreeWithOptions(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	ctx := context.Background()

	// Create a directory structure
	root := &models.Entry{
		Path: "/root", Size: 0, Kind: "directory",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(root)

	subdir := &models.Entry{
		Path: "/root/subdir", Parent: stringPtr("/root"),
		Size: 0, Kind: "directory",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(subdir)

	file1 := &models.Entry{
		Path: "/root/file1.txt", Parent: stringPtr("/root"),
		Size: 1000, Kind: "file",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(file1)

	file2 := &models.Entry{
		Path: "/root/file2.txt", Parent: stringPtr("/root"),
		Size: 500, Kind: "file",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(file2)

	subfile := &models.Entry{
		Path: "/root/subdir/subfile.txt", Parent: stringPtr("/root/subdir"),
		Size: 200, Kind: "file",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(subfile)

	db.ComputeAggregates("/root")

	// Test with default options
	nodesReturned := 0
	opts := TreeOptions{
		MaxDepth:       10,
		Limit:          nil,
		MinSize:        0,
		SortBy:         "size",
		DescendingSort: true,
		ChildThreshold: 1000,
		NodesReturned:  &nodesReturned,
	}
	tree, err := db.GetTreeWithOptions(ctx, "/root", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree)
	assert.Equal(t, "root", tree.Name)
	assert.Equal(t, "directory", tree.Kind)
	assert.GreaterOrEqual(t, len(tree.Children), 2)

	// Test with max depth = 1
	nodesReturned = 0
	opts.MaxDepth = 1
	opts.CurrentDepth = 0
	tree2, err := db.GetTreeWithOptions(ctx, "/root", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree2)

	// Test with min size filter
	nodesReturned = 0
	opts.MaxDepth = 10
	opts.MinSize = 800
	tree3, err := db.GetTreeWithOptions(ctx, "/root", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree3)

	// Test non-existent path
	_, err = db.GetTreeWithOptions(ctx, "/nonexistent", opts)
	assert.Error(t, err)
}

func TestGetTreeWithOptionsManyChildren(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	ctx := context.Background()

	// Create root directory
	root := &models.Entry{
		Path: "/manychildren", Size: 0, Kind: "directory",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(root)

	// Create many children to trigger threshold
	for i := 0; i < 20; i++ {
		file := &models.Entry{
			Path:        filepath.Join("/manychildren", "file"+string(rune('0'+i%10))+string(rune('0'+i/10))+".txt"),
			Parent:      stringPtr("/manychildren"),
			Size:        int64(i * 100),
			Kind:        "file",
			LastScanned: now,
			Mtime:       now,
		}
		db.InsertOrUpdate(file)
	}

	// Test with low threshold to trigger summarization
	nodesReturned := 0
	opts := TreeOptions{
		MaxDepth:       10,
		MinSize:        0,
		SortBy:         "size",
		DescendingSort: true,
		ChildThreshold: 5, // Low threshold to trigger summary
		NodesReturned:  &nodesReturned,
	}

	tree, err := db.GetTreeWithOptions(ctx, "/manychildren", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree)
	assert.True(t, tree.Truncated)
	assert.NotNil(t, tree.Summary)
}

func TestGetTreeSorting(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	ctx := context.Background()

	root := &models.Entry{
		Path: "/sortroot", Size: 0, Kind: "directory",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(root)

	// Create files with different sizes and mtimes
	file1 := &models.Entry{
		Path: "/sortroot/afile.txt", Parent: stringPtr("/sortroot"),
		Size: 100, Kind: "file",
		LastScanned: now, Mtime: now - 100,
	}
	db.InsertOrUpdate(file1)

	file2 := &models.Entry{
		Path: "/sortroot/bfile.txt", Parent: stringPtr("/sortroot"),
		Size: 200, Kind: "file",
		LastScanned: now, Mtime: now - 50,
	}
	db.InsertOrUpdate(file2)

	// Test sort by size descending
	nodesReturned := 0
	opts := TreeOptions{
		MaxDepth:       10,
		SortBy:         "size",
		DescendingSort: true,
		ChildThreshold: 1000,
		NodesReturned:  &nodesReturned,
	}
	tree, err := db.GetTreeWithOptions(ctx, "/sortroot", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree)
	if len(tree.Children) >= 2 {
		assert.GreaterOrEqual(t, tree.Children[0].Size, tree.Children[1].Size)
	}

	// Test sort by name
	nodesReturned = 0
	opts.SortBy = "name"
	opts.DescendingSort = false
	tree2, err := db.GetTreeWithOptions(ctx, "/sortroot", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree2)

	// Test sort by mtime
	nodesReturned = 0
	opts.SortBy = "mtime"
	opts.DescendingSort = true
	tree3, err := db.GetTreeWithOptions(ctx, "/sortroot", opts)
	assert.NoError(t, err)
	assert.NotNil(t, tree3)
}

func TestAddRemoveResourceSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a selection set
	desc := "Test selection"
	set := &models.ResourceSet{
		Name:        "test-set",
		Description: &desc,
	}
	_, err = db.CreateResourceSet(set)
	assert.NoError(t, err)

	// Create some entries
	now := time.Now().Unix()
	entry1 := &models.Entry{
		Path: "/test1.txt", Size: 100, Kind: "file",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(entry1)

	entry2 := &models.Entry{
		Path: "/test2.txt", Size: 200, Kind: "file",
		LastScanned: now, Mtime: now,
	}
	db.InsertOrUpdate(entry2)

	// Add entries to selection set
	err = db.AddToResourceSet("test-set", []string{"/test1.txt", "/test2.txt"})
	assert.NoError(t, err)

	// Get selection set entries
	entries, err := db.GetResourceSetEntries("test-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 2)

	// Remove one entry
	err = db.RemoveFromResourceSet("test-set", []string{"/test1.txt"})
	assert.NoError(t, err)

	// Verify removal
	entries, err = db.GetResourceSetEntries("test-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 1)

	// Test adding to non-existent set
	err = db.AddToResourceSet("nonexistent", []string{"/test1.txt"})
	assert.Error(t, err)

	// Test removing from non-existent set
	err = db.RemoveFromResourceSet("nonexistent", []string{"/test1.txt"})
	assert.Error(t, err)
}

func TestTransactionDoubleBegin(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Begin a transaction
	err = db.BeginTransaction()
	assert.NoError(t, err)

	// Begin again should error (already in transaction)
	err = db.BeginTransaction()
	assert.Error(t, err)

	// Rollback the first transaction
	err = db.RollbackTransaction()
	assert.NoError(t, err)

	// Now should be able to begin again
	err = db.BeginTransaction()
	assert.NoError(t, err)

	// Commit the transaction
	err = db.CommitTransaction()
	assert.NoError(t, err)
}

func TestExecuteFileFilterExtended(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	oldTime := now - 86400*30 // 30 days ago

	// Insert test files
	files := []struct {
		path  string
		size  int64
		mtime int64
	}{
		{"/root/doc.txt", 1000, now},
		{"/root/image.jpg", 5000, oldTime},
		{"/root/large.mp4", 100000, now},
		{"/root/subdir/small.txt", 100, now},
		{"/root/test_file.log", 500, oldTime},
	}
	for _, f := range files {
		entry := &models.Entry{
			Path:        f.path,
			Parent:      stringPtr(filepath.Dir(f.path)),
			Size:        f.size,
			Kind:        "file",
			Mtime:       f.mtime,
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	t.Run("FilterByPath", func(t *testing.T) {
		path := "/root"
		filter := &models.FileFilter{Path: &path}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.Len(t, entries, 5)
	})

	t.Run("FilterByExtensions", func(t *testing.T) {
		filter := &models.FileFilter{
			Extensions: []string{"txt", "log"},
		}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.Len(t, entries, 3)
	})

	t.Run("FilterByMaxSize", func(t *testing.T) {
		maxSize := int64(1000)
		filter := &models.FileFilter{MaxSize: &maxSize}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.Len(t, entries, 3) // doc.txt (1000), small.txt (100), test_file.log (500)
	})

	t.Run("FilterByDateRange", func(t *testing.T) {
		// Only recent files
		minDate := time.Unix(now-86400, 0).Format("2006-01-02")
		filter := &models.FileFilter{MinDate: &minDate}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 2)
	})

	t.Run("FilterByNameContains", func(t *testing.T) {
		name := "test"
		filter := &models.FileFilter{NameContains: &name}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Contains(t, entries[0].Path, "test")
	})

	t.Run("FilterByPathContains", func(t *testing.T) {
		path := "subdir"
		filter := &models.FileFilter{PathContains: &path}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.Len(t, entries, 1)
	})

	t.Run("FilterWithSortBy", func(t *testing.T) {
		sortBy := "size"
		descending := true
		filter := &models.FileFilter{SortBy: &sortBy, DescendingSort: &descending}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 2)
		if len(entries) >= 2 {
			assert.GreaterOrEqual(t, entries[0].Size, entries[1].Size)
		}
	})

	t.Run("FilterWithLimit", func(t *testing.T) {
		limit := 2
		filter := &models.FileFilter{Limit: &limit}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		assert.Len(t, entries, 2)
	})

	t.Run("FilterByPattern", func(t *testing.T) {
		pattern := ".*\\.txt$"
		filter := &models.FileFilter{Pattern: &pattern}
		entries, err := db.ExecuteFileFilter(filter)
		assert.NoError(t, err)
		// All should be .txt files
		for _, e := range entries {
			assert.True(t, filepath.Ext(e.Path) == ".txt")
		}
	})

	t.Run("InvalidPattern", func(t *testing.T) {
		pattern := "[invalid"
		filter := &models.FileFilter{Pattern: &pattern}
		_, err := db.ExecuteFileFilter(filter)
		assert.Error(t, err)
	})

	t.Run("InvalidSortField", func(t *testing.T) {
		sortBy := "invalid_field"
		filter := &models.FileFilter{SortBy: &sortBy}
		_, err := db.ExecuteFileFilter(filter)
		assert.Error(t, err)
	})
}

func TestQueryOperations(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Insert test files
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path:        filepath.Join("/test", "file"+string(rune('a'+i))+".txt"),
			Size:        int64(i * 100),
			Kind:        "file",
			Mtime:       now,
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	var queryID int64

	t.Run("CreateQuery", func(t *testing.T) {
		filter := models.FileFilter{
			Extensions: []string{"txt"},
		}
		filterJSON, _ := json.Marshal(filter)
		query := &models.Query{
			Name:      "test-query",
			QueryType: "file_filter",
			QueryJSON: string(filterJSON),
			CreatedAt: now,
			UpdatedAt: now,
		}
		id, err := db.CreateQuery(query)
		assert.NoError(t, err)
		assert.Greater(t, id, int64(0))
		queryID = id
	})

	t.Run("GetQuery", func(t *testing.T) {
		query, err := db.GetQuery("test-query")
		assert.NoError(t, err)
		assert.NotNil(t, query)
		assert.Equal(t, "test-query", query.Name)
	})

	t.Run("ListQueries", func(t *testing.T) {
		queries, err := db.ListQueries()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(queries), 1)
	})

	t.Run("ExecuteQuery", func(t *testing.T) {
		entries, err := db.ExecuteQuery("test-query")
		assert.NoError(t, err)
		assert.NotEmpty(t, entries)
	})

	t.Run("RecordQueryExecution", func(t *testing.T) {
		filesMatched := 5
		durationMs := 100
		exec := &models.QueryExecution{
			QueryID:      queryID,
			ExecutedAt:   now,
			FilesMatched: &filesMatched,
			DurationMs:   &durationMs,
			Status:       "success",
		}
		err := db.RecordQueryExecution(exec)
		assert.NoError(t, err)
	})

	t.Run("DeleteQuery", func(t *testing.T) {
		err := db.DeleteQuery("test-query")
		assert.NoError(t, err)

		query, err := db.GetQuery("test-query")
		assert.NoError(t, err)
		assert.Nil(t, query)
	})

	t.Run("ExecuteNonexistentQuery", func(t *testing.T) {
		_, err := db.ExecuteQuery("nonexistent")
		assert.Error(t, err)
	})
}

func TestCommitTransactionNoTx(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Commit without begin should error
	err = db.CommitTransaction()
	assert.Error(t, err)
}

func TestInsertOrUpdateWithTransaction(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Begin transaction
	err = db.BeginTransaction()
	require.NoError(t, err)

	// Insert entries within transaction
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path:        filepath.Join("/txtest", "file"+string(rune('a'+i))+".txt"),
			Size:        int64(i * 100),
			Kind:        "file",
			Mtime:       now,
			LastScanned: now,
		}
		err := db.InsertOrUpdate(entry)
		assert.NoError(t, err)
	}

	// Commit
	err = db.CommitTransaction()
	assert.NoError(t, err)

	// Verify entries exist
	entries, err := db.All()
	assert.NoError(t, err)
	assert.Len(t, entries, 5)
}

func TestTransactionRollback(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// First insert an entry outside transaction
	initialEntry := &models.Entry{
		Path:        "/initial.txt",
		Size:        100,
		Kind:        "file",
		Mtime:       now,
		LastScanned: now,
	}
	err = db.InsertOrUpdate(initialEntry)
	require.NoError(t, err)

	// Begin transaction
	err = db.BeginTransaction()
	require.NoError(t, err)

	// Insert entries within transaction
	for i := 0; i < 3; i++ {
		entry := &models.Entry{
			Path:        filepath.Join("/rollback", "file"+string(rune('a'+i))+".txt"),
			Size:        int64(i * 100),
			Kind:        "file",
			Mtime:       now,
			LastScanned: now,
		}
		err := db.InsertOrUpdate(entry)
		assert.NoError(t, err)
	}

	// Rollback
	err = db.RollbackTransaction()
	assert.NoError(t, err)

	// Verify only initial entry exists
	entries, err := db.All()
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/initial.txt", entries[0].Path)
}

func TestResourceSetOperations(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	t.Run("CreateResourceSet", func(t *testing.T) {
		desc := "Test description"
		set := &models.ResourceSet{
			Name:        "my-set",
			Description: &desc,
		}
		id, err := db.CreateResourceSet(set)
		assert.NoError(t, err)
		assert.Greater(t, id, int64(0))
	})

	t.Run("GetResourceSet", func(t *testing.T) {
		set, err := db.GetResourceSet("my-set")
		assert.NoError(t, err)
		assert.NotNil(t, set)
		assert.Equal(t, "my-set", set.Name)
	})

	t.Run("GetNonexistentResourceSet", func(t *testing.T) {
		set, err := db.GetResourceSet("nonexistent")
		assert.NoError(t, err)
		assert.Nil(t, set)
	})

	t.Run("ListResourceSets", func(t *testing.T) {
		sets, err := db.ListResourceSets()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(sets), 1)
	})

	t.Run("DeleteResourceSet", func(t *testing.T) {
		err := db.DeleteResourceSet("my-set")
		assert.NoError(t, err)

		set, err := db.GetResourceSet("my-set")
		assert.NoError(t, err)
		assert.Nil(t, set)
	})
}

func TestComputeAggregatesEmptyPath(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Compute aggregates on non-existent path should not error
	err = db.ComputeAggregates("/nonexistent")
	assert.NoError(t, err)
}

func TestGetDiskUsageSummaryEmpty(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get summary for empty path
	summary, err := db.GetDiskUsageSummary("/nonexistent")
	assert.NoError(t, err)
	assert.NotNil(t, summary)
	assert.Equal(t, int64(0), summary.TotalSize)
}

func TestGetEntriesByTimeRangeFullRange(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create entries with different times
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path:        filepath.Join("/test", "file"+string(rune('a'+i))+".txt"),
			Size:        int64(i * 100),
			Kind:        "file",
			Mtime:       now - int64(i*86400), // Each file 1 day older
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Query with full time range
	startDate := time.Unix(now-86400*10, 0).Format("2006-01-02")
	endDate := time.Unix(now+86400, 0).Format("2006-01-02")
	entries, err := db.GetEntriesByTimeRange(startDate, endDate, nil)
	assert.NoError(t, err)
	assert.Len(t, entries, 5)

	// Query with narrower range
	maxDate := time.Unix(now-86400*2, 0).Format("2006-01-02")
	entries2, err := db.GetEntriesByTimeRange(startDate, maxDate, nil)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries2), 1)
}

func TestChildrenWithSort(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create parent directory
	parent := &models.Entry{
		Path:        "/parent",
		Size:        0,
		Kind:        "directory",
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(parent)

	// Create children with different sizes
	for i := 0; i < 5; i++ {
		child := &models.Entry{
			Path:        filepath.Join("/parent", "child"+string(rune('a'+i))+".txt"),
			Parent:      stringPtr("/parent"),
			Size:        int64((5 - i) * 100),
			Kind:        "file",
			Mtime:       now,
			LastScanned: now,
		}
		db.InsertOrUpdate(child)
	}

	// Get children
	children, err := db.Children("/parent")
	assert.NoError(t, err)
	assert.Len(t, children, 5)
}

func TestComputeAggregatesDeep(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create deep directory structure
	// /root
	//   /root/a
	//     /root/a/b
	//       /root/a/b/file.txt (100)
	root := &models.Entry{
		Path: "/root", Size: 0, Kind: "directory",
		Mtime: now, LastScanned: now,
	}
	db.InsertOrUpdate(root)

	dirA := &models.Entry{
		Path: "/root/a", Parent: stringPtr("/root"),
		Size: 0, Kind: "directory",
		Mtime: now, LastScanned: now,
	}
	db.InsertOrUpdate(dirA)

	dirB := &models.Entry{
		Path: "/root/a/b", Parent: stringPtr("/root/a"),
		Size: 0, Kind: "directory",
		Mtime: now, LastScanned: now,
	}
	db.InsertOrUpdate(dirB)

	file := &models.Entry{
		Path: "/root/a/b/file.txt", Parent: stringPtr("/root/a/b"),
		Size: 100, Kind: "file",
		Mtime: now, LastScanned: now,
	}
	db.InsertOrUpdate(file)

	// Compute aggregates
	err = db.ComputeAggregates("/root")
	assert.NoError(t, err)

	// Verify sizes propagated up
	rootEntry, _ := db.Get("/root")
	assert.Equal(t, int64(100), rootEntry.Size)

	aEntry, _ := db.Get("/root/a")
	assert.Equal(t, int64(100), aEntry.Size)

	bEntry, _ := db.Get("/root/a/b")
	assert.Equal(t, int64(100), bEntry.Size)
}

func TestUpdateEntryPathNonexistent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Try to update nonexistent path - should error
	err = db.UpdateEntryPath("/nonexistent/old.txt", "/nonexistent/new.txt")
	assert.Error(t, err) // Should error when path doesn't exist
	assert.Contains(t, err.Error(), "not found")
}

func TestGetMetadataNonexistent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get nonexistent metadata
	meta, err := db.GetMetadata("nonexistent-hash")
	assert.NoError(t, err)
	assert.Nil(t, meta)
}

func TestListMetadataNilFilter(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create entry and metadata
	entry := &models.Entry{
		Path: "/test.txt", Size: 100, Kind: "file",
	}
	db.InsertOrUpdate(entry)

	meta := &models.Metadata{
		Hash:         "test-hash",
		SourcePath:   "/test.txt",
		MetadataType: "thumbnail",
		CreatedAt:    time.Now().Unix(),
	}
	db.CreateOrUpdateMetadata(meta)

	// List with nil filter
	metas, err := db.ListMetadata(nil)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(metas), 1)
}

func TestResourceSetWithMultipleEntries(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create selection set
	desc := "Test"
	set := &models.ResourceSet{
		Name:        "multi-set",
		Description: &desc,
	}
	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	// Create many entries
	paths := []string{}
	for i := 0; i < 10; i++ {
		entry := &models.Entry{
			Path: filepath.Join("/test", "file"+string(rune('a'+i))+".txt"),
			Size: int64(i * 100), Kind: "file",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
		paths = append(paths, entry.Path)
	}

	// Add all to selection set
	err = db.AddToResourceSet("multi-set", paths)
	assert.NoError(t, err)

	// Verify count
	entries, err := db.GetResourceSetEntries("multi-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 10)

	// Remove half
	err = db.RemoveFromResourceSet("multi-set", paths[:5])
	assert.NoError(t, err)

	// Verify count
	entries, err = db.GetResourceSetEntries("multi-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 5)
}

func TestDeleteStaleRecursive(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	oldTime := time.Now().Unix() - 1000
	newTime := time.Now().Unix()

	// Create directory structure with old and new entries
	parent := &models.Entry{
		Path: "/stale", Size: 0, Kind: "directory",
		Mtime: newTime, LastScanned: newTime,
	}
	db.InsertOrUpdate(parent)

	oldChild := &models.Entry{
		Path:   "/stale/old.txt",
		Parent: stringPtr("/stale"),
		Size:   100, Kind: "file",
		Mtime: oldTime, LastScanned: oldTime,
	}
	db.InsertOrUpdate(oldChild)

	newChild := &models.Entry{
		Path:   "/stale/new.txt",
		Parent: stringPtr("/stale"),
		Size:   200, Kind: "file",
		Mtime: newTime, LastScanned: newTime,
	}
	db.InsertOrUpdate(newChild)

	// Delete stale entries
	err = db.DeleteStale("/stale", newTime)
	assert.NoError(t, err)

	// Old entry should be deleted
	oldEntry, _ := db.Get("/stale/old.txt")
	assert.Nil(t, oldEntry)

	// New entry should still exist
	newEntry, _ := db.Get("/stale/new.txt")
	assert.NotNil(t, newEntry)
}

func TestAddToResourceSetWithMultiplePaths(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create some entries
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path: filepath.Join("/test", "file"+string(rune('a'+i))+".txt"),
			Size: 100, Kind: "file",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Create a selection set
	desc := "Test batch adding"
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "batch-test", Description: &desc})
	require.NoError(t, err)

	// Add multiple paths at once
	paths := []string{"/test/filea.txt", "/test/fileb.txt", "/test/filec.txt"}
	err = db.AddToResourceSet("batch-test", paths)
	assert.NoError(t, err)

	// Verify entries were added
	entries, err := db.GetResourceSetEntries("batch-test")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(entries))
}

func TestRemoveFromResourceSetWithMultiplePaths(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create entries
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path: filepath.Join("/test", "file"+string(rune('a'+i))+".txt"),
			Size: 100, Kind: "file",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Create a selection set and add entries
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "remove-test"})
	require.NoError(t, err)

	allPaths := []string{"/test/filea.txt", "/test/fileb.txt", "/test/filec.txt", "/test/filed.txt"}
	err = db.AddToResourceSet("remove-test", allPaths)
	require.NoError(t, err)

	// Remove some entries
	removePaths := []string{"/test/filea.txt", "/test/filec.txt"}
	err = db.RemoveFromResourceSet("remove-test", removePaths)
	assert.NoError(t, err)

	// Verify only two remain
	entries, err := db.GetResourceSetEntries("remove-test")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(entries))
}

func TestAddToResourceSetNonexistentSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.AddToResourceSet("nonexistent", []string{"/test/file.txt"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRemoveFromResourceSetNonexistentSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.RemoveFromResourceSet("nonexistent", []string{"/test/file.txt"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListQueriesMultiple(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple queries
	filter1 := models.FileFilter{Extensions: []string{"txt"}}
	filter1JSON, _ := json.Marshal(filter1)

	minSize := int64(1000)
	filter2 := models.FileFilter{MinSize: &minSize}
	filter2JSON, _ := json.Marshal(filter2)

	query1 := &models.Query{Name: "query1", QueryType: "file_filter", QueryJSON: string(filter1JSON)}
	query2 := &models.Query{Name: "query2", QueryType: "file_filter", QueryJSON: string(filter2JSON)}
	query3 := &models.Query{Name: "query3", QueryType: "custom_script", QueryJSON: "{}"}

	_, err = db.CreateQuery(query1)
	require.NoError(t, err)
	_, err = db.CreateQuery(query2)
	require.NoError(t, err)
	_, err = db.CreateQuery(query3)
	require.NoError(t, err)

	// List all queries
	queries, err := db.ListQueries()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(queries))

	// Verify query names
	names := make(map[string]bool)
	for _, q := range queries {
		names[q.Name] = true
	}
	assert.True(t, names["query1"])
	assert.True(t, names["query2"])
	assert.True(t, names["query3"])
}

func TestGetEntriesByTimeRangeWithDates(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create entries with different dates
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC).Unix()
	entries := []struct {
		path string
		days int
	}{
		{"/test/old.txt", -30},
		{"/test/recent.txt", -5},
		{"/test/today.txt", 0},
	}

	for _, e := range entries {
		mtime := baseTime + int64(e.days*86400)
		entry := &models.Entry{
			Path: e.path, Size: 100, Kind: "file",
			Mtime: mtime, LastScanned: baseTime,
		}
		db.InsertOrUpdate(entry)
	}

	// Get entries from last 10 days
	minDate := time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	maxDate := time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	results, err := db.GetEntriesByTimeRange(minDate, maxDate, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results)) // recent and today
}

func TestComputeAggregatesWithNestedDirs(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create nested directory structure
	dirs := []string{"/root", "/root/a", "/root/a/b"}
	for _, dir := range dirs {
		var parent *string
		if dir != "/root" {
			p := filepath.Dir(dir)
			parent = &p
		}
		entry := &models.Entry{
			Path: dir, Parent: parent, Size: 0, Kind: "directory",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Add files at different levels
	files := []struct {
		path   string
		parent string
		size   int64
	}{
		{"/root/file1.txt", "/root", 100},
		{"/root/a/file2.txt", "/root/a", 200},
		{"/root/a/b/file3.txt", "/root/a/b", 300},
	}

	for _, f := range files {
		entry := &models.Entry{
			Path: f.path, Parent: &f.parent, Size: f.size, Kind: "file",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Compute aggregates
	err = db.ComputeAggregates("/root")
	assert.NoError(t, err)

	// Verify root has aggregate of all files
	root, _ := db.Get("/root")
	assert.Equal(t, int64(600), root.Size) // 100 + 200 + 300

	// Verify intermediate directory
	dirA, _ := db.Get("/root/a")
	assert.Equal(t, int64(500), dirA.Size) // 200 + 300

	// Verify deepest directory
	dirB, _ := db.Get("/root/a/b")
	assert.Equal(t, int64(300), dirB.Size) // just file3
}

func TestBeginTransactionAndCommit(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Begin transaction
	err = db.BeginTransaction()
	require.NoError(t, err)

	// Insert an entry
	now := time.Now().Unix()
	entry := &models.Entry{
		Path: "/test/tx.txt", Size: 100, Kind: "file",
		Mtime: now, LastScanned: now,
	}
	err = db.InsertOrUpdate(entry)
	require.NoError(t, err)

	// Commit
	err = db.CommitTransaction()
	assert.NoError(t, err)

	// Verify entry exists
	retrieved, _ := db.Get("/test/tx.txt")
	assert.NotNil(t, retrieved)
}

func TestBeginTransactionAndRollback(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Insert initial entry outside transaction
	now := time.Now().Unix()
	initialEntry := &models.Entry{
		Path: "/test/initial.txt", Size: 50, Kind: "file",
		Mtime: now, LastScanned: now,
	}
	err = db.InsertOrUpdate(initialEntry)
	require.NoError(t, err)

	// Begin transaction
	err = db.BeginTransaction()
	require.NoError(t, err)

	// Insert entry within transaction
	txEntry := &models.Entry{
		Path: "/test/txentry.txt", Size: 100, Kind: "file",
		Mtime: now, LastScanned: now,
	}
	err = db.InsertOrUpdate(txEntry)
	require.NoError(t, err)

	// Rollback
	err = db.RollbackTransaction()
	assert.NoError(t, err)

	// Entry from transaction should not exist
	retrieved, _ := db.Get("/test/txentry.txt")
	assert.Nil(t, retrieved)

	// Initial entry should still exist
	initial, _ := db.Get("/test/initial.txt")
	assert.NotNil(t, initial)
}

func TestExecuteQueryWithSizeFilter(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create entries with various sizes
	entries := []struct {
		path string
		size int64
		kind string
	}{
		{"/test/small.txt", 100, "file"},
		{"/test/medium.txt", 5000, "file"},
		{"/test/large.txt", 10000, "file"},
		{"/test/subdir", 0, "directory"},
	}

	for _, e := range entries {
		entry := &models.Entry{
			Path: e.path, Size: e.size, Kind: e.kind,
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Create query for files larger than 1000 bytes
	minSize := int64(1000)
	filter := models.FileFilter{MinSize: &minSize}
	filterJSON, _ := json.Marshal(filter)
	query := &models.Query{Name: "large-files", QueryType: "file_filter", QueryJSON: string(filterJSON)}
	_, err = db.CreateQuery(query)
	require.NoError(t, err)

	// Execute query
	results, err := db.ExecuteQuery("large-files")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results)) // medium and large

	// All should be larger than 1000
	for _, r := range results {
		assert.GreaterOrEqual(t, r.Size, int64(1000))
	}
}

func TestGetChildrenMultiple(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create parent
	parent := &models.Entry{
		Path: "/parent", Size: 0, Kind: "directory",
		Mtime: now, LastScanned: now,
	}
	db.InsertOrUpdate(parent)

	// Create many children
	for i := 0; i < 20; i++ {
		entry := &models.Entry{
			Path:   filepath.Join("/parent", "file"+string(rune('a'+i%26))+string(rune('0'+i/10))+".txt"),
			Parent: stringPtr("/parent"),
			Size:   int64(i * 100),
			Kind:   "file",
			Mtime:  now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Get all children
	children, err := db.Children("/parent")
	assert.NoError(t, err)
	assert.Equal(t, 20, len(children))

	// Verify all have correct parent
	for _, c := range children {
		assert.NotNil(t, c.Parent)
		assert.Equal(t, "/parent", *c.Parent)
	}
}

func TestExecuteFileFilterWithAllOptions(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create diverse entries
	entries := []struct {
		path string
		size int64
		kind string
	}{
		{"/docs/readme.md", 500, "file"},
		{"/docs/guide.txt", 1500, "file"},
		{"/docs/manual.pdf", 5000, "file"},
		{"/docs/large.bin", 50000, "file"},
		{"/docs/subdir", 0, "directory"},
	}

	for _, e := range entries {
		entry := &models.Entry{
			Path: e.path, Size: e.size, Kind: e.kind,
			Parent: stringPtr("/docs"),
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Filter by extension
	filter1 := &models.FileFilter{Extensions: []string{"txt", "md"}}
	results1, err := db.ExecuteFileFilter(filter1)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results1))

	// Filter by min size
	minSize := int64(1000)
	filter2 := &models.FileFilter{MinSize: &minSize}
	results2, err := db.ExecuteFileFilter(filter2)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(results2)) // guide.txt, manual.pdf, large.bin

	// Filter by max size
	maxSize := int64(2000)
	filter3 := &models.FileFilter{MaxSize: &maxSize}
	results3, err := db.ExecuteFileFilter(filter3)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(results3), 2) // At least readme.md and guide.txt
}

func TestGetWithInvalidPath(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get non-existent entry
	entry, err := db.Get("/nonexistent/path")
	assert.NoError(t, err)
	assert.Nil(t, entry)
}

func TestChildrenEmptyParent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get children of non-existent parent
	children, err := db.Children("/nonexistent")
	assert.NoError(t, err)
	assert.Empty(t, children)
}

func TestAllWithManyEntries(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create many entries
	for i := 0; i < 30; i++ {
		entry := &models.Entry{
			Path: filepath.Join("/all", "file"+string(rune('a'+i%26))+string(rune('0'+i/10))+".txt"),
			Size: int64(i * 100), Kind: "file",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Get all entries
	entries, err := db.All()
	assert.NoError(t, err)
	assert.Equal(t, 30, len(entries))
}

func TestGetResourceSetEntriesEmpty(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create empty selection set
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "empty-set"})
	require.NoError(t, err)

	// Get entries from empty set
	entries, err := db.GetResourceSetEntries("empty-set")
	assert.NoError(t, err)
	assert.Empty(t, entries)
}

func TestGetResourceSetEntriesNonexistent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get entries from non-existent set
	entries, err := db.GetResourceSetEntries("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, entries)
}

func TestComputeAggregatesNonexistent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Compute aggregates for non-existent path
	err = db.ComputeAggregates("/nonexistent")
	assert.NoError(t, err) // Should not error, just do nothing
}

func TestExecuteQueryNonexistent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Execute non-existent query
	results, err := db.ExecuteQuery("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, results)
}

func TestGetPathLastScannedNonexistent(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Get last scanned for non-existent path
	lastScanned, err := db.GetPathLastScanned("/nonexistent")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), lastScanned)
}

func TestUpdateEntryPathSuccess(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()
	entry := &models.Entry{
		Path: "/old/path.txt", Size: 100, Kind: "file",
		Mtime: now, LastScanned: now,
	}
	db.InsertOrUpdate(entry)

	// Update path
	err = db.UpdateEntryPath("/old/path.txt", "/new/path.txt")
	assert.NoError(t, err)

	// Verify old path doesn't exist
	old, _ := db.Get("/old/path.txt")
	assert.Nil(t, old)

	// Verify new path exists
	new, _ := db.Get("/new/path.txt")
	assert.NotNil(t, new)
}

func TestListPlansEmpty(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// List plans when none exist
	plans, err := db.ListPlans()
	assert.NoError(t, err)
	assert.Empty(t, plans)
}

func TestAddResourceSetEdgeWithVerification(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create parent and child sets
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "rsparent"})
	require.NoError(t, err)
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "rschild"})
	require.NoError(t, err)

	// Add edge
	err = db.AddResourceSetEdge("rsparent", "rschild")
	assert.NoError(t, err)

	// Verify edge by getting children
	children, err := db.GetResourceSetChildren("rsparent")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(children))
	assert.Equal(t, "rschild", children[0].Name)

	// Verify parent retrieval
	parents, err := db.GetResourceSetParents("rschild")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(parents))
	assert.Equal(t, "rsparent", parents[0].Name)
}

func TestResourceSetDAGMultipleParents(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create hierarchy: parent1 -> child, parent2 -> child (DAG with multiple parents)
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "dagparent1"})
	require.NoError(t, err)
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "dagparent2"})
	require.NoError(t, err)
	_, err = db.CreateResourceSet(&models.ResourceSet{Name: "dagchild"})
	require.NoError(t, err)

	err = db.AddResourceSetEdge("dagparent1", "dagchild")
	require.NoError(t, err)
	err = db.AddResourceSetEdge("dagparent2", "dagchild")
	require.NoError(t, err)

	// Get parents
	parents, err := db.GetResourceSetParents("dagchild")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(parents))
}

func TestGetEntriesByTimeRangeWithRoot(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Unix()

	// Create entries under different roots
	roots := []string{"/root1", "/root2"}
	for _, root := range roots {
		entry := &models.Entry{
			Path: root + "/file.txt", Size: 100, Kind: "file",
			Mtime: now, LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Get entries with root filter
	root := "/root1"
	minDate := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
	maxDate := time.Now().Add(24 * time.Hour).Format("2006-01-02")
	results, err := db.GetEntriesByTimeRange(minDate, maxDate, &root)
	require.NoError(t, err)
	require.Equal(t, 1, len(results), "Expected 1 result but got %d", len(results))
	assert.Contains(t, results[0].Path, "/root1")
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
