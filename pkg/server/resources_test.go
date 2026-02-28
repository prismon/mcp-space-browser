package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/auth"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupResourceTestDB(t *testing.T) *database.DiskDB {
	t.Helper()
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)

	now := time.Now().Unix()
	root := "/"
	testDir := "/test"
	entries := []*models.Entry{
		{Path: "/test", Parent: &root, Size: 0, Kind: "directory", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/test/file.txt", Parent: &testDir, Size: 100, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
	}
	for _, e := range entries {
		require.NoError(t, db.InsertOrUpdate(e))
	}

	mimeVal := "text/plain"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.txt", Key: "mime", Value: &mimeVal, Source: "scan",
	}))

	return db
}

func TestResource_Registration(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	s := mcpserver.NewMCPServer("test", "1.0")
	registerResources(s, db)
	// Verify registration doesn't panic
	assert.NotNil(t, s)
}

func TestResource_ExtractURIParam(t *testing.T) {
	tests := []struct {
		uri    string
		prefix string
		expect string
	}{
		{"synthesis://entries//test/file.txt", "synthesis://entries/", "/test/file.txt"},
		{"synthesis://sets/my-set", "synthesis://sets/", "my-set"},
		{"synthesis://jobs/42", "synthesis://jobs/", "42"},
		{"synthesis://wrong/path", "synthesis://entries/", ""},
		// URL-encoded paths
		{"synthesis://entries/%2FVolumes%2FArchive", "synthesis://entries/", "/Volumes/Archive"},
		{"synthesis://entries/%2Fhome%2Fuser%2FMy%20Documents", "synthesis://entries/", "/home/user/My Documents"},
		{"synthesis://entries/%2Ftest%2Ffile%20with%20spaces.txt", "synthesis://entries/", "/test/file with spaces.txt"},
	}

	for _, tt := range tests {
		result := extractURIParam(tt.uri, tt.prefix)
		assert.Equal(t, tt.expect, result, "URI: %s", tt.uri)
	}
}

func TestResource_ResourceJSON(t *testing.T) {
	data := map[string]string{"key": "value"}
	result, err := resourceJSON(data, "synthesis://test")
	require.NoError(t, err)
	require.Len(t, result, 1)

	textContent, ok := result[0].(*mcp.TextResourceContents)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "key")
	assert.Equal(t, "application/json", textContent.MIMEType)
}

func TestResource_EntryData(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	// Verify the data that entry resource would return
	entry, err := db.Get("/test/file.txt")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "file", entry.Kind)
	assert.Equal(t, int64(100), entry.Size)

	attrs, err := db.GetSimpleMetadata("/test/file.txt")
	require.NoError(t, err)
	assert.Len(t, attrs, 1)
	assert.Equal(t, "mime", attrs[0].Key)
	assert.Equal(t, "text/plain", *attrs[0].Value)
}

func TestResource_SetsListData(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	_, err := db.CreateResourceSet(&models.ResourceSet{Name: "test-set"})
	require.NoError(t, err)

	sets, err := db.ListResourceSets()
	require.NoError(t, err)
	assert.Len(t, sets, 1)
	assert.Equal(t, "test-set", sets[0].Name)
}

func TestResource_SetEntriesData(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	_, err := db.CreateResourceSet(&models.ResourceSet{Name: "files"})
	require.NoError(t, err)
	require.NoError(t, db.AddToResourceSet("files", []string{"/test/file.txt"}))

	entries, err := db.GetResourceSetEntries("files")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/test/file.txt", entries[0].Path)
}

func TestResource_JobsListEmpty(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	jobs, err := db.ListIndexJobs(nil, 100)
	require.NoError(t, err)
	assert.Len(t, jobs, 0)
}

func TestResource_MarshalEntry(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	entry, err := db.Get("/test/file.txt")
	require.NoError(t, err)

	metadata, err := db.GetAllMetadata("/test/file.txt")
	require.NoError(t, err)

	result := map[string]interface{}{
		"entry":    entry,
		"metadata": metadata,
	}

	payload, err := json.Marshal(result)
	require.NoError(t, err)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(payload, &response))
	assert.NotNil(t, response["entry"])
	assert.NotNil(t, response["metadata"])
}

