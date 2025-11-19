package pathutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err, "Failed to get home directory")

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		wantContain string // Check if result contains this string
	}{
		{
			name:        "expand tilde alone",
			input:       "~",
			wantErr:     false,
			wantContain: homeDir,
		},
		{
			name:        "expand tilde with path",
			input:       "~/Documents",
			wantErr:     false,
			wantContain: filepath.Join(homeDir, "Documents"),
		},
		{
			name:    "empty path",
			input:   "",
			wantErr: true,
		},
		{
			name:    "absolute path unchanged",
			input:   "/usr/local",
			wantErr: false,
		},
		{
			name:    "relative path converted",
			input:   "./test",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandPath(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantContain != "" {
				assert.Equal(t, tt.wantContain, result)
			}

			// All expanded paths should be absolute
			if !tt.wantErr && tt.input != "" {
				assert.True(t, filepath.IsAbs(result), "Result should be absolute path")
			}
		})
	}
}

func TestExpandPathWithTilde(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	// Test tilde expansion
	result, err := ExpandPath("~/test")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(result, homeDir))
	assert.True(t, strings.HasSuffix(result, "test"))
}

func TestValidatePath(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile")
	err := os.WriteFile(tmpFile, []byte("test"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid directory",
			path:    tmpDir,
			wantErr: false,
		},
		{
			name:    "valid file",
			path:    tmpFile,
			wantErr: false,
		},
		{
			name:    "non-existent path",
			path:    "/this/path/definitely/does/not/exist/12345",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExpandAndValidatePath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    tmpDir,
			wantErr: false,
		},
		{
			name:    "non-existent path",
			path:    "/this/does/not/exist",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandAndValidatePath(tt.path)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.True(t, filepath.IsAbs(result))
			}
		})
	}
}
