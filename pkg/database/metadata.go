package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
)

// initMetadataTable creates the unified metadata table if it doesn't exist.
func (d *DiskDB) initMetadataTable() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS metadata (
			entry_path TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT,
			source TEXT NOT NULL DEFAULT 'scan',
			cache_path TEXT,
			data_json TEXT,
			mime_type TEXT,
			file_size INTEGER DEFAULT 0,
			generator TEXT,
			hash TEXT UNIQUE,
			created_at INTEGER DEFAULT (strftime('%s', 'now')),
			updated_at INTEGER DEFAULT (strftime('%s', 'now')),
			FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_metadata_simple ON metadata(entry_path, key) WHERE hash IS NULL;
		CREATE INDEX IF NOT EXISTS idx_metadata_entry ON metadata(entry_path);
		CREATE INDEX IF NOT EXISTS idx_metadata_key ON metadata(key);
		CREATE INDEX IF NOT EXISTS idx_metadata_source ON metadata(source);
	`)
	return err
}

// SetMetadata inserts or updates a single metadata record (upsert).
// For simple metadata (hash == nil): upserts on (entry_path, key).
// For artifact metadata (hash != nil): upserts on hash.
func (d *DiskDB) SetMetadata(m *models.MetadataRecord) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	now := time.Now().Unix()
	if m.UpdatedAt == 0 {
		m.UpdatedAt = now
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}

	if m.Hash != nil {
		// Artifact metadata: upsert by hash
		_, err := d.db.Exec(`
			INSERT INTO metadata (entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(hash) DO UPDATE SET
				entry_path = excluded.entry_path,
				key = excluded.key,
				value = excluded.value,
				source = excluded.source,
				cache_path = excluded.cache_path,
				data_json = excluded.data_json,
				mime_type = excluded.mime_type,
				file_size = excluded.file_size,
				generator = excluded.generator,
				updated_at = excluded.updated_at
		`, m.EntryPath, m.Key, m.Value, m.Source, m.CachePath, m.DataJson, m.MimeType,
			m.FileSize, m.Generator, m.Hash, m.CreatedAt, m.UpdatedAt)
		return err
	}

	// Simple metadata: upsert by (entry_path, key) where hash IS NULL
	_, err := d.db.Exec(`
		INSERT INTO metadata (entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
		ON CONFLICT(entry_path, key) WHERE hash IS NULL DO UPDATE SET
			value = excluded.value,
			source = excluded.source,
			cache_path = excluded.cache_path,
			data_json = excluded.data_json,
			mime_type = excluded.mime_type,
			file_size = excluded.file_size,
			generator = excluded.generator,
			updated_at = excluded.updated_at
	`, m.EntryPath, m.Key, m.Value, m.Source, m.CachePath, m.DataJson, m.MimeType,
		m.FileSize, m.Generator, m.CreatedAt, m.UpdatedAt)
	return err
}

// SetMetadataBatch inserts or updates multiple metadata records in a transaction.
func (d *DiskDB) SetMetadataBatch(records []*models.MetadataRecord) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for _, m := range records {
		if err := m.Validate(); err != nil {
			tx.Rollback()
			return fmt.Errorf("invalid metadata %q for %q: %w", m.Key, m.EntryPath, err)
		}

		now := time.Now().Unix()
		if m.UpdatedAt == 0 {
			m.UpdatedAt = now
		}
		if m.CreatedAt == 0 {
			m.CreatedAt = now
		}

		if m.Hash != nil {
			_, err = tx.Exec(`
				INSERT INTO metadata (entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(hash) DO UPDATE SET
					entry_path = excluded.entry_path,
					key = excluded.key,
					value = excluded.value,
					source = excluded.source,
					cache_path = excluded.cache_path,
					data_json = excluded.data_json,
					mime_type = excluded.mime_type,
					file_size = excluded.file_size,
					generator = excluded.generator,
					updated_at = excluded.updated_at
			`, m.EntryPath, m.Key, m.Value, m.Source, m.CachePath, m.DataJson, m.MimeType,
				m.FileSize, m.Generator, m.Hash, m.CreatedAt, m.UpdatedAt)
		} else {
			_, err = tx.Exec(`
				INSERT INTO metadata (entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
				ON CONFLICT(entry_path, key) WHERE hash IS NULL DO UPDATE SET
					value = excluded.value,
					source = excluded.source,
					cache_path = excluded.cache_path,
					data_json = excluded.data_json,
					mime_type = excluded.mime_type,
					file_size = excluded.file_size,
					generator = excluded.generator,
					updated_at = excluded.updated_at
			`, m.EntryPath, m.Key, m.Value, m.Source, m.CachePath, m.DataJson, m.MimeType,
				m.FileSize, m.Generator, m.CreatedAt, m.UpdatedAt)
		}

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("insert metadata %q for %q: %w", m.Key, m.EntryPath, err)
		}
	}

	return tx.Commit()
}

// GetMetadataByKey retrieves a single simple metadata record by entry path and key.
// Returns nil if not found.
func (d *DiskDB) GetMetadataByKey(entryPath, key string) (*models.MetadataRecord, error) {
	row := d.db.QueryRow(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE entry_path = ? AND key = ? AND hash IS NULL
	`, entryPath, key)

	return scanMetadataRecord(row)
}

// GetAllMetadata retrieves all metadata records (simple + artifact) for an entry.
func (d *DiskDB) GetAllMetadata(entryPath string) ([]*models.MetadataRecord, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE entry_path = ? ORDER BY key, created_at DESC
	`, entryPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMetadataRecords(rows)
}

// GetSimpleMetadata retrieves only simple metadata (hash IS NULL) for an entry.
func (d *DiskDB) GetSimpleMetadata(entryPath string) ([]*models.MetadataRecord, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE entry_path = ? AND hash IS NULL ORDER BY key
	`, entryPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMetadataRecords(rows)
}

// GetArtifactMetadata retrieves only artifact metadata (hash IS NOT NULL) for an entry.
func (d *DiskDB) GetArtifactMetadata(entryPath string) ([]*models.MetadataRecord, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE entry_path = ? AND hash IS NOT NULL ORDER BY key, created_at DESC
	`, entryPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMetadataRecords(rows)
}

// GetArtifactByKey retrieves the most recent artifact metadata for a given entry path and key.
func (d *DiskDB) GetArtifactByKey(entryPath, key string) (*models.MetadataRecord, error) {
	row := d.db.QueryRow(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE entry_path = ? AND key = ? AND hash IS NOT NULL
		ORDER BY created_at DESC LIMIT 1
	`, entryPath, key)

	return scanMetadataRecord(row)
}

// QueryMetadataByKey retrieves all metadata records with a given key across all entries.
func (d *DiskDB) QueryMetadataByKey(key string) ([]*models.MetadataRecord, error) {
	rows, err := d.db.Query(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE key = ? ORDER BY entry_path
	`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMetadataRecords(rows)
}

// GetMetadataByHash retrieves a metadata record by its unique hash.
func (d *DiskDB) GetMetadataByHash(hash string) (*models.MetadataRecord, error) {
	row := d.db.QueryRow(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE hash = ?
	`, hash)

	return scanMetadataRecord(row)
}

// GetMetadataByCachePath retrieves a metadata record by its cache path.
func (d *DiskDB) GetMetadataByCachePath(cachePath string) (*models.MetadataRecord, error) {
	row := d.db.QueryRow(`
		SELECT entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash, created_at, updated_at
		FROM metadata WHERE cache_path = ? LIMIT 1
	`, cachePath)

	return scanMetadataRecord(row)
}

// DeleteMetadataByKey removes a single simple metadata record for an entry.
func (d *DiskDB) DeleteMetadataByKey(entryPath, key string) error {
	_, err := d.db.Exec(`DELETE FROM metadata WHERE entry_path = ? AND key = ? AND hash IS NULL`, entryPath, key)
	return err
}

// DeleteMetadataByEntry removes all metadata (simple + artifact) for an entry.
func (d *DiskDB) DeleteMetadataByEntry(entryPath string) (int64, error) {
	result, err := d.db.Exec(`DELETE FROM metadata WHERE entry_path = ?`, entryPath)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteMetadataByHash removes a metadata record by its unique hash.
func (d *DiskDB) DeleteMetadataByHash(hash string) error {
	_, err := d.db.Exec(`DELETE FROM metadata WHERE hash = ?`, hash)
	return err
}

// scanMetadataRecord scans a single row into a MetadataRecord.
// Returns nil, nil if no rows found.
func scanMetadataRecord(row *sql.Row) (*models.MetadataRecord, error) {
	m := &models.MetadataRecord{}
	err := row.Scan(
		&m.EntryPath, &m.Key, &m.Value, &m.Source, &m.CachePath, &m.DataJson,
		&m.MimeType, &m.FileSize, &m.Generator, &m.Hash, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

// scanMetadataRecords scans multiple rows into MetadataRecord slices.
func scanMetadataRecords(rows *sql.Rows) ([]*models.MetadataRecord, error) {
	var records []*models.MetadataRecord
	for rows.Next() {
		m := &models.MetadataRecord{}
		if err := rows.Scan(
			&m.EntryPath, &m.Key, &m.Value, &m.Source, &m.CachePath, &m.DataJson,
			&m.MimeType, &m.FileSize, &m.Generator, &m.Hash, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, m)
	}
	return records, rows.Err()
}
