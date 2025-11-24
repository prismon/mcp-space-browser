package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_DefaultValues(t *testing.T) {
	config, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Test default server config
	if config.Server.Port != 3000 {
		t.Errorf("Expected default port 3000, got %d", config.Server.Port)
	}
	if config.Server.Host != "127.0.0.1" {
		t.Errorf("Expected default host 127.0.0.1, got %s", config.Server.Host)
	}
	if config.Server.BaseURL != "http://localhost:3000" {
		t.Errorf("Expected default baseURL http://localhost:3000, got %s", config.Server.BaseURL)
	}

	// Test default database config
	if config.Database.Path != "disk.db" {
		t.Errorf("Expected default database path disk.db, got %s", config.Database.Path)
	}

	// Test default cache config
	if config.Cache.Dir != "./cache/artifacts" {
		t.Errorf("Expected default cache dir ./cache/artifacts, got %s", config.Cache.Dir)
	}

	// Test default auth config
	if config.Auth.Enabled {
		t.Error("Expected auth to be disabled by default")
	}
	if config.Auth.RequireAuth {
		t.Error("Expected requireAuth to be false by default")
	}
	if config.Auth.Provider != "generic" {
		t.Errorf("Expected default provider generic, got %s", config.Auth.Provider)
	}
	if config.Auth.CacheTTLMinutes != 5 {
		t.Errorf("Expected default cache TTL 5 minutes, got %d", config.Auth.CacheTTLMinutes)
	}

	// Test default logging config
	if config.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", config.Logging.Level)
	}
}

func TestLoadConfig_FromYAMLFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  port: 8080
  host: "0.0.0.0"
  external_host: "example.com"
database:
  path: "/var/lib/disk.db"
cache:
  dir: "/tmp/cache"
auth:
  enabled: true
  require_auth: true
  provider: "keycloak"
  issuer: "https://auth.example.com"
  audience: "my-api"
  jwks_url: "https://auth.example.com/jwks"
  client_id: "test-client"
  client_secret: "test-secret"
  dcr_endpoint: "https://auth.example.com/register"
  cache_ttl_minutes: 10
