package server

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
)

var ppLog = logger.WithName("post-processor")

// DefaultAttributes are extracted automatically when no filter is specified.
var DefaultAttributes = []string{
	"thumbnail",
	"video.thumbnails",
	"mime",
	"metadata",
	"permissions",
}

// PostProcessConfig configures post-scan attribute extraction.
type PostProcessConfig struct {
	DB       *database.DiskDB
	CacheDir string
	// Attributes to extract. If empty, DefaultAttributes are used.
	// Acts as a filter: only listed attributes are extracted.
	Attributes []string
}

// PostProcessResult contains stats from post-processing.
type PostProcessResult struct {
	FilesProcessed int64         `json:"files_processed"`
	FeaturesCreated int64       `json:"features_created"`
	AttributesSet  int64        `json:"attributes_set"`
	Errors         int64        `json:"errors"`
	Duration       time.Duration `json:"duration_ms"`
}

// PostProcess runs attribute extraction and thumbnail generation for files
// under the given root paths.
func PostProcess(config *PostProcessConfig, roots []string) *PostProcessResult {
	start := time.Now()
	result := &PostProcessResult{}

	if config.DB == nil {
		ppLog.Warn("No database configured for post-processing")
		return result
	}

	// Determine which attributes to extract
	attrSet := make(map[string]bool)
	attrs := config.Attributes
	if len(attrs) == 0 {
		attrs = DefaultAttributes
	}
	for _, a := range attrs {
		attrSet[a] = true
	}

	// Collect files from all roots
	var allFiles []*models.Entry
	for _, root := range roots {
		files, err := config.DB.GetFilesUnderRoot(root)
		if err != nil {
			ppLog.WithError(err).WithField("root", root).Error("Failed to get files under root")
			atomic.AddInt64(&result.Errors, 1)
			continue
		}
		allFiles = append(allFiles, files...)
	}

	if len(allFiles) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	// Initialize classifier infrastructure if needed for thumbnail/video/metadata
	needsClassifier := attrSet["thumbnail"] || attrSet["video.thumbnails"] || attrSet["metadata"]
	var proc *classifier.Processor
	if needsClassifier && config.CacheDir != "" {
		if err := initArtifactCache(); err != nil {
			ppLog.WithError(err).Warn("Failed to init artifact cache, skipping classifier-based attributes")
			needsClassifier = false
		} else {
			proc = classifier.NewProcessor(&classifier.ProcessorConfig{
				CacheDir:          config.CacheDir,
				ClassifierManager: classifierManager,
				MetadataManager:   metadataManager,
				Database:          config.DB,
			})
		}
	}

	// Worker pool
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}

	fileCh := make(chan *models.Entry, workers*2)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range fileCh {
				fc, ac, errs := processFile(entry, attrSet, config, proc, needsClassifier)
				atomic.AddInt64(&result.FeaturesCreated, int64(fc))
				atomic.AddInt64(&result.AttributesSet, int64(ac))
				atomic.AddInt64(&result.Errors, int64(errs))
				atomic.AddInt64(&result.FilesProcessed, 1)
			}
		}()
	}

	for _, f := range allFiles {
		fileCh <- f
	}
	close(fileCh)
	wg.Wait()

	result.Duration = time.Since(start)

	ppLog.WithFields(map[string]interface{}{
		"files":      result.FilesProcessed,
		"features":   result.FeaturesCreated,
		"attributes": result.AttributesSet,
		"errors":     result.Errors,
		"duration":   result.Duration.String(),
	}).Info("Post-processing complete")

	return result
}

