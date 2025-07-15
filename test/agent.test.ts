import { test, expect } from 'bun:test';
import { DiskDB } from '../src/db';
import { index } from '../src/crawler';
import { promises as fs } from 'fs';
import * as path from 'path';
import * as os from 'os';

async function withTempDir(fn: (dir: string) => Promise<void>) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'disk-test-'));
  try {
    await fn(dir);
  } finally {
    await fs.rm(dir, { recursive: true, force: true });
  }
}

test('index recurses and records entries', async () => {
  await withTempDir(async (dir) => {
    await fs.writeFile(path.join(dir, 'file1'), 'hello');
    await fs.mkdir(path.join(dir, 'sub'));
    await fs.writeFile(path.join(dir, 'sub', 'file2'), 'hi');

    const db = new DiskDB(':memory:');
    await index(dir, db);

    expect(db.get(path.join(dir, 'file1'))).toBeTruthy();
    expect(db.get(path.join(dir, 'sub', 'file2'))).toBeTruthy();

    const rootEntry = db.get(dir)!;
    expect(rootEntry.size).toBe(7);
  });
});

test('aggregated size updates when file deleted', async () => {
  await withTempDir(async (dir) => {
    const f1 = path.join(dir, 'a');
    const sub = path.join(dir, 'sub');
    const f2 = path.join(sub, 'b');

    await fs.writeFile(f1, 'aa');
    await fs.mkdir(sub);
    await fs.writeFile(f2, 'bbb');

    const db = new DiskDB(':memory:');
    await index(dir, db);
    expect(db.get(dir)!.size).toBe(5);

    await fs.rm(f2);
    await index(dir, db);
    expect(db.get(dir)!.size).toBe(2);
  });
});
