package classifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextClassifier(t *testing.T) {
	classifier := NewTextClassifier()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "Text Content Extractor", classifier.Name())
	})

	t.Run("IsAvailable", func(t *testing.T) {
		assert.True(t, classifier.IsAvailable())
	})

	t.Run("CanHandle", func(t *testing.T) {
		tests := []struct {
			path     string
			expected bool
		}{
			{"test.txt", true},
			{"test.md", true},
			{"test.log", true},
			{"test.json", true},
			{"test.xml", true},
			{"test.mp3", false},
			{"test.mp4", false},
			{"test.jpg", false},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				assert.Equal(t, tt.expected, classifier.CanHandle(tt.path))
			})
		}
	})

	t.Run("Extract - Simple Text", func(t *testing.T) {
		// Create temporary test file
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		content := "Hello World\nThis is a test file.\nIt has three lines."
		err := os.WriteFile(testFile, []byte(content), 0o644)
		require.NoError(t, err)

		req := &MetadataRequest{
			SourcePath:     testFile,
			FileType:       FileTypeText,
			MaxContentSize: 0, // Use default
		}

		result := classifier.Extract(req)
		require.NoError(t, result.Error)
		assert.Equal(t, "text-content", result.MetadataType)

		// Check extracted metadata
		assert.Equal(t, int64(len(content)), result.Data["file_size"])
		assert.Equal(t, true, result.Data["is_utf8"])
		assert.Equal(t, "utf-8", result.Data["encoding"])
		assert.Equal(t, 3, result.Data["line_count"])
		assert.Equal(t, 11, result.Data["word_count"]) // "Hello World This is a test file It has three lines"
		assert.Equal(t, len([]rune(content)), result.Data["char_count"])
		assert.Equal(t, content, result.Data["content"])
		assert.Equal(t, true, result.Data["has_full_content"])
		assert.Equal(t, false, result.Data["truncated"])
	})

	t.Run("Extract - Large Text with Preview", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "large.txt")

		// Create a large text file (larger than MaxPreviewSize)
		largeContent := ""
		for i := 0; i < 2000; i++ {
			largeContent += "This is line number " + string(rune(i)) + "\n"
		}
		err := os.WriteFile(testFile, []byte(largeContent), 0o644)
		require.NoError(t, err)

		req := &MetadataRequest{
			SourcePath:     testFile,
			FileType:       FileTypeText,
			MaxContentSize: 0,
		}

		result := classifier.Extract(req)
		require.NoError(t, result.Error)

		// Should have preview, not full content
		assert.NotNil(t, result.Data["preview"])
		assert.Equal(t, false, result.Data["has_full_content"])
	})

	t.Run("Extract - Truncated Content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "truncated.txt")

		// Create a file larger than our max content size
		largeContent := ""
		for i := 0; i < 100000; i++ {
			largeContent += "x"
		}
		err := os.WriteFile(testFile, []byte(largeContent), 0o644)
		require.NoError(t, err)

		req := &MetadataRequest{
			SourcePath:     testFile,
			FileType:       FileTypeText,
			MaxContentSize: 10000, // Limit to 10KB
		}

		result := classifier.Extract(req)
		require.NoError(t, result.Error)

		// Should be marked as truncated
		assert.Equal(t, true, result.Data["truncated"])
	})

	t.Run("Extract - Nonexistent File", func(t *testing.T) {
		req := &MetadataRequest{
			SourcePath: "/nonexistent/file.txt",
			FileType:   FileTypeText,
		}

		result := classifier.Extract(req)
		assert.Error(t, result.Error)
	})
}

func TestAudioClassifier(t *testing.T) {
	classifier := NewAudioClassifier()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "Audio Metadata Extractor", classifier.Name())
	})

	t.Run("IsAvailable", func(t *testing.T) {
		assert.True(t, classifier.IsAvailable())
	})

	t.Run("CanHandle", func(t *testing.T) {
		tests := []struct {
			path     string
			expected bool
		}{
			{"test.mp3", true},
			{"test.m4a", true},
			{"test.flac", true},
			{"test.ogg", true},
			{"test.wav", true},
			{"test.txt", false},
			{"test.mp4", false},
			{"test.jpg", false},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				assert.Equal(t, tt.expected, classifier.CanHandle(tt.path))
			})
		}
	})

	// Note: We can't easily test audio extraction without creating valid audio files
	// In a real scenario, you would have test fixtures with sample MP3/audio files
	t.Run("Extract - Invalid Audio File", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "invalid.mp3")
		err := os.WriteFile(testFile, []byte("not a real mp3 file"), 0o644)
		require.NoError(t, err)

		req := &MetadataRequest{
			SourcePath: testFile,
			FileType:   FileTypeAudio,
		}

		result := classifier.Extract(req)
		// Should error because it's not a valid audio file
		assert.Error(t, result.Error)
	})
}

