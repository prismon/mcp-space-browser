import { test, expect, describe } from 'bun:test';
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

// Mock the disk-tree tool execution logic
async function executeDiskTree(args: {
  path: string;
  groupBy?: 'extension' | 'none';
  sortBy?: 'size' | 'name' | 'mtime';
  descendingSort?: boolean;
  limit?: number;
  minSize?: number;
  maxDepth?: number;
  minDate?: string;
  maxDate?: string;
}, db: DiskDB): Promise<any> {
  const abs = path.resolve(args.path);
  const maxDepth = args.maxDepth ?? Infinity;
  const minSize = args.minSize ?? 0;
  const limit = args.limit ?? Infinity;
  const sortBy = args.sortBy ?? 'size';
  const descendingSort = args.descendingSort ?? (sortBy === 'size');
  const groupBy = args.groupBy ?? 'none';
  const minDate = args.minDate ? new Date(args.minDate).getTime() / 1000 : undefined;
  const maxDate = args.maxDate ? new Date(args.maxDate).getTime() / 1000 : undefined;
  
  if (groupBy === 'extension') {
    // Group by extension logic
    const extensionMap = new Map<string, { count: number; totalSize: number; files: string[] }>();
    
    // Recursive function to collect all files
    function collectFiles(root: string, depth: number = 0): void {
      if (depth > maxDepth) return;
      
      const entry = db.get(root);
      if (!entry) return;
      
      if (entry.kind === 'file' && entry.size >= minSize) {
        // Apply date filtering
        if (minDate && entry.mtime < minDate) return;
        if (maxDate && entry.mtime > maxDate) return;
        
        const ext = path.extname(entry.path).toLowerCase() || '[no extension]';
        
        if (!extensionMap.has(ext)) {
          extensionMap.set(ext, { count: 0, totalSize: 0, files: [] });
        }
        
        const extData = extensionMap.get(ext)!;
        extData.count++;
        extData.totalSize += entry.size;
        if (extData.files.length < 10) { // Keep sample files
          extData.files.push(entry.path);
        }
      } else if (entry.kind === 'directory') {
        const children = db.children(root);
        for (const child of children) {
          collectFiles(child.path, depth + 1);
        }
      }
    }
    
    collectFiles(abs);
    
    // Convert to sorted array
    const fileTypes = Array.from(extensionMap.entries())
      .map(([extension, data]) => ({
        extension,
        count: data.count,
        totalSize: data.totalSize,
        percentage: 0, // Will calculate after we know total
        sampleFiles: data.files
      }));
    
    // Calculate total size for percentages
    const totalSize = fileTypes.reduce((sum, ft) => sum + ft.totalSize, 0);
    
    // Add percentages and sort
    fileTypes.forEach(ft => {
      ft.percentage = totalSize > 0 ? Number(((ft.totalSize / totalSize) * 100).toFixed(1)) : 0;
    });
    
    // Sort by size or name
    fileTypes.sort((a, b) => {
      if (sortBy === 'size') {
        return descendingSort ? b.totalSize - a.totalSize : a.totalSize - b.totalSize;
      } else {
        const comparison = a.extension.localeCompare(b.extension);
        return descendingSort ? -comparison : comparison;
      }
    });
    
    // Apply limit
    const limitedFileTypes = limit === Infinity ? fileTypes : fileTypes.slice(0, limit);
    
    return {
      path: abs,
      totalSize,
      fileTypes: limitedFileTypes
    };
  } else {
    // Original tree logic
    let entryCount = 0;

    function buildTree(root: string, depth: number = 0): any {
      if (depth > maxDepth || entryCount >= limit) return null;
      
      const entry = db.get(root);
      if (!entry) return null;
      
      // Filter by minimum size
      if (entry.size < minSize) return null;
      
      entryCount++;
      
      let children: any[] = [];
      if (depth < maxDepth && entryCount < limit) {
        const childEntries = db.children(root);
        
        // Sort children
        childEntries.sort((a, b) => {
          if (sortBy === 'size') {
            return descendingSort ? b.size - a.size : a.size - b.size;
          } else {
            const comparison = a.path.localeCompare(b.path);
            return descendingSort ? -comparison : comparison;
          }
        });
        
        // Build tree for children
        for (const child of childEntries) {
          if (entryCount >= limit) break;
          const childTree = buildTree(child.path, depth + 1);
          if (childTree) children.push(childTree);
        }
      }
      
      return { path: root, size: entry.size, children };
    }

    return buildTree(abs);
  }
}

