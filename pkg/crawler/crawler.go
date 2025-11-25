package crawler

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/source"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

const (
	// batchSize is the number of entries to process before committing a transaction
	// This prevents holding locks for too long during large scans
	batchSize = 1000

	// DefaultMaxAge is the default maximum age (in seconds) before a path is considered stale
	// and needs to be re-indexed. Default: 1 hour (3600 seconds)
	DefaultMaxAge = 3600
)

func init() {
	log = logger.WithName("crawler")
}

// IndexOptions configures the behavior of the Index operation
type IndexOptions struct {
	// Force forces re-indexing even if the path was recently scanned
	Force bool

	// MaxAge is the maximum age in seconds before a scan is considered stale.
	// If a path was scanned within MaxAge seconds, indexing will be skipped (unless Force is true).
	// Default: 3600 (1 hour). Set to 0 to always re-index.
	MaxAge int64
}

// DefaultIndexOptions returns the default indexing options
func DefaultIndexOptions() *IndexOptions {
	return &IndexOptions{
		Force:  false,
		MaxAge: DefaultMaxAge,
	}
}

// IndexStats contains statistics about an indexing operation
type IndexStats struct {
	FilesProcessed       int
	DirectoriesProcessed int
	TotalSize            int64
	Errors               int
	Duration             time.Duration
	StartTime            time.Time
	EndTime              time.Time
	Skipped              bool   // True if indexing was skipped due to recent scan
	SkipReason           string // Reason for skipping (if Skipped is true)
}

// ProgressCallback is a callback function for progress updates during indexing
// stats: current statistics
// remaining: number of items remaining in queue
type ProgressCallback func(stats *IndexStats, remaining int)

// Index performs indexing using a source interface with stack-based DFS traversal
// If src is nil, a default FileSystemSource will be used
// If jobID is provided (non-zero), it will update job progress in the database
// If progressCallback is provided, it will be called with progress updates
func Index(root string, db *database.DiskDB, src source.Source, jobID int64, progressCallback ProgressCallback) (*IndexStats, error) {
	return IndexWithOptions(root, db, src, jobID, progressCallback, nil)
}

