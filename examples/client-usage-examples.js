/**
 * MCP Space Browser Client - Usage Examples
 *
 * This file demonstrates how to use the MCPSpaceBrowserClient
 * in various scenarios. These examples work in both Node.js and browser environments.
 */

// For Node.js (CommonJS)
// const MCPSpaceBrowserClient = require('../mcp-client.js');

// For ES Modules or Browser
// import MCPSpaceBrowserClient from '../mcp-client.js';

// For browser with script tag
// <script src="mcp-client.js"></script>
// The MCPSpaceBrowserClient will be available globally

// ==================== Basic Setup ====================

async function basicSetup() {
  // Create a client instance
  const client = new MCPSpaceBrowserClient('http://localhost:3000', {
    timeout: 30000, // 30 seconds
    headers: {
      'X-Custom-Header': 'value'
    }
  });

  // Check if server is healthy
  const isHealthy = await client.healthCheck();
  console.log('Server healthy:', isHealthy);

  // List available tools
  const tools = await client.listTools();
  console.log('Available tools:', tools.map(t => t.name));

  return client;
}

// ==================== Indexing Examples ====================

async function indexingExamples(client) {
  // Example 1: Index a directory asynchronously (recommended for large directories)
  console.log('\n=== Async Indexing ===');
  const indexResult = await client.index('/home/user/projects', { async: true });
  console.log('Job ID:', indexResult.jobId);
  console.log('Status URL:', indexResult.statusUrl);

  // Monitor job progress
  let jobStatus = await client.getJobProgress(indexResult.jobId);
  console.log('Initial status:', jobStatus.status, jobStatus.progress + '%');

  // Poll for completion (in real app, use setInterval or websockets)
  while (jobStatus.status === 'running' || jobStatus.status === 'pending') {
    await new Promise(resolve => setTimeout(resolve, 1000)); // Wait 1 second
    jobStatus = await client.getJobProgress(indexResult.jobId);
    console.log('Progress:', jobStatus.progress + '%');
  }

  console.log('Final status:', jobStatus.status);

  // Example 2: Index synchronously (good for small directories)
  console.log('\n=== Sync Indexing ===');
  const syncResult = await client.index('/home/user/small-dir', { async: false });
  console.log('Files processed:', syncResult.files);
  console.log('Directories processed:', syncResult.directories);
  console.log('Total size:', syncResult.totalSize, 'bytes');

  // Example 3: List all jobs
  const jobs = await client.listJobs({ activeOnly: false, limit: 10 });
  console.log('\nRecent jobs:');
  jobs.jobs.forEach(job => {
    console.log(`  Job ${job.jobId}: ${job.path} - ${job.status} (${job.progress}%)`);
  });

  // Example 4: Cancel a job
  if (indexResult.jobId) {
    const cancelResult = await client.cancelJob(indexResult.jobId);
    console.log('\nCancellation:', cancelResult.message);
  }
}

// ==================== Navigation Examples ====================

async function navigationExamples(client) {
  console.log('\n=== Navigation ===');

  // Navigate to a directory
  const listing = await client.navigate('/home/user/projects', {
    limit: 20,
    offset: 0,
    sortBy: 'size',
    order: 'desc'
  });

  console.log('Current directory:', listing.cwd);
  console.log('Total entries:', listing.count);
  console.log('Summary:', listing.summary);

  // Display top 5 largest entries
  console.log('\nTop 5 largest entries:');
  listing.entries.slice(0, 5).forEach(entry => {
    const sizeMB = (entry.size / (1024 * 1024)).toFixed(2);
    console.log(`  ${entry.name} (${entry.kind}): ${sizeMB} MB`);
  });

  // Paginate through results
  if (listing.nextPageUrl) {
    console.log('\nMore results available at:', listing.nextPageUrl);
  }

  // Inspect a specific file
  const fileDetails = await client.inspect('/home/user/projects/large-file.bin', {
    limit: 10,
    offset: 0
  });
  console.log('\nFile details:', fileDetails);
}

// ==================== Selection Set Examples ====================

async function selectionSetExamples(client) {
  console.log('\n=== Selection Sets ===');

  // Create a selection set
  await client.createSelectionSet('large-files', {
    description: 'Files larger than 100MB',
    criteriaType: 'tool_query'
  });
  console.log('Created selection set: large-files');

  // Add files to the selection set
  await client.modifySelectionSet('large-files', 'add', [
    '/home/user/projects/video1.mp4',
    '/home/user/projects/video2.mkv',
    '/home/user/projects/archive.zip'
  ]);
  console.log('Added 3 files to selection set');

  // List all selection sets
  const sets = await client.listSelectionSets();
  console.log('\nAll selection sets:');
  sets.forEach(set => {
    console.log(`  ${set.name}: ${set.description || 'No description'}`);
  });

  // Get entries in a selection set
  const entries = await client.getSelectionSet('large-files');
  console.log('\nEntries in "large-files":');
  if (Array.isArray(entries)) {
    entries.forEach(entry => {
      const sizeMB = (entry.size / (1024 * 1024)).toFixed(2);
      console.log(`  ${entry.path}: ${sizeMB} MB`);
    });
  } else {
    // Compressed response
    console.log('  (Compressed response with', entries.statistics?.total_entries, 'total entries)');
  }

  // Remove files from selection set
  await client.modifySelectionSet('large-files', 'remove', [
    '/home/user/projects/video1.mp4'
  ]);
  console.log('\nRemoved 1 file from selection set');

  // Delete selection set
  await client.deleteSelectionSet('large-files');
  console.log('Deleted selection set');
}

