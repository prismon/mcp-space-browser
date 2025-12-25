package database

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// ResourceTimeRange filters entries in a resource set by a time field
// Fields: "mtime", "ctime", "added_at"
func (d *DiskDB) ResourceTimeRange(name, field string, min, max *time.Time, includeChildren bool) ([]*models.Entry, error) {
	log.WithFields(logrus.Fields{
		"name":            name,
		"field":           field,
		"includeChildren": includeChildren,
	}).Debug("Filtering resource by time range")

	// Validate field
	validFields := map[string]bool{
		"mtime":    true,
		"ctime":    true,
		"added_at": true,
	}

	if !validFields[field] {
		return nil, fmt.Errorf("invalid time field: %s (valid: mtime, ctime, added_at)", field)
	}

	// Get entries
	var entries []*models.Entry
	var err error

	if includeChildren {
		entries, err = d.GetAllDescendantEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	} else {
		entries, err = d.GetResourceSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	}

	// For added_at, we need to query differently
	if field == "added_at" {
		return d.filterByAddedAt(name, min, max, includeChildren)
	}

	// Filter by time field
	var filtered []*models.Entry
	for _, entry := range entries {
		var entryTime int64
		switch field {
		case "mtime":
			entryTime = entry.Mtime
		case "ctime":
			entryTime = entry.Ctime
		}

		if min != nil && entryTime < min.Unix() {
			continue
		}
		if max != nil && entryTime > max.Unix() {
			continue
		}

		filtered = append(filtered, entry)
	}

	return filtered, nil
}

