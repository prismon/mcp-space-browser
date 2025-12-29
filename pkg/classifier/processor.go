package classifier

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
)

var processorLog = logger.WithName("classifier-processor")

// ProcessorConfig contains configuration for the classifier processor
type ProcessorConfig struct {
	CacheDir          string
	ClassifierManager *Manager
	MetadataManager   *MetadataManager
	Database          *database.DiskDB
}

// Processor handles resource fetching and classifier execution
type Processor struct {
	config *ProcessorConfig
}

// NewProcessor creates a new classifier processor
func NewProcessor(config *ProcessorConfig) *Processor {
	return &Processor{config: config}
}

// SetDatabase sets the database for storing artifact metadata.
// This is used in multi-project mode where the database is resolved at runtime.
func (p *Processor) SetDatabase(db *database.DiskDB) {
	p.config.Database = db
}

// ProcessRequest represents a request to process a resource
type ProcessRequest struct {
	ResourceURL   string
	ArtifactTypes []string // e.g., ["thumbnail", "timeline", "metadata"]
}

// ProcessResult contains the results of processing a resource
type ProcessResult struct {
	Artifacts []database.ClassifierArtifact
	Errors    []string
}

// ProcessResource processes a resource and generates artifacts
func (p *Processor) ProcessResource(req *ProcessRequest) (*ProcessResult, error) {
	processorLog.WithField("resource", req.ResourceURL).Info("Processing resource")

	// Resolve the resource to a local file path
	localPath, cleanup, err := p.resolveResource(req.ResourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve resource: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Detect media type
	mediaType := DetectMediaType(localPath)
	if mediaType == MediaTypeUnknown {
		return nil, fmt.Errorf("unsupported media type for file: %s", localPath)
	}

	// Get file info for hash generation
	stat, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Generate hash key
	hashKey := p.generateHashKey(localPath, stat.ModTime().Unix())

	result := &ProcessResult{
		Artifacts: make([]database.ClassifierArtifact, 0),
		Errors:    make([]string, 0),
	}

	// Determine which artifact types to generate
	artifactTypes := req.ArtifactTypes
	if len(artifactTypes) == 0 {
		// Default: generate all applicable artifacts based on media type
		if mediaType == MediaTypeImage {
			artifactTypes = []string{"thumbnail", "metadata"}
		} else if mediaType == MediaTypeVideo {
			artifactTypes = []string{"thumbnail", "timeline", "metadata"}
		} else {
			artifactTypes = []string{"metadata"}
		}
	}

	// Process each artifact type
	for _, artifactType := range artifactTypes {
		switch artifactType {
		case "thumbnail":
			if artifact, err := p.generateThumbnail(localPath, mediaType, hashKey); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("thumbnail generation failed: %v", err))
			} else {
				result.Artifacts = append(result.Artifacts, *artifact)
			}

		case "timeline":
			if mediaType == MediaTypeVideo {
				artifacts, err := p.generateTimeline(localPath, hashKey, 5)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("timeline generation failed: %v", err))
				} else {
					result.Artifacts = append(result.Artifacts, artifacts...)
				}
			}

		case "metadata":
			if p.config.MetadataManager != nil && p.config.MetadataManager.CanExtractMetadata(localPath) {
				if artifact, err := p.extractMetadata(localPath, hashKey); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("metadata extraction failed: %v", err))
				} else if artifact != nil {
					result.Artifacts = append(result.Artifacts, *artifact)
				}
			}
		}
	}

	// Store artifacts as features in database for discovery via MCP resources
	if p.config.Database != nil {
		for _, artifact := range result.Artifacts {
			// Get file size of cached artifact
			var fileSize int64
			if info, err := os.Stat(artifact.CachePath); err == nil {
				fileSize = info.Size()
			}

			// Serialize metadata map to JSON
			var dataJson *string
			if artifact.Metadata != nil {
				if jsonBytes, err := json.Marshal(artifact.Metadata); err == nil {
					jsonStr := string(jsonBytes)
					dataJson = &jsonStr
				}
			}

			// Create Feature from artifact
			feature := &models.Feature{
				EntryPath:   localPath,
				FeatureType: artifact.Type,
				Hash:        artifact.Hash,
				MimeType:    &artifact.MimeType,
				CachePath:   &artifact.CachePath,
				DataJson:    dataJson,
				FileSize:    fileSize,
				Generator:   artifact.Generator,
			}

			if err := p.config.Database.CreateOrUpdateFeature(feature); err != nil {
				processorLog.WithError(err).WithField("hash", artifact.Hash).Warn("Failed to store feature in database")
			}
		}
	}

	return result, nil
}

