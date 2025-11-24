package sources

import (
	"context"
	"time"
)

// SourceType represents the type of indexing source
type SourceType string

const (
	SourceTypeManual     SourceType = "manual"
	SourceTypeLive       SourceType = "live"
	SourceTypeScheduled  SourceType = "scheduled"
)

// SourceStatus represents the current state of a source
type SourceStatus string

const (
	SourceStatusStopped  SourceStatus = "stopped"
	SourceStatusStarting SourceStatus = "starting"
	SourceStatusRunning  SourceStatus = "running"
	SourceStatusStopping SourceStatus = "stopping"
	SourceStatusError    SourceStatus = "error"
)

// SourceConfig holds configuration for a source
type SourceConfig struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Type        SourceType `json:"type"`
	RootPath    string     `json:"root_path"`
	ConfigJSON  string     `json:"config_json"`  // Type-specific config
	Status      SourceStatus `json:"status"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastError   string     `json:"last_error,omitempty"`
}

// SourceStats holds statistics about a source's activity
type SourceStats struct {
	FilesIndexed     int64     `json:"files_indexed"`
	DirsIndexed      int64     `json:"dirs_indexed"`
	BytesIndexed     int64     `json:"bytes_indexed"`
	RulesExecuted    int64     `json:"rules_executed"`
	LastUpdate       time.Time `json:"last_update"`
	ErrorCount       int64     `json:"error_count"`
	LastError        string    `json:"last_error,omitempty"`
}

// Source is the interface that all source implementations must satisfy
type Source interface {
	// Start begins the source's operation (indexing, watching, etc.)
	Start(ctx context.Context) error

	// Stop gracefully stops the source
	Stop(ctx context.Context) error

	// Config returns the source's configuration
	Config() *SourceConfig

	// Stats returns current statistics
	Stats() *SourceStats

	// Status returns the current status
	Status() SourceStatus

	// SetRuleExecutor sets the rule executor for this source
	SetRuleExecutor(executor RuleExecutor)
}

// RuleExecutor is an interface for executing rules on filesystem entries
// This will be implemented by the rules engine
type RuleExecutor interface {
	// ExecuteRulesForPath evaluates and executes all enabled rules for a given path
	ExecuteRulesForPath(ctx context.Context, path string) error
}

// EventType represents the type of filesystem event
type EventType string

const (
	EventTypeCreate EventType = "create"
	EventTypeModify EventType = "modify"
	EventTypeDelete EventType = "delete"
	EventTypeRename EventType = "rename"
)

// FilesystemEvent represents a change in the filesystem
type FilesystemEvent struct {
	Type     EventType
	Path     string
	OldPath  string  // For rename events
	Time     time.Time
}
