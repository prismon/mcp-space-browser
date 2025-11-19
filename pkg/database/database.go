package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("db")
}

// DiskDB represents the database connection and operations
type DiskDB struct {
	db         *sql.DB
	insertStmt *sql.Stmt
}

// NewDiskDB creates a new database instance
func NewDiskDB(path string) (*DiskDB, error) {
	log.WithField("path", path).Info("Initializing database")

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	diskDB := &DiskDB{db: db}
	if err := diskDB.init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := diskDB.InitJobTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize job tables: %w", err)
	}

	if err := diskDB.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
	}

	return diskDB, nil
}

// Close closes the database connection
func (d *DiskDB) Close() error {
	if d.insertStmt != nil {
		d.insertStmt.Close()
	}
	return d.db.Close()
}

// init creates all necessary tables and indexes
func (d *DiskDB) init() error {
	log.Debug("Creating tables and indexes")

	// Create entries table
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS entries (
		id INTEGER PRIMARY KEY,
		path TEXT UNIQUE NOT NULL,
		parent TEXT,
		size INTEGER,
		kind TEXT CHECK(kind IN ('file', 'directory')),
		ctime INTEGER,
		mtime INTEGER,
		last_scanned INTEGER,
		dirty INTEGER DEFAULT 0
	)`); err != nil {
		return err
	}

	if _, err := d.db.Exec("CREATE INDEX IF NOT EXISTS idx_parent ON entries(parent)"); err != nil {
		return err
	}

	if _, err := d.db.Exec("CREATE INDEX IF NOT EXISTS idx_mtime ON entries(mtime)"); err != nil {
		return err
	}

	// Create selection_sets table
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS selection_sets (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		criteria_type TEXT CHECK(criteria_type IN ('user_selected', 'tool_query')),
		criteria_json TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		return err
	}

	// Create selection_set_entries table
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS selection_set_entries (
		set_id INTEGER NOT NULL,
		entry_path TEXT NOT NULL,
		added_at INTEGER DEFAULT (strftime('%s', 'now')),
		PRIMARY KEY (set_id, entry_path),
		FOREIGN KEY (set_id) REFERENCES selection_sets(id) ON DELETE CASCADE,
		FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
	)`); err != nil {
		return err
	}

	if _, err := d.db.Exec("CREATE INDEX IF NOT EXISTS idx_set_entries ON selection_set_entries(set_id)"); err != nil {
		return err
	}

	// Create queries table
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS queries (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		query_type TEXT CHECK(query_type IN ('file_filter', 'custom_script')),
		query_json TEXT NOT NULL,
		target_selection_set TEXT,
		update_mode TEXT CHECK(update_mode IN ('replace', 'append', 'merge')) DEFAULT 'replace',
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now')),
		last_executed INTEGER,
		execution_count INTEGER DEFAULT 0
	)`); err != nil {
		return err
	}

	// Create query_executions table
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS query_executions (
		id INTEGER PRIMARY KEY,
		query_id INTEGER NOT NULL,
		executed_at INTEGER DEFAULT (strftime('%s', 'now')),
		duration_ms INTEGER,
		files_matched INTEGER,
		status TEXT CHECK(status IN ('success', 'error')),
		error_message TEXT,
		FOREIGN KEY (query_id) REFERENCES queries(id) ON DELETE CASCADE
	)`); err != nil {
		return err
	}

	if _, err := d.db.Exec("CREATE INDEX IF NOT EXISTS idx_query_executions ON query_executions(query_id, executed_at DESC)"); err != nil {
		return err
	}

	log.Debug("Database initialization complete")
	return nil
}

// prepareStatements prepares commonly used SQL statements
func (d *DiskDB) prepareStatements() error {
	var err error
	d.insertStmt, err = d.db.Prepare(`
		INSERT INTO entries
			(path, parent, size, kind, ctime, mtime, last_scanned, dirty)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(path) DO UPDATE SET
			parent=excluded.parent,
			size=excluded.size,
			kind=excluded.kind,
			ctime=excluded.ctime,
			mtime=excluded.mtime,
			last_scanned=excluded.last_scanned,
			dirty=0
	`)
	return err
}

