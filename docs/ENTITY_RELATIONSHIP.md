# Entity-Relationship Diagram

This document describes the data model for mcp-space-browser using an entity-relationship diagram and detailed table specifications.

## Core Primitives

Following Eskil Steenberg's primitive-first design principle, the system is built around these core primitives:

| Primitive | Description | Analogy |
|-----------|-------------|---------|
| **Entry** | A filesystem object (file or directory) | Unix inode |
| **ResourceSet** | A named collection of entries (DAG node) | Named selection/folder |
| **Metadata** | Derived data generated from entries | Thumbnail, timeline, etc. |
| **Plan** | An orchestration of sources and outcomes | Workflow/pipeline |
| **Source** | A data ingestion mechanism | Input channel |
| **Rule** | A condition-outcome automation | Trigger/action |

---

## Entity-Relationship Diagram

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│                              ENTITY-RELATIONSHIP DIAGRAM                                   │
│                                                                                            │
│                                                                                            │
│   ┌──────────────────┐                                                                    │
│   │     entries      │◄───────────────────────────────────────────────────────────────┐   │
│   ├──────────────────┤                                                                 │   │
│   │ PK id            │                                                                 │   │
│   │    path (unique) │◄──────────────────────────────────────────────────────┐        │   │
│   │ FK parent        │──────────────────────────────────────────────────────┐│        │   │
│   │    size          │                                                       ││        │   │
│   │    blocks        │                                                       ││        │   │
│   │    kind          │                                                       ││        │   │
│   │    ctime         │                                                       ││        │   │
│   │    mtime         │                                                       ││        │   │
│   │    last_scanned  │                                                       ││        │   │
│   │    dirty         │                                                       ││        │   │
│   └──────────────────┘                                                       ││        │   │
│           │                                                                   ││        │   │
│           │ self-reference (parent → path)                                    ││        │   │
│           └───────────────────────────────────────────────────────────────────┘│        │   │
│                                                                                │        │   │
│                                                                                │        │   │
│   ┌──────────────────┐          ┌────────────────────────┐                    │        │   │
│   │  resource_sets   │          │ resource_set_entries   │                    │        │   │
│   ├──────────────────┤          ├────────────────────────┤                    │        │   │
│   │ PK id            │◄────────┐│ PK,FK set_id           │────────────────────┘        │   │
│   │    name (unique) │         ││ PK,FK entry_path       │                             │   │
│   │    description   │         ││      added_at          │                             │   │
│   │    created_at    │         │└────────────────────────┘                             │   │
│   │    updated_at    │         │                                                       │   │
│   └──────────────────┘         │                                                       │   │
│           │ ▲                  │                                                       │   │
│           │ │                  │                                                       │   │
│           │ │  DAG edges       │                                                       │   │
│           ▼ │  (multiple       │                                                       │   │
│   ┌────────────────────────┐   │  parents allowed)                                     │   │
│   │  resource_set_edges    │   │                                                       │   │
│   ├────────────────────────┤   │                                                       │   │
│   │ PK,FK parent_id        │───┘                                                       │   │
│   │ PK,FK child_id         │────┐                                                      │   │
│   │      added_at          │    │                                                      │   │
│   │      CHECK != self     │    │                                                      │   │
│   └────────────────────────┘    │                                                      │   │
│                                 │                                                      │   │
│                                                                                        │   │
│   ┌──────────────────────────────────────────────────────────────────────────────────┐│   │
│   │                           DERIVED DATA SYSTEM                                     ││   │
│   │                                                                                   ││   │
│   │   ┌──────────────────┐                                                           ││   │
│   │   │    metadata      │  ◄── Generated artifacts linked to source entries         ││   │
│   │   ├──────────────────┤                                                           ││   │
│   │   │ PK id            │                                                           ││   │
│   │   │    hash (unique) │  ◄── SHA256(source_path + mtime + type) for dedup         ││   │
│   │   │ FK source_path   │──────────────────────────────────────────────────────────┘││   │
│   │   │    metadata_type │  ◄── thumbnail, video-timeline, text-content, etc.        ││   │
│   │   │    mime_type     │  ◄── image/jpeg, application/json, etc.                   ││   │
│   │   │    cache_path    │  ◄── cache/ab/cd/abcd.../thumb.jpg                        ││   │
│   │   │    file_size     │                                                           ││   │
│   │   │    metadata_json │  ◄── Type-specific data (frame index, duration, etc.)     ││   │
│   │   │    created_at    │                                                           ││   │
│   │   └──────────────────┘                                                           ││   │
│   │                                                                                   ││   │
│   │   Cache Directory Structure (content-addressed):                                  ││   │
│   │   ./cache/                                                                        ││   │
│   │   └── ab/                    ◄── First 2 chars of hash                           ││   │
│   │       └── cd/                ◄── Next 2 chars of hash                            ││   │
│   │           └── abcd1234.../   ◄── Full SHA256 hash                                ││   │
│   │               ├── thumb.jpg  ◄── thumbnail artifact                              ││   │
│   │               ├── poster.jpg ◄── video poster                                    ││   │
│   │               ├── timeline_00.jpg                                                 ││   │
│   │               ├── timeline_01.jpg                                                 ││   │
│   │               └── timeline_04.jpg                                                 ││   │
│   │                                                                                   ││   │
│   └──────────────────────────────────────────────────────────────────────────────────┘│   │
│                                                                                        │   │
│                                                                                        │   │
│   ┌──────────────────┐                                                                │   │
│   │     sources      │                                                                │   │
│   ├──────────────────┤                                                                │   │
│   │ PK id            │                                                                │   │
│   │    name (unique) │                                                                │   │
│   │    type          │  ◄── manual, live, scheduled                                  │   │
│   │    root_path     │                                                                │   │
│   │    config_json   │                                                                │   │
│   │    status        │                                                                │   │
│   │    enabled       │                                                                │   │
│   │    created_at    │                                                                │   │
│   │    updated_at    │                                                                │   │
│   │    last_error    │                                                                │   │
│   └──────────────────┘                                                                │   │
│                                                                                        │   │
│                                                                                        │   │
│   ┌──────────────────┐          ┌────────────────────────┐                            │   │
│   │     plans        │          │   plan_executions      │                            │   │
│   ├──────────────────┤          ├────────────────────────┤                            │   │
│   │ PK id            │◄────────┐│ PK id                  │                            │   │
│   │    name (unique) │         ││ FK plan_id             │                            │   │
│   │    description   │         ││    plan_name           │                            │   │
│   │    mode          │         ││    started_at          │                            │   │
│   │    status        │         ││    completed_at        │                            │   │
│   │    sources_json  │         ││    duration_ms         │                            │   │
│   │    conditions_json│        ││    entries_processed   │                            │   │
│   │    outcomes_json │         ││    entries_matched     │                            │   │
│   │    created_at    │         ││    outcomes_applied    │                            │   │
│   │    updated_at    │         ││    status              │                            │   │
│   │    last_run_at   │         ││    error_message       │                            │   │
│   └──────────────────┘         │└────────────────────────┘                            │   │
│                                │           │                                           │   │
│                                │           ▼                                           │   │
│                                │ ┌────────────────────────┐                           │   │
│                                │ │ plan_outcome_records   │                           │   │
│                                │ ├────────────────────────┤                           │   │
│                                │ │ PK id                  │                           │   │
│                                │ │ FK execution_id        │                           │   │
│                                └─│ FK plan_id             │                           │   │
│                                  │ FK entry_path          │───────────────────────────┘   │
│                                  │    outcome_type        │                               │
│                                  │    outcome_data        │                               │
│                                  │    status              │                               │
│                                  │    error_message       │                               │
│                                  │    created_at          │                               │
│                                  └────────────────────────┘                               │
│                                                                                            │
│                                                                                            │
│   ┌──────────────────┐          ┌────────────────────────┐                                │
│   │      rules       │          │   rule_executions      │                                │
│   ├──────────────────┤          ├────────────────────────┤                                │
│   │ PK id            │◄────────┐│ PK id                  │                                │
│   │    name (unique) │         ││ FK rule_id             │                                │
│   │    description   │         ││ FK selection_set_id    │                                │
│   │    enabled       │         ││    executed_at         │                                │
│   │    priority      │         ││    entries_matched     │                                │
│   │    condition_json│         ││    entries_processed   │                                │
│   │    outcome_json  │         ││    status              │                                │
│   │    created_at    │         ││    error_message       │                                │
│   │    updated_at    │         ││    duration_ms         │                                │
│   └──────────────────┘         │└────────────────────────┘                                │
│                                │           │                                               │
│                                │           ▼                                               │
│                                │ ┌────────────────────────┐                               │
│                                │ │   rule_outcomes        │                               │
│                                │ ├────────────────────────┤                               │
│                                │ │ PK id                  │                               │
│                                │ │ FK execution_id        │                               │
│                                └─│ FK selection_set_id    │                               │
│                                  │ FK entry_path          │                               │
│                                  │    outcome_type        │                               │
│                                  │    outcome_data        │                               │
│                                  │    status              │                               │
│                                  │    error_message       │                               │
│                                  │    created_at          │                               │
│                                  └────────────────────────┘                               │
│                                                                                            │
│                                                                                            │
│   ┌──────────────────┐          ┌────────────────────────┐                                │
│   │     queries      │          │   query_executions     │                                │
│   ├──────────────────┤          ├────────────────────────┤                                │
│   │ PK id            │◄────────┐│ PK id                  │                                │
│   │    name (unique) │         ││ FK query_id            │                                │
│   │    description   │         ││    executed_at         │                                │
│   │    query_type    │         ││    duration_ms         │                                │
│   │    query_json    │         ││    files_matched       │                                │
│   │    target_set    │         ││    status              │                                │
│   │    update_mode   │         ││    error_message       │                                │
│   │    created_at    │         │└────────────────────────┘                                │
│   │    updated_at    │         │                                                          │
│   │    last_executed │         │                                                          │
│   │    exec_count    │─────────┘                                                          │
│   └──────────────────┘                                                                    │
│                                                                                            │
│                                                                                            │
│   ┌──────────────────┐          ┌────────────────────────┐                                │
│   │   index_jobs     │          │   classifier_jobs      │                                │
│   ├──────────────────┤          ├────────────────────────┤                                │
│   │ PK id            │          │ PK id                  │                                │
│   │    root_path     │          │    resource_url        │                                │
│   │    status        │          │    local_path          │                                │
│   │    progress      │          │    artifact_types      │  ◄── JSON array               │
│   │    started_at    │          │    status              │                                │
│   │    completed_at  │          │    progress            │                                │
│   │    error         │          │    started_at          │                                │
│   │    metadata      │          │    completed_at        │                                │
│   │    created_at    │          │    error               │                                │
│   │    updated_at    │          │    result              │  ◄── JSON with artifacts      │
│   └──────────────────┘          │    created_at          │                                │
│                                  │    updated_at          │                                │
│                                  └────────────────────────┘                                │
│                                                                                            │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Derived Data (Metadata) System

