# Status Filter Design

## Problem

The `things_list_tasks` tool uses `is_completed` (bool) to toggle visibility of non-active tasks. This has two issues:

1. **Canceled tasks leak through.** Tasks with status=2 (canceled) and `schedule: "inbox"` appear in inbox queries because only status=3 (completed) was filtered. This caused old canceled items (dating back to 2017) to show up alongside active inbox tasks.
2. **No granularity.** `is_completed=true` shows both completed and canceled tasks together with no way to query one without the other.

## Design

Replace `is_completed` (bool) with `status` (string enum) on `things_list_tasks` and `things_list_projects`.

### Parameter

```go
mcp.WithString("status",
    mcp.Description("Filter by task status (default: pending — only active tasks)"),
    mcp.Enum("pending", "completed", "canceled"),
)
```

### Filtering behavior

| `status` value | Shows tasks with | SDK value |
|---|---|---|
| omitted / `"pending"` | `TaskStatusPending` (0) | Active tasks only |
| `"completed"` | `TaskStatusCompleted` (3) | Completed tasks only |
| `"canceled"` | `TaskStatusCanceled` (2) | Canceled tasks only |

### Affected tools

| Tool | Change |
|---|---|
| `things_list_tasks` | Replace `is_completed` with `status` enum |
| `things_list_projects` | Add `status` enum (currently hardcoded to exclude non-pending) |
| `things_show_project` | Add `status` enum for child task filtering |

`things_edit_item` already supports `status` with values `pending/completed/canceled/trashed/restored` — no changes needed on the write side.

### Backward compatibility

`is_completed` is removed. Since MCP tool schemas are self-describing and LLMs read them fresh each session, there is no backward compatibility concern.
