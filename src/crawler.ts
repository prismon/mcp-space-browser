import { promises as fs } from 'fs';
import * as path from 'path';
import { DiskDB } from './db';
import { createChildLogger } from './logger';

const logger = createChildLogger('crawler');

export async function index(root: string, db: DiskDB) {
  const abs = path.resolve(root);
  const runId = Date.now();
  const stack: string[] = [abs];
  
  logger.info({ root: abs, runId }, 'Starting filesystem index');
  
  let filesProcessed = 0;
  let directoriesProcessed = 0;
  let totalSize = 0;
  let errors = 0;
  
  while (stack.length) {
    const current = stack.pop()!;
    
    try {
      logger.debug({ path: current, remaining: stack.length }, 'Processing path');
      
      const info = await fs.stat(current);
      const isDir = info.isDirectory();
      
      db.insertOrUpdate({
        path: current,
        parent: path.dirname(current) === current ? null : path.dirname(current),
        size: info.size,
        kind: isDir ? 'directory' : 'file',
        ctime: info.ctimeMs,
        mtime: info.mtimeMs,
        last_scanned: runId
      });
      
      if (isDir) {
        directoriesProcessed++;
        logger.debug({ path: current }, 'Scanning directory');
        
        const children = await fs.readdir(current);
        logger.trace({ path: current, childCount: children.length }, 'Directory contents');
        
        for (const c of children) {
          stack.push(path.join(current, c));
        }
      } else {
        filesProcessed++;
        totalSize += info.size;
        logger.trace({ path: current, size: info.size }, 'File processed');
      }
    } catch (error) {
      errors++;
      logger.error({ path: current, error }, 'Failed to process path');
    }
  }
  
  logger.info({
    root: abs,
    filesProcessed,
    directoriesProcessed,
    totalSize,
    errors,
    runId
  }, 'Filesystem scan complete');
  
  logger.debug({ root: abs, runId }, 'Deleting stale entries');
  db.deleteStale(abs, runId);
  
  logger.debug({ root: abs }, 'Computing aggregate sizes');
  db.computeAggregates(abs);
  
  logger.info({ root: abs }, 'Index operation complete');
}
