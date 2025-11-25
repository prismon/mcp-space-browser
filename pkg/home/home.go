package home

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("home")
}

// Manager handles the application home directory
type Manager struct {
	path string
}

// Subdirectories within home
const (
	RulesDir         = "rules"
	RulesEnabledDir  = "rules/enabled"
	RulesDisabledDir = "rules/disabled"
	RulesExamplesDir = "rules/examples"
	CacheDir         = "cache"
	ThumbnailsDir    = "cache/thumbnails"
	TimelinesDir     = "cache/timelines"
	MetadataCacheDir = "cache/metadata"
	TempDir          = "temp"
	LogsDir          = "logs"
)

// Files within home
const (
	ConfigFile   = "config.yaml"
	DatabaseFile = "disk.db"
)

// NewManager creates a new home directory manager
func NewManager(path string) (*Manager, error) {
	log.Debug("Creating new home directory manager")

	if path == "" {
		log.Debug("No path specified, using default home path")
		path = DefaultHomePath()
	} else {
		log.WithField("path", path).Debug("Using specified home path")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		log.WithError(err).WithField("path", path).Error("Failed to resolve absolute path")
		return nil, fmt.Errorf("invalid home path: %w", err)
	}

	log.WithFields(logrus.Fields{
		"relativePath": path,
		"absolutePath": absPath,
	}).Info("Home directory manager created")

	return &Manager{path: absPath}, nil
}

// DefaultHomePath returns the default home directory path
func DefaultHomePath() string {
	log.Debug("Resolving default home directory path")

	// Check MCP_HOME environment variable
	if path := os.Getenv("MCP_HOME"); path != "" {
		log.WithFields(logrus.Fields{
			"source": "MCP_HOME",
			"path":   path,
		}).Info("Using home directory from MCP_HOME environment variable")
		return path
	}
	log.Debug("MCP_HOME environment variable not set")

	// Check MCP_SPACE_BROWSER_HOME environment variable
	if path := os.Getenv("MCP_SPACE_BROWSER_HOME"); path != "" {
		log.WithFields(logrus.Fields{
			"source": "MCP_SPACE_BROWSER_HOME",
			"path":   path,
		}).Info("Using home directory from MCP_SPACE_BROWSER_HOME environment variable")
		return path
	}
	log.Debug("MCP_SPACE_BROWSER_HOME environment variable not set")

	// Default to ~/.mcp-space-browser
	home, err := os.UserHomeDir()
	if err != nil {
		log.WithError(err).Warn("Failed to get user home directory, using relative path .mcp-space-browser")
		return ".mcp-space-browser"
	}

	log.WithFields(logrus.Fields{
		"userHomeDir": home,
		"HOME":        os.Getenv("HOME"),
	}).Debug("Got user home directory from OS")

	finalPath := filepath.Join(home, ".mcp-space-browser")
	log.WithFields(logrus.Fields{
		"source": "default",
		"path":   finalPath,
	}).Info("Using default home directory ~/.mcp-space-browser")

	return finalPath
}

// Path returns the home directory path
func (m *Manager) Path() string {
	return m.path
}

