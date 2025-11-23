package classifier

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultMaxTextSize is the default maximum size for text content extraction (1MB)
	DefaultMaxTextSize int64 = 1024 * 1024

	// MaxPreviewSize is the size of text preview to store (first N bytes)
	MaxPreviewSize = 10240 // 10KB preview
)

// TextClassifier extracts text content and metadata from text files
type TextClassifier struct{}

// NewTextClassifier creates a new text classifier
func NewTextClassifier() *TextClassifier {
	return &TextClassifier{}
}

// Name returns the classifier name
func (t *TextClassifier) Name() string {
	return "Text Content Extractor"
}

// CanHandle checks if this is a text file
func (t *TextClassifier) CanHandle(filePath string) bool {
	return DetectFileType(filePath) == FileTypeText
}

// IsAvailable returns true (always available, no dependencies)
func (t *TextClassifier) IsAvailable() bool {
	return true
}

// Extract extracts text content and metadata from the file
func (t *TextClassifier) Extract(req *MetadataRequest) *MetadataResult {
	result := &MetadataResult{
		MetadataType: "text-content",
		Data:         make(map[string]interface{}),
	}

	// Open the file
	file, err := os.Open(req.SourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to open file: %w", err)
		return result
	}
	defer file.Close()

	// Get file stats
	stat, err := file.Stat()
	if err != nil {
		result.Error = fmt.Errorf("failed to stat file: %w", err)
		return result
	}

	// Determine max read size
	maxSize := req.MaxContentSize
	if maxSize == 0 {
		maxSize = DefaultMaxTextSize
	}

	// Read content (limited to maxSize)
	readSize := stat.Size()
	if readSize > maxSize {
		readSize = maxSize
	}

	content := make([]byte, readSize)
	n, err := io.ReadFull(file, content)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		result.Error = fmt.Errorf("failed to read file: %w", err)
		return result
	}
	content = content[:n]

	// Check if content is valid UTF-8
	isUTF8 := utf8.Valid(content)

	// Extract metadata
	result.Data["file_size"] = stat.Size()
	result.Data["is_utf8"] = isUTF8
	result.Data["truncated"] = stat.Size() > maxSize

	if isUTF8 {
		contentStr := string(content)
		result.Data["encoding"] = "utf-8"
		result.Data["line_count"] = countLines(contentStr)
		result.Data["char_count"] = utf8.RuneCountInString(contentStr)
		result.Data["word_count"] = countWords(contentStr)

		// Store preview (first MaxPreviewSize bytes)
		if len(content) > MaxPreviewSize {
			result.Data["preview"] = string(content[:MaxPreviewSize])
			result.Data["has_full_content"] = false
		} else {
			result.Data["content"] = contentStr
			result.Data["has_full_content"] = true
		}
	} else {
		result.Data["encoding"] = "binary"
		result.Data["note"] = "File contains non-UTF8 content"
	}

	return result
}

// countLines counts the number of lines in the text
func countLines(text string) int {
	if text == "" {
		return 0
	}
	lines := 1
	for _, ch := range text {
		if ch == '\n' {
			lines++
		}
	}
	return lines
}

// countWords counts the number of words in the text
func countWords(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}
