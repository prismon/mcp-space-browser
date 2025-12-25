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

### CORS Support

The MCP server supports Cross-Origin Resource Sharing (CORS) to enable browser-based clients to connect directly without a proxy. This is essential for web-based MCP clients and browser extensions.

**Default Configuration:** By default, all origins are allowed with credentials support enabled.

**Configuration Options:**

| Setting | Environment Variable | YAML Key | Default | Description |
|---------|---------------------|----------|---------|-------------|
| CORS Origins | `CORS_ORIGINS` | `server.cors_origins` | `["*"]` | Comma-separated list of allowed origins |

**Example Configuration (YAML):**
```yaml
server:
  port: 3000
  host: "0.0.0.0"
  cors_origins:
    - "http://localhost:5173"
    - "https://app.example.com"
```

**Example Configuration (Environment Variable):**
```bash
CORS_ORIGINS="http://localhost:5173,https://app.example.com"
```

**CORS Headers Set:**
- `Access-Control-Allow-Origin`: Echoes the request origin (or `*` for wildcard)
- `Access-Control-Allow-Methods`: `GET, POST, PUT, PATCH, DELETE, OPTIONS`
- `Access-Control-Allow-Headers`: `Origin, Content-Type, Accept, Authorization, Mcp-Session-Id, X-Requested-With`
- `Access-Control-Expose-Headers`: `Content-Length, Content-Type, Mcp-Session-Id`
- `Access-Control-Allow-Credentials`: `true` (when credentials are enabled)
- `Access-Control-Max-Age`: `86400` (24 hours)

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

### Navigation Tools

#### navigate

Shell-like directory navigation for exploring indexed filesystem entries. Provides a familiar interface similar to `ls` and `cd` commands.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| path | string | Yes | - | Directory path to navigate to |
| limit | integer | No | 100 | Maximum entries to return |
| offset | integer | No | 0 | Pagination offset |
| sort | string | No | name | Sort by: name, size, mtime |
| desc | boolean | No | false | Sort descending |

**Example:**
```json
{"tool": "navigate", "params": {"path": "/home/user/Photos", "sort": "size", "desc": true}}
```

**Response:**
```json
{
  "path": "/home/user/Photos",
  "entries": [
    {
      "name": "vacation",
      "path": "/home/user/Photos/vacation",
      "size": 1073741824,
      "kind": "directory",
      "mtime": "2024-06-15T10:30:00Z"
    },
    {
      "name": "photo.jpg",
      "path": "/home/user/Photos/photo.jpg",
      "size": 5242880,
      "kind": "file",
      "mtime": "2024-06-15T09:00:00Z",
      "thumbnailUrl": "http://localhost:3000/api/content?path=cache/ab/cd/..."
    }
  ],
  "total": 156,
  "hasMore": true
}
```

---

#### inspect

Get detailed information about a file or directory, including generated artifacts like thumbnails and video timeline frames.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| path | string | Yes | - | Path to inspect |

**Example:**
```json
{"tool": "inspect", "params": {"path": "/home/user/Videos/movie.mp4"}}
```

**Response:**
```json
{
  "path": "/home/user/Videos/movie.mp4",
  "kind": "file",
  "size": 1073741824,
  "modifiedAt": "2024-06-15T10:30:00Z",
  "createdAt": "2024-06-01T08:00:00Z",
  "resourceUri": "synthesis://nodes/home/user/Videos/movie.mp4",
  "thumbnailUri": "http://localhost:3000/api/content?path=cache/ab/cd/.../thumb.jpg",
  "timelineUri": "http://localhost:3000/api/content?path=cache/ab/cd/.../timeline_00.jpg",
  "metadataUri": "synthesis://nodes/home/user/Videos/movie.mp4/metadata",
  "metadataCount": 6,
  "metadata": [
    {"type": "thumbnail", "mimeType": "image/jpeg", "url": "http://..."},
    {"type": "video-timeline", "mimeType": "image/jpeg", "url": "http://...", "metadata": {"frame": 0}},
    {"type": "video-timeline", "mimeType": "image/jpeg", "url": "http://...", "metadata": {"frame": 1}}
  ]
}
```

---

### File Operation Tools

#### rename-files

Rename one or more files or directories.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| renames | object[] | Yes | - | Array of {from, to} rename operations |
| dryRun | boolean | No | false | Preview changes without applying |

**Example:**
```json
{"tool": "rename-files", "params": {
  "renames": [
    {"from": "/home/user/old_name.txt", "to": "/home/user/new_name.txt"}
  ]
}}
```

---

#### delete-files

Delete one or more files or directories.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| paths | string[] | Yes | - | Paths to delete |
| dryRun | boolean | No | false | Preview changes without applying |
| force | boolean | No | false | Force deletion of non-empty directories |

**Example:**
```json
{"tool": "delete-files", "params": {
  "paths": ["/home/user/temp.txt", "/home/user/old_folder"],
  "force": true
}}
```

---

#### move-files