The metadata system generates and stores derived artifacts from source files. This includes thumbnails, video timeline frames, text extracts, and more.

### Metadata Types

| Type | Description | Generated From | Storage |
|------|-------------|----------------|---------|
| `thumbnail` | Preview image | Images, videos | JPEG in cache |
| `video-timeline` | Timeline frame at specific position | Videos | JPEG in cache |
| `text-content` | Extracted text content | Text files, documents | JSON in cache |
| `audio-metadata` | Audio file properties | Audio files | JSON in metadata_json |

### Hash Computation

The hash uniquely identifies each artifact and enables deduplication:

```
hash = SHA256(source_path + "|" + mtime + "|" + metadata_type + "|" + frame_index)
```

For example, a video timeline frame:
```
source_path: /home/user/Videos/movie.mp4
mtime:       1718441400
type:        video-timeline
frame:       2

hash = SHA256("/home/user/Videos/movie.mp4|1718441400|video-timeline|2")
     = "c2d87596d3601174ce8e3d5a0e7c11d59516dc4fbc93522bff1a41fd47eb8c85"
```

### Cache Path Structure

Artifacts are stored in a content-addressed directory structure:

```
./cache/
├── c2/                           # First 2 chars of hash
│   └── d8/                       # Next 2 chars of hash
│       └── c2d87596d360.../      # Full hash (directory)
│           ├── thumb.jpg         # Thumbnail artifact
│           ├── poster.jpg        # Video poster (first frame)
│           ├── timeline_00.jpg   # Timeline frame 0
│           ├── timeline_01.jpg   # Timeline frame 1
│           ├── timeline_02.jpg   # Timeline frame 2
│           ├── timeline_03.jpg   # Timeline frame 3
│           └── timeline_04.jpg   # Timeline frame 4
└── ab/
    └── cd/
        └── abcd1234.../
            └── content.json      # Text content extraction
```

