# MCP Reference

Complete reference for all MCP (Model Context Protocol) tools and resource templates exposed by mcp-space-browser.

## Overview

mcp-space-browser exposes its functionality **exclusively through MCP**. There are no REST APIs, gRPC interfaces, or other external access methods. All interaction happens via:

1. **MCP Tools** - Imperative operations (create, execute, modify)
2. **MCP Resources** - Declarative data access (read, list)
3. **MCP Resource Templates** - Parameterized resource URIs

### MCP Endpoint

```
POST /mcp
Content-Type: application/json
Protocol: JSON-RPC 2.0 with MCP extensions
Transport: Streamable HTTP
```

### Clients

MCP tools are designed to be usable by:
- **Human Users** via MCP client applications
- **AI Models** (Claude, ChatGPT, etc.) via MCP integration
- **Automated Systems** implementing MCP client protocol

---

## MCP Tools

### Indexing Tools

#### index

Index a filesystem path and store metadata in the database. Skips indexing if the path was recently scanned (within `maxAge` seconds) unless `force=true`.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| root | string | Yes | - | File or directory path to index |
| async | boolean | No | true | Run asynchronously (return job ID immediately) |
| force | boolean | No | false | Force re-indexing even if path was recently scanned |
| maxAge | number | No | 3600 | Maximum age in seconds before a scan is considered stale (0 = always re-index) |

**Performance Optimization:** By default, the index tool checks when a path was last scanned. If the path was scanned within `maxAge` seconds (default: 1 hour), indexing is skipped to avoid redundant work. Use `force=true` to override this behavior.

**Example (async mode):**
```json
{"tool": "index", "params": {"root": "/home/user/Photos"}}
```

**Response:**
```json
{
  "jobId": 123,
  "root": "/home/user/Photos",
  "status": "pending",
  "statusUrl": "synthesis://jobs/123"
}
```

**Example (force re-index):**
```json
{"tool": "index", "params": {"root": "/home/user/Photos", "force": true}}
```

**Example (sync mode with custom max age):**
```json
{"tool": "index", "params": {"root": "/home/user/Photos", "async": false, "maxAge": 300}}
```

**Response (when skipped):**
```json
{
  "root": "/home/user/Photos",
  "status": "completed",
  "files": 0,
  "directories": 0,
  "totalSize": 0,
  "durationMs": 5,
  "skipped": true,
  "skipReason": "Path was scanned 1200 seconds ago (max age: 3600 seconds). Use force=true to re-index."
}
```

---

#### job-progress

Retrieve status for an indexing job.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| jobId | string | Yes | - | Job identifier returned from index |

**Example:**
```json
{"tool": "job-progress", "params": {"jobId": "123"}}
```

---

#### list-jobs

List indexing jobs with optional filtering.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| activeOnly | boolean | No | false | Show only running/pending jobs |
| status | string | No | - | Filter by status (pending, running, completed, failed, cancelled) |
| minProgress | number | No | - | Filter by minimum progress (0-100) |
| maxProgress | number | No | - | Filter by maximum progress (0-100) |
| limit | number | No | 50 | Maximum jobs to return |

---

#### cancel-job

Cancel a running or pending indexing job.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| jobId | string | Yes | - | Job identifier to cancel |

---

### Resource Navigation (DAG)

#### resource-children

Get child nodes in the DAG (downstream navigation).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| depth | integer | No | 1 | Traversal depth (null = unlimited) |
| include_entries | boolean | No | false | Include file entries in response |

**Example:**
```json
{"tool": "resource-children", "params": {"name": "all-media", "depth": 1}}
```

**Response:**
```json
{
  "resource_set": "all-media",
  "children": [
    {"name": "photos", "entry_count": 1234, "child_count": 2},
    {"name": "videos", "entry_count": 56, "child_count": 1}
  ]
}
```

---

#### resource-parent

Get parent nodes in the DAG (upstream navigation, like "..").

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| depth | integer | No | 1 | Traversal depth (null = all ancestors) |

**Example:**
```json
{"tool": "resource-parent", "params": {"name": "vacation"}}
```

**Response:**
```json
{
  "resource_set": "vacation",
  "parents": [
    {"name": "photos"},
    {"name": "videos"}
  ]
}
```

---

### Resource Queries

#### resource-sum

Hierarchical aggregation of a metric across the DAG.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| metric | string | Yes | - | Metric to aggregate (size, count, etc.) |
| include_children | boolean | No | true | Aggregate through DAG |
| depth | integer | No | null | Traversal depth limit |

**Example:**
```json
{"tool": "resource-sum", "params": {"name": "all-media", "metric": "size"}}
```

