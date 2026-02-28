package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttributeValidation(t *testing.T) {
	t.Run("valid attribute", func(t *testing.T) {
		a := &Attribute{
			EntryPath:  "/test/file.txt",
			Key:        "mime",
			Value:      "image/jpeg",
			Source:     "scan",
			ComputedAt: 1000,
		}
		assert.NoError(t, a.Validate())
	})

	t.Run("missing entry path", func(t *testing.T) {
		a := &Attribute{Key: "mime", Value: "image/jpeg", Source: "scan"}
		assert.Error(t, a.Validate())
	})

	t.Run("missing key", func(t *testing.T) {
		a := &Attribute{EntryPath: "/test", Value: "x", Source: "scan"}
		assert.Error(t, a.Validate())
	})

	t.Run("missing source", func(t *testing.T) {
		a := &Attribute{EntryPath: "/test", Key: "mime", Value: "x"}
		assert.Error(t, a.Validate())
	})

	t.Run("invalid source", func(t *testing.T) {
		a := &Attribute{EntryPath: "/test", Key: "mime", Value: "x", Source: "bad"}
		assert.Error(t, a.Validate())
	})
}
