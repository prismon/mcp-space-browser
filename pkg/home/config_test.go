package home

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Initialize to create default config
	err = mgr.Initialize()
	require.NoError(t, err)

	// Load config
	config, err := mgr.LoadConfig()
	require.NoError(t, err)

	// Verify default values
	assert.Equal(t, "disk.db", config.Database.Path)
	assert.True(t, config.Rules.AutoExecute)
	assert.True(t, config.Rules.HotReload)
	assert.Equal(t, 4, config.Rules.MaxConcurrent)
	assert.True(t, config.Cache.Enabled)
	assert.Equal(t, int64(10737418240), config.Cache.MaxSize)
	assert.Equal(t, 320, config.Cache.Thumbnails.MaxWidth)
	assert.Equal(t, 320, config.Cache.Thumbnails.MaxHeight)
	assert.Equal(t, 85, config.Cache.Thumbnails.Quality)
	assert.Equal(t, "info", config.Logging.Level)
	assert.Equal(t, 3000, config.Server.Port)
	assert.Equal(t, "localhost", config.Server.Host)
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Create a config
	config := &Config{
		Database: DatabaseConfig{
			Path: "custom.db",
		},
		Rules: RulesConfig{
			AutoExecute:   false,
			HotReload:     false,
			MaxConcurrent: 8,
		},
		Cache: CacheConfig{
			Enabled: false,
			MaxSize: 1000,
			Thumbnails: ThumbnailsConfig{
				MaxWidth:  640,
				MaxHeight: 480,
				Quality:   90,
			},
			Timelines: TimelinesConfig{
				FrameCount: 5,
				MaxWidth:   200,
				MaxHeight:  150,
			},
		},
		Logging: LoggingConfig{
			Level:      "debug",
			File:       "custom.log",
			MaxSize:    1000000,
			MaxBackups: 5,
		},
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
	}

	// Save config
	err = mgr.SaveConfig(config)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(mgr.ConfigPath())
	assert.NoError(t, err)

	// Load and verify
	loaded, err := mgr.LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "custom.db", loaded.Database.Path)
	assert.False(t, loaded.Rules.AutoExecute)
	assert.False(t, loaded.Rules.HotReload)
	assert.Equal(t, 8, loaded.Rules.MaxConcurrent)
	assert.False(t, loaded.Cache.Enabled)
	assert.Equal(t, int64(1000), loaded.Cache.MaxSize)
	assert.Equal(t, 640, loaded.Cache.Thumbnails.MaxWidth)
	assert.Equal(t, 480, loaded.Cache.Thumbnails.MaxHeight)
	assert.Equal(t, 90, loaded.Cache.Thumbnails.Quality)
	assert.Equal(t, 5, loaded.Cache.Timelines.FrameCount)
	assert.Equal(t, "debug", loaded.Logging.Level)
	assert.Equal(t, 8080, loaded.Server.Port)
	assert.Equal(t, "0.0.0.0", loaded.Server.Host)
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, DatabaseFile, config.Database.Path)
	assert.True(t, config.Rules.AutoExecute)
	assert.True(t, config.Rules.HotReload)
	assert.Equal(t, 4, config.Rules.MaxConcurrent)
	assert.True(t, config.Cache.Enabled)
	assert.Equal(t, int64(10737418240), config.Cache.MaxSize)
	assert.Equal(t, 320, config.Cache.Thumbnails.MaxWidth)
	assert.Equal(t, 320, config.Cache.Thumbnails.MaxHeight)
	assert.Equal(t, 85, config.Cache.Thumbnails.Quality)
	assert.Equal(t, 10, config.Cache.Timelines.FrameCount)
	assert.Equal(t, 160, config.Cache.Timelines.MaxWidth)
	assert.Equal(t, 120, config.Cache.Timelines.MaxHeight)
	assert.Equal(t, "info", config.Logging.Level)
	assert.Equal(t, "logs/mcp-space-browser.log", config.Logging.File)
	assert.Equal(t, int64(104857600), config.Logging.MaxSize)
	assert.Equal(t, 3, config.Logging.MaxBackups)
	assert.Equal(t, 3000, config.Server.Port)
	assert.Equal(t, "localhost", config.Server.Host)
}

func TestLoadConfigErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("returns error when config file doesn't exist", func(t *testing.T) {
		_, err := mgr.LoadConfig()
		assert.Error(t, err)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		// Write invalid YAML
		invalidYAML := "invalid: yaml: content: :"
		err := os.WriteFile(mgr.ConfigPath(), []byte(invalidYAML), 0644)
		require.NoError(t, err)

		_, err = mgr.LoadConfig()
		assert.Error(t, err)
	})
}
