package classifier

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessor(t *testing.T) {
	config := &ProcessorConfig{
		CacheDir: "/tmp/cache",
	}
	processor := NewProcessor(config)
	assert.NotNil(t, processor)
	assert.Equal(t, "/tmp/cache", processor.config.CacheDir)
}

func TestProcessorResolveResource(t *testing.T) {
	// Create temp directory and file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("fake image content"), 0644)
	require.NoError(t, err)

	// Create processor
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	// Test file:// URL resolution
	fileURL := "file://" + testFile
	localPath, cleanup, err := processor.resolveResource(fileURL)
	assert.NoError(t, err)
	assert.Equal(t, testFile, localPath)
	assert.Nil(t, cleanup)
}

func TestProcessorGenerateHashKey(t *testing.T) {
	config := &ProcessorConfig{
		CacheDir: "/tmp/cache",
	}
	processor := NewProcessor(config)

	hash := processor.generateHashKey("/test/path.jpg", 1234567890)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // Full SHA256 hex string

	// Same inputs should produce same hash
	hash2 := processor.generateHashKey("/test/path.jpg", 1234567890)
	assert.Equal(t, hash, hash2)

	// Different inputs should produce different hash
	hash3 := processor.generateHashKey("/test/other.jpg", 1234567890)
	assert.NotEqual(t, hash, hash3)
}

func TestProcessorArtifactCachePath(t *testing.T) {
	tempDir := t.TempDir()
	config := &ProcessorConfig{
		CacheDir: tempDir,
	}
	processor := NewProcessor(config)

	// Test thumbnail cache path - uses hashkey for directory structure
	path, err := processor.artifactCachePath("abc123def456", "thumb.jpg")
	assert.NoError(t, err)
	assert.Contains(t, path, tempDir)
	assert.Contains(t, path, "ab")       // First 2 chars of hash
	assert.Contains(t, path, "c1")       // Chars 3-4 of hash
	assert.Contains(t, path, "thumb.jpg") // Filename

	// Test with different hash
	path2, err := processor.artifactCachePath("xyz789abcdef", "timeline.json")
	assert.NoError(t, err)
	assert.Contains(t, path2, "xy")             // First 2 chars
	assert.Contains(t, path2, "z7")             // Chars 3-4
	assert.Contains(t, path2, "timeline.json")  // Filename

	// Test error case - hash too short
	_, err = processor.artifactCachePath("ab", "test.jpg")
	assert.Error(t, err)
}

func TestProcessorValidateFilePath(t *testing.T) {
	// Create temp file for valid path tests
	tempDir := t.TempDir()
	validFile := filepath.Join(tempDir, "valid.jpg")
	err := os.WriteFile(validFile, []byte("test"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name      string
		path      string
		shouldErr bool
	}{
		{"valid file", validFile, false},
		{"empty path", "", true},
		{"nonexistent file", "/nonexistent/path.jpg", true},
		{"command injection semicolon", "/path;rm -rf /", true},
		{"command injection pipe", "/path|cat /etc/passwd", true},
		{"command injection ampersand", "/path&echo test", true},
		{"command injection backtick", "/path`whoami`", true},
		{"command injection dollar", "/path$HOME", true},
		{"command injection brackets", "/path[test]", true},
		{"command injection parens", "/path(test)", true},
		{"command injection braces", "/path{test}", true},
		{"command injection asterisk", "/path*.jpg", true},
		{"command injection question", "/path?.jpg", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilePath(tc.path)
			if tc.shouldErr {
				assert.Error(t, err, "path: %s should error", tc.path)
			} else {
				assert.NoError(t, err, "path: %s should not error", tc.path)
			}
		})
	}
}

func TestProcessResourceInvalidURL(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "invalid://url",
		ArtifactTypes: []string{"thumbnail"},
	}

	_, err = processor.ProcessResource(req)
	assert.Error(t, err)
}

func TestProcessResourceNonexistentFile(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "file:///nonexistent/file.jpg",
		ArtifactTypes: []string{"thumbnail"},
	}

	_, err = processor.ProcessResource(req)
	assert.Error(t, err)
}

func TestProcessResourceUnsupportedMediaType(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file with unsupported extension
	testFile := filepath.Join(tempDir, "test.xyz")
	err := os.WriteFile(testFile, []byte("content"), 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"thumbnail"},
	}

	_, err = processor.ProcessResource(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported media type")
}

func TestProcessResourceWithTextFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a text file - note: text files are not a supported MediaType
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Hello world, this is test content."), 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	// Text files have MediaTypeUnknown so ProcessResource should fail
	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"metadata"},
	}

	_, err = processor.ProcessResource(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported media type")
}

func TestProcessResourceWithImageDefaults(t *testing.T) {
	tempDir := t.TempDir()

	// Create a minimal valid PNG file (1x1 pixel)
	testFile := filepath.Join(tempDir, "test.png")
	// PNG header + IHDR + IDAT + IEND for a 1x1 white pixel
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR length and type
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, // bit depth, color type, CRC
		0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, // IDAT length and type
		0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0xff, 0x00, 0x05, 0xfe, 0x02, 0xfe, // compressed data
		0xa3, 0x6c, 0xce, 0x3e, // CRC
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, // IEND
		0xae, 0x42, 0x60, 0x82, // CRC
	}
	err := os.WriteFile(testFile, pngData, 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	// Test with no artifact types (should use defaults for image)
	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: nil, // Use defaults
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Should have attempted thumbnail and metadata
}

func TestProcessResourceWithVideoFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a minimal video file (MP4 header, not valid video but has extension)
	testFile := filepath.Join(tempDir, "test.mp4")
	err := os.WriteFile(testFile, []byte("fake video content"), 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	// Test with no artifact types (should use defaults for video)
	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: nil, // Use defaults
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Should have attempted thumbnail, timeline, and metadata
	// Result may have errors or not depending on ffmpeg availability
}

func TestProcessResourceTimeline(t *testing.T) {
	tempDir := t.TempDir()

	// Create a minimal video file
	testFile := filepath.Join(tempDir, "test.mp4")
	err := os.WriteFile(testFile, []byte("fake video content"), 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	// Test with explicit timeline artifact type
	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"timeline"},
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Timeline may or may not have errors depending on ffmpeg availability
}

func TestResolveResourceVariousSchemes(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	// Test invalid URL parse
	_, _, err = processor.resolveResource("://invalid")
	assert.Error(t, err)

	// Test unsupported scheme
	_, _, err = processor.resolveResource("ftp://example.com/file.jpg")
	assert.Error(t, err)

	// Test synthesis:// scheme (requires database setup)
	_, _, err = processor.resolveResource("synthesis://test/resource")
	assert.Error(t, err) // Will fail because resource doesn't exist
}

func TestProcessResourceWithJpegThumbnail(t *testing.T) {
	tempDir := t.TempDir()

	// Create a small valid JPEG file
	testFile := filepath.Join(tempDir, "test.jpg")
	// Create an actual image using Go's image library
	createTestJpegImage(t, testFile, 100, 100)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"thumbnail"},
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Should have produced thumbnail artifact
	assert.Len(t, result.Artifacts, 1)
	assert.Equal(t, "thumbnail", result.Artifacts[0].Type)
}

func TestProcessResourceWithLargeImage(t *testing.T) {
	tempDir := t.TempDir()

	// Create a larger test image
	testFile := filepath.Join(tempDir, "large.jpg")
	createTestJpegImage(t, testFile, 1000, 1000)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"thumbnail"},
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestProcessResourceWithGifImage(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test image with GIF extension
	testFile := filepath.Join(tempDir, "test.gif")
	// Create as JPEG but with gif extension - Go's image decoder will handle it
	createTestJpegImage(t, testFile, 100, 100)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"thumbnail"},
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestProcessResourceMultipleArtifactTypes(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.mp4")
	err := os.WriteFile(testFile, []byte("fake video"), 0644)
	require.NoError(t, err)

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	config := &ProcessorConfig{
		CacheDir:          tempDir,
		ClassifierManager: NewManager(),
		MetadataManager:   NewMetadataManager(),
		Database:          db,
	}
	processor := NewProcessor(config)

	req := &ProcessRequest{
		ResourceURL:   "file://" + testFile,
		ArtifactTypes: []string{"thumbnail", "timeline"},
	}

	result, err := processor.ProcessResource(req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// Helper function to create test JPEG images
func createTestJpegImage(t *testing.T, path string, width, height int) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a color pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: 128,
				A: 255,
			})
		}
	}

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 80})
	require.NoError(t, err)
}
