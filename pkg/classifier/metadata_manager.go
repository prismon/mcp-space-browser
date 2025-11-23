package classifier

import (
	"fmt"
	"os"
)

// MetadataManager orchestrates metadata extractors and provides a unified interface
type MetadataManager struct {
	extractors []MetadataExtractor
}

// NewMetadataManager creates a new metadata manager with default extractors
func NewMetadataManager() *MetadataManager {
	manager := &MetadataManager{
		extractors: make([]MetadataExtractor, 0),
	}

	// Register extractors in order of preference
	manager.RegisterExtractor(NewTextClassifier())
	manager.RegisterExtractor(NewAudioClassifier())

	return manager
}

// RegisterExtractor adds a metadata extractor to the manager
func (m *MetadataManager) RegisterExtractor(extractor MetadataExtractor) {
	m.extractors = append(m.extractors, extractor)
}

// GetExtractor returns the first available extractor that can handle the file
func (m *MetadataManager) GetExtractor(filePath string) MetadataExtractor {
	for _, extractor := range m.extractors {
		if extractor.IsAvailable() && extractor.CanHandle(filePath) {
			return extractor
		}
	}
	return nil
}

// ExtractMetadata extracts metadata from a file using the appropriate extractor
func (m *MetadataManager) ExtractMetadata(filePath string, maxContentSize int64) *MetadataResult {
	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return &MetadataResult{
			Error: fmt.Errorf("file not found: %w", err),
		}
	}

	// Find appropriate extractor
	extractor := m.GetExtractor(filePath)
	if extractor == nil {
		return &MetadataResult{
			Error: fmt.Errorf("no suitable extractor found for file: %s", filePath),
		}
	}

	// Detect file type
	fileType := DetectFileType(filePath)

	// Create request
	req := &MetadataRequest{
		SourcePath:     filePath,
		FileType:       fileType,
		MaxContentSize: maxContentSize,
	}

	// Extract metadata
	return extractor.Extract(req)
}

// CanExtractMetadata checks if metadata can be extracted from the given file
func (m *MetadataManager) CanExtractMetadata(filePath string) bool {
	return m.GetExtractor(filePath) != nil
}

// ListExtractors returns information about all registered extractors
func (m *MetadataManager) ListExtractors() []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(m.extractors))

	for _, extractor := range m.extractors {
		info := map[string]interface{}{
			"name":      extractor.Name(),
			"available": extractor.IsAvailable(),
		}
		result = append(result, info)
	}

	return result
}
