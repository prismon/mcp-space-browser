package sources

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
	log = logger.WithName("sources")
}

// FileSystemSource implements DataSource for local filesystem access
type FileSystemSource struct {
	maxEstimationTime    time.Duration
	estimationSampleRate float64
}

// NewFileSystemSource creates a new filesystem source
func NewFileSystemSource() *FileSystemSource {
	return &FileSystemSource{
		maxEstimationTime:    5 * time.Second,
		estimationSampleRate: 0.2,
	}
}

// Name returns the source type name
func (fss *FileSystemSource) Name() string {
	return "filesystem"
}

// Stat returns information about a file or directory
// Uses Lstat to not follow symlinks - this prevents double-counting when symlinks
// point to directories that are also indexed directly
func (fss *FileSystemSource) Stat(ctx context.Context, path string) (ItemInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("skipping symlink: %s", path)
	}
	return &fileSystemItemInfo{
		path: path,
		info: info,
	}, nil
}

// ReadDir reads directory contents
func (fss *FileSystemSource) ReadDir(ctx context.Context, path string) ([]DataDirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	result := make([]DataDirEntry, len(entries))
	for i, entry := range entries {
		result[i] = &fileSystemDirEntry{
			name:  entry.Name(),
			entry: entry,
		}
	}
	return result, nil
}

// EstimateSize estimates the total number of items under a path
func (fss *FileSystemSource) EstimateSize(ctx context.Context, root string) (int64, error) {
	startTime := time.Now()
	var totalEstimate atomic.Int64
	var dirsScanned atomic.Int64
	var filesScanned atomic.Int64

	err := fss.estimateRecursive(ctx, root, 0, &totalEstimate, &dirsScanned, &filesScanned, startTime)
	if err != nil {
		log.WithError(err).Warn("Error during estimation, using partial estimate")
	}

	estimate := totalEstimate.Load()
	if estimate == 0 {
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

func (fss *FileSystemSource) estimateRecursive(
	ctx context.Context,
	path string,
	depth int,
	totalEstimate *atomic.Int64,
	dirsScanned *atomic.Int64,
	filesScanned *atomic.Int64,
	startTime time.Time,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if time.Since(startTime) > fss.maxEstimationTime {
		log.Debug("Estimation time limit reached, extrapolating from sample")
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
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

	totalEstimate.Add(int64(len(entries)))
	dirsScanned.Add(1)
	filesScanned.Add(int64(fileCount))

	sampleRate := fss.estimationSampleRate
	if depth < 3 {
		sampleRate = 1.0
	} else if depth > 6 {
		sampleRate = 0.05
	}

	sampledCount := 0
	for i, subdir := range subdirs {
		if depth >= 3 && float64(i)/float64(len(subdirs)) > sampleRate {
			continue
		}

		sampledCount++
		subpath := filepath.Join(path, subdir)
		if err := fss.estimateRecursive(ctx, subpath, depth+1, totalEstimate, dirsScanned, filesScanned, startTime); err != nil {
			return err
		}
	}

	if sampledCount > 0 && sampledCount < len(subdirs) {
		multiplier := float64(len(subdirs)) / float64(sampledCount)
		extrapolated := int64(float64(filesScanned.Load()) * (multiplier - 1.0))
		totalEstimate.Add(extrapolated)
	}

	return nil
}

// Close releases resources (no-op for filesystem)
func (fss *FileSystemSource) Close() error {
	return nil
}

// fileSystemItemInfo implements ItemInfo
type fileSystemItemInfo struct {
	path string
	info fs.FileInfo
}

func (i *fileSystemItemInfo) Path() string      { return i.path }
func (i *fileSystemItemInfo) Size() int64        { return i.info.Size() }
func (i *fileSystemItemInfo) IsDir() bool        { return i.info.IsDir() }
func (i *fileSystemItemInfo) ModTime() time.Time { return i.info.ModTime() }
func (i *fileSystemItemInfo) Mode() fs.FileMode  { return i.info.Mode() }

func (i *fileSystemItemInfo) Blocks() int64 {
	if sys := i.info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			return stat.Blocks * 512
		}
	}
	return i.info.Size()
}

// fileSystemDirEntry implements DataDirEntry
type fileSystemDirEntry struct {
	name  string
	entry fs.DirEntry
}

func (e *fileSystemDirEntry) Name() string      { return e.name }
func (e *fileSystemDirEntry) IsDir() bool        { return e.entry.IsDir() }
func (e *fileSystemDirEntry) Type() fs.FileMode  { return e.entry.Type() }

// GetFullPath returns the full path for a directory entry
func GetFullPath(parentPath string, entry DataDirEntry) string {
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
