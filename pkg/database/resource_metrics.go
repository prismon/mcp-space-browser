package database

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// MetricResult represents an aggregated metric result
type MetricResult struct {
	ResourceSet string       `json:"resource_set"`
	Metric      string       `json:"metric"`
	Value       int64        `json:"value"`
	Breakdown   []MetricPart `json:"breakdown,omitempty"`
}

// MetricPart represents a component of the metric breakdown
type MetricPart struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
	Type  string `json:"type"` // "direct" (entries in this set) or "child" (from child set)
}

// ResourceSum computes aggregate metrics for a resource set
// Supports metrics: "size", "count", "files", "directories"
func (d *DiskDB) ResourceSum(name, metric string, includeChildren bool) (*MetricResult, error) {
	log.WithFields(logrus.Fields{
		"name":            name,
		"metric":          metric,
		"includeChildren": includeChildren,
	}).Debug("Computing resource sum")

	set, err := d.GetSelectionSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	result := &MetricResult{
		ResourceSet: name,
		Metric:      metric,
		Breakdown:   []MetricPart{},
	}

	// Get direct entries metric
	directValue, err := d.computeMetricForSet(set.ID, metric)
	if err != nil {
		return nil, fmt.Errorf("failed to compute direct metric: %w", err)
	}

	result.Value = directValue
	result.Breakdown = append(result.Breakdown, MetricPart{
		Name:  name,
		Value: directValue,
		Type:  "direct",
	})

	if includeChildren {
		// Get all descendants
		descendants, err := d.GetResourceSetDescendants(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get descendants: %w", err)
		}

		// Track which entries we've already counted to avoid double-counting
		// in diamond patterns (where same entry might be in multiple child sets)
		countedEntries := make(map[string]bool)

		// First mark entries from direct set as counted
		directEntries, err := d.getEntryPathsForSet(set.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get direct entries: %w", err)
		}
		for _, path := range directEntries {
			countedEntries[path] = true
		}

		// Add metrics from each descendant, avoiding double-counting
		for _, desc := range descendants {
			childValue, newPaths, err := d.computeMetricForSetExcluding(desc.ID, metric, countedEntries)
			if err != nil {
				return nil, fmt.Errorf("failed to compute metric for %s: %w", desc.Name, err)
			}

			// Mark new entries as counted
			for _, path := range newPaths {
				countedEntries[path] = true
			}

			if childValue > 0 {
				result.Value += childValue
				result.Breakdown = append(result.Breakdown, MetricPart{
					Name:  desc.Name,
					Value: childValue,
					Type:  "child",
				})
			}
		}
	}

	return result, nil
}

// computeMetricForSet computes a metric for all entries in a set
func (d *DiskDB) computeMetricForSet(setID int64, metric string) (int64, error) {
	var query string
	switch metric {
	case "size":
		query = `
			SELECT COALESCE(SUM(e.size), 0)
			FROM entries e
			JOIN selection_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ?
		`
	case "count":
		query = `
			SELECT COUNT(*)
			FROM selection_set_entries
			WHERE set_id = ?
		`
	case "files":
		query = `
			SELECT COUNT(*)
			FROM entries e
			JOIN selection_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ? AND e.kind = 'file'
		`
	case "directories":
		query = `
			SELECT COUNT(*)
			FROM entries e
			JOIN selection_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ? AND e.kind = 'directory'
		`
	default:
		return 0, fmt.Errorf("unsupported metric: %s", metric)
	}

	var value int64
	err := d.db.QueryRow(query, setID).Scan(&value)
	if err != nil {
		return 0, err
	}

	return value, nil
}

