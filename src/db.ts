import { Database } from 'bun:sqlite';
import { createChildLogger } from './logger';
import * as path from 'path';

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

export interface SelectionSet {
  id?: number;
  name: string;
  description?: string;
  criteria_type: 'user_selected' | 'tool_query';
  criteria_json?: string;
  created_at?: number;
  updated_at?: number;
}

export interface SelectionCriteria {
  tool: string;
  params: Record<string, any>;
  limit?: number;
}

export interface Query {
  id?: number;
  name: string;
  description?: string;
  query_type: 'file_filter' | 'custom_script';
  query_json: string;
  target_selection_set?: string;
  update_mode?: 'replace' | 'append' | 'merge';
  created_at?: number;
  updated_at?: number;
  last_executed?: number;
  execution_count?: number;
}

export interface FileFilter {
  path?: string;
  pattern?: string;
  extensions?: string[];
  minSize?: number;
  maxSize?: number;
  minDate?: string;
  maxDate?: string;
  nameContains?: string;
  pathContains?: string;
  sortBy?: 'size' | 'name' | 'mtime';
  descendingSort?: boolean;
  limit?: number;
}

export interface QueryExecution {
  id?: number;
  query_id: number;
  executed_at?: number;
  duration_ms?: number;
  files_matched?: number;
  status: 'success' | 'error';
  error_message?: string;
}

export class DiskDB {
  db: Database;
  private insertStmt: any;

  constructor(path: string = 'disk.db') {
    logger.info({ path }, 'Initializing database');
    this.db = new Database(path);
    this.init();
    this.prepareStatements();
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
    this.db.run('CREATE INDEX IF NOT EXISTS idx_mtime ON entries(mtime)');
    
    // Create SelectionSet tables
    this.db.run(`CREATE TABLE IF NOT EXISTS selection_sets (
      id INTEGER PRIMARY KEY,
      name TEXT UNIQUE NOT NULL,
      description TEXT,
      criteria_type TEXT CHECK(criteria_type IN ('user_selected', 'tool_query')),
      criteria_json TEXT,
      created_at INTEGER DEFAULT (strftime('%s', 'now')),
      updated_at INTEGER DEFAULT (strftime('%s', 'now'))
    )`);
    
    this.db.run(`CREATE TABLE IF NOT EXISTS selection_set_entries (
      set_id INTEGER NOT NULL,
      entry_path TEXT NOT NULL,
      added_at INTEGER DEFAULT (strftime('%s', 'now')),
      PRIMARY KEY (set_id, entry_path),
      FOREIGN KEY (set_id) REFERENCES selection_sets(id) ON DELETE CASCADE,
      FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
    )`);
    
    this.db.run('CREATE INDEX IF NOT EXISTS idx_set_entries ON selection_set_entries(set_id)');
    
    // Create Query tables
    this.db.run(`CREATE TABLE IF NOT EXISTS queries (
      id INTEGER PRIMARY KEY,
      name TEXT UNIQUE NOT NULL,
      description TEXT,
      query_type TEXT CHECK(query_type IN ('file_filter', 'custom_script')),
      query_json TEXT NOT NULL,
      target_selection_set TEXT,
      update_mode TEXT CHECK(update_mode IN ('replace', 'append', 'merge')) DEFAULT 'replace',
      created_at INTEGER DEFAULT (strftime('%s', 'now')),
      updated_at INTEGER DEFAULT (strftime('%s', 'now')),
      last_executed INTEGER,
      execution_count INTEGER DEFAULT 0
    )`);
    
    this.db.run(`CREATE TABLE IF NOT EXISTS query_executions (
      id INTEGER PRIMARY KEY,
      query_id INTEGER NOT NULL,
      executed_at INTEGER DEFAULT (strftime('%s', 'now')),
      duration_ms INTEGER,
      files_matched INTEGER,
      status TEXT CHECK(status IN ('success', 'error')),
      error_message TEXT,
      FOREIGN KEY (query_id) REFERENCES queries(id) ON DELETE CASCADE
    )`);
    
    this.db.run('CREATE INDEX IF NOT EXISTS idx_query_executions ON query_executions(query_id, executed_at DESC)');
    
    logger.debug('Database initialization complete');
  }

