# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**mcp-space-browser** is a disk space indexing agent built with Bun and TypeScript. It crawls filesystems, stores metadata in SQLite, and provides tools for exploring disk utilization (similar to Baobab/WinDirStat).

## Essential Commands

### Development
```bash
# Run CLI commands
bun src/cli.ts disk-index <path>    # Index a directory tree
bun src/cli.ts disk-du <path>        # Show disk usage for path
bun src/cli.ts disk-tree <path>      # Display tree view with sizes

# Run HTTP server
bun src/server.ts                    # Starts on port 3000

# Run tests
bun test                             # Runs all tests
bun test test/agent.test.ts          # Run specific test file
```

### Build & Type Checking
```bash
# TypeScript type checking (Bun handles TypeScript natively, no build needed)
bunx tsc --noEmit                    # Type check without emitting files
```

## Architecture

### Core Components
1. **CLI Entry Point** (`src/cli.ts`): Command-line interface with three commands
2. **Filesystem Crawler** (`src/crawler.ts`): DFS traversal, metadata collection, database updates
3. **Database Layer** (`src/db.ts`): SQLite abstraction with single `entries` table
4. **HTTP Server** (`src/server.ts`): API endpoints for indexing and tree data

### Key Design Patterns
- **Single Table Design**: All filesystem entries in one table with parent references
- **Incremental Updates**: Uses `last_scanned` timestamp to detect/remove stale entries
- **Post-Processing Aggregation**: Directory sizes computed after crawling completes
- **In-Memory Testing**: Tests use temporary directories and `:memory:` SQLite

### Database Schema
```sql
CREATE TABLE entries (
  id INTEGER PRIMARY KEY,
  path TEXT UNIQUE NOT NULL,
  parent TEXT,
  size INTEGER,
  kind TEXT CHECK(kind IN ('file', 'directory')),
  ctime INTEGER,
  mtime INTEGER,
  last_scanned INTEGER,
  dirty INTEGER DEFAULT 0
)
```

### API Endpoints
- `GET /api/index?path=...` - Trigger filesystem indexing
- `GET /api/tree?path=...` - Get hierarchical JSON data

## Development Guidelines

### Runtime Environment
- Uses Bun runtime exclusively (not Node.js)
- Bun provides built-in SQLite support via `bun:sqlite`
- TypeScript runs directly without compilation step

### Testing Approach
- Tests use Bun's built-in test runner (`bun:test`)
- Create temporary directories with `withTempDir` helper
- Use in-memory SQLite databases for test isolation
- Verify both crawling behavior and aggregate calculations

### Logging
- Uses Pino for structured logging with `pino-pretty` formatter
- Logger configuration in `src/logger.ts`
- Logging levels: trace, debug, info, warn, error
- Silent during tests (NODE_ENV=test)
- Set custom log level: `LOG_LEVEL=debug bun src/cli.ts disk-index /path`
- Each module has its own child logger with contextual name

### Future MCP Integration
The design document (`docs/disk_agent_design.md`) outlines plans for full MCP (Model Context Protocol) agent implementation. Current code is structured to facilitate this future integration.