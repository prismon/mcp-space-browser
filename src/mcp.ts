import { FastMCP } from 'fastmcp';
import { z } from 'zod';
import { index as indexFs } from './crawler.js';
import { DiskDB } from './db.js';
import * as pathModule from 'path';
import { createChildLogger } from './logger.js';

const logger = createChildLogger('mcp');

const server = new FastMCP({
  name: 'Disk Space Browser',
  version: '0.1.0',
});

server.addTool({
  name: 'disk_index',
  description: 'Index a directory into the database',
  parameters: z.object({
    path: z.string(),
  }),
  execute: async ({ path: target }) => {
    const db = new DiskDB();
    await indexFs(target, db);
    return `Indexed ${target}`;
  },
});

server.addTool({
  name: 'disk_du',
  description: 'Get disk usage for a path',
  parameters: z.object({
    path: z.string(),
  }),
  execute: async ({ path: target }) => {
    const db = new DiskDB();
    const abs = pathModule.resolve(target);
    const row = db.db
      .query('SELECT size FROM entries WHERE path = ?')
      .get(abs) as { size: number } | undefined;
    if (!row) {
      return `Path not found: ${target}`;
    }
    return String(row.size);
  },
});

server.addTool({
  name: 'disk_tree',
  description: 'Return a directory tree for a path',
  parameters: z.object({
    path: z.string(),
  }),
  execute: async ({ path: target }) => {
    const db = new DiskDB();
    function buildTree(p: string, indent = ''): string {
      const abs = pathModule.resolve(p);
      const entry = db.get(abs);
      if (!entry) return '';
      let out = `${indent}${pathModule.basename(abs)} (${entry.size})\n`;
      for (const child of db.children(abs)) {
        out += buildTree(child.path, indent + '  ');
      }
      return out;
    }
    return buildTree(target);
  },
});

server.start({ transportType: 'stdio' });
logger.info('FastMCP server started');
