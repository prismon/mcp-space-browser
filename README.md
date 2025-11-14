# mcp-space-browser

A high-performance disk space indexing agent written in Go that crawls filesystems, stores metadata in SQLite, and provides tools for exploring disk utilization (similar to Baobab/WinDirStat).

See [docs/disk_agent_design.md](docs/disk_agent_design.md) for the design and [README.go.md](README.go.md) for complete documentation.

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
- **MCP endpoint**: `http://localhost:3000/mcp` with 17 tools for disk space analysis
- **REST API**: `http://localhost:3000/api/*` for programmatic access

Available MCP tools: disk-index, disk-du, disk-tree, disk-time-range, selection set management, saved queries, and session preferences.

### CLI Commands

You can also run individual CLI commands directly:

```bash
./mcp-space-browser disk-index /path/to/scan
./mcp-space-browser disk-du /path
./mcp-space-browser disk-tree /path

# Or with go run
go run ./cmd/mcp-space-browser disk-index /path/to/scan
go run ./cmd/mcp-space-browser disk-du /path
go run ./cmd/mcp-space-browser disk-tree /path
```
