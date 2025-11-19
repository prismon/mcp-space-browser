package crawler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/queue"
	"github.com/sirupsen/logrus"
)

// ParallelIndexer performs parallel filesystem indexing using a worker pool
type ParallelIndexer struct {
	root        string
	db          *database.DiskDB
	pool        *queue.WorkerPool
	runID       int64
	jobID       int64

	// Stats
	filesProcessed       atomic.Int64
	directoriesProcessed atomic.Int64
	totalSize            atomic.Int64
	errors               atomic.Int64

	// Database write batching
	batchMu     sync.Mutex
	batch       []*models.Entry
	batchSize   int

	// Control
	ctx         context.Context
	cancel      context.CancelFunc
}

// ParallelIndexOptions configures the parallel indexer
type ParallelIndexOptions struct {
	WorkerCount int  // Number of parallel workers (default: number of CPUs)
	QueueSize   int  // Size of job queue (default: 10000)
	BatchSize   int  // Number of entries to batch before writing to DB (default: 1000)
}

// DefaultParallelIndexOptions returns default options
func DefaultParallelIndexOptions() *ParallelIndexOptions {
	return &ParallelIndexOptions{
		WorkerCount: 8,  // Reasonable default for I/O bound operations
		QueueSize:   10000,
		BatchSize:   1000,
	}
}

// IndexParallel performs filesystem indexing using parallel workers
func IndexParallel(root string, db *database.DiskDB, opts *ParallelIndexOptions) (*IndexStats, error) {
	startTime := time.Now()

	if opts == nil {
		opts = DefaultParallelIndexOptions()
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	runID := time.Now().Unix()

	// Create job in database
	jobID, err := db.CreateIndexJob(abs, &database.IndexJobMetadata{
		WorkerCount: opts.WorkerCount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create index job: %w", err)
	}

	if err := db.StartIndexJob(jobID); err != nil {
		return nil, fmt.Errorf("failed to start index job: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	indexer := &ParallelIndexer{
		root:      abs,
		db:        db,
		pool:      queue.NewWorkerPool(opts.WorkerCount, opts.QueueSize),
		runID:     runID,
		jobID:     jobID,
		batch:     make([]*models.Entry, 0, opts.BatchSize),
		batchSize: opts.BatchSize,
		ctx:       ctx,
		cancel:    cancel,
	}

	log.WithFields(logrus.Fields{
		"root":        abs,
		"runID":       runID,
		"jobID":       jobID,
		"workerCount": opts.WorkerCount,
		"queueSize":   opts.QueueSize,
		"batchSize":   opts.BatchSize,
	}).Info("Starting parallel filesystem index")

	// Start worker pool
	indexer.pool.Start()

	// Start progress reporter
	stopProgress := make(chan struct{})
	go indexer.reportProgress(stopProgress)

	// Begin database transaction
	if err := db.BeginTransaction(); err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			db.RollbackTransaction()
			panic(r)
		}
	}()

	// Submit root directory job
	rootJob := &DirectoryScanJob{
		path:    abs,
		indexer: indexer,
	}

	if err := indexer.pool.Submit(rootJob); err != nil {
		db.RollbackTransaction()
		return nil, fmt.Errorf("failed to submit root job: %w", err)
	}

	// Wait for all jobs to complete
	indexer.pool.Wait()

	// Stop progress reporter
	close(stopProgress)

	// Flush remaining batch
	if err := indexer.flushBatch(); err != nil {
		db.RollbackTransaction()
		return nil, fmt.Errorf("failed to flush batch: %w", err)
	}

	// Commit transaction
	if err := db.CommitTransaction(); err != nil {
		db.RollbackTransaction()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	filesProcessed := indexer.filesProcessed.Load()
	directoriesProcessed := indexer.directoriesProcessed.Load()
	totalSize := indexer.totalSize.Load()
	errorCount := indexer.errors.Load()

	log.WithFields(logrus.Fields{
		"root":                 abs,
		"filesProcessed":       filesProcessed,
		"directoriesProcessed": directoriesProcessed,
		"totalSize":            totalSize,
		"errors":               errorCount,
		"runID":                runID,
	}).Info("Filesystem scan complete")

	// Update job metadata
	if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
		FilesProcessed:       int(filesProcessed),
		DirectoriesProcessed: int(directoriesProcessed),
		TotalSize:            totalSize,
		ErrorCount:           int(errorCount),
		WorkerCount:          opts.WorkerCount,
	}); err != nil {
		log.WithError(err).Error("Failed to update job progress")
	}

	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
	}).Debug("Deleting stale entries")

	if err := db.DeleteStale(abs, runID); err != nil {
		db.UpdateIndexJobStatus(jobID, "failed", stringPtr(err.Error()))
		return nil, fmt.Errorf("failed to delete stale entries: %w", err)
	}

	log.WithField("root", abs).Debug("Computing aggregate sizes")

	if err := db.ComputeAggregates(abs); err != nil {
		db.UpdateIndexJobStatus(jobID, "failed", stringPtr(err.Error()))
		return nil, fmt.Errorf("failed to compute aggregates: %w", err)
	}

	// Mark job as completed
	if err := db.UpdateIndexJobStatus(jobID, "completed", nil); err != nil {
		log.WithError(err).Error("Failed to update job status")
	}

	endTime := time.Now()
	stats := &IndexStats{
		FilesProcessed:       int(filesProcessed),
		DirectoriesProcessed: int(directoriesProcessed),
		TotalSize:            totalSize,
		Errors:               int(errorCount),
		Duration:             endTime.Sub(startTime),
		StartTime:            startTime,
		EndTime:              endTime,
	}

	log.WithFields(logrus.Fields{
		"root":     abs,
		"duration": stats.Duration,
	}).Info("Index operation complete")

	return stats, nil
}

