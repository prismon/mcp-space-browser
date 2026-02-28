# Radical Consolidation Design

Collapse 59 MCP tools into 5 composable tools. Introduce first-class attribute system. Strip documentation to essentials.

## 5-Tool Interface

### `scan`

Indexes filesystem paths and extracts attributes. Replaces `index`, `navigate`, `inspect`, and job tools.

```json
{
  "tool": "scan",
  "params": {
    "paths": ["/home/user/Photos", "/home/user/Documents"],
    "attributes": ["size", "mime", "hash.md5", "exif", "permissions"],
    "depth": -1,
    "force": false,
    "target": "my-set",
    "async": true
  }
}
```

Parameters:
- `paths` (string[]): One or more filesystem paths to scan.
- `attributes` (string[]): Which attributes to extract beyond base set. Options: `mime`, `hash.md5`, `hash.sha256`, `hash.perceptual`, `exif`, `permissions`, `thumbnail`, `video.thumbnails`, `media`, `text`.
- `depth` (int): `-1` = recursive, `0` = this level only, `N` = N levels deep.
- `force` (bool): Re-index even if recently scanned. Default `false`.
- `target` (string): Resource set name to populate with results.
- `async` (bool): Return job ID immediately for long operations. Default `true`.

Base attributes are always collected: `path`, `parent`, `name`, `extension`, `kind`, `size`, `ctime`, `mtime`, `atime`.

### `query`

Unified search, filter, and aggregation. Replaces all `resource-*` tools and query tools.

```json
{
  "tool": "query",
  "params": {
    "from": "my-set",
    "where": {
      "kind": "file",
      "size": {">": 1048576},
      "mime": {"like": "image/%"},
      "mtime": {"after": "2024-01-01"}
    },
    "select": ["path", "size", "mime", "mtime"],
    "aggregate": "size",
    "group_by": "mime",
    "order_by": "-size",
    "limit": 100,
    "cursor": "base64-encoded-next-page-token"
  }
}
```

Parameters:
- `from` (string): Resource set name, or omit for global search across all entries.
- `where` (object): Composable filters. Keys are attribute names, values are exact matches or operator objects (`>`, `<`, `>=`, `<=`, `like`, `after`, `before`, `in`, `not`).
- `select` (string[]): Which fields to return. Defaults to all base attributes.
- `aggregate` (string): Aggregation function — `sum`, `count`, `avg`, `min`, `max`.
- `group_by` (string): Group aggregation results by this field.
- `order_by` (string): Sort field. Prefix `-` for descending.
- `limit` (int): Max results. Default 100.
- `cursor` (string): Opaque token for pagination. Responses over limit will return a `next_cursor`.

### `manage`

CRUD for organizational entities.

```json
{
  "tool": "manage",
  "params": {
    "entity": "resource-set",
    "action": "create",
    "name": "vacation-photos",
    "description": "Photos from 2024 vacation"
  }
}
```

Parameters:
- `entity` (string, required): `resource-set`, `plan`, `source`, `project`, `job`.
- `action` (string, required): `create`, `get`, `list`, `update`, `delete`.
- `limit` (int): Pagination limit for list actions. Default 100.
- `cursor` (string): Pagination cursor for list actions.
- Remaining parameters are entity-specific.

Entity-specific parameters:

**resource-set**: `name`, `description`, `parent` (for DAG edges), `child` (for DAG edges).
**plan**: `name`, `description`, `mode` (`oneshot`/`continuous`), `sources` (array).
**source**: `name`, `type`, `target_set`, `config`.
**project**: `name`, `path`.
**job**: `id` (for get), `status` filter (for list).

### `batch`

Multi-file operations.

```json
{
  "tool": "batch",
  "params": {
    "operation": "duplicates",
    "from": "my-set",
    "method": "perceptual",
    "threshold": 8,
    "async": true
  }
}
```

Operations:
- `attributes`: Bulk attribute extraction for a set of paths or resource set.
- `duplicates`: Find exact (`hash.md5`) or visual (`hash.perceptual`) duplicates.
- `compare`: Cross-file comparison (size diff, attribute diff).
- `move`: Bulk move files.
- `delete`: Bulk delete files.

Parameters:
- `operation` (string, required): One of the above.
- `from` (string): Resource set to operate on.
- `paths` (string[]): Explicit file paths (alternative to `from`).
- `method` (string): For `duplicates` — `exact` or `perceptual`.
- `threshold` (int): For perceptual duplicates — hamming distance threshold.
- `async` (bool): Return job ID for long operations.

### `watch`

Real-time filesystem monitoring.

```json
{
  "tool": "watch",
  "params": {
    "action": "start",
    "path": "/home/user/Downloads",
    "target": "downloads",
    "recursive": true,
    "rules": [
      {
        "condition": {"mime": {"like": "image/%"}},
        "action": "add-to-set",
        "set": "images"
      }
    ]
  }
}
```

