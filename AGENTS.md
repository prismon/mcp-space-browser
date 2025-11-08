# Repository Guidelines

## Project Structure & Module Organization
- `src/` hosts the TypeScript runtime: `cli.ts` wires disk commands, `crawler.ts` walks the filesystem, `db.ts` wraps the Bun SQLite layer, and `mcp.ts`/`server.ts` expose HTTP and MCP interfaces. 
- `test/` contains Bun-powered integration-style suites (see `agent.test.ts`, `mcp.test.ts`) that exercise the database and CLI flows with temp directories.
- `docs/` captures design context; keep `docs/disk_agent_design.md` aligned whenever workflows change.
- `disk.db` is the default local datastore; prefer `DiskDB(':memory:')` in tests to avoid polluting the file.

## Build, Test, and Development Commands
- The agent runs on Bun (Node.js is insufficient because we import Bun-specific modules like `bun:sqlite`); install Bun ≥1.1 before contributing.
- Install deps once with `bun install`.
- Run CLI tools directly, e.g. `bun src/cli.ts disk-index ./target` or `bun src/cli.ts disk-tree ./target --sortBy size`.
- Launch services with `bun src/server.ts` (HTTP API) or `bun src/mcp.ts --http-stream` for FastMCP transport.
- Execute the test suite via `bun test`; add `--watch` while iterating locally.

## Coding Style & Naming Conventions
- TypeScript uses strict ESM (`type: module`) and 2-space indentation; keep imports sorted by local vs external modules.
- Prefer `camelCase` for functions/variables, `PascalCase` for classes/interfaces (`DiskDB`, `TreeOptions`), and descriptive file names that mirror the exported symbol.
- Log through `createChildLogger` (Pino) with structured context objects; avoid `console.log` outside user-facing CLI output.
- Use async/await with explicit error handling; guard clauses keep branches shallow.

## Testing Guidelines
- Place new specs in `test/` named `<feature>.test.ts`; co-locate helpers like `check-sets.ts` when reused.
- Leverage Bun's built-in runner (`bun:test`); isolate filesystem effects with `fs.mkdtemp` helpers and in-memory databases.
- Cover both happy-path indexing and error scenarios (missing paths, selection queries) to protect the agent’s automation surface.
- Run `bun test` before submitting and note the results in your PR description.

## Commit & Pull Request Guidelines
- Follow the repo’s concise, capitalized summaries (e.g., `Add FastMCP server and tools`); use additional body lines for context when needed.
- Reference related issues or docs updates in the PR, include CLI transcripts for new flows, and attach screenshots for UI-like outputs (tree listings) when it clarifies behaviour.
- Ensure PRs describe schema or API changes explicitly and flag any backwards-incompatible disk format updates.
