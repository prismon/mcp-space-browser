package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSQLiteBackend(t *testing.T) {
	t.Run("creates backend with default config", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		assert.NotNil(t, backend)
		assert.Equal(t, "sqlite3", backend.Type())
		assert.Equal(t, filepath.Join(tmpDir, "disk.db"), backend.Path())
		assert.False(t, backend.IsOpen())
	})

	t.Run("creates backend with custom config", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &SQLiteConfig{
			Path:          "custom.db",
			WALMode:       true,
			BusyTimeoutMs: 10000,
		}
		backend := NewSQLiteBackend(tmpDir, config)

		assert.NotNil(t, backend)
		assert.Equal(t, filepath.Join(tmpDir, "custom.db"), backend.Path())
	})

	t.Run("handles absolute path", func(t *testing.T) {
		tmpDir := t.TempDir()
		absPath := filepath.Join(tmpDir, "absolute.db")
		config := &SQLiteConfig{
			Path: absPath,
		}
		backend := NewSQLiteBackend(tmpDir, config)

		assert.Equal(t, absPath, backend.Path())
	})
}

func TestSQLiteBackend_OpenClose(t *testing.T) {
	t.Run("opens and closes successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		// Should be closed initially
		assert.False(t, backend.IsOpen())

		// Open
		err := backend.Open()
		require.NoError(t, err)
		assert.True(t, backend.IsOpen())
		assert.NotNil(t, backend.DB())

		// Verify file was created
		_, err = os.Stat(backend.Path())
		assert.NoError(t, err)

		// Close
		err = backend.Close()
		require.NoError(t, err)
		assert.False(t, backend.IsOpen())
	})

	t.Run("open is idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		err := backend.Open()
		require.NoError(t, err)

		// Opening again should succeed
		err = backend.Open()
		require.NoError(t, err)
		assert.True(t, backend.IsOpen())

		backend.Close()
	})

	t.Run("close is idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		err := backend.Open()
		require.NoError(t, err)

		err = backend.Close()
		require.NoError(t, err)

		// Closing again should succeed
		err = backend.Close()
		require.NoError(t, err)
	})
}

func TestSQLiteBackend_ConnectionInfo(t *testing.T) {
	tmpDir := t.TempDir()
	backend := NewSQLiteBackend(tmpDir, nil)

	info := backend.ConnectionInfo()
	assert.Contains(t, info, "SQLite")
	assert.Contains(t, info, backend.Path())
}

func TestSQLiteBackend_InitSchema(t *testing.T) {
	t.Run("initializes schema successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		err := backend.Open()
		require.NoError(t, err)
		defer backend.Close()

		err = backend.InitSchema()
		require.NoError(t, err)

		// Verify tables exist
		db := backend.DB()
		tables := []string{
			"entries",
			"resource_sets",
			"resource_set_entries",
			"resource_set_edges",
			"queries",
			"query_executions",
			"metadata",
			"sources",
			"rules",
			"rule_executions",
			"rule_outcomes",
			"plans",
			"plan_executions",
			"plan_outcome_records",
			"index_jobs",
			"classifier_jobs",
		}

		for _, table := range tables {
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
			require.NoError(t, err, "checking table %s", table)
			assert.Equal(t, 1, count, "table %s should exist", table)
		}
	})

	t.Run("fails when not open", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		err := backend.InitSchema()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not open")
	})

	t.Run("is idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := NewSQLiteBackend(tmpDir, nil)

		err := backend.Open()
		require.NoError(t, err)
		defer backend.Close()

		// Initialize twice
		err = backend.InitSchema()
		require.NoError(t, err)

		err = backend.InitSchema()
		require.NoError(t, err)
	})
}

func TestSQLiteBackend_WALMode(t *testing.T) {
	t.Run("enables WAL mode when configured", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &SQLiteConfig{
			Path:    "wal_test.db",
			WALMode: true,
		}
		backend := NewSQLiteBackend(tmpDir, config)

		err := backend.Open()
		require.NoError(t, err)
		defer backend.Close()

		var journalMode string
		err = backend.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
		require.NoError(t, err)
		assert.Equal(t, "wal", journalMode)
	})

	t.Run("does not enable WAL mode when disabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &SQLiteConfig{
			Path:    "no_wal_test.db",
			WALMode: false,
		}
		backend := NewSQLiteBackend(tmpDir, config)

		err := backend.Open()
		require.NoError(t, err)
		defer backend.Close()

		var journalMode string
		err = backend.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
		require.NoError(t, err)
		// Default is 'delete' mode
		assert.NotEqual(t, "wal", journalMode)
	})
}

func TestSQLiteBackend_BusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	config := &SQLiteConfig{
		Path:          "timeout_test.db",
		WALMode:       true,
		BusyTimeoutMs: 10000,
	}
	backend := NewSQLiteBackend(tmpDir, config)

	err := backend.Open()
	require.NoError(t, err)
	defer backend.Close()

	var timeout int
	err = backend.DB().QueryRow("PRAGMA busy_timeout").Scan(&timeout)
	require.NoError(t, err)
	assert.Equal(t, 10000, timeout)
}

func TestNewBackend(t *testing.T) {
	t.Run("creates SQLite backend by default", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend, err := NewBackend(tmpDir, nil)
		require.NoError(t, err)
		assert.Equal(t, "sqlite3", backend.Type())
	})

	t.Run("creates SQLite backend when explicitly specified", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &BackendConfig{
			Type: "sqlite3",
		}
		backend, err := NewBackend(tmpDir, config)
		require.NoError(t, err)
		assert.Equal(t, "sqlite3", backend.Type())
	})

	t.Run("returns error for postgresql (not implemented)", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &BackendConfig{
			Type: "postgresql",
		}
		_, err := NewBackend(tmpDir, config)
		require.Error(t, err)

		var notImplErr *BackendNotImplementedError
		assert.ErrorAs(t, err, &notImplErr)
		assert.Equal(t, "postgresql", notImplErr.BackendType)
	})

	t.Run("returns error for unknown backend", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &BackendConfig{
			Type: "mongodb",
		}
		_, err := NewBackend(tmpDir, config)
		require.Error(t, err)

		var unknownErr *UnknownBackendError
		assert.ErrorAs(t, err, &unknownErr)
		assert.Equal(t, "mongodb", unknownErr.BackendType)
	})
}

func TestDefaultConfigs(t *testing.T) {
	t.Run("DefaultSQLiteConfig", func(t *testing.T) {
		config := DefaultSQLiteConfig()
		assert.Equal(t, "disk.db", config.Path)
		assert.True(t, config.WALMode)
		assert.Equal(t, 5000, config.BusyTimeoutMs)
	})

	t.Run("DefaultBackendConfig", func(t *testing.T) {
		config := DefaultBackendConfig()
		assert.Equal(t, "sqlite3", config.Type)
		assert.NotNil(t, config.SQLite)
		assert.Equal(t, "disk.db", config.SQLite.Path)
	})
}