// resolveResource resolves a resource URL to a local file path
// Returns the local path, a cleanup function (if temporary file), and an error
func (p *Processor) resolveResource(resourceURL string) (string, func(), error) {
	// Parse the URL
	u, err := url.Parse(resourceURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid resource URL: %w", err)
	}

	switch u.Scheme {
	case "file", "":
		// Local file path
		path := u.Path
		if u.Scheme == "" {
			path = resourceURL
		}
		expandedPath, err := pathutil.ExpandPath(path)
		if err != nil {
			return "", nil, fmt.Errorf("failed to expand path: %w", err)
		}
		if err := pathutil.ValidatePath(expandedPath); err != nil {
			return "", nil, fmt.Errorf("invalid path: %w", err)
		}
		return expandedPath, nil, nil

	case "http", "https":
		// Download to temporary file
		return p.downloadResource(resourceURL)

	case "synthesis":
		// Resolve synthesis:// resource
		return p.resolveSynthesisResource(resourceURL)

	default:
		return "", nil, fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}
}

// downloadResource downloads a remote resource to a temporary file
func (p *Processor) downloadResource(resourceURL string) (string, func(), error) {
	processorLog.WithField("url", resourceURL).Info("Downloading remote resource")

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "classifier-download-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		tmpFile.Close()
		os.Remove(tmpPath)
		processorLog.WithField("path", tmpPath).Debug("Cleaned up temporary file")
	}

	// Download the resource
	resp, err := http.Get(resourceURL)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to download resource: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cleanup()
		return "", nil, fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Copy to temporary file
	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write downloaded data: %w", err)
	}

	processorLog.WithFields(map[string]interface{}{
		"url":   resourceURL,
		"path":  tmpPath,
		"bytes": written,
	}).Info("Successfully downloaded resource")

	return tmpPath, cleanup, nil
}

// resolveSynthesisResource resolves a synthesis:// resource to a local file path
func (p *Processor) resolveSynthesisResource(resourceURL string) (string, func(), error) {
	// Parse synthesis:// URL
	// Format: synthesis://nodes/<path> or synthesis://metadata/<hash>
	u, err := url.Parse(resourceURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid synthesis URL: %w", err)
	}

	// Extract path from URL
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 2 {
		return "", nil, fmt.Errorf("invalid synthesis URL format: %s", resourceURL)
	}

	resourceType := parts[0]
	resourcePath := parts[1]

	switch resourceType {
	case "nodes":
		// Direct file path
		expandedPath, err := pathutil.ExpandPath(resourcePath)
		if err != nil {
			return "", nil, fmt.Errorf("failed to expand path: %w", err)
		}
		if err := pathutil.ValidatePath(expandedPath); err != nil {
			return "", nil, fmt.Errorf("invalid path: %w", err)
		}
		return expandedPath, nil, nil

	case "metadata":
		// Resolve metadata hash to source path
		if p.config.Database != nil {
			metadata, err := p.config.Database.GetMetadata(resourcePath)
			if err != nil || metadata == nil {
				return "", nil, fmt.Errorf("metadata not found for hash: %s", resourcePath)
			}
			return metadata.SourcePath, nil, nil
		}
		return "", nil, fmt.Errorf("database not available for metadata lookup")

	default:
		return "", nil, fmt.Errorf("unsupported synthesis resource type: %s", resourceType)
	}
}

// generateHashKey generates a hash key for artifact caching
func (p *Processor) generateHashKey(path string, mtime int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", path, mtime)))
	return hex.EncodeToString(sum[:])
}

