# Status Filter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the `is_completed` boolean with a `status` enum filter (`pending`/`completed`/`canceled`) on list tools, fixing the bug where canceled tasks leak into query results.

**Architecture:** Three handler functions and their tool definitions need updating. The filtering logic changes from "exclude completed unless opted in" to "match the requested status value." Default remains `pending` (active tasks only).

**Tech Stack:** Go, MCP SDK (`mcp.WithString`, `mcp.Enum`)

**Design doc:** `docs/plans/2026-03-01-status-filter-design.md`

---

### Task 1: Update `handleListTasks` handler

**Files:**
- Modify: `main.go:1536-1662` (`handleListTasks` function)

**Step 1: Replace `isCompleted` bool with `statusFilter` string**

In `handleListTasks`, change line 1551:

```go
// BEFORE:
isCompleted := req.GetBool("is_completed", false)

// AFTER:
statusFilter := req.GetString("status", "pending")
```

**Step 2: Replace the status filtering logic**

Replace lines 1611-1613:

```go
// BEFORE:
if !isCompleted && task.Status == 3 {
    continue
}

// AFTER:
switch statusFilter {
case "completed":
    if task.Status != 3 {
        continue
    }
case "canceled":
    if task.Status != 2 {
        continue
    }
default: // "pending"
    if task.Status != 0 {
        continue
    }
}
```

**Step 3: Build to verify**

Run: `/usr/local/go/bin/go build ./...`
Expected: clean build, no errors

**Step 4: Commit**

```bash
git add main.go
git commit -m "Replace is_completed bool with status filter in handleListTasks"
```

---

### Task 2: Update `handleListProjects` handler

**Files:**
- Modify: `main.go:1779-1794` (`handleListProjects` function)

**Step 1: Change the function signature to accept the request**

Replace line 1779:

```go
// BEFORE:
func (t *ThingsMCP) handleListProjects(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {

// AFTER:
func (t *ThingsMCP) handleListProjects(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
```

**Step 2: Add status parameter extraction**

After line 1783 (`state := t.getState()`), add:

```go
statusFilter := req.GetString("status", "pending")
```

**Step 3: Replace the hardcoded status filter**

Replace line 1786:

```go
// BEFORE:
if task.Type == thingscloud.TaskTypeProject && !task.InTrash && task.Status != 3 {

// AFTER:
if task.Type != thingscloud.TaskTypeProject || task.InTrash {
    continue
}
switch statusFilter {
case "completed":
    if task.Status != 3 {
        continue
    }
case "canceled":
    if task.Status != 2 {
        continue
    }
default:
    if task.Status != 0 {
        continue
    }
}
projects = append(projects, t.taskToOutput(task))
```

Note: This restructures the loop body from a single `if` with `append` inside to a `continue`-on-skip pattern matching `handleListTasks`.

**Step 4: Build to verify**

Run: `/usr/local/go/bin/go build ./...`
Expected: clean build, no errors

**Step 5: Commit**

```bash
git add main.go
git commit -m "Add status filter to handleListProjects"
```

---

### Task 3: Update `handleShowProject` handler

**Files:**
- Modify: `main.go:1738-1758` (task collection loop in `handleShowProject`)

**Step 1: Add status parameter extraction**

After line 1700 (`state := t.getState()`), add:

```go
statusFilter := req.GetString("status", "pending")
```

**Step 2: Replace the hardcoded status filter in task collection**

Replace line 1741:

```go
// BEFORE:
if task.InTrash || task.Status == 3 || task.Type == thingscloud.TaskTypeProject || task.Type == thingscloud.TaskTypeHeading {

// AFTER:
if task.InTrash || task.Type == thingscloud.TaskTypeProject || task.Type == thingscloud.TaskTypeHeading {
    continue
}
switch statusFilter {
case "completed":
    if task.Status != 3 {
        continue
    }
case "canceled":
    if task.Status != 2 {
        continue
    }
default:
    if task.Status != 0 {
        continue
    }
}
if !containsStr(task.ParentTaskIDs, projectUUID) {
```

Note: The `!containsStr(task.ParentTaskIDs, projectUUID)` check (line 1744) stays as the next filter — just remove it from the original position since we're restructuring the continues.

**Step 3: Build to verify**

Run: `/usr/local/go/bin/go build ./...`
Expected: clean build, no errors

**Step 4: Commit**

```bash
git add main.go
git commit -m "Add status filter to handleShowProject"
```

---

### Task 4: Update tool definitions

**Files:**
- Modify: `main.go:2345-2361` (`things_list_tasks` tool definition)
- Modify: `main.go:2380-2391` (`things_show_project` tool definition)
- Modify: `main.go:2393-2402` (`things_list_projects` tool definition)

**Step 1: Update `things_list_tasks` tool definition**

Replace the `is_completed` parameter (line 2360):

```go
// BEFORE:
mcp.WithBoolean("is_completed", mcp.Description("When true, include completed items in results (default false)")),

// AFTER:
mcp.WithString("status", mcp.Description("Filter by task status (default: pending — only active tasks)"), mcp.Enum("pending", "completed", "canceled")),
```

Update the tool description (line 2346) to say "Default: only pending (active) tasks. Use status parameter to query completed or canceled tasks." instead of "By default excludes trashed and completed tasks."

```go
mcp.WithDescription("List tasks from Things 3 with optional filters. Returns an array of task objects, each containing uuid, title, status (pending/completed/canceled), schedule (inbox/today/anytime/someday/upcoming), and optional fields: note, scheduledDate, deadlineDate, reminderTime, recurrence, areas, project, tags. Default: only pending (active) tasks. Use status parameter to query completed or canceled tasks."),
```

**Step 2: Add `status` parameter to `things_show_project` tool definition**

After line 2386 (`mcp.WithString("uuid", ...)`), add:

```go
mcp.WithString("status", mcp.Description("Filter child tasks by status (default: pending — only active tasks)"), mcp.Enum("pending", "completed", "canceled")),
```

**Step 3: Add `status` parameter to `things_list_projects` tool definition**

After the annotation lines (line 2398), add:

```go
mcp.WithString("status", mcp.Description("Filter by project status (default: pending — only active projects)"), mcp.Enum("pending", "completed", "canceled")),
```

Update the description (line 2394):

```go
mcp.WithDescription("List projects in Things 3 with optional filters. Returns an array of project objects, each containing uuid, title, status, schedule, and optional fields: note, scheduledDate, deadlineDate, areas, tags. Default: only pending (active) projects. Use status parameter to query completed or canceled projects."),
```

**Step 4: Build to verify**

Run: `/usr/local/go/bin/go build ./...`
Expected: clean build, no errors

**Step 5: Commit**

```bash
git add main.go
git commit -m "Update tool definitions: replace is_completed with status enum"
```

---

### Task 5: Final verification

**Step 1: Full build**

Run: `/usr/local/go/bin/go build ./...`
Expected: clean build

**Step 2: Grep to confirm no remaining references to `is_completed`**

Run: `grep -n 'is_completed\|isCompleted' main.go`
Expected: no matches

**Step 3: Grep to confirm all three handlers use `statusFilter`**

Run: `grep -n 'statusFilter' main.go`
Expected: matches in `handleListTasks`, `handleListProjects`, `handleShowProject`

**Step 4: Review diff**

Run: `git diff main`
Verify: all changes match the design
