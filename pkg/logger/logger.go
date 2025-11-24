package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

var defaultLogger *logrus.Logger

func init() {
	defaultLogger = logrus.New()

	// Check if we're in test mode
	isTest := os.Getenv("GO_ENV") == "test"

	// Set log level from environment or default to info
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		if isTest {
			logLevel = "silent"
		} else {
			logLevel = "info"
		}
	}

	// Configure logger
	if logLevel == "silent" {
		defaultLogger.SetOutput(os.NewFile(0, os.DevNull))
	} else {
		level, err := logrus.ParseLevel(strings.ToLower(logLevel))
		if err != nil {
			level = logrus.InfoLevel
		}
		defaultLogger.SetLevel(level)
	}

	// Use TextFormatter for pretty output (similar to pino-pretty)
	defaultLogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     true,
	})

	defaultLogger.SetOutput(os.Stdout)
}

// GetLogger returns the default logger instance
func GetLogger() *logrus.Logger {
	return defaultLogger
}

// WithName creates a child logger with a name field
func WithName(name string) *logrus.Entry {
	return defaultLogger.WithField("name", name)
}

// WithFields creates a logger with additional fields
func WithFields(fields logrus.Fields) *logrus.Entry {
	return defaultLogger.WithFields(fields)
}

// SetLevel sets the logging level
func SetLevel(level logrus.Level) {
	defaultLogger.SetLevel(level)
}

// IsLevelEnabled checks if a log level is enabled
func IsLevelEnabled(level logrus.Level) bool {
	return defaultLogger.IsLevelEnabled(level)
}

// ConfigureFromString configures the logger from a string level
// This is useful for applying configuration from config files
func ConfigureFromString(levelStr string) error {
	// Check if we're in test mode - test mode takes precedence
	isTest := os.Getenv("GO_ENV") == "test"
	if isTest {
		defaultLogger.SetOutput(os.NewFile(0, os.DevNull))
		return nil
	}

	// Handle silent mode
	if levelStr == "silent" {
		defaultLogger.SetOutput(os.NewFile(0, os.DevNull))
		return nil
	}

	// Parse and set log level
	level, err := logrus.ParseLevel(strings.ToLower(levelStr))
	if err != nil {
		return err
	}
	defaultLogger.SetLevel(level)
	return nil
}