// reportProgress periodically reports indexing progress
func (pi *ParallelIndexer) reportProgress(stop chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := pi.pool.Stats()
			filesProcessed := pi.filesProcessed.Load()
			directoriesProcessed := pi.directoriesProcessed.Load()

			log.WithFields(logrus.Fields{
				"filesProcessed":       filesProcessed,
				"directoriesProcessed": directoriesProcessed,
				"jobsQueued":           stats.JobsQueued,
				"jobsProcessed":        stats.JobsProcessed,
				"jobsFailed":           stats.JobsFailed,
			}).Info("Index progress")

			// Update job progress in database (estimate progress based on files processed)
			// This is a rough estimate - you could improve this with better heuristics
			progress := 50 // Arbitrarily set to 50% while running
			if err := pi.db.UpdateIndexJobProgress(pi.jobID, progress, &database.IndexJobMetadata{
				FilesProcessed:       int(filesProcessed),
				DirectoriesProcessed: int(directoriesProcessed),
				TotalSize:            pi.totalSize.Load(),
				ErrorCount:           int(pi.errors.Load()),
			}); err != nil {
				log.WithError(err).Error("Failed to update job progress")
			}

		case <-stop:
			return
		}
	}
}

// addToBatch adds an entry to the write batch
func (pi *ParallelIndexer) addToBatch(entry *models.Entry) error {
	pi.batchMu.Lock()
	defer pi.batchMu.Unlock()

	pi.batch = append(pi.batch, entry)

	if len(pi.batch) >= pi.batchSize {
		return pi.flushBatchLocked()
	}

	return nil
}

// flushBatch writes all batched entries to the database
func (pi *ParallelIndexer) flushBatch() error {
	pi.batchMu.Lock()
	defer pi.batchMu.Unlock()
	return pi.flushBatchLocked()
}

