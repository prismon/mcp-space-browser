package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Manager manages all active sources
type Manager struct {
	db            *sql.DB
	sources       map[int64]Source
	ruleExecutor  RuleExecutor
	mu            sync.RWMutex
	log           *logrus.Entry
}

// NewManager creates a new source manager
func NewManager(db *sql.DB, ruleExecutor RuleExecutor) *Manager {
	return &Manager{
		db:           db,
		sources:      make(map[int64]Source),
		ruleExecutor: ruleExecutor,
		log:          logrus.WithField("component", "source-manager"),
	}
}

// CreateSource creates a new source configuration in the database
func (m *Manager) CreateSource(ctx context.Context, config *SourceConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	result, err := m.db.ExecContext(ctx, `
		INSERT INTO sources (name, type, root_path, config_json, status, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, config.Name, config.Type, config.RootPath, config.ConfigJSON, config.Status, config.Enabled, now, now)

	if err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get source ID: %w", err)
	}

	config.ID = id
	config.CreatedAt = time.Unix(now, 0)
	config.UpdatedAt = time.Unix(now, 0)

	m.log.WithFields(logrus.Fields{
		"id":   id,
		"name": config.Name,
		"type": config.Type,
	}).Info("Created source")

	return nil
}

// GetSource retrieves a source configuration by ID
func (m *Manager) GetSource(ctx context.Context, id int64) (*SourceConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var config SourceConfig
	var createdAt, updatedAt int64

	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, type, root_path, config_json, status, enabled, created_at, updated_at, COALESCE(last_error, '')
		FROM sources WHERE id = ?
	`, id).Scan(
		&config.ID,
		&config.Name,
		&config.Type,
		&config.RootPath,
		&config.ConfigJSON,
		&config.Status,
		&config.Enabled,
		&createdAt,
		&updatedAt,
		&config.LastError,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	config.CreatedAt = time.Unix(createdAt, 0)
	config.UpdatedAt = time.Unix(updatedAt, 0)

	return &config, nil
}

// ListSources returns all source configurations
func (m *Manager) ListSources(ctx context.Context) ([]*SourceConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, name, type, root_path, config_json, status, enabled, created_at, updated_at, COALESCE(last_error, '')
		FROM sources ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}
	defer rows.Close()

	var sources []*SourceConfig
	for rows.Next() {
		var config SourceConfig
		var createdAt, updatedAt int64

		err := rows.Scan(
			&config.ID,
			&config.Name,
			&config.Type,
			&config.RootPath,
			&config.ConfigJSON,
			&config.Status,
			&config.Enabled,
			&createdAt,
			&updatedAt,
			&config.LastError,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan source: %w", err)
		}

		config.CreatedAt = time.Unix(createdAt, 0)
		config.UpdatedAt = time.Unix(updatedAt, 0)
		sources = append(sources, &config)
	}

	return sources, rows.Err()
}

// UpdateSource updates a source configuration
func (m *Manager) UpdateSource(ctx context.Context, config *SourceConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	_, err := m.db.ExecContext(ctx, `
		UPDATE sources
		SET name = ?, type = ?, root_path = ?, config_json = ?, status = ?, enabled = ?, updated_at = ?, last_error = ?
		WHERE id = ?
	`, config.Name, config.Type, config.RootPath, config.ConfigJSON, config.Status, config.Enabled, now, config.LastError, config.ID)

	if err != nil {
		return fmt.Errorf("failed to update source: %w", err)
	}

	config.UpdatedAt = time.Unix(now, 0)

	m.log.WithField("id", config.ID).Info("Updated source")
	return nil
}

