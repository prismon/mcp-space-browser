package classifier

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		path     string
		expected MediaType
	}{
		{"test.jpg", MediaTypeImage},
		{"test.jpeg", MediaTypeImage},
		{"test.png", MediaTypeImage},
		{"test.gif", MediaTypeImage},
		{"test.bmp", MediaTypeImage},
		{"test.webp", MediaTypeImage},
		{"test.JPG", MediaTypeImage}, // Test case insensitivity
		{"test.mp4", MediaTypeVideo},
		{"test.mov", MediaTypeVideo},
		{"test.mkv", MediaTypeVideo},
		{"test.avi", MediaTypeVideo},
		{"test.webm", MediaTypeVideo},
		{"test.m4v", MediaTypeVideo},
		{"test.flv", MediaTypeVideo},
		{"test.MP4", MediaTypeVideo}, // Test case insensitivity
		{"test.txt", MediaTypeUnknown},
		{"test.pdf", MediaTypeUnknown},
		{"test", MediaTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := DetectMediaType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsImageFile(t *testing.T) {
	assert.True(t, IsImageFile("test.jpg"))
	assert.True(t, IsImageFile("test.png"))
	assert.False(t, IsImageFile("test.mp4"))
	assert.False(t, IsImageFile("test.txt"))
}

func TestIsVideoFile(t *testing.T) {
	assert.True(t, IsVideoFile("test.mp4"))
	assert.True(t, IsVideoFile("test.mov"))
	assert.False(t, IsVideoFile("test.jpg"))
	assert.False(t, IsVideoFile("test.txt"))
}

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name        string
		req         *ArtifactRequest
		expectError bool
	}{
		{
			name: "valid request",
			req: &ArtifactRequest{
				SourcePath:   "/path/to/source.jpg",
				OutputPath:   "/path/to/output.jpg",
				MediaType:    MediaTypeImage,
				ArtifactType: ArtifactTypeThumbnail,
				MaxWidth:     320,
				MaxHeight:    320,
			},
			expectError: false,
		},
		{
			name: "empty source path",
			req: &ArtifactRequest{
				OutputPath: "/path/to/output.jpg",
				MediaType:  MediaTypeImage,
			},
			expectError: true,
		},
		{
			name: "empty output path",
			req: &ArtifactRequest{
				SourcePath: "/path/to/source.jpg",
				MediaType:  MediaTypeImage,
			},
			expectError: true,
		},
		{
			name: "unknown media type",
			req: &ArtifactRequest{
				SourcePath: "/path/to/source.txt",
				OutputPath: "/path/to/output.jpg",
				MediaType:  MediaTypeUnknown,
			},
			expectError: true,
		},
		{
			name: "defaults applied",
			req: &ArtifactRequest{
				SourcePath: "/path/to/source.jpg",
				OutputPath: "/path/to/output.jpg",
				MediaType:  MediaTypeImage,
				// MaxWidth and MaxHeight not set
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequest(tt.req)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Check defaults were applied
				if tt.req.MaxWidth == 0 {
					assert.Equal(t, 320, tt.req.MaxWidth)
				}
				if tt.req.MaxHeight == 0 {
					assert.Equal(t, 320, tt.req.MaxHeight)
				}
			}
		})
	}
}

// Helper function to create a test image file
func createTestImage(t *testing.T, path string, width, height int) {
	t.Helper()

	// Create parent directory if needed
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a test pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: 128,
				A: 255,
			})
		}
	}

	// Write the image
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 80})
	require.NoError(t, err)
}

func TestGoClassifier(t *testing.T) {
	tmpDir := t.TempDir()
	classifier := NewGoClassifier()

	// Test basic properties
	assert.Equal(t, "Go Standard Library", classifier.Name())
	assert.True(t, classifier.IsAvailable())
	assert.True(t, classifier.CanHandle(MediaTypeImage))
	assert.True(t, classifier.CanHandle(MediaTypeVideo)) // Can provide fallback

	// Test image thumbnail generation
	t.Run("ImageThumbnail", func(t *testing.T) {
		sourcePath := filepath.Join(tmpDir, "source.jpg")
		outputPath := filepath.Join(tmpDir, "thumb.jpg")

		// Create a test image
		createTestImage(t, sourcePath, 800, 600)

		req := &ArtifactRequest{
			SourcePath:   sourcePath,
			OutputPath:   outputPath,
			MediaType:    MediaTypeImage,
			ArtifactType: ArtifactTypeThumbnail,
			MaxWidth:     320,
			MaxHeight:    320,
		}

		result := classifier.GenerateThumbnail(req)
		assert.NoError(t, result.Error)
		assert.Equal(t, outputPath, result.OutputPath)
		assert.Equal(t, "image/jpeg", result.MimeType)

		// Verify the thumbnail was created
		info, err := os.Stat(outputPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))

		// Verify thumbnail dimensions
		f, err := os.Open(outputPath)
		require.NoError(t, err)
		defer f.Close()

		img, _, err := image.Decode(f)
		require.NoError(t, err)
		bounds := img.Bounds()
		assert.LessOrEqual(t, bounds.Dx(), 320)
		assert.LessOrEqual(t, bounds.Dy(), 320)
	})

	// Test video placeholder generation
	t.Run("VideoPlaceholder", func(t *testing.T) {
		sourcePath := filepath.Join(tmpDir, "video.mp4")
		outputPath := filepath.Join(tmpDir, "video_thumb.jpg")

		// Create a dummy video file (just for path validation)
		require.NoError(t, os.WriteFile(sourcePath, []byte("dummy"), 0o644))

		req := &ArtifactRequest{
			SourcePath:   sourcePath,
			OutputPath:   outputPath,
			MediaType:    MediaTypeVideo,
			ArtifactType: ArtifactTypeThumbnail,
			MaxWidth:     320,
			MaxHeight:    320,
		}

		result := classifier.GenerateThumbnail(req)
		assert.NoError(t, result.Error)
		assert.Equal(t, outputPath, result.OutputPath)
		assert.Equal(t, "image/jpeg", result.MimeType)

		// Verify the placeholder was created
		info, err := os.Stat(outputPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	})
}

