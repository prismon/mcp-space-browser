import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';

async function diskIndex(target: string) {
  const db = new DiskDB();
  await indexFs(target, db);
}

function diskDu(target: string) {
  const db = new DiskDB();
  const abs = path.resolve(target);
  const row = db.db
    .query('SELECT size FROM entries WHERE path = ?')
    .get(abs) as { size: number } | undefined;
  console.log(row?.size ?? 0);
}

function diskTree(target: string, indent = '') {
  const db = new DiskDB();
  const abs = path.resolve(target);
  const entry = db.get(abs);
  if (!entry) return;
  console.log(`${indent}${path.basename(abs)} (${entry.size})`);
  const children = db.children(abs);
  for (const child of children) {
    diskTree(child.path, indent + '  ');
  }
}

async function main() {
  const [cmd, arg] = process.argv.slice(2);
  if (!cmd || !arg) {
    console.log('Usage: disk-index|disk-du|disk-tree <path>');
    process.exit(1);
  }
  if (cmd === 'disk-index') {
    await diskIndex(arg);
  } else if (cmd === 'disk-du') {
    diskDu(arg);
  } else if (cmd === 'disk-tree') {
    diskTree(arg);
  } else {
    console.log('Unknown command');
    process.exit(1);
  }
}

main();