---

## Example: Video File with Timeline

A video file generates multiple metadata entries:

### Source Entry

```sql
-- entries table
INSERT INTO entries (path, parent, size, kind, mtime) VALUES
('/home/user/Videos/movie.mp4', '/home/user/Videos', 1073741824, 'file', 1718441400);
```

### Generated Metadata (6 entries for one video)

```sql
-- metadata table: thumbnail
INSERT INTO metadata (hash, source_path, metadata_type, mime_type, cache_path, metadata_json)
VALUES (
  'a1b2c3d4...',
  '/home/user/Videos/movie.mp4',
  'thumbnail',
  'image/jpeg',
  'cache/a1/b2/a1b2c3d4.../thumb.jpg',
  NULL
);

-- metadata table: 5 timeline frames
INSERT INTO metadata (hash, source_path, metadata_type, mime_type, cache_path, metadata_json)
VALUES
  ('f1e2d3c4...', '/home/user/Videos/movie.mp4', 'video-timeline', 'image/jpeg',
   'cache/f1/e2/f1e2d3c4.../timeline_00.jpg', '{"frame": 0}'),
  ('g2f3e4d5...', '/home/user/Videos/movie.mp4', 'video-timeline', 'image/jpeg',
   'cache/g2/f3/g2f3e4d5.../timeline_01.jpg', '{"frame": 1}'),
  ('h3g4f5e6...', '/home/user/Videos/movie.mp4', 'video-timeline', 'image/jpeg',
   'cache/h3/g4/h3g4f5e6.../timeline_02.jpg', '{"frame": 2}'),
  ('i4h5g6f7...', '/home/user/Videos/movie.mp4', 'video-timeline', 'image/jpeg',
   'cache/i4/h5/i4h5g6f7.../timeline_03.jpg', '{"frame": 3}'),
  ('j5i6h7g8...', '/home/user/Videos/movie.mp4', 'video-timeline', 'image/jpeg',
   'cache/j5/i6/j5i6h7g8.../timeline_04.jpg', '{"frame": 4}');
```

