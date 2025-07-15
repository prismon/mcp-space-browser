import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';
import { createChildLogger } from './logger';

const logger = createChildLogger('cli');

async function diskIndex(target: string) {
  logger.info({ command: 'disk-index', target }, 'Executing command');
  const db = new DiskDB();
  await indexFs(target, db);
  logger.info({ command: 'disk-index', target }, 'Command completed successfully');
}

function diskDu(target: string) {
  logger.info({ command: 'disk-du', target }, 'Executing command');
  const db = new DiskDB();
  const abs = path.resolve(target);
  const row = db.db
    .query('SELECT size FROM entries WHERE path = ?')
    .get(abs) as { size: number } | undefined;
  
  if (!row) {
    logger.warn({ command: 'disk-du', target }, 'Path not found in database');
    console.error(`Error: Path '${target}' not found in database. Run 'disk-index ${target}' first.`);
    process.exit(1);
  }
  
  const size = row.size;
  logger.info({ command: 'disk-du', target, size }, 'Disk usage calculated');
  console.log(size);
}

function diskTree(target: string, indent = '', isRoot = true, db?: DiskDB) {
  if (isRoot) {
    logger.info({ command: 'disk-tree', target }, 'Executing command');
    db = new DiskDB();
  }
  if (!db) {
    throw new Error('Database instance required');
  }
  const abs = path.resolve(target);
  const entry = db.get(abs);
  if (!entry) {
    logger.warn({ path: abs }, 'Entry not found');
    return;
  }
  console.log(`${indent}${path.basename(abs)} (${entry.size})`);
  const children = db.children(abs);
  logger.trace({ path: abs, childCount: children.length }, 'Processing children for tree');
  for (const child of children) {
    diskTree(child.path, indent + '  ', false, db);
  }
  if (isRoot) {
    logger.info({ command: 'disk-tree', target }, 'Tree display completed');
  }
}

async function main() {
  const [cmd, arg] = process.argv.slice(2);
  logger.info({ command: cmd, argument: arg }, 'CLI started');
  
  if (!cmd || !arg) {
    logger.error('Missing command or argument');
    console.log('Usage: disk-index|disk-du|disk-tree <path>');
    process.exit(1);
  }
  
  try {
    if (cmd === 'disk-index') {
      await diskIndex(arg);
    } else if (cmd === 'disk-du') {
      diskDu(arg);
    } else if (cmd === 'disk-tree') {
      diskTree(arg);
    } else {
      logger.error({ command: cmd }, 'Unknown command');
      console.log('Unknown command');
      process.exit(1);
    }
  } catch (error) {
    logger.error({ command: cmd, error }, 'Command failed');
    console.error('Error:', error);
    process.exit(1);
  }
}

main();
