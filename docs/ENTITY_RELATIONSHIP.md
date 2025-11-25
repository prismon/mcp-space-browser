# Entity-Relationship Diagram

This document describes the data model for mcp-space-browser using an entity-relationship diagram and detailed table specifications.

## Core Primitives

Following Eskil Steenberg's primitive-first design principle, the system is built around these core primitives:

| Primitive | Description | Analogy |
|-----------|-------------|---------|
| **Entry** | A filesystem object (file or directory) | Unix inode |
| **ResourceSet** | A named collection of entries (DAG node) | Named selection/folder |
| **Plan** | An orchestration of sources and outcomes | Workflow/pipeline |
| **Source** | A data ingestion mechanism | Input channel |
| **Rule** | A condition-outcome automation | Trigger/action |

---

## Entity-Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         ENTITY-RELATIONSHIP DIAGRAM                              │
│                                                                                  │
│                                                                                  │
│   ┌──────────────────┐                                                          │
│   │     entries      │◄─────────────────────────────────────────────────────┐   │
│   ├──────────────────┤                                                       │   │
│   │ PK id            │                                                       │   │
│   │    path (unique) │◄────────────────────────────────────────────┐        │   │
│   │ FK parent        │────────────────────────────────────────────┐│        │   │
│   │    size          │                                             ││        │   │
│   │    kind          │                                             ││        │   │
│   │    ctime         │                                             ││        │   │
│   │    mtime         │                                             ││        │   │
│   │    last_scanned  │                                             ││        │   │
│   │    dirty         │                                             ││        │   │
│   └──────────────────┘                                             ││        │   │
│           │                                                         ││        │   │
│           │ self-reference (parent → path)                          ││        │   │
│           └─────────────────────────────────────────────────────────┘│        │   │
│                                                                      │        │   │
│                                                                      │        │   │
│   ┌──────────────────┐          ┌────────────────────────┐          │        │   │
│   │  resource_sets   │          │ resource_set_entries   │          │        │   │
│   ├──────────────────┤          ├────────────────────────┤          │        │   │
│   │ PK id            │◄────────┐│ PK,FK set_id           │──────────┘        │   │
│   │    name (unique) │         ││ PK,FK entry_path       │                   │   │
│   │    description   │         ││      added_at          │                   │   │
│   │    created_at    │         │└────────────────────────┘                   │   │
│   │    updated_at    │         │                                             │   │
│   └──────────────────┘         │                                             │   │
│           │ ▲                  │                                             │   │
│           │ │                  │                                             │   │
│           │ │  DAG edges       │                                             │   │
│           ▼ │  (multiple       │                                             │   │
│   ┌────────────────────────┐   │  parents allowed)                           │   │
│   │  resource_set_edges    │   │                                             │   │
│   ├────────────────────────┤   │                                             │   │
│   │ PK,FK parent_id        │───┘                                             │   │
│   │ PK,FK child_id         │────┐                                            │   │
│   │      added_at          │    │                                            │   │
│   │      CHECK != self     │    │                                            │   │
│   └────────────────────────┘    │                                            │   │
│                                 │                                             │   │
│   ┌──────────────────┐         │                                             │   │
│   │     sources      │         │                                             │   │
│   ├──────────────────┤         │                                             │   │
│   │ PK id            │         │                                             │   │
│   │    name (unique) │         │                                             │   │
│   │    type          │         │                                             │   │
│   │ FK target_set    │─────────┘ (references resource_sets.name)             │   │
│   │    update_mode   │                                                       │   │
│   │    config_json   │                                                       │   │
│   │    status        │                                                       │   │
│   │    enabled       │                                                       │   │
│   │    created_at    │                                                       │   │
│   │    updated_at    │                                                       │   │
│   │    last_run_at   │                                                       │   │
│   │    last_error    │                                                       │   │
│   └──────────────────┘                                                       │   │
│                                                                               │   │
│                                                                               │   │
│   ┌──────────────────┐          ┌────────────────────────┐                   │   │
│   │     plans        │          │   plan_executions      │                   │   │
│   ├──────────────────┤          ├────────────────────────┤                   │   │
│   │ PK id            │◄────────┐│ PK id                  │                   │   │
│   │    name (unique) │         ││ FK plan_id             │                   │   │
│   │    description   │         ││    plan_name           │                   │   │
│   │    mode          │         ││    started_at          │                   │   │
│   │    status        │         ││    completed_at        │                   │   │
│   │    sources_json  │         ││    duration_ms         │                   │   │
│   │    conditions_json│        ││    entries_processed   │                   │   │
│   │    outcomes_json │         ││    entries_matched     │                   │   │
│   │    created_at    │         ││    outcomes_applied    │                   │   │
│   │    updated_at    │         ││    status              │                   │   │
│   │    last_run_at   │         ││    error_message       │                   │   │
│   └──────────────────┘         │└────────────────────────┘                   │   │
│                                │           │                                  │   │
│                                │           │                                  │   │
│                                │           ▼                                  │   │
│                                │ ┌────────────────────────┐                  │   │
│                                │ │ plan_outcome_records   │                  │   │
│                                │ ├────────────────────────┤                  │   │
│                                │ │ PK id                  │                  │   │
│                                │ │ FK execution_id        │                  │   │
│                                └─│ FK plan_id             │                  │   │
│                                  │ FK entry_path          │──────────────────┘   │
│                                  │    outcome_type        │                      │
│                                  │    outcome_data        │                      │
│                                  │    status              │                      │
│                                  │    error_message       │                      │
│                                  │    created_at          │                      │
│                                  └────────────────────────┘                      │
│                                                                                  │
│                                                                                  │
│   ┌──────────────────┐          ┌────────────────────────┐                      │
│   │      rules       │          │   rule_executions      │                      │
│   ├──────────────────┤          ├────────────────────────┤                      │
│   │ PK id            │◄────────┐│ PK id                  │                      │
│   │    name (unique) │         ││ FK rule_id             │                      │
│   │    description   │         ││ FK selection_set_id    │                      │
│   │    enabled       │         ││    executed_at         │                      │
│   │    priority      │         ││    entries_matched     │                      │
│   │    condition_json│         ││    entries_processed   │                      │
│   │    outcome_json  │         ││    status              │                      │
│   │    created_at    │         ││    error_message       │                      │
│   │    updated_at    │         ││    duration_ms         │                      │
│   └──────────────────┘         │└────────────────────────┘                      │
│                                │           │                                     │
│                                │           ▼                                     │
│                                │ ┌────────────────────────┐                     │
│                                │ │   rule_outcomes        │                     │
│                                │ ├────────────────────────┤                     │
│                                │ │ PK id                  │                     │
│                                │ │ FK execution_id        │                     │
│                                └─│ FK selection_set_id    │                     │
│                                  │ FK entry_path          │                     │
│                                  │    outcome_type        │                     │
│                                  │    outcome_data        │                     │
│                                  │    status              │                     │
│                                  │    error_message       │                     │
│                                  │    created_at          │                     │
│                                  └────────────────────────┘                     │
│                                                                                  │
│                                                                                  │
│   ┌──────────────────┐                                                          │
│   │    metadata      │                                                          │
│   ├──────────────────┤                                                          │
│   │ PK id            │                                                          │
│   │    hash (unique) │  ◄── SHA256 for deduplication                            │
│   │ FK source_path   │──────────────────────────────────────────────────────┘   │
│   │    metadata_type │                                                          │
│   │    mime_type     │                                                          │
│   │    cache_path    │                                                          │
│   │    file_size     │                                                          │
│   │    metadata_json │                                                          │
│   │    created_at    │                                                          │
│   └──────────────────┘                                                          │
│                                                                                  │
│                                                                                  │
│   ┌──────────────────┐                                                          │
│   │   index_jobs     │                                                          │
│   ├──────────────────┤                                                          │
│   │ PK id            │                                                          │
│   │    root_path     │                                                          │
│   │    status        │                                                          │
│   │    progress      │  ◄── 0-100 percentage                                    │
│   │    started_at    │                                                          │
│   │    completed_at  │                                                          │
│   │    error         │                                                          │
│   │    metadata      │  ◄── JSON (files_processed, total_size, etc.)            │
│   │    created_at    │                                                          │
│   │    updated_at    │                                                          │
│   └──────────────────┘                                                          │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
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
| kind | TEXT | CHECK('file','directory') | Entry type |
| ctime | INTEGER | | Creation time (Unix timestamp) |
| mtime | INTEGER | | Modification time (Unix timestamp) |
| last_scanned | INTEGER | | Last indexing timestamp |
| dirty | INTEGER | DEFAULT 0 | Mark for re-processing |

