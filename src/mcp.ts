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

const TreeParam = z.object({
  path: z.string().describe('File or directory path'),
  maxDepth: z.number().optional().describe('Maximum depth to traverse (default: unlimited)'),
  minSize: z.number().optional().describe('Minimum file size to include in bytes (default: 0)'),
  limit: z.number().optional().describe('Maximum number of entries to return (default: unlimited)'),
  sortBy: z.enum(['size', 'name', 'mtime']).optional().describe('Sort entries by size, name, or modification time (default: size)'),
  descendingSort: z.boolean().optional().describe('Sort in descending order (default: true for size, false for name)'),
  groupBy: z.enum(['extension', 'none']).optional().describe('Group files by extension or show tree structure (default: none)'),
  minDate: z.string().optional().describe('Filter files modified after this date (YYYY-MM-DD)'),
  maxDate: z.string().optional().describe('Filter files modified before this date (YYYY-MM-DD)'),
  offset: z.number().optional().describe('Number of entries to skip for pagination (default: 0)'),
  pageSize: z.number().optional().describe('Maximum number of entries per page (default: 1000)'),
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
  parameters: TreeParam,
  execute: async (args) => {
    const db = new DiskDB();
    const abs = path.resolve(args.path);
    const maxDepth = args.maxDepth ?? Infinity;
    const minSize = args.minSize ?? 0;
    const limit = args.limit ?? Infinity;
    const sortBy = args.sortBy ?? 'size';
    const descendingSort = args.descendingSort ?? (sortBy === 'size');
    const groupBy = args.groupBy ?? 'none';
    const minDate = args.minDate ? new Date(args.minDate).getTime() / 1000 : undefined;
    const maxDate = args.maxDate ? new Date(args.maxDate).getTime() / 1000 : undefined;
    const offset = args.offset ?? 0;
    const pageSize = args.pageSize ?? 1000;
    
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
      
      // Sort by size, name, or mtime (for extension groups, mtime doesn't apply)
      fileTypes.sort((a, b) => {
        if (sortBy === 'size' || sortBy === 'mtime') {
          return descendingSort ? b.totalSize - a.totalSize : a.totalSize - b.totalSize;
        } else {
          const comparison = a.extension.localeCompare(b.extension);
          return descendingSort ? -comparison : comparison;
        }
      });
      
      // Apply limit
      const limitedFileTypes = limit === Infinity ? fileTypes : fileTypes.slice(0, limit);
      
      const result = {
        path: abs,
        totalSize,
        fileTypes: limitedFileTypes
      };
      
      return JSON.stringify(result, null, 2);
    } else {
      // For tree mode, collect all matching entries first, then paginate
      const allEntries: Array<{path: string, size: number, mtime: number, depth: number}> = [];
      
      function collectEntries(root: string, depth: number = 0): void {
        if (depth > maxDepth) return;
        
        const entry = db.get(root);
        if (!entry) return;
        
        // Apply filters
        if (entry.size < minSize) return;
        if (minDate && entry.mtime < minDate) return;
        if (maxDate && entry.mtime > maxDate) return;
        
        allEntries.push({path: root, size: entry.size, mtime: entry.mtime, depth});
        
        if (depth < maxDepth && entry.kind === 'directory') {
          const childEntries = db.children(root);
          for (const child of childEntries) {
            collectEntries(child.path, depth + 1);
          }
        }
      }
      
      collectEntries(abs);
      
      // Sort all entries
      allEntries.sort((a, b) => {
        if (sortBy === 'size') {
          return descendingSort ? b.size - a.size : a.size - b.size;
        } else if (sortBy === 'mtime') {
          return descendingSort ? b.mtime - a.mtime : a.mtime - b.mtime;
        } else {
          const comparison = a.path.localeCompare(b.path);
          return descendingSort ? -comparison : comparison;
        }
      });
      
      // Apply limit if specified
      const limitedEntries = limit === Infinity ? allEntries : allEntries.slice(0, limit);
      
      // Apply pagination
      const paginatedEntries = limitedEntries.slice(offset, offset + pageSize);
      
      // Build result with pagination info
      const result = {
        pagination: {
          total: limitedEntries.length,
          offset: offset,
          pageSize: pageSize,
          hasMore: offset + pageSize < limitedEntries.length
        },
        entries: paginatedEntries.map(e => ({
          path: e.path,
          size: e.size,
          mtime: new Date(e.mtime * 1000).toISOString(),
          depth: e.depth
        }))
      };
      
      return JSON.stringify(result, null, 2);
    }
  },
});

const TimeRangeParam = z.object({
  path: z.string().describe('File or directory path'),
  count: z.number().default(10).describe('Number of oldest and newest files to return (default: 10)'),
  minSize: z.number().optional().describe('Minimum file size to include in bytes (default: 0)'),
});

server.addTool({
  name: 'disk-time-range',
  description: 'Get the oldest and newest files in a directory tree',
  parameters: TimeRangeParam,
  execute: async (args) => {
    const db = new DiskDB();
    const abs = path.resolve(args.path);
    const count = args.count ?? 10;
    const minSize = args.minSize ?? 0;
    
    // Collect all files recursively
    const allFiles: Array<{path: string, size: number, mtime: number}> = [];
    
    function collectFiles(root: string): void {
      const entry = db.get(root);
      if (!entry) return;
      
      if (entry.kind === 'file' && entry.size >= minSize) {
        allFiles.push({path: root, size: entry.size, mtime: entry.mtime});
      } else if (entry.kind === 'directory') {
        const children = db.children(root);
        for (const child of children) {
          collectFiles(child.path);
        }
      }
    }
    
    collectFiles(abs);
    
    if (allFiles.length === 0) {
      return JSON.stringify({
        message: 'No files found',
        oldest: [],
        newest: []
      }, null, 2);
    }
    
    // Sort by modification time
    allFiles.sort((a, b) => a.mtime - b.mtime);
    
    // Get oldest and newest
    const oldest = allFiles.slice(0, count).map(f => ({
      path: f.path,
      size: f.size,
      mtime: new Date(f.mtime * 1000).toISOString(),
      age: formatAge(Date.now() / 1000 - f.mtime)
    }));
    
    const newest = allFiles.slice(-count).reverse().map(f => ({
      path: f.path,
      size: f.size,
      mtime: new Date(f.mtime * 1000).toISOString(),
      age: formatAge(Date.now() / 1000 - f.mtime)
    }));
    
    return JSON.stringify({
      totalFiles: allFiles.length,
      oldest,
      newest
    }, null, 2);
  },
});

function formatAge(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  
  if (days > 365) {
    const years = Math.floor(days / 365);
    return `${years} year${years > 1 ? 's' : ''} ago`;
  } else if (days > 30) {
    const months = Math.floor(days / 30);
    return `${months} month${months > 1 ? 's' : ''} ago`;
  } else if (days > 0) {
    return `${days} day${days > 1 ? 's' : ''} ago`;
  } else if (hours > 0) {
    return `${hours} hour${hours > 1 ? 's' : ''} ago`;
  } else if (minutes > 0) {
    return `${minutes} minute${minutes > 1 ? 's' : ''} ago`;
  } else {
    return 'just now';
  }
}

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
