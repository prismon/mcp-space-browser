# Radical Consolidation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Collapse 59 MCP tools into 5 composable tools (scan, query, manage, batch, watch) with a first-class attribute system, and strip documentation to essentials.

**Architecture:** New tool files (`tool_scan.go`, `tool_query.go`, `tool_manage.go`, `tool_batch.go`, `tool_watch.go`) replace the old tool registration files. A new `attributes` table provides extensible key-value storage for content-derived metadata. The existing `entries` table, crawler, and database layer are preserved and extended.

**Tech Stack:** Go 1.24, SQLite (mattn/go-sqlite3), mcp-go v0.43.0, testify, gin

---

## Task 1: Add Attribute model to internal/models

**Files:**
- Create: `internal/models/attribute.go`
- Test: `internal/models/attribute_test.go`

**Step 1: Write the failing test**

Create `internal/models/attribute_test.go`:

```go
package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttributeValidation(t *testing.T) {
	t.Run("valid attribute", func(t *testing.T) {
		a := &Attribute{
			EntryPath:  "/test/file.txt",
			Key:        "mime",
			Value:      "image/jpeg",
			Source:     "scan",
			ComputedAt: 1000,
		}
		assert.NoError(t, a.Validate())
	})

	t.Run("missing entry path", func(t *testing.T) {
		a := &Attribute{Key: "mime", Value: "image/jpeg", Source: "scan"}
		assert.Error(t, a.Validate())
	})

	t.Run("missing key", func(t *testing.T) {
		a := &Attribute{EntryPath: "/test", Value: "x", Source: "scan"}
		assert.Error(t, a.Validate())
	})

	t.Run("missing source", func(t *testing.T) {
		a := &Attribute{EntryPath: "/test", Key: "mime", Value: "x"}
		assert.Error(t, a.Validate())
	})

	t.Run("invalid source", func(t *testing.T) {
		a := &Attribute{EntryPath: "/test", Key: "mime", Value: "x", Source: "bad"}
		assert.Error(t, a.Validate())
	})
}
```

**Step 2: Run test to verify it fails**

Run: `GO_ENV=test go test ./internal/models/ -run TestAttributeValidation -v`
Expected: FAIL — `Attribute` type undefined

**Step 3: Write minimal implementation**

Create `internal/models/attribute.go`:

```go
package models

import "fmt"

// Attribute represents an extensible key-value attribute for a filesystem entry.
// Base attributes (path, size, kind, timestamps) live in the entries table.
// Content-derived, usage, and enrichment attributes live here.
type Attribute struct {
	EntryPath  string `db:"entry_path" json:"entry_path"`
	Key        string `db:"key" json:"key"`
	Value      string `db:"value" json:"value"`
	Source     string `db:"source" json:"source"`           // "scan", "enrichment", "derived"
	ComputedAt int64  `db:"computed_at" json:"computed_at"` // Unix timestamp
}

// Valid attribute sources
const (
	AttributeSourceScan       = "scan"
	AttributeSourceEnrichment = "enrichment"
	AttributeSourceDerived    = "derived"
)

var validSources = map[string]bool{
	AttributeSourceScan:       true,
	AttributeSourceEnrichment: true,
	AttributeSourceDerived:    true,
}

func (a *Attribute) Validate() error {
	if a.EntryPath == "" {
		return fmt.Errorf("entry_path is required")
	}
	if a.Key == "" {
		return fmt.Errorf("key is required")
	}
	if a.Source == "" {
		return fmt.Errorf("source is required")
	}
	if !validSources[a.Source] {
		return fmt.Errorf("invalid source %q: must be scan, enrichment, or derived", a.Source)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `GO_ENV=test go test ./internal/models/ -run TestAttributeValidation -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/models/attribute.go internal/models/attribute_test.go
git commit -m "feat: add Attribute model for extensible entry metadata"
```

---

## Task 2: Add attributes table and CRUD to database layer

**Files:**
- Create: `pkg/database/attributes.go`
- Create: `pkg/database/attributes_test.go`

**Step 1: Write the failing test**

Create `pkg/database/attributes_test.go`:

```go
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

	// Insert a test entry for FK reference
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

	// Verify we can get it back
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

	// Update same key
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