func TestResource_EntryIncludesMetadata(t *testing.T) {
	db := setupResourceTestDB(t)
	defer db.Close()

	cachePath := "/cache/ab/cd/abcd1234/thumb.jpg"
	thumbHash := "abcd1234test"
	mimeType := "image/jpeg"
	generator := "go-image"
	require.NoError(t, db.SetMetadata(&models.MetadataRecord{
		EntryPath: "/test/file.txt",
		Key:       "thumbnail",
		Source:    models.MetadataSourceClassifier,
		Hash:      &thumbHash,
		MimeType:  &mimeType,
		CachePath: &cachePath,
		FileSize:  5000,
		Generator: &generator,
	}))

	// Verify all metadata returned for entry
	entry, err := db.Get("/test/file.txt")
	require.NoError(t, err)
	require.NotNil(t, entry)

	allMeta, err := db.GetAllMetadata("/test/file.txt")
	require.NoError(t, err)
	require.Len(t, allMeta, 2) // mime (simple) + thumbnail (artifact)

	// Marshal the combined result (same shape as registerEntryResource returns)
	result := map[string]interface{}{
		"entry":    entry,
		"metadata": allMeta,
	}

	payload, err := json.Marshal(result)
	require.NoError(t, err)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(payload, &response))
	assert.NotNil(t, response["entry"])
	assert.NotNil(t, response["metadata"])

	metadataList, ok := response["metadata"].([]interface{})
	require.True(t, ok)
	assert.Len(t, metadataList, 2)

	// Find the thumbnail metadata record
	var thumbFound bool
	for _, m := range metadataList {
		rec := m.(map[string]interface{})
		if rec["key"] == "thumbnail" {
			thumbFound = true
			assert.Equal(t, cachePath, rec["cache_path"])
			break
		}
	}
	assert.True(t, thumbFound, "thumbnail metadata should be present")
}

func TestRegisterMCPResourcesWithContext_RegistersAllTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	sc, err := NewServerContext(
		&auth.Config{},
		filepath.Join(tmpDir, "projects"),
		filepath.Join(tmpDir, "cache"),
	)
	require.NoError(t, err)
	defer sc.Close()

	s := mcpserver.NewMCPServer("test", "1.0",
		mcpserver.WithResourceCapabilities(true, true),
	)
	registerMCPResourcesWithContext(s, sc)

	// Initialize the MCP session
	initMsg := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	s.HandleMessage(context.Background(), json.RawMessage(initMsg))

	// List resource templates
	listMsg := `{"jsonrpc":"2.0","id":2,"method":"resources/templates/list"}`
	response := s.HandleMessage(context.Background(), json.RawMessage(listMsg))

	responseJSON, err := json.Marshal(response)
	require.NoError(t, err)
	responseStr := string(responseJSON)

	// All 5 resource templates must be registered
	assert.Contains(t, responseStr, "synthesis://entries/{path}", "entries template missing")
	assert.Contains(t, responseStr, "synthesis://entries/{path}/attributes", "entry attributes template missing")
	assert.Contains(t, responseStr, "synthesis://sets/{name}", "sets template missing")
	assert.Contains(t, responseStr, "synthesis://sets/{name}/entries", "set entries template missing")
	assert.Contains(t, responseStr, "synthesis://jobs/{id}", "jobs template missing")

	// Also verify static resources (synthesis://sets, synthesis://jobs, synthesis://projects)
	listResourcesMsg := `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`
	resourcesResponse := s.HandleMessage(context.Background(), json.RawMessage(listResourcesMsg))

	resourcesJSON, err := json.Marshal(resourcesResponse)
	require.NoError(t, err)
	resourcesStr := string(resourcesJSON)

	assert.Contains(t, resourcesStr, "synthesis://sets", "sets resource missing")
	assert.Contains(t, resourcesStr, "synthesis://jobs", "jobs resource missing")
	assert.Contains(t, resourcesStr, "synthesis://projects", "projects resource missing")
}