### Querying Metadata

```sql
-- Get all metadata for a file
SELECT metadata_type, mime_type, cache_path, metadata_json
FROM metadata
WHERE source_path = '/home/user/Videos/movie.mp4';

-- Get only timeline frames, ordered
SELECT cache_path, json_extract(metadata_json, '$.frame') as frame_index
FROM metadata
WHERE source_path = '/home/user/Videos/movie.mp4'
  AND metadata_type = 'video-timeline'
ORDER BY json_extract(metadata_json, '$.frame');

-- Get thumbnail for a file
SELECT cache_path
FROM metadata
WHERE source_path = '/home/user/Videos/movie.mp4'
  AND metadata_type = 'thumbnail';
```

---

## Example: Image File with Thumbnail

An image file generates a single thumbnail:

```sql
-- entries table
INSERT INTO entries (path, parent, size, kind, mtime) VALUES
('/home/user/Photos/vacation.jpg', '/home/user/Photos', 5242880, 'file', 1718355000);

-- metadata table
INSERT INTO metadata (hash, source_path, metadata_type, mime_type, cache_path, file_size)
VALUES (
  'x9y8z7w6...',
  '/home/user/Photos/vacation.jpg',
  'thumbnail',
  'image/jpeg',
  'cache/x9/y8/x9y8z7w6.../thumb.jpg',
  25600
);
```

