# MCP Domain Model for Shell-Style Navigation with Low Context

This proposal frames the MCP interaction as a lightweight shell session. Tools behave like `cd`, `ls`, and `stat`, but keep payloads small by returning handles and HTTP links for expansion. The HTTP server remains the system of record for large directory payloads and deep drill-downs, while the MCP surface stays under a few kilobytes per call.

## Goals and Constraints

- Preserve a conversational, shell-like flow without inflating the model context.
- Make every payload resumable via stable handles (`jobId`, `indexId`, `nodeId`, `selectionId`, `queryId`).
- Always provide URLs for deeper inspection so MCP responses stay concise.
- Keep existing job tracking, progress, and resource-set behaviors intact.

## Core Entities

| Entity | Purpose | MCP Payload | HTTP Expansion |
| --- | --- | --- | --- |
| **ShellSession** | Tracks the current working directory and active index. | `{ sessionId, cwd, indexId?, breadcrumb }` | `/api/sessions/{sessionId}` for hydration; `/api/indexes/{indexId}` for scan state. |
| **ListingEntry** | Summarized file/dir returned from `cd`. | `{ nodeId, name, kind, size, counts, modifiedAt, link }` where `counts` includes child counts or aggregate sizes. | `/api/indexes/{indexId}/nodes/{nodeId}` for children and metadata; `link` points at a paginated listing URL. |
| **NodeDetail** | Rich metadata for a single item. | `{ nodeId, path, kind, size, modifiedAt, permissions?, digest?, symlink?, link }` | `/api/indexes/{indexId}/nodes/{nodeId}/detail` for extended attributes and previews. |
| **JobHandle** | Long-running work such as indexing. | `{ jobId, kind, status, startedAt, progress?, statusUrl }` | `/api/jobs/{jobId}` to stream updates; completion yields an `indexId`. |
| **ResourceSet** | Named bag of node IDs tied to an index. | `{ selectionId, name, indexId, counts, totalSize, entriesUrl }` | `/api/resource-sets/{selectionId}/entries?limit=...` for pagination; mutations via PATCH/POST. |
| **SavedQuery** | Template for deriving resource sets. | `{ queryId, name, filterSummary, lastExecutedAt, runUrl }` | `/api/queries/{queryId}` for the full filter AST; executions materialize resource sets. |

## Minimal MCP Tool Surface

1. **cd** (change-directory)
   - Input: `path` (absolute or relative to `cwd`), optional `limit`/`cursor` for pagination.
   - Output: `{ cwd, breadcrumb, indexId, entries: ListingEntry[] (capped ~20), nextPageUrl }` where each entry includes a `link` to HTTP listing for further pages.

2. **index**
   - Input: `root`, optional `depthLimit`, `followSymlinks`.
   - Output: `{ jobId, indexId?, statusUrl, progressUrl, cwdHint }`; clients can immediately `cd` into `root` while monitoring job progress.

3. **job-progress**
   - Input: `jobId`.
   - Output: `{ status, progressPct?, eta?, indexId?, report?, statusUrl }` with small `report` slices (e.g., top-level counts) and HTTP links for deeper metrics.

4. **inspect**
   - Input: `nodeId` (or `path` scoped to the current `indexId`).
   - Output: `NodeDetail` plus URLs for download, child listing, and historical reports. No large content is inlined.

5. **resource-set-ops** (create/list/get/delete/add/remove)
   - Inputs reference `selectionId` and arrays of `nodeId` only.
   - Outputs: counts, total size, `entriesUrl`, and optional `sample` rows (≤10) with `nodeId`, `name`, `size`, and `link` for detail fetches.

6. **query-run / query-list / query-get**
   - `query-run` returns `{ queryId, selectionId, sampleUrl, report? }` where `report` is a small slice of matches.
   - `query-list` returns compact cards with names and `runUrl`; `query-get` emits the full filter via HTTP only when requested.

7. **session-preferences**
   - Lightweight knobs for units or default limits; never echoes large data.

## HTTP/REST Conventions to Keep Context Small

- **Listings via links**: `/api/indexes/{indexId}/tree?parent={nodeId}&limit=50&cursor=...` powers deep browsing when a chat turn needs more than the MCP `cd` slice.
- **Detail URLs everywhere**: every handle includes a `link` to its canonical HTTP representation; binary downloads stay HTTP-only.
- **Paginated reports**: aggregates like “largest files” live at `/api/indexes/{indexId}/reports/largest?limit=...` and are linked from MCP reports instead of inlined.
- **Explainability toggles**: appending `?format=explain` to query/report endpoints returns plans or filter ASTs without inflating MCP payloads.

## Interaction Patterns

- **Navigate then deepen**: use `cd` to move around and preview children; follow `link` URLs to fetch longer listings outside the chat context.
- **Index and monitor**: start an index with `index`, poll `job-progress`, then keep browsing with `cd` once `indexId` is available.
- **Curate selections**: rely on handles (`selectionId`, `nodeId`) for add/remove operations; fetch full entry lists over HTTP only when human review is required.
- **Budgeted responses**: all MCP responses cap lists (e.g., 20 entries) and supply `nextPageUrl` so the client can page via HTTP when needed.
