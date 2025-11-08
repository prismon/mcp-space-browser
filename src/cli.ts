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

interface TreeOptions {
  sortBy?: 'size' | 'mtime' | 'name';
  ascending?: boolean;
  minDate?: Date;
  maxDate?: Date;
}

function diskTree(target: string, indent = '', isRoot = true, db?: DiskDB, options?: TreeOptions) {
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
  
  // Apply date filtering (but always show root)
  if (!isRoot) {
    if (options?.minDate && entry.mtime < options.minDate.getTime() / 1000) {
      return;
    }
    if (options?.maxDate && entry.mtime > options.maxDate.getTime() / 1000) {
      return;
    }
  }
  
  const mtimeStr = new Date(entry.mtime * 1000).toISOString().split('T')[0];
  console.log(`${indent}${path.basename(abs)} (${entry.size}) [${mtimeStr}]`);
  
  let children = db.children(abs);
  
  // Apply date filtering to children
  if (options?.minDate || options?.maxDate) {
    children = children.filter(child => {
      if (options.minDate && child.mtime < options.minDate.getTime() / 1000) return false;
      if (options.maxDate && child.mtime > options.maxDate.getTime() / 1000) return false;
      return true;
    });
  }
  
  // Sort children
  if (options?.sortBy) {
    children.sort((a, b) => {
      let comparison = 0;
      switch (options.sortBy) {
        case 'size':
          comparison = a.size - b.size;
          break;
        case 'mtime':
          comparison = a.mtime - b.mtime;
          break;
        case 'name':
          comparison = path.basename(a.path).localeCompare(path.basename(b.path));
          break;
      }
      return options.ascending ? comparison : -comparison;
    });
  }
  
  logger.trace({ path: abs, childCount: children.length }, 'Processing children for tree');
  for (const child of children) {
    diskTree(child.path, indent + '  ', false, db, options);
  }
  if (isRoot) {
    logger.info({ command: 'disk-tree', target }, 'Tree display completed');
  }
}

async function main() {
  const args = process.argv.slice(2);
  const cmd = args[0];
  const target = args[1];
  
  logger.info({ command: cmd, argument: target }, 'CLI started');
  
  if (!cmd || !target) {
    logger.error('Missing command or argument');
    console.log('mcp-space-browser - Disk space indexing and analysis tool');
    console.log('');
    console.log('For MCP server mode (recommended):');
    console.log('  bun src/mcp.ts              Start MCP server with stdio transport');
    console.log('  bun src/mcp.ts --http-stream  Start MCP server with HTTP transport');
    console.log('');
    console.log('CLI Commands:');
    console.log('  disk-index <path>           Index a directory tree');
    console.log('  disk-du <path>              Show disk usage for a path');
    console.log('  disk-tree <path> [options]  Display tree view with sizes');
    console.log('');
    console.log('Options for disk-tree:');
    console.log('  --sort-by=<size|mtime|name>  Sort by size, modification time, or name');
    console.log('  --ascending                  Sort in ascending order (default: descending)');
    console.log('  --min-date=<YYYY-MM-DD>      Filter files modified after this date');
    console.log('  --max-date=<YYYY-MM-DD>      Filter files modified before this date');
    process.exit(1);
  }
  
  try {
    if (cmd === 'disk-index') {
      await diskIndex(target);
    } else if (cmd === 'disk-du') {
      diskDu(target);
    } else if (cmd === 'disk-tree') {
      // Parse options
      const options: TreeOptions = {};
      
      for (let i = 2; i < args.length; i++) {
        const arg = args[i];
        if (arg.startsWith('--sort-by=')) {
          const sortBy = arg.split('=')[1];
          if (['size', 'mtime', 'name'].includes(sortBy)) {
            options.sortBy = sortBy as 'size' | 'mtime' | 'name';
          }
        } else if (arg === '--ascending') {
          options.ascending = true;
        } else if (arg.startsWith('--min-date=')) {
          const dateStr = arg.split('=')[1];
          options.minDate = new Date(dateStr);
        } else if (arg.startsWith('--max-date=')) {
          const dateStr = arg.split('=')[1];
          options.maxDate = new Date(dateStr);
        }
      }
      
      diskTree(target, '', true, undefined, options);
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
