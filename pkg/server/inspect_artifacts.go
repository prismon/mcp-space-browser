package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
)

const (
	artifactTimelineN  = 5
	thumbnailMaxWidth  = 320
	thumbnailMaxHeight = 320
)

var (
	contentBaseURL      string
	artifactCacheDir    string
	artifactDB          *database.DiskDB           // Database reference for persisting artifact metadata
	classifierManager   *classifier.Manager        // Classifier manager for media processing
	metadataManager     *classifier.MetadataManager // Metadata manager for extracting file metadata
)

// SetArtifactDB sets the database instance for artifact persistence
func SetArtifactDB(db *database.DiskDB) {
	artifactDB = db
}

// SetArtifactCacheDir sets the directory for storing artifact cache
func SetArtifactCacheDir(dir string) {
	artifactCacheDir = dir
}

type inspectArtifact struct {
	Type        string         `json:"type"`
	MimeType    string         `json:"mimeType"`
	Url         string         `json:"url"`
	ResourceUri string         `json:"resourceUri,omitempty"` // MCP resource URI for discovery
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type inspectResponse struct {
	Path             string            `json:"path"`
	Kind             string            `json:"kind"`
	Size             int64             `json:"size"`
	ModifiedAt       string            `json:"modifiedAt"`
	CreatedAt        string            `json:"createdAt"`
	Link             string            `json:"link"`
	MetadataUri      string            `json:"metadataUri,omitempty"`      // MCP resource URI for all generated metadata of this node
	ThumbnailUri     string            `json:"thumbnailUri,omitempty"`     // MCP resource URI for thumbnail (if available)
	TimelineUri      string            `json:"timelineUri,omitempty"`      // MCP resource URI for video timeline (if available)
	ContentUrl       string            `json:"contentUrl"`
	Artifacts        []inspectArtifact `json:"artifacts"`
	ArtifactsCount   int               `json:"artifactsCount"`             // Total count of artifacts
	NextPageUrl      string            `json:"nextPageUrl,omitempty"`
}

// initArtifactCache initializes the artifact cache directory and managers.
// Returns an error if the cache directory cannot be created.
func initArtifactCache() error {
	if artifactCacheDir == "" {
		return fmt.Errorf("artifact cache directory not configured")
	}

	if err := os.MkdirAll(artifactCacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create artifact cache directory %q: %w", artifactCacheDir, err)
	}

	// Initialize classifier manager if not already done
	if classifierManager == nil {
		classifierManager = classifier.NewManager()
		inspectLog.Debug("Initialized classifier manager")
	}

	// Initialize metadata manager if not already done
	if metadataManager == nil {
		metadataManager = classifier.NewMetadataManager()
		inspectLog.Debug("Initialized metadata manager")
	}

	return nil
}

func handleInspect(c *gin.Context, db *database.DiskDB) {
	path := c.Query("path")
	limit, offset := parsePagination(c.Query("limit"), c.Query("offset"))

	response, err := buildInspectResponse(path, db, limit, offset)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

func buildInspectResponse(inputPath string, db *database.DiskDB, limit, offset int) (*inspectResponse, error) {
	if err := initArtifactCache(); err != nil {
		return nil, fmt.Errorf("artifact cache initialization failed: %w", err)
	}
	if contentBaseURL == "" {
		contentBaseURL = "http://localhost:3000"
	}

	expandedPath, err := pathutil.ExpandPath(inputPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	entry, err := db.Get(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load entry: %w", err)
	}
	if entry == nil {
		return nil, errors.New("entry not indexed")
	}

	artifacts, nextPage, totalCount, err := generateArtifacts(expandedPath, entry.Kind, entry.Mtime, limit, offset)
	if err != nil {
		return nil, err
	}

	response := &inspectResponse{
		Path:           entry.Path,
		Kind:           entry.Kind,
		Size:           entry.Size,
		ModifiedAt:     time.Unix(entry.Mtime, 0).Format(time.RFC3339),
		CreatedAt:      time.Unix(entry.Ctime, 0).Format(time.RFC3339),
		Link:           fmt.Sprintf("synthesis://nodes/%s", entry.Path),
		ContentUrl:     fmt.Sprintf("%s/api/content?path=%s", contentBaseURL, url.QueryEscape(expandedPath)),
		Artifacts:      artifacts,
		ArtifactsCount: totalCount,
		NextPageUrl: func() string {
			if nextPage == -1 {
				return ""
			}
			return fmt.Sprintf("%s/api/inspect?path=%s&offset=%d&limit=%d", contentBaseURL, url.QueryEscape(expandedPath), nextPage, limit)
		}(),
	}

	// Add metadata URI if there are any artifacts
	if totalCount > 0 {
		response.MetadataUri = fmt.Sprintf("synthesis://nodes/%s/metadata", entry.Path)

		// Check artifact types to set type-specific URIs
		hasThumbnail := false
		hasTimeline := false
		for _, artifact := range artifacts {
			if artifact.Type == "thumbnail" {
				hasThumbnail = true
			}
			if artifact.Type == "video-timeline" {
				hasTimeline = true
			}
		}

		// Set type-specific URIs if those artifact types exist
		if hasThumbnail {
			response.ThumbnailUri = fmt.Sprintf("synthesis://nodes/%s/thumbnail", entry.Path)
		}
		if hasTimeline {
			response.TimelineUri = fmt.Sprintf("synthesis://nodes/%s/timeline", entry.Path)
		}
	}

	return response, nil
}

func generateArtifacts(path string, kind string, mtime int64, limit, offset int) ([]inspectArtifact, int, int, error) {
	artifacts := make([]inspectArtifact, 0)
	hashKey := artifactHashKey(path, mtime)
	mediaType := classifier.DetectMediaType(path)

	if kind == "file" && mediaType == classifier.MediaTypeImage {
		thumbPath, mimeType, err := createImageThumbnail(path, mtime, hashKey)
		if err != nil {
			return nil, -1, 0, err
		}
		artifact := buildArtifact("thumbnail", mimeType, thumbPath, path, hashKey, nil)
		artifacts = append(artifacts, artifact)
	}

	if kind == "file" && mediaType == classifier.MediaTypeVideo {
		posterPath, mimeType, err := createVideoPoster(path, mtime, hashKey)
		if err != nil {
			return nil, -1, 0, err
		}
		artifact := buildArtifact("thumbnail", mimeType, posterPath, path, hashKey, nil)
		artifacts = append(artifacts, artifact)

		frames, err := createVideoTimeline(path, mtime, hashKey, artifactTimelineN)
		if err != nil {
			return nil, -1, 0, err
		}
		for idx, f := range frames {
			frameHash := fmt.Sprintf("%s-frame-%d", hashKey, idx)
			artifact := buildArtifact("video-timeline", "image/jpeg", f, path, frameHash, map[string]any{"frame": idx})
			artifacts = append(artifacts, artifact)
		}
	}

	// Extract metadata for text and audio files
	if kind == "file" && metadataManager != nil && metadataManager.CanExtractMetadata(path) {
		metadataArtifact, err := extractFileMetadata(path, mtime, hashKey)
		if err != nil {
			inspectLog.WithError(err).Warn("Failed to extract metadata")
		} else if metadataArtifact != nil {
			artifacts = append(artifacts, *metadataArtifact)
		}
	}

	totalCount := len(artifacts)
	start := offset
	if start > len(artifacts) {
		start = len(artifacts)
	}
	end := offset + limit
	if end > len(artifacts) {
		end = len(artifacts)
	}

	next := -1
	if end < len(artifacts) {
		next = end
	}

	return artifacts[start:end], next, totalCount, nil
}

func buildArtifact(artifactType, mimeType, cachePath, sourcePath, hash string, metadata map[string]any) inspectArtifact {
	// Convert metadata to JSON string for database storage
	var metadataJSON string
	if metadata != nil {
		if jsonBytes, err := json.Marshal(metadata); err == nil {
			metadataJSON = string(jsonBytes)
		}
	}

	// Get file size of cached artifact
	var fileSize int64
	if stat, err := os.Stat(cachePath); err == nil {
		fileSize = stat.Size()
	}

	// Persist artifact metadata to database (fire and forget)
	// We do this asynchronously to avoid slowing down artifact generation
	go func() {
		if artifactDB != nil {
			metadata := &models.Metadata{
				Hash:         hash,
				SourcePath:   sourcePath,
				MetadataType: artifactType,
				MimeType:     mimeType,
				CachePath:    cachePath,
				FileSize:     fileSize,
				MetadataJson: metadataJSON,
				CreatedAt:    time.Now().Unix(),
			}
			if err := artifactDB.CreateOrUpdateMetadata(metadata); err != nil {
				inspectLog.WithError(err).WithField("hash", hash).Warn("Failed to persist artifact metadata")
			}
		}
	}()

	return inspectArtifact{
		Type:        artifactType,
		MimeType:    mimeType,
		Url:         fmt.Sprintf("%s/api/content?path=%s", contentBaseURL, url.QueryEscape(cachePath)),
		ResourceUri: fmt.Sprintf("synthesis://metadata/%s", hash),
		Metadata:    metadata,
	}
}

func artifactHashKey(path string, mtime int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", path, mtime)))
	return hex.EncodeToString(sum[:])
}

func artifactCachePath(hashKey, filename string) (string, error) {
	if len(hashKey) < 4 {
		return "", fmt.Errorf("invalid hash key for artifact cache")
	}

	dir := filepath.Join(artifactCacheDir, hashKey[:2], hashKey[2:4], hashKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	return filepath.Join(dir, filename), nil
}

func serveContent(c *gin.Context, db *database.DiskDB) {
	path := c.Query("path")
	if path == "" {
		c.String(http.StatusBadRequest, "path required")
		return
	}

	// Expand and validate the path
	targetPath, err := pathutil.ExpandPath(path)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid path")
		return
	}

	// Get absolute cache directory for comparison
	var absCacheDir string
	if artifactCacheDir != "" {
		absCacheDir, _ = filepath.Abs(artifactCacheDir)
	}

	// Security: Ensure the file exists in the database OR is a valid artifact
	entry, _ := db.Get(targetPath)
	if entry == nil {
		// Check if it's a valid artifact by:
		// 1. Checking if path is in the configured artifact cache directory
		// 2. Checking if the path exists in the metadata table as a cache_path
		// 3. Checking if it looks like a cache path (starts with "cache/")
		isValidArtifact := false

		// Check configured cache dir (convert to absolute for comparison)
		if absCacheDir != "" && strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(absCacheDir)) {
			isValidArtifact = true
			inspectLog.Debug("Valid artifact via cache dir check")
		}

		// Check if path is in metadata table (validates it's a known artifact)
		if !isValidArtifact && db != nil {
			metadata, err := db.GetMetadataByCachePath(path)
			inspectLog.WithField("path", path).WithField("found", metadata != nil).WithField("err", err).Debug("Checking metadata table")
			if metadata != nil {
				isValidArtifact = true
				inspectLog.Debug("Valid artifact via metadata table")
			}
		}

		// Allow paths that look like cache paths (relative cache directory)
		if !isValidArtifact && strings.HasPrefix(path, "cache/") {
			isValidArtifact = true
			inspectLog.Debug("Valid artifact via cache/ prefix")
		}

		if !isValidArtifact {
			inspectLog.WithField("path", path).WithField("targetPath", targetPath).Warn("Path not accessible")
			c.String(http.StatusForbidden, "path not accessible")
			return
		}

		// Try to find the artifact on disk - handle both absolute and relative paths
		// The database may have absolute paths stored while the file is at a relative path (or vice versa)
		actualPath := targetPath
		if _, err := os.Stat(targetPath); err != nil {
			// If targetPath is absolute and within cache dir, try the relative version
			if absCacheDir != "" && strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(absCacheDir)) {
				// Extract relative path from absolute
				relPath, relErr := filepath.Rel(absCacheDir, targetPath)
				if relErr == nil {
					// Try relative to current working directory
					tryPath := filepath.Join(artifactCacheDir, relPath)
					if _, statErr := os.Stat(tryPath); statErr == nil {
						actualPath = tryPath
						inspectLog.WithField("originalPath", targetPath).WithField("actualPath", actualPath).Debug("Found artifact at relative path")
					}
				}
			}

			// If still not found, check if it's a direct absolute path that exists
			if actualPath == targetPath {
				inspectLog.WithField("targetPath", targetPath).WithError(err).Warn("Artifact not found on disk")
				c.String(http.StatusNotFound, "not found")
				return
			}
		}

		targetPath = actualPath
	}

	// Serve the file
	file, err := os.Open(targetPath)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to open file")
		return
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	mimeType := http.DetectContentType(buf[:n])
	file.Seek(0, io.SeekStart)

	c.Header("Content-Type", mimeType)
	c.File(targetPath)
}

// serveContentFromCache serves files from a cache directory without database validation
// Used for multi-project architecture where cache is shared across projects
func serveContentFromCache(c *gin.Context, cacheDir string) {
	path := c.Query("path")
	if path == "" {
		c.String(http.StatusBadRequest, "path required")
		return
	}

	// Expand and validate the path
	targetPath, err := pathutil.ExpandPath(path)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid path")
		return
	}

	// Get absolute cache directory for comparison
	var absCacheDir string
	if cacheDir != "" {
		absCacheDir, _ = filepath.Abs(cacheDir)
	}

	// Security: Only allow files in the cache directory or paths starting with "cache/"
	isValidArtifact := false

	// Check configured cache dir (convert to absolute for comparison)
	if absCacheDir != "" && strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(absCacheDir)) {
		isValidArtifact = true
	}

	// Allow paths that look like cache paths (relative cache directory)
	if !isValidArtifact && strings.HasPrefix(path, "cache/") {
		isValidArtifact = true
	}

	if !isValidArtifact {
		inspectLog.WithField("path", path).WithField("targetPath", targetPath).Warn("Path not accessible")
		c.String(http.StatusForbidden, "path not accessible")
		return
	}

	// Try to find the artifact on disk
	actualPath := targetPath
	if _, err := os.Stat(targetPath); err != nil {
		// If targetPath is absolute and within cache dir, try the relative version
		if absCacheDir != "" && strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(absCacheDir)) {
			relPath, relErr := filepath.Rel(absCacheDir, targetPath)
			if relErr == nil {
				tryPath := filepath.Join(cacheDir, relPath)
				if _, statErr := os.Stat(tryPath); statErr == nil {
					actualPath = tryPath
				}
			}
		}

		if actualPath == targetPath {
			c.String(http.StatusNotFound, "not found")
			return
		}
	}

	// Serve the file
	file, err := os.Open(actualPath)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to open file")
		return
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	mimeType := http.DetectContentType(buf[:n])
	file.Seek(0, io.SeekStart)

	c.Header("Content-Type", mimeType)
	c.File(actualPath)
}