**Response:**
```json
{
  "resource_set": "all-media",
  "metric": "size",
  "value": 1073741824,
  "breakdown": [
    {"name": "photos", "value": 524288000},
    {"name": "videos", "value": 536870912}
  ]
}
```

---

#### resource-time-range

Filter resources by a time field within a range.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| field | string | Yes | - | Time field (mtime, ctime, added_at) |
| min | string | No | - | Minimum time (ISO 8601 or Unix timestamp) |
| max | string | No | - | Maximum time (ISO 8601 or Unix timestamp) |
| include_children | boolean | No | true | Include child sets |

**Example:**
```json
{"tool": "resource-time-range", "params": {
  "name": "photos",
  "field": "mtime",
  "min": "2024-01-01",
  "max": "2024-12-31"
}}
```

---

#### resource-metric-range

Filter resources by a metric within a numeric range.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| metric | string | Yes | - | Metric field (size, etc.) |
| min | integer | No | - | Minimum value |
| max | integer | No | - | Maximum value |
| include_children | boolean | No | true | Include child sets |

**Example:**
```json
{"tool": "resource-metric-range", "params": {
  "name": "videos",
  "metric": "size",
  "min": 1073741824
}}
```

---

#### resource-is

Exact match on a field value.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| field | string | Yes | - | Field to match (kind, path, parent, etc.) |
| value | string | Yes | - | Exact value to match |
| include_children | boolean | No | true | Include child sets |

**Example:**
```json
{"tool": "resource-is", "params": {
  "name": "photos",
  "field": "kind",
  "value": "file"
}}
```

---

#### resource-fuzzy-match

Fuzzy/pattern matching on text fields.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| field | string | Yes | - | Text field to search (path, etc.) |
| pattern | string | Yes | - | Pattern to match |
| mode | string | No | contains | Match mode (see below) |
| case_sensitive | boolean | No | false | Case-sensitive matching |
| include_children | boolean | No | true | Include child sets |

**Match Modes:**
| Mode | SQL Translation | Example |
|------|-----------------|---------|
| contains | LIKE %pattern% | "vacation" matches "/photos/vacation/beach.jpg" |
| prefix | LIKE pattern% | "/home/user" matches "/home/user/file.txt" |
| suffix | LIKE %pattern | ".jpg" matches "/photo.jpg" |
| regex | REGEXP | "report_\\d{4}\\.pdf" |
| glob | GLOB | "*.jpg" matches all JPEG files |

**Example:**
```json
{"tool": "resource-fuzzy-match", "params": {
  "name": "photos",
  "field": "path",
  "pattern": ".jpg",
  "mode": "suffix"
}}
```

---

#### resource-search

Comprehensive search with multiple filter criteria combined.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| include_children | boolean | No | false | Include child sets |
| kind | string | No | - | Filter by kind (file, directory) |
| extension | string | No | - | Filter by extension (.jpg, pdf) |
| path_contains | string | No | - | Filter paths containing string |
| name_contains | string | No | - | Filter names containing string |
| min_size | integer | No | - | Minimum file size in bytes |
| max_size | integer | No | - | Maximum file size in bytes |
| min_mtime | string | No | - | Minimum modification time (ISO date) |
| max_mtime | string | No | - | Maximum modification time (ISO date) |
| limit | integer | No | 100 | Maximum results |
| offset | integer | No | 0 | Pagination offset |
| sort_by | string | No | size | Sort field (size, name, mtime) |
| sort_desc | boolean | No | true | Sort descending |

**Example:**
```json
{"tool": "resource-search", "params": {
  "name": "all-media",
  "include_children": true,
  "kind": "file",
  "extension": ".jpg",
  "min_size": 1048576,
  "sort_by": "mtime",
  "limit": 50
}}
```

**Response:**
```json
{
  "resource_set": "all-media",
  "entries": [...],
  "total_count": 234,
  "returned_count": 50,
  "offset": 0,
  "limit": 50,
  "has_more": true
}
```

---

### Resource-Set Management

#### resource-set-create

Create a new resource-set (DAG node).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Unique name |
| description | string | No | - | Optional description |

**Example:**
```json
{"tool": "resource-set-create", "params": {
  "name": "photos",
  "description": "All photo files"
}}
```

---

#### resource-set-list

List all resource-sets.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| (none) | - | - | - | - |

**Response:**
```json
{
  "resource_sets": [
    {"name": "photos", "entry_count": 1234, "child_count": 2},
    {"name": "videos", "entry_count": 56, "child_count": 0}
  ]
}
```

---

#### resource-set-get