func (pi *ParallelIndexer) flushBatchLocked() error {
	if len(pi.batch) == 0 {
		return nil
	}

	for _, entry := range pi.batch {
		if err := pi.db.InsertOrUpdate(entry); err != nil {
			return err
		}
	}

	if logger.IsLevelEnabled(logrus.DebugLevel) {
		log.WithField("batchSize", len(pi.batch)).Debug("Flushed batch to database")
	}

	pi.batch = pi.batch[:0]
	return nil
}

// Pause pauses the indexing operation
func (pi *ParallelIndexer) Pause() error {
	pi.pool.Pause()
	return pi.db.UpdateIndexJobStatus(pi.jobID, "paused", nil)
}

// Resume resumes the indexing operation
func (pi *ParallelIndexer) Resume() error {
	pi.pool.Resume()
	return pi.db.UpdateIndexJobStatus(pi.jobID, "running", nil)
}

// Cancel cancels the indexing operation
func (pi *ParallelIndexer) Cancel() error {
	pi.cancel()
	pi.pool.Cancel()
	return pi.db.UpdateIndexJobStatus(pi.jobID, "cancelled", nil)
}

// DirectoryScanJob represents a job to scan a directory
type DirectoryScanJob struct {
	path    string
	indexer *ParallelIndexer
}

// ID returns the job ID
func (j *DirectoryScanJob) ID() string {
	return j.path
}

// Execute processes a directory
func (j *DirectoryScanJob) Execute(ctx context.Context) error {
	// Check if cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	info, err := os.Stat(j.path)
	if err != nil {
		j.indexer.errors.Add(1)
		log.WithFields(logrus.Fields{
			"path":  j.path,
			"error": err,
		}).Error("Failed to stat path")
		return err
	}

	isDir := info.IsDir()
	parent := filepath.Dir(j.path)
	if parent == j.path {
		parent = ""
	}

	var parentPtr *string
	if parent != "" {
		parentPtr = &parent
	}

	entry := &models.Entry{
		Path:        j.path,
		Parent:      parentPtr,
		Size:        info.Size(),
		Kind:        "file",
		Ctime:       info.ModTime().Unix(),
		Mtime:       info.ModTime().Unix(),
		LastScanned: j.indexer.runID,
	}

	if isDir {
		entry.Kind = "directory"
	}

	// Add to batch for writing
	if err := j.indexer.addToBatch(entry); err != nil {
		j.indexer.errors.Add(1)
		log.WithFields(logrus.Fields{
			"path":  j.path,
			"error": err,
		}).Error("Failed to add entry to batch")
		return err
	}

	if isDir {
		j.indexer.directoriesProcessed.Add(1)

		children, err := os.ReadDir(j.path)
		if err != nil {
			j.indexer.errors.Add(1)
			log.WithFields(logrus.Fields{
				"path":  j.path,
				"error": err,
			}).Error("Failed to read directory")
			return err
		}

		if logger.IsLevelEnabled(logrus.TraceLevel) {
			log.WithFields(logrus.Fields{
				"path":       j.path,
				"childCount": len(children),
			}).Trace("Directory contents")
		}

		// Submit child directories and files as separate jobs
		for _, child := range children {
			childPath := filepath.Join(j.path, child.Name())
			childJob := &DirectoryScanJob{
				path:    childPath,
				indexer: j.indexer,
			}

			if err := j.indexer.pool.Submit(childJob); err != nil {
				// If queue is full or pool is shutting down, process synchronously
				if err := childJob.Execute(ctx); err != nil {
					log.WithError(err).Error("Failed to process child job")
				}
			}
		}
	} else {
		j.indexer.filesProcessed.Add(1)
		j.indexer.totalSize.Add(info.Size())

		if logger.IsLevelEnabled(logrus.TraceLevel) {
			log.WithFields(logrus.Fields{
				"path": j.path,
				"size": info.Size(),
			}).Trace("File processed")
		}
	}

	return nil
}

func stringPtr(s string) *string {
	return &s
}
