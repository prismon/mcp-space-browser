package classifier

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// FileType represents the type of file that can be processed by metadata extractors
type FileType string

const (
	FileTypeText     FileType = "text"
	FileTypeAudio    FileType = "audio"
	FileTypeDocument FileType = "document"
	FileTypeUnknown  FileType = "unknown"
)

// MetadataExtractor is an interface for extracting structured metadata from files
// Unlike Classifier which generates visual artifacts, MetadataExtractor extracts
// and stores file-specific metadata (e.g., text content, ID3 tags, EXIF data)
type MetadataExtractor interface {
	// Name returns a human-readable name for this extractor
	Name() string

	// CanHandle returns true if this extractor can process the given file
	CanHandle(filePath string) bool

	// IsAvailable returns true if this extractor has all required dependencies
	IsAvailable() bool

	// Extract extracts metadata from the file and returns it as a MetadataResult
	Extract(req *MetadataRequest) *MetadataResult
}

// MetadataRequest contains the parameters for metadata extraction
type MetadataRequest struct {
	SourcePath string // Path to the source file
	FileType   FileType
	// Optional parameters for limiting extraction
	MaxContentSize int64 // Maximum bytes of content to extract (0 = no limit)
}

// MetadataResult contains the extracted metadata
type MetadataResult struct {
	MetadataType string                 // Type identifier (e.g., "text-content", "audio-metadata")
	Data         map[string]interface{} // Structured metadata
	Error        error                  // Error if extraction failed
}

// ToJSON converts the metadata to JSON string
func (m *MetadataResult) ToJSON() (string, error) {
	if m.Error != nil {
		return "", m.Error
	}
	data, err := json.Marshal(m.Data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}
	return string(data), nil
}

// DetectFileType determines the file type based on file path/extension
func DetectFileType(filePath string) FileType {
	ext := getFileExtension(filePath)

	switch ext {
	case ".txt", ".md", ".log", ".json", ".xml", ".yaml", ".yml", ".csv", ".html", ".htm":
		return FileTypeText
	case ".mp3", ".m4a", ".flac", ".ogg", ".wav":
		return FileTypeAudio
	case ".pdf", ".doc", ".docx":
		return FileTypeDocument
	default:
		return FileTypeUnknown
	}
}

// getFileExtension returns the lowercase file extension
func getFileExtension(filePath string) string {
	return strings.ToLower(filepath.Ext(filePath))
}