// processFile extracts attributes for a single file.
// Returns (features created, attributes set, error count).
func processFile(entry *models.Entry, attrSet map[string]bool, config *PostProcessConfig, proc *classifier.Processor, needsClassifier bool) (int, int, int) {
	featuresCreated := 0
	attributesSet := 0
	errors := 0
	now := time.Now().Unix()

	// MIME detection
	if attrSet["mime"] {
		mimeType, err := detectMIME(entry.Path)
		if err != nil {
			ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to detect MIME type")
			errors++
		} else {
			if err := config.DB.SetAttribute(&models.Attribute{
				EntryPath:  entry.Path,
				Key:        "mime",
				Value:      mimeType,
				Source:     models.AttributeSourceScan,
				ComputedAt: now,
			}); err != nil {
				ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to set mime attribute")
				errors++
			} else {
				attributesSet++
			}
		}
	}

	// Permissions
	if attrSet["permissions"] {
		info, err := os.Stat(entry.Path)
		if err != nil {
			ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to stat file for permissions")
			errors++
		} else {
			perm := fmt.Sprintf("%04o", info.Mode().Perm())
			if err := config.DB.SetAttribute(&models.Attribute{
				EntryPath:  entry.Path,
				Key:        "permissions",
				Value:      perm,
				Source:     models.AttributeSourceScan,
				ComputedAt: now,
			}); err != nil {
				ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to set permissions attribute")
				errors++
			} else {
				attributesSet++
			}
		}
	}

	// Thumbnail (images)
	if attrSet["thumbnail"] && needsClassifier && proc != nil {
		mediaType := classifier.DetectMediaType(entry.Path)
		if mediaType == classifier.MediaTypeImage {
			res, err := proc.ProcessResource(&classifier.ProcessRequest{
				ResourceURL:   entry.Path,
				ArtifactTypes: []string{"thumbnail"},
			})
			if err != nil {
				ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to generate thumbnail")
				errors++
			} else {
				featuresCreated += len(res.Artifacts)
				errors += len(res.Errors)
			}
		}
	}

	// Video thumbnails (poster + timeline)
	if attrSet["video.thumbnails"] && needsClassifier && proc != nil {
		mediaType := classifier.DetectMediaType(entry.Path)
		if mediaType == classifier.MediaTypeVideo {
			res, err := proc.ProcessResource(&classifier.ProcessRequest{
				ResourceURL:   entry.Path,
				ArtifactTypes: []string{"thumbnail", "timeline"},
			})
			if err != nil {
				ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to generate video thumbnails")
				errors++
			} else {
				featuresCreated += len(res.Artifacts)
				errors += len(res.Errors)
			}
		}
	}

	// Metadata extraction (audio, text)
	if attrSet["metadata"] && needsClassifier && proc != nil {
		if metadataManager != nil && metadataManager.CanExtractMetadata(entry.Path) {
			res, err := proc.ProcessResource(&classifier.ProcessRequest{
				ResourceURL:   entry.Path,
				ArtifactTypes: []string{"metadata"},
			})
			if err != nil {
				ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to extract metadata")
				errors++
			} else {
				featuresCreated += len(res.Artifacts)
				errors += len(res.Errors)
			}
		}
	}

	// Hash MD5 (opt-in only)
	if attrSet["hash.md5"] {
		hash, err := computeHash(entry.Path, "md5")
		if err != nil {
			ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to compute MD5 hash")
			errors++
		} else {
			if err := config.DB.SetAttribute(&models.Attribute{
				EntryPath:  entry.Path,
				Key:        "hash.md5",
				Value:      hash,
				Source:     models.AttributeSourceScan,
				ComputedAt: now,
			}); err != nil {
				errors++
			} else {
				attributesSet++
			}
		}
	}

	// Hash SHA256 (opt-in only)
	if attrSet["hash.sha256"] {
		hash, err := computeHash(entry.Path, "sha256")
		if err != nil {
			ppLog.WithError(err).WithField("path", entry.Path).Debug("Failed to compute SHA256 hash")
			errors++
		} else {
			if err := config.DB.SetAttribute(&models.Attribute{
				EntryPath:  entry.Path,
				Key:        "hash.sha256",
				Value:      hash,
				Source:     models.AttributeSourceScan,
				ComputedAt: now,
			}); err != nil {
				errors++
			} else {
				attributesSet++
			}
		}
	}

	return featuresCreated, attributesSet, errors
}

// detectMIME reads the first 512 bytes of a file and detects MIME type.
func detectMIME(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

// computeHash computes a file hash using the specified algorithm.
func computeHash(path string, algo string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	switch algo {
	case "md5":
		h := md5.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	case "sha256":
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algo)
	}
}
