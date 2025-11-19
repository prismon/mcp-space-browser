package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockJob is a simple job implementation for testing
type MockJob struct {
	id        string
	shouldErr bool
	delay     time.Duration
	executed  atomic.Bool
}

func (m *MockJob) Execute(ctx context.Context) error {
	m.executed.Store(true)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.shouldErr {
		return errors.New("mock job error")
	}
	return nil
}

func (m *MockJob) ID() string {
	return m.id
}

func TestNewWorkerPool(t *testing.T) {
	wp := NewWorkerPool(5, 100)
	assert.NotNil(t, wp)
	assert.Equal(t, 5, wp.workerCount)
	assert.NotNil(t, wp.jobs)
	assert.NotNil(t, wp.results)
}

func TestWorkerPoolStartAndStop(t *testing.T) {
	wp := NewWorkerPool(2, 10)
	wp.Start()

	// Submit a simple job
	job := &MockJob{id: "test-job-1", shouldErr: false}
	err := wp.Submit(job)
	assert.NoError(t, err)

	// Wait a moment for the job to process
	time.Sleep(50 * time.Millisecond)

	wp.Stop()

	// Verify job was executed
	assert.True(t, job.executed.Load())
}

func TestWorkerPoolSubmitAndProcess(t *testing.T) {
	wp := NewWorkerPool(3, 50)
	wp.Start()

	// Submit multiple jobs
	jobs := make([]*MockJob, 10)
	for i := 0; i < 10; i++ {
		jobs[i] = &MockJob{
			id:        string(rune('a' + i)),
			shouldErr: false,
		}
		err := wp.Submit(jobs[i])
		assert.NoError(t, err)
	}

	// Wait for all jobs to complete
	wp.Wait()

	// Verify all jobs were executed
	for i, job := range jobs {
		assert.True(t, job.executed.Load(), "Job %d should be executed", i)
	}

	// Verify stats
	stats := wp.Stats()
	assert.Equal(t, int64(10), stats.JobsQueued)
	assert.Equal(t, int64(10), stats.JobsProcessed)
	assert.Equal(t, int64(0), stats.JobsFailed)
}

func TestWorkerPoolJobFailure(t *testing.T) {
	wp := NewWorkerPool(2, 10)
	wp.Start()

	// Submit a job that will fail
	job := &MockJob{id: "failing-job", shouldErr: true}
	err := wp.Submit(job)
	assert.NoError(t, err)

	// Wait for job to process
	time.Sleep(100 * time.Millisecond)

	wp.Stop()

	// Verify stats show failure
	stats := wp.Stats()
	assert.Equal(t, int64(1), stats.JobsProcessed)
	assert.Equal(t, int64(1), stats.JobsFailed)
}

func TestWorkerPoolPauseResume(t *testing.T) {
	wp := NewWorkerPool(2, 10)
	wp.Start()

	// Pause the pool
	wp.Pause()
	stats := wp.Stats()
	assert.True(t, stats.IsPaused)

	// Submit a job while paused
	job := &MockJob{id: "paused-job", shouldErr: false, delay: 50 * time.Millisecond}
	err := wp.Submit(job)
	assert.NoError(t, err)

	// Wait a bit - job should not execute while paused
	time.Sleep(30 * time.Millisecond)
	assert.False(t, job.executed.Load(), "Job should not execute while paused")

	// Resume the pool
	wp.Resume()
	stats = wp.Stats()
	assert.False(t, stats.IsPaused)

	// Wait for job to execute
	time.Sleep(80 * time.Millisecond)
	assert.True(t, job.executed.Load(), "Job should execute after resume")

	wp.Stop()
}

func TestWorkerPoolCancel(t *testing.T) {
	wp := NewWorkerPool(2, 10)
	wp.Start()

	// Submit jobs with delay
	for i := 0; i < 5; i++ {
		job := &MockJob{
			id:    string(rune('a' + i)),
			delay: 100 * time.Millisecond,
		}
		wp.Submit(job)
	}

	// Cancel immediately
	go func() {
		time.Sleep(10 * time.Millisecond)
		wp.Cancel()
	}()

	wp.Cancel()

	// All should be cancelled
	stats := wp.Stats()
	assert.True(t, stats.JobsProcessed < stats.JobsQueued, "Not all jobs should be processed after cancel")
}

