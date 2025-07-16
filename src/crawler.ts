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
  let lastProgressLog = Date.now();
  
  // Begin transaction for better performance
  db.beginTransaction();
  
  try {
    while (stack.length) {
      const current = stack.pop()!;
      
      try {
        if (logger.isLevelEnabled('debug')) {
          logger.debug({ path: current, remaining: stack.length }, 'Processing path');
        }
        
        const info = await fs.stat(current);
        const isDir = info.isDirectory();
        
        db.insertOrUpdate({
          path: current,
          parent: path.dirname(current) === current ? null : path.dirname(current),
          size: info.size,
          kind: isDir ? 'directory' : 'file',
          ctime: Math.floor(info.ctimeMs / 1000),
          mtime: Math.floor(info.mtimeMs / 1000),
          last_scanned: runId
        });
        
        if (isDir) {
          directoriesProcessed++;
          if (logger.isLevelEnabled('debug')) {
            logger.debug({ path: current }, 'Scanning directory');
          }
          
          const children = await fs.readdir(current);
          if (logger.isLevelEnabled('trace')) {
            logger.trace({ path: current, childCount: children.length }, 'Directory contents');
          }
          
          for (const c of children) {
            stack.push(path.join(current, c));
          }
        } else {
          filesProcessed++;
          totalSize += info.size;
          if (logger.isLevelEnabled('trace')) {
            logger.trace({ path: current, size: info.size }, 'File processed');
          }
        }
        
        // Log progress every 5 seconds
        const now = Date.now();
        if (now - lastProgressLog > 5000) {
          logger.info({ filesProcessed, directoriesProcessed, remaining: stack.length }, 'Index progress');
          lastProgressLog = now;
        }
      } catch (error) {
        errors++;
        logger.error({ path: current, error }, 'Failed to process path');
      }
    }
    
    // Commit the transaction
    db.commitTransaction();
  } catch (error) {
    // Rollback on error
    db.rollbackTransaction();
    throw error;
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
