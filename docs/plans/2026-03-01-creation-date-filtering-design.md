# Creation-date filtering for things_list_tasks / things_list_projects

**Issue:** https://github.com/wbopan/things-cloud-mcp/issues/3
**Date:** 2026-03-01

## Problem

The `things_list_tasks` and `things_list_projects` tools support filtering by scheduled date and deadline, but not by creation date. This makes it impossible to fetch "recently added" items — a common grooming workflow.

## Decision

Add `created_after` and `created_before` parameters to both `things_list_tasks` and `things_list_projects`. This follows the existing date filter pattern (`scheduled_after/before`, `deadline_after/before`) — consistent, composable, minimal code.

Rejected alternatives:
- **Dedicated `things_list_recent` tool**: contradicts recent tool consolidation direction; can't compose with other filters without duplicating all parameters.
- **Period shorthand (`3d`, `1w`)**: mixed format adds parsing complexity for minimal gain when LLM callers can trivially compute date strings.

## Design

### Parameters

| Parameter | Type | Format | Description |
|---|---|---|---|
| `created_after` | string | `YYYY-MM-DD` | Only return items created after this date (exclusive) |
| `created_before` | string | `YYYY-MM-DD` | Only return items created before this date (exclusive) |

### Filtering logic

- Parse using existing `parseDate()` helper
- Compare against `task.CreationDate` using `.After()` / `.Before()`
- `CreationDate` is `time.Time` (non-nullable, always populated) — no nil check needed, unlike `ScheduledDate`/`DeadlineDate`

### Scope

- `things_list_tasks` — add both params and filtering in `handleListTasks()`
- `things_list_projects` — add both params and filtering in `handleListProjects()`
- Update tool descriptions to mention new parameters
- No output format changes needed (`CreationDate` already in `TaskOutput`)