logging:
  level: "debug"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify all values were loaded correctly
	if config.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", config.Server.Port)
	}
	if config.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0, got %s", config.Server.Host)
	}
	if config.Server.ExternalHost != "example.com" {
		t.Errorf("Expected external_host example.com, got %s", config.Server.ExternalHost)
	}
	if config.Database.Path != "/var/lib/disk.db" {
		t.Errorf("Expected database path /var/lib/disk.db, got %s", config.Database.Path)
	}
	if config.Cache.Dir != "/tmp/cache" {
		t.Errorf("Expected cache dir /tmp/cache, got %s", config.Cache.Dir)
	}
	if !config.Auth.Enabled {
		t.Error("Expected auth to be enabled")
	}
	if !config.Auth.RequireAuth {
		t.Error("Expected requireAuth to be true")
	}
	if config.Auth.Provider != "keycloak" {
		t.Errorf("Expected provider keycloak, got %s", config.Auth.Provider)
	}
	if config.Auth.Issuer != "https://auth.example.com" {
		t.Errorf("Expected issuer https://auth.example.com, got %s", config.Auth.Issuer)
	}
	if config.Auth.Audience != "my-api" {
		t.Errorf("Expected audience my-api, got %s", config.Auth.Audience)
	}
	if config.Auth.JWKSURL != "https://auth.example.com/jwks" {
		t.Errorf("Expected jwks_url https://auth.example.com/jwks, got %s", config.Auth.JWKSURL)
	}
	if config.Auth.ClientID != "test-client" {
		t.Errorf("Expected client_id test-client, got %s", config.Auth.ClientID)
	}
	if config.Auth.ClientSecret != "test-secret" {
		t.Errorf("Expected client_secret test-secret, got %s", config.Auth.ClientSecret)
	}
	if config.Auth.DCREndpoint != "https://auth.example.com/register" {
		t.Errorf("Expected dcr_endpoint https://auth.example.com/register, got %s", config.Auth.DCREndpoint)
	}
	if config.Auth.CacheTTLMinutes != 10 {
		t.Errorf("Expected cache_ttl_minutes 10, got %d", config.Auth.CacheTTLMinutes)
	}
	if config.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", config.Logging.Level)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("SERVER_HOST", "192.168.1.1")
	os.Setenv("SERVER_EXTERNAL_HOST", "api.example.com")
	os.Setenv("SERVER_BASE_URL", "https://api.example.com")
	os.Setenv("DATABASE_PATH", "/custom/db.db")
	os.Setenv("CACHE_DIR", "/custom/cache")
	os.Setenv("AUTH_ENABLED", "true")
	os.Setenv("AUTH_REQUIRE_AUTH", "true")
	os.Setenv("AUTH_PROVIDER", "okta")
	os.Setenv("AUTH_ISSUER", "https://okta.example.com")
	os.Setenv("AUTH_AUDIENCE", "okta-audience")
	os.Setenv("AUTH_JWKS_URL", "https://okta.example.com/jwks")
	os.Setenv("AUTH_CLIENT_ID", "okta-client")
	os.Setenv("AUTH_CLIENT_SECRET", "okta-secret")
	os.Setenv("AUTH_DCR_ENDPOINT", "https://okta.example.com/register")
	os.Setenv("LOG_LEVEL", "trace")

	defer func() {
		// Clean up environment variables
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("SERVER_HOST")
		os.Unsetenv("SERVER_EXTERNAL_HOST")
		os.Unsetenv("SERVER_BASE_URL")
		os.Unsetenv("DATABASE_PATH")
		os.Unsetenv("CACHE_DIR")
		os.Unsetenv("AUTH_ENABLED")
		os.Unsetenv("AUTH_REQUIRE_AUTH")
		os.Unsetenv("AUTH_PROVIDER")
		os.Unsetenv("AUTH_ISSUER")
		os.Unsetenv("AUTH_AUDIENCE")
		os.Unsetenv("AUTH_JWKS_URL")
		os.Unsetenv("AUTH_CLIENT_ID")
		os.Unsetenv("AUTH_CLIENT_SECRET")
		os.Unsetenv("AUTH_DCR_ENDPOINT")
		os.Unsetenv("LOG_LEVEL")
	}()

	config, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify environment variables override defaults
	if config.Server.Port != 9090 {
		t.Errorf("Expected port 9090 from env, got %d", config.Server.Port)
	}
	if config.Server.Host != "192.168.1.1" {
		t.Errorf("Expected host 192.168.1.1 from env, got %s", config.Server.Host)
	}
	if config.Server.ExternalHost != "api.example.com" {
		t.Errorf("Expected external_host api.example.com from env, got %s", config.Server.ExternalHost)
	}
	if config.Server.BaseURL != "https://api.example.com" {
		t.Errorf("Expected baseURL https://api.example.com from env, got %s", config.Server.BaseURL)
	}
	if config.Database.Path != "/custom/db.db" {
		t.Errorf("Expected database path /custom/db.db from env, got %s", config.Database.Path)
	}
	if config.Cache.Dir != "/custom/cache" {
		t.Errorf("Expected cache dir /custom/cache from env, got %s", config.Cache.Dir)
	}
	if !config.Auth.Enabled {
		t.Error("Expected auth to be enabled from env")
	}
	if !config.Auth.RequireAuth {
		t.Error("Expected requireAuth to be true from env")
	}
	if config.Auth.Provider != "okta" {
		t.Errorf("Expected provider okta from env, got %s", config.Auth.Provider)
	}
	if config.Auth.Issuer != "https://okta.example.com" {
		t.Errorf("Expected issuer https://okta.example.com from env, got %s", config.Auth.Issuer)
	}
	if config.Auth.Audience != "okta-audience" {
		t.Errorf("Expected audience okta-audience from env, got %s", config.Auth.Audience)
	}
	if config.Logging.Level != "trace" {
		t.Errorf("Expected log level trace from env, got %s", config.Logging.Level)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_MissingIssuerWhenAuthEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with auth enabled but missing issuer
	yamlContent := `
auth:
  enabled: true
  audience: "test-audience"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for missing issuer when auth enabled, got nil")
	}
}

func TestLoadConfig_MissingAudienceWhenAuthEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with auth enabled but missing audience
	yamlContent := `
auth:
  enabled: true
  issuer: "https://auth.example.com"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for missing audience when auth enabled, got nil")
	}
}

