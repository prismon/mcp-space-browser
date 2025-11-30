package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/prismon/mcp-space-browser/pkg/logger"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("queue")
}

// Job represents a unit of work to be processed
type Job interface {
	Execute(ctx context.Context) error
	ID() string
}

// JobResult contains the result of a job execution
type JobResult struct {
	JobID string
	Error error
}

// WorkerPool manages a pool of workers that process jobs
type WorkerPool struct {
	workerCount    int
	jobs           chan Job
	results        chan JobResult
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	jobsWg         sync.WaitGroup // Tracks outstanding jobs
	resultsWg      sync.WaitGroup // Tracks result collector goroutine
	paused         atomic.Bool
	pauseChan      chan struct{}
	resumeChan     chan struct{}

	// Stats
	jobsProcessed  atomic.Int64
	jobsFailed     atomic.Int64
	jobsQueued     atomic.Int64

	// Control
	mu             sync.RWMutex
	started        bool
	closed         atomic.Bool
	cancelled      atomic.Bool
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workerCount int, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		workerCount: workerCount,
		jobs:        make(chan Job, queueSize),
		results:     make(chan JobResult, queueSize),
		ctx:         ctx,
		cancel:      cancel,
		pauseChan:   make(chan struct{}),
		resumeChan:  make(chan struct{}, 1),
	}
}

// Start begins processing jobs with the worker pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	if wp.started {
		wp.mu.Unlock()
		return
	}
	wp.started = true
	wp.mu.Unlock()

	log.WithField("workerCount", wp.workerCount).Info("Starting worker pool")

	// Start workers
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	// Start result collector
	wp.resultsWg.Add(1)
	go wp.collectResults()
}

// worker processes jobs from the queue
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	workerLog := log.WithField("workerID", id)
	workerLog.Debug("Worker started")

	for {
		// Check if paused
		if wp.paused.Load() {
			workerLog.Debug("Worker paused")
			select {
			case <-wp.resumeChan:
				workerLog.Debug("Worker resumed")
			case <-wp.ctx.Done():
				workerLog.Debug("Worker stopped (context cancelled while paused)")
				return
			}
		}

		select {
		case job, ok := <-wp.jobs:
			if !ok {
				workerLog.Debug("Worker stopped (job channel closed)")
				return
			}

			if logger.IsLevelEnabled(logrus.TraceLevel) {
				workerLog.WithField("jobID", job.ID()).Trace("Processing job")
			}

			err := job.Execute(wp.ctx)

			result := JobResult{
				JobID: job.ID(),
				Error: err,
			}

			// Signal that this job is done
			wp.jobsWg.Done()

			select {
			case wp.results <- result:
			case <-wp.ctx.Done():
				workerLog.Debug("Worker stopped (context cancelled)")
				return
			}

		case <-wp.ctx.Done():
			workerLog.Debug("Worker stopped (context cancelled)")
			return
		}
	}
}

// collectResults collects results from workers and updates stats
func (wp *WorkerPool) collectResults() {
	defer wp.resultsWg.Done()
	for result := range wp.results {
		wp.jobsProcessed.Add(1)

		if result.Error != nil {
			wp.jobsFailed.Add(1)
			log.WithFields(logrus.Fields{
				"jobID": result.JobID,
				"error": result.Error,
			}).Error("Job failed")
		} else {
			if logger.IsLevelEnabled(logrus.TraceLevel) {
				log.WithField("jobID", result.JobID).Trace("Job completed")
			}
		}
	}
}

// Submit adds a job to the queue
func (wp *WorkerPool) Submit(job Job) error {
	if wp.closed.Load() {
		return fmt.Errorf("worker pool is closed")
	}

	// Increment job counter before submitting
	wp.jobsWg.Add(1)

	select {
	case wp.jobs <- job:
		wp.jobsQueued.Add(1)
		if logger.IsLevelEnabled(logrus.TraceLevel) {
			log.WithField("jobID", job.ID()).Trace("Job queued")
		}
		return nil
	case <-wp.ctx.Done():
		wp.jobsWg.Done() // Decrement if we couldn't submit
		return fmt.Errorf("worker pool is shutting down")
	default:
		wp.jobsWg.Done() // Decrement if queue is full
		return fmt.Errorf("job queue is full")
	}
}