// artifactCachePath returns the cache path for an artifact
func (p *Processor) artifactCachePath(hashKey, filename string) (string, error) {
	if len(hashKey) < 4 {
		return "", fmt.Errorf("invalid hash key for artifact cache")
	}

	dir := filepath.Join(p.config.CacheDir, hashKey[:2], hashKey[2:4], hashKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	return filepath.Join(dir, filename), nil
}

// generateThumbnail generates a thumbnail artifact
func (p *Processor) generateThumbnail(path string, mediaType MediaType, hashKey string) (*database.ClassifierArtifact, error) {
	cachePath, err := p.artifactCachePath(hashKey, "thumb.jpg")
	if err != nil {
		return nil, err
	}

	// Check if already exists
	if _, err := os.Stat(cachePath); err == nil {
		processorLog.WithField("hash", hashKey).Debug("Thumbnail already exists in cache")
		// Determine generator from media type (cached files don't have generator info)
		generator := "go-image"
		if mediaType == MediaTypeVideo {
			generator = "ffmpeg"
		}
		return &database.ClassifierArtifact{
			Type:        "thumbnail",
			Hash:        hashKey,
			MimeType:    "image/jpeg",
			CachePath:   cachePath,
			ResourceURI: fmt.Sprintf("synthesis://metadata/%s", hashKey),
			Generator:   generator,
		}, nil
	}

	req := &ArtifactRequest{
		SourcePath:   path,
		OutputPath:   cachePath,
		MediaType:    mediaType,
		ArtifactType: ArtifactTypeThumbnail,
		MaxWidth:     320,
		MaxHeight:    320,
	}

	result := p.config.ClassifierManager.GenerateThumbnail(req)
	if result.Error != nil {
		return nil, result.Error
	}

	return &database.ClassifierArtifact{
		Type:        "thumbnail",
		Hash:        hashKey,
		MimeType:    result.MimeType,
		CachePath:   result.OutputPath,
		ResourceURI: fmt.Sprintf("synthesis://metadata/%s", hashKey),
		Generator:   result.Generator,
	}, nil
}

// generateTimeline generates timeline frame artifacts
func (p *Processor) generateTimeline(path string, hashKey string, frames int) ([]database.ClassifierArtifact, error) {
	artifacts := make([]database.ClassifierArtifact, 0, frames)

	for i := 0; i < frames; i++ {
		framePath, err := p.artifactCachePath(hashKey, fmt.Sprintf("timeline_%02d.jpg", i))
		if err != nil {
			return nil, err
		}

		var generator string
		// Check if already exists
		if _, err := os.Stat(framePath); os.IsNotExist(err) {
			req := &ArtifactRequest{
				SourcePath:   path,
				OutputPath:   framePath,
				MediaType:    MediaTypeVideo,
				ArtifactType: ArtifactTypeTimeline,
				FrameIndex:   i,
				TotalFrames:  frames,
				MaxWidth:     320,
				MaxHeight:    320,
			}

			result := p.config.ClassifierManager.GenerateTimelineFrame(req)
			if result.Error != nil {
				return nil, result.Error
			}
			generator = result.Generator
		} else {
			// Default for cached files
			generator = "ffmpeg"
		}

		frameHash := fmt.Sprintf("%s-frame-%d", hashKey, i)
		artifacts = append(artifacts, database.ClassifierArtifact{
			Type:        "video-timeline",
			Hash:        frameHash,
			MimeType:    "image/jpeg",
			CachePath:   framePath,
			ResourceURI: fmt.Sprintf("synthesis://metadata/%s", frameHash),
			Generator:   generator,
			Metadata: map[string]any{
				"frame": i,
			},
		})
	}

	return artifacts, nil
}

// extractMetadata extracts metadata from a file
func (p *Processor) extractMetadata(path string, hashKey string) (*database.ClassifierArtifact, error) {
	result := p.config.MetadataManager.ExtractMetadata(path, 0)
	if result.Error != nil {
		return nil, result.Error
	}

	return &database.ClassifierArtifact{
		Type:        result.MetadataType,
		Hash:        hashKey,
		MimeType:    "application/json",
		CachePath:   path, // No cache file for metadata
		ResourceURI: fmt.Sprintf("synthesis://nodes/%s/metadata/%s", path, result.MetadataType),
		Metadata:    result.Data,
	}, nil
}
