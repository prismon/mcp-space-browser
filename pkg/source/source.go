package source

import (
	"context"
	"io/fs"
	"time"
)

// Source represents an abstract data source that can be indexed
// This allows the crawler to work with filesystems, cloud storage, archives, etc.
type Source interface {
	// Stat returns information about the item at the given path
	Stat(ctx context.Context, path string) (ItemInfo, error)

	// ReadDir reads the directory named by path and returns a list of directory entries
	ReadDir(ctx context.Context, path string) ([]DirEntry, error)

	// EstimateSize estimates the total number of items to be indexed under the given path
	// This is used for progress tracking. Returns (estimatedCount, error)
	// The estimation can be quick and approximate - it doesn't need to be exact
	EstimateSize(ctx context.Context, path string) (int64, error)

	// Name returns a human-readable name for this source type
	Name() string

	// Close releases any resources held by the source
	Close() error
}

// ItemInfo represents metadata about a file or directory
type ItemInfo interface {
	// Path returns the full path to the item
	Path() string

	// Size returns the size in bytes (for files) or 0 (for directories)
	Size() int64

	// IsDir reports whether the item is a directory
	IsDir() bool

	// ModTime returns the modification time
	ModTime() time.Time

	// Mode returns the file mode bits
	Mode() fs.FileMode
}

// DirEntry represents an entry in a directory listing
type DirEntry interface {
	// Name returns the name of the file (without path)
	Name() string

	// IsDir reports whether the entry describes a directory
	IsDir() bool

	// Type returns the type bits for the entry
	Type() fs.FileMode
}

// ProgressEstimate contains information about indexing progress
type ProgressEstimate struct {
	// Phase describes the current indexing phase
	Phase string

	// TotalItems is the estimated total number of items to process
	TotalItems int64

	// ProcessedItems is the number of items processed so far
	ProcessedItems int64

	// FilesProcessed is the number of files processed
	FilesProcessed int64

	// DirectoriesProcessed is the number of directories processed
	DirectoriesProcessed int64

	// TotalSize is the total size in bytes processed
	TotalSize int64

	// Errors is the number of errors encountered
	Errors int64

	// QueuedItems is the number of items waiting to be processed
	QueuedItems int64

	// StartTime is when the indexing started
	StartTime time.Time

	// CurrentPath is the current path being processed (for debugging)
	CurrentPath string
}

// PercentComplete calculates the percentage complete based on the current phase
func (p *ProgressEstimate) PercentComplete() int {
	switch p.Phase {
	case "estimating":
		// Estimation phase: 0-5%
		return 0

	case "crawling":
		// Crawling phase: 5-85%
		if p.TotalItems == 0 {
			return 5
		}
		ratio := float64(p.ProcessedItems) / float64(p.TotalItems)
		progress := 5 + int(ratio*80)
		if progress > 85 {
			progress = 85
		}
		return progress

	case "cleanup":
		// Cleanup phase: 85-90%
		return 87

	case "aggregation":
		// Aggregation phase: 90-100%
		return 95

	case "complete":
		return 100

	default:
		return 0
	}
}

// ElapsedTime returns the time elapsed since the start
func (p *ProgressEstimate) ElapsedTime() time.Duration {
	return time.Since(p.StartTime)
}

// EstimatedTimeRemaining estimates the time remaining based on current progress
func (p *ProgressEstimate) EstimatedTimeRemaining() time.Duration {
	if p.ProcessedItems == 0 || p.TotalItems == 0 {
		return 0
	}

	elapsed := p.ElapsedTime()
	ratio := float64(p.ProcessedItems) / float64(p.TotalItems)
	if ratio == 0 {
		return 0
	}

	totalEstimated := time.Duration(float64(elapsed) / ratio)
	return totalEstimated - elapsed
}
