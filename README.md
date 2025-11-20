# mcp-space-browser

A high-performance disk space indexing agent written in Go that crawls filesystems, stores metadata in SQLite, and provides tools for exploring disk utilization (similar to Baobab/WinDirStat).

See [docs/disk_agent_design.md](docs/disk_agent_design.md) for the design and [README.go.md](README.go.md) for complete documentation. For an MCP shape tuned to small context windows, review [docs/mcp_low_context_domain_model.md](docs/mcp_low_context_domain_model.md).

## Installation

### Prerequisites

- Go 1.21 or later
- SQLite support (via cgo)

### Build

```bash
go build -o mcp-space-browser ./cmd/mcp-space-browser
```

This creates a single static binary with no runtime dependencies.

## Usage

### MCP Server (Recommended)

The recommended way to use mcp-space-browser is as an MCP server, which exposes disk space tools through the Model Context Protocol:

```bash
# Start unified server (provides both REST API and MCP endpoints)
./mcp-space-browser server --port=3000

# Or run directly with go
go run ./cmd/mcp-space-browser server --port=3000
```

This exposes:
- **MCP endpoint**: `http://localhost:3000/mcp` with shell-style navigation tools (`index`, `cd`, `inspect`, `job-progress`) plus selection sets, queries, and session preferences.
- **REST API**: `http://localhost:3000/api/*` for programmatic access
