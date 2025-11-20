package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
	"golang.org/x/image/draw"
)

const (
	artifactTimelineN  = 5
	thumbnailMaxWidth  = 320
	thumbnailMaxHeight = 320
)

var (
	contentBaseURL   string
	artifactCacheDir string
	artifactDB       *database.DiskDB // Database reference for persisting artifact metadata
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

func initArtifactCache() {
	if err := os.MkdirAll(artifactCacheDir, 0o755); err != nil {
		panic(fmt.Errorf("failed to create artifact cache: %w", err))
	}
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
	initArtifactCache()
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
		Link:           fmt.Sprintf("shell://nodes/%s", entry.Path),
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
		response.MetadataUri = fmt.Sprintf("shell://nodes/%s/metadata", entry.Path)

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
			response.ThumbnailUri = fmt.Sprintf("shell://nodes/%s/thumbnail", entry.Path)
		}
		if hasTimeline {
			response.TimelineUri = fmt.Sprintf("shell://nodes/%s/timeline", entry.Path)
		}
	}

	return response, nil
}

func generateArtifacts(path string, kind string, mtime int64, limit, offset int) ([]inspectArtifact, int, int, error) {
	artifacts := make([]inspectArtifact, 0)
	hashKey := artifactHashKey(path, mtime)
	lower := strings.ToLower(filepath.Ext(path))
	isImage := lower == ".jpg" || lower == ".jpeg" || lower == ".png" || lower == ".gif" || lower == ".bmp"
	isVideo := lower == ".mp4" || lower == ".mov" || lower == ".mkv" || lower == ".avi" || lower == ".webm"

	if kind == "file" && isImage {
		thumbPath, mimeType, err := createImageThumbnail(path, mtime, hashKey)
		if err != nil {
			return nil, -1, 0, err
		}
		artifact := buildArtifact("thumbnail", mimeType, thumbPath, path, hashKey, nil)
		artifacts = append(artifacts, artifact)
	}

	if kind == "file" && isVideo {
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
		ResourceUri: fmt.Sprintf("shell://metadata/%s", hash),
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

	// Security: Ensure the file exists in the database OR is an artifact
	entry, _ := db.Get(targetPath)
	if entry == nil {
		// Check if it's an artifact (in our cache directory)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(artifactCacheDir)) {
			c.String(http.StatusForbidden, "path not accessible")
			return
		}
		// Ensure artifact exists
		if _, err := os.Stat(targetPath); err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
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

func createImageThumbnail(path string, mtime int64, hashKey string) (string, string, error) {
	cachePath, err := artifactCachePath(hashKey, "thumb.jpg")
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, "image/jpeg", nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", "", err
	}

	scaled := resizeImage(img, thumbnailMaxWidth, thumbnailMaxHeight)

	if err := writeJPEG(cachePath, scaled); err != nil {
		return "", "", err
	}

	return cachePath, "image/jpeg", nil
}

func createVideoPoster(path string, mtime int64, hashKey string) (string, string, error) {
	cachePath, err := artifactCachePath(hashKey, "poster.jpg")
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, "image/jpeg", nil
	}

	if err := ensureFFmpegFrame(path, cachePath, 1); err != nil {
		inspectLog.WithError(err).Warn("ffmpeg unavailable, generating placeholder poster")
		if err := writePlaceholderImage(cachePath); err != nil {
			return "", "", err
		}
	}

	return cachePath, "image/jpeg", nil
}

func createVideoTimeline(path string, mtime int64, hashKey string, frames int) ([]string, error) {
	results := make([]string, 0, frames)
	for i := 0; i < frames; i++ {
		framePath, err := artifactCachePath(hashKey, fmt.Sprintf("timeline_%02d.jpg", i))
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(framePath); os.IsNotExist(err) {
			if err := ensureFFmpegTimelineFrame(path, framePath, i, frames); err != nil {
				inspectLog.WithError(err).Warn("ffmpeg unavailable, generating placeholder timeline frame")
				if err := writePlaceholderImage(framePath); err != nil {
					return nil, err
				}
			}
		}
		results = append(results, framePath)
	}
	return results, nil
}

func ensureFFmpegFrame(inputPath, outputPath string, frameCount int) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return err
	}

	// Validate paths to prevent command injection
	if err := validateFilePath(inputPath); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	if err := validateFilePath(outputPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}

	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vf", "thumbnail,scale=320:-1", "-frames:v", strconv.Itoa(frameCount), outputPath)
	return cmd.Run()
}

func ensureFFmpegTimelineFrame(inputPath, outputPath string, index, total int) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return err
	}

	// Validate paths to prevent command injection
	if err := validateFilePath(inputPath); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	if err := validateFilePath(outputPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}

	// Spread frames across the duration using select to approximate even sampling
	selectFilter := fmt.Sprintf("select='not(mod(n,%d))',scale=320:-1", 5)
	tmpDir := filepath.Dir(outputPath)
	pattern := filepath.Join(tmpDir, "timeline_%02d.jpg")
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vf", selectFilter, "-frames:v", strconv.Itoa(total), pattern)
	if err := cmd.Run(); err != nil {
		return err
	}

	src := filepath.Join(tmpDir, fmt.Sprintf("timeline_%02d.jpg", index))
	if _, err := os.Stat(src); err != nil {
		return err
	}
	return os.Rename(src, outputPath)
}

func resizeImage(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	ratio := float64(width) / float64(height)
	targetW := maxWidth
	targetH := int(float64(targetW) / ratio)
	if targetH > maxHeight {
		targetH = maxHeight
		targetW = int(float64(targetH) * ratio)
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}

func writeJPEG(path string, img image.Image) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, img, &jpeg.Options{Quality: 80})
}

func writePlaceholderImage(path string) error {
	placeholder := image.NewRGBA(image.Rect(0, 0, thumbnailMaxWidth, thumbnailMaxHeight))
	if err := writeJPEG(path, placeholder); err != nil {
		return err
	}
	return nil
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

// validateFilePath validates a file path to prevent command injection
func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Check for suspicious characters that could be used for command injection
	if strings.ContainsAny(path, ";|&$`<>(){}[]!*?") {
		return fmt.Errorf("path contains invalid characters")
	}

	// Ensure path is absolute or verify it exists
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("file does not exist: %w", err)
	}

	return nil
}

var inspectLog = logger.WithName("inspect")