Get a specific resource-set with metadata.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| include_entries | boolean | No | false | Include file entries |
| limit | integer | No | 100 | Max entries to return |
| offset | integer | No | 0 | Pagination offset |

---

#### resource-set-modify

Add or remove entries from a resource-set.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |
| operation | string | Yes | - | "add" or "remove" |
| paths | string[] | Yes | - | List of entry paths |

**Example:**
```json
{"tool": "resource-set-modify", "params": {
  "name": "favorites",
  "operation": "add",
  "paths": ["/home/user/photo.jpg", "/home/user/video.mp4"]
}}
```

---

#### resource-set-delete

Delete a resource-set.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Resource-set name |

---

#### resource-set-add-child

Create a parent-child edge in the DAG.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| parent | string | Yes | - | Parent resource-set name |
| child | string | Yes | - | Child resource-set name |

**Example:**
```json
{"tool": "resource-set-add-child", "params": {
  "parent": "all-media",
  "child": "photos"
}}
```

---

#### resource-set-remove-child

Remove a parent-child edge from the DAG.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| parent | string | Yes | - | Parent resource-set name |
| child | string | Yes | - | Child resource-set name |

---

### Source Management

#### source-create

Create a new data source.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Unique source name |
| type | string | Yes | - | Source type (see below) |
| target_set | string | Yes | - | Target resource-set name |
| config | object | Yes | - | Type-specific configuration |
| update_mode | string | No | append | replace, append, merge |

**Source Types:**

| Type | Description | Config Fields |
|------|-------------|---------------|
| filesystem.index | One-time scan | path, recursive, max_depth, follow_symlinks |
| filesystem.watch | Real-time monitoring | path, recursive, debounce_ms |
| query | Execute saved query | query_name |
| resource-set | Copy from set | source_set_name |

**Example:**
```json
{"tool": "source-create", "params": {
  "name": "photos-index",
  "type": "filesystem.index",
  "target_set": "photos",
  "config": {
    "path": "/home/user/Photos",
    "recursive": true
  }
}}
```

---

#### source-start

Start a source (for continuous sources like filesystem.watch).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Source name |

---

#### source-stop

Stop a running source.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Source name |

---

#### source-execute

Execute a source once (for one-shot sources).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Source name |

---

#### source-list

List all sources with status.

---

#### source-get

Get a specific source with details.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Source name |

---

#### source-delete

Delete a source.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Source name |

---

### Plan Management

Plans orchestrate indexing and processing workflows.

#### plan-create

Create a new plan.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Unique plan name |
| description | string | No | - | Optional description |
| mode | string | No | oneshot | oneshot, continuous |
| sources | object[] | Yes | - | Array of PlanSource objects |
| conditions | object | No | - | RuleCondition tree |
| outcomes | object[] | No | - | Array of RuleOutcome objects |

**PlanSource Structure:**
```json
{
  "name": "index-photos",
  "type": "filesystem.index",
  "target_set": "photos",
  "config": {
    "path": "/home/user/Photos",
    "recursive": true
  }
}
```

**Example:**
```json
{"tool": "plan-create", "params": {
  "name": "index-media",
  "sources": [
    {
      "name": "index-photos",
      "type": "filesystem.index",
      "target_set": "photos",
      "config": {"path": "/home/user/Photos", "recursive": true}
    }
  ],
  "outcomes": [
    {"type": "selection_set", "selectionSetName": "new-files", "operation": "add"}
  ]
}}
```

---

#### plan-execute

Execute a plan (triggers indexing).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Plan name |

**Response:**
```json
{
  "execution_id": 42,
  "plan_name": "index-media",
  "status": "running",
  "started_at": 1732492800
}
```

---

#### plan-list

List all plans.

---

#### plan-get

Get a specific plan with execution history.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Plan name |
| include_executions | boolean | No | true | Include execution history |
| execution_limit | integer | No | 10 | Max executions to return |

---

#### plan-update

Modify a plan.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Plan name |
| description | string | No | - | New description |
| mode | string | No | - | New mode |
| status | string | No | - | New status (active, paused, disabled) |
| sources | object[] | No | - | New sources array |
| conditions | object | No | - | New conditions |
| outcomes | object[] | No | - | New outcomes |

---

#### plan-delete

Delete a plan.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Plan name |

---

### Classifier Tools

#### rerun-classifier

Regenerate metadata for a file.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| sourcePath | string | Yes | - | Path to source file |
| operations | string[] | No | all | Operations (thumbnail, metadata, etc.) |

---

#### classifier-job-progress

Check classifier job progress.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| jobId | integer | Yes | - | Job ID |

---

### Session Tools

#### session-info