test('disk-tree groupBy extension shows file types', async () => {
  await withTempDir(async (dir) => {
    // Create various file types
    await fs.writeFile(path.join(dir, 'file1.js'), 'a'.repeat(100));
    await fs.writeFile(path.join(dir, 'file2.js'), 'b'.repeat(150));
    await fs.writeFile(path.join(dir, 'doc.md'), 'c'.repeat(50));
    await fs.writeFile(path.join(dir, 'config.json'), 'd'.repeat(30));
    await fs.writeFile(path.join(dir, 'noext'), 'e'.repeat(20));
    
    await fs.mkdir(path.join(dir, 'sub'));
    await fs.writeFile(path.join(dir, 'sub', 'file3.js'), 'f'.repeat(80));
    await fs.writeFile(path.join(dir, 'sub', 'readme.md'), 'g'.repeat(40));

    const db = new DiskDB(':memory:');
    await index(dir, db);

    const result = await executeDiskTree({ path: dir, groupBy: 'extension' }, db);
    
    expect(result.fileTypes).toBeDefined();
    expect(result.totalSize).toBe(470);
    
    // Check that extensions are grouped correctly
    const jsType = result.fileTypes.find((ft: any) => ft.extension === '.js');
    expect(jsType).toBeDefined();
    expect(jsType.count).toBe(3);
    expect(jsType.totalSize).toBe(330);
    expect(jsType.percentage).toBe(70.2);
    
    const mdType = result.fileTypes.find((ft: any) => ft.extension === '.md');
    expect(mdType).toBeDefined();
    expect(mdType.count).toBe(2);
    expect(mdType.totalSize).toBe(90);
    
    const noExtType = result.fileTypes.find((ft: any) => ft.extension === '[no extension]');
    expect(noExtType).toBeDefined();
    expect(noExtType.count).toBe(1);
    expect(noExtType.totalSize).toBe(20);
  });
});

test('disk-tree groupBy extension respects sorting', async () => {
  await withTempDir(async (dir) => {
    await fs.writeFile(path.join(dir, 'small.txt'), 'a'.repeat(10));
    await fs.writeFile(path.join(dir, 'big.js'), 'b'.repeat(100));
    await fs.writeFile(path.join(dir, 'medium.md'), 'c'.repeat(50));

    const db = new DiskDB(':memory:');
    await index(dir, db);

    // Test size sorting (descending by default)
    const sizeResult = await executeDiskTree({ 
      path: dir, 
      groupBy: 'extension',
      sortBy: 'size'
    }, db);
    
    expect(sizeResult.fileTypes[0].extension).toBe('.js');
    expect(sizeResult.fileTypes[1].extension).toBe('.md');
    expect(sizeResult.fileTypes[2].extension).toBe('.txt');
    
    // Test name sorting
    const nameResult = await executeDiskTree({ 
      path: dir, 
      groupBy: 'extension',
      sortBy: 'name',
      descendingSort: false
    }, db);
    
    expect(nameResult.fileTypes[0].extension).toBe('.js');
    expect(nameResult.fileTypes[1].extension).toBe('.md');
    expect(nameResult.fileTypes[2].extension).toBe('.txt');
  });
});