func TestLoadConfig_NonexistentFile(t *testing.T) {
	// Should succeed with defaults when file doesn't exist
	config, err := LoadConfig("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Expected LoadConfig to succeed with defaults when file doesn't exist, got error: %v", err)
	}

	// Verify defaults
	if config.Server.Port != 3000 {
		t.Errorf("Expected default port 3000, got %d", config.Server.Port)
	}
}

func TestLoadConfig_ResourceMetadataDefaults(t *testing.T) {
	config, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Check resource metadata defaults
	if len(config.Auth.ResourceMetadata.BearerMethods) != 1 || config.Auth.ResourceMetadata.BearerMethods[0] != "header" {
		t.Errorf("Expected bearer_methods [header], got %v", config.Auth.ResourceMetadata.BearerMethods)
	}

	expectedAlgs := []string{"RS256", "ES256"}
	if len(config.Auth.ResourceMetadata.SigningAlgs) != len(expectedAlgs) {
		t.Errorf("Expected %d signing algorithms, got %d", len(expectedAlgs), len(config.Auth.ResourceMetadata.SigningAlgs))
	}
}

func TestLoadConfig_ResourceMetadataDerived(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  base_url: "https://custom.example.com"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Resource should be derived from base_url
	if config.Auth.ResourceMetadata.Resource != "https://custom.example.com" {
		t.Errorf("Expected resource https://custom.example.com, got %s", config.Auth.ResourceMetadata.Resource)
	}
}

func TestAuthConfig_GetCacheTTL(t *testing.T) {
	authConfig := &AuthConfig{
		CacheTTLMinutes: 15,
	}

	expectedTTL := 15 * time.Minute
	actualTTL := authConfig.GetCacheTTL()

	if actualTTL != expectedTTL {
		t.Errorf("Expected cache TTL %v, got %v", expectedTTL, actualTTL)
	}
}

func TestAuthConfig_GetCacheTTLZero(t *testing.T) {
	authConfig := &AuthConfig{
		CacheTTLMinutes: 0,
	}

	expectedTTL := 0 * time.Minute
	actualTTL := authConfig.GetCacheTTL()

	if actualTTL != expectedTTL {
		t.Errorf("Expected cache TTL %v, got %v", expectedTTL, actualTTL)
	}
}

func TestApplyEnvOverrides_BooleanValues(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"true lowercase", "true", true},
		{"True uppercase", "True", true},
		{"TRUE all caps", "TRUE", true},
		{"false", "false", false},
		{"empty string", "", false},
		{"invalid", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Auth: AuthConfig{
					Enabled: false,
				},
			}

			if tt.envValue != "" {
				os.Setenv("AUTH_ENABLED", tt.envValue)
				defer os.Unsetenv("AUTH_ENABLED")
			}

			applyEnvOverrides(config)

			if config.Auth.Enabled != tt.expected {
				t.Errorf("Expected AUTH_ENABLED=%v for value %q, got %v", tt.expected, tt.envValue, config.Auth.Enabled)
			}
		})
	}
}

func TestValidateConfig_AuthDisabled(t *testing.T) {
	config := &Config{
		Auth: AuthConfig{
			Enabled: false,
		},
	}

	err := validateConfig(config)
	if err != nil {
		t.Errorf("Expected no error when auth is disabled, got: %v", err)
	}
}

func TestValidateConfig_AuthEnabledValid(t *testing.T) {
	config := &Config{
		Auth: AuthConfig{
			Enabled:  true,
			Issuer:   "https://auth.example.com",
			Audience: "test-audience",
		},
	}

	err := validateConfig(config)
	if err != nil {
		t.Errorf("Expected no error for valid auth config, got: %v", err)
	}
}
