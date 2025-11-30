package logger

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetLogger(t *testing.T) {
	logger := GetLogger()
	assert.NotNil(t, logger)
	assert.IsType(t, &logrus.Logger{}, logger)
}

func TestWithName(t *testing.T) {
	entry := WithName("test-logger")
	assert.NotNil(t, entry)
	assert.Equal(t, "test-logger", entry.Data["name"])
}

func TestWithFields(t *testing.T) {
	fields := logrus.Fields{
		"key1": "value1",
		"key2": "value2",
	}
	entry := WithFields(fields)
	assert.NotNil(t, entry)
	assert.Equal(t, "value1", entry.Data["key1"])
	assert.Equal(t, "value2", entry.Data["key2"])
}

func TestSetLevel(t *testing.T) {
	originalLevel := defaultLogger.Level
	defer SetLevel(originalLevel) // Restore original level

	SetLevel(logrus.DebugLevel)
	assert.Equal(t, logrus.DebugLevel, defaultLogger.Level)

	SetLevel(logrus.WarnLevel)
	assert.Equal(t, logrus.WarnLevel, defaultLogger.Level)
}

func TestIsLevelEnabled(t *testing.T) {
	originalLevel := defaultLogger.Level
	defer SetLevel(originalLevel) // Restore original level

	SetLevel(logrus.DebugLevel)
	assert.True(t, IsLevelEnabled(logrus.DebugLevel))
	assert.True(t, IsLevelEnabled(logrus.InfoLevel))
	assert.False(t, IsLevelEnabled(logrus.TraceLevel))

	SetLevel(logrus.ErrorLevel)
	assert.False(t, IsLevelEnabled(logrus.InfoLevel))
	assert.True(t, IsLevelEnabled(logrus.ErrorLevel))
}

func TestInit(t *testing.T) {
	// Test that init properly configured the logger
	assert.NotNil(t, defaultLogger)

	// In test environment (GO_ENV=test), logger should be silent
	env := os.Getenv("GO_ENV")
	if env == "test" {
		// Logger should be configured but output is redirected
		assert.NotNil(t, defaultLogger.Out)
	}
}

func TestConfigureFromString(t *testing.T) {
	// Save original state
	originalLevel := defaultLogger.Level
	originalOut := defaultLogger.Out
	originalEnv := os.Getenv("GO_ENV")
	defer func() {
		SetLevel(originalLevel)
		defaultLogger.Out = originalOut
		os.Setenv("GO_ENV", originalEnv)
	}()

	t.Run("in test mode returns nil", func(t *testing.T) {
		os.Setenv("GO_ENV", "test")
		err := ConfigureFromString("debug")
		assert.NoError(t, err)
	})

	t.Run("silent mode in non-test", func(t *testing.T) {
		os.Setenv("GO_ENV", "")
		err := ConfigureFromString("silent")
		assert.NoError(t, err)
	})

	t.Run("valid log levels", func(t *testing.T) {
		os.Setenv("GO_ENV", "")

		levels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
		for _, level := range levels {
			err := ConfigureFromString(level)
			assert.NoError(t, err, "Should not error for level: %s", level)
		}
	})

	t.Run("invalid log level", func(t *testing.T) {
		os.Setenv("GO_ENV", "")
		err := ConfigureFromString("invalid_level")
		assert.Error(t, err)
	})

	t.Run("case insensitive", func(t *testing.T) {
		os.Setenv("GO_ENV", "")
		err := ConfigureFromString("DEBUG")
		assert.NoError(t, err)

		err = ConfigureFromString("Info")
		assert.NoError(t, err)
	})
}