func TestQueryAttributesByKey(t *testing.T) {
	db := setupTestDBWithEntry(t)
	defer db.Close()

	// Add a second entry
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
```

**Step 2: Run test to verify it fails**

Run: `GO_ENV=test go test ./pkg/database/ -run TestSetAttribute -v`
Expected: FAIL — methods undefined

**Step 3: Write minimal implementation**

Create `pkg/database/attributes.go`:

```go
package database

import (
	"fmt"

	"github.com/prismon/mcp-space-browser/internal/models"
)

// initAttributesTable creates the attributes table if it doesn't exist
func (d *DiskDB) initAttributesTable() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS attributes (
			entry_path TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT,
			source TEXT NOT NULL,
			computed_at INTEGER,
			PRIMARY KEY (entry_path, key)
		);
		CREATE INDEX IF NOT EXISTS idx_attributes_key ON attributes(key);
		CREATE INDEX IF NOT EXISTS idx_attributes_source ON attributes(source);
	`)
	return err
}

// SetAttribute inserts or updates a single attribute (upsert)
func (d *DiskDB) SetAttribute(attr *models.Attribute) error {
	if err := attr.Validate(); err != nil {
		return fmt.Errorf("invalid attribute: %w", err)
	}
	_, err := d.db.Exec(`
		INSERT INTO attributes (entry_path, key, value, source, computed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entry_path, key) DO UPDATE SET
			value = excluded.value,
			source = excluded.source,
			computed_at = excluded.computed_at
	`, attr.EntryPath, attr.Key, attr.Value, attr.Source, attr.ComputedAt)
	return err
}

// SetAttributesBatch inserts or updates multiple attributes in a transaction
func (d *DiskDB) SetAttributesBatch(attrs []*models.Attribute) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO attributes (entry_path, key, value, source, computed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entry_path, key) DO UPDATE SET
			value = excluded.value,
			source = excluded.source,
			computed_at = excluded.computed_at
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, attr := range attrs {
		if err := attr.Validate(); err != nil {
			tx.Rollback()
			return fmt.Errorf("invalid attribute %q for %q: %w", attr.Key, attr.EntryPath, err)
		}
		if _, err := stmt.Exec(attr.EntryPath, attr.Key, attr.Value, attr.Source, attr.ComputedAt); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert attribute %q for %q: %w", attr.Key, attr.EntryPath, err)
		}
	}

	return tx.Commit()
}

// GetAttribute retrieves a single attribute by entry path and key. Returns nil if not found.
func (d *DiskDB) GetAttribute(entryPath, key string) (*models.Attribute, error) {
	row := d.db.QueryRow(`
		SELECT entry_path, key, value, source, computed_at
		FROM attributes WHERE entry_path = ? AND key = ?
	`, entryPath, key)

	attr := &models.Attribute{}
	err := row.Scan(&attr.EntryPath, &attr.Key, &attr.Value, &attr.Source, &attr.ComputedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return attr, nil
}

// GetAttributes retrieves all attributes for an entry
func (d *DiskDB) GetAttributes(entryPath string) ([]*models.Attribute, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, computed_at
		FROM attributes WHERE entry_path = ? ORDER BY key
	`, entryPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attrs []*models.Attribute
	for rows.Next() {
		attr := &models.Attribute{}
		if err := rows.Scan(&attr.EntryPath, &attr.Key, &attr.Value, &attr.Source, &attr.ComputedAt); err != nil {
			return nil, err
		}
		attrs = append(attrs, attr)
	}
	return attrs, rows.Err()
}

