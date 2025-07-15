# mcp-space-browser

This project implements a disk space indexing agent using the Bun runtime.
See [docs/disk_agent_design.md](docs/disk_agent_design.md) for the design.

## Usage

Install [Bun](https://bun.sh/) and run one of the CLI commands:

```bash
bun src/cli.ts disk-index /path/to/scan
bun src/cli.ts disk-du /path
bun src/cli.ts disk-tree /path
```

A simple HTTP server is available via:

```bash
bun src/server.ts
```

### MCP Server

To start the FastMCP server with tools for indexing and querying the disk database:

```bash
bun src/mcp.ts
```
