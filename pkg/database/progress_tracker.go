package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var ptLog *logrus.Entry

func init() {
	ptLog = logger.WithName("progress-tracker")
}

// ProgressTracker provides in-memory progress tracking with batched database updates.
// Instead of writing to the database on every progress update (which causes lock
// contention), updates accumulate in memory and are flushed periodically.
type ProgressTracker struct {
	jobID         int64
	progress      int
	metadata      *IndexJobMetadata
	lastFlush     time.Time
	flushInterval time.Duration
	writeQueue    *WriteQueue
	mu            sync.Mutex
	dirty         bool // true if there are unflushed updates
}

// ProgressTrackerConfig configures the progress tracker behavior
type ProgressTrackerConfig struct {
	// FlushInterval is how often to write progress to the database (default: 10s)
	FlushInterval time.Duration
}

// DefaultProgressTrackerConfig returns sensible defaults
func DefaultProgressTrackerConfig() *ProgressTrackerConfig {
	return &ProgressTrackerConfig{
		FlushInterval: 10 * time.Second,
	}
}

// NewProgressTracker creates a new progress tracker for the given job
func NewProgressTracker(jobID int64, writeQueue *WriteQueue, config *ProgressTrackerConfig) *ProgressTracker {
	if config == nil {
		config = DefaultProgressTrackerConfig()
	}

	return &ProgressTracker{
		jobID:         jobID,
		progress:      0,
		metadata:      nil,
		lastFlush:     time.Now(),
		flushInterval: config.FlushInterval,
		writeQueue:    writeQueue,
		dirty:         false,
	}
}

// Update records a progress update in memory. If enough time has passed since
// the last flush, the update will be written to the database.
func (pt *ProgressTracker) Update(progress int, metadata *IndexJobMetadata) {
	pt.mu.Lock()
	pt.progress = progress
	pt.metadata = metadata
	pt.dirty = true
	shouldFlush := time.Since(pt.lastFlush) >= pt.flushInterval
	pt.mu.Unlock()

	if shouldFlush {
		pt.Flush()
	}
}

// Flush writes the current progress to the database immediately.
// This is safe to call even if there are no updates to flush.
func (pt *ProgressTracker) Flush() error {
	pt.mu.Lock()
	if !pt.dirty {
		pt.mu.Unlock()
		return nil
	}

	progress := pt.progress
	metadata := pt.metadata
	jobID := pt.jobID
	pt.dirty = false
	pt.lastFlush = time.Now()
	pt.mu.Unlock()

	ctx := context.Background()

	return pt.writeQueue.Submit(ctx, func(db *sql.DB) error {
		var metadataJSON *string
		if metadata != nil {
			bytes, err := json.Marshal(metadata)
			if err != nil {
				ptLog.WithError(err).Error("Failed to marshal metadata")
				return err
			}
			str := string(bytes)
			metadataJSON = &str
		}

		now := time.Now().Unix()

		_, err := db.Exec(`
			UPDATE index_jobs
			SET progress = ?, metadata = ?, updated_at = ?
			WHERE id = ?
		`, progress, metadataJSON, now, jobID)

		if err != nil {
			ptLog.WithError(err).WithField("jobID", jobID).Error("Failed to update job progress")
			return err
		}

		ptLog.WithFields(logrus.Fields{
			"jobID":    jobID,
			"progress": progress,
		}).Debug("Flushed progress to database")

		return nil
	})
}

// FlushSync flushes progress and waits for completion with a timeout
func (pt *ProgressTracker) FlushSync(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pt.mu.Lock()
	if !pt.dirty {
		pt.mu.Unlock()
		return nil
	}

	progress := pt.progress
	metadata := pt.metadata
	jobID := pt.jobID
	pt.dirty = false
	pt.lastFlush = time.Now()
	pt.mu.Unlock()

	return pt.writeQueue.Submit(ctx, func(db *sql.DB) error {
		var metadataJSON *string
		if metadata != nil {
			bytes, err := json.Marshal(metadata)
			if err != nil {
				return err
			}
			str := string(bytes)
			metadataJSON = &str
		}

		now := time.Now().Unix()

		_, err := db.Exec(`
			UPDATE index_jobs
			SET progress = ?, metadata = ?, updated_at = ?
			WHERE id = ?
		`, progress, metadataJSON, now, jobID)

		return err
	})
}

// GetProgress returns the current progress value (from memory, not database)
func (pt *ProgressTracker) GetProgress() int {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.progress
}

// GetMetadata returns the current metadata (from memory, not database)
func (pt *ProgressTracker) GetMetadata() *IndexJobMetadata {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if pt.metadata == nil {
		return nil
	}
	// Return a copy to avoid race conditions
	copy := *pt.metadata
	return &copy
}

// JobID returns the job ID being tracked
func (pt *ProgressTracker) JobID() int64 {
	return pt.jobID
}

// IsDirty returns whether there are unflushed updates
func (pt *ProgressTracker) IsDirty() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.dirty
}

// SetFlushInterval updates the flush interval (useful for testing)
func (pt *ProgressTracker) SetFlushInterval(interval time.Duration) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.flushInterval = interval
}
