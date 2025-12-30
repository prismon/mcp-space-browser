package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteBackend implements the Backend interface for SQLite databases
type SQLiteBackend struct {
	config     *SQLiteConfig
	path       string  // Absolute path to database file
	db         *sql.DB // Underlying database connection
	writeQueue *WriteQueue
	diskDB     *DiskDB // Cached DiskDB wrapper for domain operations
	mu         sync.RWMutex
	isOpen     bool
}

// NewSQLiteBackend creates a new SQLite backend for a project
func NewSQLiteBackend(projectPath string, config *SQLiteConfig) *SQLiteBackend {
	if config == nil {
		config = DefaultSQLiteConfig()
	}

	dbPath := config.Path
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(projectPath, config.Path)
	}

	return &SQLiteBackend{
		config: config,
		path:   dbPath,
	}
}

// Open opens the database connection
func (s *SQLiteBackend) Open() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isOpen {
		return nil // Already open
	}

	log.WithField("path", s.path).Info("Opening SQLite database")

	db, err := sql.Open("sqlite3", s.path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create and start the write queue
	writeQueue := NewWriteQueue(db, nil)
	writeQueue.Start()

	// Enable WAL mode if configured
	if s.config.WALMode {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			writeQueue.Stop()
			db.Close()
			return fmt.Errorf("failed to enable WAL mode: %w", err)
		}
	}

	// Set busy timeout
	busyTimeout := s.config.BusyTimeoutMs
	if busyTimeout <= 0 {
		busyTimeout = 5000
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeout)); err != nil {
		writeQueue.Stop()
		db.Close()
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	s.db = db
	s.writeQueue = writeQueue
	s.isOpen = true

	return nil
}

// Close closes the database connection
func (s *SQLiteBackend) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isOpen {
		return nil // Already closed
	}

	log.WithField("path", s.path).Info("Closing SQLite database")

	// Close the cached DiskDB's prepared statements (not the connection)
	if s.diskDB != nil {
		if s.diskDB.insertStmt != nil {
			s.diskDB.insertStmt.Close()
		}
		s.diskDB = nil
	}

	// Stop the write queue first
	if s.writeQueue != nil {
		s.writeQueue.Stop()
		s.writeQueue = nil
	}

	err := s.db.Close()
	s.db = nil
	s.isOpen = false

	return err
}

// IsOpen returns true if the database connection is open
func (s *SQLiteBackend) IsOpen() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isOpen
}

// DB returns the underlying sql.DB instance
func (s *SQLiteBackend) DB() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db
}

// WriteQueue returns the write queue for serializing write operations
func (s *SQLiteBackend) WriteQueue() *WriteQueue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.writeQueue
}

// Type returns the backend type identifier
func (s *SQLiteBackend) Type() string {
	return "sqlite3"
}

// ConnectionInfo returns human-readable connection info
func (s *SQLiteBackend) ConnectionInfo() string {
	return fmt.Sprintf("SQLite database at %s", s.path)
}

// Path returns the absolute path to the database file
func (s *SQLiteBackend) Path() string {
	return s.path
}

// DiskDB returns a DiskDB wrapper for domain-level database operations.
// The DiskDB is lazily created and cached for the lifetime of the backend.
// The returned DiskDB shares the underlying connection and should not be closed directly.
func (s *SQLiteBackend) DiskDB() (*DiskDB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isOpen || s.db == nil {
		return nil, fmt.Errorf("database not open")
	}

	// Return cached DiskDB if available
	if s.diskDB != nil {
		return s.diskDB, nil
	}

	// Create new DiskDB wrapper
	diskDB, err := NewDiskDBFromConnection(s.db, s.writeQueue, s.path)
	if err != nil {
		return nil, fmt.Errorf("failed to create DiskDB wrapper: %w", err)
	}

	s.diskDB = diskDB
	return s.diskDB, nil
}

