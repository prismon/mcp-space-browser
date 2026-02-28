package database

import (
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDBWithEntry(t *testing.T) *DiskDB {
	t.Helper()
	db, err := NewDiskDB(":memory:")
	require.NoError(t, err)

	parent := "/test"
	err = db.InsertOrUpdate(&models.Entry{
		Path:        "/test/file.txt",
		Parent:      &parent,
		Size:        1024,
		Kind:        "file",
		Ctime:       time.Now().Unix(),
		Mtime:       time.Now().Unix(),
		LastScanned: time.Now().Unix(),
	})
	require.NoError(t, err)
	return db
}

func TestSetAttribute(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	now := time.Now().Unix()
	attr := &models.Attribute{
		EntryPath:  "/test/file.txt",
		Key:        "mime",
		Value:      "text/plain",
		Source:     "scan",
		ComputedAt: now,
	}

	err := db.SetAttribute(attr)
	assert.NoError(t, err)

	got, err := db.GetAttribute("/test/file.txt", "mime")
	assert.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "text/plain", got.Value)
	assert.Equal(t, "scan", got.Source)
}

func TestSetAttribute_Upsert(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	now := time.Now().Unix()
	attr := &models.Attribute{
		EntryPath: "/test/file.txt", Key: "mime",
		Value: "text/plain", Source: "scan", ComputedAt: now,
	}
	require.NoError(t, db.SetAttribute(attr))

	attr.Value = "application/json"
	attr.ComputedAt = now + 10
	require.NoError(t, db.SetAttribute(attr))

	got, err := db.GetAttribute("/test/file.txt", "mime")
	require.NoError(t, err)
	assert.Equal(t, "application/json", got.Value)
}

func TestGetAttributes(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	now := time.Now().Unix()
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/file.txt", Key: "mime",
		Value: "text/plain", Source: "scan", ComputedAt: now,
	}))
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/file.txt", Key: "hash.md5",
		Value: "abc123", Source: "scan", ComputedAt: now,
	}))

	attrs, err := db.GetAttributes("/test/file.txt")
	assert.NoError(t, err)
	assert.Len(t, attrs, 2)
}

func TestGetAttribute_NotFound(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	got, err := db.GetAttribute("/test/file.txt", "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteAttribute(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	now := time.Now().Unix()
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/file.txt", Key: "mime",
		Value: "text/plain", Source: "scan", ComputedAt: now,
	}))

	err := db.DeleteAttribute("/test/file.txt", "mime")
	assert.NoError(t, err)

	got, err := db.GetAttribute("/test/file.txt", "mime")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteAttributesByEntry(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	now := time.Now().Unix()
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/file.txt", Key: "mime",
		Value: "text/plain", Source: "scan", ComputedAt: now,
	}))
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/file.txt", Key: "hash.md5",
		Value: "abc123", Source: "scan", ComputedAt: now,
	}))

	err := db.DeleteAttributesByEntry("/test/file.txt")
	assert.NoError(t, err)

	attrs, err := db.GetAttributes("/test/file.txt")
	assert.NoError(t, err)
	assert.Len(t, attrs, 0)
}

func TestSetAttributesBatch(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	now := time.Now().Unix()
	attrs := []*models.Attribute{
		{EntryPath: "/test/file.txt", Key: "mime", Value: "text/plain", Source: "scan", ComputedAt: now},
		{EntryPath: "/test/file.txt", Key: "hash.md5", Value: "abc123", Source: "scan", ComputedAt: now},
		{EntryPath: "/test/file.txt", Key: "hash.sha256", Value: "def456", Source: "scan", ComputedAt: now},
	}

	err := db.SetAttributesBatch(attrs)
	assert.NoError(t, err)

	got, err := db.GetAttributes("/test/file.txt")
	assert.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestSetAttributesBatch_ValidationError(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	attrs := []*models.Attribute{
		{EntryPath: "/test/file.txt", Key: "mime", Value: "text/plain", Source: "scan", ComputedAt: 1},
		{EntryPath: "", Key: "bad", Value: "x", Source: "scan", ComputedAt: 1}, // invalid
	}

	err := db.SetAttributesBatch(attrs)
	assert.Error(t, err)
}

func TestQueryAttributesByKey(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	parent := "/test"
	require.NoError(t, db.InsertOrUpdate(&models.Entry{
		Path: "/test/other.txt", Parent: &parent, Size: 2048,
		Kind: "file", Ctime: time.Now().Unix(), Mtime: time.Now().Unix(),
		LastScanned: time.Now().Unix(),
	}))

	now := time.Now().Unix()
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/file.txt", Key: "mime", Value: "text/plain", Source: "scan", ComputedAt: now,
	}))
	require.NoError(t, db.SetAttribute(&models.Attribute{
		EntryPath: "/test/other.txt", Key: "mime", Value: "image/jpeg", Source: "scan", ComputedAt: now,
	}))

	attrs, err := db.QueryAttributesByKey("mime")
	assert.NoError(t, err)
	assert.Len(t, attrs, 2)
}
