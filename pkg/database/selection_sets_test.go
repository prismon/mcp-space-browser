package database

import (
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSelectionSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	desc := "Test selection set"
	set := &models.SelectionSet{
		Name:         "test-set",
		Description:  &desc,
		CriteriaType: "user_selected",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}

	id, err := db.CreateSelectionSet(set)
	assert.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestGetSelectionSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test getting non-existent set
	set, err := db.GetSelectionSet("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, set)

	// Create and retrieve
	desc := "My set"
	newSet := &models.SelectionSet{
		Name:         "my-set",
		Description:  &desc,
		CriteriaType: "tool_query",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}

	id, err := db.CreateSelectionSet(newSet)
	require.NoError(t, err)

	retrieved, err := db.GetSelectionSet("my-set")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, id, retrieved.ID)
	assert.Equal(t, "my-set", retrieved.Name)
	assert.Equal(t, "tool_query", retrieved.CriteriaType)
}

func TestListSelectionSets(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple sets
	for i := 0; i < 3; i++ {
		name := string(rune('a' + i))
		set := &models.SelectionSet{
			Name:         name,
			CriteriaType: "user_selected",
			CreatedAt:    time.Now().Unix(),
			UpdatedAt:    time.Now().Unix(),
		}
		_, err := db.CreateSelectionSet(set)
		require.NoError(t, err)
	}

	sets, err := db.ListSelectionSets()
	assert.NoError(t, err)
	assert.Len(t, sets, 3)
}

func TestDeleteSelectionSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	set := &models.SelectionSet{
		Name:         "to-delete",
		CriteriaType: "user_selected",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}

	_, err = db.CreateSelectionSet(set)
	require.NoError(t, err)

	// Delete it
	err = db.DeleteSelectionSet("to-delete")
	assert.NoError(t, err)

	// Verify it's gone
	retrieved, err := db.GetSelectionSet("to-delete")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestAddToSelectionSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create set
	set := &models.SelectionSet{
		Name:         "file-set",
		CriteriaType: "user_selected",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err = db.CreateSelectionSet(set)
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
	err = db.AddToSelectionSet("file-set", paths)
	assert.NoError(t, err)

	// Verify entries were added
	entries, err := db.GetSelectionSetEntries("file-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestRemoveFromSelectionSet(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create set
	set := &models.SelectionSet{
		Name:         "removal-set",
		CriteriaType: "user_selected",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err = db.CreateSelectionSet(set)
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
	err = db.AddToSelectionSet("removal-set", paths)
	require.NoError(t, err)

	// Remove one
	err = db.RemoveFromSelectionSet("removal-set", []string{"/test/file2.txt"})
	assert.NoError(t, err)

	// Verify only 2 remain
	entries, err := db.GetSelectionSetEntries("removal-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetSelectionSetEntries(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test non-existent set
	entries, err := db.GetSelectionSetEntries("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, entries)

	// Create set and add entries
	set := &models.SelectionSet{
		Name:         "entries-set",
		CriteriaType: "user_selected",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err = db.CreateSelectionSet(set)
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
	err = db.AddToSelectionSet("entries-set", paths)
	require.NoError(t, err)

	// Get entries
	entries, err = db.GetSelectionSetEntries("entries-set")
	assert.NoError(t, err)
	assert.Len(t, entries, 3)
}