---

## Example: Text File with Content Extraction

A text file has its content extracted:

```sql
-- entries table
INSERT INTO entries (path, parent, size, kind, mtime) VALUES
('/home/user/Documents/notes.txt', '/home/user/Documents', 4096, 'file', 1718268600);

-- metadata table
INSERT INTO metadata (hash, source_path, metadata_type, mime_type, cache_path, metadata_json)
VALUES (
  'p1q2r3s4...',
  '/home/user/Documents/notes.txt',
  'text-content',
  'application/json',
  'cache/p1/q2/p1q2r3s4.../content.json',
  '{"lines": 42, "words": 1234, "encoding": "utf-8"}'
);
```

---

## Example: Audio File with Metadata

An audio file has its properties extracted:

```sql
-- entries table
INSERT INTO entries (path, parent, size, kind, mtime) VALUES
('/home/user/Music/song.mp3', '/home/user/Music', 8388608, 'file', 1718182200);

-- metadata table (stored in metadata_json, no cache file)
INSERT INTO metadata (hash, source_path, metadata_type, mime_type, cache_path, metadata_json)
VALUES (
  't5u6v7w8...',
  '/home/user/Music/song.mp3',
  'audio-metadata',
  'application/json',
  '',
  '{
    "duration": 245.5,
    "bitrate": 320000,
    "sample_rate": 44100,
    "channels": 2,
    "codec": "mp3",
    "title": "Song Title",
    "artist": "Artist Name"
  }'
);
```

---

## Table Specifications

### Core Tables

#### entries

Primary storage for all indexed filesystem objects.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| path | TEXT | UNIQUE NOT NULL | Absolute filesystem path |
| parent | TEXT | FK → entries.path | Parent directory path |
| size | INTEGER | | File size in bytes |
| blocks | INTEGER | DEFAULT 0 | Disk blocks allocated |
| kind | TEXT | CHECK('file','directory') | Entry type |
| ctime | INTEGER | | Creation time (Unix timestamp) |
| mtime | INTEGER | | Modification time (Unix timestamp) |
| last_scanned | INTEGER | | Last indexing timestamp |
| dirty | INTEGER | DEFAULT 0 | Mark for re-processing |

**Indexes:**
- `idx_entries_parent` on (parent)
- `idx_entries_mtime` on (mtime)

---

#### metadata

Generated artifacts (thumbnails, timelines, etc.) with deduplication.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| hash | TEXT | UNIQUE NOT NULL | SHA256 for deduplication |
| source_path | TEXT | FK → entries.path CASCADE | Original file |
| metadata_type | TEXT | NOT NULL | thumbnail, video-timeline, text-content, audio-metadata |
| mime_type | TEXT | NOT NULL | Generated file MIME type |
| cache_path | TEXT | NOT NULL | Location in cache directory (empty if inline) |
| file_size | INTEGER | DEFAULT 0 | Size of cached file |
| metadata_json | TEXT | | Type-specific structured data |
| created_at | INTEGER | DEFAULT now() | Generation timestamp |

