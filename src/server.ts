import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';

const db = new DiskDB();

const server = Bun.serve({
  port: 3000,
  fetch(req, server) {
    const url = new URL(req.url);
    if (url.pathname === '/api/index') {
      const p = url.searchParams.get('path');
      if (!p) return new Response('path required', { status: 400 });
      indexFs(p, db).then(() => console.log('indexed', p));
      return new Response('OK');
    } else if (url.pathname === '/api/tree') {
      const p = url.searchParams.get('path') || '.';
      const abs = path.resolve(p);
      function buildTree(root: string): any {
        const entry = db.get(root);
        if (!entry) return null;
        const children = db.children(root).map((c) => buildTree(c.path));
        return { path: root, size: entry.size, children: children.filter(Boolean) };
      }
      return Response.json(buildTree(abs));
    }
    return new Response('Not found', { status: 404 });
  },
});

console.log('server running on', server.port);
