package classifier

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prismon/mcp-space-browser/pkg/logger"
)

var ffmpegLog = logger.WithName("ffmpeg-classifier")

// FFmpegClassifier uses ffmpeg to process video files
type FFmpegClassifier struct {
	ffmpegPath string
}

// NewFFmpegClassifier creates a new FFmpeg-based classifier
func NewFFmpegClassifier() *FFmpegClassifier {
	path, _ := exec.LookPath("ffmpeg")
	return &FFmpegClassifier{
		ffmpegPath: path,
	}
}

// Name returns the classifier name
func (f *FFmpegClassifier) Name() string {
	return "FFmpeg"
}

// CanHandle returns true for video files
func (f *FFmpegClassifier) CanHandle(mediaType MediaType) bool {
	return mediaType == MediaTypeVideo
}

// IsAvailable checks if ffmpeg is available on the system
func (f *FFmpegClassifier) IsAvailable() bool {
	return f.ffmpegPath != ""
}

// GenerateThumbnail creates a video thumbnail using ffmpeg
func (f *FFmpegClassifier) GenerateThumbnail(req *ArtifactRequest) *ArtifactResult {
	if err := validateRequest(req); err != nil {
		return &ArtifactResult{Error: err}
	}

	if !f.IsAvailable() {
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg not available")}
	}

	if req.MediaType != MediaTypeVideo {
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg classifier only handles video files")}
	}

	// Validate paths to prevent command injection
	if err := validateFilePath(req.SourcePath); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("invalid source path: %w", err)}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to create output directory: %w", err)}
	}

	// Use ffmpeg to extract a thumbnail frame
	// -y: overwrite output file
	// -i: input file
	// -vf: video filter (thumbnail selects a representative frame, scale resizes)
	// -frames:v: number of frames to output
	scaleFilter := fmt.Sprintf("thumbnail,scale=%d:-1", req.MaxWidth)
	cmd := exec.Command(f.ffmpegPath,
		"-y",
		"-i", req.SourcePath,
		"-vf", scaleFilter,
		"-frames:v", "1",
		req.OutputPath,
	)

	ffmpegLog.WithField("source", req.SourcePath).
		WithField("output", req.OutputPath).
		Debug("Generating video thumbnail")

	if err := cmd.Run(); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg thumbnail failed: %w", err)}
	}

	return &ArtifactResult{
		OutputPath: req.OutputPath,
		MimeType:   "image/jpeg",
	}
}

// GenerateTimelineFrame creates a single timeline frame using ffmpeg
func (f *FFmpegClassifier) GenerateTimelineFrame(req *ArtifactRequest) *ArtifactResult {
	if err := validateRequest(req); err != nil {
		return &ArtifactResult{Error: err}
	}

	if !f.IsAvailable() {
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg not available")}
	}

	if req.MediaType != MediaTypeVideo {
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg classifier only handles video files")}
	}

	if req.TotalFrames <= 0 {
		return &ArtifactResult{Error: fmt.Errorf("totalFrames must be > 0")}
	}

	// Validate paths
	if err := validateFilePath(req.SourcePath); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("invalid source path: %w", err)}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to create output directory: %w", err)}
	}

	// Generate all timeline frames at once, then select the one we need
	// This is more efficient than calling ffmpeg multiple times
	tmpDir := filepath.Dir(req.OutputPath)
	pattern := filepath.Join(tmpDir, "timeline_%02d.jpg")

	// Use select filter to sample frames evenly throughout the video
	// The formula 'not(mod(n,5))' selects every 5th frame
	selectFilter := fmt.Sprintf("select='not(mod(n,%d))',scale=%d:-1", 5, req.MaxWidth)

	cmd := exec.Command(f.ffmpegPath,
		"-y",
		"-i", req.SourcePath,
		"-vf", selectFilter,
		"-frames:v", strconv.Itoa(req.TotalFrames),
		pattern,
	)

	ffmpegLog.WithField("source", req.SourcePath).
		WithField("frames", req.TotalFrames).
		Debug("Generating video timeline frames")

	if err := cmd.Run(); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg timeline failed: %w", err)}
	}

	// Find the specific frame file we generated
	generatedPath := filepath.Join(tmpDir, fmt.Sprintf("timeline_%02d.jpg", req.FrameIndex+1))
	if _, err := os.Stat(generatedPath); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("timeline frame not generated: %w", err)}
	}

	// Rename to the requested output path
	if generatedPath != req.OutputPath {
		if err := os.Rename(generatedPath, req.OutputPath); err != nil {
			return &ArtifactResult{Error: fmt.Errorf("failed to rename frame: %w", err)}
		}
	}

	return &ArtifactResult{
		OutputPath: req.OutputPath,
		MimeType:   "image/jpeg",
	}
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