**Indexes:**
- `idx_metadata_source` on (source_path)
- `idx_metadata_type` on (metadata_type)

**metadata_json Examples:**

```json
// video-timeline
{"frame": 2}

// text-content
{"lines": 42, "words": 1234, "encoding": "utf-8"}

// audio-metadata
{"duration": 245.5, "bitrate": 320000, "codec": "mp3"}
```

---

#### resource_sets

Named collections of entries forming a DAG structure.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| name | TEXT | UNIQUE NOT NULL | Human-readable identifier |
| description | TEXT | | Optional description |
| created_at | INTEGER | DEFAULT now() | Creation timestamp |
| updated_at | INTEGER | DEFAULT now() | Last modification |

---

#### resource_set_entries

Join table linking resource sets to filesystem entries.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| set_id | INTEGER | PK, FK → resource_sets.id | Parent set |
| entry_path | TEXT | PK, FK → entries.path | Entry reference |
| added_at | INTEGER | DEFAULT now() | When entry was added |

**Indexes:**
- `idx_set_entries` on (set_id)

---

#### resource_set_edges

DAG edges allowing multiple parents per resource set.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| parent_id | INTEGER | PK, FK → resource_sets.id | Parent node |
| child_id | INTEGER | PK, FK → resource_sets.id | Child node |
| added_at | INTEGER | DEFAULT now() | Edge creation time |

**Constraints:**
- CHECK (parent_id != child_id) - No self-references
- Cycle prevention enforced at application level

**Indexes:**
- `idx_edges_parent` on (parent_id)
- `idx_edges_child` on (child_id)

---

### Source Tables

#### sources

Filesystem sources for indexing and monitoring.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| name | TEXT | UNIQUE NOT NULL | Source identifier |
| type | TEXT | CHECK (enum) | manual, live, scheduled |
| root_path | TEXT | NOT NULL | Directory path to index/watch |
| config_json | TEXT | | Type-specific configuration |
| status | TEXT | CHECK (enum) | stopped, starting, running, stopping, error |
| enabled | INTEGER | DEFAULT 1 | Active/inactive flag |
| created_at | INTEGER | NOT NULL | Creation timestamp |
| updated_at | INTEGER | NOT NULL | Last modification |
| last_error | TEXT | | Error message if any |

**Source Types:**
- `manual`: One-time scan triggered explicitly
- `live`: Real-time monitoring using fsnotify
- `scheduled`: Periodic scan (future)

---

### Query Tables

#### queries

Saved queries for reuse.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| name | TEXT | UNIQUE NOT NULL | Query identifier |
| description | TEXT | | Optional description |
| query_type | TEXT | CHECK (enum) | file_filter, custom_script |
| query_json | TEXT | NOT NULL | Query definition |
| target_resource_set | TEXT | | Target set for results |
| update_mode | TEXT | CHECK (enum) | replace, append, merge |
| created_at | INTEGER | DEFAULT now() | Creation timestamp |
| updated_at | INTEGER | DEFAULT now() | Last modification |
| last_executed | INTEGER | | Last execution timestamp |
| execution_count | INTEGER | DEFAULT 0 | Total executions |

#### query_executions

Tracks query execution history.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| query_id | INTEGER | FK → queries.id CASCADE | Parent query |
| executed_at | INTEGER | DEFAULT now() | Execution time |
| duration_ms | INTEGER | | Execution duration |
| files_matched | INTEGER | | Files found |
| status | TEXT | CHECK (enum) | success, error |
| error_message | TEXT | | Error details if any |

---

### Plan Tables

#### plans

