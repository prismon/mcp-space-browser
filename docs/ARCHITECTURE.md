# Architecture

## Overview

mcp-space-browser is a disk space indexing agent that crawls filesystems, stores metadata in SQLite, and provides 5 composable MCP tools for exploring disk utilization. It compiles to a single static Go binary with no runtime dependencies.

## Component Diagram

```
                    ┌──────────────────────────────────────────┐
                    │              MCP Clients                 │
                    │  (Claude, ChatGPT, Humans, Automation)   │
                    └────────────────┬─────────────────────────┘
                                     │ JSON-RPC 2.0
                                     ▼
┌──────────┐     ┌──────────────────────────────────────────────┐
│   CLI    │────▶│              MCP Server (Gin)                │
│  (Cobra) │     │  POST /mcp  (streamable HTTP transport)      │
└──────────┘     └──────────────────┬───────────────────────────┘
                                    │
                 ┌──────────────────┼──────────────────┐
                 ▼                  ▼                   ▼
          ┌────────────┐   ┌──────────────┐   ┌──────────────┐
          │  5 Tools   │   │ 8 Resources  │   │   Sources    │
          │scan, query,│   │ synthesis:// │   │  Manager     │
          │manage,batch│   │  templates   │   │ (fsnotify)   │
          │   watch    │   │              │   │              │
          └─────┬──────┘   └──────┬───────┘   └──────┬───────┘
                │                 │                   │
                ▼                 ▼                   ▼
          ┌──────────────────────────────────────────────┐
          │              Database (SQLite)                │
          │   entries, metadata, resource_sets, plans     │
          └──────────────────┬───────────────────────────┘
                             │
                             ▼
          ┌──────────────────────────────────────────────┐
          │            Filesystem Crawler                 │
          │    Stack-based DFS + bottom-up aggregation    │
          └──────────────────────────────────────────────┘
```

## Data Flow

1. **Scan**: `scan` tool invokes crawler on filesystem paths → entries + metadata stored in SQLite
2. **Query**: `query` tool builds dynamic SQL with WHERE/JOIN → filtered entries returned with cursor pagination
3. **Manage**: `manage` tool provides CRUD for resource-sets, plans, and jobs
4. **Batch**: `batch` tool operates on sets of files (attributes, duplicates, move, delete)
5. **Watch**: `watch` tool wraps source manager for real-time fsnotify monitoring

## Key Packages

| Package | Role |
|---------|------|
| `cmd/mcp-space-browser` | CLI entry point (Cobra), server and utility commands |
| `pkg/server` | MCP server, 5 tool handlers, 8 resource templates |
| `pkg/database` | SQLite abstraction: entries, metadata, resource sets, plans, sources, rules, jobs |
| `pkg/crawler` | Stack-based DFS traversal, metadata collection, bottom-up size aggregation |
| `pkg/sources` | Source abstraction (index, watch, query) and live filesystem monitoring |
| `pkg/rules` | Rule engine: condition evaluation and outcome execution |
| `pkg/classifier` | Media file classification and thumbnail generation |
| `internal/models` | Shared data structures: Entry, MetadataRecord, ResourceSet, Plan, Source, Rule |

## Database Schema (Core)

```sql
-- Filesystem entries (one row per file/directory)
entries (path PK, parent, size, blocks, kind, ctime, mtime, last_scanned, dirty)

-- Unified metadata: simple key-value pairs + classifier artifacts
metadata (entry_path, key, value, source, cache_path, data_json, mime_type, file_size, generator, hash UNIQUE)

-- Named collections forming a DAG
resource_sets (name UNIQUE, description, created_at, updated_at)
resource_set_entries (set_id + entry_path PK)
resource_set_edges (parent_id + child_id PK)
```

See [SCHEMA.md](SCHEMA.md) for the complete schema.

## Key Design Patterns

- **5-Tool Composable Interface**: All MCP interaction through scan, query, manage, batch, watch
- **Unified Metadata**: Extensible key-value pairs and classifier artifacts per entry, queryable via the `where` clause
- **Cursor Pagination**: Base64-encoded offset tokens for LLM-friendly pagination
- **DAG Resource Sets**: Named collections with parent-child edges (multiple parents allowed)
- **Bottom-Up Aggregation**: Directory sizes computed from leaf to root after crawling
- **In-Memory Testing**: All tests use `:memory:` SQLite for speed and isolation

## Size Calculation

The system builds accurate space usage trees in two phases:

1. **Crawl (DFS)**: Files get actual sizes; directories start at size=0
2. **Aggregate (bottom-up)**: Query directories deepest-first, each gets `SUM(size)` of direct children

**Invariant**: A directory's size always equals the sum of its direct children's sizes.
