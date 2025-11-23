package home

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	t.Run("creates manager with custom path", func(t *testing.T) {
		mgr, err := NewManager("/tmp/test-home")
		require.NoError(t, err)
		assert.Contains(t, mgr.Path(), "test-home")
	})

	t.Run("creates manager with default path when empty", func(t *testing.T) {
		mgr, err := NewManager("")
		require.NoError(t, err)
		assert.NotEmpty(t, mgr.Path())
	})
}

func TestDefaultHomePath(t *testing.T) {
	// Save original env vars
	origMcpHome := os.Getenv("MCP_HOME")
	origMcpSpaceHome := os.Getenv("MCP_SPACE_BROWSER_HOME")
	defer func() {
		os.Setenv("MCP_HOME", origMcpHome)
		os.Setenv("MCP_SPACE_BROWSER_HOME", origMcpSpaceHome)
	}()

	t.Run("respects MCP_HOME env var", func(t *testing.T) {
		os.Setenv("MCP_HOME", "/custom/mcp/home")
		os.Setenv("MCP_SPACE_BROWSER_HOME", "")

		path := DefaultHomePath()
		assert.Equal(t, "/custom/mcp/home", path)
	})

	t.Run("respects MCP_SPACE_BROWSER_HOME env var", func(t *testing.T) {
		os.Setenv("MCP_HOME", "")
		os.Setenv("MCP_SPACE_BROWSER_HOME", "/custom/space/home")

		path := DefaultHomePath()
		assert.Equal(t, "/custom/space/home", path)
	})

	t.Run("MCP_HOME takes precedence over MCP_SPACE_BROWSER_HOME", func(t *testing.T) {
		os.Setenv("MCP_HOME", "/mcp/home")
		os.Setenv("MCP_SPACE_BROWSER_HOME", "/space/home")

		path := DefaultHomePath()
		assert.Equal(t, "/mcp/home", path)
	})

	t.Run("falls back to default when no env vars set", func(t *testing.T) {
		os.Setenv("MCP_HOME", "")
		os.Setenv("MCP_SPACE_BROWSER_HOME", "")

		path := DefaultHomePath()
		assert.Contains(t, path, ".mcp-space-browser")
	})
}

func TestManagerInitialize(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Initialize
	err = mgr.Initialize()
	require.NoError(t, err)

	// Verify all directories were created
	dirs := []string{
		RulesDir,
		RulesEnabledDir,
		RulesDisabledDir,
		RulesExamplesDir,
		CacheDir,
		ThumbnailsDir,
		TimelinesDir,
		MetadataCacheDir,
		TempDir,
		LogsDir,
	}

	for _, dir := range dirs {
		path := mgr.JoinPath(dir)
		info, err := os.Stat(path)
		assert.NoError(t, err, "directory should exist: %s", dir)
		assert.True(t, info.IsDir(), "should be a directory: %s", dir)
	}

	// Verify config file was created
	configPath := mgr.ConfigPath()
	_, err = os.Stat(configPath)
	assert.NoError(t, err, "config.yaml should exist")

	// Verify example files were created
	examplesPath := mgr.RulesExamplesPath()
	readmePath := filepath.Join(examplesPath, "README.md")
	_, err = os.Stat(readmePath)
	assert.NoError(t, err, "README.md should exist in examples")

	// Verify .gitignore was created
	gitignorePath := mgr.JoinPath(".gitignore")
	_, err = os.Stat(gitignorePath)
	assert.NoError(t, err, ".gitignore should exist")
}

func TestManagerExists(t *testing.T) {
	t.Run("returns true for existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		assert.True(t, mgr.Exists())
	})

	t.Run("returns false for non-existing directory", func(t *testing.T) {
		mgr, err := NewManager("/tmp/does-not-exist-12345")
		require.NoError(t, err)

		assert.False(t, mgr.Exists())
	})
}