Orchestration layer owning indexing and processing workflows.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| name | TEXT | UNIQUE NOT NULL | Plan identifier |
| description | TEXT | | Optional description |
| mode | TEXT | CHECK (enum) | oneshot, continuous |
| status | TEXT | CHECK (enum) | active, paused, disabled |
| sources_json | TEXT | NOT NULL | PlanSource[] serialized |
| conditions_json | TEXT | | RuleCondition tree serialized |
| outcomes_json | TEXT | | RuleOutcome[] serialized |
| created_at | INTEGER | DEFAULT now() | Creation timestamp |
| updated_at | INTEGER | DEFAULT now() | Last modification |
| last_run_at | INTEGER | | Last execution timestamp |

---

### Job Tables

#### index_jobs

Tracks async indexing operations with progress.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| root_path | TEXT | NOT NULL | Directory being indexed |
| status | TEXT | CHECK (enum) | pending, running, paused, completed, failed, cancelled |
| progress | INTEGER | DEFAULT 0 | 0-100 percentage |
| started_at | INTEGER | | Job start time |
| completed_at | INTEGER | | Job completion time |
| error | TEXT | | Error message if failed |
| metadata | TEXT | | JSON (IndexJobMetadata) |
| created_at | INTEGER | DEFAULT now() | Job creation time |
| updated_at | INTEGER | DEFAULT now() | Last status update |

**IndexJobMetadata Structure:**
```json
{
  "files_processed": 1234,
  "directories_processed": 56,
  "total_size": 1073741824,
  "error_count": 0,
  "worker_count": 4
}
```

#### classifier_jobs

Tracks artifact generation jobs.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| resource_url | TEXT | NOT NULL | File URI being processed |
| local_path | TEXT | | Local filesystem path |
| artifact_types | TEXT | | JSON array of artifact types to generate |
| status | TEXT | CHECK (enum) | pending, running, completed, failed, cancelled |
| progress | INTEGER | DEFAULT 0 | 0-100 percentage |
| started_at | INTEGER | | Job start time |
| completed_at | INTEGER | | Job completion time |
| error | TEXT | | Error message if failed |
| result | TEXT | | JSON with generated artifacts |
| created_at | INTEGER | DEFAULT now() | Job creation time |
| updated_at | INTEGER | DEFAULT now() | Last status update |

**artifact_types Example:**
```json
["thumbnail", "timeline", "metadata"]
```

**result Example:**
```json
{
  "artifacts": [
    {"type": "thumbnail", "hash": "abc123", "mime_type": "image/jpeg", "cache_path": "cache/ab/c1/..."},
    {"type": "video-timeline", "hash": "def456", "mime_type": "image/jpeg", "cache_path": "cache/de/f4/...", "metadata": {"frame": 0}}
  ]
}
```

---

## Relationship Summary

| Relationship | Type | Description |
|--------------|------|-------------|
| entries.parent → entries.path | Self-reference | Directory hierarchy |
| resource_set_entries → entries | Many-to-Many | Set membership |
| resource_set_edges | Many-to-Many | DAG parent-child |
| metadata.source_path → entries | Many-to-One | Generated artifacts |
| plan_executions → plans | Many-to-One | Execution history |
| plan_outcome_records → entries | Many-to-One | Outcome audit |
| classifier_jobs → entries | Many-to-One | Artifact generation jobs |
| query_executions → queries | Many-to-One | Query execution history |

---

## Data Integrity Constraints

1. **Referential Integrity**: All foreign keys enforce ON DELETE CASCADE
2. **Unique Constraints**: Names are unique across entities
3. **Check Constraints**: Enum columns validate allowed values
4. **DAG Integrity**: Cycles prevented at application level via BFS/DFS check
5. **Self-Reference Prevention**: resource_set_edges.parent_id != child_id
6. **Hash Uniqueness**: Metadata entries are deduplicated by content hash

---

## See Also

- [C3 Architecture](./ARCHITECTURE_C3.md) - System context and container views
- [MCP Reference](./MCP_REFERENCE.md) - Complete tool and resource documentation