// Entry Operations

// InsertOrUpdate inserts or updates an entry in the database
func (d *DiskDB) InsertOrUpdate(entry *models.Entry) error {
	if logger.IsLevelEnabled(logrus.TraceLevel) {
		log.WithFields(logrus.Fields{
			"path": entry.Path,
			"kind": entry.Kind,
			"size": entry.Size,
		}).Trace("Inserting/updating entry")
	}

	_, err := d.insertStmt.Exec(
		entry.Path,
		entry.Parent,
		entry.Size,
		entry.Kind,
		entry.Ctime,
		entry.Mtime,
		entry.LastScanned,
	)
	return err
}

// Get retrieves an entry by path
func (d *DiskDB) Get(path string) (*models.Entry, error) {
	if logger.IsLevelEnabled(logrus.TraceLevel) {
		log.WithField("path", path).Trace("Fetching entry")
	}

	var entry models.Entry
	var parent sql.NullString

	err := d.db.QueryRow(`SELECT id, path, parent, size, kind, ctime, mtime, last_scanned FROM entries WHERE path = ?`, path).
		Scan(&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if parent.Valid {
		entry.Parent = &parent.String
	}

	if logger.IsLevelEnabled(logrus.TraceLevel) {
		log.WithFields(logrus.Fields{
			"path":  path,
			"found": true,
		}).Trace("Entry fetch complete")
	}

	return &entry, nil
}

// Children retrieves all children of a parent path
func (d *DiskDB) Children(parent string) ([]*models.Entry, error) {
	if logger.IsLevelEnabled(logrus.TraceLevel) {
		log.WithField("parent", parent).Trace("Fetching children")
	}

	rows, err := d.db.Query(`SELECT id, path, parent, size, kind, ctime, mtime, last_scanned FROM entries WHERE parent = ?`, parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.Entry
	for rows.Next() {
		var entry models.Entry
		var parentNull sql.NullString

		if err := rows.Scan(&entry.ID, &entry.Path, &parentNull, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned); err != nil {
			return nil, err
		}

		if parentNull.Valid {
			entry.Parent = &parentNull.String
		}

		entries = append(entries, &entry)
	}

	if logger.IsLevelEnabled(logrus.TraceLevel) {
		log.WithFields(logrus.Fields{
			"parent":     parent,
			"childCount": len(entries),
		}).Trace("Children fetched")
	}

	return entries, rows.Err()
}

// DeleteStale removes entries that were not seen in the current scan
func (d *DiskDB) DeleteStale(root string, runID int64) error {
	log.WithFields(logrus.Fields{
		"root":  root,
		"runID": runID,
	}).Debug("Deleting stale entries")

	result, err := d.db.Exec(
		`DELETE FROM entries WHERE (path = ? OR path LIKE ?) AND last_scanned < ?`,
		root,
		root+"/%",
		runID,
	)
	if err != nil {
		return err
	}

	deletedCount, _ := result.RowsAffected()
	log.WithFields(logrus.Fields{
		"root":         root,
		"deletedCount": deletedCount,
	}).Info("Stale entries deleted")

	return nil
}

// ComputeAggregates computes aggregate sizes for directories
func (d *DiskDB) ComputeAggregates(root string) error {
	log.WithField("root", root).Debug("Computing aggregate sizes")

	// Get all directories ordered by depth (deepest first)
	rows, err := d.db.Query(
		`SELECT path FROM entries WHERE kind = 'directory' AND (path = ? OR path LIKE ?) ORDER BY length(path) DESC`,
		root,
		root+"/%",
	)
	if err != nil {
		return err
	}

	var dirs []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		dirs = append(dirs, path)
	}
	rows.Close()

	log.WithField("directoryCount", len(dirs)).Debug("Processing directories for aggregation")

	// Prepare statements
	updateStmt, err := d.db.Prepare(`UPDATE entries SET size = ? WHERE path = ?`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	sumStmt, err := d.db.Prepare(`SELECT COALESCE(SUM(size), 0) as total FROM entries WHERE parent = ?`)
	if err != nil {
		return err
	}
	defer sumStmt.Close()

	// Use transaction for better performance
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	txUpdateStmt := tx.Stmt(updateStmt)
	txSumStmt := tx.Stmt(sumStmt)

	for _, dir := range dirs {
		var total int64
		if err := txSumStmt.QueryRow(dir).Scan(&total); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := txUpdateStmt.Exec(total, dir); err != nil {
			tx.Rollback()
			return err
		}

		if logger.IsLevelEnabled(logrus.TraceLevel) {
			log.WithFields(logrus.Fields{
				"path":          dir,
				"aggregateSize": total,
			}).Trace("Updated directory size")
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"root":                root,
		"directoriesProcessed": len(dirs),
	}).Info("Aggregate computation complete")

	return nil
}

// Transaction Methods

// BeginTransaction starts a new transaction
func (d *DiskDB) BeginTransaction() error {
	_, err := d.db.Exec("BEGIN")
	return err
}

// CommitTransaction commits the current transaction
func (d *DiskDB) CommitTransaction() error {
	_, err := d.db.Exec("COMMIT")
	return err
}

// RollbackTransaction rolls back the current transaction
func (d *DiskDB) RollbackTransaction() error {
	_, err := d.db.Exec("ROLLBACK")
	return err
}

// All retrieves all entries
func (d *DiskDB) All() ([]*models.Entry, error) {
	rows, err := d.db.Query(`SELECT id, path, parent, size, kind, ctime, mtime, last_scanned FROM entries`)
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

// TreeOptions configures tree building behavior
type TreeOptions struct {
	MaxDepth       int        // Maximum depth to traverse (0 = unlimited)
	CurrentDepth   int        // Current depth in recursion
	MinSize        int64      // Minimum file size to include
	Limit          *int       // Total node limit
	SortBy         string     // Sort by: "size", "name", or "mtime"
	DescendingSort bool       // Sort in descending order
	MinDate        *time.Time // Filter files modified after this date
	MaxDate        *time.Time // Filter files modified before this date
	ChildThreshold int        // When to summarize (default: 100)
	NodesReturned  *int       // Track total nodes returned
}

// GetTree builds a tree structure starting from a root path
// Deprecated: Use GetTreeWithOptions for better control over large directories
func (d *DiskDB) GetTree(root string) (*models.TreeNode, error) {
	opts := TreeOptions{
		MaxDepth:       0, // Unlimited
		CurrentDepth:   0,
		MinSize:        0,
		SortBy:         "size",
		DescendingSort: true,
		ChildThreshold: 1000, // High default to maintain backward compatibility
		NodesReturned:  new(int),
	}
	return d.GetTreeWithOptions(context.Background(), root, opts)
}

// GetTreeWithOptions builds a tree structure with configurable options
func (d *DiskDB) GetTreeWithOptions(ctx context.Context, root string, opts TreeOptions) (*models.TreeNode, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check depth limit
	if opts.MaxDepth > 0 && opts.CurrentDepth >= opts.MaxDepth {
		return d.createSummaryNode(root)
	}

	// Check total node limit
	if opts.Limit != nil && opts.NodesReturned != nil {
		if *opts.NodesReturned >= *opts.Limit {
			return d.createSummaryNode(root)
		}
	}

	entry, err := d.Get(root)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("path not found: %s", root)
	}

	node := &models.TreeNode{
		Name:     filepath.Base(entry.Path),
		Path:     entry.Path,
		Size:     entry.Size,
		Kind:     entry.Kind,
		Mtime:    time.Unix(entry.Mtime, 0),
		Children: []*models.TreeNode{},
	}

	if entry.Kind == "directory" {
		children, err := d.Children(entry.Path)
		if err != nil {
			return nil, err
		}

		// Filter children by size and date
		children = d.filterChildren(children, opts)

		// Check if we should summarize this directory
		if len(children) > opts.ChildThreshold {
			node.Summary = d.createDirectorySummary(children, opts.ChildThreshold)
			node.Truncated = true
			// Keep only top N largest children for detailed view
			children = d.getLargestN(children, 10)
		}

		// Sort children
		d.sortChildren(children, opts.SortBy, opts.DescendingSort)

		// Recurse with incremented depth
		for _, child := range children {
			// Check context and limits before each child
			select {
			case <-ctx.Done():
				return node, nil // Return partial tree
			default:
			}

			if opts.Limit != nil && opts.NodesReturned != nil {
				if *opts.NodesReturned >= *opts.Limit {
					break
				}
			}

			childOpts := opts
			childOpts.CurrentDepth = opts.CurrentDepth + 1
			childNode, err := d.GetTreeWithOptions(ctx, child.Path, childOpts)
			if err != nil {
				// Skip errors instead of failing entire tree
				log.WithError(err).WithField("path", child.Path).Warn("Failed to get child node")
				continue
			}
			node.Children = append(node.Children, childNode)
			if opts.NodesReturned != nil {
				*opts.NodesReturned++
			}
		}
	}

	return node, nil
}

// createSummaryNode creates a minimal node with just metadata (no children)
func (d *DiskDB) createSummaryNode(root string) (*models.TreeNode, error) {
	entry, err := d.Get(root)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("path not found: %s", root)
	}

	node := &models.TreeNode{
		Name:      filepath.Base(entry.Path),
		Path:      entry.Path,
		Size:      entry.Size,
		Kind:      entry.Kind,
		Mtime:     time.Unix(entry.Mtime, 0),
		Children:  []*models.TreeNode{},
		Truncated: true,
	}

	// If it's a directory, add summary
	if entry.Kind == "directory" {
		children, err := d.Children(entry.Path)
		if err == nil && len(children) > 0 {
			node.Summary = d.createDirectorySummary(children, 10)
		}
	}

	return node, nil
}

// filterChildren filters entries based on size and date criteria
func (d *DiskDB) filterChildren(children []*models.Entry, opts TreeOptions) []*models.Entry {
	if opts.MinSize == 0 && opts.MinDate == nil && opts.MaxDate == nil {
		return children // No filtering needed
	}

	filtered := make([]*models.Entry, 0, len(children))
	for _, child := range children {
		// Skip files smaller than minimum
		if opts.MinSize > 0 && child.Size < opts.MinSize {
			continue
		}

		// Skip files outside date range
		childMtime := time.Unix(child.Mtime, 0)
		if opts.MinDate != nil && childMtime.Before(*opts.MinDate) {
			continue
		}
		if opts.MaxDate != nil && childMtime.After(*opts.MaxDate) {
			continue
		}

		filtered = append(filtered, child)
	}

	return filtered
}

// createDirectorySummary creates aggregate statistics for a directory
func (d *DiskDB) createDirectorySummary(children []*models.Entry, topN int) *models.TreeSummary {
	summary := &models.TreeSummary{
		TotalChildren:  len(children),
		FileCount:      0,
		DirectoryCount: 0,
		TotalSize:      0,
	}

	for _, child := range children {
		summary.TotalSize += child.Size
		if child.Kind == "file" {
			summary.FileCount++
		} else {
			summary.DirectoryCount++
		}
	}

	// Get top N largest children
	largest := d.getLargestN(children, topN)
	summary.LargestChildren = make([]*models.SimplifiedNode, len(largest))
	for i, entry := range largest {
		summary.LargestChildren[i] = &models.SimplifiedNode{
			Name:  filepath.Base(entry.Path),
			Path:  entry.Path,
			Size:  entry.Size,
			Kind:  entry.Kind,
			Mtime: time.Unix(entry.Mtime, 0),
		}
	}

	return summary
}

// getLargestN returns the N largest entries by size
func (d *DiskDB) getLargestN(entries []*models.Entry, n int) []*models.Entry {
	if len(entries) <= n {
		return entries
	}

	// Sort by size descending
	sorted := make([]*models.Entry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Size > sorted[j].Size
	})

	return sorted[:n]
}