test('disk-tree groupBy extension respects limit', async () => {
  await withTempDir(async (dir) => {
    await fs.writeFile(path.join(dir, 'a.txt'), 'a'.repeat(10));
    await fs.writeFile(path.join(dir, 'b.js'), 'b'.repeat(20));
    await fs.writeFile(path.join(dir, 'c.md'), 'c'.repeat(30));
    await fs.writeFile(path.join(dir, 'd.json'), 'd'.repeat(40));

    const db = new DiskDB(':memory:');
    await index(dir, db);

    const result = await executeDiskTree({ 
      path: dir, 
      groupBy: 'extension',
      limit: 2
    }, db);
    
    expect(result.fileTypes.length).toBe(2);
    // Should get the two largest
    expect(result.fileTypes[0].extension).toBe('.json');
    expect(result.fileTypes[1].extension).toBe('.md');
  });
});

test('disk-tree groupBy extension respects minSize', async () => {
  await withTempDir(async (dir) => {
    await fs.writeFile(path.join(dir, 'tiny.txt'), 'a'.repeat(5));
    await fs.writeFile(path.join(dir, 'small.js'), 'b'.repeat(15));
    await fs.writeFile(path.join(dir, 'big.md'), 'c'.repeat(50));

    const db = new DiskDB(':memory:');
    await index(dir, db);

    const result = await executeDiskTree({ 
      path: dir, 
      groupBy: 'extension',
      minSize: 10
    }, db);
    
    // Should only include files >= 10 bytes
    expect(result.fileTypes.length).toBe(2);
    expect(result.fileTypes.find((ft: any) => ft.extension === '.txt')).toBeUndefined();
    expect(result.fileTypes.find((ft: any) => ft.extension === '.js')).toBeDefined();
    expect(result.fileTypes.find((ft: any) => ft.extension === '.md')).toBeDefined();
  });
});

test('disk-tree groupBy none works as before', async () => {
  await withTempDir(async (dir) => {
    await fs.writeFile(path.join(dir, 'file1.txt'), 'hello');
    await fs.mkdir(path.join(dir, 'sub'));
    await fs.writeFile(path.join(dir, 'sub', 'file2.txt'), 'world');

    const db = new DiskDB(':memory:');
    await index(dir, db);

    const result = await executeDiskTree({ path: dir, groupBy: 'none' }, db);
    
    expect(result.path).toBe(dir);
    expect(result.size).toBe(10);
    expect(result.children).toBeDefined();
    expect(result.children.length).toBeGreaterThan(0);
  });
});

describe('Date filtering tests', () => {
  test('disk-tree respects date filtering', async () => {
    await withTempDir(async (dir) => {
      // Create files with specific modification times
      const oldFile = path.join(dir, 'old.txt');
      const newFile = path.join(dir, 'new.txt');
      
      await fs.writeFile(oldFile, 'old content');
      await fs.writeFile(newFile, 'new content');
      
      // Set modification times using utimes
      const oldTime = new Date('2025-01-01').getTime();
      const newTime = new Date('2025-07-01').getTime();
      
      await fs.utimes(oldFile, oldTime / 1000, oldTime / 1000);
      await fs.utimes(newFile, newTime / 1000, newTime / 1000);
      
      const db = new DiskDB(':memory:');
      await index(dir, db);
      
      // Test max date filtering
      const beforeJuneResult = await executeDiskTree({ 
        path: dir, 
        groupBy: 'extension',
        maxDate: '2025-06-01'
      }, db);
      
      expect(beforeJuneResult.fileTypes.length).toBe(1);
      expect(beforeJuneResult.fileTypes[0].extension).toBe('.txt');
      expect(beforeJuneResult.fileTypes[0].count).toBe(1);
      
      // Test min date filtering
      const afterJuneResult = await executeDiskTree({ 
        path: dir, 
        groupBy: 'extension',
        minDate: '2025-06-01'
      }, db);
      
      expect(afterJuneResult.fileTypes.length).toBe(1);
      expect(afterJuneResult.fileTypes[0].extension).toBe('.txt');
      expect(afterJuneResult.fileTypes[0].count).toBe(1);
    });
  });
});