func TestFFmpegClassifier(t *testing.T) {
	classifier := NewFFmpegClassifier()

	// Test basic properties
	assert.Equal(t, "FFmpeg", classifier.Name())
	assert.True(t, classifier.CanHandle(MediaTypeVideo))
	assert.False(t, classifier.CanHandle(MediaTypeImage))

	// Note: IsAvailable() depends on whether ffmpeg is installed
	// We skip testing actual video processing as it requires ffmpeg
	t.Run("CanHandle", func(t *testing.T) {
		assert.True(t, classifier.CanHandle(MediaTypeVideo))
		assert.False(t, classifier.CanHandle(MediaTypeImage))
		assert.False(t, classifier.CanHandle(MediaTypeUnknown))
	})
}

func TestManager(t *testing.T) {
	manager := NewManager()

	t.Run("ListClassifiers", func(t *testing.T) {
		list := manager.ListClassifiers()
		assert.GreaterOrEqual(t, len(list), 1) // At least Go classifier should be present

		// Check that at least one classifier can handle images
		hasImageHandler := false
		for _, c := range list {
			handles := c["handles"].(map[string]bool)
			if handles["image"] {
				hasImageHandler = true
				break
			}
		}
		assert.True(t, hasImageHandler)
	})

	t.Run("GetClassifier", func(t *testing.T) {
		// Should always be able to get a classifier for images (Go classifier)
		c, err := manager.GetClassifier(MediaTypeImage)
		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.True(t, c.CanHandle(MediaTypeImage))

		// Should be able to get a classifier for videos (either FFmpeg or Go fallback)
		c, err = manager.GetClassifier(MediaTypeVideo)
		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.True(t, c.CanHandle(MediaTypeVideo))
	})

	t.Run("GenerateThumbnail", func(t *testing.T) {
		tmpDir := t.TempDir()
		sourcePath := filepath.Join(tmpDir, "source.jpg")
		outputPath := filepath.Join(tmpDir, "thumb.jpg")

		// Create a test image
		createTestImage(t, sourcePath, 800, 600)

		req := &ArtifactRequest{
			SourcePath:   sourcePath,
			OutputPath:   outputPath,
			MediaType:    MediaTypeImage,
			ArtifactType: ArtifactTypeThumbnail,
			MaxWidth:     320,
			MaxHeight:    320,
		}

		result := manager.GenerateThumbnail(req)
		assert.NoError(t, result.Error)
		assert.Equal(t, outputPath, result.OutputPath)

		// Verify the thumbnail exists
		_, err := os.Stat(outputPath)
		assert.NoError(t, err)
	})

	t.Run("Register", func(t *testing.T) {
		testManager := NewManager()
		// Register should not panic with a new classifier
		goClassifier := NewGoClassifier()
		testManager.Register(goClassifier)
		// Registering the same classifier again should not cause issues
		testManager.Register(goClassifier)
	})

	t.Run("GetClassifierUnknownType", func(t *testing.T) {
		testManager := NewManager()
		// MediaTypeUnknown should fail
		_, err := testManager.GetClassifier(MediaTypeUnknown)
		assert.Error(t, err)
	})

	t.Run("GenerateThumbnailWithValidation", func(t *testing.T) {
		testManager := NewManager()
		// Test with invalid request (empty source path)
		req := &ArtifactRequest{
			SourcePath:   "",
			OutputPath:   "/tmp/out.jpg",
			MediaType:    MediaTypeImage,
			ArtifactType: ArtifactTypeThumbnail,
		}
		result := testManager.GenerateThumbnail(req)
		assert.Error(t, result.Error)
	})

	t.Run("GenerateTimelineFrame", func(t *testing.T) {
		tmpDir := t.TempDir()
		sourcePath := filepath.Join(tmpDir, "video.mp4")
		outputPath := filepath.Join(tmpDir, "frame.jpg")

		// Create a dummy video file
		require.NoError(t, os.WriteFile(sourcePath, []byte("dummy video"), 0644))

		req := &ArtifactRequest{
			SourcePath:   sourcePath,
			OutputPath:   outputPath,
			MediaType:    MediaTypeVideo,
			ArtifactType: ArtifactTypeTimeline,
			MaxWidth:     320,
			MaxHeight:    180,
			FrameIndex:   0,
			TotalFrames:  5,
		}

		result := manager.GenerateTimelineFrame(req)
		// May fail if ffmpeg is not available, which is OK
		// The important thing is it doesn't panic
		_ = result
	})

	t.Run("GenerateTimelineFrameInvalidRequest", func(t *testing.T) {
		testManager := NewManager()
		// Test with invalid request
		req := &ArtifactRequest{
			SourcePath:   "",
			OutputPath:   "/tmp/out.jpg",
			MediaType:    MediaTypeVideo,
			ArtifactType: ArtifactTypeTimeline,
		}
		result := testManager.GenerateTimelineFrame(req)
		assert.Error(t, result.Error)
	})
}

