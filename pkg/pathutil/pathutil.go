package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands tilde (~) to home directory and converts to absolute path
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Handle tilde expansion
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}

		if path == "~" {
			path = homeDir
		} else {
			path = filepath.Join(homeDir, path[2:])
		}
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return absPath, nil
}

// ValidatePath checks if a path exists on the filesystem
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("cannot access path: %w", err)
	}

	return nil
}

// ExpandAndValidatePath expands tilde and validates that the path exists
func ExpandAndValidatePath(path string) (string, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return "", err
	}

	if err := ValidatePath(expanded); err != nil {
		return "", err
	}

	return expanded, nil
}