Move files or directories to a new location.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| moves | object[] | Yes | - | Array of {from, to} move operations |
| dryRun | boolean | No | false | Preview changes without applying |

**Example:**
```json
{"tool": "move-files", "params": {
  "moves": [
    {"from": "/home/user/Downloads/file.pdf", "to": "/home/user/Documents/file.pdf"}
  ]
}}
```

---

### Database Tools

#### db-diagnose

Run diagnostics on the database to check integrity and statistics.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| (none) | - | - | - | - |

**Response:**
```json
{
  "database_path": "/path/to/disk.db",
  "database_size": 52428800,
  "integrity_check": "ok",
  "entry_count": 12345,
  "resource_set_count": 15,
  "orphaned_entries": 0,
  "stale_entries": 23
}
```

---

### Query Management Tools

#### query-create

Create a saved query for reuse.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Unique query name |
| description | string | No | - | Optional description |
| query | object | Yes | - | Query definition |

**Example:**
```json
{"tool": "query-create", "params": {
  "name": "large-videos",
  "description": "Videos larger than 1GB",
  "query": {
    "kind": "file",
    "extension": ".mp4",
    "min_size": 1073741824
  }
}}
```

---

#### query-execute

Execute a saved query.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Query name to execute |
| limit | integer | No | 100 | Maximum results |
| offset | integer | No | 0 | Pagination offset |

---

#### query-list

List all saved queries.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| (none) | - | - | - | - |

---

#### query-get

Get a saved query definition.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Query name |

---

#### query-update

Update a saved query.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Query name |
| description | string | No | - | New description |
| query | object | No | - | New query definition |

---

#### query-delete

Delete a saved query.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Query name to delete |

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

Create a new filesystem source for monitoring or indexing.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Unique source name |
| type | string | Yes | - | Source type: `live` or `manual` |
| path | string | Yes | - | Root path to watch/index |
| enabled | boolean | No | true | Whether source should start automatically |
| watch_recursive | boolean | No | true | For live sources: watch subdirectories |
| debounce_ms | integer | No | 500 | For live sources: debounce delay in ms |

**Source Types:**

| Type | Description |
|------|-------------|
| live | Real-time monitoring using fsnotify - detects file changes immediately |
| manual | One-time scan triggered explicitly via source-start |

**Example (live monitoring):**
```json
{"tool": "source-create", "params": {
  "name": "photos-watch",
  "type": "live",
  "path": "/home/user/Photos",
  "watch_recursive": true,
  "debounce_ms": 500
}}
```

**Example (manual indexing):**
```json
{"tool": "source-create", "params": {
  "name": "archive-scan",
  "type": "manual",
  "path": "/mnt/archive",
  "enabled": false
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

#### source-stats

Get statistics for a source including scan history and performance metrics.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| name | string | Yes | - | Source name |

**Response:**
```json
{
  "name": "photos-watch",
  "type": "live",
  "status": "running",
  "entries_indexed": 1234,
  "last_scan_at": "2024-06-15T10:00:00Z",
  "total_scans": 15,
  "average_scan_duration_ms": 2500
}
```

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

#### list-classifier-jobs

List all classifier jobs with optional filtering.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| status | string | No | - | Filter by status (pending, running, completed, failed) |
| limit | integer | No | 50 | Maximum jobs to return |

**Response:**
```json
{
  "jobs": [
    {
      "id": 1,
      "source_path": "/home/user/video.mp4",
      "status": "completed",
      "progress": 100,
      "created_at": "2024-06-15T10:00:00Z",
      "completed_at": "2024-06-15T10:01:00Z"
    }
  ],
  "total": 15
}
```

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

#### session-set-preferences

Set user preferences for the current session.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| preferences | object | Yes | - | Key-value pairs of preferences |

**Example:**
```json
{"tool": "session-set-preferences", "params": {
  "preferences": {
    "default_sort": "size",
    "show_hidden": false,
    "thumbnail_size": "medium"
  }
}}
```

---

## MCP Resource Templates

Resources provide declarative, URI-based access to data.

### Resource Set Templates

| URI Pattern | Description | MIME Type |
|-------------|-------------|-----------|
| `synthesis://resource-sets/{name}` | Resource-set metadata | application/json |
| `synthesis://resource-sets/{name}/entries` | File entries in set | application/json |
| `synthesis://resource-sets/{name}/children` | Child resource-sets (DAG downstream) | application/json |
| `synthesis://resource-sets/{name}/parents` | Parent resource-sets (DAG upstream) | application/json |
| `synthesis://resource-sets/{name}/stats` | Comprehensive statistics | application/json |
| `synthesis://resource-sets/{name}/metrics/{metric}` | Aggregated metric (size, count, files, directories) | application/json |

### Static Resources

| URI | Description |
|-----|-------------|
| `synthesis://nodes` | All filesystem entries |
| `synthesis://resource-sets` | All resource sets |
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
| `synthesis://resource-sets/{name}` | Selection set by name |
| `synthesis://resource-sets/{name}/entries` | Entries in resource set |
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
