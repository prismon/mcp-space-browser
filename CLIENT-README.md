# MCP Space Browser - JavaScript Client Library

A comprehensive JavaScript/TypeScript client library for integrating with the MCP Space Browser server. This library provides easy-to-use methods for all 21 MCP tools, enabling frontend applications to interact with disk space indexing, navigation, queries, and file management features.

## Features

✨ **Complete API Coverage** - All 21 MCP tools are supported
🎯 **TypeScript Support** - Full TypeScript definitions included
🌐 **Universal** - Works in Node.js and browser environments
📦 **Zero Dependencies** - Pure JavaScript, no external dependencies
⚡ **Async/Await** - Modern Promise-based API
🛡️ **Error Handling** - Comprehensive error handling with timeouts
📝 **Well Documented** - JSDoc comments for IDE autocomplete

## Installation

### Browser (CDN)

```html
<script src="mcp-client.js"></script>
<script>
  const client = new MCPSpaceBrowserClient('http://localhost:3000');
</script>
```

### Node.js (CommonJS)

```javascript
const MCPSpaceBrowserClient = require('./mcp-client.js');
const client = new MCPSpaceBrowserClient('http://localhost:3000');
```

### ES Modules

```javascript
import MCPSpaceBrowserClient from './mcp-client.js';
const client = new MCPSpaceBrowserClient('http://localhost:3000');
```

### TypeScript

```typescript
import MCPSpaceBrowserClient from './mcp-client';
// TypeScript definitions are automatically loaded from mcp-client.d.ts

const client = new MCPSpaceBrowserClient('http://localhost:3000');
```

## Quick Start

```javascript
// Create a client instance
const client = new MCPSpaceBrowserClient('http://localhost:3000');

// Check server health
const healthy = await client.healthCheck();
console.log('Server healthy:', healthy);

// Index a directory (async mode)
const indexResult = await client.index('/home/user/projects', { async: true });
console.log('Job ID:', indexResult.jobId);

// Monitor job progress
const status = await client.getJobProgress(indexResult.jobId);
console.log('Progress:', status.progress + '%');

// Navigate a directory
const listing = await client.navigate('/home/user/projects', {
  limit: 50,
  sortBy: 'size',
  order: 'desc'
});

console.log('Found', listing.count, 'entries');
listing.entries.forEach(entry => {
  console.log(entry.name, '-', entry.size, 'bytes');
});
```

## API Documentation

### Constructor

```javascript
const client = new MCPSpaceBrowserClient(baseUrl, options);
```

**Parameters:**
- `baseUrl` (string): Base URL of the MCP server (e.g., 'http://localhost:3000')
- `options` (object, optional):
  - `timeout` (number): Request timeout in milliseconds (default: 30000)
  - `headers` (object): Additional headers to include in requests

### Core Tools

#### `index(root, options)`

Index a directory tree.

```javascript
// Async mode (recommended for large directories)
const result = await client.index('/home/user/projects', { async: true });
console.log('Job ID:', result.jobId);

// Sync mode (for small directories)
const result = await client.index('/home/user/small-dir', { async: false });
console.log('Files:', result.files, 'Dirs:', result.directories);
```

**Parameters:**
- `root` (string): Path to index
- `options` (object):
  - `async` (boolean): Run asynchronously (default: true)

**Returns:** Promise<IndexAsyncResult | IndexSyncResult>

#### `navigate(path, options)`

Navigate to a directory and list its contents.

```javascript
const listing = await client.navigate('/home/user', {
  limit: 50,
  offset: 0,
  sortBy: 'size',
  order: 'desc'
});

console.log('Entries:', listing.entries);
console.log('Summary:', listing.summary);
```

**Parameters:**
- `path` (string): Directory path
- `options` (object):
  - `limit` (number): Maximum entries to return (default: 20)
  - `offset` (number): Pagination offset
  - `sortBy` (string): Sort by 'size', 'name', or 'mtime' (default: 'size')
  - `order` (string): Sort order 'asc' or 'desc' (default: 'desc')

**Returns:** Promise<NavigationResult>

#### `inspect(path, options)`

Inspect a specific file or directory.

```javascript
const details = await client.inspect('/home/user/photo.jpg', {
  limit: 10,
  offset: 0
});
```

**Parameters:**
- `path` (string): Path to inspect
- `options` (object):
  - `limit` (number): Maximum artifacts to return (default: 20)
  - `offset` (number): Pagination offset

**Returns:** Promise<any>

#### `getJobProgress(jobId)`

Get progress status for an indexing job.

```javascript
const status = await client.getJobProgress('123');
console.log('Status:', status.status, 'Progress:', status.progress + '%');
```

**Parameters:**
- `jobId` (string | number): Job identifier

**Returns:** Promise<JobStatus>

#### `listJobs(options)`

List indexing jobs with optional filtering.

