package models

import "fmt"

// MetadataRecord represents a unified metadata entry for a filesystem entry.
// Simple metadata (mime, permissions, hashes) has Hash == nil.
// Artifact metadata (thumbnails, timelines) has a non-nil Hash for deduplication.
type MetadataRecord struct {
	EntryPath string  `db:"entry_path" json:"entry_path"`
	Key       string  `db:"key" json:"key"`
	Value     *string `db:"value" json:"value,omitempty"`
	Source    string  `db:"source" json:"source"`
	CachePath *string `db:"cache_path" json:"cache_path,omitempty"`
	DataJson  *string `db:"data_json" json:"data_json,omitempty"`
	MimeType  *string `db:"mime_type" json:"mime_type,omitempty"`
	FileSize  int64   `db:"file_size" json:"file_size"`
	Generator *string `db:"generator" json:"generator,omitempty"`
	Hash      *string `db:"hash" json:"hash,omitempty"`
	CreatedAt int64   `db:"created_at" json:"created_at"`
	UpdatedAt int64   `db:"updated_at" json:"updated_at"`
	HttpUrl   string  `db:"-" json:"http_url,omitempty"` // Computed, not stored in DB
}

// Metadata key constants
const (
	MetadataKeyMime          = "mime"
	MetadataKeyPermissions   = "permissions"
	MetadataKeyHashMD5       = "hash.md5"
	MetadataKeyHashSHA256    = "hash.sha256"
	MetadataKeyThumbnail     = "thumbnail"
	MetadataKeyVideoTimeline = "video-timeline"
	MetadataKeyExif          = "exif"
	MetadataKeyMediaInfo     = "media-info"
)

// Metadata source constants
const (
	MetadataSourceScan       = "scan"
	MetadataSourceEnrichment = "enrichment"
	MetadataSourceDerived    = "derived"
	MetadataSourceClassifier = "classifier"
)

var validMetadataSources = map[string]bool{
	MetadataSourceScan:       true,
	MetadataSourceEnrichment: true,
	MetadataSourceDerived:    true,
	MetadataSourceClassifier: true,
}

// Validate checks that the metadata record has the required fields.
func (m *MetadataRecord) Validate() error {
	if m.EntryPath == "" {
		return fmt.Errorf("entry_path is required")
	}
	if m.Key == "" {
		return fmt.Errorf("key is required")
	}
	if m.Source == "" {
		return fmt.Errorf("source is required")
	}
	if !validMetadataSources[m.Source] {
		return fmt.Errorf("invalid source %q: must be scan, enrichment, derived, or classifier", m.Source)
	}
	return nil
}

// IsSimple returns true if this is a simple key-value metadata record (no artifact hash).
func (m *MetadataRecord) IsSimple() bool {
	return m.Hash == nil
}
