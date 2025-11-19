package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("crawler")
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
}

// ProgressCallback is a callback function for progress updates during indexing
// stats: current statistics
// remaining: number of items remaining in queue
type ProgressCallback func(stats *IndexStats, remaining int)

// Index performs filesystem indexing using stack-based DFS traversal
// If jobID is provided (non-zero), it will update job progress in the database
// If progressCallback is provided, it will be called with progress updates
func Index(root string, db *database.DiskDB, jobID int64, progressCallback ProgressCallback) (*IndexStats, error) {
	startTime := time.Now()

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	runID := time.Now().Unix()
	stack := []string{abs}

	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
		"jobID": jobID,
	}).Info("Starting filesystem index")

	stats := &IndexStats{
		StartTime: startTime,
	}
	lastProgressLog := time.Now()
	lastProgressUpdate := time.Now()

	// Begin transaction for better performance
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

		if logger.IsLevelEnabled(logrus.DebugLevel) {
			log.WithFields(logrus.Fields{
				"path":      current,
				"remaining": len(stack),
			}).Debug("Processing path")
		}

		info, err := os.Stat(current)
		if err != nil {
			stats.Errors++
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
			log.WithFields(logrus.Fields{
				"path":  current,
				"error": err,
			}).Error("Failed to insert/update entry")
			continue
		}

		if isDir {
			stats.DirectoriesProcessed++

			if logger.IsLevelEnabled(logrus.DebugLevel) {
				log.WithField("path", current).Debug("Scanning directory")
			}

			children, err := os.ReadDir(current)
			if err != nil {
				stats.Errors++
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
				stack = append(stack, filepath.Join(current, child.Name()))
			}
		} else {
			stats.FilesProcessed++
			stats.TotalSize += info.Size()

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
			log.WithFields(logrus.Fields{
				"filesProcessed":       stats.FilesProcessed,
				"directoriesProcessed": stats.DirectoriesProcessed,
				"remaining":            len(stack),
			}).Info("Index progress")

			// Call progress callback if provided
			if progressCallback != nil {
				progressCallback(stats, len(stack))
			}

			lastProgressLog = now
		}

		// Update job progress in database every 5 seconds
		if jobID > 0 && now.Sub(lastProgressUpdate) > 5*time.Second {
			// Estimate progress (10-90% range, as final steps happen after crawling)
			// We don't have total count, so we just show activity by incrementing
			progress := 10 // Start at 10%
			if stats.FilesProcessed > 0 || stats.DirectoriesProcessed > 0 {
				// Increment progress gradually, capped at 90%
				entriesProcessed := stats.FilesProcessed + stats.DirectoriesProcessed
				// Simple heuristic: 1% per 100 entries processed, up to 90%
				progress = 10 + min(80, entriesProcessed/100)
			}

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

	// Commit the transaction
	if err := db.CommitTransaction(); err != nil {
		db.RollbackTransaction()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
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

	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
	}).Debug("Deleting stale entries")

	if err := db.DeleteStale(abs, runID); err != nil {
		return nil, fmt.Errorf("failed to delete stale entries: %w", err)
	}

	log.WithField("root", abs).Debug("Computing aggregate sizes")

	if err := db.ComputeAggregates(abs); err != nil {
		return nil, fmt.Errorf("failed to compute aggregates: %w", err)
	}

	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	log.WithFields(logrus.Fields{
		"root":     abs,
		"duration": stats.Duration,
	}).Info("Index operation complete")

	return stats, nil
}
