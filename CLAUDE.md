# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
/usr/local/go/bin/go build -o things-cloud-mcp .    # build
./things-cloud-mcp                                   # run (default port 8080)
```

No test suite or linter configured. Verify changes with `go build ./...`.

Environment variables: `PORT` (default 8080), `JWT_SECRET` (base64, auto-generated if unset), `DATA_DIR` (default `data/`, for SQLite), `THINGS_DEBUG` (enables SDK debug logging).

## Architecture

Single-package Go MCP server (~3 files): `main.go` (tools + state), `oauth.go` (OAuth 2.1 + JWT), `landing.go` (web UI).

**Multi-user data flow:**
1. HTTP request arrives with `Authorization: Basic` (email:password) or `Bearer` (JWT from OAuth flow)
2. `UserManager.httpContextFunc()` stores credentials in context
3. `wrap()` extracts user → `UserManager.GetOrCreateUser()` → per-user `ThingsMCP` instance (cached by email)
4. Each `ThingsMCP` holds its own `thingscloud.Client`, `thingscloud.History` (sync cursor), and `memory.State` (in-memory task graph)
5. Read handlers call `syncAndRebuild()` then query `state.Tasks`; write handlers call `writeAndSync()`

**Key SDK types** (from `github.com/arthursoares/things-cloud-sdk`):
- `Task.CreationDate` is `time.Time` (non-nullable); `ScheduledDate`, `DeadlineDate`, `CompletionDate` are `*time.Time`
- `TaskType`: 0=task, 1=project, 2=heading. `TaskStatus`: 0=pending, 2=canceled, 3=completed
- `TaskSchedule`: 0=inbox, 1=anytime, 2=someday

## MCP Tool Design

Before adding or modifying MCP tool definitions, check the [MCP builder skill](https://github.com/anthropics/skills/blob/main/skills/mcp-builder/SKILL.md) for current guidelines on tool naming, parameter descriptions, enum design, output structure, and behavioral annotations.

## Coding Patterns

**Tool handlers** follow: `func (t *ThingsMCP) handle<Name>(ctx, req) (*CallToolResult, error)`. Tool definitions use `mcp.NewTool()` with `mcp.WithString/WithBoolean` params. All registered via `wrap()` which handles user extraction. Exception: `things_diagnose` uses a custom handler with `extractCredentials()` because it needs raw email/password to create a fresh diagnostic client.

**Wire format for writes** uses abbreviated JSON fields (`tt`=title, `nt`=note, `st`=schedule, `dd`=deadline). Notes require CRC32 checksums: `WireNote{_t: "tx", ch: crc32, v: text, t: 1}`. Use `newTaskUpdate()` builder for constructing updates.

**Date handling**: `parseDate()` parses `YYYY-MM-DD` → `*time.Time`. Date filters are exclusive (strict `.Before()`/`.After()`). Output uses ISO8601. Zero-value dates (year ≤ 1970) are filtered from output.

**Recurrence**: User-facing strings (`daily`, `weekly:mon,wed`, `monthly:15`, `every 3 days`) converted to Things Cloud wire format (`rrv`, `fu`, `fa`, `of`, `wd`). Weekly uses `wd` (weekday bitmask), not `dy`.

**Error/result helpers**: `errResult(msg)` for errors, `jsonResult(v)` for success. Validation functions (`validateTaskUUID`, `validateOpts`) run before writes.

**Diagnostic helpers**: `diagStepDefs` defines step metadata, `addSkippedSteps(report, fromStep)` marks remaining steps as skipped on failure, `extractCredentials(ctx, um)` resolves email/password from context (shared by `getUserFromContext` and `things_diagnose`). JSON output uses camelCase keys throughout.

## OAuth (oauth.go)

OAuth 2.1 with PKCE. State persisted in SQLite (`DATA_DIR/oauth.db`): clients, auth codes, refresh tokens, JWT secret. Endpoints: `/authorize`, `/token`, `/register`, `/.well-known/oauth-*`.
