import { FastMCP } from 'fastmcp';
import { z } from 'zod';
import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';

const server = new FastMCP({
  name: 'mcp-space-browser',
  version: '0.1.0',
});

const PathParam = z.object({
  path: z.string().describe('File or directory path'),
});

server.addTool({
  name: 'disk-index',
  description: 'Index the specified path',
  parameters: PathParam,
  execute: async (args) => {
    const db = new DiskDB();
    await indexFs(args.path, db);
    return 'OK';
  },
});

server.addTool({
  name: 'disk-du',
  description: 'Get disk usage for a path',
  parameters: PathParam,
  execute: async (args) => {
    const db = new DiskDB();
    const abs = path.resolve(args.path);
    const row = db.db
      .query('SELECT size FROM entries WHERE path = ?')
      .get(abs) as { size: number } | undefined;
    if (!row) {
      return `Path ${args.path} not found`;
    }
    return String(row.size);
  },
});

server.addTool({
  name: 'disk-tree',
  description: 'Return a JSON tree of directories and file sizes',
  parameters: PathParam,
  execute: async (args) => {
    const db = new DiskDB();
    const abs = path.resolve(args.path);

    function buildTree(root: string): any {
      const entry = db.get(root);
      if (!entry) return null;
      const children = db
        .children(root)
        .map((c) => buildTree(c.path))
        .filter(Boolean);
      return { path: root, size: entry.size, children };
    }

    return buildTree(abs);
  },
});

const transportType = process.argv.includes('--http-stream') ? 'httpStream' : 'stdio';

if (transportType === 'httpStream') {
  const port = process.env.PORT ? parseInt(process.env.PORT, 10) : 8080;
  server.start({
    transportType: 'httpStream',
    httpStream: { port },
  });
  console.log(`HTTP Stream MCP server running at http://localhost:${port}/mcp`);
} else {
  server.start({ transportType: 'stdio' });
}
