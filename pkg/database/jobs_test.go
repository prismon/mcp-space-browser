package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIndexJob(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	metadata := &IndexJobMetadata{
		FilesProcessed:       0,
		DirectoriesProcessed: 0,
		TotalSize:            0,
		ErrorCount:           0,
		WorkerCount:          4,
	}

	id, err := db.CreateIndexJob("/test/path", metadata)
	assert.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestGetIndexJob(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test getting non-existent job
	job, err := db.GetIndexJob(999)
	assert.NoError(t, err)
	assert.Nil(t, job)

	// Create and retrieve
	metadata := &IndexJobMetadata{
		FilesProcessed:       100,
		DirectoriesProcessed: 10,
		TotalSize:            50000,
		ErrorCount:           2,
		WorkerCount:          8,
	}

	id, err := db.CreateIndexJob("/my/path", metadata)
	require.NoError(t, err)

	retrieved, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, id, retrieved.ID)
	assert.Equal(t, "/my/path", retrieved.RootPath)
	assert.Equal(t, "pending", retrieved.Status)
	assert.Equal(t, 0, retrieved.Progress)
	assert.Nil(t, retrieved.StartedAt)
	assert.Nil(t, retrieved.CompletedAt)
	assert.NotNil(t, retrieved.Metadata)
}

func TestUpdateIndexJobStatus(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	id, err := db.CreateIndexJob("/test", nil)
	require.NoError(t, err)

	// Update to running
	err = db.UpdateIndexJobStatus(id, "running", nil)
	assert.NoError(t, err)

	job, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.Equal(t, "running", job.Status)
	assert.Nil(t, job.CompletedAt)

	// Update to completed
	err = db.UpdateIndexJobStatus(id, "completed", nil)
	assert.NoError(t, err)

	job, err = db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.Equal(t, "completed", job.Status)
	assert.NotNil(t, job.CompletedAt)
}

func TestUpdateIndexJobStatusWithError(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	id, err := db.CreateIndexJob("/test", nil)
	require.NoError(t, err)

	errorMsg := "something went wrong"
	err = db.UpdateIndexJobStatus(id, "failed", &errorMsg)
	assert.NoError(t, err)

	job, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.Equal(t, "failed", job.Status)
	assert.NotNil(t, job.Error)
	assert.Equal(t, "something went wrong", *job.Error)
	assert.NotNil(t, job.CompletedAt)
}

func TestUpdateIndexJobProgress(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	id, err := db.CreateIndexJob("/test", nil)
	require.NoError(t, err)

	metadata := &IndexJobMetadata{
		FilesProcessed:       500,
		DirectoriesProcessed: 50,
		TotalSize:            1000000,
		ErrorCount:           1,
		WorkerCount:          4,
	}

	err = db.UpdateIndexJobProgress(id, 75, metadata)
	assert.NoError(t, err)

	job, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.Equal(t, 75, job.Progress)
	assert.NotNil(t, job.Metadata)
}

func TestStartIndexJob(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	id, err := db.CreateIndexJob("/test", nil)
	require.NoError(t, err)

	err = db.StartIndexJob(id)
	assert.NoError(t, err)

	job, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.Equal(t, "running", job.Status)
	assert.NotNil(t, job.StartedAt)
}

func TestListIndexJobs(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple jobs
	for i := 0; i < 5; i++ {
		_, err := db.CreateIndexJob("/path"+string(rune('a'+i)), nil)
		require.NoError(t, err)
	}

	// List all jobs
	jobs, err := db.ListIndexJobs(nil, 0)
	assert.NoError(t, err)
	assert.Len(t, jobs, 5)

	// List with limit
	jobs, err = db.ListIndexJobs(nil, 3)
	assert.NoError(t, err)
	assert.Len(t, jobs, 3)
}

func TestListIndexJobsByStatus(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create jobs with different statuses
	id1, _ := db.CreateIndexJob("/path1", nil)
	id2, _ := db.CreateIndexJob("/path2", nil)
	id3, _ := db.CreateIndexJob("/path3", nil)

	db.StartIndexJob(id1)
	db.UpdateIndexJobStatus(id2, "completed", nil)
	// id3 remains pending

	// List pending jobs
	pendingStatus := "pending"
	jobs, err := db.ListIndexJobs(&pendingStatus, 0)
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, id3, jobs[0].ID)

	// List running jobs
	runningStatus := "running"
	jobs, err = db.ListIndexJobs(&runningStatus, 0)
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, id1, jobs[0].ID)

	// List completed jobs
	completedStatus := "completed"
	jobs, err = db.ListIndexJobs(&completedStatus, 0)
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, id2, jobs[0].ID)
}

func TestDeleteIndexJob(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	id, err := db.CreateIndexJob("/to-delete", nil)
	require.NoError(t, err)

	// Verify it exists
	job, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Delete it
	err = db.DeleteIndexJob(id)
	assert.NoError(t, err)

	// Verify it's gone
	job, err = db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.Nil(t, job)
}

func TestIndexJobMetadata(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	metadata := &IndexJobMetadata{
		FilesProcessed:       1000,
		DirectoriesProcessed: 100,
		TotalSize:            5000000,
		ErrorCount:           3,
		WorkerCount:          8,
	}

	id, err := db.CreateIndexJob("/test", metadata)
	require.NoError(t, err)

	job, err := db.GetIndexJob(id)
	assert.NoError(t, err)
	assert.NotNil(t, job.Metadata)

	// The metadata is stored as JSON string, so we just verify it's not empty
	assert.NotEmpty(t, *job.Metadata)
}

func TestIndexJobLifecycle(t *testing.T) {
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create job
	id, err := db.CreateIndexJob("/lifecycle", nil)
	require.NoError(t, err)

	job, err := db.GetIndexJob(id)
	assert.Equal(t, "pending", job.Status)
	assert.Nil(t, job.StartedAt)
	assert.Nil(t, job.CompletedAt)

	// Start job
	err = db.StartIndexJob(id)
	require.NoError(t, err)

	job, err = db.GetIndexJob(id)
	assert.Equal(t, "running", job.Status)
	assert.NotNil(t, job.StartedAt)
	assert.Nil(t, job.CompletedAt)

	// Update progress
	metadata := &IndexJobMetadata{
		FilesProcessed: 50,
	}
	err = db.UpdateIndexJobProgress(id, 50, metadata)
	require.NoError(t, err)

	job, err = db.GetIndexJob(id)
	assert.Equal(t, 50, job.Progress)

	// Complete job
	err = db.UpdateIndexJobStatus(id, "completed", nil)
	require.NoError(t, err)

	job, err = db.GetIndexJob(id)
	assert.Equal(t, "completed", job.Status)
	assert.NotNil(t, job.StartedAt)
	assert.NotNil(t, job.CompletedAt)
	assert.True(t, *job.CompletedAt >= *job.StartedAt)
}
