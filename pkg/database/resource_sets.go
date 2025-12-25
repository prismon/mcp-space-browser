package database

import (
	"database/sql"
	"fmt"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// ResourceSetEdge represents a parent-child relationship in the DAG
type ResourceSetEdge struct {
	ParentID int64 `db:"parent_id" json:"parent_id"`
	ChildID  int64 `db:"child_id" json:"child_id"`
	AddedAt  int64 `db:"added_at" json:"added_at"`
}

// AddResourceSetEdge creates a parent-child edge in the DAG
// Returns error if edge would create a cycle
func (d *DiskDB) AddResourceSetEdge(parentName, childName string) error {
	log.WithFields(logrus.Fields{
		"parent": parentName,
		"child":  childName,
	}).Debug("Adding resource set edge")

	// Get parent set
	parent, err := d.GetResourceSet(parentName)
	if err != nil {
		return fmt.Errorf("failed to get parent set: %w", err)
	}
	if parent == nil {
		return fmt.Errorf("parent resource set '%s' not found", parentName)
	}

	// Get child set
	child, err := d.GetResourceSet(childName)
	if err != nil {
		return fmt.Errorf("failed to get child set: %w", err)
	}
	if child == nil {
		return fmt.Errorf("child resource set '%s' not found", childName)
	}

	// Check for cycles: child cannot be an ancestor of parent
	if d.isAncestor(child.ID, parent.ID) {
		return fmt.Errorf("adding edge would create a cycle: %s is an ancestor of %s", childName, parentName)
	}

	// Insert edge
	_, err = d.db.Exec(
		`INSERT OR IGNORE INTO resource_set_edges (parent_id, child_id) VALUES (?, ?)`,
		parent.ID, child.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to add edge: %w", err)
	}

	log.WithFields(logrus.Fields{
		"parent": parentName,
		"child":  childName,
	}).Info("Resource set edge added")

	return nil
}

// RemoveResourceSetEdge removes a parent-child edge from the DAG
func (d *DiskDB) RemoveResourceSetEdge(parentName, childName string) error {
	log.WithFields(logrus.Fields{
		"parent": parentName,
		"child":  childName,
	}).Debug("Removing resource set edge")

	// Get parent set
	parent, err := d.GetResourceSet(parentName)
	if err != nil {
		return fmt.Errorf("failed to get parent set: %w", err)
	}
	if parent == nil {
		return fmt.Errorf("parent resource set '%s' not found", parentName)
	}

	// Get child set
	child, err := d.GetResourceSet(childName)
	if err != nil {
		return fmt.Errorf("failed to get child set: %w", err)
	}
	if child == nil {
		return fmt.Errorf("child resource set '%s' not found", childName)
	}

	// Delete edge
	result, err := d.db.Exec(
		`DELETE FROM resource_set_edges WHERE parent_id = ? AND child_id = ?`,
		parent.ID, child.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to remove edge: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("edge from '%s' to '%s' not found", parentName, childName)
	}

	log.WithFields(logrus.Fields{
		"parent": parentName,
		"child":  childName,
	}).Info("Resource set edge removed")

	return nil
}

// GetResourceSetChildren returns all immediate child resource sets
func (d *DiskDB) GetResourceSetChildren(name string) ([]*models.ResourceSet, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	rows, err := d.db.Query(`
		SELECT s.id, s.name, s.description, s.created_at, s.updated_at
		FROM resource_sets s
		JOIN resource_set_edges e ON s.id = e.child_id
		WHERE e.parent_id = ?
		ORDER BY s.name
	`, set.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query children: %w", err)
	}
	defer rows.Close()

	return d.scanResourceSets(rows)
}

// GetResourceSetParents returns all immediate parent resource sets
func (d *DiskDB) GetResourceSetParents(name string) ([]*models.ResourceSet, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	rows, err := d.db.Query(`
		SELECT s.id, s.name, s.description, s.created_at, s.updated_at
		FROM resource_sets s
		JOIN resource_set_edges e ON s.id = e.parent_id
		WHERE e.child_id = ?
		ORDER BY s.name
	`, set.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query parents: %w", err)
	}
	defer rows.Close()

	return d.scanResourceSets(rows)
}

// GetResourceSetDescendants returns all descendant resource sets (recursive children)
func (d *DiskDB) GetResourceSetDescendants(name string) ([]*models.ResourceSet, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	// Use BFS to find all descendants
	visited := make(map[int64]bool)
	queue := []int64{set.ID}
	var descendantIDs []int64

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		// Don't add the starting node to descendants
		if currentID != set.ID {
			descendantIDs = append(descendantIDs, currentID)
		}

		// Get children
		rows, err := d.db.Query(`SELECT child_id FROM resource_set_edges WHERE parent_id = ?`, currentID)
		if err != nil {
			return nil, fmt.Errorf("failed to query children: %w", err)
		}

		for rows.Next() {
			var childID int64
			if err := rows.Scan(&childID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan child ID: %w", err)
			}
			if !visited[childID] {
				queue = append(queue, childID)
			}
		}
		rows.Close()
	}

	if len(descendantIDs) == 0 {
		return []*models.ResourceSet{}, nil
	}

	return d.getResourceSetsByIDs(descendantIDs)
}