// computeMetricForSetExcluding computes metric for set excluding already-counted entries
// Returns the metric value and the list of new paths that were counted
func (d *DiskDB) computeMetricForSetExcluding(setID int64, metric string, excluded map[string]bool) (int64, []string, error) {
	// Get all entry paths for this set
	rows, err := d.db.Query(`
		SELECT sse.entry_path, e.size, e.kind
		FROM selection_set_entries sse
		JOIN entries e ON sse.entry_path = e.path
		WHERE sse.set_id = ?
	`, setID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var value int64
	var newPaths []string

	for rows.Next() {
		var path string
		var size int64
		var kind string

		if err := rows.Scan(&path, &size, &kind); err != nil {
			return 0, nil, err
		}

		// Skip if already counted
		if excluded[path] {
			continue
		}

		newPaths = append(newPaths, path)

		switch metric {
		case "size":
			value += size
		case "count":
			value++
		case "files":
			if kind == "file" {
				value++
			}
		case "directories":
			if kind == "directory" {
				value++
			}
		}
	}

	return value, newPaths, rows.Err()
}

// getEntryPathsForSet returns all entry paths in a set
func (d *DiskDB) getEntryPathsForSet(setID int64) ([]string, error) {
	rows, err := d.db.Query(`SELECT entry_path FROM selection_set_entries WHERE set_id = ?`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}

	return paths, rows.Err()
}

// ResourceMetricBreakdown returns a detailed breakdown of metrics by file type
type ResourceMetricBreakdown struct {
	ResourceSet string                `json:"resource_set"`
	TotalSize   int64                 `json:"total_size"`
	TotalCount  int                   `json:"total_count"`
	FileCount   int                   `json:"file_count"`
	DirCount    int                   `json:"directory_count"`
	ByExtension map[string]ExtMetrics `json:"by_extension,omitempty"`
}

// ExtMetrics represents metrics for a file extension
type ExtMetrics struct {
	Count int   `json:"count"`
	Size  int64 `json:"size"`
}

// GetResourceMetricBreakdown returns detailed metrics breakdown for a resource set
func (d *DiskDB) GetResourceMetricBreakdown(name string, includeChildren bool) (*ResourceMetricBreakdown, error) {
	set, err := d.GetSelectionSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	// Get all set IDs to query
	setIDs := []int64{set.ID}
	if includeChildren {
		descendants, err := d.GetResourceSetDescendants(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get descendants: %w", err)
		}
		for _, desc := range descendants {
			setIDs = append(setIDs, desc.ID)
		}
	}

	breakdown := &ResourceMetricBreakdown{
		ResourceSet: name,
		ByExtension: make(map[string]ExtMetrics),
	}

	// Track counted entries to avoid double-counting
	counted := make(map[string]bool)

	for _, setID := range setIDs {
		rows, err := d.db.Query(`
			SELECT e.path, e.size, e.kind
			FROM entries e
			JOIN selection_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ?
		`, setID)
		if err != nil {
			return nil, fmt.Errorf("failed to query entries: %w", err)
		}

		for rows.Next() {
			var path string
			var size int64
			var kind string

			if err := rows.Scan(&path, &size, &kind); err != nil {
				rows.Close()
				return nil, err
			}

			if counted[path] {
				continue
			}
			counted[path] = true

			breakdown.TotalSize += size
			breakdown.TotalCount++

			if kind == "file" {
				breakdown.FileCount++
				// Extract extension
				ext := getExtension(path)
				extMetrics := breakdown.ByExtension[ext]
				extMetrics.Count++
				extMetrics.Size += size
				breakdown.ByExtension[ext] = extMetrics
			} else {
				breakdown.DirCount++
			}
		}
		rows.Close()
	}

	return breakdown, nil
}

// getExtension extracts the file extension from a path
func getExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
		if path[i] == '/' {
			break
		}
	}
	return "(no extension)"
}

// ResourceSizeDistribution represents size distribution statistics
type ResourceSizeDistribution struct {
	ResourceSet string          `json:"resource_set"`
	Buckets     []SizeBucket    `json:"buckets"`
	Statistics  SizeStatistics  `json:"statistics"`
}

// SizeBucket represents a size range bucket
type SizeBucket struct {
	Label    string `json:"label"`
	MinSize  int64  `json:"min_size"`
	MaxSize  int64  `json:"max_size"`
	Count    int    `json:"count"`
	TotalSize int64 `json:"total_size"`
}

// SizeStatistics represents size statistics
type SizeStatistics struct {
	MinSize     int64 `json:"min_size"`
	MaxSize     int64 `json:"max_size"`
	AvgSize     int64 `json:"avg_size"`
	MedianSize  int64 `json:"median_size"`
	TotalSize   int64 `json:"total_size"`
	TotalCount  int   `json:"total_count"`
}

