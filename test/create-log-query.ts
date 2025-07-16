#!/usr/bin/env bun

import { DiskDB, FileFilter } from '../src/db';

async function createLogFilesQuery() {
  console.log('Creating persistent query for log files...\n');
  
  const db = new DiskDB();
  
  // Define the log files query
  const logQuery = {
    name: 'all_log_files',
    description: 'Find all log-related files (logs, CHANGELOG, etc.) sorted by size',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      nameContains: 'log',
      sortBy: 'size',
      descendingSort: true,
      limit: 100
    } as FileFilter),
    target_selection_set: 'log_files',
    update_mode: 'replace' as const
  };
  
  // Create the query
  try {
    db.deleteQuery(logQuery.name);
  } catch (e) {
    // Ignore if doesn't exist
  }
  
  const queryId = db.createQuery(logQuery);
  console.log(`Created query '${logQuery.name}' with ID: ${queryId}`);
  
  // Execute the query
  console.log('\nExecuting query to find log files...');
  const result = db.executeQuery(logQuery.name);
  
  console.log(`\nQuery Results:`);
  console.log(`Files matched: ${result.filesMatched}`);
  console.log(`Selection set: ${result.selectionSet}`);
  
  // Get the selection set entries
  const entries = db.getSelectionSetEntries(result.selectionSet);
  const stats = db.getSelectionSetStats(result.selectionSet);
  
  console.log(`\nLog files from largest to smallest:\n`);
  
  entries.slice(0, 20).forEach((entry, index) => {
    const sizeKB = (entry.size / 1024).toFixed(2);
    const filename = entry.path.split('/').pop();
    
    console.log(`${index + 1}. ${filename} - ${sizeKB} KB`);
    console.log(`   Path: ${entry.path}`);
  });
  
  console.log(`\nSummary:`);
  console.log(`Total files: ${stats.count}`);
  console.log(`Total size: ${(stats.totalSize / 1024 / 1024).toFixed(2)} MB`);
  
  // Show how to re-execute the query later
  console.log(`\nTo re-execute this query later, use:`);
  console.log(`db.executeQuery('${logQuery.name}')`);
  
  // Create additional useful queries
  console.log(`\n\nCreating additional useful queries...`);
  
  // Recent files query
  const recentQuery = {
    name: 'recent_files',
    description: 'Files modified in the last 7 days',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      minDate: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString().split('T')[0],
      sortBy: 'mtime',
      descendingSort: true,
      limit: 50
    } as FileFilter),
    target_selection_set: 'recent_files',
    update_mode: 'replace' as const
  };
  
  try {
    db.deleteQuery(recentQuery.name);
  } catch (e) {}
  
  db.createQuery(recentQuery);
  const recentResult = db.executeQuery(recentQuery.name);
  console.log(`- Created 'recent_files' query: ${recentResult.filesMatched} files found`);
  
  // Source code query
  const sourceQuery = {
    name: 'source_code',
    description: 'All source code files (ts, js, tsx, jsx)',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      extensions: ['ts', 'js', 'tsx', 'jsx'],
      pathContains: 'src',
      sortBy: 'mtime',
      descendingSort: true
    } as FileFilter),
    target_selection_set: 'source_code',
    update_mode: 'replace' as const
  };
  
  try {
    db.deleteQuery(sourceQuery.name);
  } catch (e) {}
  
  db.createQuery(sourceQuery);
  const sourceResult = db.executeQuery(sourceQuery.name);
  console.log(`- Created 'source_code' query: ${sourceResult.filesMatched} files found`);
  
  console.log(`\nAll queries have been created and executed!`);
}

createLogFilesQuery().catch(console.error);