Get session and database information.

**Response:**
```json
{
  "database_path": "/home/user/.mcp-space-browser/disk.db",
  "database_size": 52428800,
  "entry_count": 12345,
  "resource_set_count": 15,
  "source_count": 5,
  "plan_count": 3
}
```

---

## MCP Resource Templates

Resources provide declarative, URI-based access to data.

### Resource Set Templates

| URI Pattern | Description | MIME Type |
|-------------|-------------|-----------|
| `synthesis://selection-sets/{name}` | Resource-set metadata | application/json |
| `synthesis://selection-sets/{name}/entries` | File entries in set | application/json |
| `synthesis://selection-sets/{name}/children` | Child resource-sets (DAG downstream) | application/json |
| `synthesis://selection-sets/{name}/parents` | Parent resource-sets (DAG upstream) | application/json |
| `synthesis://selection-sets/{name}/stats` | Comprehensive statistics | application/json |
| `synthesis://selection-sets/{name}/metrics/{metric}` | Aggregated metric (size, count, files, directories) | application/json |

### Static Resources

| URI | Description |
|-----|-------------|
| `synthesis://nodes` | All filesystem entries |
| `synthesis://selection-sets` | All selection sets |
| `synthesis://jobs` | All indexing jobs |
| `synthesis://jobs/pending` | Pending jobs |
| `synthesis://jobs/running` | Running jobs |
| `synthesis://jobs/completed` | Completed jobs |
| `synthesis://jobs/failed` | Failed jobs |
| `synthesis://metadata` | All generated metadata |
| `synthesis://metadata/thumbnails` | Thumbnail metadata |
| `synthesis://plans` | All plans |
| `synthesis://plan-executions` | All plan executions |

### Parameterized Templates

| URI Pattern | Description |
|-------------|-------------|
| `synthesis://nodes/{path}` | Single filesystem entry |
| `synthesis://selection-sets/{name}` | Selection set by name |
| `synthesis://selection-sets/{name}/entries` | Entries in selection set |
| `synthesis://jobs/{id}` | Job by ID |
| `synthesis://plans/{name}` | Plan by name |
| `synthesis://plans/{name}/executions` | Plan execution history |
| `synthesis://metadata/{hash}` | Metadata by hash |
| `synthesis://nodes/{path}/metadata` | All metadata for entry |
| `synthesis://nodes/{path}/thumbnail` | Thumbnail for entry |

---

## Example Workflows

### 1. Index and Query Filesystem

```json
// Step 1: Create resource-sets
{"tool": "resource-set-create", "params": {"name": "photos"}}
{"tool": "resource-set-create", "params": {"name": "all-media"}}
{"tool": "resource-set-add-child", "params": {"parent": "all-media", "child": "photos"}}

// Step 2: Create and execute plan
{"tool": "plan-create", "params": {
  "name": "index-photos",
  "sources": [{
    "name": "scan-photos",
    "type": "filesystem.index",
    "target_set": "photos",
    "config": {"path": "/home/user/Photos", "recursive": true}
  }]
}}
{"tool": "plan-execute", "params": {"name": "index-photos"}}

// Step 3: Query
{"tool": "resource-sum", "params": {"name": "all-media", "metric": "size"}}
{"tool": "resource-fuzzy-match", "params": {"name": "photos", "field": "path", "pattern": ".jpg", "mode": "suffix"}}
```

### 2. Live Filesystem Monitoring

```json
// Create continuous plan with watch source
{"tool": "plan-create", "params": {
  "name": "watch-downloads",
  "mode": "continuous",
  "sources": [{
    "name": "downloads-watch",
    "type": "filesystem.watch",
    "target_set": "downloads",
    "config": {"path": "/home/user/Downloads", "recursive": true, "debounce_ms": 500}
  }]
}}
{"tool": "plan-execute", "params": {"name": "watch-downloads"}}
```

### 3. Find Large Files

```json
{"tool": "resource-metric-range", "params": {
  "name": "all-files",
  "metric": "size",
  "min": 1073741824
}}
```

---

## Error Handling

All tools return errors in MCP-standard format:

```json
{
  "error": {
    "code": -32602,
    "message": "Invalid params",
    "data": {
      "field": "name",
      "reason": "resource-set not found"
    }
  }
}
```

Common error codes:
- `-32600`: Invalid Request
- `-32601`: Method not found
- `-32602`: Invalid params
- `-32603`: Internal error

---

## See Also

- [C3 Architecture](./ARCHITECTURE_C3.md) - System context and container views
- [Entity-Relationship Diagram](./ENTITY_RELATIONSHIP.md) - Data model documentation