func TestMetadataManager(t *testing.T) {
	manager := NewMetadataManager()

	t.Run("ListExtractors", func(t *testing.T) {
		extractors := manager.ListExtractors()
		assert.NotEmpty(t, extractors)

		// Should have at least text and audio extractors
		names := make([]string, len(extractors))
		for i, e := range extractors {
			names[i] = e["name"].(string)
		}
		assert.Contains(t, names, "Text Content Extractor")
		assert.Contains(t, names, "Audio Metadata Extractor")
	})

	t.Run("GetExtractor", func(t *testing.T) {
		tests := []struct {
			path        string
			expectNil   bool
			expectName  string
		}{
			{"test.txt", false, "Text Content Extractor"},
			{"test.mp3", false, "Audio Metadata Extractor"},
			{"test.mp4", true, ""},
			{"test.jpg", true, ""},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				extractor := manager.GetExtractor(tt.path)
				if tt.expectNil {
					assert.Nil(t, extractor)
				} else {
					require.NotNil(t, extractor)
					assert.Equal(t, tt.expectName, extractor.Name())
				}
			})
		}
	})

	t.Run("CanExtractMetadata", func(t *testing.T) {
		tests := []struct {
			path     string
			expected bool
		}{
			{"test.txt", true},
			{"test.mp3", true},
			{"test.mp4", false},
			{"test.jpg", false},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				assert.Equal(t, tt.expected, manager.CanExtractMetadata(tt.path))
			})
		}
	})

	t.Run("ExtractMetadata - Text File", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		content := "Test content"
		err := os.WriteFile(testFile, []byte(content), 0o644)
		require.NoError(t, err)

		result := manager.ExtractMetadata(testFile, 0)
		require.NoError(t, result.Error)
		assert.Equal(t, "text-content", result.MetadataType)
		assert.NotEmpty(t, result.Data)
	})

	t.Run("ExtractMetadata - Nonexistent File", func(t *testing.T) {
		result := manager.ExtractMetadata("/nonexistent/file.txt", 0)
		assert.Error(t, result.Error)
	})

	t.Run("ExtractMetadata - Unsupported File", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.jpg")
		err := os.WriteFile(testFile, []byte("fake image"), 0o644)
		require.NoError(t, err)

		result := manager.ExtractMetadata(testFile, 0)
		assert.Error(t, result.Error)
		assert.Contains(t, result.Error.Error(), "no suitable extractor found")
	})
}

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		path     string
		expected FileType
	}{
		{"test.txt", FileTypeText},
		{"test.md", FileTypeText},
		{"test.log", FileTypeText},
		{"test.json", FileTypeText},
		{"test.xml", FileTypeText},
		{"test.yaml", FileTypeText},
		{"test.yml", FileTypeText},
		{"test.csv", FileTypeText},
		{"test.html", FileTypeText},
		{"test.htm", FileTypeText},
		{"test.mp3", FileTypeAudio},
		{"test.m4a", FileTypeAudio},
		{"test.flac", FileTypeAudio},
		{"test.ogg", FileTypeAudio},
		{"test.wav", FileTypeAudio},
		{"test.pdf", FileTypeDocument},
		{"test.doc", FileTypeDocument},
		{"test.docx", FileTypeDocument},
		{"test.mp4", FileTypeUnknown},
		{"test.jpg", FileTypeUnknown},
		{"test.unknown", FileTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, DetectFileType(tt.path))
		})
	}
}

func TestMetadataResult_ToJSON(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		result := &MetadataResult{
			MetadataType: "text-content",
			Data: map[string]interface{}{
				"foo": "bar",
				"baz": 123,
			},
		}

		json, err := result.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, json, "foo")
		assert.Contains(t, json, "bar")
		assert.Contains(t, json, "baz")
	})

	t.Run("Error", func(t *testing.T) {
		result := &MetadataResult{
			Error: assert.AnError,
		}

		_, err := result.ToJSON()
		assert.Error(t, err)
	})
}

func TestTextClassifierEmptyFile(t *testing.T) {
	classifier := NewTextClassifier()
	tmpDir := t.TempDir()

	// Create empty file
	testFile := filepath.Join(tmpDir, "empty.txt")
	err := os.WriteFile(testFile, []byte{}, 0o644)
	require.NoError(t, err)

	req := &MetadataRequest{
		SourcePath:     testFile,
		FileType:       FileTypeText,
		MaxContentSize: 0,
	}

	result := classifier.Extract(req)
	require.NoError(t, result.Error)
	assert.Equal(t, int64(0), result.Data["file_size"])
	assert.Equal(t, 0, result.Data["line_count"])
	assert.Equal(t, 0, result.Data["word_count"])
}

func TestTextClassifierSingleLine(t *testing.T) {
	classifier := NewTextClassifier()
	tmpDir := t.TempDir()

	// Create single line file without newline
	testFile := filepath.Join(tmpDir, "single.txt")
	content := "Single line without newline"
	err := os.WriteFile(testFile, []byte(content), 0o644)
	require.NoError(t, err)

	req := &MetadataRequest{
		SourcePath:     testFile,
		FileType:       FileTypeText,
		MaxContentSize: 0,
	}

	result := classifier.Extract(req)
	require.NoError(t, result.Error)
	assert.Equal(t, 1, result.Data["line_count"])
	assert.Equal(t, 4, result.Data["word_count"]) // "Single" "line" "without" "newline"
}

func TestTextClassifierMultipleSpaces(t *testing.T) {
	classifier := NewTextClassifier()
	tmpDir := t.TempDir()

	// Create file with multiple spaces between words
	testFile := filepath.Join(tmpDir, "spaces.txt")
	content := "Word1    Word2\t\tWord3\nWord4"
	err := os.WriteFile(testFile, []byte(content), 0o644)
	require.NoError(t, err)

	req := &MetadataRequest{
		SourcePath:     testFile,
		FileType:       FileTypeText,
		MaxContentSize: 0,
	}

	result := classifier.Extract(req)
	require.NoError(t, result.Error)
	assert.Equal(t, 4, result.Data["word_count"])
}

func TestAudioClassifierNonexistentFile(t *testing.T) {
	classifier := NewAudioClassifier()

	req := &MetadataRequest{
		SourcePath: "/nonexistent/audio.mp3",
		FileType:   FileTypeAudio,
	}

	result := classifier.Extract(req)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to open file")
}

