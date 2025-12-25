package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/database"
)

// validProjectName defines valid project name characters
var validProjectName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Manager handles project lifecycle operations
type Manager struct {
	basePath  string // Path to projects directory (e.g., ~/.mcp-space-browser/projects/)
	cachePath string // Path to shared cache directory
	pool      *DatabasePool
	mu        sync.RWMutex
}

// NewManager creates a new project manager
func NewManager(basePath, cachePath string) (*Manager, error) {
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}

	absCachePath := cachePath
	if cachePath != "" {
		absCachePath, err = filepath.Abs(cachePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve cache path: %w", err)
		}
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(absBasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects directory: %w", err)
	}

	if absCachePath != "" {
		if err := os.MkdirAll(absCachePath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	pool := NewDatabasePool(DefaultMaxOpen, DefaultIdleTimeout)
	pool.StartIdleCleanup()

	return &Manager{
		basePath:  absBasePath,
		cachePath: absCachePath,
		pool:      pool,
	}, nil
}

// Close closes the manager and all open databases
func (m *Manager) Close() error {
	m.pool.StopIdleCleanup()
	return m.pool.CloseAll()
}

// CreateProject creates a new project with the given name and description
func (m *Manager) CreateProject(name, description string) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate name
	if !validProjectName.MatchString(name) {
		return nil, fmt.Errorf("invalid project name: must start with alphanumeric and contain only alphanumeric, underscore, or hyphen")
	}

	projectPath := filepath.Join(m.basePath, name)

	// Check if project already exists
	if _, err := os.Stat(projectPath); err == nil {
		return nil, fmt.Errorf("project already exists: %s", name)
	}

	// Create project configuration
	config := DefaultConfig(name)
	config.Description = description

	// Create project
	project := &Project{
		Name:        name,
		Description: description,
		Path:        projectPath,
		ConfigPath:  filepath.Join(projectPath, ConfigFileName),
		LogsPath:    filepath.Join(projectPath, LogsDir),
		Config:      config,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
	}

	// Initialize directory structure
	if err := project.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize project: %w", err)
	}

	log.WithField("name", name).Info("Project created")
	return project, nil
}

// DeleteProject deletes a project and all its data
func (m *Manager) DeleteProject(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	projectPath := filepath.Join(m.basePath, name)

	// Check if project exists
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return fmt.Errorf("project not found: %s", name)
	}

	// Close database if open
	if err := m.pool.Close(name); err != nil {
		log.WithError(err).WithField("name", name).Warn("Failed to close project database before deletion")
	}

	// Delete project directory
	if err := os.RemoveAll(projectPath); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	log.WithField("name", name).Info("Project deleted")
	return nil
}

// ListProjects returns all projects
func (m *Manager) ListProjects() ([]*Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	var projects []*Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(m.basePath, entry.Name())
		configPath := filepath.Join(projectPath, ConfigFileName)

		// Skip directories without project.yaml
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			continue
		}

		project, err := NewProject(projectPath)
		if err != nil {
			log.WithError(err).WithField("path", projectPath).Warn("Failed to load project")
			continue
		}

		projects = append(projects, project)
	}

	return projects, nil
}

// GetProject returns a project by name
func (m *Manager) GetProject(name string) (*Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	projectPath := filepath.Join(m.basePath, name)

	// Check if project exists
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("project not found: %s", name)
	}

	return NewProject(projectPath)
}

// GetProjectDB returns the database backend for a project
func (m *Manager) GetProjectDB(name string) (database.Backend, error) {
	project, err := m.GetProject(name)
	if err != nil {
		return nil, err
	}

	return m.pool.Get(project)
}

// ReleaseProjectDB releases a project's database back to the pool
func (m *Manager) ReleaseProjectDB(name string) {
	m.pool.Release(name)
}

// CloseProjectDB closes a project's database
func (m *Manager) CloseProjectDB(name string) error {
	return m.pool.Close(name)
}

// ProjectExists checks if a project exists
func (m *Manager) ProjectExists(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	projectPath := filepath.Join(m.basePath, name)
	configPath := filepath.Join(projectPath, ConfigFileName)

	_, err := os.Stat(configPath)
	return err == nil
}

// BasePath returns the projects base directory path
func (m *Manager) BasePath() string {
	return m.basePath
}

// CachePath returns the shared cache directory path
func (m *Manager) CachePath() string {
	return m.cachePath
}

// Pool returns the database pool
func (m *Manager) Pool() *DatabasePool {
	return m.pool
}

// UpdateProject updates project metadata
func (m *Manager) UpdateProject(name, description string) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	projectPath := filepath.Join(m.basePath, name)
	project, err := NewProject(projectPath)
	if err != nil {
		return nil, err
	}

	project.Description = description
	project.Config.UpdatedAt = time.Now()

	if err := project.SaveConfig(); err != nil {
		return nil, fmt.Errorf("failed to save project config: %w", err)
	}

	return project, nil
}