// handleInspectWithDB handles file inspection with a database backend
// Used for multi-project architecture where database is resolved from context
func handleInspectWithDB(c *gin.Context, db database.Backend) {
	path := c.Query("path")
	limit, offset := parsePagination(c.Query("limit"), c.Query("offset"))

	// For now, we need to cast to DiskDB until we refactor buildInspectResponse
	// to work with the Backend interface
	diskDB, ok := db.(*database.DiskDB)
	if !ok {
		c.String(http.StatusInternalServerError, "unsupported database backend")
		return
	}

	response, err := buildInspectResponse(path, diskDB, limit, offset)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

func createImageThumbnail(path string, mtime int64, hashKey string) (string, string, error) {
	cachePath, err := artifactCachePath(hashKey, "thumb.jpg")
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, "image/jpeg", nil
	}

	req := &classifier.ArtifactRequest{
		SourcePath:   path,
		OutputPath:   cachePath,
		MediaType:    classifier.MediaTypeImage,
		ArtifactType: classifier.ArtifactTypeThumbnail,
		MaxWidth:     thumbnailMaxWidth,
		MaxHeight:    thumbnailMaxHeight,
	}

	result := classifierManager.GenerateThumbnail(req)
	if result.Error != nil {
		return "", "", result.Error
	}

	return result.OutputPath, result.MimeType, nil
}

