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

// ClassifierJob represents a classifier job for processing media files
type ClassifierJob struct {
	ID            int64
	ResourceURL   string
	LocalPath     string // Path to the file being processed
	ArtifactTypes string // JSON array of artifact types to generate
	Status        string // pending, running, completed, failed, cancelled
	Progress      int    // percentage 0-100
	StartedAt     *int64
	CompletedAt   *int64
	Error         *string
	Result        *string // JSON result
	CreatedAt     int64
	UpdatedAt     int64
}

// ClassifierJobResult contains the result of a classifier job
type ClassifierJobResult struct {
	Artifacts []ClassifierArtifact `json:"artifacts"`
	Errors    []string             `json:"errors,omitempty"`
}

// ClassifierArtifact represents a generated artifact
type ClassifierArtifact struct {
	Type        string         `json:"type"`
	Hash        string         `json:"hash"`
	MimeType    string         `json:"mimeType"`
	CachePath   string         `json:"cachePath"`
	ResourceURI string         `json:"resourceUri"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// InitClassifierJobTables creates the classifier_jobs table
func (d *DiskDB) InitClassifierJobTables() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS classifier_jobs (
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
		)
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec("CREATE INDEX IF NOT EXISTS idx_classifier_job_status ON classifier_jobs(status)")
	return err
}

// CreateClassifierJob creates a new classifier job
func (d *DiskDB) CreateClassifierJob(resourceURL, localPath string, artifactTypes []string) (int64, error) {
	artifactTypesJSON, err := json.Marshal(artifactTypes)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal artifact types: %w", err)
	}

	result, err := d.db.Exec(`
		INSERT INTO classifier_jobs (resource_url, local_path, artifact_types)
		VALUES (?, ?, ?)
	`, resourceURL, localPath, string(artifactTypesJSON))

	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	log.WithFields(map[string]interface{}{
		"jobID":       id,
		"resourceURL": resourceURL,
		"localPath":   localPath,
	}).Info("Created classifier job")

	return id, nil
}

// GetClassifierJob retrieves a classifier job by ID
func (d *DiskDB) GetClassifierJob(id int64) (*ClassifierJob, error) {
	var job ClassifierJob
	var localPath sql.NullString
	var startedAt, completedAt sql.NullInt64
	var errorMsg, result sql.NullString

	err := d.db.QueryRow(`
		SELECT id, resource_url, local_path, artifact_types, status, progress,
		       started_at, completed_at, error, result, created_at, updated_at
		FROM classifier_jobs
		WHERE id = ?
	`, id).Scan(
		&job.ID, &job.ResourceURL, &localPath, &job.ArtifactTypes, &job.Status, &job.Progress,
		&startedAt, &completedAt, &errorMsg, &result,
		&job.CreatedAt, &job.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if localPath.Valid {
		job.LocalPath = localPath.String
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
	if result.Valid {
		job.Result = &result.String
	}

	return &job, nil
}

// UpdateClassifierJobStatus updates the status of a classifier job
func (d *DiskDB) UpdateClassifierJobStatus(id int64, status string, errorMsg *string) error {
	now := time.Now().Unix()

	var completedAt *int64
	if status == "completed" || status == "failed" || status == "cancelled" {
		completedAt = &now
	}

	_, err := d.db.Exec(`
		UPDATE classifier_jobs
		SET status = ?, error = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, status, errorMsg, completedAt, now, id)

	if err != nil {
		return err
	}

	log.WithFields(map[string]interface{}{
		"jobID":  id,
		"status": status,
	}).Debug("Updated classifier job status")

	return nil
}

// UpdateClassifierJobProgress updates the progress of a classifier job
func (d *DiskDB) UpdateClassifierJobProgress(id int64, progress int) error {
	now := time.Now().Unix()

	_, err := d.db.Exec(`
		UPDATE classifier_jobs
		SET progress = ?, updated_at = ?
		WHERE id = ?
	`, progress, now, id)

	return err
}

// UpdateClassifierJobResult updates the result of a classifier job
func (d *DiskDB) UpdateClassifierJobResult(id int64, result *ClassifierJobResult) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	now := time.Now().Unix()
	resultStr := string(resultJSON)

	_, err = d.db.Exec(`
		UPDATE classifier_jobs
		SET result = ?, updated_at = ?
		WHERE id = ?
	`, resultStr, now, id)

	return err
}

// StartClassifierJob marks a classifier job as started
func (d *DiskDB) StartClassifierJob(id int64) error {
	now := time.Now().Unix()

	_, err := d.db.Exec(`
		UPDATE classifier_jobs
		SET status = 'running', started_at = ?, updated_at = ?
		WHERE id = ?
	`, now, now, id)

	if err != nil {
		return err
	}

	log.WithField("jobID", id).Info("Started classifier job")

	return nil
}

// ListClassifierJobs lists all classifier jobs
func (d *DiskDB) ListClassifierJobs(status *string, limit int) ([]*ClassifierJob, error) {
	query := `
		SELECT id, resource_url, local_path, artifact_types, status, progress,
		       started_at, completed_at, error, result, created_at, updated_at
		FROM classifier_jobs
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

	var jobs []*ClassifierJob
	for rows.Next() {
		var job ClassifierJob
		var localPath sql.NullString
		var startedAt, completedAt sql.NullInt64
		var errorMsg, result sql.NullString

		if err := rows.Scan(
			&job.ID, &job.ResourceURL, &localPath, &job.ArtifactTypes, &job.Status, &job.Progress,
			&startedAt, &completedAt, &errorMsg, &result,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if localPath.Valid {
			job.LocalPath = localPath.String
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
		if result.Valid {
			job.Result = &result.String
		}

		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}
