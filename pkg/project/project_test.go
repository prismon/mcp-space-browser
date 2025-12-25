package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("test-project")

	assert.Equal(t, "test-project", config.Name)
	assert.NotZero(t, config.CreatedAt)
	assert.NotZero(t, config.UpdatedAt)
	assert.Equal(t, "sqlite3", config.Database.Type)
	assert.NotNil(t, config.Database.SQLite)
	assert.Equal(t, "disk.db", config.Database.SQLite.Path)
}

func TestConfigSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "project.yaml")

	// Create and save config
	config := DefaultConfig("my-project")
	config.Description = "Test project"

	err := config.Save(configPath)
	require.NoError(t, err)

	// Load config
	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, "my-project", loaded.Name)
	assert.Equal(t, "Test project", loaded.Description)
	assert.Equal(t, "sqlite3", loaded.Database.Type)
}

func TestConfigValidate(t *testing.T) {
	t.Run("valid sqlite config", func(t *testing.T) {
		config := DefaultConfig("test")
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing name", func(t *testing.T) {
		config := DefaultConfig("")
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("missing sqlite path", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Database.SQLite.Path = ""
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path is required")
	})

	t.Run("unknown backend", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Database.Type = "mongodb"
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
	})
}

func TestConfigClone(t *testing.T) {
	config := DefaultConfig("original")
	config.Description = "Original description"

	clone := config.Clone()

	// Modify original
	config.Name = "modified"
	config.Description = "Modified description"
	config.Database.SQLite.Path = "modified.db"

	// Clone should be unchanged
	assert.Equal(t, "original", clone.Name)
	assert.Equal(t, "Original description", clone.Description)
	assert.Equal(t, "disk.db", clone.Database.SQLite.Path)
}

func TestProject(t *testing.T) {
	t.Run("creates and initializes project", func(t *testing.T) {
		tmpDir := t.TempDir()
		projectPath := filepath.Join(tmpDir, "test-project")

		// Create config
		config := DefaultConfig("test-project")
		config.Description = "Test"

		project := &Project{
			Name:        config.Name,
			Description: config.Description,
			Path:        projectPath,
			ConfigPath:  filepath.Join(projectPath, ConfigFileName),
			LogsPath:    filepath.Join(projectPath, LogsDir),
			Config:      config,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
		}

		// Initialize
		err := project.Initialize()
		require.NoError(t, err)

		// Check directories exist
		assert.DirExists(t, projectPath)
		assert.DirExists(t, project.LogsPath)
		assert.FileExists(t, project.ConfigPath)

		// Check project exists
		assert.True(t, project.Exists())
	})

	t.Run("loads existing project", func(t *testing.T) {
		tmpDir := t.TempDir()
		projectPath := filepath.Join(tmpDir, "existing-project")

		// Create project first
		config := DefaultConfig("existing-project")
		config.Description = "Existing"

		project := &Project{
			Name:       config.Name,
			Path:       projectPath,
			ConfigPath: filepath.Join(projectPath, ConfigFileName),
			LogsPath:   filepath.Join(projectPath, LogsDir),
			Config:     config,
		}
		err := project.Initialize()
		require.NoError(t, err)

		// Load it
		loaded, err := NewProject(projectPath)
		require.NoError(t, err)

		assert.Equal(t, "existing-project", loaded.Name)
		assert.Equal(t, "Existing", loaded.Description)
	})

	t.Run("returns correct DBPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := DefaultConfig("test")

		project := &Project{
			Path:   tmpDir,
			Config: config,
		}

		assert.Equal(t, filepath.Join(tmpDir, "disk.db"), project.DBPath())
	})
}

func TestProjectStats(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "stats-project")

	config := DefaultConfig("stats-project")
	project := &Project{
		Name:       config.Name,
		Path:       projectPath,
		ConfigPath: filepath.Join(projectPath, ConfigFileName),
		LogsPath:   filepath.Join(projectPath, LogsDir),
		Config:     config,
		CreatedAt:  config.CreatedAt,
		UpdatedAt:  config.UpdatedAt,
	}

	err := project.Initialize()
	require.NoError(t, err)

	stats, err := project.Stats()
	require.NoError(t, err)

	assert.Equal(t, "stats-project", stats.Name)
	assert.Equal(t, "sqlite3", stats.BackendType)
	assert.NotEmpty(t, stats.DatabasePath)
}