func createVideoPoster(path string, mtime int64, hashKey string) (string, string, error) {
	cachePath, err := artifactCachePath(hashKey, "poster.jpg")
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, "image/jpeg", nil
	}

	req := &classifier.ArtifactRequest{
		SourcePath:   path,
		OutputPath:   cachePath,
		MediaType:    classifier.MediaTypeVideo,
		ArtifactType: classifier.ArtifactTypeThumbnail,
		MaxWidth:     thumbnailMaxWidth,
		MaxHeight:    thumbnailMaxHeight,
	}

	result := classifierManager.GenerateThumbnail(req)
	if result.Error != nil {
		inspectLog.WithError(result.Error).Warn("Failed to generate video poster")
		return "", "", result.Error
	}

	return result.OutputPath, result.MimeType, nil
}

func createVideoTimeline(path string, mtime int64, hashKey string, frames int) ([]string, error) {
	results := make([]string, 0, frames)
	for i := 0; i < frames; i++ {
		framePath, err := artifactCachePath(hashKey, fmt.Sprintf("timeline_%02d.jpg", i))
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(framePath); os.IsNotExist(err) {
			req := &classifier.ArtifactRequest{
				SourcePath:   path,
				OutputPath:   framePath,
				MediaType:    classifier.MediaTypeVideo,
				ArtifactType: classifier.ArtifactTypeTimeline,
				FrameIndex:   i,
				TotalFrames:  frames,
				MaxWidth:     thumbnailMaxWidth,
				MaxHeight:    thumbnailMaxHeight,
			}

			result := classifierManager.GenerateTimelineFrame(req)
			if result.Error != nil {
				inspectLog.WithError(result.Error).Warn("Failed to generate timeline frame")
				return nil, result.Error
			}
		}
		results = append(results, framePath)
	}
	return results, nil
}

