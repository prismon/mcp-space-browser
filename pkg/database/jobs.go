package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// IndexJob represents a filesystem indexing job
type IndexJob struct {
	ID          int64
	RootPath    string
	Status      string // pending, running, paused, completed, failed, cancelled
	Progress    int    // percentage 0-100
	StartedAt   *int64
	CompletedAt *int64
	Error       *string
	Metadata    *string // JSON metadata
	CreatedAt   int64
	UpdatedAt   int64
}

// IndexJobMetadata contains metadata about an indexing job
type IndexJobMetadata struct {
	FilesProcessed       int   `json:"filesProcessed"`
	DirectoriesProcessed int   `json:"directoriesProcessed"`
	TotalSize            int64 `json:"totalSize"`
	ErrorCount           int   `json:"errorCount"`
	WorkerCount          int   `json:"workerCount"`
}

// InitJobTables creates the jobs table
func (d *DiskDB) InitJobTables() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS index_jobs (
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
		)
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec("CREATE INDEX IF NOT EXISTS idx_job_status ON index_jobs(status)")
	return err
}

// CreateIndexJob creates a new indexing job
func (d *DiskDB) CreateIndexJob(rootPath string, metadata *IndexJobMetadata) (int64, error) {
	var metadataJSON *string
	if metadata != nil {
		bytes, err := json.Marshal(metadata)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		str := string(bytes)
		metadataJSON = &str
	}

	result, err := d.db.Exec(`
		INSERT INTO index_jobs (root_path, metadata)
		VALUES (?, ?)
	`, rootPath, metadataJSON)

	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	log.WithFields(map[string]interface{}{
		"jobID":    id,
		"rootPath": rootPath,
	}).Info("Created indexing job")

	return id, nil
}

// GetIndexJob retrieves an indexing job by ID
func (d *DiskDB) GetIndexJob(id int64) (*IndexJob, error) {
	var job IndexJob
	var startedAt, completedAt sql.NullInt64
	var errorMsg, metadata sql.NullString

	err := d.db.QueryRow(`
		SELECT id, root_path, status, progress, started_at, completed_at, error, metadata, created_at, updated_at
		FROM index_jobs
		WHERE id = ?
	`, id).Scan(
		&job.ID, &job.RootPath, &job.Status, &job.Progress,
		&startedAt, &completedAt, &errorMsg, &metadata,
		&job.CreatedAt, &job.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		job.StartedAt = &startedAt.Int64
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Int64
	}
	if errorMsg.Valid {
		job.Error = &errorMsg.String
	}
	if metadata.Valid {
		job.Metadata = &metadata.String
	}

	return &job, nil
}

// UpdateIndexJobStatus updates the status of an indexing job
func (d *DiskDB) UpdateIndexJobStatus(id int64, status string, errorMsg *string) error {
	now := time.Now().Unix()

	var completedAt *int64
	if status == "completed" || status == "failed" || status == "cancelled" {
		completedAt = &now
	}

	_, err := d.db.Exec(`
		UPDATE index_jobs
		SET status = ?, error = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, status, errorMsg, completedAt, now, id)

	if err != nil {
		return err
	}

	log.WithFields(map[string]interface{}{
		"jobID":  id,
		"status": status,
	}).Debug("Updated job status")

	return nil
}

// UpdateIndexJobProgress updates the progress and metadata of an indexing job
func (d *DiskDB) UpdateIndexJobProgress(id int64, progress int, metadata *IndexJobMetadata) error {
	var metadataJSON *string
	if metadata != nil {
		bytes, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		str := string(bytes)
		metadataJSON = &str
	}

	now := time.Now().Unix()

	_, err := d.db.Exec(`
		UPDATE index_jobs
		SET progress = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, progress, metadataJSON, now, id)

	return err
}

// StartIndexJob marks a job as started
func (d *DiskDB) StartIndexJob(id int64) error {
	now := time.Now().Unix()

	_, err := d.db.Exec(`
		UPDATE index_jobs
		SET status = 'running', started_at = ?, updated_at = ?
		WHERE id = ?
	`, now, now, id)

	if err != nil {
		return err
	}

	log.WithField("jobID", id).Info("Started indexing job")

	return nil
}

// ListIndexJobs lists all indexing jobs
func (d *DiskDB) ListIndexJobs(status *string, limit int) ([]*IndexJob, error) {
	query := `
		SELECT id, root_path, status, progress, started_at, completed_at, error, metadata, created_at, updated_at
		FROM index_jobs
	`

	args := []interface{}{}

	if status != nil {
		query += " WHERE status = ?"
		args = append(args, *status)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*IndexJob
	for rows.Next() {
		var job IndexJob
		var startedAt, completedAt sql.NullInt64
		var errorMsg, metadata sql.NullString

		if err := rows.Scan(
			&job.ID, &job.RootPath, &job.Status, &job.Progress,
			&startedAt, &completedAt, &errorMsg, &metadata,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if startedAt.Valid {
			job.StartedAt = &startedAt.Int64
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Int64
		}
		if errorMsg.Valid {
			job.Error = &errorMsg.String
		}
		if metadata.Valid {
			job.Metadata = &metadata.String
		}

		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}

// DeleteIndexJob deletes an indexing job
func (d *DiskDB) DeleteIndexJob(id int64) error {
	_, err := d.db.Exec("DELETE FROM index_jobs WHERE id = ?", id)
	if err != nil {
		return err
	}

	log.WithField("jobID", id).Info("Deleted indexing job")

	return nil
}
