package database

import (
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResourceSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	desc := "Test selection set"
	set := &models.ResourceSet{
		Name:        "test-set",
		Description: &desc,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	id, err := db.CreateResourceSet(set)
	assert.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestGetResourceSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test getting non-existent set
	set, err := db.GetResourceSet("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, set)

	// Create and retrieve
	desc := "My set"
	newSet := &models.ResourceSet{
		Name:        "my-set",
		Description: &desc,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	id, err := db.CreateResourceSet(newSet)
	require.NoError(t, err)

	retrieved, err := db.GetResourceSet("my-set")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, id, retrieved.ID)
	assert.Equal(t, "my-set", retrieved.Name)
}

func TestListResourceSets(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple sets
	for i := 0; i < 3; i++ {
		name := string(rune('a' + i))
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	sets, err := db.ListResourceSets()
	assert.NoError(t, err)
	assert.Len(t, sets, 3)
}

func TestDeleteResourceSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	set := &models.ResourceSet{
		Name:      "to-delete",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	// Delete it
	err = db.DeleteResourceSet("to-delete")
	assert.NoError(t, err)

	// Verify it's gone
	retrieved, err := db.GetResourceSet("to-delete")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestAddToResourceSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create set
	set := &models.ResourceSet{
		Name:      "file-set",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	// Create some entries
	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		entry := &models.Entry{
			Path:        "/test/file" + string(rune('a'+i)) + ".txt",
			Size:        100,
			Kind:        "file",
			Ctime:       now,
			Mtime:       now,
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Add entries to set
	paths := []string{"/test/filea.txt", "/test/fileb.txt"}
	err = db.AddToResourceSet("file-set", paths)
	assert.NoError(t, err)

	// Verify entries were added
	entries, err := db.GetResourceSetEntries("file-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestRemoveFromResourceSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create set
	set := &models.ResourceSet{
		Name:      "removal-set",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	// Create entries
	now := time.Now().Unix()
	paths := []string{"/test/file1.txt", "/test/file2.txt", "/test/file3.txt"}
	for _, path := range paths {
		entry := &models.Entry{
			Path:        path,
			Size:        100,
			Kind:        "file",
			Ctime:       now,
			Mtime:       now,
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	// Add all to set
	err = db.AddToResourceSet("removal-set", paths)
	require.NoError(t, err)

	// Remove one
	err = db.RemoveFromResourceSet("removal-set", []string{"/test/file2.txt"})
	assert.NoError(t, err)

	// Verify only 2 remain
	entries, err := db.GetResourceSetEntries("removal-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetResourceSetEntries(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test non-existent set
	entries, err := db.GetResourceSetEntries("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, entries)

	// Create set and add entries
	set := &models.ResourceSet{
		Name:      "entries-set",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	_, err = db.CreateResourceSet(set)
	require.NoError(t, err)

	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		entry := &models.Entry{
			Path:        "/entries/file" + string(rune('a'+i)) + ".txt",
			Size:        int64(i * 100),
			Kind:        "file",
			Ctime:       now,
			Mtime:       now,
			LastScanned: now,
		}
		db.InsertOrUpdate(entry)
	}

	paths := []string{
		"/entries/filea.txt",
		"/entries/fileb.txt",
		"/entries/filec.txt",
	}
	err = db.AddToResourceSet("entries-set", paths)
	require.NoError(t, err)

	// Get entries
	entries, err = db.GetResourceSetEntries("entries-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 3)
}