```javascript
const jobs = await client.listJobs({
  activeOnly: true,
  limit: 10
});

jobs.jobs.forEach(job => {
  console.log(`Job ${job.jobId}: ${job.status}`);
});
```

**Parameters:**
- `options` (object):
  - `activeOnly` (boolean): Show only active jobs (default: false)
  - `status` (string): Filter by status
  - `minProgress` (number): Filter jobs with progress >= this value (0-100)
  - `maxProgress` (number): Filter jobs with progress <= this value (0-100)
  - `limit` (number): Maximum jobs to return (default: 50)

**Returns:** Promise<ListJobsResult>

#### `cancelJob(jobId)`

Cancel a running or pending indexing job.

```javascript
const result = await client.cancelJob('123');
console.log(result.message);
```

**Parameters:**
- `jobId` (string | number): Job identifier to cancel

**Returns:** Promise<object>

### Selection Set Tools

#### `createSelectionSet(name, options)`

Create a new selection set.

```javascript
await client.createSelectionSet('large-files', {
  description: 'Files larger than 100MB',
  criteriaType: 'tool_query'
});
```

**Parameters:**
- `name` (string): Name of the selection set
- `options` (object):
  - `description` (string): Description
  - `criteriaType` (string, required): 'user_selected' or 'tool_query'

**Returns:** Promise<string>

#### `listSelectionSets()`

List all selection sets.

```javascript
const sets = await client.listSelectionSets();
sets.forEach(set => console.log(set.name));
```

**Returns:** Promise<SelectionSet[]>

#### `getSelectionSet(name)`

Get entries in a selection set.

```javascript
const entries = await client.getSelectionSet('large-files');
```

**Parameters:**
- `name` (string): Name of the selection set

**Returns:** Promise<any[] | any>

#### `modifySelectionSet(name, operation, paths)`

Modify a selection set by adding or removing entries.

```javascript
// Add files
await client.modifySelectionSet('large-files', 'add', [
  '/home/user/file1.bin',
  '/home/user/file2.iso'
]);

// Remove files
await client.modifySelectionSet('large-files', 'remove', [
  '/home/user/file1.bin'
]);
```

**Parameters:**
- `name` (string): Selection set name
- `operation` (string): 'add' or 'remove'
- `paths` (string[] | string): Array of paths or comma-separated string

**Returns:** Promise<string>

#### `deleteSelectionSet(name)`

Delete a selection set.

```javascript
await client.deleteSelectionSet('large-files');
```

**Parameters:**
- `name` (string): Selection set name

**Returns:** Promise<string>

### Query Tools

#### `createQuery(name, options)`

Create a new saved query.

```javascript
await client.createQuery('large-videos', {
  description: 'Video files larger than 100MB',
  queryType: 'file_filter',
  queryJSON: {
    minSize: 104857600,
    extensions: ['.mp4', '.mkv', '.avi']
  }
});
```

**Parameters:**
- `name` (string): Query name
- `options` (object):
  - `description` (string): Description
  - `queryType` (string, required): 'file_filter' or 'custom_script'
  - `queryJSON` (object | string, required): Query filter

**Returns:** Promise<string>

#### `executeQuery(name)`

Execute a saved query.

```javascript
const results = await client.executeQuery('large-videos');
console.log('Found', results.length, 'files');
```

**Parameters:**
- `name` (string): Query name

**Returns:** Promise<any[] | any>

#### `listQueries()`

List all saved queries.

```javascript
const queries = await client.listQueries();
```

**Returns:** Promise<Query[]>

#### `getQuery(name)`

Get details of a saved query.

```javascript
const query = await client.getQuery('large-videos');
console.log(query.queryJSON);
```

**Parameters:**
- `name` (string): Query name

**Returns:** Promise<Query | string>

#### `updateQuery(name, queryJSON)`

Update a saved query.

```javascript
await client.updateQuery('large-videos', {
  minSize: 209715200,
  extensions: ['.mp4', '.mkv', '.avi', '.mov']
});
```

**Parameters:**
- `name` (string): Query name
- `queryJSON` (object | string): Updated query filter

**Returns:** Promise<string>

#### `deleteQuery(name)`

Delete a saved query.

```javascript
await client.deleteQuery('large-videos');
```

**Parameters:**
- `name` (string): Query name

**Returns:** Promise<string>

### File Action Tools

#### `renameFiles(paths, pattern, replacement, options)`

Rename files based on a regex pattern.

```javascript
const result = await client.renameFiles(
  ['/home/user/photo_001.jpg', '/home/user/photo_002.jpg'],
  'photo_(\\d+)',
  'image_$1',
  { dryRun: true }
);

console.log('Renamed:', result.successCount, 'files');
```

**Parameters:**
- `paths` (string[] | string): Paths to rename
- `pattern` (string): Regex pattern to match
- `replacement` (string): Replacement string (supports $1, $2, etc.)
- `options` (object):
  - `dryRun` (boolean): Preview without executing (default: false)

