import { FastMCP } from 'fastmcp';
import { z } from 'zod';
import { DiskDB } from './db';
import { index as indexFs } from './crawler';
import * as path from 'path';

const server = new FastMCP({
  name: 'mcp-space-browser',
  version: '0.1.0',
  instructions: 'File system browser with saved queries and selection sets. Use query-execute to run saved queries like "all_log_files". Use query-list to see available queries.'
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
    // Use session's working directory if available
    const abs = path.resolve(args.path);
    
    // Log query to session history
    if (context.session?.userId) {
      const state = sessionStates.get(context.session.userId);
      if (state) {
        state.history.push(`disk-du: ${args.path}`);
        if (state.history.length > 50) state.history.shift();
      }
    }
    
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
    const limit = args.limit ?? context.session?.preferences?.maxResults ?? Infinity;
    const sortBy = args.sortBy ?? context.session?.preferences?.sortBy ?? 'size';
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

// SelectionSet Management Tools

const CreateSelectionSetParam = z.object({
  name: z.string().describe('Name of the selection set'),
  description: z.string().optional().describe('Description of the selection set'),
  fromTool: z.object({
    tool: z.string().describe('Tool name to execute for getting files'),
    params: z.record(z.any()).describe('Parameters to pass to the tool'),
    limit: z.number().optional().describe('Maximum number of files to add (default: 100)')
  }).optional().describe('Create set from tool results')
});

server.addTool({
  name: 'selection-set-create',
  description: 'Create a new selection set, optionally populated from tool results',
  parameters: CreateSelectionSetParam,
  execute: async (args) => {
    const db = new DiskDB();
    
    // Track session's current selection set
    if (context.session?.userId) {
      const state = sessionStates.get(context.session.userId);
      if (state) {
        state.currentSelectionSet = args.name;
      }
    }
    
    // Create the selection set
    const setId = db.createSelectionSet({
      name: args.name,
      description: args.description,
      criteria_type: args.fromTool ? 'tool_query' : 'user_selected',
      criteria_json: args.fromTool ? JSON.stringify(args.fromTool) : undefined
    });
    
    let addedCount = 0;
    
    // If fromTool is specified, execute the tool and add results
    if (args.fromTool) {
      const limit = args.fromTool.limit || 100;
      
      if (args.fromTool.tool === 'disk-time-range') {
        // Special handling for disk-time-range
        const count = args.fromTool.params.count || 10;
        const workingDir = context.session?.workingDirectory || process.cwd();
        const abs = path.resolve(workingDir, args.fromTool.params.path || '.');
        const minSize = args.fromTool.params.minSize || 0;
        
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
        allFiles.sort((a, b) => a.mtime - b.mtime);
        
        // Get oldest files (or newest if specified)
        const files = args.fromTool.params.newest 
          ? allFiles.slice(-Math.min(count, limit)).reverse()
          : allFiles.slice(0, Math.min(count, limit));
        
        const paths = files.map(f => f.path);
        if (paths.length > 0) {
          db.addToSelectionSet(args.name, paths);
          addedCount = paths.length;
        }
      } else if (args.fromTool.tool === 'disk-tree') {
        // Handle disk-tree results
        const workingDir = context.session?.workingDirectory || process.cwd();
        const abs = path.resolve(workingDir, args.fromTool.params.path || '.');
        const sortBy = args.fromTool.params.sortBy || context.session?.preferences?.sortBy || 'size';
        const descendingSort = args.fromTool.params.descendingSort ?? (sortBy === 'size');
        const minSize = args.fromTool.params.minSize || 0;
        
        const allEntries: Array<{path: string, size: number, mtime: number}> = [];
        
        function collectEntries(root: string, depth: number = 0): void {
          const entry = db.get(root);
          if (!entry || entry.size < minSize) return;
          
          allEntries.push({path: root, size: entry.size, mtime: entry.mtime});
          
          if (entry.kind === 'directory') {
            const children = db.children(root);
            for (const child of children) {
              collectEntries(child.path, depth + 1);
            }
          }
        }
        
        collectEntries(abs);
        
        // Sort entries
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
        
        const paths = allEntries.slice(0, limit).map(e => e.path);
        if (paths.length > 0) {
          db.addToSelectionSet(args.name, paths);
          addedCount = paths.length;
        }
      }
    }
    
    return JSON.stringify({
      name: args.name,
      created: true,
      entriesAdded: addedCount
    }, null, 2);
  },
});

const SelectionSetParam = z.object({
  name: z.string().describe('Name of the selection set')
});

server.addTool({
  name: 'selection-set-list',
  description: 'List all selection sets',
  parameters: z.object({}),
  execute: async (args) => {
    const db = new DiskDB();
    const sets = db.listSelectionSets();
    
    const setsWithStats = sets.map(set => {
      const stats = db.getSelectionSetStats(set.name);
      return {
        name: set.name,
        description: set.description,
        type: set.criteria_type,
        criteria: set.criteria_json ? JSON.parse(set.criteria_json) : null,
        fileCount: stats.count,
        totalSize: stats.totalSize,
        created: new Date(set.created_at! * 1000).toISOString(),
        updated: new Date(set.updated_at! * 1000).toISOString()
      };
    });
    
    return JSON.stringify(setsWithStats, null, 2);
  },
});

server.addTool({
  name: 'selection-set-get',
  description: 'Get all files in a named selection set (e.g., "log_files") to see what files are included',
  parameters: SelectionSetParam,
  execute: async (args) => {
    const db = new DiskDB();
    const entries = db.getSelectionSetEntries(args.name);
    const stats = db.getSelectionSetStats(args.name);
    
    const files = entries.map(e => ({
      path: e.path,
      size: e.size,
      kind: e.kind,
      mtime: new Date(e.mtime * 1000).toISOString()
    }));
    
    return JSON.stringify({
      name: args.name,
      stats: stats,
      files: files
    }, null, 2);
  },
});

const ModifySelectionSetParam = z.object({
  name: z.string().describe('Name of the selection set'),
  paths: z.array(z.string()).describe('Paths to add or remove'),
  operation: z.enum(['add', 'remove']).describe('Operation to perform')
});

server.addTool({
  name: 'selection-set-modify',
  description: 'Add or remove paths from a selection set',
  parameters: ModifySelectionSetParam,
  execute: async (args) => {
    const db = new DiskDB();
    
    if (args.operation === 'add') {
      db.addToSelectionSet(args.name, args.paths);
    } else {
      db.removeFromSelectionSet(args.name, args.paths);
    }
    
    const stats = db.getSelectionSetStats(args.name);
    
    return JSON.stringify({
      name: args.name,
      operation: args.operation,
      pathsModified: args.paths.length,
      newStats: stats
    }, null, 2);
  },
});

server.addTool({
  name: 'selection-set-delete',
  description: 'Delete a selection set',
  parameters: SelectionSetParam,
  execute: async (args) => {
    const db = new DiskDB();
    db.deleteSelectionSet(args.name);
    
    return JSON.stringify({
      name: args.name,
      deleted: true
    }, null, 2);
  },
});

// Session-specific tools

server.addTool({
  name: 'session-info',
  description: 'Get information about the current session',
  parameters: z.object({}),
  execute: async (args) => {
    const userId = context.session?.userId || 'anonymous';
    const state = sessionStates.get(userId);
    
    return JSON.stringify({
      userId,
      workingDirectory: context.session?.workingDirectory || process.cwd(),
      preferences: context.session?.preferences,
      currentSelectionSet: state?.currentSelectionSet,
      historyCount: state?.history.length || 0,
      recentHistory: state?.history.slice(-10) || []
    }, null, 2);
  },
});

server.addTool({
  name: 'session-set-preferences',
  description: 'Update session preferences',
  parameters: z.object({
    maxResults: z.number().optional().describe('Maximum results to return'),
    sortBy: z.enum(['size', 'name', 'mtime']).optional().describe('Default sort order')
  }),
  execute: async (args) => {
    if (!context.session) {
      return 'No active session';
    }
    
    if (!context.session.preferences) {
      context.session.preferences = {};
    }
    
    if (args.maxResults !== undefined) {
      context.session.preferences.maxResults = args.maxResults;
    }
    if (args.sortBy !== undefined) {
      context.session.preferences.sortBy = args.sortBy;
    }
    
    return JSON.stringify({
      updated: true,
      preferences: context.session.preferences
    }, null, 2);
  },
});

// Query Management Tools

const CreateQueryParam = z.object({
  name: z.string().describe('Name of the query'),
  description: z.string().optional().describe('Description of the query'),
  filter: z.object({
    path: z.string().optional().describe('Root path to search from'),
    pattern: z.string().optional().describe('Regex pattern to match file paths'),
    extensions: z.array(z.string()).optional().describe('File extensions to include (e.g., ["ts", "js"])'),
    minSize: z.number().optional().describe('Minimum file size in bytes'),
    maxSize: z.number().optional().describe('Maximum file size in bytes'),
    minDate: z.string().optional().describe('Minimum modification date (YYYY-MM-DD)'),
    maxDate: z.string().optional().describe('Maximum modification date (YYYY-MM-DD)'),
    nameContains: z.string().optional().describe('File name must contain this string'),
    pathContains: z.string().optional().describe('File path must contain this string'),
    sortBy: z.enum(['size', 'name', 'mtime']).optional().describe('Sort results by'),
    descendingSort: z.boolean().optional().describe('Sort in descending order'),
    limit: z.number().optional().describe('Maximum number of results')
  }).describe('File filter criteria'),
  targetSelectionSet: z.string().optional().describe('Selection set to update with results'),
  updateMode: z.enum(['replace', 'append', 'merge']).optional().describe('How to update the selection set (default: replace)')
});

server.addTool({
  name: 'query-create',
  description: 'Create a persistent query that can be executed on demand',
  parameters: CreateQueryParam,
  execute: async (args) => {
    const db = new DiskDB();
    
    const queryId = db.createQuery({
      name: args.name,
      description: args.description,
      query_type: 'file_filter',
      query_json: JSON.stringify(args.filter),
      target_selection_set: args.targetSelectionSet,
      update_mode: args.updateMode || 'replace'
    });
    
    return JSON.stringify({
      name: args.name,
      created: true,
      queryId
    }, null, 2);
  },
});

server.addTool({
  name: 'query-execute',
  description: 'Execute a saved file search query by name (e.g., "all_log_files") to find matching files and update the associated selection set',
  parameters: z.object({
    name: z.string().describe('Name of the saved query to execute (e.g., "all_log_files", "recent_files", "source_code")')
  }),
  execute: async (args) => {
    const db = new DiskDB();
    
    try {
      const result = db.executeQuery(args.name);
      const query = db.getQuery(args.name);
      
      // Track in session history
      if (context.session?.userId) {
        const state = sessionStates.get(context.session.userId);
        if (state) {
          state.history.push(`query-execute: ${args.name}`);
          if (state.history.length > 50) state.history.shift();
        }
      }
      
      return JSON.stringify({
        query: args.name,
        executed: true,
        filesMatched: result.filesMatched,
        selectionSet: result.selectionSet,
        executionCount: query?.execution_count || 0
      }, null, 2);
    } catch (error) {
      return JSON.stringify({
        query: args.name,
        executed: false,
        error: error instanceof Error ? error.message : String(error)
      }, null, 2);
    }
  },
});

server.addTool({
  name: 'query-list',
  description: 'List all saved file search queries that can be executed (shows available query names like "all_log_files")',
  parameters: z.object({}),
  execute: async (args) => {
    const db = new DiskDB();
    const queries = db.listQueries();
    
    const queriesWithInfo = queries.map(q => ({
      name: q.name,
      description: q.description,
      type: q.query_type,
      targetSelectionSet: q.target_selection_set,
      updateMode: q.update_mode,
      executionCount: q.execution_count || 0,
      lastExecuted: q.last_executed ? new Date(q.last_executed * 1000).toISOString() : null,
      created: new Date(q.created_at! * 1000).toISOString()
    }));
    
    return JSON.stringify(queriesWithInfo, null, 2);
  },
});

server.addTool({
  name: 'query-get',
  description: 'Get details of a specific query',
  parameters: z.object({
    name: z.string().describe('Name of the query')
  }),
  execute: async (args) => {
    const db = new DiskDB();
    const query = db.getQuery(args.name);
    
    if (!query) {
      return `Query '${args.name}' not found`;
    }
    
    const executions = db.getQueryExecutions(args.name, 5);
    
    return JSON.stringify({
      name: query.name,
      description: query.description,
      type: query.query_type,
      filter: JSON.parse(query.query_json),
      targetSelectionSet: query.target_selection_set,
      updateMode: query.update_mode,
      executionCount: query.execution_count || 0,
      lastExecuted: query.last_executed ? new Date(query.last_executed * 1000).toISOString() : null,
      created: new Date(query.created_at! * 1000).toISOString(),
      recentExecutions: executions.map(e => ({
        executedAt: new Date(e.executed_at! * 1000).toISOString(),
        duration: e.duration_ms,
        filesMatched: e.files_matched,
        status: e.status,
        error: e.error_message
      }))
    }, null, 2);
  },
});

server.addTool({
  name: 'query-update',
  description: 'Update an existing query',
  parameters: z.object({
    name: z.string().describe('Name of the query to update'),
    description: z.string().optional().describe('New description'),
    filter: z.object({
      path: z.string().optional(),
      pattern: z.string().optional(),
      extensions: z.array(z.string()).optional(),
      minSize: z.number().optional(),
      maxSize: z.number().optional(),
      minDate: z.string().optional(),
      maxDate: z.string().optional(),
      nameContains: z.string().optional(),
      pathContains: z.string().optional(),
      sortBy: z.enum(['size', 'name', 'mtime']).optional(),
      descendingSort: z.boolean().optional(),
      limit: z.number().optional()
    }).optional().describe('New filter criteria'),
    targetSelectionSet: z.string().optional().describe('New target selection set'),
    updateMode: z.enum(['replace', 'append', 'merge']).optional().describe('New update mode')
  }),
  execute: async (args) => {
    const db = new DiskDB();
    
    const updates: any = {};
    if (args.description !== undefined) updates.description = args.description;
    if (args.filter !== undefined) updates.query_json = JSON.stringify(args.filter);
    if (args.targetSelectionSet !== undefined) updates.target_selection_set = args.targetSelectionSet;
    if (args.updateMode !== undefined) updates.update_mode = args.updateMode;
    
    db.updateQuery(args.name, updates);
    
    return JSON.stringify({
      name: args.name,
      updated: true
    }, null, 2);
  },
});

server.addTool({
  name: 'query-delete',
  description: 'Delete a saved query',
  parameters: z.object({
    name: z.string().describe('Name of the query to delete')
  }),
  execute: async (args) => {
    const db = new DiskDB();
    db.deleteQuery(args.name);
    
    return JSON.stringify({
      name: args.name,
      deleted: true
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
