package database

import (
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddResourceSetEdge(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create parent and child sets
	parent := &models.ResourceSet{
		Name:      "parent",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	child := &models.ResourceSet{
		Name:      "child",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	_, err = db.CreateResourceSet(parent)
	require.NoError(t, err)

	_, err = db.CreateResourceSet(child)
	require.NoError(t, err)

	// Add edge
	err = db.AddResourceSetEdge("parent", "child")
	assert.NoError(t, err)

	// Verify edge exists
	children, err := db.GetResourceSetChildren("parent")
	assert.NoError(t, err)
	assert.Len(t, children, 1)
	assert.Equal(t, "child", children[0].Name)

	parents, err := db.GetResourceSetParents("child")
	assert.NoError(t, err)
	assert.Len(t, parents, 1)
	assert.Equal(t, "parent", parents[0].Name)
}

func TestAddResourceSetEdge_CycleDetection(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create sets: A -> B -> C
	for _, name := range []string{"A", "B", "C"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	// Create chain A -> B -> C
	err = db.AddResourceSetEdge("A", "B")
	require.NoError(t, err)

	err = db.AddResourceSetEdge("B", "C")
	require.NoError(t, err)

	// Try to create cycle C -> A (should fail)
	err = db.AddResourceSetEdge("C", "A")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestRemoveResourceSetEdge(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create and link sets
	for _, name := range []string{"parent", "child"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	err = db.AddResourceSetEdge("parent", "child")
	require.NoError(t, err)

	// Remove edge
	err = db.RemoveResourceSetEdge("parent", "child")
	assert.NoError(t, err)

	// Verify edge is gone
	children, err := db.GetResourceSetChildren("parent")
	assert.NoError(t, err)
	assert.Len(t, children, 0)
}

func TestGetResourceSetDescendants(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a tree structure:
	//       root
	//      /    \
	//    child1  child2
	//      |
	//   grandchild

	for _, name := range []string{"root", "child1", "child2", "grandchild"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("root", "child1")
	db.AddResourceSetEdge("root", "child2")
	db.AddResourceSetEdge("child1", "grandchild")

	// Get all descendants of root
	descendants, err := db.GetResourceSetDescendants("root")
	assert.NoError(t, err)
	assert.Len(t, descendants, 3) // child1, child2, grandchild

	// Get descendants of child1
	descendants, err = db.GetResourceSetDescendants("child1")
	assert.NoError(t, err)
	assert.Len(t, descendants, 1)
	assert.Equal(t, "grandchild", descendants[0].Name)
}

func TestGetResourceSetAncestors(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create tree
	for _, name := range []string{"root", "child", "grandchild"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("root", "child")
	db.AddResourceSetEdge("child", "grandchild")

	// Get ancestors of grandchild
	ancestors, err := db.GetResourceSetAncestors("grandchild")
	assert.NoError(t, err)
	assert.Len(t, ancestors, 2) // child, root
}

func TestMultipleParents_DAG(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a diamond pattern:
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D

	for _, name := range []string{"A", "B", "C", "D"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("A", "B")
	db.AddResourceSetEdge("A", "C")
	db.AddResourceSetEdge("B", "D")
	db.AddResourceSetEdge("C", "D")

	// D should have two parents
	parents, err := db.GetResourceSetParents("D")
	assert.NoError(t, err)
	assert.Len(t, parents, 2)

	// D should have two ancestors (B, C) plus A
	ancestors, err := db.GetResourceSetAncestors("D")
	assert.NoError(t, err)
	assert.Len(t, ancestors, 3) // A, B, C
}

func TestGetAllDescendantEntries(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create sets
	for _, name := range []string{"parent", "child"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("parent", "child")

	// Create entries
	now := time.Now().Unix()
	entries := []*models.Entry{
		{Path: "/file1.txt", Size: 100, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/file2.txt", Size: 200, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/child/file3.txt", Size: 300, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
	}

	for _, e := range entries {
		db.InsertOrUpdate(e)
	}

	// Add entries to sets
	db.AddToResourceSet("parent", []string{"/file1.txt", "/file2.txt"})
	db.AddToResourceSet("child", []string{"/child/file3.txt"})

	// Get all entries from parent and children
	allEntries, err := db.GetAllDescendantEntries("parent")
	assert.NoError(t, err)
	assert.Len(t, allEntries, 3)
}

func TestGetResourceSetStats(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create sets with edges
	for _, name := range []string{"root", "child1", "child2"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("root", "child1")
	db.AddResourceSetEdge("root", "child2")

	// Create entries
	now := time.Now().Unix()
	entry := &models.Entry{
		Path:        "/test.txt",
		Size:        1000,
		Kind:        "file",
		Ctime:       now,
		Mtime:       now,
		LastScanned: now,
	}
	db.InsertOrUpdate(entry)
	db.AddToResourceSet("root", []string{"/test.txt"})

	// Get stats
	stats, err := db.GetResourceSetStats("root")
	assert.NoError(t, err)
	assert.Equal(t, "root", stats.Name)
	assert.Equal(t, 1, stats.EntryCount)
	assert.Equal(t, int64(1000), stats.TotalSize)
	assert.Equal(t, 2, stats.ChildCount)
	assert.Equal(t, 0, stats.ParentCount)
}

func TestResourceSum(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create sets
	for _, name := range []string{"parent", "child"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("parent", "child")

	// Create entries
	now := time.Now().Unix()
	db.InsertOrUpdate(&models.Entry{Path: "/file1.txt", Size: 100, Kind: "file", Ctime: now, Mtime: now, LastScanned: now})
	db.InsertOrUpdate(&models.Entry{Path: "/file2.txt", Size: 200, Kind: "file", Ctime: now, Mtime: now, LastScanned: now})

	db.AddToResourceSet("parent", []string{"/file1.txt"})
	db.AddToResourceSet("child", []string{"/file2.txt"})

	// Test size metric without children
	result, err := db.ResourceSum("parent", "size", false)
	assert.NoError(t, err)
	assert.Equal(t, int64(100), result.Value)

	// Test size metric with children
	result, err = db.ResourceSum("parent", "size", true)
	assert.NoError(t, err)
	assert.Equal(t, int64(300), result.Value)

	// Test count metric
	result, err = db.ResourceSum("parent", "count", true)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), result.Value)
}

func TestResourceSum_DiamondPattern(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a diamond pattern where an entry exists in both B and C
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D

	for _, name := range []string{"A", "B", "C", "D"} {
		set := &models.ResourceSet{
			Name:      name,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		_, err := db.CreateResourceSet(set)
		require.NoError(t, err)
	}

	db.AddResourceSetEdge("A", "B")
	db.AddResourceSetEdge("A", "C")
	db.AddResourceSetEdge("B", "D")
	db.AddResourceSetEdge("C", "D")

	// Create entries
	now := time.Now().Unix()
	db.InsertOrUpdate(&models.Entry{Path: "/shared.txt", Size: 500, Kind: "file", Ctime: now, Mtime: now, LastScanned: now})
	db.InsertOrUpdate(&models.Entry{Path: "/only-b.txt", Size: 100, Kind: "file", Ctime: now, Mtime: now, LastScanned: now})

	// Add shared file to both B and C
	db.AddToResourceSet("B", []string{"/shared.txt", "/only-b.txt"})
	db.AddToResourceSet("C", []string{"/shared.txt"})

	// Sum from A with children should count shared.txt only once
	result, err := db.ResourceSum("A", "size", true)
	assert.NoError(t, err)
	assert.Equal(t, int64(600), result.Value) // 500 + 100, not 500 + 500 + 100
}
