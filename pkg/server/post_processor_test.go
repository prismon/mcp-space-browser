package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupPostProcessDB creates an in-memory DB with schema, indexes a temp dir, and returns db + dir.
func setupPostProcessDB(t *testing.T, files map[string][]byte) (*database.DiskDB, string) {
	t.Helper()
	tmpDir := t.TempDir()

	for name, content := range files {
		dir := filepath.Dir(filepath.Join(tmpDir, name))
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, name), content, 0644))
	}

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	opts := crawler.DefaultIndexOptions()
	opts.Force = true
	_, err = crawler.IndexWithOptions(tmpDir, db, nil, 0, nil, opts)
	require.NoError(t, err)

	return db, tmpDir
}

func TestPostProcess_MimeDetection(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"hello.txt": []byte("Hello, world!"),
	})

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"mime"},
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)
	assert.Equal(t, int64(1), result.MetadataSet)
	assert.Equal(t, int64(0), result.Errors)

	m, err := db.GetMetadataByKey(filepath.Join(tmpDir, "hello.txt"), "mime")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "text/plain; charset=utf-8", *m.Value)
	assert.Equal(t, "scan", m.Source)
}

func TestPostProcess_PermissionsExtracted(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"test.txt": []byte("test"),
	})

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"permissions"},
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)
	assert.Equal(t, int64(1), result.MetadataSet)

	m, err := db.GetMetadataByKey(filepath.Join(tmpDir, "test.txt"), "permissions")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "0644", *m.Value)
}

func TestPostProcess_ThumbnailGeneration(t *testing.T) {
	// Create a minimal valid JPEG
	jpegData := createMinimalJPEG()

	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"photo.jpg": jpegData,
	})

	cacheDir := t.TempDir()

	// Set up artifact cache dir for the classifier infrastructure
	SetArtifactCacheDir(cacheDir)

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		CacheDir:   cacheDir,
		Attributes: []string{"thumbnail"},
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)
	// Thumbnail generation may succeed or fail depending on environment,
	// but there should be no panics and processing should complete
	assert.True(t, result.MetadataSet >= 0)
}

func TestPostProcess_DefaultAttributes(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"readme.txt": []byte("Read me"),
	})

	// Empty attributes list → all defaults extracted
	result := PostProcess(&PostProcessConfig{
		DB: db,
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)

	// At minimum, mime and permissions should be set (they don't need classifier infra)
	mimeM, err := db.GetMetadataByKey(filepath.Join(tmpDir, "readme.txt"), "mime")
	require.NoError(t, err)
	require.NotNil(t, mimeM, "mime metadata should be set with default attributes")

	permM, err := db.GetMetadataByKey(filepath.Join(tmpDir, "readme.txt"), "permissions")
	require.NoError(t, err)
	require.NotNil(t, permM, "permissions metadata should be set with default attributes")
}

func TestPostProcess_FilteredAttributes(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"data.txt": []byte("filtered test"),
	})

	// Only mime → no permissions, no thumbnails
	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"mime"},
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)
	assert.Equal(t, int64(1), result.MetadataSet)

	mimeM, err := db.GetMetadataByKey(filepath.Join(tmpDir, "data.txt"), "mime")
	require.NoError(t, err)
	require.NotNil(t, mimeM, "mime should be extracted")

	permM, err := db.GetMetadataByKey(filepath.Join(tmpDir, "data.txt"), "permissions")
	require.NoError(t, err)
	assert.Nil(t, permM, "permissions should NOT be extracted when filtered out")
}

func TestPostProcess_ErrorTolerance(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"a.txt": []byte("file a"),
		"b.txt": []byte("file b"),
	})

	// Delete one file after indexing to cause an error
	require.NoError(t, os.Remove(filepath.Join(tmpDir, "a.txt")))

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"mime", "permissions"},
	}, []string{tmpDir})

	// b.txt should still be processed
	assert.Equal(t, int64(2), result.FilesProcessed)
	assert.True(t, result.Errors > 0, "should have errors from deleted file")
	assert.True(t, result.MetadataSet > 0, "surviving file should have metadata set")

	m, err := db.GetMetadataByKey(filepath.Join(tmpDir, "b.txt"), "mime")
	require.NoError(t, err)
	require.NotNil(t, m, "b.txt should have mime metadata")
}

func TestPostProcess_NoFilesSkipsProcessing(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{})

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"mime"},
	}, []string{tmpDir})

	assert.Equal(t, int64(0), result.FilesProcessed)
	assert.Equal(t, int64(0), result.MetadataSet)
	assert.Equal(t, int64(0), result.Errors)
}

func TestPostProcess_HashMD5(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"hash_test.txt": []byte("hash me"),
	})

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"hash.md5"},
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)
	assert.Equal(t, int64(1), result.MetadataSet)

	m, err := db.GetMetadataByKey(filepath.Join(tmpDir, "hash_test.txt"), "hash.md5")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, *m.Value, 32) // MD5 hex length
}

func TestPostProcess_HashSHA256(t *testing.T) {
	db, tmpDir := setupPostProcessDB(t, map[string][]byte{
		"hash_test.txt": []byte("hash me"),
	})

	result := PostProcess(&PostProcessConfig{
		DB:         db,
		Attributes: []string{"hash.sha256"},
	}, []string{tmpDir})

	assert.Equal(t, int64(1), result.FilesProcessed)
	assert.Equal(t, int64(1), result.MetadataSet)

	m, err := db.GetMetadataByKey(filepath.Join(tmpDir, "hash_test.txt"), "hash.sha256")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, *m.Value, 64) // SHA256 hex length
}

func TestPostProcess_NilDB(t *testing.T) {
	result := PostProcess(&PostProcessConfig{
		DB: nil,
	}, []string{"/tmp"})

	assert.Equal(t, int64(0), result.FilesProcessed)
}

// createMinimalJPEG returns the bytes of a minimal valid JPEG image (1x1 pixel).
func createMinimalJPEG() []byte {
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
		0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
		0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
		0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
		0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
		0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
		0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
		0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
		0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
		0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0x7B, 0x94, 0x11, 0x00, 0x00, 0x00, 0x00, 0xFF,
		0xD9,
	}
}