// Pause pauses all workers
func (wp *WorkerPool) Pause() {
	if !wp.paused.Swap(true) {
		close(wp.pauseChan)
		log.Info("Worker pool paused")
	}
}

// Resume resumes all workers
func (wp *WorkerPool) Resume() {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if wp.paused.Swap(false) {
		// Close the existing resumeChan to wake up all waiting workers
		close(wp.resumeChan)
		// Create a new channel for the next pause/resume cycle
		wp.pauseChan = make(chan struct{})
		wp.resumeChan = make(chan struct{}, 1)
		log.Info("Worker pool resumed")
	}
}

// Stop gracefully stops the worker pool
func (wp *WorkerPool) Stop() {
	log.Info("Stopping worker pool")

	// Close job channel to signal workers to stop after processing remaining jobs
	close(wp.jobs)

	// Wait for all workers to finish
	wp.wg.Wait()

	// Close results channel
	close(wp.results)

	// Wait for result collector to finish processing
	wp.resultsWg.Wait()

	log.Info("Worker pool stopped")
}

// Cancel immediately cancels the worker pool
func (wp *WorkerPool) Cancel() {
	// Prevent multiple cancellations
	if wp.cancelled.Swap(true) {
		return
	}

	log.Info("Cancelling worker pool")
	wp.cancel()
	wp.wg.Wait()

	// Close results channel to stop the collector
	close(wp.results)
	wp.resultsWg.Wait()

	log.Info("Worker pool cancelled")
}

// Wait waits for all submitted jobs to complete
func (wp *WorkerPool) Wait() {
	// Wait for all jobs to complete
	wp.jobsWg.Wait()

	// Mark as closed
	wp.closed.Store(true)

	// Close the jobs channel to signal no more jobs
	close(wp.jobs)

	// Wait for workers to finish
	wp.wg.Wait()

	// Close results channel
	close(wp.results)

	// Wait for result collector to finish processing
	wp.resultsWg.Wait()
}

// Stats returns current statistics
func (wp *WorkerPool) Stats() WorkerPoolStats {
	return WorkerPoolStats{
		WorkerCount:   wp.workerCount,
		JobsQueued:    wp.jobsQueued.Load(),
		JobsProcessed: wp.jobsProcessed.Load(),
		JobsFailed:    wp.jobsFailed.Load(),
		IsPaused:      wp.paused.Load(),
	}
}

// WorkerPoolStats contains statistics about the worker pool
type WorkerPoolStats struct {
	WorkerCount   int
	JobsQueued    int64
	JobsProcessed int64
	JobsFailed    int64
	IsPaused      bool
}

// BatchProcessor batches jobs and submits them to the worker pool
type BatchProcessor struct {
	pool       *WorkerPool
	batch      []Job
	batchSize  int
	flushTimer *time.Timer
	mu         sync.Mutex
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(pool *WorkerPool, batchSize int, flushInterval time.Duration) *BatchProcessor {
	bp := &BatchProcessor{
		pool:      pool,
		batch:     make([]Job, 0, batchSize),
		batchSize: batchSize,
	}

	if flushInterval > 0 {
		bp.flushTimer = time.AfterFunc(flushInterval, func() {
			bp.Flush()
		})
	}

	return bp
}

// Add adds a job to the batch
func (bp *BatchProcessor) Add(job Job) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.batch = append(bp.batch, job)

	if len(bp.batch) >= bp.batchSize {
		return bp.flushLocked()
	}

	return nil
}

// Flush submits all batched jobs to the worker pool
func (bp *BatchProcessor) Flush() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.flushLocked()
}

func (bp *BatchProcessor) flushLocked() error {
	if len(bp.batch) == 0 {
		return nil
	}

	for _, job := range bp.batch {
		if err := bp.pool.Submit(job); err != nil {
			return err
		}
	}

	bp.batch = bp.batch[:0]

	// Reset flush timer
	if bp.flushTimer != nil {
		bp.flushTimer.Reset(time.Second)
	}

	return nil
}
