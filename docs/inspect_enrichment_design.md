# Inspect Command Data Enrichment Design

## Background and Goals

The existing `inspect` tool returns basic metadata (path, kind, size, timestamps, link) for an indexed node. We want richer inspection results that also surface URLs to the raw content plus generated assets such as thumbnails, video timelines, and AI-authored summaries. The design keeps the existing indexing pipeline, but layers specialized indexers and AI jobs that can be registered to emit additional artifacts without blocking the user-facing inspect call.

## Desired Outputs

Every inspection should be able to return:

- **Content URL**: a dereferenceable link (e.g., `synthesis://nodes/<path>` or HTTP download URL) for the file contents.
- **Generated artifacts** (zero or more):
  - Static thumbnail for images, PDFs, or videos.
  - Timeline of thumbnails or poster frames for movies.
  - AI-generated summary (and optionally key frames or captions for media).
- **Provenance metadata**: which indexer produced the artifact, when, and the job ID so callers can poll status.

Artifacts and summaries should be optional and populated asynchronously so the base inspect response is fast.

## Data Model Additions

We will extend SQLite with inspection-aware tables alongside the existing `entries` and `index_jobs` tables:

- `inspection_jobs`: one row per enrichment task (e.g., `video-thumbnail`, `video-timeline`, `ai-summary`). Columns include `id`, `entry_path`, `job_type`, `status`, `progress`, `started_at`, `completed_at`, `error`, `metadata` (JSON such as frame count or model name), and timestamps. This mirrors the semantics of `index_jobs` but is decoupled so different job types can run concurrently.
- `inspection_artifacts`: stores materialized outputs keyed by `job_id` and `entry_path`. Columns include `id`, `entry_path`, `job_id`, `artifact_type` (content, thumbnail, timeline, summary), `mime_type`, `url`, `size`, `checksum`, `metadata` (JSON describing frame timestamps or summary tokens), and `created_at`.
- Indexes on `entry_path`, `artifact_type`, and `job_type` support paginated queries by resource type.

These tables keep the core `entries` table untouched while enabling multiple artifacts per path and multiple runs over time.

## Indexer and AI Job Model

Enrichment is implemented as **registrable jobs** so new generators can be plugged in without changing the inspect tool logic:

- **Job Registry**: a Go interface such as `type InspectionJob interface { Type() string; Supports(entry models.Entry) bool; Enqueue(ctx, entry) (jobID int64, err error) }` backed by a registry map. Indexers self-register during server startup.
- **Built-in indexers**:
  - *Movie indexer*: uses `ffmpeg`/`ffprobe` to extract a poster frame and generate a configurable number of timeline thumbnails. Each output is written to a cache directory and recorded as `thumbnail` or `timeline` artifacts with URLs pointing to HTTP or `synthesis://` download handlers.
  - *Image/document indexer*: captures a single thumbnail using an image processing library.
  - *AI summarizer*: streams file bytes (or transcript for media) to an LLM, stores the summary as a `summary` artifact, and records the model name and prompt in artifact metadata.
- **Execution flow**: when `inspect` is invoked, the server checks whether artifacts already exist. Missing artifacts trigger job enqueues for each matching indexer. Jobs run asynchronously via workers that update `inspection_jobs` status/progress and insert artifacts upon completion.

## Inspect API Shape and Pagination

The `inspect` MCP tool should return a stable JSON envelope:

```json
{
  "path": "/videos/trip.mp4",
  "kind": "file",
  "size": 1234567,
  "createdAt": "2024-05-01T12:00:00Z",
  "modifiedAt": "2024-06-01T09:30:00Z",
  "contentUrl": "synthesis://nodes//videos/trip.mp4",
  "artifacts": [
    { "id": 10, "type": "thumbnail", "mimeType": "image/jpeg", "url": "synthesis://artifacts/10" },
    { "id": 11, "type": "summary", "mimeType": "application/json", "url": "synthesis://artifacts/11" }
  ],
  "jobs": [
    { "id": 42, "type": "video-timeline", "status": "running", "progress": 30 }
  ],
  "nextPageUrl": "synthesis://inspect?path=/videos/trip.mp4&offset=2&limit=2"
}
```

Pagination applies to the `artifacts` array and optional `jobs` listing:

- Requests accept `offset` and `limit` to slice artifacts by `artifact_type`, defaulting to 20. The response includes `nextPageUrl` when more data is available.
- Clients can filter by `type` (e.g., `type=thumbnail` or `type=summary`) to retrieve a specific artifact kind efficiently.
- Job listings include recent runs for the entry so callers can check whether enrichment is pending or failed before re-enqueueing.

## Workflow Scenarios

1. **First-time inspect**: user calls `inspect /videos/trip.mp4`. The response returns base metadata and `contentUrl`; artifact slots are empty but job IDs for `video-thumbnail`, `video-timeline`, and `ai-summary` are enqueued and returned so the client can poll.
2. **Follow-up inspect**: after workers finish, calling `inspect` again (or following pagination links) returns populated artifacts. If new versions are needed (e.g., higher resolution thumbnails), a fresh job can be enqueued and recorded as a new artifact row.
3. **Partial data**: for large videos, only the first page of timeline thumbnails is returned. Clients paginate to fetch subsequent frames without overloading responses.

## Operational Considerations

- **Storage and caching**: artifacts are stored under a configurable cache root with deterministic filenames so `contentUrl` links remain stable even across restarts. Thumbnails and video timelines are written into hashed directory shards (e.g., `<cache>/<h0>/<h1>/<hash>/poster.jpg`) to keep lookup fast and avoid large flat directories.
- **Security**: URLs should be scoped to the MCP transport (e.g., `synthesis://` URLs or authenticated HTTP) and not expose arbitrary filesystem paths. Artifact metadata must include the source entry path for traceability.
- **Backpressure**: job workers enforce concurrency limits per job type (e.g., `ffmpeg` pool size) and skip enqueuing if a recent successful artifact already exists unless the caller requests a refresh.
- **Telemetry**: job metadata captures generation duration, frame counts, and model token usage to aid monitoring and cost control.

This design gives the `inspect` command a richer, extensible set of outputs while keeping responses fast through asynchronous, paginated artifact delivery.