**Indexes:**
- `idx_entries_parent` on (parent)
- `idx_entries_mtime` on (mtime)

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
Unified source abstraction for data ingestion.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| name | TEXT | UNIQUE NOT NULL | Source identifier |
| type | TEXT | CHECK (enum) | filesystem.index, filesystem.watch, query, resource-set |
| target_set_name | TEXT | FK → resource_sets.name | Where to store results |
| update_mode | TEXT | CHECK (enum) | replace, append, merge |
| config_json | TEXT | | Type-specific configuration |
| status | TEXT | CHECK (enum) | stopped, starting, running, stopping, completed, error |
| enabled | INTEGER | DEFAULT 1 | Active/inactive flag |
| created_at | INTEGER | NOT NULL | Creation timestamp |
| updated_at | INTEGER | NOT NULL | Last modification |
| last_run_at | INTEGER | | Last execution timestamp |
| last_error | TEXT | | Error message if any |

**Source Types:**
- `filesystem.index`: One-time directory scan
- `filesystem.watch`: Real-time monitoring (fsnotify)
- `query`: Execute saved query
- `resource-set`: Copy from another set

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

#### plan_executions
Tracks individual plan runs for auditing.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| plan_id | INTEGER | FK → plans.id CASCADE | Parent plan |
| plan_name | TEXT | NOT NULL | Plan name (denormalized) |
| started_at | INTEGER | NOT NULL | Execution start |
| completed_at | INTEGER | | Execution end |
| duration_ms | INTEGER | | Execution duration |
| entries_processed | INTEGER | DEFAULT 0 | Total entries scanned |
| entries_matched | INTEGER | DEFAULT 0 | Entries matching conditions |
| outcomes_applied | INTEGER | DEFAULT 0 | Outcomes executed |
| status | TEXT | CHECK (enum) | running, success, partial, error |
| error_message | TEXT | | Error details if any |

