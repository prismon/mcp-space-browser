#!/usr/bin/env bun

// Test SelectionSet functionality via MCP HTTP API

async function testMCPSelectionSets() {
  console.log('Testing SelectionSet via MCP HTTP API...\n');
  
  const baseUrl = 'http://localhost:8080/mcp';
  
  // Helper to make MCP calls
  async function callMCP(method: string, params: any, id: number = 1) {
    const response = await fetch(baseUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        jsonrpc: '2.0',
        method,
        params,
        id
      })
    });
    return response.json();
  }
  
  try {
    // Test 1: List available tools
    console.log('1. Listing available tools:');
    const toolsResult = await callMCP('tools/list', {});
    if (toolsResult.result) {
      const selectionTools = toolsResult.result.tools.filter((t: any) => 
        t.name.includes('selection-set')
      );
      console.log('Selection set tools found:');
      selectionTools.forEach((t: any) => {
        console.log(`  - ${t.name}: ${t.description}`);
      });
    }
    
    // Test 2: Create a selection set from disk-time-range
    console.log('\n2. Creating "newest_files" selection set:');
    const createResult = await callMCP('tools/call', {
      name: 'selection-set-create',
      arguments: {
        name: 'newest_files',
        description: 'The 20 most recently modified files',
        fromTool: {
          tool: 'disk-time-range',
          params: { path: '.', count: 20, newest: true },
          limit: 20
        }
      }
    });
    
    if (createResult.result) {
      const content = JSON.parse(createResult.result.content[0].text);
      console.log(`Created set: ${content.name}, added ${content.entriesAdded} files`);
    }
    
    // Test 3: List all selection sets
    console.log('\n3. Listing all selection sets:');
    const listResult = await callMCP('tools/call', {
      name: 'selection-set-list',
      arguments: {}
    });
    
    if (listResult.result) {
      const sets = JSON.parse(listResult.result.content[0].text);
      sets.forEach((set: any) => {
        console.log(`  - ${set.name}: ${set.fileCount} files, ${(set.totalSize / 1024).toFixed(2)} KB`);
        if (set.description) console.log(`    Description: ${set.description}`);
      });
    }
    
    // Test 4: Get files from a selection set
    console.log('\n4. Getting files from "newest_files" set:');
    const getResult = await callMCP('tools/call', {
      name: 'selection-set-get',
      arguments: { name: 'newest_files' }
    });
    
    if (getResult.result) {
      const data = JSON.parse(getResult.result.content[0].text);
      console.log(`Set contains ${data.stats.count} files, total size: ${(data.stats.totalSize / 1024).toFixed(2)} KB`);
      console.log('First 5 files:');
      data.files.slice(0, 5).forEach((f: any, i: number) => {
        console.log(`  ${i + 1}. ${f.path} (${f.size} bytes)`);
      });
    }
    
  } catch (error) {
    console.error('Error:', error);
  }
}

testMCPSelectionSets().catch(console.error);