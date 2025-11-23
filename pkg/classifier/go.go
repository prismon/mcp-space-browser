package classifier

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // Register GIF format
	"image/jpeg"
	_ "image/png"  // Register PNG format
	"os"
	"path/filepath"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"golang.org/x/image/draw"
)

var goLog = logger.WithName("go-classifier")

// GoClassifier uses standard Go libraries for media processing
type GoClassifier struct{}

// NewGoClassifier creates a new Go-based classifier
func NewGoClassifier() *GoClassifier {
	return &GoClassifier{}
}

// Name returns the classifier name
func (g *GoClassifier) Name() string {
	return "Go Standard Library"
}

// CanHandle returns true for images (and provides fallback for videos)
func (g *GoClassifier) CanHandle(mediaType MediaType) bool {
	// Primarily handles images, but can provide fallback for videos
	return mediaType == MediaTypeImage || mediaType == MediaTypeVideo
}

// IsAvailable always returns true as it uses standard library
func (g *GoClassifier) IsAvailable() bool {
	return true
}

// GenerateThumbnail creates a thumbnail for an image or placeholder for video
func (g *GoClassifier) GenerateThumbnail(req *ArtifactRequest) *ArtifactResult {
	if err := validateRequest(req); err != nil {
		return &ArtifactResult{Error: err}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to create output directory: %w", err)}
	}

	switch req.MediaType {
	case MediaTypeImage:
		return g.generateImageThumbnail(req)
	case MediaTypeVideo:
		return g.generateVideoPlaceholder(req)
	default:
		return &ArtifactResult{Error: fmt.Errorf("unsupported media type: %s", req.MediaType)}
	}
}

// GenerateTimelineFrame creates a timeline frame (placeholder for videos)
func (g *GoClassifier) GenerateTimelineFrame(req *ArtifactRequest) *ArtifactResult {
	if err := validateRequest(req); err != nil {
		return &ArtifactResult{Error: err}
	}

	if req.MediaType != MediaTypeVideo {
		return &ArtifactResult{Error: fmt.Errorf("timeline frames only supported for videos")}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to create output directory: %w", err)}
	}

	// Create a placeholder for timeline frames
	// In a real implementation, you might use a video decoding library
	return g.generateVideoPlaceholder(req)
}

// generateImageThumbnail creates a thumbnail from an image file
func (g *GoClassifier) generateImageThumbnail(req *ArtifactRequest) *ArtifactResult {
	goLog.WithField("source", req.SourcePath).
		WithField("output", req.OutputPath).
		Debug("Generating image thumbnail")

	// Open the source image
	file, err := os.Open(req.SourcePath)
	if err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to open image: %w", err)}
	}
	defer file.Close()

	// Decode the image
	img, format, err := image.Decode(file)
	if err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to decode image: %w", err)}
	}

	goLog.WithField("format", format).Debug("Decoded image")

	// Resize the image
	scaled := resizeImage(img, req.MaxWidth, req.MaxHeight)

	// Write the thumbnail as JPEG
	if err := writeJPEG(req.OutputPath, scaled, 80); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to write thumbnail: %w", err)}
	}

	return &ArtifactResult{
		OutputPath: req.OutputPath,
		MimeType:   "image/jpeg",
	}
}

// generateVideoPlaceholder creates a placeholder image for videos
func (g *GoClassifier) generateVideoPlaceholder(req *ArtifactRequest) *ArtifactResult {
	goLog.WithField("source", req.SourcePath).
		WithField("output", req.OutputPath).
		Debug("Generating video placeholder")

	// Create a simple colored placeholder
	// You could enhance this to show text, icons, etc.
	placeholder := createPlaceholderImage(req.MaxWidth, req.MaxHeight)

	if err := writeJPEG(req.OutputPath, placeholder, 80); err != nil {
		return &ArtifactResult{Error: fmt.Errorf("failed to write placeholder: %w", err)}
	}

	return &ArtifactResult{
		OutputPath: req.OutputPath,
		MimeType:   "image/jpeg",
	}
}

// resizeImage resizes an image to fit within maxWidth and maxHeight while preserving aspect ratio
func resizeImage(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate aspect ratio
	ratio := float64(width) / float64(height)

	// Calculate target dimensions
	targetW := maxWidth
	targetH := int(float64(targetW) / ratio)

	if targetH > maxHeight {
		targetH = maxHeight
		targetW = int(float64(targetH) * ratio)
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	// Scale using high-quality bilinear interpolation
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	return dst
}

// createPlaceholderImage creates a simple placeholder image
func createPlaceholderImage(width, height int) image.Image {
	// Create a gray rectangle as placeholder
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a gray color (RGB: 128, 128, 128)
	gray := image.NewUniform(color.RGBA{R: 128, G: 128, B: 128, A: 255})
	draw.Draw(img, img.Bounds(), gray, image.Point{}, draw.Src)

	return img
}

// writeJPEG writes an image to a file as JPEG
func writeJPEG(path string, img image.Image, quality int) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	return jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
}
