# mcp-space-browser

This project implements a disk space indexing agent using the Bun runtime.
See [docs/disk_agent_design.md](docs/disk_agent_design.md) for the design.

## Usage

Install [Bun](https://bun.sh/) and run one of the CLI commands:

```bash
bun src/cli.ts disk-index /path/to/scan
bun src/cli.ts disk-du /path
bun src/cli.ts disk-tree /path
bun src/cli.ts disk-info /path
```

Running `bun run` with no arguments will print a usage summary describing these
commands.

A simple HTTP server is available via:

```bash
bun src/server.ts
```

## MCP Server

You can also run the project as an
[FastMCP](https://github.com/punkpeye/fastmcp) server. This exposes each CLI
command as an MCP tool:

```bash
bun src/mcp.ts                   # stdio transport
bun src/mcp.ts --http-stream     # HTTP streaming on port 8080
```
