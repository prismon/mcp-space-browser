#!/usr/bin/env bun

import { DiskDB, FileFilter } from '../src/db';

async function testQueryFunctionality() {
  console.log('Testing Query Persistence and Execution...\n');
  
  const db = new DiskDB();
  
  // Test 1: Create a query for log files
  console.log('1. Creating query for log files:');
  
  const logFilesQuery = {
    name: 'find_log_files',
    description: 'Find all log-related files in the project',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      nameContains: 'log',
      sortBy: 'size',
      descendingSort: true,
      limit: 100
    } as FileFilter),
    target_selection_set: 'log_files_query',
    update_mode: 'replace' as const
  };
  
  try {
    db.deleteQuery(logFilesQuery.name);
  } catch (e) {
    // Ignore if doesn't exist
  }
  
  const queryId = db.createQuery(logFilesQuery);
  console.log(`Created query '${logFilesQuery.name}' with ID: ${queryId}`);
  
  // Test 2: Execute the query
  console.log('\n2. Executing the query:');
  const result = db.executeQuery(logFilesQuery.name);
  console.log(`Query executed successfully!`);
  console.log(`Files matched: ${result.filesMatched}`);
  console.log(`Selection set updated: ${result.selectionSet}`);
  
  // Test 3: Create a query for TypeScript files
  console.log('\n3. Creating query for TypeScript files:');
  
  const tsFilesQuery = {
    name: 'typescript_files',
    description: 'Find all TypeScript source files',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      extensions: ['ts'],
      pathContains: 'src',
      sortBy: 'mtime',
      descendingSort: true,
      limit: 50
    } as FileFilter),
    target_selection_set: 'typescript_sources',
    update_mode: 'replace' as const
  };
  
  try {
    db.deleteQuery(tsFilesQuery.name);
  } catch (e) {
    // Ignore
  }
  
  db.createQuery(tsFilesQuery);
  const tsResult = db.executeQuery(tsFilesQuery.name);
  console.log(`TypeScript files found: ${tsResult.filesMatched}`);
  
  // Test 4: Create a query for large files
  console.log('\n4. Creating query for large files (>100KB):');
  
  const largeFilesQuery = {
    name: 'large_files',
    description: 'Find files larger than 100KB',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      minSize: 100 * 1024, // 100KB
      sortBy: 'size',
      descendingSort: true,
      limit: 20
    } as FileFilter),
    target_selection_set: 'large_files_set',
    update_mode: 'replace' as const
  };
  
  try {
    db.deleteQuery(largeFilesQuery.name);
  } catch (e) {
    // Ignore
  }
  
  db.createQuery(largeFilesQuery);
  const largeResult = db.executeQuery(largeFilesQuery.name);
  console.log(`Large files found: ${largeResult.filesMatched}`);
  
  // Test 5: List all queries
  console.log('\n5. Listing all queries:');
  const queries = db.listQueries();
  queries.forEach(q => {
    console.log(`- ${q.name}: ${q.description}`);
    console.log(`  Target: ${q.target_selection_set}, Mode: ${q.update_mode}`);
    console.log(`  Executed: ${q.execution_count || 0} times`);
    if (q.last_executed) {
      console.log(`  Last run: ${new Date(q.last_executed * 1000).toLocaleString()}`);
    }
  });
  
  // Test 6: Show query execution history
  console.log('\n6. Query execution history for "find_log_files":');
  const executions = db.getQueryExecutions('find_log_files');
  executions.forEach((e, i) => {
    console.log(`Execution ${i + 1}:`);
    console.log(`  Time: ${new Date(e.executed_at! * 1000).toLocaleString()}`);
    console.log(`  Duration: ${e.duration_ms}ms`);
    console.log(`  Files matched: ${e.files_matched}`);
    console.log(`  Status: ${e.status}`);
  });
  
  // Test 7: Update a query
  console.log('\n7. Updating the log files query to limit to 50 files:');
  db.updateQuery('find_log_files', {
    description: 'Find top 50 log-related files by size',
    query_json: JSON.stringify({
      path: '.',
      nameContains: 'log',
      sortBy: 'size',
      descendingSort: true,
      limit: 50
    } as FileFilter)
  });
  
  // Re-execute after update
  const updatedResult = db.executeQuery('find_log_files');
  console.log(`Updated query executed, files matched: ${updatedResult.filesMatched}`);
  
  // Test 8: Show selection sets created by queries
  console.log('\n8. Selection sets created by queries:');
  const sets = db.listSelectionSets();
  const querySets = sets.filter(s => s.criteria_type === 'tool_query' && s.criteria_json?.includes('query'));
  
  querySets.forEach(set => {
    const stats = db.getSelectionSetStats(set.name);
    console.log(`- ${set.name}: ${stats.count} files, ${(stats.totalSize / 1024 / 1024).toFixed(2)} MB`);
  });
  
  // Test 9: Test append mode
  console.log('\n9. Testing append mode:');
  const appendQuery = {
    name: 'append_test',
    description: 'Test append mode',
    query_type: 'file_filter' as const,
    query_json: JSON.stringify({
      path: '.',
      extensions: ['md'],
      limit: 5
    } as FileFilter),
    target_selection_set: 'append_test_set',
    update_mode: 'append' as const
  };
  
  try {
    db.deleteQuery(appendQuery.name);
    db.deleteSelectionSet('append_test_set');
  } catch (e) {
    // Ignore
  }
  
  db.createQuery(appendQuery);
  
  // First execution
  db.executeQuery('append_test');
  const firstCount = db.getSelectionSetStats('append_test_set').count;
  console.log(`First execution: ${firstCount} files`);
  
  // Update to find different files
  db.updateQuery('append_test', {
    query_json: JSON.stringify({
      path: '.',
      extensions: ['ts'],
      limit: 5
    } as FileFilter)
  });
  
  // Second execution (should append)
  db.executeQuery('append_test');
  const secondCount = db.getSelectionSetStats('append_test_set').count;
  console.log(`After append: ${secondCount} files (added ${secondCount - firstCount})`);
  
  console.log('\nQuery persistence test complete!');
}

testQueryFunctionality().catch(console.error);