package source

import (
	"sync"
	"sync/atomic"
	"time"
)

// ProgressTracker tracks indexing progress and provides estimates
type ProgressTracker struct {
	mu sync.RWMutex

	estimate ProgressEstimate

	// Atomic counters for thread-safe updates
	processedItems       atomic.Int64
	filesProcessed       atomic.Int64
	directoriesProcessed atomic.Int64
	totalSize            atomic.Int64
	errors               atomic.Int64
	queuedItems          atomic.Int64
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(totalItems int64) *ProgressTracker {
	return &ProgressTracker{
		estimate: ProgressEstimate{
			Phase:      "estimating",
			TotalItems: totalItems,
			StartTime:  time.Now(),
		},
	}
}

// SetPhase updates the current phase
func (pt *ProgressTracker) SetPhase(phase string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.estimate.Phase = phase
}

// SetTotalItems updates the total items estimate
func (pt *ProgressTracker) SetTotalItems(total int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.estimate.TotalItems = total
}

// IncrementProcessed increments the processed items counter
func (pt *ProgressTracker) IncrementProcessed() {
	pt.processedItems.Add(1)
}

// IncrementFiles increments the files counter
func (pt *ProgressTracker) IncrementFiles(size int64) {
	pt.filesProcessed.Add(1)
	pt.totalSize.Add(size)
	pt.processedItems.Add(1)
}

// IncrementDirectories increments the directories counter
func (pt *ProgressTracker) IncrementDirectories() {
	pt.directoriesProcessed.Add(1)
	pt.processedItems.Add(1)
}

// IncrementErrors increments the error counter
func (pt *ProgressTracker) IncrementErrors() {
	pt.errors.Add(1)
}

// SetQueuedItems sets the number of queued items
func (pt *ProgressTracker) SetQueuedItems(count int64) {
	pt.queuedItems.Store(count)
}

// SetCurrentPath sets the current path being processed
func (pt *ProgressTracker) SetCurrentPath(path string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.estimate.CurrentPath = path
}

// GetEstimate returns the current progress estimate (snapshot)
func (pt *ProgressTracker) GetEstimate() ProgressEstimate {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	// Create a snapshot with current atomic values
	estimate := pt.estimate
	estimate.ProcessedItems = pt.processedItems.Load()
	estimate.FilesProcessed = pt.filesProcessed.Load()
	estimate.DirectoriesProcessed = pt.directoriesProcessed.Load()
	estimate.TotalSize = pt.totalSize.Load()
	estimate.Errors = pt.errors.Load()
	estimate.QueuedItems = pt.queuedItems.Load()

	return estimate
}

// GetStats returns basic stats (without locking)
func (pt *ProgressTracker) GetStats() (filesProcessed, directoriesProcessed, totalSize, errors int64) {
	return pt.filesProcessed.Load(),
		pt.directoriesProcessed.Load(),
		pt.totalSize.Load(),
		pt.errors.Load()
}

// GetPercentComplete returns the current completion percentage
func (pt *ProgressTracker) GetPercentComplete() int {
	estimate := pt.GetEstimate()
	return estimate.PercentComplete()
}

// Reset resets all counters
func (pt *ProgressTracker) Reset() {
	pt.processedItems.Store(0)
	pt.filesProcessed.Store(0)
	pt.directoriesProcessed.Store(0)
	pt.totalSize.Store(0)
	pt.errors.Store(0)
	pt.queuedItems.Store(0)

	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.estimate.Phase = "estimating"
	pt.estimate.StartTime = time.Now()
	pt.estimate.CurrentPath = ""
}