// sortChildren sorts entries based on the specified criteria
func (d *DiskDB) sortChildren(entries []*models.Entry, sortBy string, descending bool) {
	sort.Slice(entries, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "name":
			less = filepath.Base(entries[i].Path) < filepath.Base(entries[j].Path)
		case "mtime":
			less = entries[i].Mtime < entries[j].Mtime
		default: // "size"
			less = entries[i].Size < entries[j].Size
		}
		if descending {
			return !less
		}
		return less
	})
}

// GetDiskUsageSummary computes a disk usage summary for a path
func (d *DiskDB) GetDiskUsageSummary(root string) (*models.DiskUsageSummary, error) {
	var totalSize int64
	var fileCount, directoryCount int

	// Get total size and counts
	err := d.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN kind = 'file' THEN size ELSE 0 END), 0) as total_size,
			COUNT(CASE WHEN kind = 'file' THEN 1 END) as file_count,
			COUNT(CASE WHEN kind = 'directory' THEN 1 END) as directory_count
		FROM entries
		WHERE path = ? OR path LIKE ?
	`, root, root+"/%").Scan(&totalSize, &fileCount, &directoryCount)

	if err != nil {
		return nil, err
	}

	summary := &models.DiskUsageSummary{
		Path:           root,
		TotalSize:      totalSize,
		FileCount:      fileCount,
		DirectoryCount: directoryCount,
	}

	// Get largest file
	var largestFile sql.NullString
	var largestFileSize sql.NullInt64
	err = d.db.QueryRow(`
		SELECT path, size FROM entries
		WHERE kind = 'file' AND (path = ? OR path LIKE ?)
		ORDER BY size DESC LIMIT 1
	`, root, root+"/%").Scan(&largestFile, &largestFileSize)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if largestFile.Valid {
		summary.LargestFile = largestFile.String
		summary.LargestFileSize = largestFileSize.Int64
	}

	// Get oldest and newest files
	var oldestFile sql.NullString
	var oldestFileTime sql.NullInt64
	err = d.db.QueryRow(`
		SELECT path, mtime FROM entries
		WHERE kind = 'file' AND (path = ? OR path LIKE ?)
		ORDER BY mtime ASC LIMIT 1
	`, root, root+"/%").Scan(&oldestFile, &oldestFileTime)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if oldestFile.Valid {
		summary.OldestFile = oldestFile.String
		summary.OldestFileTime = oldestFileTime.Int64
	}

	var newestFile sql.NullString
	var newestFileTime sql.NullInt64
	err = d.db.QueryRow(`
		SELECT path, mtime FROM entries
		WHERE kind = 'file' AND (path = ? OR path LIKE ?)
		ORDER BY mtime DESC LIMIT 1
	`, root, root+"/%").Scan(&newestFile, &newestFileTime)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if newestFile.Valid {
		summary.NewestFile = newestFile.String
		summary.NewestFileTime = newestFileTime.Int64
	}

	return summary, nil
}

// ExecuteFileFilter executes a file filter and returns matching entries
func (d *DiskDB) ExecuteFileFilter(filter *models.FileFilter) ([]*models.Entry, error) {
	query := "SELECT id, path, parent, size, kind, ctime, mtime, last_scanned FROM entries WHERE 1=1"
	args := []interface{}{}

	// Build the WHERE clause
	if filter.Path != nil {
		query += " AND (path = ? OR path LIKE ?)"
		args = append(args, *filter.Path, *filter.Path+"/%")
	}

	if filter.Extensions != nil && len(filter.Extensions) > 0 {
		placeholders := make([]string, len(filter.Extensions))
		for i, ext := range filter.Extensions {
			placeholders[i] = "?"
			args = append(args, "%."+ext)
		}
		query += " AND path LIKE ANY(" + strings.Join(placeholders, ",") + ")"
	}

	if filter.MinSize != nil {
		query += " AND size >= ?"
		args = append(args, *filter.MinSize)
	}

	if filter.MaxSize != nil {
		query += " AND size <= ?"
		args = append(args, *filter.MaxSize)
	}

	if filter.MinDate != nil {
		t, err := time.Parse("2006-01-02", *filter.MinDate)
		if err == nil {
			query += " AND mtime >= ?"
			args = append(args, t.Unix())
		}
	}

	if filter.MaxDate != nil {
		t, err := time.Parse("2006-01-02", *filter.MaxDate)
		if err == nil {
			query += " AND mtime <= ?"
			args = append(args, t.Unix())
		}
	}

	if filter.NameContains != nil {
		query += " AND path LIKE ?"
		args = append(args, "%"+*filter.NameContains+"%")
	}

	if filter.PathContains != nil {
		query += " AND path LIKE ?"
		args = append(args, "%"+*filter.PathContains+"%")
	}

	if filter.Pattern != nil {
		// Note: SQLite doesn't have native regex, we'll filter in Go
	}

	// Add sorting
	sortBy := "mtime"
	if filter.SortBy != nil {
		sortBy = *filter.SortBy
	}

	descending := true
	if filter.DescendingSort != nil {
		descending = *filter.DescendingSort
	}

	sortOrder := "DESC"
	if !descending {
		sortOrder = "ASC"
	}

	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Add limit
	if filter.Limit != nil {
		query += " LIMIT ?"
		args = append(args, *filter.Limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.Entry
	var pattern *regexp.Regexp
	if filter.Pattern != nil {
		pattern, err = regexp.Compile(*filter.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	for rows.Next() {
		var entry models.Entry
		var parent sql.NullString

		if err := rows.Scan(&entry.ID, &entry.Path, &parent, &entry.Size, &entry.Kind, &entry.Ctime, &entry.Mtime, &entry.LastScanned); err != nil {
			return nil, err
		}

		if parent.Valid {
			entry.Parent = &parent.String
		}

		// Apply regex pattern if specified
		if pattern != nil && !pattern.MatchString(entry.Path) {
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// GetEntriesByTimeRange retrieves entries modified within a time range
func (d *DiskDB) GetEntriesByTimeRange(startDate, endDate string, root *string) ([]*models.Entry, error) {
	query := "SELECT id, path, parent, size, kind, ctime, mtime, last_scanned FROM entries WHERE 1=1"
	args := []interface{}{}

	if root != nil {
		query += " AND (path = ? OR path LIKE ?)"
		args = append(args, *root, *root+"/%")
	}

	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %w", err)
	}

	endTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %w", err)
	}

	query += " AND mtime >= ? AND mtime <= ?"
	args = append(args, startTime.Unix(), endTime.Unix())

	query += " ORDER BY mtime DESC"

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

// Selection Set Operations

// CreateSelectionSet creates a new selection set
func (d *DiskDB) CreateSelectionSet(set *models.SelectionSet) (int64, error) {
	log.WithFields(logrus.Fields{
		"name": set.Name,
		"type": set.CriteriaType,
	}).Info("Creating selection set")

	result, err := d.db.Exec(`
		INSERT INTO selection_sets (name, description, criteria_type, criteria_json)
		VALUES (?, ?, ?, ?)
	`, set.Name, set.Description, set.CriteriaType, set.CriteriaJSON)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetSelectionSet retrieves a selection set by name
func (d *DiskDB) GetSelectionSet(name string) (*models.SelectionSet, error) {
	var set models.SelectionSet
	var description, criteriaJSON sql.NullString

	err := d.db.QueryRow(`SELECT id, name, description, criteria_type, criteria_json, created_at, updated_at FROM selection_sets WHERE name = ?`, name).
		Scan(&set.ID, &set.Name, &description, &set.CriteriaType, &criteriaJSON, &set.CreatedAt, &set.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		set.Description = &description.String
	}
	if criteriaJSON.Valid {
		set.CriteriaJSON = &criteriaJSON.String
	}

	return &set, nil
}

// ListSelectionSets retrieves all selection sets
func (d *DiskDB) ListSelectionSets() ([]*models.SelectionSet, error) {
	rows, err := d.db.Query(`SELECT id, name, description, criteria_type, criteria_json, created_at, updated_at FROM selection_sets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []*models.SelectionSet
	for rows.Next() {
		var set models.SelectionSet
		var description, criteriaJSON sql.NullString

		if err := rows.Scan(&set.ID, &set.Name, &description, &set.CriteriaType, &criteriaJSON, &set.CreatedAt, &set.UpdatedAt); err != nil {
			return nil, err
		}

		if description.Valid {
			set.Description = &description.String
		}
		if criteriaJSON.Valid {
			set.CriteriaJSON = &criteriaJSON.String
		}

		sets = append(sets, &set)
	}

	return sets, rows.Err()
}