func TestManager(t *testing.T) {
	t.Run("creates and manages projects", func(t *testing.T) {
		tmpDir := t.TempDir()
		projectsDir := filepath.Join(tmpDir, "projects")
		cacheDir := filepath.Join(tmpDir, "cache")

		manager, err := NewManager(projectsDir, cacheDir)
		require.NoError(t, err)
		defer manager.Close()

		// Create project
		project, err := manager.CreateProject("my-project", "My test project")
		require.NoError(t, err)
		assert.Equal(t, "my-project", project.Name)
		assert.Equal(t, "My test project", project.Description)

		// List projects
		projects, err := manager.ListProjects()
		require.NoError(t, err)
		assert.Len(t, projects, 1)
		assert.Equal(t, "my-project", projects[0].Name)

		// Get project
		got, err := manager.GetProject("my-project")
		require.NoError(t, err)
		assert.Equal(t, "my-project", got.Name)

		// Check exists
		assert.True(t, manager.ProjectExists("my-project"))
		assert.False(t, manager.ProjectExists("non-existent"))

		// Delete project
		err = manager.DeleteProject("my-project")
		require.NoError(t, err)

		assert.False(t, manager.ProjectExists("my-project"))
	})

	t.Run("validates project names", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir, "")
		require.NoError(t, err)
		defer manager.Close()

		// Valid names
		validNames := []string{"project", "my-project", "project_1", "Project123"}
		for _, name := range validNames {
			_, err := manager.CreateProject(name, "")
			assert.NoError(t, err, "name %s should be valid", name)
		}

		// Invalid names
		invalidNames := []string{"", "-project", "_project", "project/bad", "project.bad"}
		for _, name := range invalidNames {
			_, err := manager.CreateProject(name, "")
			assert.Error(t, err, "name %s should be invalid", name)
		}
	})

	t.Run("prevents duplicate projects", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir, "")
		require.NoError(t, err)
		defer manager.Close()

		_, err = manager.CreateProject("duplicate", "")
		require.NoError(t, err)

		_, err = manager.CreateProject("duplicate", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("gets project database", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir, "")
		require.NoError(t, err)
		defer manager.Close()

		_, err = manager.CreateProject("db-test", "")
		require.NoError(t, err)

		db, err := manager.GetProjectDB("db-test")
		require.NoError(t, err)
		assert.NotNil(t, db)
		assert.Equal(t, "sqlite3", db.Type())
	})
}

func TestDatabasePool(t *testing.T) {
	t.Run("opens and closes databases", func(t *testing.T) {
		tmpDir := t.TempDir()
		pool := NewDatabasePool(5, time.Minute)

		// Create a project
		config := DefaultConfig("pool-test")
		projectPath := filepath.Join(tmpDir, "pool-test")
		os.MkdirAll(projectPath, 0755)
		config.Save(filepath.Join(projectPath, ConfigFileName))

		project, err := NewProject(projectPath)
		require.NoError(t, err)

		// Get database
		db, err := pool.Get(project)
		require.NoError(t, err)
		assert.True(t, db.IsOpen())

		// Check pool stats
		stats := pool.Stats()
		assert.Equal(t, 1, stats.OpenDatabases)
		assert.Equal(t, 1, stats.ActiveDatabases)

		// Release
		pool.Release("pool-test")
		stats = pool.Stats()
		assert.Equal(t, 1, stats.OpenDatabases)
		assert.Equal(t, 0, stats.ActiveDatabases)

		// Close all
		err = pool.CloseAll()
		require.NoError(t, err)

		stats = pool.Stats()
		assert.Equal(t, 0, stats.OpenDatabases)
	})

	t.Run("enforces max open limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		pool := NewDatabasePool(2, time.Minute)

		// Create 3 projects
		for i := 0; i < 3; i++ {
			name := filepath.Join(tmpDir, string(rune('a'+i)))
			config := DefaultConfig(string(rune('a' + i)))
			os.MkdirAll(name, 0755)
			config.Save(filepath.Join(name, ConfigFileName))
		}

		// Open first two
		for i := 0; i < 2; i++ {
			project, _ := NewProject(filepath.Join(tmpDir, string(rune('a'+i))))
			_, err := pool.Get(project)
			require.NoError(t, err)
		}

		// Third should fail (all in use)
		project, _ := NewProject(filepath.Join(tmpDir, "c"))
		_, err := pool.Get(project)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "maximum")

		// Release one
		pool.Release("a")

		// Now third should work (evicts oldest idle)
		_, err = pool.Get(project)
		assert.NoError(t, err)

		pool.CloseAll()
	})
}