// ==================== Query Examples ====================

async function queryExamples(client) {
  console.log('\n=== Queries ===');

  // Create a query for large video files
  await client.createQuery('large-videos', {
    description: 'Video files larger than 100MB',
    queryType: 'file_filter',
    queryJSON: {
      minSize: 104857600, // 100MB in bytes
      extensions: ['.mp4', '.mkv', '.avi', '.mov'],
      kind: 'file'
    }
  });
  console.log('Created query: large-videos');

  // List all queries
  const queries = await client.listQueries();
  console.log('\nAll queries:');
  queries.forEach(query => {
    console.log(`  ${query.name} (${query.queryType}): ${query.description || 'No description'}`);
  });

  // Get query details
  const queryDetails = await client.getQuery('large-videos');
  console.log('\nQuery details:', queryDetails);

  // Execute the query
  const results = await client.executeQuery('large-videos');
  console.log('\nQuery results:');
  if (Array.isArray(results)) {
    console.log(`  Found ${results.length} matching files`);
    results.slice(0, 5).forEach(file => {
      const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
      console.log(`  ${file.path}: ${sizeMB} MB`);
    });
  } else {
    // Compressed response
    console.log('  (Compressed response)');
    console.log('  Total entries:', results.statistics?.total_entries);
    console.log('  Total size:', (results.statistics?.total_size_mb || 0).toFixed(2), 'MB');
  }

  // Update the query to include more formats
  await client.updateQuery('large-videos', {
    minSize: 104857600,
    extensions: ['.mp4', '.mkv', '.avi', '.mov', '.webm', '.flv'],
    kind: 'file'
  });
  console.log('\nUpdated query to include more video formats');

  // Delete the query
  await client.deleteQuery('large-videos');
  console.log('Deleted query');
}

// ==================== File Action Examples ====================

async function fileActionExamples(client) {
  console.log('\n=== File Actions ===');

  // Example 1: Rename files with dry run (preview only)
  console.log('\n--- Rename Preview ---');
  const renamePreview = await client.renameFiles(
    ['/home/user/temp/photo_001.jpg', '/home/user/temp/photo_002.jpg'],
    'photo_(\\d+)',
    'image_$1',
    { dryRun: true }
  );
  console.log('Rename preview:');
  renamePreview.results.forEach(result => {
    if (result.status === 'preview') {
      console.log(`  ${result.oldPath} -> ${result.newPath}`);
    }
  });

  // Example 2: Actually rename files
  console.log('\n--- Rename Execution ---');
  const renameResult = await client.renameFiles(
    ['/home/user/temp/photo_001.jpg'],
    'photo_(\\d+)',
    'image_$1',
    { dryRun: false }
  );
  console.log('Renamed:', renameResult.successCount, 'files');
  console.log('Errors:', renameResult.errorCount);

  // Example 3: Delete files with dry run
  console.log('\n--- Delete Preview ---');
  const deletePreview = await client.deleteFiles(
    ['/home/user/temp/old_file.txt', '/home/user/temp/old_dir'],
    { recursive: true, dryRun: true }
  );
  console.log('Delete preview:');
  deletePreview.results.forEach(result => {
    if (result.status === 'preview') {
      console.log(`  ${result.path} (${result.type})`);
    }
  });

  // Example 4: Actually delete files
  console.log('\n--- Delete Execution ---');
  const deleteResult = await client.deleteFiles(
    ['/home/user/temp/old_file.txt'],
    { recursive: false, dryRun: false }
  );
  console.log('Deleted:', deleteResult.successCount, 'items');
  console.log('Errors:', deleteResult.errorCount);

  // Example 5: Move files with dry run
  console.log('\n--- Move Preview ---');
  const movePreview = await client.moveFiles(
    ['/home/user/temp/file1.txt', '/home/user/temp/file2.txt'],
    '/home/user/archive',
    { dryRun: true }
  );
  console.log('Move preview:');
  movePreview.results.forEach(result => {
    if (result.status === 'preview') {
      console.log(`  ${result.sourcePath} -> ${result.targetPath}`);
    }
  });

  // Example 6: Actually move files
  console.log('\n--- Move Execution ---');
  const moveResult = await client.moveFiles(
    ['/home/user/temp/file1.txt'],
    '/home/user/archive',
    { dryRun: false }
  );
  console.log('Moved:', moveResult.successCount, 'items to', moveResult.destination);
  console.log('Errors:', moveResult.errorCount);
}

// ==================== Session Examples ====================

