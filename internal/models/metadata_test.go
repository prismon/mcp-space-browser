package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadataRecord_Validate(t *testing.T) {
	t.Run("valid simple metadata", func(t *testing.T) {
		val := "image/jpeg"
		m := &MetadataRecord{
			EntryPath: "/test/file.jpg",
			Key:       MetadataKeyMime,
			Value:     &val,
			Source:    MetadataSourceScan,
		}
		assert.NoError(t, m.Validate())
	})

	t.Run("valid artifact metadata", func(t *testing.T) {
		hash := "abc123"
		cachePath := "/cache/thumb.jpg"
		mimeType := "image/jpeg"
		generator := "go-image"
		m := &MetadataRecord{
			EntryPath: "/test/file.jpg",
			Key:       MetadataKeyThumbnail,
			Source:    MetadataSourceClassifier,
			Hash:      &hash,
			CachePath: &cachePath,
			MimeType:  &mimeType,
			Generator: &generator,
		}
		assert.NoError(t, m.Validate())
	})

	t.Run("missing entry_path", func(t *testing.T) {
		m := &MetadataRecord{Key: "mime", Source: "scan"}
		err := m.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "entry_path")
	})

	t.Run("missing key", func(t *testing.T) {
		m := &MetadataRecord{EntryPath: "/test", Source: "scan"}
		err := m.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "key")
	})

	t.Run("missing source", func(t *testing.T) {
		m := &MetadataRecord{EntryPath: "/test", Key: "mime"}
		err := m.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source")
	})

	t.Run("invalid source", func(t *testing.T) {
		m := &MetadataRecord{EntryPath: "/test", Key: "mime", Source: "invalid"}
		err := m.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid source")
	})

	t.Run("classifier source is valid", func(t *testing.T) {
		val := "test"
		m := &MetadataRecord{
			EntryPath: "/test/file.txt",
			Key:       "test",
			Value:     &val,
			Source:    MetadataSourceClassifier,
		}
		assert.NoError(t, m.Validate())
	})
}

func TestMetadataRecord_IsSimple(t *testing.T) {
	t.Run("nil hash is simple", func(t *testing.T) {
		m := &MetadataRecord{Hash: nil}
		assert.True(t, m.IsSimple())
	})

	t.Run("non-nil hash is not simple", func(t *testing.T) {
		hash := "abc123"
		m := &MetadataRecord{Hash: &hash}
		assert.False(t, m.IsSimple())
	})
}

func TestMetadataRecord_KeyConstants(t *testing.T) {
	assert.Equal(t, "mime", MetadataKeyMime)
	assert.Equal(t, "permissions", MetadataKeyPermissions)
	assert.Equal(t, "hash.md5", MetadataKeyHashMD5)
	assert.Equal(t, "hash.sha256", MetadataKeyHashSHA256)
	assert.Equal(t, "thumbnail", MetadataKeyThumbnail)
	assert.Equal(t, "video-timeline", MetadataKeyVideoTimeline)
	assert.Equal(t, "exif", MetadataKeyExif)
	assert.Equal(t, "media-info", MetadataKeyMediaInfo)
}

func TestMetadataRecord_SourceConstants(t *testing.T) {
	assert.Equal(t, "scan", MetadataSourceScan)
	assert.Equal(t, "enrichment", MetadataSourceEnrichment)
	assert.Equal(t, "derived", MetadataSourceDerived)
	assert.Equal(t, "classifier", MetadataSourceClassifier)
}
