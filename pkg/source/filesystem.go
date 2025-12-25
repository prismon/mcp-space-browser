package source

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("source")
}

// FileSystemSource implements Source for local filesystem access
type FileSystemSource struct {
	// Config
	maxEstimationTime time.Duration
	estimationSampleRate float64 // What fraction of dirs to sample during estimation (0.1 = 10%)
}

// NewFileSystemSource creates a new filesystem source
func NewFileSystemSource() *FileSystemSource {
	return &FileSystemSource{
		maxEstimationTime: 5 * time.Second, // Max time to spend on estimation
		estimationSampleRate: 0.2, // Sample 20% of directories for estimation
	}
}

// Name returns the source type name
func (fs *FileSystemSource) Name() string {
	return "filesystem"
}

// Stat returns information about a file or directory
// Uses Lstat to not follow symlinks - this prevents double-counting when symlinks
// point to directories that are also indexed directly (e.g., macOS container symlinks)
func (fs *FileSystemSource) Stat(ctx context.Context, path string) (ItemInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	// Skip symlinks entirely - they should not be indexed as files/directories
	// because their targets are indexed directly
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("skipping symlink: %s", path)
	}
	return &fileSystemItemInfo{
		path: path,
		info: info,
	}, nil
}

// ReadDir reads directory contents
func (fs *FileSystemSource) ReadDir(ctx context.Context, path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	result := make([]DirEntry, len(entries))
	for i, entry := range entries {
		result[i] = &fileSystemDirEntry{
			name:  entry.Name(),
			entry: entry,
		}
	}
	return result, nil
}

// EstimateSize estimates the total number of items under a path
// Uses a sampling approach to avoid scanning the entire tree
func (fs *FileSystemSource) EstimateSize(ctx context.Context, root string) (int64, error) {
	startTime := time.Now()
	var totalEstimate atomic.Int64
	var dirsScanned atomic.Int64
	var filesScanned atomic.Int64

	// Quick sampling-based estimation
	err := fs.estimateRecursive(ctx, root, 0, &totalEstimate, &dirsScanned, &filesScanned, startTime)
	if err != nil {
		log.WithError(err).Warn("Error during estimation, using partial estimate")
	}

	estimate := totalEstimate.Load()
	if estimate == 0 {
		// If we couldn't estimate, return a reasonable default
		estimate = 1000
	}

	log.WithFields(logrus.Fields{
		"root":         root,
		"estimate":     estimate,
		"dirsScanned":  dirsScanned.Load(),
		"filesScanned": filesScanned.Load(),
		"duration":     time.Since(startTime),
	}).Info("Completed size estimation")

	return estimate, nil
}

// estimateRecursive performs recursive estimation with sampling
func (fs *FileSystemSource) estimateRecursive(
	ctx context.Context,
	path string,
	depth int,
	totalEstimate *atomic.Int64,
	dirsScanned *atomic.Int64,
	filesScanned *atomic.Int64,
	startTime time.Time,
) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Stop if we've exceeded max estimation time
	if time.Since(startTime) > fs.maxEstimationTime {
		log.Debug("Estimation time limit reached, extrapolating from sample")
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		// Permission errors are common, just skip
		if logger.IsLevelEnabled(logrus.TraceLevel) {
			log.WithError(err).WithField("path", path).Trace("Cannot read directory for estimation")
		}
		return nil
	}

	var subdirs []string
	fileCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			subdirs = append(subdirs, entry.Name())
		} else {
			fileCount++
		}
	}

	// Add counts
	totalEstimate.Add(int64(len(entries)))
	dirsScanned.Add(1)
	filesScanned.Add(int64(fileCount))

	// Sample subdirectories based on depth
	// At shallow depths, scan everything; at deeper depths, sample more aggressively
	sampleRate := fs.estimationSampleRate
	if depth < 3 {
		sampleRate = 1.0 // Scan all subdirs at shallow depths
	} else if depth > 6 {
		sampleRate = 0.05 // Only 5% at deep depths
	}

	sampledCount := 0
	for i, subdir := range subdirs {
		// Deterministic sampling based on position
		if depth >= 3 && float64(i)/float64(len(subdirs)) > sampleRate {
			continue
		}

		sampledCount++
		subpath := filepath.Join(path, subdir)
		if err := fs.estimateRecursive(ctx, subpath, depth+1, totalEstimate, dirsScanned, filesScanned, startTime); err != nil {
			return err
		}
	}

	// Extrapolate for unsampled directories
	if sampledCount > 0 && sampledCount < len(subdirs) {
		// Multiply the estimate to account for unsampled dirs
		multiplier := float64(len(subdirs)) / float64(sampledCount)
		extrapolated := int64(float64(filesScanned.Load()) * (multiplier - 1.0))
		totalEstimate.Add(extrapolated)

		if logger.IsLevelEnabled(logrus.TraceLevel) {
			log.WithFields(logrus.Fields{
				"path":          path,
				"subdirs":       len(subdirs),
				"sampled":       sampledCount,
				"multiplier":    multiplier,
				"extrapolated":  extrapolated,
			}).Trace("Extrapolated estimate")
		}
	}

	return nil
}

// Close releases resources (no-op for filesystem)
func (fs *FileSystemSource) Close() error {
	return nil
}

// fileSystemItemInfo implements ItemInfo
type fileSystemItemInfo struct {
	path string
	info fs.FileInfo
}

func (i *fileSystemItemInfo) Path() string {
	return i.path
}

func (i *fileSystemItemInfo) Size() int64 {
	return i.info.Size()
}

func (i *fileSystemItemInfo) Blocks() int64 {
	// Get disk usage from st_blocks (each block is 512 bytes)
	if sys := i.info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			// st_blocks is in 512-byte units, convert to bytes
			return stat.Blocks * 512
		}
	}
	// Fallback to logical size if syscall info unavailable
	return i.info.Size()
}

func (i *fileSystemItemInfo) IsDir() bool {
	return i.info.IsDir()
}

func (i *fileSystemItemInfo) ModTime() time.Time {
	return i.info.ModTime()
}

func (i *fileSystemItemInfo) Mode() fs.FileMode {
	return i.info.Mode()
}

// fileSystemDirEntry implements DirEntry
type fileSystemDirEntry struct {
	name  string
	entry fs.DirEntry
}

func (e *fileSystemDirEntry) Name() string {
	return e.name
}

func (e *fileSystemDirEntry) IsDir() bool {
	return e.entry.IsDir()
}

func (e *fileSystemDirEntry) Type() fs.FileMode {
	return e.entry.Type()
}

// GetFullPath returns the full path for a directory entry
func GetFullPath(parentPath string, entry DirEntry) string {
	return filepath.Join(parentPath, entry.Name())
}

// ValidatePath validates that a path is absolute and exists
func ValidatePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("path does not exist or is not accessible: %w", err)
	}

	return abs, nil
}
