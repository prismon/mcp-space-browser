package models

import "fmt"

// Attribute represents an extensible key-value attribute for a filesystem entry.
// Base attributes (path, size, kind, timestamps) live in the entries table.
// Content-derived, usage, and enrichment attributes live here.
type Attribute struct {
	EntryPath  string `db:"entry_path" json:"entry_path"`
	Key        string `db:"key" json:"key"`
	Value      string `db:"value" json:"value"`
	Source     string `db:"source" json:"source"`
	ComputedAt int64  `db:"computed_at" json:"computed_at"`
}

const (
	AttributeSourceScan       = "scan"
	AttributeSourceEnrichment = "enrichment"
	AttributeSourceDerived    = "derived"
)

var validSources = map[string]bool{
	AttributeSourceScan:       true,
	AttributeSourceEnrichment: true,
	AttributeSourceDerived:    true,
}

func (a *Attribute) Validate() error {
	if a.EntryPath == "" {
		return fmt.Errorf("entry_path is required")
	}
	if a.Key == "" {
		return fmt.Errorf("key is required")
	}
	if a.Source == "" {
		return fmt.Errorf("source is required")
	}
	if !validSources[a.Source] {
		return fmt.Errorf("invalid source %q: must be scan, enrichment, or derived", a.Source)
	}
	return nil
}