// DeleteSelectionSet deletes a selection set
func (d *DiskDB) DeleteSelectionSet(name string) error {
	log.WithField("name", name).Info("Deleting selection set")
	_, err := d.db.Exec(`DELETE FROM selection_sets WHERE name = ?`, name)
	return err
}

// AddToSelectionSet adds entries to a selection set
func (d *DiskDB) AddToSelectionSet(setName string, paths []string) error {
	set, err := d.GetSelectionSet(setName)
	if err != nil {
		return err
	}
	if set == nil {
		return fmt.Errorf("selection set '%s' not found", setName)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO selection_set_entries (set_id, entry_path) VALUES (?, ?)`)
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
	if _, err := tx.Exec(`UPDATE selection_sets SET updated_at = strftime('%s', 'now') WHERE id = ?`, set.ID); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"setName": setName,
		"count":   len(paths),
	}).Info("Added entries to selection set")

	return nil
}

// RemoveFromSelectionSet removes entries from a selection set
func (d *DiskDB) RemoveFromSelectionSet(setName string, paths []string) error {
	set, err := d.GetSelectionSet(setName)
	if err != nil {
		return err
	}
	if set == nil {
		return fmt.Errorf("selection set '%s' not found", setName)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`DELETE FROM selection_set_entries WHERE set_id = ? AND entry_path = ?`)
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
	if _, err := tx.Exec(`UPDATE selection_sets SET updated_at = strftime('%s', 'now') WHERE id = ?`, set.ID); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"setName": setName,
		"count":   len(paths),
	}).Info("Removed entries from selection set")

	return nil
}

// GetSelectionSetEntries retrieves all entries in a selection set
func (d *DiskDB) GetSelectionSetEntries(setName string) ([]*models.Entry, error) {
	set, err := d.GetSelectionSet(setName)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, fmt.Errorf("selection set '%s' not found", setName)
	}

	rows, err := d.db.Query(`
		SELECT e.id, e.path, e.parent, e.size, e.kind, e.ctime, e.mtime, e.last_scanned
		FROM entries e
		JOIN selection_set_entries sse ON e.path = sse.entry_path
		WHERE sse.set_id = ?
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

// Query Operations

// CreateQuery creates a new query
func (d *DiskDB) CreateQuery(query *models.Query) (int64, error) {
	log.WithField("name", query.Name).Info("Creating query")

	result, err := d.db.Exec(`
		INSERT INTO queries (name, description, query_type, query_json, target_selection_set, update_mode)
		VALUES (?, ?, ?, ?, ?, ?)
	`, query.Name, query.Description, query.QueryType, query.QueryJSON, query.TargetSelectionSet, query.UpdateMode)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetQuery retrieves a query by name
func (d *DiskDB) GetQuery(name string) (*models.Query, error) {
	var query models.Query
	var description, targetSet, updateMode sql.NullString
	var lastExecuted sql.NullInt64

	err := d.db.QueryRow(`
		SELECT id, name, description, query_type, query_json, target_selection_set, update_mode,
		       created_at, updated_at, last_executed, execution_count
		FROM queries WHERE name = ?
	`, name).Scan(
		&query.ID, &query.Name, &description, &query.QueryType, &query.QueryJSON,
		&targetSet, &updateMode, &query.CreatedAt, &query.UpdatedAt,
		&lastExecuted, &query.ExecutionCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		query.Description = &description.String
	}
	if targetSet.Valid {
		query.TargetSelectionSet = &targetSet.String
	}
	if updateMode.Valid {
		query.UpdateMode = &updateMode.String
	}
	if lastExecuted.Valid {
		query.LastExecuted = &lastExecuted.Int64
	}

	return &query, nil
}

// ListQueries retrieves all queries
func (d *DiskDB) ListQueries() ([]*models.Query, error) {
	rows, err := d.db.Query(`
		SELECT id, name, description, query_type, query_json, target_selection_set, update_mode,
		       created_at, updated_at, last_executed, execution_count
		FROM queries ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []*models.Query
	for rows.Next() {
		var query models.Query
		var description, targetSet, updateMode sql.NullString
		var lastExecuted sql.NullInt64

		if err := rows.Scan(
			&query.ID, &query.Name, &description, &query.QueryType, &query.QueryJSON,
			&targetSet, &updateMode, &query.CreatedAt, &query.UpdatedAt,
			&lastExecuted, &query.ExecutionCount,
		); err != nil {
			return nil, err
		}

		if description.Valid {
			query.Description = &description.String
		}
		if targetSet.Valid {
			query.TargetSelectionSet = &targetSet.String
		}
		if updateMode.Valid {
			query.UpdateMode = &updateMode.String
		}
		if lastExecuted.Valid {
			query.LastExecuted = &lastExecuted.Int64
		}

		queries = append(queries, &query)
	}

	return queries, rows.Err()
}

// ExecuteQuery executes a saved query
func (d *DiskDB) ExecuteQuery(queryName string) ([]*models.Entry, error) {
	query, err := d.GetQuery(queryName)
	if err != nil {
		return nil, err
	}
	if query == nil {
		return nil, fmt.Errorf("query '%s' not found", queryName)
	}

	startTime := time.Now()

	var filter models.FileFilter
	if err := json.Unmarshal([]byte(query.QueryJSON), &filter); err != nil {
		return nil, fmt.Errorf("invalid query JSON: %w", err)
	}

	entries, err := d.ExecuteFileFilter(&filter)
	duration := time.Since(startTime).Milliseconds()

	// Record execution
	status := "success"
	var errorMsg *string
	filesMatched := len(entries)

	if err != nil {
		status = "error"
		msg := err.Error()
		errorMsg = &msg
		filesMatched = 0
	}

	d.RecordQueryExecution(&models.QueryExecution{
		QueryID:      query.ID,
		ExecutedAt:   time.Now().Unix(),
		DurationMs:   intPtr(int(duration)),
		FilesMatched: intPtr(filesMatched),
		Status:       status,
		ErrorMessage: errorMsg,
	})

	// Update query stats
	d.db.Exec(`
		UPDATE queries
		SET last_executed = strftime('%s', 'now'), execution_count = execution_count + 1
		WHERE id = ?
	`, query.ID)

	return entries, err
}

// RecordQueryExecution records a query execution
func (d *DiskDB) RecordQueryExecution(exec *models.QueryExecution) error {
	_, err := d.db.Exec(`
		INSERT INTO query_executions (query_id, executed_at, duration_ms, files_matched, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?)
	`, exec.QueryID, exec.ExecutedAt, exec.DurationMs, exec.FilesMatched, exec.Status, exec.ErrorMessage)
	return err
}

// GetQueryExecutions retrieves executions for a query
func (d *DiskDB) GetQueryExecutions(queryID int64, limit int) ([]*models.QueryExecution, error) {
	query := `
		SELECT id, query_id, executed_at, duration_ms, files_matched, status, error_message
		FROM query_executions
		WHERE query_id = ?
		ORDER BY executed_at DESC
	`

	args := []interface{}{queryID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*models.QueryExecution
	for rows.Next() {
		var exec models.QueryExecution
		var durationMs, filesMatched sql.NullInt64
		var errorMsg sql.NullString

		if err := rows.Scan(
			&exec.ID, &exec.QueryID, &exec.ExecutedAt,
			&durationMs, &filesMatched, &exec.Status, &errorMsg,
		); err != nil {
			return nil, err
		}

		if durationMs.Valid {
			dm := int(durationMs.Int64)
			exec.DurationMs = &dm
		}
		if filesMatched.Valid {
			fm := int(filesMatched.Int64)
			exec.FilesMatched = &fm
		}
		if errorMsg.Valid {
			exec.ErrorMessage = &errorMsg.String
		}

		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// DeleteQuery deletes a query
func (d *DiskDB) DeleteQuery(name string) error {
	log.WithField("name", name).Info("Deleting query")
	_, err := d.db.Exec(`DELETE FROM queries WHERE name = ?`, name)
	return err
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
