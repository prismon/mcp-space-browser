import { test, expect, describe } from 'bun:test';
import { spawn } from 'child_process';
import * as http from 'http';

describe('MCP Server', () => {
  test('tool enumeration works via stdio', async () => {
    const server = spawn('bun', ['src/mcp.ts'], {
      stdio: ['pipe', 'pipe', 'inherit']
    });

    const toolsListRequest = {
      jsonrpc: '2.0',
      id: 1,
      method: 'tools/list',
      params: {}
    };

    const response = await new Promise<any>((resolve, reject) => {
      let output = '';

      const timeout = setTimeout(() => {
        server.kill();
        reject(new Error('Timeout waiting for response'));
      }, 5000);

      server.stdout.on('data', (data) => {
        output += data.toString();
        // Try to parse each line as JSON
        const lines = output.split('\n');
        for (const line of lines) {
          if (line.trim()) {
            try {
              const parsed = JSON.parse(line);
              clearTimeout(timeout);
              server.kill();
              resolve(parsed);
              return;
            } catch (e) {
              // Not valid JSON yet, keep accumulating
            }
          }
        }
      });

      server.stdin.write(JSON.stringify(toolsListRequest) + '\n');
    });

    expect(response.result).toBeDefined();
    expect(response.result.tools).toBeDefined();
    expect(Array.isArray(response.result.tools)).toBe(true);

    // Check that we have the expected number of tools
    expect(response.result.tools.length).toBeGreaterThan(0);

    // Verify each tool has required properties
    for (const tool of response.result.tools) {
      expect(tool.name).toBeDefined();
      expect(typeof tool.name).toBe('string');
      expect(tool.description).toBeDefined();
      expect(typeof tool.description).toBe('string');
      expect(tool.inputSchema).toBeDefined();
      expect(tool.inputSchema.type).toBe('object');
    }

    // Verify specific tools exist
    const toolNames = response.result.tools.map((t: any) => t.name);
    expect(toolNames).toContain('disk-index');
    expect(toolNames).toContain('disk-du');
    expect(toolNames).toContain('disk-tree');
    expect(toolNames).toContain('disk-time-range');
    expect(toolNames).toContain('selection-set-list');
    expect(toolNames).toContain('session-info');
    expect(toolNames).toContain('query-list');
  });

  test('tools with empty parameters work correctly', async () => {
    const server = spawn('bun', ['src/mcp.ts'], {
      stdio: ['pipe', 'pipe', 'inherit']
    });

    const toolsListRequest = {
      jsonrpc: '2.0',
      id: 1,
      method: 'tools/list',
      params: {}
    };

    const response = await new Promise<any>((resolve, reject) => {
      let output = '';

      const timeout = setTimeout(() => {
        server.kill();
        reject(new Error('Timeout waiting for response'));
      }, 5000);

      server.stdout.on('data', (data) => {
        output += data.toString();
        const lines = output.split('\n');
        for (const line of lines) {
          if (line.trim()) {
            try {
              const parsed = JSON.parse(line);
              clearTimeout(timeout);
              server.kill();
              resolve(parsed);
              return;
            } catch (e) {
              // Not valid JSON yet
            }
          }
        }
      });

      server.stdin.write(JSON.stringify(toolsListRequest) + '\n');
    });

    // Find tools with empty parameters
    const emptyParamTools = response.result.tools.filter((t: any) =>
      Object.keys(t.inputSchema.properties || {}).length === 0
    );

    expect(emptyParamTools.length).toBeGreaterThan(0);

    // Verify tools with empty parameters have correct schema
    for (const tool of emptyParamTools) {
      expect(tool.inputSchema.type).toBe('object');
      expect(tool.inputSchema.properties).toEqual({});
      expect(tool.inputSchema.additionalProperties).toBe(false);
    }
  });

  test('HTTP Stream server starts and responds', async () => {
    const server = spawn('bun', ['src/mcp.ts', '--http-stream'], {
      stdio: ['inherit', 'pipe', 'inherit'],
      env: { ...process.env, PORT: '8082' }
    });

    // Wait for server to start
    await new Promise(resolve => setTimeout(resolve, 2000));

    try {
      // Test that the server is listening
      const response = await new Promise<string>((resolve, reject) => {
        const req = http.request({
          hostname: 'localhost',
          port: 8082,
          path: '/mcp',
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          }
        }, (res) => {
          let data = '';
          res.on('data', chunk => data += chunk);
          res.on('end', () => resolve(data));
        });

        req.on('error', reject);

        req.write(JSON.stringify({
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/list',
          params: {}
        }));

        req.end();
      });

      // The response might be an error about session ID, but the server should respond
      expect(response).toBeDefined();
      const parsed = JSON.parse(response);
      expect(parsed.jsonrpc).toBe('2.0');

      // Either we get a result or an error about session ID
      expect(parsed.result || parsed.error).toBeDefined();
    } finally {
      server.kill();
    }
  });

  test('tools can be invoked', async () => {
    const server = spawn('bun', ['src/mcp.ts'], {
      stdio: ['pipe', 'pipe', 'inherit']
    });

    // First, list tools to ensure server is working
    const listRequest = {
      jsonrpc: '2.0',
      id: 1,
      method: 'tools/list',
      params: {}
    };

    let listResponse: any = null;

    const readResponse = () => new Promise<any>((resolve, reject) => {
      let output = '';

      const timeout = setTimeout(() => {
        server.kill();
        reject(new Error('Timeout waiting for response'));
      }, 5000);

      const handler = (data: Buffer) => {
        output += data.toString();
        const lines = output.split('\n');
        for (const line of lines) {
          if (line.trim()) {
            try {
              const parsed = JSON.parse(line);
              clearTimeout(timeout);
              server.stdout.off('data', handler);
              resolve(parsed);
              return;
            } catch (e) {
              // Not valid JSON yet
            }
          }
        }
      };

      server.stdout.on('data', handler);
    });

    server.stdin.write(JSON.stringify(listRequest) + '\n');
    listResponse = await readResponse();

    expect(listResponse.result).toBeDefined();

    // Now try to invoke session-info which has no parameters
    const invokeRequest = {
      jsonrpc: '2.0',
      id: 2,
      method: 'tools/call',
      params: {
        name: 'session-info',
        arguments: {}
      }
    };

    server.stdin.write(JSON.stringify(invokeRequest) + '\n');
    const invokeResponse = await readResponse();

    expect(invokeResponse.result).toBeDefined();

    server.kill();
  });
});
