package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("project")
}

// LogsDir is the name of the project logs directory
const LogsDir = "logs"

// Project represents a named project with its own database and configuration
type Project struct {
	// Name is the unique project identifier
	Name string `json:"name"`

	// Description is an optional human-readable description
	Description string `json:"description,omitempty"`

	// Path is the absolute path to the project directory
	Path string `json:"path"`

	// ConfigPath is the absolute path to project.yaml
	ConfigPath string `json:"configPath"`

	// LogsPath is the absolute path to the logs directory
	LogsPath string `json:"logsPath"`

	// Config is the loaded project configuration (not serialized)
	Config *Config `json:"-"`

	// CreatedAt is when the project was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is when the project was last modified
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewProject creates a new Project instance from a project directory path
func NewProject(projectPath string) (*Project, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project path: %w", err)
	}

	configPath := filepath.Join(absPath, ConfigFileName)
	logsPath := filepath.Join(absPath, LogsDir)

	// Load configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	return &Project{
		Name:        config.Name,
		Description: config.Description,
		Path:        absPath,
		ConfigPath:  configPath,
		LogsPath:    logsPath,
		Config:      config,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
	}, nil
}

// DBPath returns the database path based on backend configuration
func (p *Project) DBPath() string {
	if p.Config == nil {
		return ""
	}

	switch p.Config.Database.Type {
	case "sqlite3", "":
		if p.Config.Database.SQLite != nil {
			dbPath := p.Config.Database.SQLite.Path
			if !filepath.IsAbs(dbPath) {
				return filepath.Join(p.Path, dbPath)
			}
			return dbPath
		}
		return filepath.Join(p.Path, "disk.db")
	default:
		// Non-file backends don't have a local path
		return ""
	}
}

// Exists checks if the project directory and configuration exist
func (p *Project) Exists() bool {
	_, err := os.Stat(p.ConfigPath)
	return err == nil
}

// Initialize creates the project directory structure
func (p *Project) Initialize() error {
	log.WithField("path", p.Path).Info("Initializing project directory")

	// Create project directory
	if err := os.MkdirAll(p.Path, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create logs directory
	if err := os.MkdirAll(p.LogsPath, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Save configuration
	if err := p.Config.Save(p.ConfigPath); err != nil {
		return fmt.Errorf("failed to save project config: %w", err)
	}

	return nil
}

// Delete removes the project directory and all its contents
func (p *Project) Delete() error {
	log.WithField("path", p.Path).Warn("Deleting project directory")
	return os.RemoveAll(p.Path)
}

// Reload reloads the project configuration from disk
func (p *Project) Reload() error {
	config, err := LoadConfig(p.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to reload project config: %w", err)
	}

	p.Config = config
	p.Description = config.Description
	p.UpdatedAt = config.UpdatedAt

	return nil
}

// SaveConfig saves the current configuration to disk
func (p *Project) SaveConfig() error {
	if p.Config == nil {
		return fmt.Errorf("project has no configuration")
	}

	p.Config.Name = p.Name
	p.Config.Description = p.Description

	if err := p.Config.Save(p.ConfigPath); err != nil {
		return err
	}

	p.UpdatedAt = p.Config.UpdatedAt
	return nil
}

// ToJSON returns a JSON representation of the project
func (p *Project) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// Stats returns basic statistics about the project
type ProjectStats struct {
	Name           string    `json:"name"`
	DatabasePath   string    `json:"databasePath,omitempty"`
	DatabaseSizeKB int64     `json:"databaseSizeKB,omitempty"`
	BackendType    string    `json:"backendType"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Stats returns statistics about the project
func (p *Project) Stats() (*ProjectStats, error) {
	stats := &ProjectStats{
		Name:      p.Name,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}

	if p.Config != nil {
		stats.BackendType = p.Config.Database.Type
		if stats.BackendType == "" {
			stats.BackendType = "sqlite3"
		}
	}

	dbPath := p.DBPath()
	if dbPath != "" {
		stats.DatabasePath = dbPath
		if info, err := os.Stat(dbPath); err == nil {
			stats.DatabaseSizeKB = info.Size() / 1024
		}
	}

	return stats, nil
}
