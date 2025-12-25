package database

import (
	"database/sql"
)

// Backend defines the interface all database backends must implement.
// This allows the system to support different database backends (SQLite, PostgreSQL, etc.)
// while maintaining a consistent API for the rest of the application.
type Backend interface {
	// Connection management
	Open() error
	Close() error
	IsOpen() bool

	// Query execution - returns standard sql.DB for compatibility with existing code
	DB() *sql.DB

	// Schema management
	InitSchema() error

	// Backend info
	Type() string           // "sqlite3", "postgresql", etc.
	ConnectionInfo() string // Human-readable connection info for logging/debugging
}

// BackendConfig is the configuration for a database backend
type BackendConfig struct {
	Type     string          `yaml:"backend" json:"backend"`
	SQLite   *SQLiteConfig   `yaml:"sqlite,omitempty" json:"sqlite,omitempty"`
	Postgres *PostgresConfig `yaml:"postgresql,omitempty" json:"postgresql,omitempty"`
}

// SQLiteConfig contains SQLite-specific configuration
type SQLiteConfig struct {
	Path          string `yaml:"path" json:"path"`                       // Path to database file (relative to project dir)
	WALMode       bool   `yaml:"wal_mode" json:"wal_mode"`               // Enable WAL mode (default: true)
	BusyTimeoutMs int    `yaml:"busy_timeout_ms" json:"busy_timeout_ms"` // Busy timeout in milliseconds (default: 5000)
}

// PostgresConfig contains PostgreSQL-specific configuration (for future use)
type PostgresConfig struct {
	Host        string `yaml:"host" json:"host"`
	Port        int    `yaml:"port" json:"port"`
	Database    string `yaml:"database" json:"database"`
	User        string `yaml:"user" json:"user"`
	PasswordEnv string `yaml:"password_env" json:"password_env"` // Environment variable containing password
	SSLMode     string `yaml:"ssl_mode" json:"ssl_mode"`
}

// DefaultSQLiteConfig returns the default SQLite configuration
func DefaultSQLiteConfig() *SQLiteConfig {
	return &SQLiteConfig{
		Path:          "disk.db",
		WALMode:       true,
		BusyTimeoutMs: 5000,
	}
}

// DefaultBackendConfig returns the default backend configuration (SQLite)
func DefaultBackendConfig() *BackendConfig {
	return &BackendConfig{
		Type:   "sqlite3",
		SQLite: DefaultSQLiteConfig(),
	}
}

// NewBackend creates a backend based on configuration
func NewBackend(projectPath string, config *BackendConfig) (Backend, error) {
	if config == nil {
		config = DefaultBackendConfig()
	}

	switch config.Type {
	case "sqlite3", "":
		// Default to SQLite
		sqliteConfig := config.SQLite
		if sqliteConfig == nil {
			sqliteConfig = DefaultSQLiteConfig()
		}
		return NewSQLiteBackend(projectPath, sqliteConfig), nil
	case "postgresql":
		return nil, &BackendNotImplementedError{BackendType: "postgresql"}
	default:
		return nil, &UnknownBackendError{BackendType: config.Type}
	}
}

// BackendNotImplementedError is returned when a backend type is not yet implemented
type BackendNotImplementedError struct {
	BackendType string
}

func (e *BackendNotImplementedError) Error() string {
	return "database backend not yet implemented: " + e.BackendType
}

// UnknownBackendError is returned when an unknown backend type is specified
type UnknownBackendError struct {
	BackendType string
}

func (e *UnknownBackendError) Error() string {
	return "unknown database backend type: " + e.BackendType
}