  private prepareStatements() {
    this.insertStmt = this.db.prepare(`
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
  }

  insertOrUpdate(entry: Entry): void {
    if (logger.isLevelEnabled('trace')) {
      logger.trace({ path: entry.path, kind: entry.kind, size: entry.size }, 'Inserting/updating entry');
    }
    this.insertStmt.run(
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
    
    // Use prepared statements and transactions for better performance
    const updateStmt = this.db.prepare(`UPDATE entries SET size = ? WHERE path = ?`);
    const sumStmt = this.db.prepare(`SELECT SUM(size) as total FROM entries WHERE parent = ?`);
    
    this.db.transaction(() => {
      for (const d of dirs) {
        const row = sumStmt.get(d.path) as { total: number } | undefined;
        const total = row?.total ?? 0;
        updateStmt.run(total, d.path);
        if (logger.isLevelEnabled('trace')) {
          logger.trace({ path: d.path, aggregateSize: total }, 'Updated directory size');
        }
      }
    })();
    
    logger.info({ root, directoriesProcessed: dirs.length }, 'Aggregate computation complete');
  }

  children(parent: string): Entry[] {
    if (logger.isLevelEnabled('trace')) {
      logger.trace({ parent }, 'Fetching children');
    }
    const results = this.db
      .query(`SELECT * FROM entries WHERE parent = ?`)
      .all(parent) as Entry[];
    if (logger.isLevelEnabled('trace')) {
      logger.trace({ parent, childCount: results.length }, 'Children fetched');
    }
    return results;
  }

  get(path: string): Entry | undefined {
    if (logger.isLevelEnabled('trace')) {
      logger.trace({ path }, 'Fetching entry');
    }
    const result = this.db
      .query(`SELECT * FROM entries WHERE path = ?`)
      .get(path) as Entry | undefined;
    if (logger.isLevelEnabled('trace')) {
      logger.trace({ path, found: !!result }, 'Entry fetch complete');
    }
    return result;
  }

  beginTransaction() {
    this.db.run('BEGIN');
  }

  commitTransaction() {
    this.db.run('COMMIT');
  }

  rollbackTransaction() {
    this.db.run('ROLLBACK');
  }

  // SelectionSet Management Methods
  
  createSelectionSet(set: SelectionSet): number {
    logger.info({ name: set.name, type: set.criteria_type }, 'Creating selection set');
    
    const stmt = this.db.prepare(`
      INSERT INTO selection_sets (name, description, criteria_type, criteria_json)
      VALUES (?, ?, ?, ?)
    `);
    
    const result = stmt.run(
      set.name,
      set.description || null,
      set.criteria_type,
      set.criteria_json || null
    );
    
    return (result as any).lastInsertRowid;
  }

  getSelectionSet(name: string): SelectionSet | undefined {
    const result = this.db
      .query(`SELECT * FROM selection_sets WHERE name = ?`)
      .get(name) as SelectionSet | undefined;
    return result;
  }

  listSelectionSets(): SelectionSet[] {
    const results = this.db
      .query(`SELECT * FROM selection_sets ORDER BY created_at DESC`)
      .all() as SelectionSet[];
    return results;
  }

  deleteSelectionSet(name: string): void {
    logger.info({ name }, 'Deleting selection set');
    this.db.query(`DELETE FROM selection_sets WHERE name = ?`).run(name);
  }

  addToSelectionSet(setName: string, paths: string[]): void {
    const set = this.getSelectionSet(setName);
    if (!set || !set.id) {
      throw new Error(`Selection set '${setName}' not found`);
    }
    
    const stmt = this.db.prepare(`
      INSERT OR IGNORE INTO selection_set_entries (set_id, entry_path)
      VALUES (?, ?)
    `);
    
    this.db.transaction(() => {
      for (const path of paths) {
        stmt.run(set.id, path);
      }
    })();
    
    // Update the set's updated_at timestamp
    this.db.query(`UPDATE selection_sets SET updated_at = strftime('%s', 'now') WHERE id = ?`).run(set.id);
    
    logger.info({ setName, count: paths.length }, 'Added entries to selection set');
  }

  removeFromSelectionSet(setName: string, paths: string[]): void {
    const set = this.getSelectionSet(setName);
    if (!set || !set.id) {
      throw new Error(`Selection set '${setName}' not found`);
    }
    
    const stmt = this.db.prepare(`
      DELETE FROM selection_set_entries WHERE set_id = ? AND entry_path = ?
    `);
    
    this.db.transaction(() => {
      for (const path of paths) {
        stmt.run(set.id, path);
      }
    })();
    
    // Update the set's updated_at timestamp
    this.db.query(`UPDATE selection_sets SET updated_at = strftime('%s', 'now') WHERE id = ?`).run(set.id);
    
    logger.info({ setName, count: paths.length }, 'Removed entries from selection set');
  }

  getSelectionSetEntries(setName: string): Entry[] {
    const set = this.getSelectionSet(setName);
    if (!set || !set.id) {
      throw new Error(`Selection set '${setName}' not found`);
    }
    
    const results = this.db.query(`
      SELECT e.* FROM entries e
      JOIN selection_set_entries sse ON e.path = sse.entry_path
      WHERE sse.set_id = ?
      ORDER BY sse.added_at DESC
    `).all(set.id) as Entry[];
    
    return results;
  }

  getSelectionSetStats(setName: string): { count: number; totalSize: number } {
    const set = this.getSelectionSet(setName);
    if (!set || !set.id) {
      throw new Error(`Selection set '${setName}' not found`);
    }
    
    const result = this.db.query(`
      SELECT COUNT(*) as count, COALESCE(SUM(e.size), 0) as totalSize
      FROM entries e
      JOIN selection_set_entries sse ON e.path = sse.entry_path
      WHERE sse.set_id = ?
    `).get(set.id) as { count: number; totalSize: number };
    
    return result;
  }

  // Query Management Methods
  
  createQuery(query: Query): number {
    logger.info({ name: query.name, type: query.query_type }, 'Creating query');
    
    const stmt = this.db.prepare(`
      INSERT INTO queries (name, description, query_type, query_json, target_selection_set, update_mode)
      VALUES (?, ?, ?, ?, ?, ?)
    `);
    
    const result = stmt.run(
      query.name,
      query.description || null,
      query.query_type,
      query.query_json,
      query.target_selection_set || null,
      query.update_mode || 'replace'
    );
    
    return (result as any).lastInsertRowid;
  }

  getQuery(name: string): Query | undefined {
    const result = this.db
      .query(`SELECT * FROM queries WHERE name = ?`)
      .get(name) as Query | undefined;
    return result;
  }

  listQueries(): Query[] {
    const results = this.db
      .query(`SELECT * FROM queries ORDER BY created_at DESC`)
      .all() as Query[];
    return results;
  }

  updateQuery(name: string, updates: Partial<Query>): void {
    const fieldsToUpdate: string[] = [];
    const values: any[] = [];
    
    if (updates.description !== undefined) {
      fieldsToUpdate.push('description = ?');
      values.push(updates.description);
    }
    if (updates.query_json !== undefined) {
      fieldsToUpdate.push('query_json = ?');
      values.push(updates.query_json);
    }
    if (updates.target_selection_set !== undefined) {
      fieldsToUpdate.push('target_selection_set = ?');
      values.push(updates.target_selection_set);
    }
    if (updates.update_mode !== undefined) {
      fieldsToUpdate.push('update_mode = ?');
      values.push(updates.update_mode);
    }
    
    if (fieldsToUpdate.length === 0) return;
    
    fieldsToUpdate.push('updated_at = strftime(\'%s\', \'now\')');
    values.push(name);
    
    const sql = `UPDATE queries SET ${fieldsToUpdate.join(', ')} WHERE name = ?`;
    this.db.query(sql).run(...values);
  }

  deleteQuery(name: string): void {
    logger.info({ name }, 'Deleting query');
    this.db.query(`DELETE FROM queries WHERE name = ?`).run(name);
  }

  executeQuery(name: string): { selectionSet: string; filesMatched: number } {
    const query = this.getQuery(name);
    if (!query) {
      throw new Error(`Query '${name}' not found`);
    }
    
    const startTime = Date.now();
    let error: string | null = null;
    let filesMatched = 0;
    
    try {
      // Parse the query
      const filter = JSON.parse(query.query_json) as FileFilter;
      
      // Execute the file filter
      const matchedFiles = this.executeFileFilter(filter);
      filesMatched = matchedFiles.length;
      
      // Update or create the selection set
      if (query.target_selection_set) {
        const existingSet = this.getSelectionSet(query.target_selection_set);
        
        if (!existingSet) {
          // Create new selection set
          this.createSelectionSet({
            name: query.target_selection_set,
            description: `Generated by query: ${query.name}`,
            criteria_type: 'tool_query',
            criteria_json: JSON.stringify({ query: query.name })
          });
        }
        
        // Update the selection set based on update_mode
        if (query.update_mode === 'replace') {
          // Clear existing entries and add new ones
          if (existingSet?.id) {
            this.db.query(`DELETE FROM selection_set_entries WHERE set_id = ?`).run(existingSet.id);
          }
          this.addToSelectionSet(query.target_selection_set, matchedFiles);
        } else if (query.update_mode === 'append') {
          // Just add new entries
          this.addToSelectionSet(query.target_selection_set, matchedFiles);
        } else if (query.update_mode === 'merge') {
          // Add only new entries that don't exist
          this.addToSelectionSet(query.target_selection_set, matchedFiles);
        }
      }
      
      // Update query metadata
      this.db.query(`
        UPDATE queries 
        SET last_executed = strftime('%s', 'now'), 
            execution_count = execution_count + 1 
        WHERE id = ?
      `).run(query.id);
      
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      logger.error({ query: name, error }, 'Query execution failed');
    }
    
    // Record execution
    const duration = Date.now() - startTime;
    this.db.query(`
      INSERT INTO query_executions (query_id, duration_ms, files_matched, status, error_message)
      VALUES (?, ?, ?, ?, ?)
    `).run(
      query.id,
      duration,
      filesMatched,
      error ? 'error' : 'success',
      error
    );
    
    if (error) {
      throw new Error(error);
    }
    
    return {
      selectionSet: query.target_selection_set || '',
      filesMatched
    };
  }

  private executeFileFilter(filter: FileFilter): string[] {
    const matchedPaths: string[] = [];
    const rootPath = filter.path ? path.resolve(filter.path) : '/home/josh/Projects/mcp-space-browser';
    
    const minDate = filter.minDate ? new Date(filter.minDate).getTime() / 1000 : undefined;
    const maxDate = filter.maxDate ? new Date(filter.maxDate).getTime() / 1000 : undefined;
    
    function collectFiles(root: string, db: DiskDB): void {
      const entry = db.get(root);
      if (!entry) return;
      
      if (entry.kind === 'file') {
        let matches = true;
        
        // Check size filters
        if (filter.minSize !== undefined && entry.size < filter.minSize) matches = false;
        if (filter.maxSize !== undefined && entry.size > filter.maxSize) matches = false;
        
        // Check date filters
        if (minDate && entry.mtime < minDate) matches = false;
        if (maxDate && entry.mtime > maxDate) matches = false;
        
        // Check name/path filters
        const basename = path.basename(entry.path);
        const ext = path.extname(entry.path).toLowerCase();
        
        if (filter.extensions && filter.extensions.length > 0) {
          const extensions = filter.extensions.map(e => e.startsWith('.') ? e : '.' + e);
          if (!extensions.includes(ext)) matches = false;
        }
        
        if (filter.nameContains && !basename.toLowerCase().includes(filter.nameContains.toLowerCase())) {
          matches = false;
        }
        
        if (filter.pathContains && !entry.path.toLowerCase().includes(filter.pathContains.toLowerCase())) {
          matches = false;
        }
        
        if (filter.pattern) {
          try {
            const regex = new RegExp(filter.pattern);
            if (!regex.test(entry.path)) matches = false;
          } catch (e) {
            // Invalid regex, skip
            matches = false;
          }
        }
        
        if (matches) {
          matchedPaths.push(entry.path);
        }
      } else if (entry.kind === 'directory') {
        const children = db.children(entry.path);
        for (const child of children) {
          collectFiles(child.path, db);
        }
      }
    }
    
    // Collect all matching files
    collectFiles(rootPath, this);
    
    // Sort if requested
    if (filter.sortBy) {
      matchedPaths.sort((a, b) => {
        const entryA = this.get(a)!;
        const entryB = this.get(b)!;
        
        if (filter.sortBy === 'size') {
          return filter.descendingSort ? entryB.size - entryA.size : entryA.size - entryB.size;
        } else if (filter.sortBy === 'mtime') {
          return filter.descendingSort ? entryB.mtime - entryA.mtime : entryA.mtime - entryB.mtime;
        } else {
          const comparison = a.localeCompare(b);
          return filter.descendingSort ? -comparison : comparison;
        }
      });
    }
    
    // Apply limit
    if (filter.limit && filter.limit > 0) {
      return matchedPaths.slice(0, filter.limit);
    }
    
    return matchedPaths;
  }

  getQueryExecutions(queryName: string, limit: number = 10): QueryExecution[] {
    const query = this.getQuery(queryName);
    if (!query || !query.id) {
      return [];
    }
    
    const results = this.db.query(`
      SELECT * FROM query_executions 
      WHERE query_id = ? 
      ORDER BY executed_at DESC 
      LIMIT ?
    `).all(query.id, limit) as QueryExecution[];
    
    return results;
  }
}