// InitSchema initializes all database tables and indexes
func (s *SQLiteBackend) InitSchema() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isOpen || s.db == nil {
		return fmt.Errorf("database not open")
	}

	log.Debug("Initializing SQLite schema")

	// Create entries table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS entries (
		id INTEGER PRIMARY KEY,
		path TEXT UNIQUE NOT NULL,
		parent TEXT,
		size INTEGER,
		blocks INTEGER DEFAULT 0,
		kind TEXT CHECK(kind IN ('file', 'directory')),
		ctime INTEGER,
		mtime INTEGER,
		last_scanned INTEGER,
		dirty INTEGER DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("failed to create entries table: %w", err)
	}

	// Migration: Add blocks column if it doesn't exist (for existing databases)
	s.db.Exec("ALTER TABLE entries ADD COLUMN blocks INTEGER DEFAULT 0")

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_parent ON entries(parent)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_mtime ON entries(mtime)"); err != nil {
		return err
	}

	// Create resource_sets table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS resource_sets (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		return fmt.Errorf("failed to create resource_sets table: %w", err)
	}

	// Create resource_set_entries table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS resource_set_entries (
		set_id INTEGER NOT NULL,
		entry_path TEXT NOT NULL,
		added_at INTEGER DEFAULT (strftime('%s', 'now')),
		PRIMARY KEY (set_id, entry_path),
		FOREIGN KEY (set_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
		FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create resource_set_entries table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_set_entries ON resource_set_entries(set_id)"); err != nil {
		return err
	}

	// Create resource_set_edges table for DAG structure
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS resource_set_edges (
		parent_id INTEGER NOT NULL,
		child_id INTEGER NOT NULL,
		added_at INTEGER DEFAULT (strftime('%s', 'now')),
		PRIMARY KEY (parent_id, child_id),
		FOREIGN KEY (parent_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
		FOREIGN KEY (child_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
		CHECK (parent_id != child_id)
	)`); err != nil {
		return fmt.Errorf("failed to create resource_set_edges table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_edges_parent ON resource_set_edges(parent_id)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_edges_child ON resource_set_edges(child_id)"); err != nil {
		return err
	}

	// Create queries table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS queries (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		query_type TEXT CHECK(query_type IN ('file_filter', 'custom_script')),
		query_json TEXT NOT NULL,
		target_resource_set TEXT,
		update_mode TEXT CHECK(update_mode IN ('replace', 'append', 'merge')) DEFAULT 'replace',
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now')),
		last_executed INTEGER,
		execution_count INTEGER DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("failed to create queries table: %w", err)
	}

	// Create query_executions table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS query_executions (
		id INTEGER PRIMARY KEY,
		query_id INTEGER NOT NULL,
		executed_at INTEGER DEFAULT (strftime('%s', 'now')),
		duration_ms INTEGER,
		files_matched INTEGER,
		status TEXT CHECK(status IN ('success', 'error')),
		error_message TEXT,
		FOREIGN KEY (query_id) REFERENCES queries(id) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create query_executions table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_query_executions ON query_executions(query_id, executed_at DESC)"); err != nil {
		return err
	}

	// Create metadata table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS metadata (
		id INTEGER PRIMARY KEY,
		hash TEXT UNIQUE NOT NULL,
		source_path TEXT NOT NULL,
		metadata_type TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		cache_path TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		metadata_json TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (source_path) REFERENCES entries(path) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_metadata_source ON metadata(source_path)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_metadata_type ON metadata(metadata_type)"); err != nil {
		return err
	}

	// Create features table - stores generated characteristics of entries
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS features (
		id INTEGER PRIMARY KEY,
		entry_path TEXT NOT NULL,
		feature_type TEXT NOT NULL,
		hash TEXT UNIQUE NOT NULL,
		mime_type TEXT,
		cache_path TEXT,
		data_json TEXT,
		file_size INTEGER DEFAULT 0,
		generator TEXT NOT NULL,
		generator_version TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create features table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_features_entry ON features(entry_path)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_features_type ON features(feature_type)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_features_entry_type ON features(entry_path, feature_type)"); err != nil {
		return err
	}

	// Create sources table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sources (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		type TEXT CHECK(type IN ('manual', 'live', 'scheduled')) NOT NULL,
		root_path TEXT NOT NULL,
		config_json TEXT,
		status TEXT CHECK(status IN ('stopped', 'starting', 'running', 'stopping', 'error')) DEFAULT 'stopped',
		enabled INTEGER DEFAULT 1,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		last_error TEXT
	)`); err != nil {
		return fmt.Errorf("failed to create sources table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_sources_type ON sources(type)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_sources_enabled ON sources(enabled)"); err != nil {
		return err
	}

	// Create rules table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS rules (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		enabled INTEGER DEFAULT 1,
		priority INTEGER DEFAULT 0,
		condition_json TEXT NOT NULL,
		outcome_json TEXT NOT NULL,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		return fmt.Errorf("failed to create rules table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled, priority DESC)"); err != nil {
		return err
	}

	// Create rule_executions table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS rule_executions (
		id INTEGER PRIMARY KEY,
		rule_id INTEGER NOT NULL,
		selection_set_id INTEGER NOT NULL,
		executed_at INTEGER DEFAULT (strftime('%s', 'now')),
		entries_matched INTEGER DEFAULT 0,
		entries_processed INTEGER DEFAULT 0,
		status TEXT CHECK(status IN ('success', 'partial', 'error')),
		error_message TEXT,
		duration_ms INTEGER,
		FOREIGN KEY (rule_id) REFERENCES rules(id) ON DELETE CASCADE,
		FOREIGN KEY (selection_set_id) REFERENCES resource_sets(id) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create rule_executions table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rule_executions_rule ON rule_executions(rule_id, executed_at DESC)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rule_executions_selection_set ON rule_executions(selection_set_id)"); err != nil {
		return err
	}

	// Create rule_outcomes table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS rule_outcomes (
		id INTEGER PRIMARY KEY,
		execution_id INTEGER NOT NULL,
		selection_set_id INTEGER NOT NULL,
		entry_path TEXT NOT NULL,
		outcome_type TEXT NOT NULL,
		outcome_data TEXT,
		status TEXT CHECK(status IN ('success', 'error')),
		error_message TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (execution_id) REFERENCES rule_executions(id) ON DELETE CASCADE,
		FOREIGN KEY (selection_set_id) REFERENCES resource_sets(id) ON DELETE CASCADE,
		FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create rule_outcomes table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rule_outcomes_execution ON rule_outcomes(execution_id)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rule_outcomes_selection_set ON rule_outcomes(selection_set_id)"); err != nil {
		return err
	}

	// Create plans table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS plans (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		mode TEXT CHECK(mode IN ('oneshot', 'continuous')) DEFAULT 'oneshot',
		status TEXT CHECK(status IN ('active', 'paused', 'disabled')) DEFAULT 'active',
		trigger TEXT CHECK(trigger IS NULL OR trigger = '' OR trigger IN ('manual', 'on_add', 'on_remove', 'on_refresh')) DEFAULT 'manual',
		sources_json TEXT NOT NULL,
		conditions_json TEXT,
		outcomes_json TEXT NOT NULL,
		preferences_json TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now')),
		last_run_at INTEGER
	)`); err != nil {
		return fmt.Errorf("failed to create plans table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plans_mode ON plans(mode)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plans_trigger ON plans(trigger)"); err != nil {
		return err
	}

	// Create plan_executions table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS plan_executions (
		id INTEGER PRIMARY KEY,
		plan_id INTEGER NOT NULL,
		plan_name TEXT NOT NULL,
		started_at INTEGER NOT NULL,
		completed_at INTEGER,
		duration_ms INTEGER,
		entries_processed INTEGER DEFAULT 0,
		entries_matched INTEGER DEFAULT 0,
		outcomes_applied INTEGER DEFAULT 0,
		status TEXT CHECK(status IN ('running', 'success', 'partial', 'error')) DEFAULT 'running',
		error_message TEXT,
		FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create plan_executions table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plan_executions_plan ON plan_executions(plan_id)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plan_executions_status ON plan_executions(status)"); err != nil {
		return err
	}

	// Create plan_outcome_records table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS plan_outcome_records (
		id INTEGER PRIMARY KEY,
		execution_id INTEGER NOT NULL,
		plan_id INTEGER NOT NULL,
		entry_path TEXT NOT NULL,
		outcome_type TEXT NOT NULL,
		outcome_data TEXT,
		status TEXT CHECK(status IN ('success', 'error')) DEFAULT 'success',
		error_message TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (execution_id) REFERENCES plan_executions(id) ON DELETE CASCADE,
		FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE,
		FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("failed to create plan_outcome_records table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plan_outcomes_execution ON plan_outcome_records(execution_id)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plan_outcomes_plan ON plan_outcome_records(plan_id)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_plan_outcomes_entry ON plan_outcome_records(entry_path)"); err != nil {
		return err
	}

	// Create index_jobs table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS index_jobs (
		id INTEGER PRIMARY KEY,
		root_path TEXT NOT NULL,
		status TEXT CHECK(status IN ('pending', 'running', 'paused', 'completed', 'failed', 'cancelled')) DEFAULT 'pending',
		progress INTEGER DEFAULT 0,
		started_at INTEGER,
		completed_at INTEGER,
		error TEXT,
		metadata TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		return fmt.Errorf("failed to create index_jobs table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_index_jobs_status ON index_jobs(status)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_index_jobs_root ON index_jobs(root_path)"); err != nil {
		return err
	}

	// Create classifier_jobs table
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS classifier_jobs (
		id INTEGER PRIMARY KEY,
		resource_url TEXT NOT NULL,
		local_path TEXT,
		artifact_types TEXT,
		status TEXT CHECK(status IN ('pending', 'running', 'completed', 'failed', 'cancelled')) DEFAULT 'pending',
		progress INTEGER DEFAULT 0,
		started_at INTEGER,
		completed_at INTEGER,
		error TEXT,
		result TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		return fmt.Errorf("failed to create classifier_jobs table: %w", err)
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_classifier_jobs_status ON classifier_jobs(status)"); err != nil {
		return err
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_classifier_jobs_resource ON classifier_jobs(resource_url)"); err != nil {
		return err
	}

	log.Debug("SQLite schema initialization complete")
	return nil
}