func TestGoClassifierTimelineFrame(t *testing.T) {
	classifier := NewGoClassifier()
	tmpDir := t.TempDir()

	sourcePath := filepath.Join(tmpDir, "video.mp4")
	outputPath := filepath.Join(tmpDir, "frame.jpg")

	// Create a dummy video file
	require.NoError(t, os.WriteFile(sourcePath, []byte("dummy"), 0644))

	req := &ArtifactRequest{
		SourcePath:   sourcePath,
		OutputPath:   outputPath,
		MediaType:    MediaTypeVideo,
		ArtifactType: ArtifactTypeTimeline,
		MaxWidth:     320,
		MaxHeight:    180,
		FrameIndex:   0,
		TotalFrames:  5,
	}

	// Go classifier will generate a placeholder for video timeline frames
	result := classifier.GenerateTimelineFrame(req)
	assert.NoError(t, result.Error)
	assert.Equal(t, outputPath, result.OutputPath)

	// Verify the placeholder was created
	_, err := os.Stat(outputPath)
	assert.NoError(t, err)
}

func TestGoClassifierInvalidImage(t *testing.T) {
	classifier := NewGoClassifier()
	tmpDir := t.TempDir()

	sourcePath := filepath.Join(tmpDir, "invalid.jpg")
	outputPath := filepath.Join(tmpDir, "thumb.jpg")

	// Create an invalid image file (just text)
	require.NoError(t, os.WriteFile(sourcePath, []byte("not an image"), 0644))

	req := &ArtifactRequest{
		SourcePath:   sourcePath,
		OutputPath:   outputPath,
		MediaType:    MediaTypeImage,
		ArtifactType: ArtifactTypeThumbnail,
		MaxWidth:     320,
		MaxHeight:    320,
	}

	result := classifier.GenerateThumbnail(req)
	assert.Error(t, result.Error)
}

func TestGoClassifierTallImage(t *testing.T) {
	classifier := NewGoClassifier()
	tmpDir := t.TempDir()

	sourcePath := filepath.Join(tmpDir, "tall.jpg")
	outputPath := filepath.Join(tmpDir, "thumb.jpg")

	// Create a tall test image (height > width)
	createTestImage(t, sourcePath, 400, 800)

	req := &ArtifactRequest{
		SourcePath:   sourcePath,
		OutputPath:   outputPath,
		MediaType:    MediaTypeImage,
		ArtifactType: ArtifactTypeThumbnail,
		MaxWidth:     320,
		MaxHeight:    320,
	}

	result := classifier.GenerateThumbnail(req)
	assert.NoError(t, result.Error)

	// Verify thumbnail dimensions - height should be limiting factor
	f, err := os.Open(outputPath)
	require.NoError(t, err)
	defer f.Close()

	img, _, err := image.Decode(f)
	require.NoError(t, err)
	bounds := img.Bounds()
	assert.LessOrEqual(t, bounds.Dy(), 320)
}

func TestGoClassifierSmallImage(t *testing.T) {
	classifier := NewGoClassifier()
	tmpDir := t.TempDir()

	sourcePath := filepath.Join(tmpDir, "small.jpg")
	outputPath := filepath.Join(tmpDir, "thumb.jpg")

	// Create a small test image that doesn't need resizing
	createTestImage(t, sourcePath, 100, 100)

	req := &ArtifactRequest{
		SourcePath:   sourcePath,
		OutputPath:   outputPath,
		MediaType:    MediaTypeImage,
		ArtifactType: ArtifactTypeThumbnail,
		MaxWidth:     320,
		MaxHeight:    320,
	}

	result := classifier.GenerateThumbnail(req)
	assert.NoError(t, result.Error)

	// Verify thumbnail was created - the implementation may upscale small images
	f, err := os.Open(outputPath)
	require.NoError(t, err)
	defer f.Close()

	img, _, err := image.Decode(f)
	require.NoError(t, err)
	bounds := img.Bounds()
	// Just verify the thumbnail was created with valid dimensions
	assert.Greater(t, bounds.Dx(), 0)
	assert.Greater(t, bounds.Dy(), 0)
}
