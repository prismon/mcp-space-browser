package database

import (
	"testing"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mdStrPtr(s string) *string { return &s }

func setupMetadataTestDB(t *testing.T) *DiskDB {
	t.Helper()
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Insert a test entry
	err = db.InsertOrUpdate(&models.Entry{
		Path: "/test/file.jpg",
		Size: 1024,
		Kind: "file",
	})
	require.NoError(t, err)

	err = db.InsertOrUpdate(&models.Entry{
		Path: "/test/file2.txt",
		Size: 512,
		Kind: "file",
	})
	require.NoError(t, err)

	return db
}

func TestSetMetadata_SimpleUpsert(t *testing.T) {
	db := setupMetadataTestDB(t)

	// Insert simple metadata
	val := "image/jpeg"
	m := &models.MetadataRecord{
		EntryPath: "/test/file.jpg",
		Key:       models.MetadataKeyMime,
		Value:     &val,
		Source:    models.MetadataSourceScan,
	}
	require.NoError(t, db.SetMetadata(m))

	// Verify
	got, err := db.GetMetadataByKey("/test/file.jpg", models.MetadataKeyMime)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "image/jpeg", *got.Value)
	assert.True(t, got.IsSimple())

	// Upsert: update value
	newVal := "image/png"
	m.Value = &newVal
	require.NoError(t, db.SetMetadata(m))

	got, err = db.GetMetadataByKey("/test/file.jpg", models.MetadataKeyMime)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "image/png", *got.Value)
}

func TestSetMetadata_ArtifactUpsert(t *testing.T) {
	db := setupMetadataTestDB(t)

	hash := "abc123hash"
	cachePath := "/cache/thumb.jpg"
	mimeType := "image/jpeg"
	generator := "go-image"

	m := &models.MetadataRecord{
		EntryPath: "/test/file.jpg",
		Key:       models.MetadataKeyThumbnail,
		Source:    models.MetadataSourceClassifier,
		Hash:      &hash,
		CachePath: &cachePath,
		MimeType:  &mimeType,
		FileSize:  5000,
		Generator: &generator,
	}
	require.NoError(t, db.SetMetadata(m))

	// Verify via hash lookup
	got, err := db.GetMetadataByHash("abc123hash")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/test/file.jpg", got.EntryPath)
	assert.Equal(t, "thumbnail", got.Key)
	assert.Equal(t, "image/jpeg", *got.MimeType)
	assert.False(t, got.IsSimple())

	// Upsert: update cache_path
	newCachePath := "/cache/thumb_v2.jpg"
	m.CachePath = &newCachePath
	require.NoError(t, db.SetMetadata(m))

	got, err = db.GetMetadataByHash("abc123hash")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/cache/thumb_v2.jpg", *got.CachePath)
}

func TestSetMetadata_SimpleAndArtifactCoexist(t *testing.T) {
	db := setupMetadataTestDB(t)

	// Simple mime metadata
	mimeVal := "image/jpeg"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg",
		Key:       models.MetadataKeyThumbnail,
		Value:     &mimeVal,
		Source:    models.MetadataSourceScan,
	}))

	// Artifact thumbnail metadata with same key but a hash
	hash := "thumb-hash-123"
	cachePath := "/cache/thumb.jpg"
	mimeType := "image/jpeg"
	generator := "go-image"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg",
		Key:       models.MetadataKeyThumbnail,
		Source:    models.MetadataSourceClassifier,
		Hash:      &hash,
		CachePath: &cachePath,
		MimeType:  &mimeType,
		Generator: &generator,
	}))

	// Both should coexist: GetAllMetadata returns both
	all, err := db.GetAllMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// GetSimpleMetadata returns only the simple one
	simple, err := db.GetSimpleMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, simple, 1)
	assert.True(t, simple[0].IsSimple())

	// GetArtifactMetadata returns only the artifact
	artifacts, err := db.GetArtifactMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.False(t, artifacts[0].IsSimple())
}

func TestGetMetadataByKey(t *testing.T) {
	db := setupMetadataTestDB(t)

	// Not found
	got, err := db.GetMetadataByKey("/test/file.jpg", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Insert and find
	val := "0644"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg",
		Key:       models.MetadataKeyPermissions,
		Value:     &val,
		Source:    models.MetadataSourceScan,
	}))

	got, err = db.GetMetadataByKey("/test/file.jpg", models.MetadataKeyPermissions)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "0644", *got.Value)
}

func TestGetAllMetadata(t *testing.T) {
	db := setupMetadataTestDB(t)

	mimeVal := "image/jpeg"
	permVal := "0644"
	hash := "thumbhash"
	cachePath := "/cache/thumb.jpg"
	mimeType := "image/jpeg"
	generator := "go-image"

	// Insert simple + artifact
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "mime", Value: &mimeVal, Source: "scan",
	}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "permissions", Value: &permVal, Source: "scan",
	}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: &cachePath, MimeType: &mimeType, Generator: &generator,
	}))

	all, err := db.GetAllMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestGetSimpleMetadata(t *testing.T) {
	db := setupMetadataTestDB(t)

	val := "image/jpeg"
	hash := "h1"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "mime", Value: &val, Source: "scan",
	}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: mdStrPtr("/cache/t.jpg"), MimeType: mdStrPtr("image/jpeg"), Generator: mdStrPtr("go-image"),
	}))

	simple, err := db.GetSimpleMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, simple, 1)
	assert.Equal(t, "mime", simple[0].Key)
}

