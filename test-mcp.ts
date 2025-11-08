import { FastMCP } from 'fastmcp';
import { z } from 'zod';

const server = new FastMCP({
  name: 'test-server',
  version: '1.0.0',
});

server.addTool({
  name: 'test-tool',
  description: 'A simple test tool',
  parameters: z.object({
    message: z.string().describe('A test message'),
  }),
  execute: async (args, context) => {
    return `You said: ${args.message}`;
  },
});

server.addTool({
  name: 'no-params-tool',
  description: 'A tool with no parameters',
  execute: async (args, context) => {
    return 'No params!';
  },
});

const port = 8081;
server.start({
  transportType: 'httpStream',
  httpStream: { port, host: '0.0.0.0', stateless: true },
});
console.log(`Test server running at http://0.0.0.0:${port}/mcp`);
