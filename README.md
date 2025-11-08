# mcp-space-browser

This project implements a disk space indexing agent using the Bun runtime.
See [docs/disk_agent_design.md](docs/disk_agent_design.md) for the design.

## Installation

Install [Bun](https://bun.sh/) (Node.js alone is insufficient because we depend on Bun-native modules such as `bun:sqlite`):

```bash
curl -fsSL https://bun.sh/install | bash
```

Then install dependencies:

```bash
bun install
```

## Usage

### MCP Server (Recommended)

The recommended way to use mcp-space-browser is as an [FastMCP](https://github.com/punkpeye/fastmcp) server, which exposes disk space tools through the Model Context Protocol:

```bash
# Start with stdio transport (default for MCP clients like Claude Desktop)
bun src/mcp.ts

# Or use the npm script
bun run mcp

# Start with HTTP streaming transport on port 8080
bun src/mcp.ts --http-stream
```

This exposes tools for disk indexing (`disk-index`), disk usage queries (`disk-du`), tree views (`disk-tree`), time-based analysis (`disk-time-range`), and saved query/selection set management.

### CLI Commands

You can also run individual CLI commands directly:

```bash
bun src/cli.ts disk-index /path/to/scan
bun src/cli.ts disk-du /path
bun src/cli.ts disk-tree /path
```

### HTTP Server

A simple HTTP server is available for REST API access:

```bash
bun src/server.ts
```
