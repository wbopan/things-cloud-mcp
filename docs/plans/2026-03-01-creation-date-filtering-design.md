# Creation-date filtering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `created_after` and `created_before` date filters to `things_list_tasks` and `things_list_projects` tools so users can fetch recently-created items.

**Architecture:** Client-side filtering on the already-populated `task.CreationDate` field (`time.Time`, non-nullable). Follows the exact same pattern as existing `scheduled_before/after` and `deadline_before/after` filters — parse `YYYY-MM-DD` string with `parseDate()`, compare with `.Before()`/`.After()`.

**Tech Stack:** Go 1.24, mcp-go SDK, things-cloud-sdk

**Issue:** https://github.com/wbopan/things-cloud-mcp/issues/3

---

### Task 1: Add created_after/created_before to things_list_tasks handler

**Files:**
- Modify: `main.go:979-983` (parameter extraction)
- Modify: `main.go:1011-1036` (date parsing)
- Modify: `main.go:1060-1080` (filter logic)

**Step 1: Add parameter extraction**

At `main.go:983` (after `deadlineAfter`), add:

```go
	createdBefore := req.GetString("created_before", "")
	createdAfter := req.GetString("created_after", "")
```

**Step 2: Add date parsing**

At `main.go:1036` (after the `deadlineAfter` parsing block), add:

```go
	var createdBeforeDate, createdAfterDate *time.Time
	if createdBefore != "" {
		createdBeforeDate = parseDate(createdBefore)
		if createdBeforeDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", createdBefore)), nil
		}
	}
	if createdAfter != "" {
		createdAfterDate = parseDate(createdAfter)
		if createdAfterDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", createdAfter)), nil
		}
	}
```

**Step 3: Add filter logic**

At `main.go:1080` (after the `deadlineAfterDate` filter block), add:

```go
		// Creation date filters (exclusive) — CreationDate is non-nullable, no nil check needed
		if createdBeforeDate != nil {
			if !task.CreationDate.Before(*createdBeforeDate) {
				continue
			}
		}
		if createdAfterDate != nil {
			if !task.CreationDate.After(*createdAfterDate) {
				continue
			}
		}
```

**Step 4: Build to verify compilation**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: clean build, no errors

**Step 5: Commit**

```bash
git add main.go
git commit -m "Add created_after/created_before filtering to things_list_tasks"
```

---

### Task 2: Add created_after/created_before to things_list_projects handler

**Files:**
- Modify: `main.go:1217-1232` (handleListProjects)

The current `handleListProjects` handler accepts no parameters (`_ mcp.CallToolRequest`). It needs to be updated to extract, parse, and filter on the new params.

**Step 1: Update function signature**

At `main.go:1217`, change:

```go
func (t *ThingsMCP) handleListProjects(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
```

to:

```go
func (t *ThingsMCP) handleListProjects(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
```

**Step 2: Add parameter extraction, parsing, and filtering**

Replace the loop body (lines 1222-1227) with:

```go
	createdBefore := req.GetString("created_before", "")
	createdAfter := req.GetString("created_after", "")

	var createdBeforeDate, createdAfterDate *time.Time
	if createdBefore != "" {
		createdBeforeDate = parseDate(createdBefore)
		if createdBeforeDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", createdBefore)), nil
		}
	}
	if createdAfter != "" {
		createdAfterDate = parseDate(createdAfter)
		if createdAfterDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", createdAfter)), nil
		}
	}

	var projects []TaskOutput
	for _, task := range state.Tasks {
		if task.Type != thingscloud.TaskTypeProject || task.InTrash || task.Status == 3 {
			continue
		}
		if createdBeforeDate != nil && !task.CreationDate.Before(*createdBeforeDate) {
			continue
		}
		if createdAfterDate != nil && !task.CreationDate.After(*createdAfterDate) {
			continue
		}
		projects = append(projects, t.taskToOutput(task))
	}
```

**Step 3: Build to verify compilation**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: clean build, no errors

**Step 4: Commit**

```bash
git add main.go
git commit -m "Add created_after/created_before filtering to things_list_projects"
```

---

### Task 3: Add tool parameter declarations for both tools

**Files:**
- Modify: `main.go:1783-1799` (things_list_tasks tool definition)
- Modify: `main.go:1831-1837` (things_list_projects tool definition)

**Step 1: Add parameter declarations to things_list_tasks**

At `main.go:1793` (after the `deadline_after` parameter), add:

```go
			mcp.WithString("created_before", mcp.Description("Return tasks created before this date (YYYY-MM-DD, exclusive)")),
			mcp.WithString("created_after", mcp.Description("Return tasks created after this date (YYYY-MM-DD, exclusive)")),
```

**Step 2: Add parameter declarations to things_list_projects**

At the `things_list_projects` tool definition (line 1831), add parameters. Change from no params:

```go
		Tool: mcp.NewTool("things_list_projects",
			mcp.WithDescription("List all active (non-trashed, non-completed) projects in Things 3. Returns an array of project objects, each containing uuid, title, status, schedule, and optional fields: note, scheduledDate, deadlineDate, areas, tags."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
		),
```

to:

```go
		Tool: mcp.NewTool("things_list_projects",
			mcp.WithDescription("List active (non-trashed, non-completed) projects in Things 3 with optional filters. Returns an array of project objects, each containing uuid, title, status, schedule, and optional fields: note, scheduledDate, deadlineDate, areas, tags."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("created_before", mcp.Description("Return projects created before this date (YYYY-MM-DD, exclusive)")),
			mcp.WithString("created_after", mcp.Description("Return projects created after this date (YYYY-MM-DD, exclusive)")),
		),
```

**Step 3: Build to verify compilation**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: clean build, no errors

**Step 4: Commit**

```bash
git add main.go
git commit -m "Add created_before/created_after param declarations to tool definitions

Closes #3"
```
