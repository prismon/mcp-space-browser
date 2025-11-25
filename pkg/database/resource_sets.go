package database

import (
	"database/sql"
	"fmt"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// Resource Set Operations

// CreateResourceSet creates a new resource set
func (d *DiskDB) CreateResourceSet(set *models.ResourceSet) (int64, error) {
	log.WithFields(logrus.Fields{
		"name": set.Name,
	}).Info("Creating resource set")

	result, err := d.db.Exec(`
		INSERT INTO resource_sets (name, description)
		VALUES (?, ?)
	`, set.Name, set.Description)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetResourceSet retrieves a resource set by name
func (d *DiskDB) GetResourceSet(name string) (*models.ResourceSet, error) {
	var set models.ResourceSet
	var description sql.NullString

	err := d.db.QueryRow(`SELECT id, name, description, created_at, updated_at FROM resource_sets WHERE name = ?`, name).
		Scan(&set.ID, &set.Name, &description, &set.CreatedAt, &set.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		set.Description = &description.String
	}

	return &set, nil
}

// GetResourceSetByID retrieves a resource set by ID
func (d *DiskDB) GetResourceSetByID(id int64) (*models.ResourceSet, error) {
	var set models.ResourceSet
	var description sql.NullString

	err := d.db.QueryRow(`SELECT id, name, description, created_at, updated_at FROM resource_sets WHERE id = ?`, id).
		Scan(&set.ID, &set.Name, &description, &set.CreatedAt, &set.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		set.Description = &description.String
	}

	return &set, nil
}

// ListResourceSets retrieves all resource sets
func (d *DiskDB) ListResourceSets() ([]*models.ResourceSet, error) {
	rows, err := d.db.Query(`SELECT id, name, description, created_at, updated_at FROM resource_sets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []*models.ResourceSet
	for rows.Next() {
		var set models.ResourceSet
		var description sql.NullString

		if err := rows.Scan(&set.ID, &set.Name, &description, &set.CreatedAt, &set.UpdatedAt); err != nil {
			return nil, err
		}

		if description.Valid {
			set.Description = &description.String
		}

		sets = append(sets, &set)
	}

	return sets, rows.Err()
}

// ListResourceSetsWithInfo retrieves all resource sets with entry and child counts
func (d *DiskDB) ListResourceSetsWithInfo() ([]*models.ResourceSetInfo, error) {
	rows, err := d.db.Query(`
		SELECT
			rs.id, rs.name, rs.description, rs.created_at, rs.updated_at,
			(SELECT COUNT(*) FROM resource_set_entries WHERE set_id = rs.id) as entry_count,
			(SELECT COUNT(*) FROM resource_set_edges WHERE parent_id = rs.id) as child_count
		FROM resource_sets rs
		ORDER BY rs.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []*models.ResourceSetInfo
	for rows.Next() {
		var set models.ResourceSetInfo
		var description sql.NullString

		if err := rows.Scan(&set.ID, &set.Name, &description, &set.CreatedAt, &set.UpdatedAt, &set.EntryCount, &set.ChildCount); err != nil {
			return nil, err
		}

		if description.Valid {
			set.Description = &description.String
		}

		sets = append(sets, &set)
	}

	return sets, rows.Err()
}

// UpdateResourceSet updates a resource set's description
func (d *DiskDB) UpdateResourceSet(name string, description *string) error {
	log.WithField("name", name).Info("Updating resource set")
	_, err := d.db.Exec(`
		UPDATE resource_sets
		SET description = ?, updated_at = strftime('%s', 'now')
		WHERE name = ?
	`, description, name)
	return err
}

// DeleteResourceSet deletes a resource set
func (d *DiskDB) DeleteResourceSet(name string) error {
	log.WithField("name", name).Info("Deleting resource set")
	_, err := d.db.Exec(`DELETE FROM resource_sets WHERE name = ?`, name)
	return err
}

// AddToResourceSet adds entries to a resource set
func (d *DiskDB) AddToResourceSet(setName string, paths []string) error {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return err
	}
	if set == nil {
		return fmt.Errorf("resource set '%s' not found", setName)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO resource_set_entries (set_id, entry_path) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, path := range paths {
		if _, err := stmt.Exec(set.ID, path); err != nil {
			tx.Rollback()
			return err
		}
	}

	// Update the set's updated_at timestamp
	if _, err := tx.Exec(`UPDATE resource_sets SET updated_at = strftime('%s', 'now') WHERE id = ?`, set.ID); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"setName": setName,
		"count":   len(paths),
	}).Info("Added entries to resource set")

	return nil
}

// RemoveFromResourceSet removes entries from a resource set
func (d *DiskDB) RemoveFromResourceSet(setName string, paths []string) error {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return err
	}
	if set == nil {
		return fmt.Errorf("resource set '%s' not found", setName)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`DELETE FROM resource_set_entries WHERE set_id = ? AND entry_path = ?`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, path := range paths {
		if _, err := stmt.Exec(set.ID, path); err != nil {
			tx.Rollback()
			return err
		}
	}

	// Update the set's updated_at timestamp
	if _, err := tx.Exec(`UPDATE resource_sets SET updated_at = strftime('%s', 'now') WHERE id = ?`, set.ID); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"setName": setName,
		"count":   len(paths),
	}).Info("Removed entries from resource set")

	return nil
}

