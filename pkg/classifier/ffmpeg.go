package classifier

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prismon/mcp-space-browser/pkg/logger"
)

// FFmpeg configuration constants
const (
	// ffmpegFrameSampleInterval is the interval for selecting frames (every Nth frame)
	ffmpegFrameSampleInterval = 5
	// ffmpegDefaultQuality is the default quality for output (lower = better)
	ffmpegDefaultQuality = 2
	// ffmpegOutputFormat is the format pattern for timeline frame files
	ffmpegTimelinePattern = "timeline_%02d.jpg"
)

// FFmpeg argument constants - these are ffmpeg CLI arguments
const (
	ffmpegArgOverwrite   = "-y"
	ffmpegArgInput       = "-i"
	ffmpegArgVideoFilter = "-vf"
	ffmpegArgFrameCount  = "-frames:v"
)

// FFmpeg error types for better error handling
var (
	// ErrFFmpegNotAvailable indicates ffmpeg binary is not found on the system
	ErrFFmpegNotAvailable = fmt.Errorf("ffmpeg binary not available on system PATH")
	// ErrFFmpegVideoOnly indicates this classifier only handles video files
	ErrFFmpegVideoOnly = fmt.Errorf("ffmpeg classifier only handles video files")
)

var ffmpegLog = logger.WithName("ffmpeg-classifier")

// FFmpegClassifier uses ffmpeg to process video files.
// Note: This classifier requires the ffmpeg binary to be installed on the system.
// While we use exec.Command to call ffmpeg, this is wrapped in a proper Go interface
// to provide type safety, proper error handling, and testability.
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
		return &ArtifactResult{Error: ErrFFmpegNotAvailable}
	}

	if req.MediaType != MediaTypeVideo {
		return &ArtifactResult{Error: ErrFFmpegVideoOnly}
	}

	// Validate paths to prevent command injection
	if err := validateFilePath(req.SourcePath); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("invalid source path: %w", err)}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to create output directory: %w", err)}
	}

	// Build video filter: thumbnail selects a representative frame, scale resizes
	scaleFilter := fmt.Sprintf("thumbnail,scale=%d:-1", req.MaxWidth)

	// Execute ffmpeg command
	result := f.runFFmpeg(req.SourcePath, req.OutputPath, scaleFilter, 1)
	if result.Error != nil {
		return result
	}

	ffmpegLog.WithField("source", req.SourcePath).
		WithField("output", req.OutputPath).
		Debug("Generated video thumbnail")

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
		return &ArtifactResult{Error: ErrFFmpegNotAvailable}
	}

	if req.MediaType != MediaTypeVideo {
		return &ArtifactResult{Error: ErrFFmpegVideoOnly}
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
	pattern := filepath.Join(tmpDir, ffmpegTimelinePattern)

	// Use select filter to sample frames evenly throughout the video
	// The formula 'not(mod(n,N))' selects every Nth frame
	selectFilter := fmt.Sprintf("select='not(mod(n,%d))',scale=%d:-1", ffmpegFrameSampleInterval, req.MaxWidth)

	// Execute ffmpeg command for timeline generation
	result := f.runFFmpegTimeline(req.SourcePath, pattern, selectFilter, req.TotalFrames)
	if result.Error != nil {
		return result
	}

	ffmpegLog.WithField("source", req.SourcePath).
		WithField("frames", req.TotalFrames).
		Debug("Generated video timeline frames")

	// Find the specific frame file we generated (ffmpeg uses 1-based indexing)
	generatedPath := filepath.Join(tmpDir, fmt.Sprintf("timeline_%02d.jpg", req.FrameIndex+1))
	if _, err := os.Stat(generatedPath); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("timeline frame not generated: %w", err)}
	}

	// Rename to the requested output path if needed
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
	// Note: We allow [] () {} since these are valid in filenames and exec.Command
	// doesn't use shell expansion, so they're safe. We only block shell operators.
	if strings.ContainsAny(path, ";|&$`<>!") {
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

// runFFmpeg executes ffmpeg to extract a single frame from a video
func (f *FFmpegClassifier) runFFmpeg(sourcePath, outputPath, videoFilter string, frameCount int) *ArtifactResult {
	cmd := exec.Command(f.ffmpegPath,
		ffmpegArgOverwrite,
		ffmpegArgInput, sourcePath,
		ffmpegArgVideoFilter, videoFilter,
		ffmpegArgFrameCount, strconv.Itoa(frameCount),
		outputPath,
	)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg != "" {
			return &ArtifactResult{Error: fmt.Errorf("ffmpeg failed: %w (stderr: %s)", err, errMsg)}
		}
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg failed: %w", err)}
	}

	return &ArtifactResult{}
}

// runFFmpegTimeline executes ffmpeg to extract multiple timeline frames
func (f *FFmpegClassifier) runFFmpegTimeline(sourcePath, outputPattern, videoFilter string, frameCount int) *ArtifactResult {
	cmd := exec.Command(f.ffmpegPath,
		ffmpegArgOverwrite,
		ffmpegArgInput, sourcePath,
		ffmpegArgVideoFilter, videoFilter,
		ffmpegArgFrameCount, strconv.Itoa(frameCount),
		outputPattern,
	)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg != "" {
			return &ArtifactResult{Error: fmt.Errorf("ffmpeg timeline failed: %w (stderr: %s)", err, errMsg)}
		}
		return &ArtifactResult{Error: fmt.Errorf("ffmpeg timeline failed: %w", err)}
	}

	return &ArtifactResult{}
}
