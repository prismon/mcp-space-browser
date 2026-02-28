# MCP Reference

All interaction with mcp-space-browser happens via MCP (Model Context Protocol) over JSON-RPC 2.0 at `POST /mcp`.

## Tools (5)

### scan

Index filesystem paths and extract attributes.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| paths | string[] | yes | Filesystem paths to scan |
| attributes | string[] | no | Filter which attributes to extract. **Default** (when omitted): thumbnail, video.thumbnails, mime, metadata, permissions. **Opt-in only**: hash.md5, hash.sha256 (slow for large files). Acts as a filter — omitting means all defaults run. |
| depth | number | no | Scan depth: -1=recursive (default), 0=this level, N=N levels |
| force | boolean | no | Re-index even if recently scanned (default: false) |
| target | string | no | Resource set name to populate with results |
| async | boolean | no | Return job ID immediately (default: true) |
| maxAge | number | no | Max age in seconds before rescan (default: 3600) |

**Sync response** includes a `post_processing` field with stats on metadata extraction and thumbnail generation:
```json
{"status": "completed", "duration_ms": 1234, "results": [...], "post_processing": {"files_processed": 50, "metadata_set": 100, "errors": 0, "duration_ms": 500}}
```

```json
{"tool": "scan", "params": {"paths": ["/home/user/Photos"], "depth": -1, "target": "photos"}}
```

```json
{"tool": "scan", "params": {"paths": ["/data"], "attributes": ["mime", "hash.sha256"], "async": false}}
```

### query

Search, filter, and aggregate across entries and metadata.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| from | string | no | Resource set name to query within |
| where | object | no | Filters: keys are field/attribute names, values are exact matches or operator objects ({">": 1000}, {"like": "%.jpg"}) |
| select | string[] | no | Fields to return |
| aggregate | string | no | Function: sum, count, avg, min, max |
| field | string | no | Field for aggregation (e.g. size) |
| group_by | string | no | Group aggregation by this field |
| order_by | string | no | Sort field, prefix `-` for descending |
| limit | number | no | Max results (default: 100) |
| cursor | string | no | Pagination cursor from previous response |

```json
{"tool": "query", "params": {"where": {"kind": "file", "size": {">": 1000000}}, "order_by": "-size", "limit": 10}}
```

```json
{"tool": "query", "params": {"from": "photos", "aggregate": "sum", "field": "size"}}
```

```json
{"tool": "query", "params": {"where": {"mime": {"like": "image/%"}}, "aggregate": "count", "group_by": "mime"}}
```

### manage

CRUD for organizational entities: resource-sets, plans, jobs, and projects.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| entity | string | yes | Entity type: resource-set, plan, job, project |
| action | string | yes | Action: create, get, list, update, delete, open (project only) |
| name | string | no | Entity name |
| description | string | no | Entity description |
| parent | string | no | Parent resource-set name (DAG edges) |
| child | string | no | Child resource-set name (DAG edges) |
| mode | string | no | Plan mode: oneshot, continuous |
| status | string | no | Filter by status (job list) |
| id | number | no | Entity ID (job get) |
| limit | number | no | Max results for list (default: 100) |
| cursor | string | no | Pagination cursor |

```json
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "name": "photos"}}
```

```json
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "parent": "all-media", "child": "photos"}}
```

```json
{"tool": "manage", "params": {"entity": "plan", "action": "list", "limit": 10}}
```

```json
{"tool": "manage", "params": {"entity": "project", "action": "list"}}
```

```json
{"tool": "manage", "params": {"entity": "project", "action": "open", "name": "my-project"}}
```

### batch

Multi-file operations on resource sets or explicit paths.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| operation | string | yes | Operation: attributes, duplicates, move, delete |
| from | string | no | Resource set name to operate on |
| paths | string[] | no | Explicit file paths (alternative to from) |
| keys | string[] | no | Attribute keys to extract (for attributes) |
| method | string | no | Duplicate detection: exact (hash.md5) or perceptual |
| threshold | number | no | Perceptual hamming distance threshold (default: 8) |
| destination | string | no | Destination directory (for move) |
| async | boolean | no | Return job ID for long operations |

```json
{"tool": "batch", "params": {"operation": "attributes", "from": "photos", "keys": ["mime", "exif"]}}
```

```json
{"tool": "batch", "params": {"operation": "duplicates", "from": "photos", "method": "exact"}}
```

### watch

Real-time filesystem monitoring via fsnotify.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| action | string | yes | Action: start, stop, status, list |
| path | string | no | Filesystem path to watch (for start) |
| name | string | no | Watcher name (for start, stop, status) |
| target | string | no | Resource set to populate |
| recursive | boolean | no | Watch subdirectories (default: true) |
| debounce_ms | number | no | Debounce delay in ms (default: 500) |

```json
{"tool": "watch", "params": {"action": "start", "path": "/home/user/Downloads", "name": "downloads-watcher", "recursive": true}}
```

```json
{"tool": "watch", "params": {"action": "list"}}
```

## Resource Templates (8)

| URI | Description |
|-----|-------------|
| `synthesis://entries/{path}` | Entry with all metadata |
| `synthesis://entries/{path}/attributes` | Simple metadata only (key-value pairs) |
| `synthesis://sets` | List all resource sets |
| `synthesis://sets/{name}` | Resource set details |
| `synthesis://sets/{name}/entries` | Entries in a set |
| `synthesis://jobs` | List indexing jobs |
| `synthesis://jobs/{id}` | Job details |
| `synthesis://projects` | List projects |

## Common Patterns

### Index and explore

```json
{"tool": "scan", "params": {"paths": ["/home"], "depth": 1, "target": "home-root"}}
{"tool": "query", "params": {"from": "home-root", "where": {"kind": "directory"}, "order_by": "-size", "limit": 10}}
```

### Find large files

```json
{"tool": "query", "params": {"where": {"kind": "file", "size": {">": 100000000}}, "order_by": "-size"}}
```

### Organize with DAG

```json
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "name": "media"}}
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "name": "photos"}}
{"tool": "manage", "params": {"entity": "resource-set", "action": "create", "parent": "media", "child": "photos"}}
{"tool": "scan", "params": {"paths": ["/home/user/Photos"], "target": "photos"}}
{"tool": "query", "params": {"from": "media", "aggregate": "sum", "field": "size"}}
```