func TestManagerPaths(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name     string
		method   func() string
		expected string
	}{
		{"ConfigPath", mgr.ConfigPath, filepath.Join(tmpDir, ConfigFile)},
		{"DatabasePath", mgr.DatabasePath, filepath.Join(tmpDir, DatabaseFile)},
		{"RulesEnabledPath", mgr.RulesEnabledPath, filepath.Join(tmpDir, RulesEnabledDir)},
		{"RulesDisabledPath", mgr.RulesDisabledPath, filepath.Join(tmpDir, RulesDisabledDir)},
		{"RulesExamplesPath", mgr.RulesExamplesPath, filepath.Join(tmpDir, RulesExamplesDir)},
		{"ThumbnailsPath", mgr.ThumbnailsPath, filepath.Join(tmpDir, ThumbnailsDir)},
		{"TimelinesPath", mgr.TimelinesPath, filepath.Join(tmpDir, TimelinesDir)},
		{"MetadataCachePath", mgr.MetadataCachePath, filepath.Join(tmpDir, MetadataCacheDir)},
		{"TempPath", mgr.TempPath, filepath.Join(tmpDir, TempDir)},
		{"LogsPath", mgr.LogsPath, filepath.Join(tmpDir, LogsDir)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.method())
		})
	}
}

func TestManagerGetCachePath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("creates content-addressed path for normal hash", func(t *testing.T) {
		hash := "abcdef1234567890"
		path := mgr.GetCachePath("thumbnails", hash, ".jpg")

		expected := filepath.Join(tmpDir, CacheDir, "thumbnails", "ab", "cd", hash+".jpg")
		assert.Equal(t, expected, path)
	})

	t.Run("handles short hash", func(t *testing.T) {
		hash := "abc"
		path := mgr.GetCachePath("thumbnails", hash, ".jpg")

		expected := filepath.Join(tmpDir, CacheDir, "thumbnails", hash+".jpg")
		assert.Equal(t, expected, path)
	})
}

func TestManagerEnsureCacheDir(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("creates cache subdirectories", func(t *testing.T) {
		hash := "abcdef1234567890"
		err := mgr.EnsureCacheDir("thumbnails", hash)
		require.NoError(t, err)

		expectedDir := filepath.Join(tmpDir, CacheDir, "thumbnails", "ab", "cd")
		info, err := os.Stat(expectedDir)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("handles short hash", func(t *testing.T) {
		hash := "abc"
		err := mgr.EnsureCacheDir("thumbnails", hash)
		require.NoError(t, err)

		expectedDir := filepath.Join(tmpDir, CacheDir, "thumbnails")
		info, err := os.Stat(expectedDir)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestManagerCleanTemp(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Create temp directory with some files
	err = mgr.Initialize()
	require.NoError(t, err)

	tempFile := filepath.Join(mgr.TempPath(), "test.txt")
	err = os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Clean temp
	err = mgr.CleanTemp()
	require.NoError(t, err)

	// Verify temp directory exists but file is gone
	info, err := os.Stat(mgr.TempPath())
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	_, err = os.Stat(tempFile)
	assert.True(t, os.IsNotExist(err), "temp file should be deleted")
}

func TestManagerGetCacheSize(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	err = mgr.Initialize()
	require.NoError(t, err)

	// Create some files in cache
	file1 := filepath.Join(mgr.ThumbnailsPath(), "test1.jpg")
	file2 := filepath.Join(mgr.TimelinesPath(), "test2.jpg")

	err = os.WriteFile(file1, make([]byte, 1024), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2, make([]byte, 2048), 0644)
	require.NoError(t, err)

	// Get cache size
	size, err := mgr.GetCacheSize()
	require.NoError(t, err)

	assert.Equal(t, int64(3072), size)
}

func TestManagerInitializeIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Initialize twice
	err = mgr.Initialize()
	require.NoError(t, err)

	err = mgr.Initialize()
	require.NoError(t, err)

	// Should not error and files should still exist
	_, err = os.Stat(mgr.ConfigPath())
	assert.NoError(t, err)
}
