import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';
import { createChildLogger } from './logger';

const logger = createChildLogger('cli');

function printUsage() {
  console.log(`Usage: bun run <command> <path>

Commands:
  disk-index <path>  Index the path
  disk-du <path>     Display disk usage of path
  disk-tree <path>   Display a tree of path contents
  disk-info <path>   Show database info for path`);
}

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

function diskInfo(target: string) {
  logger.info({ command: 'disk-info', target }, 'Executing command');
  const db = new DiskDB();
  const abs = path.resolve(target);
  const entry = db.get(abs);
  if (!entry) {
    logger.warn({ command: 'disk-info', target }, 'Path not found in database');
    console.error(`Error: Path '${target}' not found in database. Run 'disk-index ${target}' first.`);
    process.exit(1);
  }
  logger.info({ command: 'disk-info', target }, 'Entry fetched');
  console.log(JSON.stringify(entry, null, 2));
}

async function main() {
  const [cmd, arg] = process.argv.slice(2);
  logger.info({ command: cmd, argument: arg }, 'CLI started');
  
  if (!cmd || !arg) {
    logger.error('Missing command or argument');
    printUsage();
    process.exit(1);
  }
  
  try {
    if (cmd === 'disk-index') {
      await diskIndex(arg);
    } else if (cmd === 'disk-du') {
      diskDu(arg);
    } else if (cmd === 'disk-tree') {
      diskTree(arg);
    } else if (cmd === 'disk-info') {
      diskInfo(arg);
    } else {
      logger.error({ command: cmd }, 'Unknown command');
      printUsage();
      process.exit(1);
    }
  } catch (error) {
    logger.error({ command: cmd, error }, 'Command failed');
    console.error('Error:', error);
    process.exit(1);
  }
}

main();