func TestGetArtifactMetadata(t *testing.T) {
	db := setupMetadataTestDB(t)

	val := "image/jpeg"
	hash := "h1"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "mime", Value: &val, Source: "scan",
	}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: mdStrPtr("/cache/t.jpg"), MimeType: mdStrPtr("image/jpeg"), Generator: mdStrPtr("go-image"),
	}))

	artifacts, err := db.GetArtifactMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "thumbnail", artifacts[0].Key)
	assert.Equal(t, "h1", *artifacts[0].Hash)
}

func TestGetMetadataByHash(t *testing.T) {
	db := setupMetadataTestDB(t)

	// Not found
	got, err := db.GetMetadataByHash("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Insert artifact and find
	hash := "unique-hash-42"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: mdStrPtr("/cache/t.jpg"), MimeType: mdStrPtr("image/jpeg"), Generator: mdStrPtr("go-image"),
	}))

	got, err = db.GetMetadataByHash("unique-hash-42")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/test/file.jpg", got.EntryPath)
}

func TestGetMetadataByCachePath(t *testing.T) {
	db := setupMetadataTestDB(t)

	// Not found
	got, err := db.GetMetadataByCachePath("/nonexistent/path")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Insert and find
	hash := "cp-hash"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: mdStrPtr("/cache/special/t.jpg"), MimeType: mdStrPtr("image/jpeg"), Generator: mdStrPtr("go-image"),
	}))

	got, err = db.GetMetadataByCachePath("/cache/special/t.jpg")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "cp-hash", *got.Hash)
}

func TestQueryMetadataByKey(t *testing.T) {
	db := setupMetadataTestDB(t)

	val1 := "image/jpeg"
	val2 := "text/plain"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "mime", Value: &val1, Source: "scan",
	}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file2.txt", Key: "mime", Value: &val2, Source: "scan",
	}))

	records, err := db.QueryMetadataByKey("mime")
	require.NoError(t, err)
	assert.Len(t, records, 2)
	// Ordered by entry_path
	assert.Equal(t, "/test/file.jpg", records[0].EntryPath)
	assert.Equal(t, "/test/file2.txt", records[1].EntryPath)
}

func TestSetMetadataBatch(t *testing.T) {
	db := setupMetadataTestDB(t)

	val1 := "image/jpeg"
	val2 := "0644"
	records := []*models.MetadataRecord{
		{EntryPath: "/test/file.jpg", Key: "mime", Value: &val1, Source: "scan"},
		{EntryPath: "/test/file.jpg", Key: "permissions", Value: &val2, Source: "scan"},
	}

	require.NoError(t, db.SetMetadataBatch(records))

	all, err := db.GetAllMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestSetMetadataBatch_ValidationError(t *testing.T) {
	db := setupMetadataTestDB(t)

	records := []*models.MetadataRecord{
		{EntryPath: "/test/file.jpg", Key: "mime", Source: "scan"},
		{EntryPath: "", Key: "bad", Source: "scan"}, // invalid: empty entry_path
	}

	err := db.SetMetadataBatch(records)
	assert.Error(t, err)

	// Transaction should have rolled back, so no records
	all, err := db.GetAllMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, all, 0)
}

func TestDeleteMetadataByKey(t *testing.T) {
	db := setupMetadataTestDB(t)

	val := "image/jpeg"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "mime", Value: &val, Source: "scan",
	}))

	require.NoError(t, db.DeleteMetadataByKey("/test/file.jpg", "mime"))

	got, err := db.GetMetadataByKey("/test/file.jpg", "mime")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteMetadataByEntry(t *testing.T) {
	db := setupMetadataTestDB(t)

	val := "image/jpeg"
	hash := "del-hash"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "mime", Value: &val, Source: "scan",
	}))
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: mdStrPtr("/cache/t.jpg"), MimeType: mdStrPtr("image/jpeg"), Generator: mdStrPtr("go-image"),
	}))

	count, err := db.DeleteMetadataByEntry("/test/file.jpg")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	all, err := db.GetAllMetadata("/test/file.jpg")
	require.NoError(t, err)
	assert.Len(t, all, 0)
}

func TestDeleteMetadataByHash(t *testing.T) {
	db := setupMetadataTestDB(t)

	hash := "del-by-hash"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.jpg", Key: "thumbnail", Source: "classifier",
		Hash: &hash, CachePath: mdStrPtr("/cache/t.jpg"), MimeType: mdStrPtr("image/jpeg"), Generator: mdStrPtr("go-image"),
	}))

	require.NoError(t, db.DeleteMetadataByHash("del-by-hash"))

	got, err := db.GetMetadataByHash("del-by-hash")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSetMetadata_Validation(t *testing.T) {
	db := setupMetadataTestDB(t)

	err := db.SetMetadata(&models.MetadataRecord{
		EntryPath: "",
		Key:       "mime",
		Source:    "scan",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metadata")
}
