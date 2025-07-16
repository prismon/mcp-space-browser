#!/usr/bin/env bun

import { DiskDB } from '../src/db';
import * as path from 'path';

async function createLogFilesSelectionSet() {
  console.log('Creating selection set for log files...\n');
  
  const db = new DiskDB();
  
  // First, find all files with log-related names
  const allFiles: Array<{path: string, size: number, mtime: number}> = [];
  
  function collectFiles(root: string): void {
    const entry = db.get(root);
    if (!entry) return;
    
    if (entry.kind === 'file') {
      const filename = path.basename(entry.path).toLowerCase();
      const ext = path.extname(entry.path).toLowerCase();
      
      // Check if it's a log-related file
      if (ext === '.log' || 
          ext === '.logs' || 
          ext === '.out' || 
          ext === '.err' ||
          filename.includes('log') ||
          filename.includes('changelog') ||
          filename.includes('.log.')) {
        allFiles.push({path: entry.path, size: entry.size, mtime: entry.mtime});
      }
    } else if (entry.kind === 'directory') {
      const children = db.children(entry.path);
      for (const child of children) {
        collectFiles(child.path);
      }
    }
  }
  
  // Start from project root
  collectFiles('/home/josh/Projects/mcp-space-browser');
  
  // Sort by size (largest first)
  allFiles.sort((a, b) => b.size - a.size);
  
  // Create selection set
  try {
    db.deleteSelectionSet('log_files');
  } catch (e) {
    // Ignore if doesn't exist
  }
  
  const setId = db.createSelectionSet({
    name: 'log_files',
    description: 'All log files in the project',
    criteria_type: 'tool_query',
    criteria_json: JSON.stringify({
      tool: 'disk-tree',
      params: { path: '.', sortBy: 'size', descendingSort: true },
      filter: 'log-related files'
    })
  });
  
  // Add files to selection set
  if (allFiles.length > 0) {
    db.addToSelectionSet('log_files', allFiles.map(f => f.path));
  }
  
  console.log(`Found ${allFiles.length} log-related files\n`);
  
  // Display files sorted by size (largest to smallest)
  console.log('Log files from largest to smallest:\n');
  
  allFiles.forEach((file, index) => {
    const sizeKB = (file.size / 1024).toFixed(2);
    const sizeMB = (file.size / 1024 / 1024).toFixed(2);
    const filename = path.basename(file.path);
    const dir = path.dirname(file.path);
    
    if (file.size > 1024 * 1024) {
      console.log(`${index + 1}. ${filename} - ${sizeMB} MB`);
    } else {
      console.log(`${index + 1}. ${filename} - ${sizeKB} KB`);
    }
    console.log(`   Path: ${dir}`);
    console.log(`   Size: ${file.size.toLocaleString()} bytes`);
    console.log('');
  });
  
  // Show summary statistics
  const totalSize = allFiles.reduce((sum, f) => sum + f.size, 0);
  console.log('\nSummary:');
  console.log(`Total files: ${allFiles.length}`);
  console.log(`Total size: ${(totalSize / 1024 / 1024).toFixed(2)} MB`);
  
  if (allFiles.length > 0) {
    console.log(`Largest file: ${path.basename(allFiles[0].path)} (${(allFiles[0].size / 1024).toFixed(2)} KB)`);
    console.log(`Smallest file: ${path.basename(allFiles[allFiles.length - 1].path)} (${allFiles[allFiles.length - 1].size} bytes)`);
  }
}

createLogFilesSelectionSet().catch(console.error);