// QueryAttributesByKey retrieves all attributes with a given key across all entries
func (d *DiskDB) QueryAttributesByKey(key string) ([]*models.Attribute, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, computed_at
		FROM attributes WHERE key = ? ORDER BY entry_path
	`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attrs []*models.Attribute
	for rows.Next() {
		attr := &models.Attribute{}
		if err := rows.Scan(&attr.EntryPath, &attr.Key, &attr.Value, &attr.Source, &attr.ComputedAt); err != nil {
			return nil, err
		}
		attrs = append(attrs, attr)
	}
	return attrs, rows.Err()
}

// DeleteAttribute removes a single attribute
func (d *DiskDB) DeleteAttribute(entryPath, key string) error {
	_, err := d.db.Exec(`DELETE FROM attributes WHERE entry_path = ? AND key = ?`, entryPath, key)
	return err
}

// DeleteAttributesByEntry removes all attributes for an entry
func (d *DiskDB) DeleteAttributesByEntry(entryPath string) error {
	_, err := d.db.Exec(`DELETE FROM attributes WHERE entry_path = ?`, entryPath)
	return err
}
```

**Step 4: Wire table creation into database init**

In `pkg/database/database.go`, find the `init()` method (called from `NewDiskDB`). Add `initAttributesTable()` call after the other table initialization calls. Look for the pattern where other `init*Tables()` are called and add:

```go
if err := diskDB.initAttributesTable(); err != nil {
    writeQueue.Stop()
    db.Close()
    return nil, fmt.Errorf("failed to initialize attributes table: %w", err)
}
```

Add this after the existing `InitJobTables()` call around line 80 of `pkg/database/database.go`.

**Step 5: Run tests to verify they pass**

Run: `GO_ENV=test go test ./pkg/database/ -run "TestSetAttribute|TestGetAttribute|TestDeleteAttribute|TestSetAttributesBatch|TestQueryAttributesByKey" -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add pkg/database/attributes.go pkg/database/attributes_test.go pkg/database/database.go
git commit -m "feat: add attributes table and CRUD methods"
```

---

## Task 3: Add Backend interface methods for attributes

**Files:**
- Modify: `pkg/database/sqlite_backend.go` (add interface methods if Backend interface is defined there)

**Step 1: Check where Backend interface is defined**

Look for the `Backend` interface definition. It will need `SetAttribute`, `GetAttribute`, `GetAttributes`, `SetAttributesBatch`, `DeleteAttribute`, `DeleteAttributesByEntry`, `QueryAttributesByKey` methods added.

**Step 2: Add methods to Backend interface**

Add these method signatures to the `Backend` interface:

```go
// Attribute operations
SetAttribute(attr *models.Attribute) error
SetAttributesBatch(attrs []*models.Attribute) error
GetAttribute(entryPath, key string) (*models.Attribute, error)
GetAttributes(entryPath string) ([]*models.Attribute, error)
QueryAttributesByKey(key string) ([]*models.Attribute, error)
DeleteAttribute(entryPath, key string) error
DeleteAttributesByEntry(entryPath string) error
```

**Step 3: Run existing tests to verify nothing breaks**

Run: `GO_ENV=test go test ./pkg/database/... -v`
Expected: ALL PASS (DiskDB already implements these methods)

**Step 4: Commit**

```bash
git add pkg/database/sqlite_backend.go
git commit -m "feat: add attribute methods to Backend interface"
```

---

## Task 4: Implement the scan tool

**Files:**
- Create: `pkg/server/tool_scan.go`
- Create: `pkg/server/tool_scan_test.go`

**Step 1: Write the failing test**

Create `pkg/server/tool_scan_test.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanTool_BasicIndex(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir", "nested.txt"), []byte("world"), 0644))

	// Create in-memory DB
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Register tool
	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerScanTool(mcpServer, db)

	// Call scan synchronously (async=false)
	args := map[string]interface{}{
		"paths": []interface{}{tmpDir},
		"depth": -1,
		"async": false,
		"force": true,
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "scan"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "scan should not return error")

	// Verify entries in database
	entries, err := db.All()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 3, "should have at least tmpDir, test.txt, subdir, nested.txt")
}

func TestScanTool_MultiplePaths(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "a.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "b.txt"), []byte("b"), 0644))

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerScanTool(mcpServer, db)

	args := map[string]interface{}{
		"paths": []interface{}{tmpDir1, tmpDir2},
		"async": false,
		"force": true,
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "scan"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify both directories were indexed
	entry1, err := db.Get(tmpDir1)
	assert.NoError(t, err)
	assert.NotNil(t, entry1)

	entry2, err := db.Get(tmpDir2)
	assert.NoError(t, err)
	assert.NotNil(t, entry2)
}

func TestScanTool_AsyncReturnsJobID(t *testing.T) {
	tmpDir := t.TempDir()

	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerScanTool(mcpServer, db)

	args := map[string]interface{}{
		"paths": []interface{}{tmpDir},
		"async": true,
		"force": true,
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "scan"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Parse response - should contain job IDs
	var response map[string]interface{}
	textContent := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(textContent.Text), &response)
	require.NoError(t, err)
	assert.Contains(t, response, "jobs")
}

func TestScanTool_MissingPaths(t *testing.T) {
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerScanTool(mcpServer, db)

	request := mcp.CallToolRequest{}
	request.Params.Name = "scan"
	request.Params.Arguments = map[string]interface{}{}

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError, "scan without paths should error")
}
```

**Step 2: Run test to verify it fails**

Run: `GO_ENV=test go test ./pkg/server/ -run TestScanTool -v`
Expected: FAIL — `registerScanTool` undefined

**Step 3: Write the scan tool implementation**

Create `pkg/server/tool_scan.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
	"github.com/sirupsen/logrus"
)

// registerScanTool registers the scan tool (single-db, for tests)
func registerScanTool(s *server.MCPServer, db database.Backend) {
	tool := mcp.NewTool("scan",
		mcp.WithDescription("Index filesystem paths and extract attributes. Supports multiple paths, configurable depth, and optional attribute extraction."),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description("One or more filesystem paths to scan"),
		),
		mcp.WithArray("attributes",
			mcp.Description("Attributes to extract beyond base set: mime, hash.md5, hash.sha256, hash.perceptual, exif, permissions, thumbnail, video.thumbnails, media, text"),
		),
		mcp.WithNumber("depth",
			mcp.Description("Scan depth: -1=recursive (default), 0=this level only, N=N levels"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Re-index even if recently scanned (default: false)"),
		),
		mcp.WithString("target",
			mcp.Description("Resource set name to populate with results"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Return job ID immediately (default: true)"),
		),
		mcp.WithNumber("maxAge",
			mcp.Description("Max age in seconds before rescan (default: 3600)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleScan(ctx, request, db)
	})
}

// registerScanToolMP registers the scan tool with multi-project support
func registerScanToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("scan",
		mcp.WithDescription("Index filesystem paths and extract attributes. Supports multiple paths, configurable depth, and optional attribute extraction."),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description("One or more filesystem paths to scan"),
		),
		mcp.WithArray("attributes",
			mcp.Description("Attributes to extract beyond base set: mime, hash.md5, hash.sha256, hash.perceptual, exif, permissions, thumbnail, video.thumbnails, media, text"),
		),
		mcp.WithNumber("depth",
			mcp.Description("Scan depth: -1=recursive (default), 0=this level only, N=N levels"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Re-index even if recently scanned (default: false)"),
		),
		mcp.WithString("target",
			mcp.Description("Resource set name to populate with results"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Return job ID immediately (default: true)"),
		),
		mcp.WithNumber("maxAge",
			mcp.Description("Max age in seconds before rescan (default: 3600)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		return handleScan(ctx, request, db)
	})
}

func handleScan(ctx context.Context, request mcp.CallToolRequest, db database.Backend) (*mcp.CallToolResult, error) {
	var args struct {
		Paths      []string `json:"paths"`
		Attributes []string `json:"attributes,omitempty"`
		Depth      *int     `json:"depth,omitempty"`
		Force      *bool    `json:"force,omitempty"`
		Target     *string  `json:"target,omitempty"`
		Async      *bool    `json:"async,omitempty"`
		MaxAge     *int64   `json:"maxAge,omitempty"`
	}

	if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	if len(args.Paths) == 0 {
		return mcp.NewToolResultError("paths is required and must contain at least one path"), nil
	}

	// Build index options
	indexOpts := crawler.DefaultIndexOptions()
	if args.Force != nil && *args.Force {
		indexOpts.Force = true
	}
	if args.MaxAge != nil {
		indexOpts.MaxAge = *args.MaxAge
	}

	asyncMode := true
	if args.Async != nil {
		asyncMode = *args.Async
	}

	// Validate all paths first
	expandedPaths := make([]string, 0, len(args.Paths))
	for _, p := range args.Paths {
		expanded, err := pathutil.ExpandPath(p)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path %q: %v", p, err)), nil
		}
		if err := pathutil.ValidatePath(expanded); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path %q: %v", p, err)), nil
		}
		expandedPaths = append(expandedPaths, expanded)
	}

	if asyncMode {
		return handleScanAsync(db, expandedPaths, indexOpts)
	}
	return handleScanSync(db, expandedPaths, indexOpts)
}

func handleScanAsync(db database.Backend, paths []string, opts crawler.IndexOptions) (*mcp.CallToolResult, error) {
	type jobInfo struct {
		JobID     int64  `json:"job_id"`
		Path      string `json:"path"`
		StatusURL string `json:"status_url"`
	}

	jobs := make([]jobInfo, 0, len(paths))
	for _, p := range paths {
		jobID, err := db.CreateIndexJob(p, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create job for %q: %v", p, err)), nil
		}

		go func(path string, id int64) {
			if err := db.UpdateIndexJobStatus(id, "running", nil); err != nil {
				log.WithError(err).WithField("jobID", id).Error("Failed to mark job running")
				return
			}

			_, err := crawler.IndexWithOptions(path, db, nil, id, nil, opts)
			if err != nil {
				errMsg := err.Error()
				db.UpdateIndexJobStatus(id, "failed", &errMsg)
				log.WithError(err).WithFields(logrus.Fields{"jobID": id, "path": path}).Error("Scan failed")
			} else {
				db.UpdateIndexJobStatus(id, "completed", nil)
				log.WithFields(logrus.Fields{"jobID": id, "path": path}).Info("Scan completed")
			}
		}(p, jobID)

		jobs = append(jobs, jobInfo{
			JobID:     jobID,
			Path:      p,
			StatusURL: fmt.Sprintf("synthesis://jobs/%d", jobID),
		})
	}

	response := map[string]interface{}{
		"status": "started",
		"jobs":   jobs,
	}
	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}

func handleScanSync(db database.Backend, paths []string, opts crawler.IndexOptions) (*mcp.CallToolResult, error) {
	type pathResult struct {
		Path             string `json:"path"`
		FilesProcessed   int    `json:"files_processed"`
		DirsProcessed    int    `json:"dirs_processed"`
		TotalSize        int64  `json:"total_size"`
		Skipped          bool   `json:"skipped,omitempty"`
		Error            string `json:"error,omitempty"`
	}

	startTime := time.Now()
	results := make([]pathResult, 0, len(paths))

	for _, p := range paths {
		stats, err := crawler.IndexWithOptions(p, db, nil, 0, nil, opts)
		if err != nil {
			results = append(results, pathResult{Path: p, Error: err.Error()})
		} else {
			results = append(results, pathResult{
				Path:           p,
				FilesProcessed: stats.FilesProcessed,
				DirsProcessed:  stats.DirectoriesProcessed,
				TotalSize:      stats.TotalSize,
				Skipped:        stats.Skipped,
			})
		}
	}

	response := map[string]interface{}{
		"status":      "completed",
		"duration_ms": time.Since(startTime).Milliseconds(),
		"results":     results,
	}
	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}
```

**Step 4: Run tests to verify they pass**

Run: `GO_ENV=test go test ./pkg/server/ -run TestScanTool -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add pkg/server/tool_scan.go pkg/server/tool_scan_test.go
git commit -m "feat: add scan tool (replaces index/navigate/inspect)"
```

---

## Task 5: Implement the query tool

**Files:**
- Create: `pkg/server/tool_query.go`
- Create: `pkg/server/tool_query_test.go`

**Step 1: Write the failing test**

Create `pkg/server/tool_query_test.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupQueryTestDB(t *testing.T) *database.DiskDB {
	t.Helper()
	db, err := database.NewDiskDB(":memory:")
	require.NoError(t, err)

	now := time.Now().Unix()
	root := "/"
	testDir := "/photos"
	entries := []*models.Entry{
		{Path: "/photos", Parent: &root, Size: 0, Kind: "directory", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/photos/a.jpg", Parent: &testDir, Size: 5000, Kind: "file", Ctime: now, Mtime: now, LastScanned: now},
		{Path: "/photos/b.png", Parent: &testDir, Size: 10000, Kind: "file", Ctime: now, Mtime: now - 86400, LastScanned: now},
		{Path: "/photos/c.txt", Parent: &testDir, Size: 100, Kind: "file", Ctime: now, Mtime: now - 172800, LastScanned: now},
	}
	for _, e := range entries {
		require.NoError(t, db.InsertOrUpdate(e))
	}

	// Add attributes
	require.NoError(t, db.SetAttribute(&models.Attribute{EntryPath: "/photos/a.jpg", Key: "mime", Value: "image/jpeg", Source: "scan", ComputedAt: now}))
	require.NoError(t, db.SetAttribute(&models.Attribute{EntryPath: "/photos/b.png", Key: "mime", Value: "image/png", Source: "scan", ComputedAt: now}))
	require.NoError(t, db.SetAttribute(&models.Attribute{EntryPath: "/photos/c.txt", Key: "mime", Value: "text/plain", Source: "scan", ComputedAt: now}))

	return db
}

func TestQueryTool_BasicFilter(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerQueryTool(mcpServer, db)

	args := map[string]interface{}{
		"where": map[string]interface{}{
			"kind": "file",
		},
		"limit": 100,
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "query"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	textContent := result.Content[0].(mcp.TextContent)
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &response))
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 3) // a.jpg, b.png, c.txt
}

func TestQueryTool_SizeFilter(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerQueryTool(mcpServer, db)

	args := map[string]interface{}{
		"where": map[string]interface{}{
			"kind": "file",
			"size": map[string]interface{}{">": 1000},
		},
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "query"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	textContent := result.Content[0].(mcp.TextContent)
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &response))
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2) // a.jpg (5000), b.png (10000)
}

func TestQueryTool_Aggregate(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerQueryTool(mcpServer, db)

	args := map[string]interface{}{
		"where":     map[string]interface{}{"kind": "file"},
		"aggregate": "sum",
		"field":     "size",
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "query"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	textContent := result.Content[0].(mcp.TextContent)
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &response))
	assert.Equal(t, float64(15100), response["value"]) // 5000+10000+100
}

func TestQueryTool_OrderBy(t *testing.T) {
	db := setupQueryTestDB(t)
	defer db.Close()

	mcpServer := server.NewMCPServer("test", "0.0.1")
	registerQueryTool(mcpServer, db)

	args := map[string]interface{}{
		"where":    map[string]interface{}{"kind": "file"},
		"order_by": "-size",
		"limit":    2,
	}
	argsJSON, _ := json.Marshal(args)
	var argsMap map[string]interface{}
	json.Unmarshal(argsJSON, &argsMap)

	request := mcp.CallToolRequest{}
	request.Params.Name = "query"
	request.Params.Arguments = argsMap

	result, err := mcpServer.CallTool(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	textContent := result.Content[0].(mcp.TextContent)
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &response))
	entries := response["entries"].([]interface{})
	assert.Len(t, entries, 2)
	first := entries[0].(map[string]interface{})
	assert.Equal(t, "/photos/b.png", first["path"]) // largest first
}
```

**Step 2: Run test to verify it fails**

Run: `GO_ENV=test go test ./pkg/server/ -run TestQueryTool -v`
Expected: FAIL — `registerQueryTool` undefined

**Step 3: Write the query tool implementation**

Create `pkg/server/tool_query.go`. This is the most complex tool — it builds SQL dynamically from the `where` clause.

The implementer should:
1. Register `query` tool with parameters: `from`, `where`, `select`, `aggregate`, `field`, `group_by`, `order_by`, `limit`
2. Parse `where` object into SQL WHERE clauses. Support operators: exact match (string value), `>`, `<`, `>=`, `<=`, `like`, `after`/`before` (parse dates to unix timestamps)
3. For `aggregate` mode: run `SELECT {aggregate}({field}) FROM entries WHERE ...` and return `{"value": N}`
4. For normal mode: run `SELECT * FROM entries WHERE ... ORDER BY ... LIMIT ...` and return `{"entries": [...]}`
5. Support `from` parameter to filter by resource set membership (JOIN with `resource_set_entries`)
6. Support attribute-based filters by JOINing with `attributes` table when filter keys aren't base entry columns

Base entry columns (filter directly on `entries` table): `path`, `parent`, `size`, `kind`, `ctime`, `mtime`, `last_scanned`.

Attribute columns (require JOIN on `attributes`): anything else (e.g., `mime`, `hash.md5`, etc.)

Follow the same `registerQueryTool(s, db)` / `registerQueryToolMP(s, sc)` / `handleQuery(ctx, request, db)` pattern as the scan tool.

**Step 4: Run tests to verify they pass**

Run: `GO_ENV=test go test ./pkg/server/ -run TestQueryTool -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add pkg/server/tool_query.go pkg/server/tool_query_test.go
git commit -m "feat: add query tool (replaces all resource-* filter tools)"
```

---

## Task 6: Implement the manage tool

**Files:**
- Create: `pkg/server/tool_manage.go`
- Create: `pkg/server/tool_manage_test.go`

**Step 1: Write the failing test**

Create `pkg/server/tool_manage_test.go` with tests for resource-set CRUD, plan CRUD, and job listing. Follow the same pattern as scan/query tests.

Key test cases:
- `TestManageTool_ResourceSetCreate` — create a set, verify it exists
- `TestManageTool_ResourceSetList` — create 2 sets, list returns both
- `TestManageTool_ResourceSetDelete` — create then delete, verify gone
- `TestManageTool_PlanCreate` — create a plan with sources
- `TestManageTool_PlanList` — list plans
- `TestManageTool_JobList` — list jobs (should start empty)
- `TestManageTool_InvalidEntity` — error for unknown entity type
- `TestManageTool_InvalidAction` — error for unknown action

**Step 2: Write the manage tool**

Create `pkg/server/tool_manage.go`. The handler dispatches on `entity` + `action`:

```
entity=resource-set, action=create → db.CreateResourceSet(name, description)
entity=resource-set, action=list   → db.ListResourceSets()
entity=resource-set, action=get    → db.GetResourceSet(name)
entity=resource-set, action=update → db.UpdateResourceSet(name, description)
entity=resource-set, action=delete → db.DeleteResourceSet(name)
entity=plan, action=create         → db.CreatePlan(plan)
entity=plan, action=list           → db.ListPlans()
entity=plan, action=get            → db.GetPlan(name)
entity=plan, action=update         → db.UpdatePlan(plan)
entity=plan, action=delete         → db.DeletePlan(name)
entity=project, action=list        → sc.ProjectManager.ListProjects()
entity=job, action=list            → db.ListJobs()
entity=job, action=get             → db.GetIndexJobStatus(id)
```

Follow the `registerManageTool(s, db)` / `registerManageToolMP(s, sc)` / `handleManage(...)` pattern.

**Step 3: Run tests, verify pass**

Run: `GO_ENV=test go test ./pkg/server/ -run TestManageTool -v`

**Step 4: Commit**

```bash
git add pkg/server/tool_manage.go pkg/server/tool_manage_test.go
git commit -m "feat: add manage tool (replaces 21 CRUD tools)"
```

---

## Task 7: Implement the batch tool

**Files:**
- Create: `pkg/server/tool_batch.go`
- Create: `pkg/server/tool_batch_test.go`

**Step 1: Write the failing test**

Key test cases:
- `TestBatchTool_Attributes` — bulk extract attributes for multiple paths
- `TestBatchTool_Duplicates_Exact` — find duplicate files by hash (create 2 files with same content, verify detected)
- `TestBatchTool_Move` — move files and verify entries updated
- `TestBatchTool_Delete` — delete files and verify entries removed
- `TestBatchTool_InvalidOperation` — error for unknown operation

**Step 2: Write the batch tool**

Create `pkg/server/tool_batch.go`. Operations:

- `attributes`: For each path in `paths` or entries in `from` resource set, extract requested attributes and store in `attributes` table
- `duplicates`: Query entries, group by `hash.md5` attribute where count > 1, return groups
- `move`: `os.Rename` each file, update entry paths in DB
- `delete`: `os.Remove` each file, delete entries from DB

**Step 3: Run tests, verify pass**

Run: `GO_ENV=test go test ./pkg/server/ -run TestBatchTool -v`

**Step 4: Commit**

```bash
git add pkg/server/tool_batch.go pkg/server/tool_batch_test.go
git commit -m "feat: add batch tool for multi-file operations"
```

---

## Task 8: Implement the watch tool

**Files:**
- Create: `pkg/server/tool_watch.go`
- Create: `pkg/server/tool_watch_test.go`

**Step 1: Write the failing test**

Key test cases:
- `TestWatchTool_StartStop` — start watching a temp dir, verify status, stop it
- `TestWatchTool_List` — start 2 watches, list returns both
- `TestWatchTool_Status` — start a watch, check status returns correct info
- `TestWatchTool_InvalidAction` — error for unknown action

**Step 2: Write the watch tool**

Create `pkg/server/tool_watch.go`. This wraps the existing `pkg/sources/` filesystem.watch logic.

Actions:
- `start`: Create an fsnotify watcher on the path, register it in a global `watchRegistry` map
- `stop`: Look up watcher by path or ID, close it
- `status`: Return info about a specific watcher
- `list`: Return all active watchers

**Step 3: Run tests, verify pass**

Run: `GO_ENV=test go test ./pkg/server/ -run TestWatchTool -v`

**Step 4: Commit**

```bash
git add pkg/server/tool_watch.go pkg/server/tool_watch_test.go
git commit -m "feat: add watch tool for real-time filesystem monitoring"
```

---

## Task 9: Rewrite resource templates

**Files:**
- Create: `pkg/server/resources.go` (replaces `mcp_resources.go`)
- Create: `pkg/server/resources_test.go`

**Step 1: Write the failing test**

Test that 8 resource templates are registered and return correct data:
- `synthesis://entries/{path}` — returns entry + attributes
- `synthesis://entries/{path}/attributes` — returns just attributes
- `synthesis://sets` — returns resource set list
- `synthesis://sets/{name}` — returns set details
- `synthesis://sets/{name}/entries` — returns entries in set
- `synthesis://jobs` — returns job list
- `synthesis://jobs/{id}` — returns job details
- `synthesis://projects` — returns project list

**Step 2: Write the resource templates**

Create `pkg/server/resources.go` with `registerResources(s, db)` and `registerResourcesMP(s, sc)` functions. Each template handler queries the database and returns JSON.

**Step 3: Run tests, verify pass**

Run: `GO_ENV=test go test ./pkg/server/ -run TestResource -v`

**Step 4: Commit**

```bash
git add pkg/server/resources.go pkg/server/resources_test.go
git commit -m "feat: add 8 resource templates (replaces 31)"
```

---

## Task 10: Wire new tools into server, remove old tools

**Files:**
- Modify: `pkg/server/server.go` (lines 343-353, 390-394)
- Delete: `pkg/server/mcp_tools.go`
- Delete: `pkg/server/mcp_resource_tools.go`
- Delete: `pkg/server/mcp_source_tools.go`
- Delete: `pkg/server/mcp_project_tools.go`
- Delete: `pkg/server/mcp_classifier_tools.go`
- Delete: `pkg/server/mcp_resources.go`

**Step 1: Update server.go tool registration**

Replace lines 343-353 in `pkg/server/server.go`:

```go
// OLD:
registerProjectTools(mcpServer, sc)
registerMCPToolsWithContext(mcpServer, sc, classifierProcessor)
registerClassifierToolsWithContext(mcpServer, sc, classifierProcessor)
registerMCPResourcesWithContext(mcpServer, sc)

// NEW:
registerScanToolMP(mcpServer, sc)
registerQueryToolMP(mcpServer, sc)
registerManageToolMP(mcpServer, sc)
registerBatchToolMP(mcpServer, sc)
registerWatchToolMP(mcpServer, sc)
registerResourcesMP(mcpServer, sc)
```

Also remove the now-unused wrapper functions `registerMCPToolsWithContext`, `registerClassifierToolsWithContext`, `registerMCPResourcesWithContext` from server.go (lines 390-406).

**Step 2: Delete old tool files**

```bash
rm pkg/server/mcp_tools.go
rm pkg/server/mcp_resource_tools.go
rm pkg/server/mcp_source_tools.go
rm pkg/server/mcp_project_tools.go
rm pkg/server/mcp_classifier_tools.go
rm pkg/server/mcp_resources.go
```

**Step 3: Fix compilation**

After deleting old files, run `go build ./...` and fix any remaining references to deleted functions. Common things to fix:
- `requireProjectDB` — may need to move to a shared location if it was in `mcp_tools.go`
- `unmarshalArgs` — same, move to `shared_utils.go` if needed
- `getBoolOrDefault` — same
- `contentBaseURL`, `artifactCacheDir` — keep wherever they are currently defined

**Step 4: Run all tests**

Run: `GO_ENV=test go test ./... -v`
Expected: ALL PASS (some old tests may need removal if they tested deleted functions)

**Step 5: Commit**

```bash
git add -A
git commit -m "refactor: wire 5 new tools, remove 59 old tools"
```

---

## Task 11: Delete old documentation

**Files:**
- Delete: all files in `docs/` except `docs/plans/`

**Step 1: Remove old docs**

```bash
rm docs/ARCHITECTURE_C3.md
rm docs/ENTITY_RELATIONSHIP.md
rm docs/PLANS_IMPLEMENTATION_GUIDE.md
rm docs/RESOURCE_SET_ARCHITECTURE.md
rm docs/MCP_REFERENCE.md
rm docs/PLANS_ARCHITECTURE.md
rm docs/REPLATFORM_ANALYSIS.md
rm docs/PLANS_MIGRATION_GUIDE.md
rm docs/MODULE_ARCHITECTURE.md
rm docs/RESOURCE_SET_IMPLEMENTATION_PLAN.md
rm docs/REPLATFORM_SUMMARY.md
rm docs/TEST_COVERAGE.md
rm docs/mcp_low_context_domain_model.md
rm docs/inspect_enrichment_design.md
rm docs/REPLATFORM_INDEX.md
rm -f docs/swagger.json docs/swagger.yaml docs/docs.go
```

**Step 2: Commit**

```bash
git add -A
git commit -m "chore: remove outdated documentation"
```

---

## Task 12: Write new ARCHITECTURE.md

**Files:**
- Create: `docs/ARCHITECTURE.md`

Write a concise architecture doc covering:
1. System overview (one paragraph)
2. Component diagram (ASCII): CLI → Server → MCP Tools → Database/Crawler
3. Data flow: scan → entries + attributes → query
4. Key packages and their roles (one line each)
5. Database schema (entries + attributes tables)
6. Keep it under 150 lines

**Commit:**

```bash
git add docs/ARCHITECTURE.md
git commit -m "docs: add architecture overview"
```

---

## Task 13: Write new MCP_REFERENCE.md

**Files:**
- Create: `docs/MCP_REFERENCE.md`

Document the 5 tools and 8 resource templates from the design doc. For each tool: description, parameters table, example request, example response. Keep it factual and concise.

**Commit:**

```bash
git add docs/MCP_REFERENCE.md
git commit -m "docs: add MCP reference for 5-tool interface"
```

---

## Task 14: Write new SCHEMA.md

**Files:**
- Create: `docs/SCHEMA.md`

Document the database schema: `entries`, `attributes`, `resource_sets`, `resource_set_entries`, `resource_set_edges`, `plans`, `sources`, `rules`, `index_jobs` tables. Show CREATE TABLE statements and explain the attribute model.

**Commit:**

```bash
git add docs/SCHEMA.md
git commit -m "docs: add database schema reference"
```

---

## Task 15: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

Update to reflect:
1. 5 MCP tools instead of 59 (scan, query, manage, batch, watch)
2. 8 resource templates instead of 31
3. Attribute system description
4. Remove references to old tool names
5. Update the MCP Tools section with new tool list
6. Update the architecture section

**Commit:**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for radical consolidation"
```

---

## Task 16: Merge source packages

**Files:**
- Modify: `pkg/sources/` (keep this one)
- Delete: `pkg/source/` (merge into sources)

**Step 1: Check what's in pkg/source/**

Identify what `pkg/source/` exports that `pkg/sources/` doesn't. Move any unique code into `pkg/sources/`.

**Step 2: Update all imports**

Find all files importing `pkg/source` and change to `pkg/sources`.

**Step 3: Delete pkg/source/**

```bash
rm -rf pkg/source/
```

**Step 4: Run tests**

Run: `GO_ENV=test go test ./... -v`

**Step 5: Commit**

```bash
git add -A
git commit -m "refactor: merge pkg/source into pkg/sources"
```

---

## Task 17: Final verification

**Step 1: Run full test suite with coverage**

```bash
GO_ENV=test go test -v -cover ./...
```

Verify coverage is >= 80%.

**Step 2: Build binary**

```bash
go build -o mcp-space-browser ./cmd/mcp-space-browser
```

**Step 3: Smoke test**

```bash
./mcp-space-browser server --port=3000 &
# Verify MCP endpoint responds:
curl -X POST http://localhost:3000/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
# Should return 5 tools: scan, query, manage, batch, watch
kill %1
```

**Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address issues found during verification"
```
