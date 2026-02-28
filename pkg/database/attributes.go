package database

import (
	"database/sql"
	"fmt"

	"github.com/prismon/mcp-space-browser/internal/models"
)

// initAttributesTable creates the attributes table if it doesn't exist
func (d *DiskDB) initAttributesTable() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS attributes (
			entry_path TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT,
			source TEXT NOT NULL,
			computed_at INTEGER,
			PRIMARY KEY (entry_path, key)
		);
		CREATE INDEX IF NOT EXISTS idx_attributes_key ON attributes(key);
		CREATE INDEX IF NOT EXISTS idx_attributes_source ON attributes(source);
	`)
	return err
}

// SetAttribute inserts or updates a single attribute (upsert)
func (d *DiskDB) SetAttribute(attr *models.Attribute) error {
	if err := attr.Validate(); err != nil {
		return fmt.Errorf("invalid attribute: %w", err)
	}
	_, err := d.db.Exec(`
		INSERT INTO attributes (entry_path, key, value, source, computed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entry_path, key) DO UPDATE SET
			value = excluded.value,
			source = excluded.source,
			computed_at = excluded.computed_at
	`, attr.EntryPath, attr.Key, attr.Value, attr.Source, attr.ComputedAt)
	return err
}

// SetAttributesBatch inserts or updates multiple attributes in a transaction
func (d *DiskDB) SetAttributesBatch(attrs []*models.Attribute) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO attributes (entry_path, key, value, source, computed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entry_path, key) DO UPDATE SET
			value = excluded.value,
			source = excluded.source,
			computed_at = excluded.computed_at
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, attr := range attrs {
		if err := attr.Validate(); err != nil {
			tx.Rollback()
			return fmt.Errorf("invalid attribute %q for %q: %w", attr.Key, attr.EntryPath, err)
		}
		if _, err := stmt.Exec(attr.EntryPath, attr.Key, attr.Value, attr.Source, attr.ComputedAt); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert attribute %q for %q: %w", attr.Key, attr.EntryPath, err)
		}
	}

	return tx.Commit()
}

// GetAttribute retrieves a single attribute by entry path and key. Returns nil if not found.
func (d *DiskDB) GetAttribute(entryPath, key string) (*models.Attribute, error) {
	row := d.db.QueryRow(`
		SELECT entry_path, key, value, source, computed_at
		FROM attributes WHERE entry_path = ? AND key = ?
	`, entryPath, key)

	attr := &models.Attribute{}
	err := row.Scan(&attr.EntryPath, &attr.Key, &attr.Value, &attr.Source, &attr.ComputedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return attr, nil
}

// GetAttributes retrieves all attributes for an entry
func (d *DiskDB) GetAttributes(entryPath string) ([]*models.Attribute, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, computed_at
		FROM attributes WHERE entry_path = ? ORDER BY key
	`, entryPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attrs []*models.Attribute
	for rows.Next() {
		attr := &models.Attribute{}
		if err := rows.Scan(&attr.EntryPath, &attr.Key, &attr.Value, &attr.Source, &attr.ComputedAt); err != nil {
			return nil, err
		}
		attrs = append(attrs, attr)
	}
	return attrs, rows.Err()
}

// QueryAttributesByKey retrieves all attributes with a given key across all entries
func (d *DiskDB) QueryAttributesByKey(key string) ([]*models.Attribute, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, computed_at
		FROM attributes WHERE key = ? ORDER BY entry_path
	`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attrs []*models.Attribute
	for rows.Next() {
		attr := &models.Attribute{}
		if err := rows.Scan(&attr.EntryPath, &attr.Key, &attr.Value, &attr.Source, &attr.ComputedAt); err != nil {
			return nil, err
		}
		attrs = append(attrs, attr)
	}
	return attrs, rows.Err()
}

// DeleteAttribute removes a single attribute
func (d *DiskDB) DeleteAttribute(entryPath, key string) error {
	_, err := d.db.Exec(`DELETE FROM attributes WHERE entry_path = ? AND key = ?`, entryPath, key)
	return err
}

// DeleteAttributesByEntry removes all attributes for an entry
func (d *DiskDB) DeleteAttributesByEntry(entryPath string) error {
	_, err := d.db.Exec(`DELETE FROM attributes WHERE entry_path = ?`, entryPath)
	return err
}
