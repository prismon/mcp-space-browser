package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
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
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
)

const (
	contentTokenTTL    = 10 * time.Minute
	artifactTimelineN  = 5
	thumbnailMaxWidth  = 320
	thumbnailMaxHeight = 320
)

var (
	contentTokenSecret []byte
	contentBaseURL     string
	artifactCacheDir   = filepath.Join(os.TempDir(), "mcp-space-browser-artifacts")
)

type inspectArtifact struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mimeType"`
	Url      string         `json:"url"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type inspectResponse struct {
	Path        string            `json:"path"`
	Kind        string            `json:"kind"`
	Size        int64             `json:"size"`
	ModifiedAt  string            `json:"modifiedAt"`
	CreatedAt   string            `json:"createdAt"`
	Link        string            `json:"link"`
	ContentUrl  string            `json:"contentUrl"`
	Artifacts   []inspectArtifact `json:"artifacts"`
	NextPageUrl string            `json:"nextPageUrl,omitempty"`
}

func initContentTokenSecret() {
	if contentTokenSecret != nil {
		return
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(fmt.Errorf("failed to initialize content token secret: %w", err))
	}
	contentTokenSecret = secret

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
	if contentTokenSecret == nil {
		initContentTokenSecret()
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

	contentToken := createContentToken(expandedPath, time.Now().Add(contentTokenTTL))
	artifacts, nextPage, err := generateArtifacts(expandedPath, entry.Kind, entry.Mtime, limit, offset)
	if err != nil {
		return nil, err
	}

	return &inspectResponse{
		Path:       entry.Path,
		Kind:       entry.Kind,
		Size:       entry.Size,
		ModifiedAt: time.Unix(entry.Mtime, 0).Format(time.RFC3339),
		CreatedAt:  time.Unix(entry.Ctime, 0).Format(time.RFC3339),
		Link:       fmt.Sprintf("shell://nodes/%s", entry.Path),
		ContentUrl: fmt.Sprintf("%s/api/content?token=%s", contentBaseURL, url.QueryEscape(contentToken)),
		Artifacts:  artifacts,
		NextPageUrl: func() string {
			if nextPage == -1 {
				return ""
			}
			return fmt.Sprintf("%s/api/inspect?path=%s&offset=%d&limit=%d", contentBaseURL, url.QueryEscape(expandedPath), nextPage, limit)
		}(),
	}, nil
}

func generateArtifacts(path string, kind string, mtime int64, limit, offset int) ([]inspectArtifact, int, error) {
	artifacts := make([]inspectArtifact, 0)
	hashKey := artifactHashKey(path, mtime)
	lower := strings.ToLower(filepath.Ext(path))
	isImage := lower == ".jpg" || lower == ".jpeg" || lower == ".png" || lower == ".gif" || lower == ".bmp"
	isVideo := lower == ".mp4" || lower == ".mov" || lower == ".mkv" || lower == ".avi" || lower == ".webm"

	if kind == "file" && isImage {
		thumbPath, mimeType, err := createImageThumbnail(path, mtime, hashKey)
		if err != nil {
			return nil, -1, err
		}
		artifacts = append(artifacts, buildArtifact("thumbnail", mimeType, thumbPath, nil))
	}

	if kind == "file" && isVideo {
		posterPath, mimeType, err := createVideoPoster(path, mtime, hashKey)
		if err != nil {
			return nil, -1, err
		}
		artifacts = append(artifacts, buildArtifact("thumbnail", mimeType, posterPath, nil))

		frames, err := createVideoTimeline(path, mtime, hashKey, artifactTimelineN)
		if err != nil {
			return nil, -1, err
		}
		for idx, f := range frames {
			artifacts = append(artifacts, buildArtifact("video-timeline", "image/jpeg", f, map[string]any{"frame": idx}))
		}
	}

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

	return artifacts[start:end], next, nil
}

func buildArtifact(artifactType, mimeType, filePath string, metadata map[string]any) inspectArtifact {
	token := createContentToken(filePath, time.Now().Add(contentTokenTTL))
	return inspectArtifact{
		Type:     artifactType,
		MimeType: mimeType,
		Url:      fmt.Sprintf("%s/api/content?token=%s", contentBaseURL, url.QueryEscape(token)),
		Metadata: metadata,
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
	token := c.Query("token")
	if token == "" {
		c.String(http.StatusBadRequest, "token required")
		return
	}

	targetPath, err := validateContentToken(token)
	if err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}

	// If the file is part of the index ensure it exists
	entry, _ := db.Get(targetPath)
	if entry == nil {
		// It might be an artifact; just ensure path exists
		if _, err := os.Stat(targetPath); err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
	}

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

func createContentToken(path string, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s|%d", path, expiresAt.Unix())
	mac := hmac.New(sha256.New, contentTokenSecret)
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := fmt.Sprintf("%s|%s", payload, signature)
	return base64.RawURLEncoding.EncodeToString([]byte(token))
}

func validateContentToken(token string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", errors.New("invalid token")
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return "", errors.New("invalid token")
	}

	path := parts[0]
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", errors.New("invalid token")
	}

	payload := fmt.Sprintf("%s|%d", path, expiresUnix)
	mac := hmac.New(sha256.New, contentTokenSecret)
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(parts[2])) {
		return "", errors.New("invalid token signature")
	}

	if time.Now().Unix() > expiresUnix {
		return "", errors.New("token expired")
	}

	return path, nil
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

	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vf", "thumbnail,scale=320:-1", "-frames:v", strconv.Itoa(frameCount), outputPath)
	return cmd.Run()
}

func ensureFFmpegTimelineFrame(inputPath, outputPath string, index, total int) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return err
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

var inspectLog = logger.WithName("inspect")