// GetResourceSetEntries retrieves all entries in a resource set
func (d *DiskDB) GetResourceSetEntries(setName string) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	rows, err := d.db.Query(`
		SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
		FROM entries e
		JOIN resource_set_entries rse ON e.path = rse.entry_path
		WHERE rse.set_id = ?
		ORDER BY e.path
	`, set.ID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.Entry
	for rows.Next() {
		var entry models.Entry
		var parent sql.NullString

		if err := rows.Scan(&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned); err != nil {
			return nil, err
		}

		if parent.Valid {
			entry.Parent = &parent.String
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// GetResourceSetEntryCount returns the number of entries in a resource set
func (d *DiskDB) GetResourceSetEntryCount(setName string) (int, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return 0, err
	}
	if set == nil {
		return 0, fmt.Errorf("resource set '%s' not found", setName)
	}

	var count int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM resource_set_entries WHERE set_id = ?`, set.ID).Scan(&count)
	return count, err
}

// DAG Operations

// AddResourceSetChild creates a parent-child edge in the DAG
// Returns an error if the edge would create a cycle
func (d *DiskDB) AddResourceSetChild(parentName, childName string) error {
	log.WithFields(logrus.Fields{
		"parent": parentName,
		"child":  childName,
	}).Info("Adding resource set child")

	parent, err := d.GetResourceSet(parentName)
	if err != nil {
		return err
	}
	if parent == nil {
		return fmt.Errorf("parent resource set '%s' not found", parentName)
	}

	child, err := d.GetResourceSet(childName)
	if err != nil {
		return err
	}
	if child == nil {
		return fmt.Errorf("child resource set '%s' not found", childName)
	}

	// Check for self-reference
	if parent.ID == child.ID {
		return fmt.Errorf("cannot add resource set as its own child")
	}

	// Check if this edge would create a cycle
	// (i.e., if child is already an ancestor of parent)
	if d.isAncestor(child.ID, parent.ID) {
		return fmt.Errorf("cannot add edge: would create a cycle (child '%s' is an ancestor of parent '%s')", childName, parentName)
	}

	// Check if edge already exists
	var exists int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM resource_set_edges WHERE parent_id = ? AND child_id = ?`, parent.ID, child.ID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil // Edge already exists, no-op
	}

	// Create the edge
	_, err = d.db.Exec(`INSERT INTO resource_set_edges (parent_id, child_id) VALUES (?, ?)`, parent.ID, child.ID)
	if err != nil {
		return err
	}

	// Update timestamps
	_, err = d.db.Exec(`UPDATE resource_sets SET updated_at = strftime('%s', 'now') WHERE id IN (?, ?)`, parent.ID, child.ID)
	return err
}

// RemoveResourceSetChild removes a parent-child edge from the DAG
func (d *DiskDB) RemoveResourceSetChild(parentName, childName string) error {
	log.WithFields(logrus.Fields{
		"parent": parentName,
		"child":  childName,
	}).Info("Removing resource set child")

	parent, err := d.GetResourceSet(parentName)
	if err != nil {
		return err
	}
	if parent == nil {
		return fmt.Errorf("parent resource set '%s' not found", parentName)
	}

	child, err := d.GetResourceSet(childName)
	if err != nil {
		return err
	}
	if child == nil {
		return fmt.Errorf("child resource set '%s' not found", childName)
	}

	_, err = d.db.Exec(`DELETE FROM resource_set_edges WHERE parent_id = ? AND child_id = ?`, parent.ID, child.ID)
	if err != nil {
		return err
	}

	// Update timestamps
	_, err = d.db.Exec(`UPDATE resource_sets SET updated_at = strftime('%s', 'now') WHERE id IN (?, ?)`, parent.ID, child.ID)
	return err
}

// GetResourceSetChildren returns all immediate children of a resource set
func (d *DiskDB) GetResourceSetChildren(setName string) ([]*models.ResourceSetInfo, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	rows, err := d.db.Query(`
		SELECT
			rs.id, rs.name, rs.description, rs.created_at, rs.updated_at,
			(SELECT COUNT(*) FROM resource_set_entries WHERE set_id = rs.id) as entry_count,
			(SELECT COUNT(*) FROM resource_set_edges WHERE parent_id = rs.id) as child_count
		FROM resource_sets rs
		JOIN resource_set_edges e ON rs.id = e.child_id
		WHERE e.parent_id = ?
		ORDER BY rs.name
	`, set.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var children []*models.ResourceSetInfo
	for rows.Next() {
		var child models.ResourceSetInfo
		var description sql.NullString

		if err := rows.Scan(&child.ID, &child.Name, &description, &child.CreatedAt, &child.UpdatedAt, &child.EntryCount, &child.ChildCount); err != nil {
			return nil, err
		}

		if description.Valid {
			child.Description = &description.String
		}

		children = append(children, &child)
	}

	return children, rows.Err()
}

// GetResourceSetParents returns all immediate parents of a resource set
func (d *DiskDB) GetResourceSetParents(setName string) ([]*models.ResourceSet, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	rows, err := d.db.Query(`
		SELECT rs.id, rs.name, rs.description, rs.created_at, rs.updated_at
		FROM resource_sets rs
		JOIN resource_set_edges e ON rs.id = e.parent_id
		WHERE e.child_id = ?
		ORDER BY rs.name
	`, set.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parents []*models.ResourceSet
	for rows.Next() {
		var parent models.ResourceSet
		var description sql.NullString

		if err := rows.Scan(&parent.ID, &parent.Name, &description, &parent.CreatedAt, &parent.UpdatedAt); err != nil {
			return nil, err
		}

		if description.Valid {
			parent.Description = &description.String
		}

		parents = append(parents, &parent)
	}

	return parents, rows.Err()
}

// GetAllDescendants returns all descendant resource set IDs using recursive CTE
func (d *DiskDB) GetAllDescendants(setName string) ([]int64, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	rows, err := d.db.Query(`
		WITH RECURSIVE descendants(id) AS (
			SELECT id FROM resource_sets WHERE id = ?
			UNION ALL
			SELECT e.child_id
			FROM resource_set_edges e
			JOIN descendants d ON e.parent_id = d.id
		)
		SELECT DISTINCT id FROM descendants
	`, set.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// GetAllAncestors returns all ancestor resource set IDs using recursive CTE
func (d *DiskDB) GetAllAncestors(setName string) ([]int64, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	rows, err := d.db.Query(`
		WITH RECURSIVE ancestors(id) AS (
			SELECT id FROM resource_sets WHERE id = ?
			UNION ALL
			SELECT e.parent_id
			FROM resource_set_edges e
			JOIN ancestors a ON e.child_id = a.id
		)
		SELECT DISTINCT id FROM ancestors
	`, set.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// isAncestor checks if potentialAncestor is an ancestor of node
// Used for cycle detection when adding edges
func (d *DiskDB) isAncestor(potentialAncestorID, nodeID int64) bool {
	var count int
	err := d.db.QueryRow(`
		WITH RECURSIVE ancestors(id) AS (
			SELECT parent_id FROM resource_set_edges WHERE child_id = ?
			UNION ALL
			SELECT e.parent_id
			FROM resource_set_edges e
			JOIN ancestors a ON e.child_id = a.id
		)
		SELECT COUNT(*) FROM ancestors WHERE id = ?
	`, nodeID, potentialAncestorID).Scan(&count)

	if err != nil {
		log.WithError(err).Error("Error checking ancestor relationship")
		return false
	}

	return count > 0
}

// GetResourceSetChildCount returns the number of children for a resource set
func (d *DiskDB) GetResourceSetChildCount(setName string) (int, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return 0, err
	}
	if set == nil {
		return 0, fmt.Errorf("resource set '%s' not found", setName)
	}

	var count int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM resource_set_edges WHERE parent_id = ?`, set.ID).Scan(&count)
	return count, err
}

// Resource Query Operations

// ResourceSum computes aggregate sum of a metric for a resource set (with optional DAG traversal)
func (d *DiskDB) ResourceSum(setName string, metric string, includeChildren bool) (*models.ResourceSumResult, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	// Validate metric field
	validMetrics := map[string]string{
		"size":  "size",
		"count": "1",
	}
	metricColumn, ok := validMetrics[metric]
	if !ok {
		return nil, fmt.Errorf("invalid metric: %s (valid: size, count)", metric)
	}

	var query string
	var args []interface{}

	if includeChildren {
		// Use recursive CTE to get all descendant sets
		query = fmt.Sprintf(`
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM resource_sets WHERE id = ?
				UNION ALL
				SELECT e.child_id
				FROM resource_set_edges e
				JOIN descendants d ON e.parent_id = d.id
			)
			SELECT COALESCE(SUM(e.%s), 0)
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id IN (SELECT id FROM descendants)
		`, metricColumn)
		args = []interface{}{set.ID}
	} else {
		query = fmt.Sprintf(`
			SELECT COALESCE(SUM(e.%s), 0)
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id = ?
		`, metricColumn)
		args = []interface{}{set.ID}
	}

	var value int64
	err = d.db.QueryRow(query, args...).Scan(&value)
	if err != nil {
		return nil, err
	}

	result := &models.ResourceSumResult{
		ResourceSet: setName,
		Metric:      metric,
		Value:       value,
	}

	// Get breakdown by immediate children if DAG traversal is enabled
	if includeChildren {
		children, err := d.GetResourceSetChildren(setName)
		if err == nil && len(children) > 0 {
			result.Breakdown = make([]models.ResourceSumBreakdown, 0, len(children))
			for _, child := range children {
				childResult, err := d.ResourceSum(child.Name, metric, true)
				if err == nil {
					result.Breakdown = append(result.Breakdown, models.ResourceSumBreakdown{
						Name:  child.Name,
						Value: childResult.Value,
					})
				}
			}
		}
	}

	return result, nil
}

// ResourceTimeRange filters entries by time field within a range
func (d *DiskDB) ResourceTimeRange(setName string, field string, minTime, maxTime *int64, includeChildren bool) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	// Validate time field
	validFields := map[string]bool{"mtime": true, "ctime": true}
	if !validFields[field] {
		return nil, fmt.Errorf("invalid time field: %s (valid: mtime, ctime)", field)
	}

	var query string
	var args []interface{}

	if includeChildren {
		query = fmt.Sprintf(`
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM resource_sets WHERE id = ?
				UNION ALL
				SELECT e.child_id
				FROM resource_set_edges e
				JOIN descendants d ON e.parent_id = d.id
			)
			SELECT DISTINCT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id IN (SELECT id FROM descendants)
		`)
		args = []interface{}{set.ID}
	} else {
		query = fmt.Sprintf(`
			SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id = ?
		`)
		args = []interface{}{set.ID}
	}

	if minTime != nil {
		query += fmt.Sprintf(" AND e.%s >= ?", field)
		args = append(args, *minTime)
	}
	if maxTime != nil {
		query += fmt.Sprintf(" AND e.%s <= ?", field)
		args = append(args, *maxTime)
	}

	query += fmt.Sprintf(" ORDER BY e.%s DESC", field)

	return d.queryEntries(query, args)
}

// ResourceMetricRange filters entries by metric value within a range
func (d *DiskDB) ResourceMetricRange(setName string, metric string, minVal, maxVal *int64, includeChildren bool) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	// Validate metric field
	validMetrics := map[string]bool{"size": true}
	if !validMetrics[metric] {
		return nil, fmt.Errorf("invalid metric: %s (valid: size)", metric)
	}

	var query string
	var args []interface{}

	if includeChildren {
		query = `
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM resource_sets WHERE id = ?
				UNION ALL
				SELECT e.child_id
				FROM resource_set_edges e
				JOIN descendants d ON e.parent_id = d.id
			)
			SELECT DISTINCT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id IN (SELECT id FROM descendants)
		`
		args = []interface{}{set.ID}
	} else {
		query = `
			SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id = ?
		`
		args = []interface{}{set.ID}
	}

	if minVal != nil {
		query += fmt.Sprintf(" AND e.%s >= ?", metric)
		args = append(args, *minVal)
	}
	if maxVal != nil {
		query += fmt.Sprintf(" AND e.%s <= ?", metric)
		args = append(args, *maxVal)
	}

	query += fmt.Sprintf(" ORDER BY e.%s DESC", metric)

	return d.queryEntries(query, args)
}

// ResourceIs filters entries by exact match on a field
func (d *DiskDB) ResourceIs(setName string, field string, value string, includeChildren bool) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	// Validate field
	validFields := map[string]bool{"kind": true, "path": true, "parent": true}
	if !validFields[field] {
		return nil, fmt.Errorf("invalid field for exact match: %s (valid: kind, path, parent)", field)
	}

	var query string
	var args []interface{}

	if includeChildren {
		query = fmt.Sprintf(`
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM resource_sets WHERE id = ?
				UNION ALL
				SELECT e.child_id
				FROM resource_set_edges e
				JOIN descendants d ON e.parent_id = d.id
			)
			SELECT DISTINCT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id IN (SELECT id FROM descendants)
			  AND e.%s = ?
			ORDER BY e.path
		`, field)
		args = []interface{}{set.ID, value}
	} else {
		query = fmt.Sprintf(`
			SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id = ?
			  AND e.%s = ?
			ORDER BY e.path
		`, field)
		args = []interface{}{set.ID, value}
	}

	return d.queryEntries(query, args)
}

// ResourceFuzzyMatch filters entries by pattern matching on text fields
func (d *DiskDB) ResourceFuzzyMatch(setName string, field string, pattern string, mode string, caseSensitive bool, includeChildren bool) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", setName)
	}

	// Validate field
	validFields := map[string]bool{"path": true, "parent": true}
	if !validFields[field] {
		return nil, fmt.Errorf("invalid field for fuzzy match: %s (valid: path, parent)", field)
	}

	// Build the match clause based on mode
	var matchClause string
	var matchArg interface{}

	switch mode {
	case "contains":
		if caseSensitive {
			matchClause = fmt.Sprintf("e.%s LIKE '%%' || ? || '%%'", field)
		} else {
			matchClause = fmt.Sprintf("LOWER(e.%s) LIKE '%%' || LOWER(?) || '%%'", field)
		}
		matchArg = pattern
	case "prefix":
		if caseSensitive {
			matchClause = fmt.Sprintf("e.%s LIKE ? || '%%'", field)
		} else {
			matchClause = fmt.Sprintf("LOWER(e.%s) LIKE LOWER(?) || '%%'", field)
		}
		matchArg = pattern
	case "suffix":
		if caseSensitive {
			matchClause = fmt.Sprintf("e.%s LIKE '%%' || ?", field)
		} else {
			matchClause = fmt.Sprintf("LOWER(e.%s) LIKE '%%' || LOWER(?)", field)
		}
		matchArg = pattern
	case "glob":
		matchClause = fmt.Sprintf("e.%s GLOB ?", field)
		matchArg = pattern
	case "regex":
		// SQLite supports REGEXP via user-defined function, but we'll use LIKE for basic support
		// For full regex support, you'd need to register a REGEXP function
		matchClause = fmt.Sprintf("e.%s REGEXP ?", field)
		matchArg = pattern
	default:
		return nil, fmt.Errorf("invalid match mode: %s (valid: contains, prefix, suffix, glob, regex)", mode)
	}

	var query string
	var args []interface{}

	if includeChildren {
		query = fmt.Sprintf(`
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM resource_sets WHERE id = ?
				UNION ALL
				SELECT e.child_id
				FROM resource_set_edges e
				JOIN descendants d ON e.parent_id = d.id
			)
			SELECT DISTINCT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id IN (SELECT id FROM descendants)
			  AND %s
			ORDER BY e.path
		`, matchClause)
		args = []interface{}{set.ID, matchArg}
	} else {
		query = fmt.Sprintf(`
			SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries rse ON e.path = rse.entry_path
			WHERE rse.set_id = ?
			  AND %s
			ORDER BY e.path
		`, matchClause)
		args = []interface{}{set.ID, matchArg}
	}

	return d.queryEntries(query, args)
}

// queryEntries is a helper to execute entry queries
func (d *DiskDB) queryEntries(query string, args []interface{}) ([]*models.Entry, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.Entry
	for rows.Next() {
		var entry models.Entry
		var parent sql.NullString

		if err := rows.Scan(&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned); err != nil {
			return nil, err
		}

		if parent.Valid {
			entry.Parent = &parent.String
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}
