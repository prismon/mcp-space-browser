// Package auth provides OAuth/OIDC authentication for REST and MCP endpoints
package auth

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Logging  LoggingConfig  `yaml:"logging"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	Port    int    `yaml:"port"`
	BaseURL string `yaml:"base_url"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// AuthConfig holds OAuth/OIDC authentication settings
type AuthConfig struct {
	Enabled           bool                         `yaml:"enabled"`
	RequireAuth       bool                         `yaml:"require_auth"`
	Provider          string                       `yaml:"provider"`
	Issuer            string                       `yaml:"issuer"`
	Audience          string                       `yaml:"audience"`
	JWKSURL           string                       `yaml:"jwks_url"`
	ClientID          string                       `yaml:"client_id"`
	ClientSecret      string                       `yaml:"client_secret"`
	DCREndpoint       string                       `yaml:"dcr_endpoint"`
	CacheTTLMinutes   int                          `yaml:"cache_ttl_minutes"`
	ResourceMetadata  ResourceMetadataConfig       `yaml:"resource_metadata"`
}

// ResourceMetadataConfig holds RFC 9728 Protected Resource Metadata settings
type ResourceMetadataConfig struct {
	Resource       string   `yaml:"resource"`
	BearerMethods  []string `yaml:"bearer_methods"`
	SigningAlgs    []string `yaml:"signing_algs"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// LoadConfig loads configuration from YAML file with environment variable overrides
func LoadConfig(configPath string) (*Config, error) {
	// Set defaults
	config := &Config{
		Server: ServerConfig{
			Port:    3000,
			BaseURL: "http://localhost:3000",
		},
		Database: DatabaseConfig{
			Path: "disk.db",
		},
		Auth: AuthConfig{
			Enabled:         false,
			RequireAuth:     false,
			Provider:        "generic",
			CacheTTLMinutes: 5,
			ResourceMetadata: ResourceMetadataConfig{
				BearerMethods: []string{"header"},
				SigningAlgs:   []string{"RS256", "ES256"},
			},
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}

	// Load from YAML file if provided
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			// If file doesn't exist, just use defaults
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
		}
	}

	// Override with environment variables
	applyEnvOverrides(config)

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	// Set derived values
	if config.Auth.ResourceMetadata.Resource == "" {
		config.Auth.ResourceMetadata.Resource = config.Server.BaseURL
	}

	return config, nil
}

// applyEnvOverrides applies environment variable overrides to configuration
func applyEnvOverrides(config *Config) {
	// Server overrides
	if port := os.Getenv("SERVER_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &config.Server.Port)
	}
	if baseURL := os.Getenv("SERVER_BASE_URL"); baseURL != "" {
		config.Server.BaseURL = baseURL
	}

	// Database overrides
	if dbPath := os.Getenv("DATABASE_PATH"); dbPath != "" {
		config.Database.Path = dbPath
	}

	// Auth overrides
	if enabled := os.Getenv("AUTH_ENABLED"); enabled != "" {
		config.Auth.Enabled = strings.ToLower(enabled) == "true"
	}
	if requireAuth := os.Getenv("AUTH_REQUIRE_AUTH"); requireAuth != "" {
		config.Auth.RequireAuth = strings.ToLower(requireAuth) == "true"
	}
	if provider := os.Getenv("AUTH_PROVIDER"); provider != "" {
		config.Auth.Provider = provider
	}
	if issuer := os.Getenv("AUTH_ISSUER"); issuer != "" {
		config.Auth.Issuer = issuer
	}
	if audience := os.Getenv("AUTH_AUDIENCE"); audience != "" {
		config.Auth.Audience = audience
	}
	if jwksURL := os.Getenv("AUTH_JWKS_URL"); jwksURL != "" {
		config.Auth.JWKSURL = jwksURL
	}
	if clientID := os.Getenv("AUTH_CLIENT_ID"); clientID != "" {
		config.Auth.ClientID = clientID
	}
	if clientSecret := os.Getenv("AUTH_CLIENT_SECRET"); clientSecret != "" {
		config.Auth.ClientSecret = clientSecret
	}
	if dcrEndpoint := os.Getenv("AUTH_DCR_ENDPOINT"); dcrEndpoint != "" {
		config.Auth.DCREndpoint = dcrEndpoint
	}

	// Logging overrides
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		config.Logging.Level = level
	}
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	if config.Auth.Enabled {
		if config.Auth.Issuer == "" {
			return fmt.Errorf("auth.issuer is required when auth.enabled is true")
		}
		if config.Auth.Audience == "" {
			return fmt.Errorf("auth.audience is required when auth.enabled is true")
		}
	}

	return nil
}

// GetCacheTTL returns the token cache TTL as a duration
func (a *AuthConfig) GetCacheTTL() time.Duration {
	return time.Duration(a.CacheTTLMinutes) * time.Minute
}
