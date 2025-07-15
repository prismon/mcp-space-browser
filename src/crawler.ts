import { promises as fs } from 'fs';
import * as path from 'path';
import { DiskDB } from './db';

export async function index(root: string, db: DiskDB) {
  const abs = path.resolve(root);
  const runId = Date.now();
  const stack: string[] = [abs];
  while (stack.length) {
    const current = stack.pop()!;
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
      const children = await fs.readdir(current);
      for (const c of children) {
        stack.push(path.join(current, c));
      }
    }
  }
  db.deleteStale(abs, runId);
  db.computeAggregates(abs);
}