**Returns:** Promise<FileOperationResult>

#### `deleteFiles(paths, options)`

Delete files or directories.

```javascript
const result = await client.deleteFiles(
  ['/home/user/temp/file.txt', '/home/user/temp/old_dir'],
  { recursive: true, dryRun: false }
);

console.log('Deleted:', result.successCount, 'items');
```

**Parameters:**
- `paths` (string[] | string): Paths to delete
- `options` (object):
  - `recursive` (boolean): Delete directories recursively (default: false)
  - `dryRun` (boolean): Preview without executing (default: false)

**Returns:** Promise<FileOperationResult>

#### `moveFiles(sources, destination, options)`

Move files or directories to a destination.

```javascript
const result = await client.moveFiles(
  ['/home/user/file1.txt', '/home/user/file2.txt'],
  '/home/user/archive',
  { dryRun: false }
);

console.log('Moved:', result.successCount, 'files');
```

**Parameters:**
- `sources` (string[] | string): Source paths
- `destination` (string): Destination directory
- `options` (object):
  - `dryRun` (boolean): Preview without executing (default: false)

**Returns:** Promise<FileOperationResult>

### Session Tools

#### `getSessionInfo()`

Get session information.

```javascript
const info = await client.getSessionInfo();
console.log('Database:', info.database);
console.log('Version:', info.version);
```

**Returns:** Promise<SessionInfo>

#### `setSessionPreferences(preferences)`

Set session preferences.

```javascript
await client.setSessionPreferences({
  theme: 'dark',
  pageSize: 50
});
```

**Parameters:**
- `preferences` (object | string): Preferences object or JSON string

**Returns:** Promise<string>

### Utility Methods

#### `listTools()`

List all available MCP tools.

```javascript
const tools = await client.listTools();
tools.forEach(tool => console.log(tool.name, '-', tool.description));
```

**Returns:** Promise<MCPTool[]>

#### `healthCheck()`

Check if the server is responding.

```javascript
const healthy = await client.healthCheck();
console.log(healthy ? 'Server is up' : 'Server is down');
```

**Returns:** Promise<boolean>

## Framework Integration

### React Example

```javascript
import React, { useState, useEffect } from 'react';
import MCPSpaceBrowserClient from './mcp-client';

function DiskExplorer() {
  const [client] = useState(() => new MCPSpaceBrowserClient('http://localhost:3000'));
  const [path, setPath] = useState('/home/user');
  const [entries, setEntries] = useState([]);

  useEffect(() => {
    async function load() {
      const result = await client.navigate(path, { limit: 50 });
      setEntries(result.entries);
    }
    load();
  }, [path]);

  return (
    <div>
      <h1>Disk Explorer</h1>
      <ul>
        {entries.map(entry => (
          <li key={entry.path} onClick={() => entry.kind === 'directory' && setPath(entry.path)}>
            {entry.name} - {(entry.size / 1024 / 1024).toFixed(2)} MB
          </li>
        ))}
      </ul>
    </div>
  );
}
```

### Vue Example

```vue
<template>
  <div>
    <h1>Disk Explorer</h1>
    <ul>
      <li v-for="entry in entries" :key="entry.path" @click="navigate(entry)">
        {{ entry.name }} - {{ formatSize(entry.size) }}
      </li>
    </ul>
  </div>
</template>

<script>
import MCPSpaceBrowserClient from './mcp-client';

export default {
  data() {
    return {
      client: new MCPSpaceBrowserClient('http://localhost:3000'),
      path: '/home/user',
      entries: []
    };
  },
  mounted() {
    this.loadDirectory(this.path);
  },
  methods: {
    async loadDirectory(path) {
      const result = await this.client.navigate(path, { limit: 50 });
      this.entries = result.entries;
    },
    navigate(entry) {
      if (entry.kind === 'directory') {
        this.path = entry.path;
        this.loadDirectory(entry.path);
      }
    },
    formatSize(bytes) {
      return (bytes / 1024 / 1024).toFixed(2) + ' MB';
    }
  }
};
</script>
```

## Error Handling

```javascript
try {
  const result = await client.navigate('/invalid/path');
} catch (error) {
  if (error.message.includes('timeout')) {
    console.error('Request timed out');
  } else if (error.message.includes('HTTP')) {
    console.error('Server error:', error.message);
  } else {
    console.error('MCP error:', error.message);
  }
}
```

## Examples

See `examples/client-usage-examples.js` for comprehensive examples including:
- Indexing with job monitoring
- Directory navigation and pagination
- Selection set management
- Query creation and execution
- File operations (rename, delete, move)
- Session management
- React and Vue integration examples

## License

This client library is part of the mcp-space-browser project.

## Support

For issues and questions, please visit: https://github.com/prismon/mcp-space-browser/issues