// GetResourceSetAncestors returns all ancestor resource sets (recursive parents)
func (d *DiskDB) GetResourceSetAncestors(name string) ([]*models.ResourceSet, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	// Use BFS to find all ancestors
	visited := make(map[int64]bool)
	queue := []int64{set.ID}
	var ancestorIDs []int64

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		// Don't add the starting node to ancestors
		if currentID != set.ID {
			ancestorIDs = append(ancestorIDs, currentID)
		}

		// Get parents
		rows, err := d.db.Query(`SELECT parent_id FROM resource_set_edges WHERE child_id = ?`, currentID)
		if err != nil {
			return nil, fmt.Errorf("failed to query parents: %w", err)
		}

		for rows.Next() {
			var parentID int64
			if err := rows.Scan(&parentID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan parent ID: %w", err)
			}
			if !visited[parentID] {
				queue = append(queue, parentID)
			}
		}
		rows.Close()
	}

	if len(ancestorIDs) == 0 {
		return []*models.ResourceSet{}, nil
	}

	return d.getResourceSetsByIDs(ancestorIDs)
}

// isAncestor checks if potentialAncestor is an ancestor of node
// Used for cycle detection when adding edges
func (d *DiskDB) isAncestor(potentialAncestorID, nodeID int64) bool {
	// BFS from nodeID upward to see if we reach potentialAncestorID
	visited := make(map[int64]bool)
	queue := []int64{nodeID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if currentID == potentialAncestorID {
			return true
		}

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		// Get parents
		rows, err := d.db.Query(`SELECT parent_id FROM resource_set_edges WHERE child_id = ?`, currentID)
		if err != nil {
			return false // On error, assume not an ancestor
		}

		for rows.Next() {
			var parentID int64
			if err := rows.Scan(&parentID); err != nil {
				rows.Close()
				return false
			}
			if !visited[parentID] {
				queue = append(queue, parentID)
			}
		}
		rows.Close()
	}

	return false
}

// GetAllDescendantEntries returns all entries from a resource set and all its descendants
func (d *DiskDB) GetAllDescendantEntries(name string) ([]*models.Entry, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource set: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	// Get all descendant set IDs including self
	allSetIDs := []int64{set.ID}

	descendants, err := d.GetResourceSetDescendants(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}

	for _, desc := range descendants {
		allSetIDs = append(allSetIDs, desc.ID)
	}

	// Build query to get all entries from all sets
	// Use a map to deduplicate entries (in case same entry is in multiple sets)
	entryMap := make(map[string]*models.Entry)

	for _, setID := range allSetIDs {
		rows, err := d.db.Query(`
			SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
			FROM entries e
			JOIN resource_set_entries sse ON e.path = sse.entry_path
			WHERE sse.set_id = ?
		`, setID)
		if err != nil {
			return nil, fmt.Errorf("failed to query entries for set %d: %w", setID, err)
		}

		for rows.Next() {
			var entry models.Entry
			var parent sql.NullString

			if err := rows.Scan(&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan entry: %w", err)
			}

			if parent.Valid {
				entry.Parent = &parent.String
			}

			entryMap[entry.Path] = &entry
		}
		rows.Close()
	}

	// Convert map to slice
	entries := make([]*models.Entry, 0, len(entryMap))
	for _, entry := range entryMap {
		entries = append(entries, entry)
	}

	return entries, nil
}

// Helper functions

// scanResourceSets scans rows into ResourceSet slice
func (d *DiskDB) scanResourceSets(rows *sql.Rows) ([]*models.ResourceSet, error) {
	var sets []*models.ResourceSet
	for rows.Next() {
		var set models.ResourceSet
		var description sql.NullString

		if err := rows.Scan(&set.ID, &set.Name, &description, &set.CreatedAt, &set.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan resource set: %w", err)
		}

		if description.Valid {
			set.Description = &description.String
		}

		sets = append(sets, &set)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return sets, nil
}

// getResourceSetsByIDs retrieves resource sets by their IDs
func (d *DiskDB) getResourceSetsByIDs(ids []int64) ([]*models.ResourceSet, error) {
	if len(ids) == 0 {
		return []*models.ResourceSet{}, nil
	}

	// Build placeholder query
	placeholders := ""
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, name, description, created_at, updated_at
		FROM resource_sets
		WHERE id IN (%s)
		ORDER BY name
	`, placeholders)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query resource sets: %w", err)
	}
	defer rows.Close()

	return d.scanResourceSets(rows)
}

// GetResourceSetWithEntryCount returns a resource set with entry count
type ResourceSetWithStats struct {
	*models.ResourceSet
	EntryCount    int   `json:"entry_count"`
	ChildCount    int   `json:"child_count"`
	ParentCount   int   `json:"parent_count"`
	TotalSize     int64 `json:"total_size"`
}

// GetResourceSetStats returns a resource set with statistics
func (d *DiskDB) GetResourceSetStats(name string) (*ResourceSetWithStats, error) {
	set, err := d.GetResourceSet(name)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("resource set '%s' not found", name)
	}

	stats := &ResourceSetWithStats{ResourceSet: set}

	// Get entry count and total size
	err = d.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(e.size), 0)
		FROM resource_set_entries sse
		JOIN entries e ON sse.entry_path = e.path
		WHERE sse.set_id = ?
	`, set.ID).Scan(&stats.EntryCount, &stats.TotalSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get entry stats: %w", err)
	}

	// Get child count
	err = d.db.QueryRow(`
		SELECT COUNT(*) FROM resource_set_edges WHERE parent_id = ?
	`, set.ID).Scan(&stats.ChildCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get child count: %w", err)
	}

	// Get parent count
	err = d.db.QueryRow(`
		SELECT COUNT(*) FROM resource_set_edges WHERE child_id = ?
	`, set.ID).Scan(&stats.ParentCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent count: %w", err)
	}

	return stats, nil
}

// UpdateResourceSet updates a resource set's description
func (d *DiskDB) UpdateResourceSet(name string, description *string) error {
	result, err := d.db.Exec(`
		UPDATE resource_sets
		SET description = ?, updated_at = strftime('%s', 'now')
		WHERE name = ?
	`, description, name)
	if err != nil {
		return fmt.Errorf("failed to update resource set: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("resource set '%s' not found", name)
	}

	return nil
}
