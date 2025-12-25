package database

import (
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to set up test data for resource queries
func setupResourceQueryTestData(t *testing.T, db *DiskDB) {
	// Create selection set
	set := &models.ResourceSet{Name: "test-resources"}
	_, err := db.CreateResourceSet(set)
	require.NoError(t, err)

	// Create test entries
	now := time.Now().Unix()
	entries := []*models.Entry{
		{Path: "/test/file1.txt", Size: 100, Kind: "file", Mtime: now - 3600, Ctime: now - 7200},
		{Path: "/test/file2.jpg", Size: 5000, Kind: "file", Mtime: now - 1800, Ctime: now - 3600},
		{Path: "/test/file3.mp4", Size: 100000, Kind: "file", Mtime: now, Ctime: now - 1800},
		{Path: "/test/subdir", Size: 0, Kind: "directory", Mtime: now, Ctime: now},
		{Path: "/test/subdir/file4.txt", Size: 200, Kind: "file", Mtime: now - 600, Ctime: now - 1200},
	}

	for _, entry := range entries {
		err := db.InsertOrUpdate(entry)
		require.NoError(t, err)
	}

	// Add entries to selection set
	paths := []string{"/test/file1.txt", "/test/file2.jpg", "/test/file3.mp4", "/test/subdir", "/test/subdir/file4.txt"}
	err = db.AddToResourceSet("test-resources", paths)
	require.NoError(t, err)
}

func TestResourceTimeRange(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("filter by mtime", func(t *testing.T) {
		now := time.Now()
		oneHourAgo := now.Add(-time.Hour)
		entries, err := db.ResourceTimeRange("test-resources", "mtime", &oneHourAgo, &now, false)
		assert.NoError(t, err)
		// Should include recent entries
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("filter by ctime", func(t *testing.T) {
		now := time.Now()
		twoHoursAgo := now.Add(-2 * time.Hour)
		entries, err := db.ResourceTimeRange("test-resources", "ctime", &twoHoursAgo, &now, false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("invalid time field", func(t *testing.T) {
		now := time.Now()
		_, err := db.ResourceTimeRange("test-resources", "invalid", &now, &now, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid time field")
	})

	t.Run("non-existent set", func(t *testing.T) {
		now := time.Now()
		_, err := db.ResourceTimeRange("nonexistent", "mtime", &now, &now, false)
		assert.Error(t, err)
	})
}

func TestResourceMetricRange(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("filter by size - min only", func(t *testing.T) {
		minSize := int64(1000)
		entries, err := db.ResourceMetricRange("test-resources", "size", &minSize, nil, false)
		assert.NoError(t, err)
		// Should include files >= 1000 bytes
		for _, e := range entries {
			assert.GreaterOrEqual(t, e.Size, minSize)
		}
	})

	t.Run("filter by size - max only", func(t *testing.T) {
		maxSize := int64(500)
		entries, err := db.ResourceMetricRange("test-resources", "size", nil, &maxSize, false)
		assert.NoError(t, err)
		// Should include files <= 500 bytes
		for _, e := range entries {
			assert.LessOrEqual(t, e.Size, maxSize)
		}
	})

	t.Run("filter by size - range", func(t *testing.T) {
		minSize := int64(100)
		maxSize := int64(10000)
		entries, err := db.ResourceMetricRange("test-resources", "size", &minSize, &maxSize, false)
		assert.NoError(t, err)
		for _, e := range entries {
			assert.GreaterOrEqual(t, e.Size, minSize)
			assert.LessOrEqual(t, e.Size, maxSize)
		}
	})

	t.Run("invalid metric", func(t *testing.T) {
		_, err := db.ResourceMetricRange("test-resources", "invalid", nil, nil, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid metric")
	})
}

func TestResourceIs(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("filter by kind - file", func(t *testing.T) {
		entries, err := db.ResourceIs("test-resources", "kind", "file", false)
		assert.NoError(t, err)
		for _, e := range entries {
			assert.Equal(t, "file", e.Kind)
		}
	})

	t.Run("filter by kind - directory", func(t *testing.T) {
		entries, err := db.ResourceIs("test-resources", "kind", "directory", false)
		assert.NoError(t, err)
		for _, e := range entries {
			assert.Equal(t, "directory", e.Kind)
		}
	})

	t.Run("filter by extension", func(t *testing.T) {
		entries, err := db.ResourceIs("test-resources", "extension", "txt", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("filter by extension with dot", func(t *testing.T) {
		entries, err := db.ResourceIs("test-resources", "extension", ".jpg", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("invalid field", func(t *testing.T) {
		_, err := db.ResourceIs("test-resources", "invalid", "value", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field")
	})
}

func TestResourceFuzzyMatch(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("contains match on path", func(t *testing.T) {
		entries, err := db.ResourceFuzzyMatch("test-resources", "path", "file", "contains", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("prefix match on name", func(t *testing.T) {
		entries, err := db.ResourceFuzzyMatch("test-resources", "name", "file", "prefix", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("suffix match on name", func(t *testing.T) {
		entries, err := db.ResourceFuzzyMatch("test-resources", "name", ".txt", "suffix", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("regex match", func(t *testing.T) {
		entries, err := db.ResourceFuzzyMatch("test-resources", "name", "file[0-9]+\\.txt", "regex", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("glob match", func(t *testing.T) {
		// Note: glob pattern is converted to regex; using simple pattern without special chars
		entries, err := db.ResourceFuzzyMatch("test-resources", "name", "file*", "glob", false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("invalid field", func(t *testing.T) {
		_, err := db.ResourceFuzzyMatch("test-resources", "invalid", "pattern", "contains", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field")
	})

	t.Run("invalid match type", func(t *testing.T) {
		_, err := db.ResourceFuzzyMatch("test-resources", "path", "pattern", "invalid", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid match type")
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		_, err := db.ResourceFuzzyMatch("test-resources", "path", "[invalid(regex", "regex", false)
		assert.Error(t, err)
	})
}

func TestResourceSearch(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("basic search", func(t *testing.T) {
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:  "test-resources",
			Limit: 100,
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.GreaterOrEqual(t, result.TotalCount, 1)
	})

	t.Run("search with kind filter", func(t *testing.T) {
		kind := "file"
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name: "test-resources",
			Kind: &kind,
		})
		assert.NoError(t, err)
		for _, e := range result.Entries {
			assert.Equal(t, "file", e.Kind)
		}
	})

	t.Run("search with extension filter", func(t *testing.T) {
		ext := "txt"
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:      "test-resources",
			Extension: &ext,
		})
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entries), 1)
	})

	t.Run("search with size filters", func(t *testing.T) {
		minSize := int64(100)
		maxSize := int64(10000)
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:    "test-resources",
			MinSize: &minSize,
			MaxSize: &maxSize,
		})
		assert.NoError(t, err)
		for _, e := range result.Entries {
			assert.GreaterOrEqual(t, e.Size, minSize)
			assert.LessOrEqual(t, e.Size, maxSize)
		}
	})

	t.Run("search with path contains", func(t *testing.T) {
		pathContains := "subdir"
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:         "test-resources",
			PathContains: &pathContains,
		})
		assert.NoError(t, err)
		for _, e := range result.Entries {
			assert.Contains(t, e.Path, "subdir")
		}
	})

	t.Run("search with name contains", func(t *testing.T) {
		nameContains := "file"
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:         "test-resources",
			NameContains: &nameContains,
		})
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entries), 1)
	})

	t.Run("search with pagination", func(t *testing.T) {
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:   "test-resources",
			Limit:  2,
			Offset: 0,
		})
		assert.NoError(t, err)
		assert.LessOrEqual(t, len(result.Entries), 2)
	})

	t.Run("search with sorting", func(t *testing.T) {
		result, err := db.ResourceSearch(ResourceSearchParams{
			Name:     "test-resources",
			SortBy:   "size",
			SortDesc: true,
		})
		assert.NoError(t, err)
		if len(result.Entries) > 1 {
			// Verify descending order
			for i := 1; i < len(result.Entries); i++ {
				assert.LessOrEqual(t, result.Entries[i].Size, result.Entries[i-1].Size)
			}
		}
	})
}

func TestGetResourceMetricBreakdown(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("basic breakdown", func(t *testing.T) {
		breakdown, err := db.GetResourceMetricBreakdown("test-resources", false)
		assert.NoError(t, err)
		assert.NotNil(t, breakdown)
		assert.Equal(t, "test-resources", breakdown.ResourceSet)
		assert.Greater(t, breakdown.TotalCount, 0)
		assert.Greater(t, breakdown.FileCount, 0)
	})

	t.Run("nonexistent set", func(t *testing.T) {
		_, err := db.GetResourceMetricBreakdown("nonexistent", false)
		assert.Error(t, err)
	})
}

func TestGetResourceSizeDistribution(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("basic distribution", func(t *testing.T) {
		dist, err := db.GetResourceSizeDistribution("test-resources", false)
		assert.NoError(t, err)
		assert.NotNil(t, dist)
		assert.Equal(t, "test-resources", dist.ResourceSet)
		assert.NotEmpty(t, dist.Buckets)
	})

	t.Run("nonexistent set", func(t *testing.T) {
		_, err := db.GetResourceSizeDistribution("nonexistent", false)
		assert.Error(t, err)
	})
}

func TestGetResourceSetEntryCount(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("count entries", func(t *testing.T) {
		count, err := db.GetResourceSetEntryCount("test-resources")
		assert.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("nonexistent set", func(t *testing.T) {
		_, err := db.GetResourceSetEntryCount("nonexistent")
		assert.Error(t, err)
	})
}

func TestGetResourceSetTotalSize(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("total size", func(t *testing.T) {
		size, err := db.GetResourceSetTotalSize("test-resources", false)
		assert.NoError(t, err)
		assert.Greater(t, size, int64(0))
	})
}

func TestGetResourceTimeStats(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	setupResourceQueryTestData(t, db)

	t.Run("time stats", func(t *testing.T) {
		stats, err := db.GetResourceTimeStats("test-resources", false)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, "test-resources", stats.ResourceSet)
	})

	t.Run("nonexistent set", func(t *testing.T) {
		_, err := db.GetResourceTimeStats("nonexistent", false)
		assert.Error(t, err)
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("globToRegex", func(t *testing.T) {
		tests := []struct {
			glob     string
			expected string
		}{
			{"*.txt", `^.*\\.txt$`},
			{"file?.txt", `^file.\\.txt$`},
			{"test", "^test$"},
		}

		for _, tc := range tests {
			result := globToRegex(tc.glob)
			assert.Equal(t, tc.expected, result, "glob: %s", tc.glob)
		}
	})

	t.Run("getBasename", func(t *testing.T) {
		tests := []struct {
			path     string
			expected string
		}{
			{"/path/to/file.txt", "file.txt"},
			{"/file.txt", "file.txt"},
			{"file.txt", "file.txt"},
		}

		for _, tc := range tests {
			result := getBasename(tc.path)
			assert.Equal(t, tc.expected, result, "path: %s", tc.path)
		}
	})

	t.Run("sortEntries", func(t *testing.T) {
		entries := []*models.Entry{
			{Path: "/b", Size: 200, Mtime: 2},
			{Path: "/a", Size: 100, Mtime: 1},
			{Path: "/c", Size: 300, Mtime: 3},
		}

		// Sort by size ascending
		sortEntries(entries, "size", false)
		assert.Equal(t, int64(100), entries[0].Size)

		// Sort by name ascending
		entries2 := []*models.Entry{
			{Path: "/b", Size: 200, Mtime: 2},
			{Path: "/a", Size: 100, Mtime: 1},
			{Path: "/c", Size: 300, Mtime: 3},
		}
		sortEntries(entries2, "name", false)
		assert.Equal(t, "/a", entries2[0].Path)

		// Sort by mtime descending
		entries3 := []*models.Entry{
			{Path: "/b", Size: 200, Mtime: 2},
			{Path: "/a", Size: 100, Mtime: 1},
			{Path: "/c", Size: 300, Mtime: 3},
		}
		sortEntries(entries3, "mtime", true)
		assert.Equal(t, int64(3), entries3[0].Mtime)
	})
}