// extractFileMetadata extracts metadata from text and audio files
func extractFileMetadata(path string, mtime int64, hashKey string) (*inspectArtifact, error) {
	// Check if metadata already exists in database
	if artifactDB != nil {
		existing, err := artifactDB.GetMetadata(hashKey)
		if err != nil {
			inspectLog.WithError(err).Debug("Failed to check existing metadata")
		}
		if existing != nil {
			// Metadata already extracted, return it
			var metadataMap map[string]interface{}
			if existing.MetadataJson != "" {
				if err := json.Unmarshal([]byte(existing.MetadataJson), &metadataMap); err != nil {
					inspectLog.WithError(err).Warn("Failed to unmarshal existing metadata")
				}
			}

			return &inspectArtifact{
				Type:        existing.MetadataType,
				MimeType:    "application/json",
				Url:         fmt.Sprintf("%s/api/content?path=%s", contentBaseURL, url.QueryEscape(path)),
				ResourceUri: fmt.Sprintf("synthesis://nodes/%s/metadata/%s", path, existing.MetadataType),
				Metadata:    metadataMap,
			}, nil
		}
	}

	// Extract metadata using the metadata manager
	result := metadataManager.ExtractMetadata(path, 0) // 0 = use default max size
	if result.Error != nil {
		return nil, result.Error
	}

	// Convert to JSON
	metadataJSON, err := result.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to convert metadata to JSON: %w", err)
	}

	// Get file size
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Store metadata in database asynchronously
	if artifactDB != nil {
		metadata := &models.Metadata{
			Hash:         hashKey,
			SourcePath:   path,
			MetadataType: result.MetadataType,
			MimeType:     "application/json",
			CachePath:    path, // No cache file for metadata, use source path
			FileSize:     stat.Size(),
			MetadataJson: metadataJSON,
		}

		go func() {
			if err := artifactDB.CreateOrUpdateMetadata(metadata); err != nil {
				inspectLog.WithError(err).Warn("Failed to persist metadata")
			} else {
				inspectLog.WithFields(map[string]interface{}{
					"path": path,
					"type": result.MetadataType,
				}).Debug("Persisted metadata")
			}
		}()
	}

	// Return artifact
	return &inspectArtifact{
		Type:        result.MetadataType,
		MimeType:    "application/json",
		Url:         fmt.Sprintf("%s/api/content?path=%s", contentBaseURL, url.QueryEscape(path)),
		ResourceUri: fmt.Sprintf("synthesis://nodes/%s/metadata/%s", path, result.MetadataType),
		Metadata:    result.Data,
	}, nil
}

func parsePagination(limitRaw, offsetRaw string) (int, int) {
	limit := 20
	offset := 0

	if limitRaw != "" {
		if v, err := strconv.Atoi(limitRaw); err == nil && v > 0 {
			limit = v
		}
	}
	if offsetRaw != "" {
		if v, err := strconv.Atoi(offsetRaw); err == nil && v >= 0 {
			offset = v
		}
	}

	return limit, offset
}

var inspectLog = logger.WithName("inspect")
