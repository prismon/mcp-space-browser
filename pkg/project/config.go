package project

import (
	"fmt"
	"os"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"gopkg.in/yaml.v3"
)

// ConfigFileName is the name of the project configuration file
const ConfigFileName = "project.yaml"

// Config is the per-project configuration stored in project.yaml
type Config struct {
	// Name is the unique project name
	Name string `yaml:"name" json:"name"`

	// Description is an optional human-readable description
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// CreatedAt is when the project was created
	CreatedAt time.Time `yaml:"created_at" json:"createdAt"`

	// UpdatedAt is when the project was last modified
	UpdatedAt time.Time `yaml:"updated_at" json:"updatedAt"`

	// Database contains database backend configuration
	Database database.BackendConfig `yaml:"database" json:"database"`
}

// DefaultConfig returns the default project configuration with SQLite backend
func DefaultConfig(name string) *Config {
	now := time.Now()
	return &Config{
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Database:  *database.DefaultBackendConfig(),
	}
}

// Load reads a project configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults for missing fields
	if config.Database.Type == "" {
		config.Database.Type = "sqlite3"
	}
	if config.Database.SQLite == nil && config.Database.Type == "sqlite3" {
		config.Database.SQLite = database.DefaultSQLiteConfig()
	}

	return &config, nil
}

// Save writes the project configuration to a file
func (c *Config) Save(path string) error {
	c.UpdatedAt = time.Now()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := "# Project configuration for mcp-space-browser\n# Generated automatically - edit with care\n\n"
	fullData := append([]byte(header), data...)

	if err := os.WriteFile(path, fullData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("project name is required")
	}

	// Validate database configuration
	switch c.Database.Type {
	case "sqlite3", "":
		if c.Database.SQLite == nil {
			return fmt.Errorf("sqlite configuration is required for sqlite3 backend")
		}
		if c.Database.SQLite.Path == "" {
			return fmt.Errorf("sqlite path is required")
		}
	case "postgresql":
		if c.Database.Postgres == nil {
			return fmt.Errorf("postgresql configuration is required for postgresql backend")
		}
	default:
		return fmt.Errorf("unknown database backend: %s", c.Database.Type)
	}

	return nil
}

// Clone creates a deep copy of the configuration
func (c *Config) Clone() *Config {
	clone := &Config{
		Name:        c.Name,
		Description: c.Description,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
		Database: database.BackendConfig{
			Type: c.Database.Type,
		},
	}

	if c.Database.SQLite != nil {
		clone.Database.SQLite = &database.SQLiteConfig{
			Path:          c.Database.SQLite.Path,
			WALMode:       c.Database.SQLite.WALMode,
			BusyTimeoutMs: c.Database.SQLite.BusyTimeoutMs,
		}
	}

	if c.Database.Postgres != nil {
		clone.Database.Postgres = &database.PostgresConfig{
			Host:        c.Database.Postgres.Host,
			Port:        c.Database.Postgres.Port,
			Database:    c.Database.Postgres.Database,
			User:        c.Database.Postgres.User,
			PasswordEnv: c.Database.Postgres.PasswordEnv,
			SSLMode:     c.Database.Postgres.SSLMode,
		}
	}

	return clone
}