// IndexWithOptions performs indexing with configurable options
// If opts is nil, default options will be used (skip if scanned within 1 hour)
func IndexWithOptions(root string, db *database.DiskDB, src source.Source, jobID int64, progressCallback ProgressCallback, opts *IndexOptions) (*IndexStats, error) {
	startTime := time.Now()
	ctx := context.Background()

	// Use default options if none provided
	if opts == nil {
		opts = DefaultIndexOptions()
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

	// Check if the path was recently scanned (unless Force is set)
	if !opts.Force && opts.MaxAge > 0 {
		scanInfo, err := db.GetPathScanInfo(abs)
		if err != nil {
			log.WithError(err).WithField("path", abs).Warn("Failed to get path scan info, proceeding with indexing")
		} else if scanInfo.Exists && scanInfo.LastScanned > 0 {
			now := time.Now().Unix()
			age := now - scanInfo.LastScanned

			if age < opts.MaxAge {
				log.WithFields(logrus.Fields{
					"path":        abs,
					"lastScanned": time.Unix(scanInfo.LastScanned, 0).Format(time.RFC3339),
					"ageSeconds":  age,
					"maxAge":      opts.MaxAge,
					"entryCount":  scanInfo.EntryCount,
				}).Info("Skipping indexing - path was recently scanned")

				// Update job status if provided
				if jobID > 0 {
					if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
						FilesProcessed:       0,
						DirectoriesProcessed: 0,
						TotalSize:            0,
						ErrorCount:           0,
					}); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
					}
				}

				return &IndexStats{
					StartTime:  startTime,
					EndTime:    time.Now(),
					Duration:   time.Since(startTime),
					Skipped:    true,
					SkipReason: fmt.Sprintf("Path was scanned %d seconds ago (max age: %d seconds). Use force=true to re-index.", age, opts.MaxAge),
				}, nil
			}

			log.WithFields(logrus.Fields{
				"path":        abs,
				"lastScanned": time.Unix(scanInfo.LastScanned, 0).Format(time.RFC3339),
				"ageSeconds":  age,
				"maxAge":      opts.MaxAge,
			}).Info("Path scan is stale, proceeding with re-indexing")
		}
	} else if opts.Force {
		log.WithField("path", abs).Info("Force flag set, proceeding with indexing regardless of last scan time")
	}

	// Acquire indexing lock to prevent concurrent indexing operations
	if err := db.LockIndexing(); err != nil {
		return nil, fmt.Errorf("failed to acquire indexing lock: %w", err)
	}
	defer db.UnlockIndexing()

	runID := time.Now().Unix()

	log.WithFields(logrus.Fields{
		"root":       abs,
		"runID":      runID,
		"jobID":      jobID,
		"sourceType": src.Name(),
	}).Info("Starting index operation")

	// Phase 1: Estimate total size
	log.WithField("root", abs).Info("Estimating total items to index")
	totalEstimate, err := src.EstimateSize(ctx, abs)
	if err != nil {
		log.WithError(err).Warn("Failed to estimate size, using default")
		totalEstimate = 1000 // Default estimate
	}

	log.WithFields(logrus.Fields{
		"root":     abs,
		"estimate": totalEstimate,
	}).Info("Estimation complete")

	// Create progress tracker
	tracker := source.NewProgressTracker(totalEstimate)
	tracker.SetPhase("crawling")

	// Update job progress: estimation complete (5%)
	if jobID > 0 {
		if err := db.UpdateIndexJobProgress(jobID, 5, &database.IndexJobMetadata{
			FilesProcessed:       0,
			DirectoriesProcessed: 0,
			TotalSize:            0,
			ErrorCount:           0,
		}); err != nil {
			log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
		}
	}

	stack := []string{abs}

	log.WithFields(logrus.Fields{
		"root":          abs,
		"runID":         runID,
		"estimatedItems": totalEstimate,
	}).Info("Starting crawl phase")

	stats := &IndexStats{
		StartTime: startTime,
	}
	lastProgressLog := time.Now()
	lastProgressUpdate := time.Now()
	entriesInBatch := 0

	// Begin transaction for better performance with batching
	if err := db.BeginTransaction(); err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			db.RollbackTransaction()
			panic(r)
		}
	}()

	for len(stack) > 0 {
		// Pop from stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Update progress tracker
		tracker.SetCurrentPath(current)
		tracker.SetQueuedItems(int64(len(stack)))

		if logger.IsLevelEnabled(logrus.DebugLevel) {
			log.WithFields(logrus.Fields{
				"path":      current,
				"remaining": len(stack),
			}).Debug("Processing path")
		}

		info, err := src.Stat(ctx, current)
		if err != nil {
			stats.Errors++
			tracker.IncrementErrors()
			log.WithFields(logrus.Fields{
				"path":  current,
				"error": err,
			}).Error("Failed to stat path")
			continue
		}

		isDir := info.IsDir()
		parent := filepath.Dir(current)
		if parent == current {
			parent = ""
		}

		var parentPtr *string
		if parent != "" {
			parentPtr = &parent
		}

		entry := &models.Entry{
			Path:        current,
			Parent:      parentPtr,
			Size:        info.Size(),
			Kind:        "file",
			Ctime:       info.ModTime().Unix(), // Go doesn't expose ctime directly
			Mtime:       info.ModTime().Unix(),
			LastScanned: runID,
		}

		if isDir {
			entry.Kind = "directory"
		}

		if err := db.InsertOrUpdate(entry); err != nil {
			stats.Errors++
			tracker.IncrementErrors()
			log.WithFields(logrus.Fields{
				"path":  current,
				"error": err,
			}).Error("Failed to insert/update entry")
			continue
		}

		entriesInBatch++

		// Commit in batches to avoid holding locks for too long
		if entriesInBatch >= batchSize {
			if err := db.CommitTransaction(); err != nil {
				return nil, fmt.Errorf("failed to commit batch transaction: %w", err)
			}

			if logger.IsLevelEnabled(logrus.DebugLevel) {
				log.WithField("entriesCommitted", entriesInBatch).Debug("Committed batch")
			}

			// Start a new transaction for the next batch
			if err := db.BeginTransaction(); err != nil {
				return nil, fmt.Errorf("failed to begin new batch transaction: %w", err)
			}

			entriesInBatch = 0
		}

		if isDir {
			stats.DirectoriesProcessed++
			tracker.IncrementDirectories()

			if logger.IsLevelEnabled(logrus.DebugLevel) {
				log.WithField("path", current).Debug("Scanning directory")
			}

			children, err := src.ReadDir(ctx, current)
			if err != nil {
				stats.Errors++
				tracker.IncrementErrors()
				log.WithFields(logrus.Fields{
					"path":  current,
					"error": err,
				}).Error("Failed to read directory")
				continue
			}

			if logger.IsLevelEnabled(logrus.TraceLevel) {
				log.WithFields(logrus.Fields{
					"path":       current,
					"childCount": len(children),
				}).Trace("Directory contents")
			}

			for _, child := range children {
				stack = append(stack, source.GetFullPath(current, child))
			}
		} else {
			stats.FilesProcessed++
			stats.TotalSize += info.Size()
			tracker.IncrementFiles(info.Size())

			if logger.IsLevelEnabled(logrus.TraceLevel) {
				log.WithFields(logrus.Fields{
					"path": current,
					"size": info.Size(),
				}).Trace("File processed")
			}
		}

		// Log and update progress every 5 seconds
		now := time.Now()
		if now.Sub(lastProgressLog) > 5*time.Second {
			estimate := tracker.GetEstimate()
			log.WithFields(logrus.Fields{
				"filesProcessed":       stats.FilesProcessed,
				"directoriesProcessed": stats.DirectoriesProcessed,
				"remaining":            len(stack),
				"percentComplete":      estimate.PercentComplete(),
			}).Info("Index progress")

			// Call progress callback if provided
			if progressCallback != nil {
				progressCallback(stats, len(stack))
			}

			lastProgressLog = now
		}

		// Update job progress in database every 5 seconds
		if jobID > 0 && now.Sub(lastProgressUpdate) > 5*time.Second {
			// Use progress tracker for accurate percentage
			progress := tracker.GetPercentComplete()

			if err := db.UpdateIndexJobProgress(jobID, progress, &database.IndexJobMetadata{
				FilesProcessed:       stats.FilesProcessed,
				DirectoriesProcessed: stats.DirectoriesProcessed,
				TotalSize:            stats.TotalSize,
				ErrorCount:           stats.Errors,
			}); err != nil {
				log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
			}
			lastProgressUpdate = now
		}
	}

	// Commit the final batch (if any entries remain uncommitted)
	if err := db.CommitTransaction(); err != nil {
		db.RollbackTransaction()
		return nil, fmt.Errorf("failed to commit final batch: %w", err)
	}

	if logger.IsLevelEnabled(logrus.DebugLevel) {
		log.WithFields(logrus.Fields{
			"finalBatchSize": entriesInBatch,
			"totalEntries":   stats.FilesProcessed + stats.DirectoriesProcessed,
		}).Debug("Committed final batch")
	}

	log.WithFields(logrus.Fields{
		"root":                 abs,
		"filesProcessed":       stats.FilesProcessed,
		"directoriesProcessed": stats.DirectoriesProcessed,
		"totalSize":            stats.TotalSize,
		"errors":               stats.Errors,
		"runID":                runID,
	}).Info("Filesystem scan complete")

	// Call progress callback after scan complete
	if progressCallback != nil {
		progressCallback(stats, 0)
	}

	// Phase 3: Cleanup - Delete stale entries (85-90%)
	tracker.SetPhase("cleanup")
	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
	}).Info("Deleting stale entries")

	if jobID > 0 {
		if err := db.UpdateIndexJobProgress(jobID, 87, &database.IndexJobMetadata{
			FilesProcessed:       stats.FilesProcessed,
			DirectoriesProcessed: stats.DirectoriesProcessed,
			TotalSize:            stats.TotalSize,
			ErrorCount:           stats.Errors,
		}); err != nil {
			log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
		}
	}

	if err := db.DeleteStale(abs, runID); err != nil {
		return nil, fmt.Errorf("failed to delete stale entries: %w", err)
	}

	// Phase 4: Aggregation - Compute aggregate sizes (90-100%)
	tracker.SetPhase("aggregation")
	log.WithField("root", abs).Info("Computing aggregate sizes")

	if jobID > 0 {
		if err := db.UpdateIndexJobProgress(jobID, 92, &database.IndexJobMetadata{
			FilesProcessed:       stats.FilesProcessed,
			DirectoriesProcessed: stats.DirectoriesProcessed,
			TotalSize:            stats.TotalSize,
			ErrorCount:           stats.Errors,
		}); err != nil {
			log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
		}
	}

	if err := db.ComputeAggregates(abs); err != nil {
		return nil, fmt.Errorf("failed to compute aggregates: %w", err)
	}

	// Complete
	tracker.SetPhase("complete")
	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	if jobID > 0 {
		if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
			FilesProcessed:       stats.FilesProcessed,
			DirectoriesProcessed: stats.DirectoriesProcessed,
			TotalSize:            stats.TotalSize,
			ErrorCount:           stats.Errors,
		}); err != nil {
			log.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
		}
	}

	log.WithFields(logrus.Fields{
		"root":     abs,
		"duration": stats.Duration,
	}).Info("Index operation complete")

	return stats, nil
}
