package crawler

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/queue"
	"github.com/prismon/mcp-space-browser/pkg/source"
	"github.com/sirupsen/logrus"
)

// ParallelIndexer performs parallel indexing using a worker pool
type ParallelIndexer struct {
	root        string
	src         source.Source
	db          *database.DiskDB
	pool        *queue.WorkerPool
	runID       int64
	jobID       int64
	opts        *ParallelIndexOptions
	tracker     *source.ProgressTracker

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
	WorkerCount      int               // Number of parallel workers (default: number of CPUs)
	QueueSize        int               // Size of job queue (default: 10000)
	BatchSize        int               // Number of entries to batch before writing to DB (default: 1000)
	ProgressCallback ProgressCallback  // Optional progress callback
}

// DefaultParallelIndexOptions returns default options
func DefaultParallelIndexOptions() *ParallelIndexOptions {
	return &ParallelIndexOptions{
		WorkerCount: 8,  // Reasonable default for I/O bound operations
		QueueSize:   10000,
		BatchSize:   1000,
	}
}

// IndexParallel performs indexing using parallel workers with a source interface
// If src is nil, a default FileSystemSource will be used
func IndexParallel(root string, db *database.DiskDB, src source.Source, opts *ParallelIndexOptions) (*IndexStats, error) {
	startTime := time.Now()

	if opts == nil {
		opts = DefaultParallelIndexOptions()
	}

	// Use default filesystem source if none provided
	if src == nil {
		src = source.NewFileSystemSource()
		defer src.Close()
	}

	abs, err := source.ValidatePath(root)
	if err != nil {
		return nil, err
	}

	// Acquire indexing lock to prevent concurrent indexing operations
	if err := db.LockIndexing(); err != nil {
		return nil, fmt.Errorf("failed to acquire indexing lock: %w", err)
	}
	defer db.UnlockIndexing()

	runID := time.Now().Unix()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.WithFields(logrus.Fields{
		"root":       abs,
		"runID":      runID,
		"sourceType": src.Name(),
	}).Info("Starting parallel index operation")

	// Phase 1: Estimate total size
	log.WithField("root", abs).Info("Estimating total items to index")
	totalEstimate, err := src.EstimateSize(ctx, abs)
	if err != nil {
		log.WithError(err).Warn("Failed to estimate size, using default")
		totalEstimate = 1000
	}

	log.WithFields(logrus.Fields{
		"root":     abs,
		"estimate": totalEstimate,
	}).Info("Estimation complete")

	// Create progress tracker
	tracker := source.NewProgressTracker(totalEstimate)
	tracker.SetPhase("crawling")

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

	// Update job progress: estimation complete (5%)
	if err := db.UpdateIndexJobProgress(jobID, 5, &database.IndexJobMetadata{
		FilesProcessed:       0,
		DirectoriesProcessed: 0,
		TotalSize:            0,
		ErrorCount:           0,
		WorkerCount:          opts.WorkerCount,
	}); err != nil {
		log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
	}

	indexer := &ParallelIndexer{
		root:      abs,
		src:       src,
		db:        db,
		pool:      queue.NewWorkerPool(opts.WorkerCount, opts.QueueSize),
		runID:     runID,
		jobID:     jobID,
		opts:      opts,
		tracker:   tracker,
		batch:     make([]*models.Entry, 0, opts.BatchSize),
		batchSize: opts.BatchSize,
		ctx:       ctx,
		cancel:    cancel,
	}

	log.WithFields(logrus.Fields{
		"root":           abs,
		"runID":          runID,
		"jobID":          jobID,
		"workerCount":    opts.WorkerCount,
		"queueSize":      opts.QueueSize,
		"batchSize":      opts.BatchSize,
		"estimatedItems": totalEstimate,
	}).Info("Starting parallel crawl phase")

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

	// Phase 3: Cleanup - Delete stale entries (85-90%)
	tracker.SetPhase("cleanup")
	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
	}).Info("Deleting stale entries")

	if err := db.UpdateIndexJobProgress(jobID, 87, &database.IndexJobMetadata{
		FilesProcessed:       int(filesProcessed),
		DirectoriesProcessed: int(directoriesProcessed),
		TotalSize:            totalSize,
		ErrorCount:           int(errorCount),
		WorkerCount:          opts.WorkerCount,
	}); err != nil {
		log.WithError(err).Error("Failed to update job progress")
	}

	if err := db.DeleteStale(abs, runID); err != nil {
		db.UpdateIndexJobStatus(jobID, "failed", stringPtr(err.Error()))
		return nil, fmt.Errorf("failed to delete stale entries: %w", err)
	}

	// Phase 4: Aggregation - Compute aggregate sizes (90-100%)
	tracker.SetPhase("aggregation")
	log.WithField("root", abs).Info("Computing aggregate sizes")

	if err := db.UpdateIndexJobProgress(jobID, 92, &database.IndexJobMetadata{
		FilesProcessed:       int(filesProcessed),
		DirectoriesProcessed: int(directoriesProcessed),
		TotalSize:            totalSize,
		ErrorCount:           int(errorCount),
		WorkerCount:          opts.WorkerCount,
	}); err != nil {
		log.WithError(err).Error("Failed to update job progress")
	}

	if err := db.ComputeAggregates(abs); err != nil {
		db.UpdateIndexJobStatus(jobID, "failed", stringPtr(err.Error()))
		return nil, fmt.Errorf("failed to compute aggregates: %w", err)
	}

	// Complete
	tracker.SetPhase("complete")
	endTime := time.Now()

	// Update job progress to 100%
	if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
		FilesProcessed:       int(filesProcessed),
		DirectoriesProcessed: int(directoriesProcessed),
		TotalSize:            totalSize,
		ErrorCount:           int(errorCount),
		WorkerCount:          opts.WorkerCount,
	}); err != nil {
		log.WithError(err).Error("Failed to update job progress")
	}

	// Mark job as completed
	if err := db.UpdateIndexJobStatus(jobID, "completed", nil); err != nil {
		log.WithError(err).Error("Failed to update job status")
	}

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
			poolStats := pi.pool.Stats()
			filesProcessed := pi.filesProcessed.Load()
			directoriesProcessed := pi.directoriesProcessed.Load()

			// Update tracker with queue size
			pi.tracker.SetQueuedItems(poolStats.JobsQueued)

			estimate := pi.tracker.GetEstimate()

			log.WithFields(logrus.Fields{
				"filesProcessed":       filesProcessed,
				"directoriesProcessed": directoriesProcessed,
				"jobsQueued":           poolStats.JobsQueued,
				"jobsProcessed":        poolStats.JobsProcessed,
				"jobsFailed":           poolStats.JobsFailed,
				"percentComplete":      estimate.PercentComplete(),
			}).Info("Index progress")

			// Call progress callback if provided
			if pi.opts.ProgressCallback != nil {
				indexStats := &IndexStats{
					FilesProcessed:       int(filesProcessed),
					DirectoriesProcessed: int(directoriesProcessed),
					TotalSize:            pi.totalSize.Load(),
					Errors:               int(pi.errors.Load()),
				}
				pi.opts.ProgressCallback(indexStats, int(poolStats.JobsQueued))
			}

			// Use progress tracker for accurate percentage
			progress := pi.tracker.GetPercentComplete()

			// Update job progress in database with accurate calculation
			if err := pi.db.UpdateIndexJobProgress(pi.jobID, progress, &database.IndexJobMetadata{
				FilesProcessed:       int(filesProcessed),
				DirectoriesProcessed: int(directoriesProcessed),
				TotalSize:            pi.totalSize.Load(),
				ErrorCount:           int(pi.errors.Load()),
				WorkerCount:          pi.opts.WorkerCount,
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

	// Update tracker
	j.indexer.tracker.SetCurrentPath(j.path)

	info, err := j.indexer.src.Stat(ctx, j.path)
	if err != nil {
		j.indexer.errors.Add(1)
		j.indexer.tracker.IncrementErrors()
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
		j.indexer.tracker.IncrementErrors()
		log.WithFields(logrus.Fields{
			"path":  j.path,
			"error": err,
		}).Error("Failed to add entry to batch")
		return err
	}

	if isDir {
		j.indexer.directoriesProcessed.Add(1)
		j.indexer.tracker.IncrementDirectories()

		children, err := j.indexer.src.ReadDir(ctx, j.path)
		if err != nil {
			j.indexer.errors.Add(1)
			j.indexer.tracker.IncrementErrors()
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
			childPath := source.GetFullPath(j.path, child)
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
		j.indexer.tracker.IncrementFiles(info.Size())

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
