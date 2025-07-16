#!/usr/bin/env bun

// Test session functionality in the MCP server

import { spawn } from 'child_process';
import { createReadStream, createWriteStream } from 'fs';

async function testSessionFunctionality() {
  console.log('Testing MCP Session Functionality...\n');
  
  // Note: Since we're testing with stdio transport, we won't have HTTP sessions
  // but we can still test the context parameter functionality
  
  // Start the MCP server in stdio mode
  const mcpProcess = spawn('bun', ['src/mcp.ts'], {
    stdio: ['pipe', 'pipe', 'pipe']
  });
  
  let output = '';
  
  mcpProcess.stdout.on('data', (data) => {
    output += data.toString();
  });
  
  mcpProcess.stderr.on('data', (data) => {
    console.error('MCP stderr:', data.toString());
  });
  
  // Send initialization
  const init = {
    jsonrpc: '2.0',
    id: 1,
    method: 'initialize',
    params: {
      protocolVersion: '1.0.0',
      capabilities: {
        tools: {},
        prompts: {},
        resources: {}
      },
      clientInfo: {
        name: 'test-client',
        version: '1.0.0'
      }
    }
  };
  
  mcpProcess.stdin.write(JSON.stringify(init) + '\n');
  
  // Wait a bit for initialization
  await new Promise(resolve => setTimeout(resolve, 1000));
  
  // Test session-info tool
  console.log('1. Testing session-info tool:');
  const sessionInfoReq = {
    jsonrpc: '2.0',
    id: 2,
    method: 'tools/call',
    params: {
      name: 'session-info',
      arguments: {}
    }
  };
  
  mcpProcess.stdin.write(JSON.stringify(sessionInfoReq) + '\n');
  
  // Wait for response
  await new Promise(resolve => setTimeout(resolve, 500));
  
  // Test setting preferences
  console.log('\n2. Testing session-set-preferences:');
  const setPrefReq = {
    jsonrpc: '2.0',
    id: 3,
    method: 'tools/call',
    params: {
      name: 'session-set-preferences',
      arguments: {
        maxResults: 50,
        sortBy: 'mtime'
      }
    }
  };
  
  mcpProcess.stdin.write(JSON.stringify(setPrefReq) + '\n');
  
  // Wait for response
  await new Promise(resolve => setTimeout(resolve, 500));
  
  // Test disk-du with session context
  console.log('\n3. Testing disk-du with session context:');
  const diskDuReq = {
    jsonrpc: '2.0',
    id: 4,
    method: 'tools/call',
    params: {
      name: 'disk-du',
      arguments: {
        path: 'src'
      }
    }
  };
  
  mcpProcess.stdin.write(JSON.stringify(diskDuReq) + '\n');
  
  // Wait for response
  await new Promise(resolve => setTimeout(resolve, 500));
  
  // Create a selection set
  console.log('\n4. Creating selection set with session tracking:');
  const createSetReq = {
    jsonrpc: '2.0',
    id: 5,
    method: 'tools/call',
    params: {
      name: 'selection-set-create',
      arguments: {
        name: 'test_session_set',
        description: 'Created during session test',
        fromTool: {
          tool: 'disk-time-range',
          params: { path: '.', count: 5 },
          limit: 5
        }
      }
    }
  };
  
  mcpProcess.stdin.write(JSON.stringify(createSetReq) + '\n');
  
  // Wait for response
  await new Promise(resolve => setTimeout(resolve, 1000));
  
  // Get session info again to see history
  console.log('\n5. Getting session info to see history:');
  mcpProcess.stdin.write(JSON.stringify({...sessionInfoReq, id: 6}) + '\n');
  
  // Wait and clean up
  await new Promise(resolve => setTimeout(resolve, 1000));
  
  console.log('\nMCP Output:');
  console.log(output);
  
  mcpProcess.kill();
}

// Alternative: Test with direct imports
async function testDirectly() {
  console.log('\nTesting session functionality directly...\n');
  
  // Import the necessary modules
  const { DiskDB } = await import('../src/db');
  
  // Test session state management
  const sessionStates = new Map<string, {
    lastQuery?: string;
    currentSelectionSet?: string;
    history: string[];
  }>();
  
  // Simulate session
  const userId = 'test-user';
  sessionStates.set(userId, { history: [] });
  
  // Add to history
  const state = sessionStates.get(userId)!;
  state.history.push('disk-du: /test/path');
  state.history.push('disk-tree: /another/path');
  state.currentSelectionSet = 'test_set';
  
  console.log('Session state:', {
    userId,
    ...state
  });
  
  // Test with preferences
  const mockContext = {
    session: {
      userId,
      workingDirectory: '/home/josh/Projects/mcp-space-browser',
      preferences: {
        maxResults: 50,
        sortBy: 'mtime' as const
      }
    },
    log: {
      debug: console.log,
      error: console.error,
      info: console.log,
      warn: console.warn
    },
    reportProgress: async () => {},
    streamContent: async () => {}
  };
  
  console.log('\nMock context:', mockContext);
  
  // Clean up test selection set
  const db = new DiskDB();
  try {
    db.deleteSelectionSet('test_session_set');
  } catch (e) {
    // Ignore if doesn't exist
  }
}

// Run tests
testDirectly().catch(console.error);