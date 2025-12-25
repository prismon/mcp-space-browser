# C3 Architecture: Context and Container Views

This document provides C3 (a subset of C4) architecture diagrams for the mcp-space-browser system, documenting the system context and internal container structure.

## Design Philosophy

This architecture follows Eskil Steenberg's principles for building large-scale maintainable systems:

1. **Black Box Interfaces**: Every module has a clean, documented API with hidden implementation details
2. **Replaceable Components**: Any module can be rewritten from scratch using only its interface
3. **Single Responsibility**: One module = one clear purpose that one person can maintain
4. **Primitive-First Design**: Core primitives (Entry, ResourceSet, Plan) flow through the system consistently

---

## C3 Level 1: System Context

The System Context diagram shows how mcp-space-browser interacts with external actors.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              SYSTEM CONTEXT                                       │
│                                                                                   │
│  ┌─────────────┐         ┌─────────────┐         ┌─────────────────────┐        │
│  │   Human     │         │    LLM      │         │   MCP Client        │        │
│  │   User      │         │  (Claude/   │         │   Application       │        │
│  │             │         │   ChatGPT)  │         │                     │        │
│  └──────┬──────┘         └──────┬──────┘         └──────────┬──────────┘        │
│         │                       │                           │                    │
│         │    MCP Protocol       │      MCP Protocol         │                    │
│         │    (JSON-RPC 2.0)     │      (JSON-RPC 2.0)        │                    │
│         │                       │                           │                    │
│         └───────────────────────┼───────────────────────────┘                    │
│                                 │                                                 │
│                                 ▼                                                 │
│                    ╔═══════════════════════════╗                                 │
│                    ║   mcp-space-browser       ║                                 │
│                    ║                           ║                                 │
│                    ║   Disk Space Indexing     ║                                 │
│                    ║   and Analysis Agent      ║                                 │
│                    ║                           ║                                 │
│                    ║   [Go Application]        ║                                 │
│                    ╚═══════════════════════════╝                                 │
│                                 │                                                 │
│                    ┌────────────┼────────────┐                                   │
│                    │            │            │                                    │
│                    ▼            ▼            ▼                                    │
│           ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                        │
│           │  SQLite     │ │  Local      │ │  Artifact   │                        │
│           │  Database   │ │  Filesystem │ │  Cache      │                        │
│           │  (disk.db)  │ │             │ │  (./cache)  │                        │
│           └─────────────┘ └─────────────┘ └─────────────┘                        │
│                                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────────┐│
│  │                         Web Frontend                                         ││
│  │  ┌─────────────┐                                                             ││
│  │  │   Browser   │───────── HTTP ──────────▶ /web/* (Static Assets)           ││
│  │  │   Client    │───────── HTTP ──────────▶ /api/content (Thumbnails)        ││
│  │  │             │───────── MCP  ──────────▶ /mcp (JSON-RPC 2.0)              ││
│  │  └─────────────┘                                                             ││
│  └─────────────────────────────────────────────────────────────────────────────┘│
│                                                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Context Actors

| Actor | Type | Description |
|-------|------|-------------|
| **Human User** | Person | Interacts with system through MCP client tools or web frontend |
| **LLM (Claude/ChatGPT)** | System | AI models that use MCP tools for disk analysis |
| **MCP Client Application** | System | Any application implementing MCP client protocol |
| **Browser Client** | System | Web frontend for visual exploration |
| **SQLite Database** | Data Store | Persistent storage for all system state |
| **Local Filesystem** | External System | The filesystem being indexed and monitored |
| **Artifact Cache** | Data Store | Generated thumbnails and extracted metadata |

### Key Architectural Decision

> **MCP-Only Interface**: This system is ONLY exposed through the Model Context Protocol (MCP). There are NO REST APIs for data operations. The only HTTP endpoints are:
> - `POST /mcp` - MCP JSON-RPC 2.0 transport
> - `GET /api/content` - Serve cached artifacts (thumbnails, metadata files)
> - `GET /web/*` - Static web frontend assets

---

## C3 Level 2: Container View

The Container diagram shows the major deployable units within mcp-space-browser.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              mcp-space-browser                                    │
│                                                                                   │
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                           HTTP SERVER LAYER                                  │ │
│  │                                                                              │ │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                      Gin HTTP Server                                  │  │ │
│  │  │  POST /mcp ──────────────────▶ MCP Transport (Streamable HTTP)       │  │ │
│  │  │  GET /api/content ───────────▶ Artifact Content Server               │  │ │
│  │  │  GET /api/inspect ───────────▶ File Inspection API                   │  │ │
│  │  │  GET /web/* ─────────────────▶ Static Web Assets                     │  │ │
│  │  └──────────────────────────────────────────────────────────────────────┘  │ │
│  │                                    │                                        │ │
│  └────────────────────────────────────┼────────────────────────────────────────┘ │
│                                       │                                          │
│  ┌────────────────────────────────────┼────────────────────────────────────────┐ │
│  │                          MCP TOOLS LAYER (42 tools)                          │ │
│  │                                                                              │ │
│  │  ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ ┌──────────────┐  │ │
│  │  │  Navigation    │ │   Resource     │ │    Source      │ │    Plan      │  │ │
│  │  │  Tools (4)     │ │   Tools (16)   │ │   Tools (7)    │ │  Tools (6)   │  │ │
│  │  │                │ │                │ │                │ │              │  │ │
│  │  │ • index        │ │ • resource-sum │ │ • source-create│ │ • plan-create│  │ │
│  │  │ • navigate     │ │ • resource-is  │ │ • source-start │ │ • plan-execute│ │ │
│  │  │ • inspect      │ │ • resource-... │ │ • source-stop  │ │ • plan-list  │  │ │
│  │  │ • job-progress │ │ • resource-set-│ │ • source-list  │ │ • plan-get   │  │ │
│  │  └────────────────┘ └────────────────┘ └────────────────┘ └──────────────┘  │ │
│  │                                                                              │ │
│  │  ┌────────────────┐ ┌────────────────┐ ┌────────────────┐                   │ │
│  │  │  Classifier    │ │   File Ops     │ │    Query       │                   │ │
│  │  │  Tools (3)     │ │   Tools (3)    │ │   Tools (6)    │                   │ │
│  │  │                │ │                │ │                │                   │ │
│  │  │ • rerun-class..│ │ • rename-files │ │ • query-create │                   │ │
│  │  │ • classifier-..│ │ • delete-files │ │ • query-execute│                   │ │
│  │  │ • list-class.. │ │ • move-files   │ │ • query-...    │                   │ │
│  │  └────────────────┘ └────────────────┘ └────────────────┘                   │ │
│  │                                                                              │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                       │                                          │
│  ┌────────────────────────────────────┼───────────────────────────────────────┐ │
│  │                        DOMAIN LAYER                                          │ │
│  │                                                                              │ │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │ │
│  │  │    Plans     │ │   Sources    │ │    Rules     │ │  Classifier  │       │ │
│  │  │              │ │              │ │              │ │              │       │ │
│  │  │ Orchestration│ │ Data Ingest  │ │ Automation   │ │ Media        │       │ │
│  │  │ & Execution  │ │ & Monitoring │ │ Engine       │ │ Analysis     │       │ │
│  │  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘       │ │
│  │         │                │                │                │                │ │
│  │         └────────────────┴────────────────┴────────────────┘                │ │
│  │                                   │                                          │ │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │ │
│  │  │   Crawler    │ │ Write Queue  │ │   Progress   │ │  Path Utils  │       │ │
│  │  │              │ │              │ │   Tracker    │ │              │       │ │
│  │  │ Filesystem   │ │ Async DB     │ │ Job Status   │ │ Validation   │       │ │
│  │  │ Traversal    │ │ Writes       │ │ Updates      │ │ & Expansion  │       │ │
│  │  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘       │ │
│  │                                                                              │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                       │                                          │
│  ┌────────────────────────────────────┼───────────────────────────────────────┐ │
│  │                      DATA ACCESS LAYER                                       │ │
│  │                                                                              │ │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                        Database (pkg/database)                        │  │ │
│  │  │                                                                        │  │ │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐         │  │ │
│  │  │  │  Entries   │ │ ResourceSet│ │   Plans    │ │   Rules    │         │  │ │
│  │  │  │  CRUD      │ │   CRUD     │ │   CRUD     │ │   CRUD     │         │  │ │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘         │  │ │
│  │  │                                                                        │  │ │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐         │  │ │
│  │  │  │  Sources   │ │   Jobs     │ │  Metadata  │ │  Queries   │         │  │ │
│  │  │  │  CRUD      │ │   CRUD     │ │   CRUD     │ │   CRUD     │         │  │ │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘         │  │ │
│  │  │                                                                        │  │ │
│  │  └──────────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                              │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                       │                                          │
│                          ┌────────────┴────────────┐                            │
│                          │                         │                            │
│                          ▼                         ▼                            │
│             ┌──────────────────────┐  ┌──────────────────────┐                 │
│             │   SQLite Database    │  │   Artifact Cache     │                 │
│             │      (disk.db)       │  │     (./cache/)       │                 │
│             │                      │  │                      │                 │
│             │ • entries            │  │ • Thumbnails         │                 │
│             │ • resource_sets      │  │ • Video posters      │                 │
│             │ • plans              │  │ • Timeline frames    │                 │
│             │ • sources            │  │ • Extracted metadata │                 │
│             │ • metadata           │  │                      │                 │
│             │ • jobs               │  │ Content-addressed    │                 │
│             │ • queries            │  │ storage (SHA256)     │                 │
│             └──────────────────────┘  └──────────────────────┘                 │
│                                                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Container Descriptions

| Container | Technology | Responsibility |
|-----------|------------|----------------|
| **HTTP Server** | Gin HTTP | Routes requests to MCP transport or content server |
| **MCP Transport** | mcp-go | JSON-RPC 2.0 transport, stateless sessions |
| **MCP Tools** | Go handlers | 42 tools for resource manipulation |
| **Content Server** | Gin handler | Serves cached thumbnails and metadata files |
| **Plans** | Go package | Orchestration of indexing and processing |
| **Sources** | Go package | Data ingestion (filesystem, watch, query) |
| **Rules** | Go package | Condition evaluation and outcome application |
| **Classifier** | Go package | Media analysis, thumbnail generation, FFmpeg |
| **Crawler** | Go package | DFS filesystem traversal with parallel workers |
| **Write Queue** | Go package | Async database writes for performance |
| **Progress Tracker** | Go package | Real-time job status updates |
| **Database** | Go + SQLite | All persistence operations |
| **Artifact Cache** | Filesystem | Content-addressed storage for generated files |

---

## Component Interactions

### Data Flow: Indexing via Plans

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      INDEXING DATA FLOW                                       │
│                                                                               │
│   MCP Client                                                                  │
│       │                                                                       │
│       │ 1. plan-execute("my-plan")                                            │
│       ▼                                                                       │
│   ┌─────────────┐                                                             │
│   │ MCP Server  │                                                             │
│   └──────┬──────┘                                                             │
│          │                                                                     │
│          │ 2. Execute plan                                                     │
│          ▼                                                                     │
│   ┌─────────────┐      3. Resolve sources      ┌─────────────┐               │
│   │   Plans     │─────────────────────────────▶│   Sources   │               │
│   │   Manager   │                              │   Manager   │               │
│   └──────┬──────┘                              └──────┬──────┘               │
│          │                                            │                       │
│          │                              4. Start crawl│                       │
│          │                                            ▼                       │
│          │                                     ┌─────────────┐               │
│          │                                     │   Crawler   │               │
│          │                                     │ (Parallel)  │               │
│          │                                     └──────┬──────┘               │
│          │                                            │                       │
│          │                                            │ 5. Write Queue        │
│          │                                            ▼                       │
│          │                                     ┌─────────────┐               │
│          │                                     │ Write Queue │               │
│          │                                     │  (Async)    │               │
│          │                                     └──────┬──────┘               │
│          │                                            │                       │
│          │ 6. Evaluate conditions                     │ 7. Batch insert       │
│          ▼                                            ▼                       │
│   ┌─────────────┐                              ┌─────────────┐               │
│   │ Condition   │                              │  Database   │               │
│   │ Evaluator   │                              │  (entries)  │               │
│   └──────┬──────┘                              └─────────────┘               │
│          │                                                                     │
│          │ 8. Apply outcomes                                                   │
│          ▼                                                                     │
│   ┌─────────────┐      9. Generate artifacts   ┌─────────────┐               │
│   │  Outcome    │─────────────────────────────▶│ Classifier  │               │
│   │  Applier    │                              │  (FFmpeg)   │               │
│   └─────────────┘                              └──────┬──────┘               │
│                                                       │                       │
│                                                       │ 10. Store in cache    │
│                                                       ▼                       │
│                                                ┌─────────────┐               │
│                                                │  Artifact   │               │
│                                                │   Cache     │               │
│                                                └─────────────┘               │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow: File Inspection with Thumbnails

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      INSPECT DATA FLOW                                        │
│                                                                               │
│   Web Frontend / MCP Client                                                   │
│       │                                                                       │
│       │ 1. inspect(path="/path/to/video.mp4")                                 │
│       ▼                                                                       │
│   ┌─────────────┐                                                             │
│   │ MCP Server  │                                                             │
│   └──────┬──────┘                                                             │
│          │                                                                     │
│          │ 2. Get entry metadata                                               │
│          ▼                                                                     │
│   ┌─────────────┐                                                             │
│   │  Database   │                                                             │
│   │  (entries)  │                                                             │
│   └──────┬──────┘                                                             │
│          │                                                                     │
│          │ 3. Generate artifacts if needed                                     │
│          ▼                                                                     │
│   ┌─────────────┐      4. Extract frames       ┌─────────────┐               │
│   │ Classifier  │─────────────────────────────▶│   FFmpeg    │               │
│   │  Manager    │                              │             │               │
│   └──────┬──────┘                              └──────┬──────┘               │
│          │                                            │                       │
│          │                              5. Write files│                       │
│          │                                            ▼                       │
│          │                                     ┌─────────────┐               │
│          │                                     │  Artifact   │               │
│          │                                     │   Cache     │               │
│          │                                     │ (./cache/)  │               │
│          │                                     └─────────────┘               │
│          │                                                                     │
│          │ 6. Store metadata                                                   │
│          ▼                                                                     │
│   ┌─────────────┐                                                             │
│   │  Database   │                                                             │
│   │ (metadata)  │                                                             │
│   └──────┬──────┘                                                             │
│          │                                                                     │
│          │ 7. Return response with HTTP URLs                                   │
│          ▼                                                                     │
│   {                                                                            │
│     "path": "/path/to/video.mp4",                                             │
│     "thumbnailUri": "http://host/api/content?path=cache/ab/cd/...",           │
│     "metadata": [                                                              │
│       {"type": "thumbnail", "url": "http://..."},                             │
│       {"type": "video-timeline", "url": "http://...", "metadata": {"frame":0}}│
│     ]                                                                          │
│   }                                                                            │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow: Resource Query

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      QUERY DATA FLOW                                          │
│                                                                               │
│   MCP Client                                                                  │
│       │                                                                       │
│       │ 1. resource-sum("photos", metric="size")                              │
│       ▼                                                                       │
│   ┌─────────────┐                                                             │
│   │ MCP Server  │                                                             │
│   └──────┬──────┘                                                             │
│          │                                                                     │
│          │ 2. Build ResourceQuery                                              │
│          ▼                                                                     │
│   ┌─────────────┐                                                             │
│   │   Query     │                                                             │
│   │  Builder    │                                                             │
│   └──────┬──────┘                                                             │
│          │                                                                     │
│          │ 3. Generate SQL with recursive CTE                                  │
│          ▼                                                                     │
│   ┌─────────────┐                                                             │
│   │  Database   │──────────────────┐                                          │
│   │  (query)    │                  │                                          │
│   └─────────────┘                  │                                          │
│                                    │ 4. DAG traversal via CTE                 │
│                                    ▼                                          │
│   ┌────────────────────────────────────────────────────────────────────────┐ │
│   │  WITH RECURSIVE descendants AS (                                        │ │
│   │    SELECT id FROM resource_sets WHERE name = 'photos'                   │ │
│   │    UNION ALL                                                             │ │
│   │    SELECT e.child_id FROM resource_set_edges e                          │ │
│   │    JOIN descendants d ON e.parent_id = d.id                             │ │
│   │  )                                                                       │ │
│   │  SELECT SUM(size) FROM entries e                                        │ │
│   │  JOIN resource_set_entries rse ON e.path = rse.entry_path              │ │
│   │  WHERE rse.set_id IN (SELECT id FROM descendants)                       │ │
│   └────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                          │
│                                    │ 5. Return aggregated result              │
│                                    ▼                                          │
│                          {"resource_set": "photos",                           │
│                           "metric": "size",                                   │
│                           "value": 524288000}                                 │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Module Boundaries (Black Box Interfaces)

Each module exposes a clean interface hiding implementation details:

### Plans Module Interface

```go
// pkg/plans - Black Box Interface
type Executor interface {
    ExecutePlan(ctx context.Context, plan *models.Plan) (*ExecutionResult, error)
}

type ExecutionResult struct {
    ExecutionID      int64
    PlanName         string
    Status           string
    EntriesProcessed int
    EntriesMatched   int
    OutcomesApplied  int
    Duration         time.Duration
    Error            error
}
```

### Sources Module Interface

```go
// pkg/sources - Black Box Interface
type Manager interface {
    CreateSource(ctx context.Context, src *models.Source) error
    StartSource(ctx context.Context, name string) error
    StopSource(ctx context.Context, name string) error
    GetStatus(ctx context.Context, name string) (*SourceStatus, error)
    RestoreActiveSources(ctx context.Context) error
    StopAll(ctx context.Context) error
}
```

### Classifier Module Interface

```go
// pkg/classifier - Black Box Interface
type Classifier interface {
    DetectMediaType(path string) MediaType
    CanProcess(path string) bool
}

type Manager interface {
    GenerateThumbnail(req *ArtifactRequest) *ArtifactResult
    GenerateTimelineFrame(req *ArtifactRequest) *ArtifactResult
}

type MetadataManager interface {
    CanExtractMetadata(path string) bool
    ExtractMetadata(path string, maxSize int64) *MetadataResult
}
```

### Database Module Interface

```go
// pkg/database - Black Box Interface
type DiskDB interface {
    // Entries
    Get(path string) (*models.Entry, error)
    Upsert(entry *models.Entry) error
    Delete(path string) error
    GetChildren(path string) ([]*models.Entry, error)

    // ResourceSets
    CreateResourceSet(set *models.ResourceSet) error
    GetResourceSet(name string) (*models.ResourceSet, error)
    AddToResourceSet(name string, paths []string) error
    RemoveFromResourceSet(name string, paths []string) error

    // Metadata
    CreateOrUpdateMetadata(metadata *models.Metadata) error
    GetMetadata(hash string) (*models.Metadata, error)
    GetMetadataByPath(path string) ([]*models.Metadata, error)
    GetMetadataByCachePath(cachePath string) (*models.Metadata, error)

    // Plans, Sources, Rules, Jobs, Queries...
    // (similar CRUD patterns)
}
```

---

## Deployment View

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DEPLOYMENT                                            │
│                                                                               │
│   ┌───────────────────────────────────────────────────────────────────────┐ │
│   │                        Host Machine                                    │ │
│   │                                                                         │ │
│   │   ┌─────────────────────────────────────────────────────────────────┐ │ │
│   │   │           mcp-space-browser (Single Static Binary)              │ │ │
│   │   │                                                                   │ │ │
│   │   │   • No runtime dependencies                                      │ │ │
│   │   │   • Embeds SQLite driver                                        │ │ │
│   │   │   • Listens on configurable port (default: 3000)                │ │ │
│   │   │   • MCP endpoint: POST /mcp                                      │ │ │
│   │   │   • Content endpoint: GET /api/content                           │ │ │
│   │   │   • Web frontend: GET /web/*                                     │ │ │
│   │   │                                                                   │ │ │
│   │   └─────────────────────────────────────────────────────────────────┘ │ │
│   │                                    │                                    │ │
│   │   ┌─────────────────────────────────┼─────────────────────────────────┐│ │
│   │   │                                ▼                                   ││ │
│   │   │   Working Directory (configurable)                                ││ │
│   │   │   ├── config.yaml           # Configuration                       ││ │
│   │   │   ├── disk.db               # SQLite database                     ││ │
│   │   │   └── cache/                # Artifact cache                      ││ │
│   │   │       ├── ab/               # First 2 chars of SHA256             ││ │
│   │   │       │   └── cd/           # Next 2 chars                        ││ │
│   │   │       │       └── abcd.../  # Full hash directory                 ││ │
│   │   │       │           ├── thumb.jpg                                   ││ │
│   │   │       │           ├── poster.jpg                                  ││ │
│   │   │       │           └── timeline_00.jpg                             ││ │
│   │   │       └── ...                                                     ││ │
│   │   │                                                                    ││ │
│   │   └────────────────────────────────────────────────────────────────────┘│ │
│   │                                                                         │ │
│   └───────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Deployment Characteristics

| Aspect | Description |
|--------|-------------|
| **Binary** | Single statically-linked Go executable |
| **Dependencies** | None (SQLite embedded via CGO) |
| **Configuration** | YAML file + environment variables + CLI flags |
| **Data Storage** | SQLite database file (portable) |
| **Cache** | Content-addressed filesystem directory (SHA256 hashes) |
| **Networking** | Single HTTP port for all endpoints |

---

## Technology Stack Summary

| Layer | Technology | Purpose |
|-------|------------|---------|
| **Transport** | Gin HTTP | HTTP server and routing |
| **Protocol** | mcp-go v0.43.0 | MCP JSON-RPC 2.0 implementation |
| **Database** | go-sqlite3 | Embedded SQLite with CGO |
| **Monitoring** | fsnotify | Filesystem event watching |
| **Media** | FFmpeg (external) | Thumbnail and timeline generation |
| **Logging** | logrus | Structured logging with colors |
| **CLI** | cobra | Command-line interface |
| **Testing** | testify | Test assertions and mocks |

---

## Cross-Cutting Concerns

### Logging
- Structured logging via logrus
- Per-module child loggers with contextual names
- Silent during tests (GO_ENV=test)
- Configurable log levels (trace, debug, info, warn, error)

### Error Handling
- Errors propagate up through clean interfaces
- Database errors include context
- MCP tools return structured error responses

### Caching
- Content-addressed artifact storage using SHA256 hashes
- Path normalization handles both absolute and relative paths
- Lazy generation on first access via inspect tool

### Testing
- Unit tests with in-memory SQLite (`:memory:`)
- Temporary directories via `t.TempDir()`
- **Minimum 80% code coverage required**

---

## See Also

- [Entity-Relationship Diagram](./ENTITY_RELATIONSHIP.md) - Data model documentation
- [MCP Reference](./MCP_REFERENCE.md) - Complete tool and resource documentation
- [CLAUDE.md](../CLAUDE.md) - Development guidelines and principles