func TestWorkerPoolSubmitAfterClose(t *testing.T) {
	wp := NewWorkerPool(2, 10)
	wp.Start()

	// Close the pool
	wp.Wait()

	// Try to submit after close
	job := &MockJob{id: "after-close", shouldErr: false}
	err := wp.Submit(job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker pool is closed")
}

func TestWorkerPoolFullQueue(t *testing.T) {
	// Create a small queue
	wp := NewWorkerPool(1, 2)
	wp.Start()

	// Submit enough jobs to fill the queue
	job1 := &MockJob{id: "job1", delay: 100 * time.Millisecond}
	job2 := &MockJob{id: "job2", delay: 100 * time.Millisecond}
	job3 := &MockJob{id: "job3", delay: 100 * time.Millisecond}
	job4 := &MockJob{id: "job4", delay: 100 * time.Millisecond}

	err1 := wp.Submit(job1)
	assert.NoError(t, err1)

	err2 := wp.Submit(job2)
	assert.NoError(t, err2)

	// The next submission should fail because queue is full
	err3 := wp.Submit(job3)
	if err3 == nil {
		// If it succeeded, the next one should definitely fail
		err4 := wp.Submit(job4)
		assert.Error(t, err4)
	} else {
		assert.Error(t, err3)
	}

	wp.Cancel()
}

func TestBatchProcessor(t *testing.T) {
	wp := NewWorkerPool(3, 50)
	wp.Start()

	bp := NewBatchProcessor(wp, 5, 0) // No flush interval

	// Add jobs to batch
	jobs := make([]*MockJob, 12)
	for i := 0; i < 12; i++ {
		jobs[i] = &MockJob{id: string(rune('a' + i))}
		err := bp.Add(jobs[i])
		assert.NoError(t, err)
	}

	// Flush remaining jobs
	err := bp.Flush()
	assert.NoError(t, err)

	// Wait for all jobs to complete
	wp.Wait()

	// Verify all jobs were executed
	for i, job := range jobs {
		assert.True(t, job.executed.Load(), "Job %d should be executed", i)
	}
}

func TestBatchProcessorAutoFlush(t *testing.T) {
	wp := NewWorkerPool(3, 50)
	wp.Start()

	// Batch size of 3, so every 3 jobs should auto-flush
	bp := NewBatchProcessor(wp, 3, 0)

	jobs := make([]*MockJob, 7)
	for i := 0; i < 7; i++ {
		jobs[i] = &MockJob{id: string(rune('a' + i))}
		err := bp.Add(jobs[i])
		assert.NoError(t, err)
	}

	// Wait a moment
	time.Sleep(30 * time.Millisecond)

	// First 6 should be executed (2 batches of 3)
	for i := 0; i < 6; i++ {
		assert.True(t, jobs[i].executed.Load(), "Job %d should be auto-flushed", i)
	}

	// Last one should not be executed yet
	assert.False(t, jobs[6].executed.Load(), "Job 6 should not be executed yet")

	// Manual flush
	bp.Flush()

	// Wait for completion
	wp.Wait()

	// Now all should be executed
	assert.True(t, jobs[6].executed.Load(), "Job 6 should be executed after flush")
}

func TestBatchProcessorEmptyFlush(t *testing.T) {
	wp := NewWorkerPool(2, 10)
	wp.Start()

	bp := NewBatchProcessor(wp, 5, 0)

	// Flush without adding jobs
	err := bp.Flush()
	assert.NoError(t, err)

	wp.Stop()
}

func TestWorkerPoolStats(t *testing.T) {
	wp := NewWorkerPool(4, 20)
	assert.Equal(t, 4, wp.Stats().WorkerCount)
	assert.Equal(t, int64(0), wp.Stats().JobsQueued)
	assert.Equal(t, int64(0), wp.Stats().JobsProcessed)
	assert.Equal(t, int64(0), wp.Stats().JobsFailed)
	assert.False(t, wp.Stats().IsPaused)
}