Parameters:
- `action` (string, required): `start`, `stop`, `status`, `list`.
- `path` (string): Filesystem path to watch.
- `target` (string): Resource set to populate.
- `recursive` (bool): Watch subdirectories. Default `true`.
- `rules` (array): Automatic classification rules applied on file events.
- `debounce_ms` (int): Debounce interval for rapid changes. Default `500`.

## Attribute System

### Categories

**Base attributes** (always collected on scan):
- `path`, `parent`, `name`, `extension`
- `kind` (file / directory / symlink)
- `size` (bytes)
- `ctime`, `mtime`, `atime` (Unix timestamps)

**Content attributes** (requested via `scan.attributes`):
- `mime` — MIME type detection
- `hash.md5`, `hash.sha256` — content hashes
- `hash.perceptual` — perceptual hash for visual similarity (pHash/dHash)
- `media.width`, `media.height`, `media.duration` — image/video/audio dimensions
- `exif.*` — EXIF metadata (camera, GPS, date, etc.)
- `text.lines`, `text.encoding` — text file properties
- `thumbnail` — single thumbnail for images (base64 JPEG or file ref)
- `video.thumbnail.{N}` — video thumbnails at 5% intervals (0, 5, 10, ... 95)

**Derived attributes** (computed by query aggregations):
- `dir.total_size` — recursive directory size
- `dir.file_count`, `dir.dir_count` — child counts
- `dir.depth` — nesting depth

**Usage attributes** (computed over time from multiple scans):
- `growth_rate` — size change between scans
- `access_frequency` — atime change frequency
- `age` — time since creation

### Storage

Two tables:

1. **`entries`** — base attributes (unchanged from current schema).
2. **`attributes`** — extensible key-value store:

```sql
CREATE TABLE attributes (
  entry_path TEXT NOT NULL,
  key TEXT NOT NULL,
  value TEXT,
  source TEXT,
  computed_at INTEGER,
  PRIMARY KEY (entry_path, key),
  FOREIGN KEY (entry_path) REFERENCES entries(path) ON DELETE CASCADE
)

CREATE INDEX idx_attributes_key ON attributes(key);
CREATE INDEX idx_attributes_source ON attributes(source);
```

The existing `features` table merges into `attributes`. Thumbnails stored as `thumbnail` and `video.thumbnail.{N}` keys with base64-encoded values or file path references.

## Resource Templates

8 templates replacing the current 31:

| URI | Returns |
|-----|---------|
| `synthesis://entries/{path}` | Single entry with all attributes |
| `synthesis://entries/{path}/attributes` | Just the attributes for an entry |
| `synthesis://sets` | List all resource sets |
| `synthesis://sets/{name}` | Set details + child sets |
| `synthesis://sets/{name}/entries` | Entries in a set |
| `synthesis://jobs` | Active/recent jobs |
| `synthesis://jobs/{id}` | Job status + progress |
| `synthesis://projects` | List all projects |

## Async Model

- `scan` and `batch` with `async: true` return `{"job_id": "..."}` immediately.
- Poll via `manage({entity: "job", action: "get", id: "..."})`.
- Or read resource template `synthesis://jobs/{id}`.
- Attribute extraction is partial-failure tolerant: if EXIF fails, other attributes still succeed.
- `batch` reports per-file results.

## Cleanup Scope

### Code: Delete

| File | Lines | Reason |
|------|-------|--------|
| `pkg/server/mcp_tools.go` | 3,789 | Replaced by 5 tool files |
| `pkg/server/mcp_resource_tools.go` | 771 | Folded into `query` tool |
| `pkg/server/mcp_source_tools.go` | 375 | Folded into `manage` and `watch` |
| `pkg/server/mcp_project_tools.go` | 484 | Folded into `manage` |
| `pkg/server/mcp_classifier_tools.go` | 344 | Folded into `scan` attributes |
| `pkg/server/mcp_resources.go` | 1,426 | Rewritten with 8 templates |

### Code: Keep and Refactor

- `pkg/database/` — add `attributes` table, merge `features` table into it.
- `pkg/crawler/` — add attribute extraction hooks.
- `pkg/server/server.go` — keep HTTP/CORS setup.
- `pkg/server/context.go` — keep multi-project support.
- `pkg/classifier/` — wire into attribute system.

### Code: Consolidate

- `pkg/sources/` + `pkg/source/` — merge into one package.
- `pkg/plans/` — simplify as thin wrappers.

### Code: New Files

- `pkg/server/tool_scan.go`
- `pkg/server/tool_query.go`
- `pkg/server/tool_manage.go`
- `pkg/server/tool_batch.go`
- `pkg/server/tool_watch.go`
- `pkg/database/attributes.go`

### Docs: Delete All 14 Files in `docs/`

Historical replatform docs, migration guides, and implementation plans are no longer relevant.

### Docs: Write Fresh

- `docs/ARCHITECTURE.md` — system overview, component diagram, data flow.
- `docs/MCP_REFERENCE.md` — the 5 tools, parameters, examples.
- `docs/SCHEMA.md` — database tables and attribute model.

### Docs: Update

- `CLAUDE.md` — reflect new 5-tool design.

### Web: Follow-on

Web components map to the old interface. Updating them is a separate task after the MCP tool layer is stable.
