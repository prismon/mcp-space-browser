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
│           │  SQLite     │ │  Local      │ │  Metadata   │                        │
│           │  Database   │ │  Filesystem │ │  Cache      │                        │
│           │             │ │             │ │             │                        │
│           └─────────────┘ └─────────────┘ └─────────────┘                        │
│                                                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Context Actors

| Actor | Type | Description |
|-------|------|-------------|
| **Human User** | Person | Interacts with system through MCP client tools |
| **LLM (Claude/ChatGPT)** | System | AI models that use MCP tools for disk analysis |
| **MCP Client Application** | System | Any application implementing MCP client protocol |
| **SQLite Database** | Data Store | Persistent storage for all system state |
| **Local Filesystem** | External System | The filesystem being indexed and monitored |
| **Metadata Cache** | Data Store | Generated thumbnails and extracted metadata |

### Key Architectural Decision

> **MCP-Only Interface**: This system is ONLY exposed through the Model Context Protocol (MCP). There are NO REST APIs, gRPC interfaces, or other external access methods. All interaction happens via MCP tools and resource templates over JSON-RPC 2.0.

---

## C3 Level 2: Container View

The Container diagram shows the major deployable units within mcp-space-browser.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              mcp-space-browser                                    │
│                                                                                   │
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                           MCP SERVER LAYER                                   │ │
│  │                                                                              │ │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                      MCP Transport (Gin HTTP)                         │  │ │
│  │  │                      POST /mcp (Streamable HTTP)                      │  │ │
│  │  └──────────────────────────────────────────────────────────────────────┘  │ │
│  │                                    │                                        │ │
│  │           ┌────────────────────────┼────────────────────────┐               │ │
│  │           ▼                        ▼                        ▼               │ │
│  │  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │ │
│  │  │  MCP Tools      │     │  MCP Resources  │     │  MCP Templates  │       │ │
│  │  │  (30+ tools)    │     │  (Static URIs)  │     │  (Dynamic URIs) │       │ │
│  │  └─────────────────┘     └─────────────────┘     └─────────────────┘       │ │
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
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐                         │ │
│  │  │   Crawler    │ │ Queue Mgmt   │ │  Path Utils  │                         │ │
│  │  │              │ │              │ │              │                         │ │
│  │  │ Filesystem   │ │ Job Tracking │ │ Validation   │                         │ │
│  │  │ Traversal    │ │ & Progress   │ │ & Expansion  │                         │ │
│  │  └──────────────┘ └──────────────┘ └──────────────┘                         │ │
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
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐                         │  │ │
│  │  │  │  Sources   │ │   Jobs     │ │  Metadata  │                         │  │ │
│  │  │  │  CRUD      │ │   CRUD     │ │   CRUD     │                         │  │ │
│  │  │  └────────────┘ └────────────┘ └────────────┘                         │  │ │
│  │  │                                                                        │  │ │
│  │  └──────────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                              │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                       │                                          │
│                                       ▼                                          │
│                         ┌──────────────────────────┐                            │
│                         │      SQLite Database      │                            │
│                         │     (~/.mcp-space-browser │                            │
│                         │        /disk.db)          │                            │
│                         └──────────────────────────┘                            │
│                                                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Container Descriptions

| Container | Technology | Responsibility |
|-----------|------------|----------------|
| **MCP Transport** | Gin HTTP, mcp-go | JSON-RPC 2.0 transport, stateless sessions |
| **MCP Tools** | Go handlers | 30+ tools for resource manipulation |
| **MCP Resources** | Go handlers | Static and template-based resource URIs |
| **Plans** | Go package | Orchestration of indexing and processing |
| **Sources** | Go package | Data ingestion (filesystem, watch, query) |
| **Rules** | Go package | Condition evaluation and outcome application |
| **Classifier** | Go package | Media analysis and metadata extraction |
| **Crawler** | Go package | DFS filesystem traversal with parallel workers |
| **Queue Management** | Go package | Async job tracking with progress |
| **Database** | Go + SQLite | All persistence operations |

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
│          │ 5. Evaluate conditions                     │ 6. Store entries      │
│          ▼                                            ▼                       │
│   ┌─────────────┐                              ┌─────────────┐               │
│   │ Condition   │                              │  Database   │               │
│   │ Evaluator   │                              │  (entries)  │               │
│   └──────┬──────┘                              └─────────────┘               │
│          │                                                                     │
│          │ 7. Apply outcomes                                                   │
│          ▼                                                                     │
│   ┌─────────────┐      8. Update sets          ┌─────────────┐               │
│   │  Outcome    │─────────────────────────────▶│  Database   │               │
│   │  Applier    │                              │(resource_   │               │
│   └─────────────┘                              │ sets)       │               │
│                                                 └─────────────┘               │
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
type PlanManager interface {
    CreatePlan(plan *models.Plan) error
    GetPlan(name string) (*models.Plan, error)
    ListPlans() ([]*models.Plan, error)
    UpdatePlan(plan *models.Plan) error
    DeletePlan(name string) error
    ExecutePlan(name string) (*models.PlanExecution, error)
    StopPlan(name string) error
}
```

### Sources Module Interface

```go
// pkg/sources - Black Box Interface
type SourceManager interface {
    CreateSource(src *models.Source) error
    StartSource(name string) error
    StopSource(name string) error
    ExecuteSource(name string) error
    GetSourceStatus(name string) (*SourceStatus, error)
}
```

### Database Module Interface

```go
// pkg/database - Black Box Interface
type DiskDB interface {
    // Entries
    GetEntry(path string) (*models.Entry, error)
    UpsertEntry(entry *models.Entry) error
    DeleteEntry(path string) error

    // ResourceSets
    CreateResourceSet(set *models.ResourceSet) error
    GetResourceSet(name string) (*models.ResourceSet, error)
    AddToResourceSet(name string, paths []string) error
    RemoveFromResourceSet(name string, paths []string) error

    // Plans, Sources, Rules...
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
│   │   │                                                                   │ │ │
│   │   └─────────────────────────────────────────────────────────────────┘ │ │
│   │                                    │                                    │ │
│   │   ┌─────────────────────────────────┼─────────────────────────────────┐│ │
│   │   │                                ▼                                   ││ │
│   │   │   ~/.mcp-space-browser/                                           ││ │
│   │   │   ├── config.yaml          # Configuration                        ││ │
│   │   │   ├── disk.db              # SQLite database                      ││ │
│   │   │   └── cache/               # Metadata cache (thumbnails, etc.)    ││ │
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
| **Cache** | Local filesystem directory for generated metadata |
| **Networking** | Single HTTP port for MCP transport |

---

## Technology Stack Summary

| Layer | Technology | Purpose |
|-------|------------|---------|
| **Transport** | Gin HTTP | HTTP server and routing |
| **Protocol** | mcp-go | MCP JSON-RPC 2.0 implementation |
| **Database** | go-sqlite3 | Embedded SQLite with CGO |
| **Monitoring** | fsnotify | Filesystem event watching |
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

### Testing
- Unit tests with in-memory SQLite (`:memory:`)
- Temporary directories via `t.TempDir()`
- **Minimum 80% code coverage required**

---

## See Also

- [Entity-Relationship Diagram](./ENTITY_RELATIONSHIP.md) - Data model documentation
- [MCP Reference](./MCP_REFERENCE.md) - Complete tool and resource documentation
- [CLAUDE.md](../CLAUDE.md) - Development guidelines and principles
