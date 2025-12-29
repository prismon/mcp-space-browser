package classifier

import (
	"fmt"
	"path/filepath"
	"strings"
)

// MediaType represents the type of media file
type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
	MediaTypeUnknown MediaType = "unknown"
)

// ArtifactType represents the type of artifact to generate
type ArtifactType string

const (
	ArtifactTypeThumbnail ArtifactType = "thumbnail"
	ArtifactTypeTimeline  ArtifactType = "timeline"
)

// ArtifactRequest contains the parameters for generating an artifact
type ArtifactRequest struct {
	SourcePath string       // Path to the source media file
	OutputPath string       // Path where the artifact should be saved
	MediaType  MediaType    // Type of media (image/video)
	ArtifactType ArtifactType // Type of artifact to generate

	// For timeline generation
	FrameIndex int // Which frame to generate (0-based)
	TotalFrames int // Total number of frames in timeline

	// Size constraints
	MaxWidth  int
	MaxHeight int
}

// ArtifactResult contains the result of artifact generation
type ArtifactResult struct {
	OutputPath       string // Path to the generated artifact
	MimeType         string // MIME type of the generated artifact
	Generator        string // Name of the generator that created this artifact (e.g., "ffmpeg", "go")
	GeneratorVersion string // Version of the generator (optional)
	Error            error  // Error if generation failed
}

// Classifier defines the interface for media classification and artifact generation
type Classifier interface {
	// Name returns a human-readable name for this classifier
	Name() string

	// CanHandle returns true if this classifier can handle the given media type
	CanHandle(mediaType MediaType) bool

	// IsAvailable returns true if this classifier is available (e.g., required tools are installed)
	IsAvailable() bool

	// GenerateThumbnail creates a thumbnail for an image or video
	GenerateThumbnail(req *ArtifactRequest) *ArtifactResult

	// GenerateTimelineFrame creates a single frame for a video timeline
	GenerateTimelineFrame(req *ArtifactRequest) *ArtifactResult
}

// DetectMediaType determines the media type from file extension
func DetectMediaType(path string) MediaType {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return MediaTypeImage
	case ".mp4", ".mov", ".mkv", ".avi", ".webm", ".m4v", ".flv":
		return MediaTypeVideo
	default:
		return MediaTypeUnknown
	}
}

// IsImageFile returns true if the path is an image file
func IsImageFile(path string) bool {
	return DetectMediaType(path) == MediaTypeImage
}

// IsVideoFile returns true if the path is a video file
func IsVideoFile(path string) bool {
	return DetectMediaType(path) == MediaTypeVideo
}

// validateRequest performs basic validation on an artifact request
func validateRequest(req *ArtifactRequest) error {
	if req.SourcePath == "" {
		return fmt.Errorf("source path cannot be empty")
	}
	if req.OutputPath == "" {
		return fmt.Errorf("output path cannot be empty")
	}
	if req.MediaType == MediaTypeUnknown {
		return fmt.Errorf("unknown media type")
	}
	if req.MaxWidth <= 0 {
		req.MaxWidth = 320 // Default
	}
	if req.MaxHeight <= 0 {
		req.MaxHeight = 320 // Default
	}
	return nil
}
