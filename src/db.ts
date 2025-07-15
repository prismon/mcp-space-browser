import { Database } from 'bun:sqlite';
import { createChildLogger } from './logger';

const logger = createChildLogger('db');

export interface Entry {
  id?: number;
  path: string;
  parent: string | null;
  size: number;
  kind: 'file' | 'directory';
  ctime: number;
  mtime: number;
  last_scanned: number;
}

export class DiskDB {
  db: Database;

  constructor(path: string = 'disk.db') {
    logger.info({ path }, 'Initializing database');
    this.db = new Database(path);
    this.init();
  }

  private init() {
    logger.debug('Creating tables and indexes');
    this.db.run(`CREATE TABLE IF NOT EXISTS entries (
      id INTEGER PRIMARY KEY,
      path TEXT UNIQUE NOT NULL,
      parent TEXT,
      size INTEGER,
      kind TEXT CHECK(kind IN ('file', 'directory')),
      ctime INTEGER,
      mtime INTEGER,
      last_scanned INTEGER,
      dirty INTEGER DEFAULT 0
    )`);
    this.db.run('CREATE INDEX IF NOT EXISTS idx_parent ON entries(parent)');
    logger.debug('Database initialization complete');
  }

  insertOrUpdate(entry: Entry): void {
    logger.trace({ path: entry.path, kind: entry.kind, size: entry.size }, 'Inserting/updating entry');
    const stmt = this.db.prepare(`
      INSERT INTO entries
        (path, parent, size, kind, ctime, mtime, last_scanned, dirty)
      VALUES (?, ?, ?, ?, ?, ?, ?, 0)
      ON CONFLICT(path) DO UPDATE SET
        parent=excluded.parent,
        size=excluded.size,
        kind=excluded.kind,
        ctime=excluded.ctime,
        mtime=excluded.mtime,
        last_scanned=excluded.last_scanned,
        dirty=0
    `);
    stmt.run(
      entry.path,
      entry.parent,
      entry.size,
      entry.kind,
      entry.ctime,
      entry.mtime,
      entry.last_scanned
    );
  }

  deleteStale(root: string, runId: number) {
    logger.debug({ root, runId }, 'Deleting stale entries');
    const result = this.db
      .query(
        `DELETE FROM entries WHERE (path = ? OR path LIKE ?) AND last_scanned < ?`
      )
      .run(root, `${root}/%`, runId);
    const deletedCount = (this.db as any).changes;
    logger.info({ root, deletedCount }, 'Stale entries deleted');
  }

  computeAggregates(root: string) {
    logger.debug({ root }, 'Computing aggregate sizes');
    const dirs = this.db
      .query(
        `SELECT path FROM entries WHERE kind = 'directory' AND (path = ? OR path LIKE ?) ORDER BY length(path) DESC`
      )
      .all(root, `${root}/%`) as { path: string }[];
    
    logger.debug({ directoryCount: dirs.length }, 'Processing directories for aggregation');
    
    for (const d of dirs) {
      const row = this.db
        .query(`SELECT SUM(size) as total FROM entries WHERE parent = ?`)
        .get(d.path) as { total: number } | undefined;
      const total = row?.total ?? 0;
      this.db.query(`UPDATE entries SET size = ? WHERE path = ?`).run(total, d.path);
      logger.trace({ path: d.path, aggregateSize: total }, 'Updated directory size');
    }
    
    logger.info({ root, directoriesProcessed: dirs.length }, 'Aggregate computation complete');
  }

  children(parent: string): Entry[] {
    logger.trace({ parent }, 'Fetching children');
    const results = this.db
      .query(`SELECT * FROM entries WHERE parent = ?`)
      .all(parent) as Entry[];
    logger.trace({ parent, childCount: results.length }, 'Children fetched');
    return results;
  }

  get(path: string): Entry | undefined {
    logger.trace({ path }, 'Fetching entry');
    const result = this.db
      .query(`SELECT * FROM entries WHERE path = ?`)
      .get(path) as Entry | undefined;
    logger.trace({ path, found: !!result }, 'Entry fetch complete');
    return result;
  }
}