**Indexes:**
- `idx_plan_executions_plan` on (plan_id)
- `idx_plan_executions_status` on (status)

---

#### plan_outcome_records
Audit trail for individual outcome applications.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| execution_id | INTEGER | FK → plan_executions.id CASCADE | Parent execution |
| plan_id | INTEGER | FK → plans.id CASCADE | Parent plan |
| entry_path | TEXT | FK → entries.path CASCADE | Affected entry |
| outcome_type | TEXT | NOT NULL | selection_set, classifier, etc. |
| outcome_data | TEXT | | JSON details |
| status | TEXT | CHECK (enum) | success, error |
| error_message | TEXT | | Error details if any |
| created_at | INTEGER | DEFAULT now() | Record timestamp |

**Indexes:**
- `idx_plan_outcomes_execution` on (execution_id)
- `idx_plan_outcomes_plan` on (plan_id)
- `idx_plan_outcomes_entry` on (entry_path)

---

### Rule Tables

#### rules
Automation rules with conditions and outcomes.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| name | TEXT | UNIQUE NOT NULL | Rule identifier |
| description | TEXT | | Optional description |
| enabled | INTEGER | DEFAULT 1 | Active/inactive flag |
| priority | INTEGER | DEFAULT 0 | Execution order (higher = first) |
| condition_json | TEXT | NOT NULL | RuleCondition serialized |
| outcome_json | TEXT | NOT NULL | RuleOutcome[] serialized |
| created_at | INTEGER | DEFAULT now() | Creation timestamp |
| updated_at | INTEGER | DEFAULT now() | Last modification |

---

### Metadata Tables

#### metadata
Generated metadata (thumbnails, extracted data) with deduplication.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PRIMARY KEY | Auto-increment ID |
| hash | TEXT | UNIQUE NOT NULL | SHA256 for deduplication |
| source_path | TEXT | FK → entries.path CASCADE | Original file |
| metadata_type | TEXT | NOT NULL | thumbnail, video-timeline, exif, etc. |
| mime_type | TEXT | NOT NULL | Generated file type |
| cache_path | TEXT | NOT NULL | Location in cache directory |
| file_size | INTEGER | DEFAULT 0 | Size of metadata file |
| metadata_json | TEXT | | Additional structured data |
| created_at | INTEGER | DEFAULT now() | Generation timestamp |

**Indexes:**
- `idx_metadata_source` on (source_path)
- `idx_metadata_type` on (metadata_type)

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

**Indexes:**
- `idx_jobs_status` on (status)

---

## Relationship Summary

| Relationship | Type | Description |
|--------------|------|-------------|
| entries.parent → entries.path | Self-reference | Directory hierarchy |
| resource_set_entries → entries | Many-to-Many | Set membership |
| resource_set_edges | Many-to-Many | DAG parent-child |
| sources.target_set → resource_sets | Many-to-One | Source target |
| plan_executions → plans | Many-to-One | Execution history |
| plan_outcome_records → entries | Many-to-One | Outcome audit |
| metadata.source_path → entries | Many-to-One | Generated metadata |

---

## Data Integrity Constraints

1. **Referential Integrity**: All foreign keys enforce ON DELETE CASCADE
2. **Unique Constraints**: Names are unique across entities
3. **Check Constraints**: Enum columns validate allowed values
4. **DAG Integrity**: Cycles prevented at application level via BFS/DFS check
5. **Self-Reference Prevention**: resource_set_edges.parent_id != child_id

---

## See Also

- [C3 Architecture](./ARCHITECTURE_C3.md) - System context and container views
- [MCP Reference](./MCP_REFERENCE.md) - Complete tool and resource documentation
