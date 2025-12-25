package database

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProgressTrackerTest(t *testing.T) (*sql.DB, *WriteQueue, func()) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create the index_jobs table
	_, err = db.Exec(`
		CREATE TABLE index_jobs (
			id INTEGER PRIMARY KEY,
			root_path TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			progress INTEGER DEFAULT 0,
			metadata TEXT,
			created_at INTEGER,
			updated_at INTEGER
		)
	`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()

	cleanup := func() {
		wq.Stop()
		db.Close()
	}

	return db, wq, cleanup
}

func createTestJob(t *testing.T, db *sql.DB) int64 {
	result, err := db.Exec(`
		INSERT INTO index_jobs (root_path, status, progress, created_at, updated_at)
		VALUES (?, 'running', 0, ?, ?)
	`, "/test/path", time.Now().Unix(), time.Now().Unix())
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestProgressTracker_Basic(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Initial state
	assert.Equal(t, int64(jobID), pt.JobID())
	assert.Equal(t, 0, pt.GetProgress())
	assert.Nil(t, pt.GetMetadata())
	assert.False(t, pt.IsDirty())
}

func TestProgressTracker_Update(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Update progress
	metadata := &IndexJobMetadata{
		FilesProcessed:       10,
		DirectoriesProcessed: 5,
		TotalSize:            1024,
		ErrorCount:           0,
	}
	pt.Update(50, metadata)

	// Check in-memory state
	assert.Equal(t, 50, pt.GetProgress())
	assert.True(t, pt.IsDirty())

	storedMetadata := pt.GetMetadata()
	require.NotNil(t, storedMetadata)
	assert.Equal(t, 10, storedMetadata.FilesProcessed)
	assert.Equal(t, 5, storedMetadata.DirectoriesProcessed)
	assert.Equal(t, int64(1024), storedMetadata.TotalSize)
}

func TestProgressTracker_Flush(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Update and flush
	metadata := &IndexJobMetadata{
		FilesProcessed:       100,
		DirectoriesProcessed: 20,
		TotalSize:            10240,
		ErrorCount:           2,
	}
	pt.Update(75, metadata)

	err := pt.Flush()
	require.NoError(t, err)
	assert.False(t, pt.IsDirty())

	// Verify in database
	var progress int
	var metadataJSON sql.NullString
	err = db.QueryRow(`SELECT progress, metadata FROM index_jobs WHERE id = ?`, jobID).Scan(&progress, &metadataJSON)
	require.NoError(t, err)
	assert.Equal(t, 75, progress)
	assert.True(t, metadataJSON.Valid)
	assert.Contains(t, metadataJSON.String, `"filesProcessed":100`)
}

func TestProgressTracker_FlushSync(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Update and flush synchronously
	pt.Update(100, &IndexJobMetadata{
		FilesProcessed:       200,
		DirectoriesProcessed: 50,
		TotalSize:            20480,
		ErrorCount:           0,
	})

	err := pt.FlushSync(5 * time.Second)
	require.NoError(t, err)

	// Verify in database
	var progress int
	err = db.QueryRow(`SELECT progress FROM index_jobs WHERE id = ?`, jobID).Scan(&progress)
	require.NoError(t, err)
	assert.Equal(t, 100, progress)
}

func TestProgressTracker_FlushNoOp(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Flush without any updates should be a no-op
	err := pt.Flush()
	require.NoError(t, err)
	assert.False(t, pt.IsDirty())
}

func TestProgressTracker_AutoFlushByInterval(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)

	// Configure with a very short flush interval
	config := &ProgressTrackerConfig{
		FlushInterval: 50 * time.Millisecond,
	}
	pt := NewProgressTracker(jobID, wq, config)

	// Update progress
	pt.Update(25, nil)

	// Wait for interval to pass
	time.Sleep(100 * time.Millisecond)

	// Another update should trigger a flush
	pt.Update(50, nil)

	// Give time for async flush to complete
	time.Sleep(50 * time.Millisecond)

	// Verify flush occurred
	assert.False(t, pt.IsDirty())
}

func TestProgressTracker_SetFlushInterval(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Change flush interval
	pt.SetFlushInterval(100 * time.Millisecond)

	// Update should work
	pt.Update(10, nil)
	assert.Equal(t, 10, pt.GetProgress())
}

func TestProgressTracker_MetadataCopy(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Update with metadata
	original := &IndexJobMetadata{
		FilesProcessed:       10,
		DirectoriesProcessed: 5,
	}
	pt.Update(50, original)

	// Get metadata and modify it
	retrieved := pt.GetMetadata()
	require.NotNil(t, retrieved)
	retrieved.FilesProcessed = 999

	// Original stored metadata should be unchanged
	storedAgain := pt.GetMetadata()
	assert.Equal(t, 10, storedAgain.FilesProcessed)
}

func TestProgressTracker_MultipleUpdates(t *testing.T) {
	db, wq, cleanup := setupProgressTrackerTest(t)
	defer cleanup()

	jobID := createTestJob(t, db)
	pt := NewProgressTracker(jobID, wq, nil)

	// Multiple updates before flush
	pt.Update(10, &IndexJobMetadata{FilesProcessed: 10})
	pt.Update(20, &IndexJobMetadata{FilesProcessed: 20})
	pt.Update(30, &IndexJobMetadata{FilesProcessed: 30})

	// Only latest should be stored
	assert.Equal(t, 30, pt.GetProgress())
	assert.Equal(t, 30, pt.GetMetadata().FilesProcessed)

	// Flush and verify
	err := pt.FlushSync(5 * time.Second)
	require.NoError(t, err)

	var progress int
	err = db.QueryRow(`SELECT progress FROM index_jobs WHERE id = ?`, jobID).Scan(&progress)
	require.NoError(t, err)
	assert.Equal(t, 30, progress)
}

func TestDefaultProgressTrackerConfig(t *testing.T) {
	config := DefaultProgressTrackerConfig()
	assert.Equal(t, 10*time.Second, config.FlushInterval)
}