// Initialize creates the home directory structure
func (m *Manager) Initialize() error {
	log.WithField("path", m.path).Info("Initializing home directory structure")

	dirs := []string{
		"", // Home directory itself
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
		path := m.JoinPath(dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			log.WithError(err).WithField("directory", path).Error("Failed to create directory")
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
		if dir != "" {
			log.WithField("directory", path).Debug("Created directory")
		}
	}

	// Create default config if it doesn't exist
	log.WithField("configPath", m.ConfigPath()).Debug("Initializing configuration file")
	if err := m.initializeConfig(); err != nil {
		log.WithError(err).Error("Failed to initialize config")
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Create example rules if they don't exist
	log.WithField("examplesPath", m.RulesExamplesPath()).Debug("Initializing example rules")
	if err := m.initializeExamples(); err != nil {
		log.WithError(err).Error("Failed to initialize examples")
		return fmt.Errorf("failed to initialize examples: %w", err)
	}

	// Create .gitignore
	gitignorePath := m.JoinPath(".gitignore")
	log.WithField("gitignorePath", gitignorePath).Debug("Creating .gitignore")
	if err := m.createGitignore(); err != nil {
		log.WithError(err).Error("Failed to create .gitignore")
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	log.WithFields(logrus.Fields{
		"path":         m.path,
		"configPath":   m.ConfigPath(),
		"databasePath": m.DatabasePath(),
	}).Info("Home directory initialization complete")

	return nil
}

// Exists checks if the home directory exists
func (m *Manager) Exists() bool {
	info, err := os.Stat(m.path)
	return err == nil && info.IsDir()
}

// JoinPath joins path elements relative to home directory
func (m *Manager) JoinPath(elem ...string) string {
	parts := append([]string{m.path}, elem...)
	return filepath.Join(parts...)
}

// ConfigPath returns the path to config.yaml
func (m *Manager) ConfigPath() string {
	return m.JoinPath(ConfigFile)
}

// DatabasePath returns the path to disk.db
func (m *Manager) DatabasePath() string {
	return m.JoinPath(DatabaseFile)
}

// RulesEnabledPath returns the path to enabled rules directory
func (m *Manager) RulesEnabledPath() string {
	return m.JoinPath(RulesEnabledDir)
}

// RulesDisabledPath returns the path to disabled rules directory
func (m *Manager) RulesDisabledPath() string {
	return m.JoinPath(RulesDisabledDir)
}

// RulesExamplesPath returns the path to examples directory
func (m *Manager) RulesExamplesPath() string {
	return m.JoinPath(RulesExamplesDir)
}

// ThumbnailsPath returns the path to thumbnails cache
func (m *Manager) ThumbnailsPath() string {
	return m.JoinPath(ThumbnailsDir)
}

// TimelinesPath returns the path to timelines cache
func (m *Manager) TimelinesPath() string {
	return m.JoinPath(TimelinesDir)
}

// MetadataCachePath returns the path to metadata cache
func (m *Manager) MetadataCachePath() string {
	return m.JoinPath(MetadataCacheDir)
}

// TempPath returns the path to temp directory
func (m *Manager) TempPath() string {
	return m.JoinPath(TempDir)
}

// LogsPath returns the path to logs directory
func (m *Manager) LogsPath() string {
	return m.JoinPath(LogsDir)
}

// GetCachePath returns a content-addressed path for a cache file
// Uses first 2 chars for first directory, next 2 for second directory
func (m *Manager) GetCachePath(cacheType, hash, extension string) string {
	if len(hash) < 4 {
		// Fallback for short hashes
		return m.JoinPath(CacheDir, cacheType, hash+extension)
	}

	dir1 := hash[0:2]
	dir2 := hash[2:4]
	filename := hash + extension

	return m.JoinPath(CacheDir, cacheType, dir1, dir2, filename)
}

// EnsureCacheDir creates cache subdirectories for a hash
func (m *Manager) EnsureCacheDir(cacheType, hash string) error {
	if len(hash) < 4 {
		return os.MkdirAll(m.JoinPath(CacheDir, cacheType), 0755)
	}

	dir1 := hash[0:2]
	dir2 := hash[2:4]
	path := m.JoinPath(CacheDir, cacheType, dir1, dir2)

	return os.MkdirAll(path, 0755)
}

// CleanTemp removes all files in temp directory
func (m *Manager) CleanTemp() error {
	tempPath := m.TempPath()

	// Remove and recreate temp directory
	if err := os.RemoveAll(tempPath); err != nil {
		return err
	}
	return os.MkdirAll(tempPath, 0755)
}

// GetCacheSize returns the total size of the cache directory in bytes
func (m *Manager) GetCacheSize() (int64, error) {
	var size int64
	cachePath := m.JoinPath(CacheDir)

	err := filepath.Walk(cachePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// initializeConfig creates a default config.yaml if it doesn't exist
func (m *Manager) initializeConfig() error {
	configPath := m.ConfigPath()

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		log.WithField("configPath", configPath).Debug("Configuration file already exists, skipping creation")
		return nil // Config exists, don't overwrite
	}

	log.WithField("configPath", configPath).Info("Creating default configuration file")

	defaultConfig := `# mcp-space-browser configuration

# Database settings
database:
  path: disk.db  # Relative to home directory

# Rules settings
rules:
  autoExecute: true      # Auto-execute rules during indexing
  hotReload: true        # Watch for rule file changes
  maxConcurrent: 4       # Max concurrent rule executions

# Cache settings
cache:
  enabled: true
  maxSize: 10737418240   # 10 GB in bytes
  thumbnails:
    maxWidth: 320
    maxHeight: 320
    quality: 85
  timelines:
    frameCount: 10
    maxWidth: 160
    maxHeight: 120

# Logging settings
logging:
  level: info            # trace, debug, info, warn, error
  file: logs/mcp-space-browser.log
  maxSize: 104857600     # 100 MB
  maxBackups: 3

# Server settings
server:
  port: 3000
  host: localhost
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		log.WithError(err).WithField("configPath", configPath).Error("Failed to write configuration file")
		return err
	}

	log.WithField("configPath", configPath).Info("Default configuration file created successfully")
	return nil
}

// initializeExamples creates example rule files
func (m *Manager) initializeExamples() error {
	examplesPath := m.RulesExamplesPath()

	examples := map[string]string{
		"README.md": `# Example Rules

This directory contains example rule files for mcp-space-browser.

## Usage

1. Copy an example rule to the enabled/ directory:
   cp examples/auto-thumbnail.yaml enabled/

2. Edit the rule to match your needs

3. The rule will be automatically loaded (if hotReload is enabled)

## Rule Structure

Each rule consists of:
- **name**: Unique identifier
- **description**: What the rule does
- **enabled**: Whether the rule is active
- **priority**: Execution order (higher = first)
- **condition**: When to trigger (if)
- **outcome**: What to do (then)

## Important: Resource Set Association

**ALL rule outcomes MUST include a resourceSetName field.**

This ensures traceability and accountability:
- Every action taken by a rule is tracked
- All processed files are associated with a named resource set
- You can review what each rule has done
- Resource sets can be queried, exported, or further processed

Example outcome:
  type: classifier
  resourceSetName: my-thumbnails  # REQUIRED
  classifierOperation: generate_thumbnail

If the resource set doesn't exist, it will be auto-created.

See individual example files for more details.
`,
		"auto-thumbnail.yaml": `name: auto-thumbnail-large-images
description: Generate thumbnails for images larger than 1MB
enabled: true
priority: 10

condition:
  type: all
  conditions:
    - type: media_type
      mediaType: image
    - type: size
      minSize: 1048576  # 1MB in bytes

outcome:
  type: classifier
  resourceSetName: large-images-thumbnails
  classifierOperation: generate_thumbnail
  maxWidth: 320
  maxHeight: 320
`,
		"collect-videos.yaml": `name: collect-videos
description: Add all video files to a resource set
enabled: true
priority: 5

condition:
  type: media_type
  mediaType: video

outcome:
  type: resource_set
  resourceSetName: all-videos
  operation: add
`,
		"nested-conditions.yaml": `name: process-old-large-media
description: Example of nested conditions with multiple outcomes
enabled: false
priority: 1

condition:
  type: all
  conditions:
    # Must match at least one media type
    - type: any
      conditions:
        - type: media_type
          mediaType: image
        - type: media_type
          mediaType: video
    # Must be larger than 10MB
    - type: size
      minSize: 10485760
    # Must be older than 2020-01-01
    - type: time
      maxMtime: 1577836800
    # Must NOT be in cache or thumbnails directory
    - type: none
      conditions:
        - type: path
          pathContains: /cache/
        - type: path
          pathContains: /thumbnails/

outcome:
  type: chained
  resourceSetName: old-large-media
  stopOnError: false
  outcomes:
    - type: classifier
      resourceSetName: old-large-media-thumbnails
      classifierOperation: generate_thumbnail
    - type: resource_set
      resourceSetName: old-large-media
      operation: add
`,
	}

	for filename, content := range examples {
		path := filepath.Join(examplesPath, filename)

		// Don't overwrite existing examples
		if _, err := os.Stat(path); err == nil {
			continue
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

// createGitignore creates a .gitignore file for the home directory
func (m *Manager) createGitignore() error {
	gitignorePath := m.JoinPath(".gitignore")

	// Don't overwrite existing .gitignore
	if _, err := os.Stat(gitignorePath); err == nil {
		return nil
	}

	content := `# Database files
*.db
*.db-shm
*.db-wal

# Cache directory
cache/

# Temporary files
temp/

# Log files
logs/
*.log

# OS files
.DS_Store
Thumbs.db

# Keep rules structure but ignore specific rules if needed
# rules/enabled/my-private-rule.yaml
`

	return os.WriteFile(gitignorePath, []byte(content), 0644)
}
