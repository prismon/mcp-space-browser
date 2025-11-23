package home

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Rules    RulesConfig    `yaml:"rules"`
	Cache    CacheConfig    `yaml:"cache"`
	Logging  LoggingConfig  `yaml:"logging"`
	Server   ServerConfig   `yaml:"server"`
}

// DatabaseConfig contains database settings
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// RulesConfig contains rules engine settings
type RulesConfig struct {
	AutoExecute   bool `yaml:"autoExecute"`
	HotReload     bool `yaml:"hotReload"`
	MaxConcurrent int  `yaml:"maxConcurrent"`
}

// CacheConfig contains cache settings
type CacheConfig struct {
	Enabled    bool             `yaml:"enabled"`
	MaxSize    int64            `yaml:"maxSize"`
	Thumbnails ThumbnailsConfig `yaml:"thumbnails"`
	Timelines  TimelinesConfig  `yaml:"timelines"`
}

// ThumbnailsConfig contains thumbnail generation settings
type ThumbnailsConfig struct {
	MaxWidth  int `yaml:"maxWidth"`
	MaxHeight int `yaml:"maxHeight"`
	Quality   int `yaml:"quality"`
}

// TimelinesConfig contains video timeline generation settings
type TimelinesConfig struct {
	FrameCount int `yaml:"frameCount"`
	MaxWidth   int `yaml:"maxWidth"`
	MaxHeight  int `yaml:"maxHeight"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSize    int64  `yaml:"maxSize"`
	MaxBackups int    `yaml:"maxBackups"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// LoadConfig loads configuration from config.yaml
func (m *Manager) LoadConfig() (*Config, error) {
	configPath := m.ConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// SaveConfig saves configuration to config.yaml
func (m *Manager) SaveConfig(config *Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := m.ConfigPath()
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Database: DatabaseConfig{
			Path: DatabaseFile,
		},
		Rules: RulesConfig{
			AutoExecute:   true,
			HotReload:     true,
			MaxConcurrent: 4,
		},
		Cache: CacheConfig{
			Enabled: true,
			MaxSize: 10737418240, // 10 GB
			Thumbnails: ThumbnailsConfig{
				MaxWidth:  320,
				MaxHeight: 320,
				Quality:   85,
			},
			Timelines: TimelinesConfig{
				FrameCount: 10,
				MaxWidth:   160,
				MaxHeight:  120,
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "logs/mcp-space-browser.log",
			MaxSize:    104857600, // 100 MB
			MaxBackups: 3,
		},
		Server: ServerConfig{
			Port: 3000,
			Host: "localhost",
		},
	}
}