// DeleteSource deletes a source configuration
func (m *Manager) DeleteSource(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop the source if it's running
	if source, exists := m.sources[id]; exists {
		if err := source.Stop(ctx); err != nil {
			m.log.WithError(err).Warn("Failed to stop source before deletion")
		}
		delete(m.sources, id)
	}

	_, err := m.db.ExecContext(ctx, "DELETE FROM sources WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}

	m.log.WithField("id", id).Info("Deleted source")
	return nil
}

// StartSource starts a source by ID
func (m *Manager) StartSource(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if _, exists := m.sources[id]; exists {
		return fmt.Errorf("source %d is already running", id)
	}

	// Get source config
	config, err := m.getSourceUnlocked(ctx, id)
	if err != nil {
		return err
	}

	if !config.Enabled {
		return fmt.Errorf("source %d is disabled", id)
	}

	// Create the appropriate source implementation
	var source Source
	switch config.Type {
	case SourceTypeLive:
		source, err = NewLiveFilesystemSource(config, m.db)
		if err != nil {
			return fmt.Errorf("failed to create live source: %w", err)
		}
	default:
		return fmt.Errorf("unsupported source type: %s", config.Type)
	}

	// Set rule executor
	if m.ruleExecutor != nil {
		source.SetRuleExecutor(m.ruleExecutor)
	}

	// Start the source
	if err := source.Start(ctx); err != nil {
		return fmt.Errorf("failed to start source: %w", err)
	}

	m.sources[id] = source

	// Update status in database
	config.Status = SourceStatusRunning
	if err := m.updateSourceUnlocked(ctx, config); err != nil {
		m.log.WithError(err).Warn("Failed to update source status")
	}

	m.log.WithFields(logrus.Fields{
		"id":   id,
		"type": config.Type,
		"name": config.Name,
	}).Info("Started source")

	return nil
}

// StopSource stops a running source
func (m *Manager) StopSource(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	source, exists := m.sources[id]
	if !exists {
		return fmt.Errorf("source %d is not running", id)
	}

	if err := source.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop source: %w", err)
	}

	delete(m.sources, id)

	// Update status in database
	config := source.Config()
	config.Status = SourceStatusStopped
	if err := m.updateSourceUnlocked(ctx, config); err != nil {
		m.log.WithError(err).Warn("Failed to update source status")
	}

	m.log.WithField("id", id).Info("Stopped source")
	return nil
}

// GetSourceStats returns statistics for a running source
func (m *Manager) GetSourceStats(id int64) (*SourceStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	source, exists := m.sources[id]
	if !exists {
		return nil, fmt.Errorf("source %d is not running", id)
	}

	return source.Stats(), nil
}

// StopAll stops all running sources
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for id, source := range m.sources {
		if err := source.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("source %d: %w", id, err))
		}
	}

	m.sources = make(map[int64]Source)

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping sources: %v", errs)
	}

	return nil
}

// RestoreActiveSources restarts all enabled sources on manager startup
func (m *Manager) RestoreActiveSources(ctx context.Context) error {
	sources, err := m.ListSources(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sources: %w", err)
	}

	for _, config := range sources {
		if config.Enabled && config.Type == SourceTypeLive {
			if err := m.StartSource(ctx, config.ID); err != nil {
				m.log.WithError(err).WithField("id", config.ID).Error("Failed to restore source")
			}
		}
	}

	return nil
}

// Helper methods that assume lock is already held

func (m *Manager) getSourceUnlocked(ctx context.Context, id int64) (*SourceConfig, error) {
	var config SourceConfig
	var createdAt, updatedAt int64

	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, type, root_path, config_json, status, enabled, created_at, updated_at, COALESCE(last_error, '')
		FROM sources WHERE id = ?
	`, id).Scan(
		&config.ID,
		&config.Name,
		&config.Type,
		&config.RootPath,
		&config.ConfigJSON,
		&config.Status,
		&config.Enabled,
		&createdAt,
		&updatedAt,
		&config.LastError,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	config.CreatedAt = time.Unix(createdAt, 0)
	config.UpdatedAt = time.Unix(updatedAt, 0)

	return &config, nil
}

func (m *Manager) updateSourceUnlocked(ctx context.Context, config *SourceConfig) error {
	now := time.Now().Unix()
	_, err := m.db.ExecContext(ctx, `
		UPDATE sources
		SET status = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`, config.Status, config.LastError, now, config.ID)

	return err
}

// LiveFilesystemConfig holds configuration specific to live filesystem sources
type LiveFilesystemConfig struct {
	WatchRecursive bool     `json:"watch_recursive"`    // Watch subdirectories
	IgnorePatterns []string `json:"ignore_patterns"`    // Glob patterns to ignore
	DebounceMs     int      `json:"debounce_ms"`       // Debounce delay in milliseconds
	BatchSize      int      `json:"batch_size"`        // Max events to batch together
}

// MarshalLiveConfig serializes a LiveFilesystemConfig to JSON
func MarshalLiveConfig(config *LiveFilesystemConfig) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}
	return string(data), nil
}

// UnmarshalLiveConfig deserializes a LiveFilesystemConfig from JSON
func UnmarshalLiveConfig(configJSON string) (*LiveFilesystemConfig, error) {
	var config LiveFilesystemConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, nil
}
