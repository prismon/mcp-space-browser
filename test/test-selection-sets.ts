#!/usr/bin/env bun

import { DiskDB } from '../src/db';

async function testSelectionSets() {
  console.log('Testing SelectionSet functionality...\n');
  
  const db = new DiskDB();
  
  // Test 1: Create a selection set for old files
  console.log('1. Creating selection set "old_files" with 30 oldest files:');
  
  // First, collect all files and sort by mtime
  const allFiles: Array<{path: string, size: number, mtime: number}> = [];
  
  function collectFiles(root: string): void {
    const entry = db.get(root);
    if (!entry) return;
    
    if (entry.kind === 'file') {
      allFiles.push({path: root, size: entry.size, mtime: entry.mtime});
    } else if (entry.kind === 'directory') {
      const children = db.children(root);
      for (const child of children) {
        collectFiles(child.path);
      }
    }
  }
  
  collectFiles('/home/josh/Projects/mcp-space-browser');
  allFiles.sort((a, b) => a.mtime - b.mtime);
  
  // Create selection set
  const setId = db.createSelectionSet({
    name: 'old_files',
    description: 'The 30 oldest files in the project',
    criteria_type: 'tool_query',
    criteria_json: JSON.stringify({
      tool: 'disk-time-range',
      params: { path: '.', count: 30 },
      limit: 30
    })
  });
  
  // Add the 30 oldest files
  const oldestFiles = allFiles.slice(0, 30);
  const paths = oldestFiles.map(f => f.path);
  db.addToSelectionSet('old_files', paths);
  
  console.log(`Created selection set with ${paths.length} files`);
  
  // Test 2: List selection sets
  console.log('\n2. Listing all selection sets:');
  const sets = db.listSelectionSets();
  for (const set of sets) {
    const stats = db.getSelectionSetStats(set.name);
    console.log(`- ${set.name}: ${stats.count} files, ${(stats.totalSize / 1024).toFixed(2)} KB total`);
  }
  
  // Test 3: Get entries from selection set
  console.log('\n3. First 10 files in "old_files" set:');
  const entries = db.getSelectionSetEntries('old_files');
  entries.slice(0, 10).forEach((entry, i) => {
    const date = new Date(entry.mtime * 1000).toISOString();
    console.log(`  ${i + 1}. ${entry.path} (${entry.size} bytes, ${date})`);
  });
  
  // Test 4: Create a user-selected set
  console.log('\n4. Creating user-selected set "important_files":');
  db.createSelectionSet({
    name: 'important_files',
    description: 'Manually selected important files',
    criteria_type: 'user_selected'
  });
  
  // Add some specific files
  db.addToSelectionSet('important_files', [
    '/home/josh/Projects/mcp-space-browser/src/mcp.ts',
    '/home/josh/Projects/mcp-space-browser/src/db.ts',
    '/home/josh/Projects/mcp-space-browser/README.md'
  ]);
  
  const importantStats = db.getSelectionSetStats('important_files');
  console.log(`Added ${importantStats.count} files to "important_files"`);
  
  // Test 5: Create large files set
  console.log('\n5. Creating "large_files" set (files > 10KB):');
  
  const largeFiles = allFiles.filter(f => f.size > 10240).slice(0, 50);
  
  db.createSelectionSet({
    name: 'large_files',
    description: 'Files larger than 10KB',
    criteria_type: 'tool_query',
    criteria_json: JSON.stringify({
      tool: 'disk-tree',
      params: { path: '.', minSize: 10240, sortBy: 'size', descendingSort: true },
      limit: 50
    })
  });
  
  db.addToSelectionSet('large_files', largeFiles.map(f => f.path));
  
  const largeStats = db.getSelectionSetStats('large_files');
  console.log(`Created "large_files" set with ${largeStats.count} files, total size: ${(largeStats.totalSize / 1024 / 1024).toFixed(2)} MB`);
  
  // Final summary
  console.log('\n6. Final summary of all selection sets:');
  const allSets = db.listSelectionSets();
  for (const set of allSets) {
    const stats = db.getSelectionSetStats(set.name);
    console.log(`- ${set.name}: ${stats.count} files, ${(stats.totalSize / 1024).toFixed(2)} KB`);
  }
}

testSelectionSets().catch(console.error);