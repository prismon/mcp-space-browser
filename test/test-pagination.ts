#!/usr/bin/env bun

// Test the pagination feature and disk-time-range tool

async function testPagination() {
  console.log('Testing MCP pagination and time range features...\n');
  
  console.log('Testing via MCP server (assuming it\'s running on port 8080)...');
  
  // Test 1: Use the new disk-time-range tool (fastest for getting oldest/newest)
  console.log('\n1. Testing disk-time-range tool:');
  const response1 = await fetch('http://localhost:8080/mcp', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      jsonrpc: '2.0',
      method: 'tools/call',
      params: {
        name: 'disk-time-range',
        arguments: {
          path: '.',
          count: 10
        }
      },
      id: 1
    })
  });
  
  const result1 = await response1.json();
  if (result1.error) {
    console.error('Error:', result1.error);
  } else {
    const content = JSON.parse(result1.result.content[0].text);
    console.log('Total files:', content.totalFiles);
    console.log('\nOldest files:');
    content.oldest.forEach((f: any, i: number) => {
      console.log(`  ${i + 1}. ${f.path} (${f.size} bytes, ${f.age})`);
    });
    console.log('\nNewest files:');
    content.newest.forEach((f: any, i: number) => {
      console.log(`  ${i + 1}. ${f.path} (${f.size} bytes, ${f.age})`);
    });
  }
  
  // Test 2: Get paginated results with disk-tree
  console.log('\n\n2. Testing disk-tree with pagination (sorted by mtime):');
  const response2 = await fetch('http://localhost:8080/mcp', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      jsonrpc: '2.0',
      method: 'tools/call',
      params: {
        name: 'disk-tree',
        arguments: {
          path: '.',
          sortBy: 'mtime',
          descendingSort: false,
          pageSize: 5,
          offset: 0
        }
      },
      id: 2
    })
  });
  
  const result2 = await response2.json();
  if (result2.error) {
    console.error('Error:', result2.error);
  } else {
    const content = JSON.parse(result2.result.content[0].text);
    console.log('Pagination info:', content.pagination);
    console.log('\nFirst 5 entries (oldest):');
    content.entries.forEach((e: any, i: number) => {
      console.log(`  ${i + 1}. ${e.path} (${e.size} bytes, ${e.mtime})`);
    });
  }
}

testPagination().catch(console.error);