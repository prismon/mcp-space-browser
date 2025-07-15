import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';
import { createChildLogger } from './logger';

const logger = createChildLogger('server');

const db = new DiskDB();

const server = Bun.serve({
  port: 3000,
  fetch(req, server) {
    const startTime = Date.now();
    const url = new URL(req.url);
    const method = req.method;
    logger.info({ method, path: url.pathname, query: url.search }, 'Incoming request');
    
    if (url.pathname === '/api/index') {
      const p = url.searchParams.get('path');
      if (!p) {
        logger.warn({ path: url.pathname }, 'Missing path parameter');
        return new Response('path required', { status: 400 });
      }
      logger.info({ path: p }, 'Starting filesystem index via API');
      indexFs(p, db)
        .then(() => {
          const duration = Date.now() - startTime;
          logger.info({ path: p, duration }, 'Filesystem index completed via API');
        })
        .catch((error) => {
          logger.error({ path: p, error }, 'Filesystem index failed via API');
        });
      return new Response('OK');
    } else if (url.pathname === '/api/tree') {
      const p = url.searchParams.get('path') || '.';
      const abs = path.resolve(p);
      logger.debug({ path: abs }, 'Building tree structure');
      
      function buildTree(root: string, depth = 0): any {
        const entry = db.get(root);
        if (!entry) {
          logger.trace({ path: root }, 'Entry not found while building tree');
          return null;
        }
        const children = db.children(root).map((c) => buildTree(c.path, depth + 1));
        return { path: root, size: entry.size, children: children.filter(Boolean) };
      }
      
      const tree = buildTree(abs);
      const duration = Date.now() - startTime;
      logger.info({ path: abs, duration }, 'Tree structure built successfully');
      return Response.json(tree);
    }
    
    logger.warn({ path: url.pathname }, 'Route not found');
    return new Response('Not found', { status: 404 });
  },
});

logger.info({ port: server.port }, 'HTTP server started');