// GetResourceSizeDistribution returns size distribution for a resource set
func (d *DiskDB) GetResourceSizeDistribution(name string, includeChildren bool) (*ResourceSizeDistribution, error) {
	set, err := d.GetSelectionSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	// Define size buckets
	buckets := []SizeBucket{
		{Label: "< 1KB", MinSize: 0, MaxSize: 1024},
		{Label: "1KB - 10KB", MinSize: 1024, MaxSize: 10 * 1024},
		{Label: "10KB - 100KB", MinSize: 10 * 1024, MaxSize: 100 * 1024},
		{Label: "100KB - 1MB", MinSize: 100 * 1024, MaxSize: 1024 * 1024},
		{Label: "1MB - 10MB", MinSize: 1024 * 1024, MaxSize: 10 * 1024 * 1024},
		{Label: "10MB - 100MB", MinSize: 10 * 1024 * 1024, MaxSize: 100 * 1024 * 1024},
		{Label: "100MB - 1GB", MinSize: 100 * 1024 * 1024, MaxSize: 1024 * 1024 * 1024},
		{Label: "> 1GB", MinSize: 1024 * 1024 * 1024, MaxSize: -1},
	}

	// Get all entries
	var entries []int64
	if includeChildren {
		allEntries, err := d.GetAllDescendantEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
		for _, e := range allEntries {
			if e.Kind == "file" {
				entries = append(entries, e.Size)
			}
		}
	} else {
		rows, err := d.db.Query(`
			SELECT e.size
			FROM entries e
			JOIN selection_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ? AND e.kind = 'file'
		`, set.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to query entries: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var size int64
			if err := rows.Scan(&size); err != nil {
				return nil, err
			}
			entries = append(entries, size)
		}
	}

	// Compute statistics
	stats := SizeStatistics{TotalCount: len(entries)}
	if len(entries) > 0 {
		stats.MinSize = entries[0]
		stats.MaxSize = entries[0]
		for _, size := range entries {
			stats.TotalSize += size
			if size < stats.MinSize {
				stats.MinSize = size
			}
			if size > stats.MaxSize {
				stats.MaxSize = size
			}

			// Bucket the entry
			for i := range buckets {
				if size >= buckets[i].MinSize && (buckets[i].MaxSize == -1 || size < buckets[i].MaxSize) {
					buckets[i].Count++
					buckets[i].TotalSize += size
					break
				}
			}
		}
		stats.AvgSize = stats.TotalSize / int64(len(entries))
	}

	return &ResourceSizeDistribution{
		ResourceSet: name,
		Buckets:     buckets,
		Statistics:  stats,
	}, nil
}

// GetResourceSetEntryCount returns the count of direct entries in a resource set
func (d *DiskDB) GetResourceSetEntryCount(name string) (int, error) {
	set, err := d.GetSelectionSet(name)
	if err != nil {
		return 0, err
	}
	if set == nil {
		return 0, fmt.Errorf("resource set '%s' not found", name)
	}

	var count int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM selection_set_entries WHERE set_id = ?`, set.ID).Scan(&count)
	return count, err
}

// GetResourceSetTotalSize returns total size of entries in a resource set
func (d *DiskDB) GetResourceSetTotalSize(name string, includeChildren bool) (int64, error) {
	result, err := d.ResourceSum(name, "size", includeChildren)
	if err != nil {
		return 0, err
	}
	return result.Value, nil
}

// ResourceTimeStats represents time-based statistics
type ResourceTimeStats struct {
	ResourceSet  string `json:"resource_set"`
	OldestMtime  int64  `json:"oldest_mtime"`
	NewestMtime  int64  `json:"newest_mtime"`
	OldestCtime  int64  `json:"oldest_ctime"`
	NewestCtime  int64  `json:"newest_ctime"`
	OldestPath   string `json:"oldest_path"`
	NewestPath   string `json:"newest_path"`
}

// GetResourceTimeStats returns time statistics for a resource set
func (d *DiskDB) GetResourceTimeStats(name string, includeChildren bool) (*ResourceTimeStats, error) {
	var entries []*entryWithPath

	if includeChildren {
		allEntries, err := d.GetAllDescendantEntries(name)
		if err != nil {
			return nil, err
		}
		for _, e := range allEntries {
			entries = append(entries, &entryWithPath{path: e.Path, mtime: e.Mtime, ctime: e.Ctime})
		}
	} else {
		set, err := d.GetSelectionSet(name)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, fmt.Errorf("resource set '%s' not found", name)
		}

		rows, err := d.db.Query(`
			SELECT e.path, e.mtime, e.ctime
			FROM entries e
			JOIN selection_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ?
		`, set.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var e entryWithPath
			if err := rows.Scan(&e.path, &e.mtime, &e.ctime); err != nil {
				return nil, err
			}
			entries = append(entries, &e)
		}
	}

	if len(entries) == 0 {
		return &ResourceTimeStats{ResourceSet: name}, nil
	}

	stats := &ResourceTimeStats{
		ResourceSet:  name,
		OldestMtime:  entries[0].mtime,
		NewestMtime:  entries[0].mtime,
		OldestCtime:  entries[0].ctime,
		NewestCtime:  entries[0].ctime,
		OldestPath:   entries[0].path,
		NewestPath:   entries[0].path,
	}

	for _, e := range entries {
		if e.mtime < stats.OldestMtime {
			stats.OldestMtime = e.mtime
			stats.OldestPath = e.path
		}
		if e.mtime > stats.NewestMtime {
			stats.NewestMtime = e.mtime
			stats.NewestPath = e.path
		}
		if e.ctime < stats.OldestCtime {
			stats.OldestCtime = e.ctime
		}
		if e.ctime > stats.NewestCtime {
			stats.NewestCtime = e.ctime
		}
	}

	return stats, nil
}

type entryWithPath struct {
	path  string
	mtime int64
	ctime int64
}