async function sessionExamples(client) {
  console.log('\n=== Session Management ===');

  // Get session info
  const sessionInfo = await client.getSessionInfo();
  console.log('Session info:');
  console.log('  Database:', sessionInfo.database);
  console.log('  Version:', sessionInfo.version);
  console.log('  Current directory:', sessionInfo.cwd);

  // Set preferences
  await client.setSessionPreferences({
    theme: 'dark',
    pageSize: 50,
    sortBy: 'size',
    showHiddenFiles: false
  });
  console.log('\nPreferences updated');
}

// ==================== Error Handling Example ====================

async function errorHandlingExample(client) {
  console.log('\n=== Error Handling ===');

  try {
    // Try to navigate to a non-existent path
    await client.navigate('/this/path/does/not/exist');
  } catch (error) {
    console.log('Caught error:', error.message);
  }

  try {
    // Try to index with invalid path
    await client.index('', { async: true });
  } catch (error) {
    console.log('Caught error:', error.message);
  }

  try {
    // Try to get non-existent job
    await client.getJobProgress('99999');
  } catch (error) {
    console.log('Caught error:', error.message);
  }
}

// ==================== Main Example Runner ====================

async function runAllExamples() {
  try {
    console.log('='.repeat(60));
    console.log('MCP Space Browser Client - Usage Examples');
    console.log('='.repeat(60));

    const client = await basicSetup();

    // Run examples (comment out any you don't want to run)
    // await indexingExamples(client);
    // await navigationExamples(client);
    // await selectionSetExamples(client);
    // await queryExamples(client);
    // await fileActionExamples(client);
    await sessionExamples(client);
    await errorHandlingExample(client);

    console.log('\n' + '='.repeat(60));
    console.log('All examples completed!');
    console.log('='.repeat(60));
  } catch (error) {
    console.error('Example failed:', error);
  }
}

// ==================== React/Vue Integration Example ====================

/**
 * Example: Using the client in a React component
 */
/*
import React, { useState, useEffect } from 'react';
import MCPSpaceBrowserClient from './mcp-client';

function DiskSpaceExplorer() {
  const [client] = useState(() => new MCPSpaceBrowserClient('http://localhost:3000'));
  const [currentPath, setCurrentPath] = useState('/home/user');
  const [entries, setEntries] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadDirectory(currentPath);
  }, [currentPath]);

  async function loadDirectory(path) {
    setLoading(true);
    try {
      const result = await client.navigate(path, { limit: 50, sortBy: 'size' });
      setEntries(result.entries);
    } catch (error) {
      console.error('Failed to load directory:', error);
    } finally {
      setLoading(false);
    }
  }

  async function indexDirectory(path) {
    try {
      const result = await client.index(path, { async: true });
      console.log('Indexing started, job ID:', result.jobId);
      // Poll for job completion...
    } catch (error) {
      console.error('Failed to index:', error);
    }
  }

  return (
    <div>
      <h1>Disk Space Explorer</h1>
      <p>Current: {currentPath}</p>
      <button onClick={() => indexDirectory(currentPath)}>Index This Directory</button>
      {loading ? (
        <p>Loading...</p>
      ) : (
        <ul>
          {entries.map(entry => (
            <li key={entry.path} onClick={() => entry.kind === 'directory' && setCurrentPath(entry.path)}>
              {entry.name} - {(entry.size / (1024 * 1024)).toFixed(2)} MB
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
*/

// ==================== Vue Integration Example ====================

/**
 * Example: Using the client in a Vue component
 */
/*
<template>
  <div>
    <h1>Disk Space Explorer</h1>
    <p>Current: {{ currentPath }}</p>
    <button @click="indexDirectory(currentPath)">Index This Directory</button>
    <div v-if="loading">Loading...</div>
    <ul v-else>
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
      currentPath: '/home/user',
      entries: [],
      loading: false
    };
  },
  mounted() {
    this.loadDirectory(this.currentPath);
  },
  methods: {
    async loadDirectory(path) {
      this.loading = true;
      try {
        const result = await this.client.navigate(path, { limit: 50, sortBy: 'size' });
        this.entries = result.entries;
      } catch (error) {
        console.error('Failed to load directory:', error);
      } finally {
        this.loading = false;
      }
    },
    async indexDirectory(path) {
      try {
        const result = await this.client.index(path, { async: true });
        console.log('Indexing started, job ID:', result.jobId);
      } catch (error) {
        console.error('Failed to index:', error);
      }
    },
    navigate(entry) {
      if (entry.kind === 'directory') {
        this.currentPath = entry.path;
        this.loadDirectory(entry.path);
      }
    },
    formatSize(bytes) {
      return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
    }
  }
};
</script>
*/

// Run examples if this file is executed directly
if (typeof require !== 'undefined' && require.main === module) {
  runAllExamples();
}

// Export for use in other files
if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    runAllExamples,
    basicSetup,
    indexingExamples,
    navigationExamples,
    selectionSetExamples,
    queryExamples,
    fileActionExamples,
    sessionExamples,
    errorHandlingExample
  };
}