// filterByAddedAt filters entries by when they were added to the resource set
func (d *DiskDB) filterByAddedAt(name string, min, max *time.Time, includeChildren bool) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	// Get all set IDs
	setIDs := []int64{set.ID}
	if includeChildren {
		descendants, err := d.GetResourceSetDescendants(name)
		if err != nil {
			return nil, err
		}
		for _, desc := range descendants {
			setIDs = append(setIDs, desc.ID)
		}
	}

	// Build query with time filter
	var args []interface{}
	placeholders := make([]string, len(setIDs))
	for i, id := range setIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
		FROM entries e
		JOIN resource_set_entries sse ON e.path = sse.entry_path
		WHERE sse.set_id IN (%s)
	`, strings.Join(placeholders, ","))

	if min != nil {
		query += " AND sse.added_at >= ?"
		args = append(args, min.Unix())
	}
	if max != nil {
		query += " AND sse.added_at <= ?"
		args = append(args, max.Unix())
	}

	query += " ORDER BY e.path"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return d.scanEntries(rows)
}

// ResourceMetricRange filters entries in a resource set by a numeric metric
// Metrics: "size"
func (d *DiskDB) ResourceMetricRange(name, metric string, min, max *int64, includeChildren bool) ([]*models.Entry, error) {
	log.WithFields(logrus.Fields{
		"name":            name,
		"metric":          metric,
		"includeChildren": includeChildren,
	}).Debug("Filtering resource by metric range")

	// Validate metric
	if metric != "size" {
		return nil, fmt.Errorf("invalid metric: %s (valid: size)", metric)
	}

	// Get entries
	var entries []*models.Entry
	var err error

	if includeChildren {
		entries, err = d.GetAllDescendantEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	} else {
		entries, err = d.GetResourceSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	}

	// Filter by metric
	var filtered []*models.Entry
	for _, entry := range entries {
		var value int64
		switch metric {
		case "size":
			value = entry.Size
		}

		if min != nil && value < *min {
			continue
		}
		if max != nil && value > *max {
			continue
		}

		filtered = append(filtered, entry)
	}

	return filtered, nil
}

// ResourceIs filters entries by exact field match
// Fields: "kind" (file/directory), "extension"
func (d *DiskDB) ResourceIs(name, field, value string, includeChildren bool) ([]*models.Entry, error) {
	log.WithFields(logrus.Fields{
		"name":            name,
		"field":           field,
		"value":           value,
		"includeChildren": includeChildren,
	}).Debug("Filtering resource by exact match")

	// Get entries
	var entries []*models.Entry
	var err error

	if includeChildren {
		entries, err = d.GetAllDescendantEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	} else {
		entries, err = d.GetResourceSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	}

	// Filter by field
	var filtered []*models.Entry
	for _, entry := range entries {
		var matches bool
		switch field {
		case "kind":
			matches = entry.Kind == value
		case "extension":
			ext := getExtension(entry.Path)
			// Handle with or without dot
			if !strings.HasPrefix(value, ".") {
				value = "." + value
			}
			matches = strings.EqualFold(ext, value)
		default:
			return nil, fmt.Errorf("invalid field: %s (valid: kind, extension)", field)
		}

		if matches {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// ResourceFuzzyMatch filters entries by fuzzy pattern matching
// Match types: "contains", "prefix", "suffix", "regex", "glob"
func (d *DiskDB) ResourceFuzzyMatch(name, field, pattern, matchType string, includeChildren bool) ([]*models.Entry, error) {
	log.WithFields(logrus.Fields{
		"name":            name,
		"field":           field,
		"pattern":         pattern,
		"matchType":       matchType,
		"includeChildren": includeChildren,
	}).Debug("Filtering resource by fuzzy match")

	// Get entries
	var entries []*models.Entry
	var err error

	if includeChildren {
		entries, err = d.GetAllDescendantEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	} else {
		entries, err = d.GetResourceSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	}

	// Compile regex if needed
	var re *regexp.Regexp
	if matchType == "regex" {
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// Convert glob to regex if needed
	if matchType == "glob" {
		regexPattern := globToRegex(pattern)
		re, err = regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern: %w", err)
		}
		matchType = "regex" // Use regex matching
	}

	// Filter by pattern
	var filtered []*models.Entry
	for _, entry := range entries {
		var fieldValue string
		switch field {
		case "path":
			fieldValue = entry.Path
		case "name":
			fieldValue = getBasename(entry.Path)
		default:
			return nil, fmt.Errorf("invalid field: %s (valid: path, name)", field)
		}

		var matches bool
		switch matchType {
		case "contains":
			matches = strings.Contains(strings.ToLower(fieldValue), strings.ToLower(pattern))
		case "prefix":
			matches = strings.HasPrefix(strings.ToLower(fieldValue), strings.ToLower(pattern))
		case "suffix":
			matches = strings.HasSuffix(strings.ToLower(fieldValue), strings.ToLower(pattern))
		case "regex":
			matches = re.MatchString(fieldValue)
		default:
			return nil, fmt.Errorf("invalid match type: %s (valid: contains, prefix, suffix, regex, glob)", matchType)
		}

		if matches {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// ResourceSearch performs a comprehensive search across resource set entries
type ResourceSearchParams struct {
	Name            string     `json:"name"`
	IncludeChildren bool       `json:"include_children"`
	Kind            *string    `json:"kind,omitempty"`           // "file" or "directory"
	Extension       *string    `json:"extension,omitempty"`
	PathContains    *string    `json:"path_contains,omitempty"`
	NameContains    *string    `json:"name_contains,omitempty"`
	MinSize         *int64     `json:"min_size,omitempty"`
	MaxSize         *int64     `json:"max_size,omitempty"`
	MinMtime        *time.Time `json:"min_mtime,omitempty"`
	MaxMtime        *time.Time `json:"max_mtime,omitempty"`
	Limit           int        `json:"limit,omitempty"`
	Offset          int        `json:"offset,omitempty"`
	SortBy          string     `json:"sort_by,omitempty"`   // "size", "name", "mtime"
	SortDesc        bool       `json:"sort_desc,omitempty"`
}

// ResourceSearchResult contains search results with pagination info
type ResourceSearchResult struct {
	Entries    []*models.Entry `json:"entries"`
	TotalCount int             `json:"total_count"`
	Offset     int             `json:"offset"`
	Limit      int             `json:"limit"`
	HasMore    bool            `json:"has_more"`
}

// ResourceSearch performs a search with multiple filter criteria
func (d *DiskDB) ResourceSearch(params ResourceSearchParams) (*ResourceSearchResult, error) {
	log.WithFields(logrus.Fields{
		"name":            params.Name,
		"includeChildren": params.IncludeChildren,
	}).Debug("Performing resource search")

	// Get all entries first
	var entries []*models.Entry
	var err error

	if params.IncludeChildren {
		entries, err = d.GetAllDescendantEntries(params.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	} else {
		entries, err = d.GetResourceSetEntries(params.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
	}

	// Apply filters
	var filtered []*models.Entry
	for _, entry := range entries {
		// Kind filter
		if params.Kind != nil && entry.Kind != *params.Kind {
			continue
		}

		// Extension filter
		if params.Extension != nil {
			ext := getExtension(entry.Path)
			expectedExt := *params.Extension
			if !strings.HasPrefix(expectedExt, ".") {
				expectedExt = "." + expectedExt
			}
			if !strings.EqualFold(ext, expectedExt) {
				continue
			}
		}

		// Path contains filter
		if params.PathContains != nil {
			if !strings.Contains(strings.ToLower(entry.Path), strings.ToLower(*params.PathContains)) {
				continue
			}
		}

		// Name contains filter
		if params.NameContains != nil {
			name := getBasename(entry.Path)
			if !strings.Contains(strings.ToLower(name), strings.ToLower(*params.NameContains)) {
				continue
			}
		}

		// Size filters
		if params.MinSize != nil && entry.Size < *params.MinSize {
			continue
		}
		if params.MaxSize != nil && entry.Size > *params.MaxSize {
			continue
		}

		// Time filters
		if params.MinMtime != nil && entry.Mtime < params.MinMtime.Unix() {
			continue
		}
		if params.MaxMtime != nil && entry.Mtime > params.MaxMtime.Unix() {
			continue
		}

		filtered = append(filtered, entry)
	}

	totalCount := len(filtered)

	// Sort
	sortEntries(filtered, params.SortBy, params.SortDesc)

	// Apply pagination
	if params.Limit <= 0 {
		params.Limit = 100
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	start := params.Offset
	if start > len(filtered) {
		start = len(filtered)
	}

	end := start + params.Limit
	if end > len(filtered) {
		end = len(filtered)
	}

	paged := filtered[start:end]

	return &ResourceSearchResult{
		Entries:    paged,
		TotalCount: totalCount,
		Offset:     params.Offset,
		Limit:      params.Limit,
		HasMore:    end < totalCount,
	}, nil
}

// Helper functions

// scanEntries scans rows into Entry slice
func (d *DiskDB) scanEntries(rows *sql.Rows) ([]*models.Entry, error) {
	var entries []*models.Entry
	for rows.Next() {
		var entry models.Entry
		var parent sql.NullString

		if err := rows.Scan(&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}

		if parent.Valid {
			entry.Parent = &parent.String
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entries, nil
}

// globToRegex converts a glob pattern to a regex pattern
func globToRegex(glob string) string {
	// Escape special regex characters
	specialChars := []string{".", "+", "^", "$", "(", ")", "[", "]", "{", "}", "|", "\\"}
	result := glob
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}

	// Convert glob wildcards to regex
	result = strings.ReplaceAll(result, "*", ".*")
	result = strings.ReplaceAll(result, "?", ".")

	return "^" + result + "$"
}

// getBasename extracts the filename from a path
func getBasename(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// sortEntries sorts entries by the specified field
func sortEntries(entries []*models.Entry, sortBy string, desc bool) {
	if len(entries) <= 1 {
		return
	}

	// Simple bubble sort for clarity (could use sort.Slice for efficiency)
	for i := 0; i < len(entries)-1; i++ {
		for j := 0; j < len(entries)-i-1; j++ {
			var swap bool
			switch sortBy {
			case "name":
				nameA := getBasename(entries[j].Path)
				nameB := getBasename(entries[j+1].Path)
				swap = nameA > nameB
			case "mtime":
				swap = entries[j].Mtime > entries[j+1].Mtime
			default: // "size"
				swap = entries[j].Size > entries[j+1].Size
			}

			if desc {
				swap = !swap
			}

			if swap {
				entries[j], entries[j+1] = entries[j+1], entries[j]
			}
		}
	}
}
