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
  execute: async (args, context) => {
    const db = new DiskDB();
    await indexFs(args.path, db);
    return 'OK';
  },
});

server.addTool({
  name: 'disk-du',
  description: 'Get disk usage for a path',
  parameters: PathParam,
  execute: async (args, context) => {
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

const port = 8080;
const host = '0.0.0.0';
server.start({
  transportType: 'httpStream',
  httpStream: { port, host, stateless: true },
});
console.log(`HTTP Stream MCP server running at http://${host}:${port}/mcp`);
