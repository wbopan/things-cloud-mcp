# Recurring Tasks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix Issue #5 — completing a recurring task should complete only the current instance, not destroy the recurrence. Align create/edit flows with the Things Cloud template+instance model.

**Architecture:** Things Cloud represents a recurring task as two entities: a **template** (has `rr` recurrence rule, `icsd`, `tir` pointing to next occurrence) and an **instance** (has `rt` pointing to template, `tir` = current occurrence date, no `rr`). The server does NOT auto-create instances — we must create both template and instance ourselves. Our MCP API hides templates entirely — users only see instances with an `isRecurring` flag. Creating a recurring task means writing both a template and an instance in one batch. Editing recurrence means creating/replacing/trashing the template behind the scenes.

**Tech Stack:** Go, Things Cloud wire format (JSON), `writeAndSync` for batch writes.

---

## Background: Wire Format Reference

| Field | Template | Instance | Regular Task |
|-------|----------|----------|-------------|
| `rr` | recurrence rule JSON | absent | absent |
| `rt` | `[]` | `[templateUUID]` | `[]` |
| `icsd` | today (unix) | absent | absent |
| `tir` | next occurrence date | current occurrence date | same as `sr` |
| `sr` | next occurrence date | current occurrence date | schedule date |
| `st` | `1` (anytime, always) | varies | varies |
| `sb` | `0` (default, always) | varies | varies |

**Template detection:** `task.Repeater != nil && len(task.RecurrenceIDs) == 0`

**Instance detection:** `len(task.RecurrenceIDs) > 0`

**Completing an instance:** simple `ss=3, sp=now` (already works correctly).

**Completing a template:** permanently ends the recurrence (user should never do this through normal API).

---

## Task 1: Add `isRecurringTemplate` helper and filter templates from read endpoints

**Files:**
- Modify: `main.go:~565` (TaskOutput struct)
- Modify: `main.go:~653` (taskToOutput)
- Modify: `main.go:~1066` (after findTask helper)
- Modify: `main.go:~2040` (handleFindTasks)
- Modify: `main.go:~2255` (handleShowTask)
- Modify: `main.go:~2330` (handleShowProject child loop)
- Modify: `main.go:~2475` (handleFindProjects)
- Modify: `main.go:~2888` (handleEditTask — guard against template UUIDs)

**Step 1: Add helper after `findTask`**

At `main.go:~1076`, after the `findTask` function, add:

```go
// isRecurringTemplate returns true if the task is a recurring template
// (has rr but no rt). Templates are internal; the API only exposes instances.
func isRecurringTemplate(task *thingscloud.Task) bool {
	return task.Repeater != nil && len(task.RecurrenceIDs) == 0
}
```

**Step 2: Change `TaskOutput.Recurrence` to `IsRecurring`**

At `main.go:~572`, replace:
```go
Recurrence       *string `json:"recurrence,omitempty"`
```
with:
```go
IsRecurring      bool    `json:"isRecurring"`
```

**Step 3: Update `taskToOutput` to use `tir` and set `IsRecurring`**

At `main.go:~653`, replace the opening of `taskToOutput` to prefer `tir` over `sr` for recurring instances, and set `IsRecurring` from `RecurrenceIDs`:

```go
func (t *ThingsMCP) taskToOutput(task *thingscloud.Task) TaskOutput {
	state := t.getState()
	// For recurring instances, tir tracks the current occurrence date;
	// sr may differ. Prefer tir when available.
	effectiveDate := task.ScheduledDate
	if task.TodayIndexRefDate != nil {
		effectiveDate = task.TodayIndexRefDate
	}
	out := TaskOutput{
		UUID:     task.UUID,
		Title:    task.Title,
		Note:     task.Note,
		Status:   statusString(task.Status),
		Schedule: scheduleString(task.Schedule, effectiveDate, task.StartBucket),
	}
	if effectiveDate != nil && effectiveDate.Year() > 1970 {
		s := effectiveDate.Format("2006-01-02")
		out.ScheduledDate = &s
	}
	// ...existing deadline, reminder logic unchanged...
	if len(task.RecurrenceIDs) > 0 {
		out.IsRecurring = true
	}
	// ...rest unchanged...
```

**Step 4: Add template filter to `handleFindTasks`**

At `main.go:~2042`, after the project/heading skip, add:
```go
if isRecurringTemplate(task) {
    continue
}
```

**Step 5: Add template filter to `handleShowTask`**

At `main.go:~2257`, inside the `strings.HasPrefix` match, before building output:
```go
if isRecurringTemplate(task) {
    return errResult(fmt.Sprintf("task not found: %s", uuidPrefix)), nil
}
```

**Step 6: Add template filter to `handleShowProject` child loop**

At `main.go:~2332`, after the `InTrash || Project || Heading` skip:
```go
if isRecurringTemplate(task) {
    continue
}
```

**Step 7: Add template filter to `handleFindProjects`**

At `main.go:~2477`, after `task.Type != TaskTypeProject` skip:
```go
if isRecurringTemplate(task) {
    continue
}
```

**Step 8: Guard `handleEditTask` against template UUIDs**

At `main.go:~2894`, after `validateTaskUUID`, add a sync+check to reject templates:
```go
if err := t.syncAndRebuild(); err != nil {
    return errResult(fmt.Sprintf("sync: %v", err)), nil
}
if task := t.findTask(taskUUID); task != nil && isRecurringTemplate(task) {
    return errResult(fmt.Sprintf("cannot edit recurring template directly: %s", taskUUID)), nil
}
```

**Step 9: Add `rr` to `taskToRawWire` (debug tool)**

In `taskToRawWire`, before `return raw`, add:
```go
if task.Repeater != nil {
    raw["rr"] = task.Repeater
} else {
    raw["rr"] = nil
}
```

**Step 10: Build and verify**

Run: `go build ./...`
Expected: no errors.

**Step 11: Commit**

```
git add main.go
git commit -m "Add isRecurringTemplate, filter templates from API, use tir for dates"
```

---

## Task 2: Extract shared helper and implement template+instance creation in `handleCreateTask`

**Files:**
- Modify: `main.go:~499` (add helpers)
- Modify: `main.go:~2634` (handleCreateTask)

**Concept:** When `recurrence` is provided, `handleCreateTask` creates TWO entities:
1. **Template**: same title/note/tags/project/area, has `rr`+`icsd`, `tir`/`sr` = next occurrence, `rt=[]`, `st=1`, `sb=0`
2. **Instance**: same fields, NO `rr`, `rt=[templateUUID]`, `tir`/`sr` = user's scheduled date

When `recurrence` is NOT provided: behavior unchanged.

**Step 1: Create `newTemplatePayload` and `splitRecurringPayload` helpers**

Add near `newTaskCreatePayload` (~line 499):

```go
// newTemplatePayload creates a recurring template payload from an instance payload.
// The template has rr, icsd, tir/sr advanced to the next occurrence.
// Templates always use st=1 (anytime) and sb=0 (default) regardless of instance values.
func newTemplatePayload(instance *TaskCreatePayload, rr *json.RawMessage, nextTir int64) TaskCreatePayload {
	tpl := *instance
	tpl.Rr = rr
	today := todayMidnightUTC()
	tpl.Icsd = &today
	tpl.Rt = []string{}
	tpl.Sr = &nextTir
	tpl.Tir = &nextTir
	tpl.St = 1  // templates are always anytime
	tpl.Sb = 0  // templates never use tonight bucket
	return tpl
}

// splitRecurringPayload takes a payload with rr set and returns (templatePayload, instancePayload, error).
// The template gets rr/icsd/advanced tir; the instance gets rt=[templateUUID] and no rr.
// Returns templateUUID so the caller can use it.
func splitRecurringPayload(payload TaskCreatePayload) (templateUUID string, tplPayload TaskCreatePayload, instPayload TaskCreatePayload, err error) {
	// Determine schedule date for computing next occurrence
	schedDate := time.Now().UTC()
	if payload.Tir != nil {
		schedDate = time.Unix(*payload.Tir, 0).UTC()
	} else if payload.Sr != nil {
		schedDate = time.Unix(*payload.Sr, 0).UTC()
	}

	// Parse rr to compute next date
	var rc thingscloud.RepeaterConfiguration
	if err := json.Unmarshal(*payload.Rr, &rc); err != nil {
		return "", TaskCreatePayload{}, TaskCreatePayload{}, fmt.Errorf("parse recurrence: %v", err)
	}
	nextDate := rc.NextOccurrenceAfter(schedDate)
	if nextDate.IsZero() {
		return "", TaskCreatePayload{}, TaskCreatePayload{}, fmt.Errorf("could not compute next recurrence date")
	}
	nextTir := nextDate.Unix()

	templateUUID = generateUUID()
	tplPayload = newTemplatePayload(&payload, payload.Rr, nextTir)

	// Instance: clear rr/icsd, set rt to template
	instPayload = payload
	instPayload.Rr = nil
	instPayload.Icsd = nil
	instPayload.Rt = []string{templateUUID}

	return templateUUID, tplPayload, instPayload, nil
}
```

**Step 2: Modify `handleCreateTask` to use `splitRecurringPayload`**

Replace `main.go:2661-2684` (from `taskUUID := generateUUID()` to the return):

```go
	taskUUID := generateUUID()
	payload := newTaskCreatePayload(title, opts, ix)

	var envelopes []thingscloud.Identifiable

	if _, hasRecurrence := opts["recurrence"]; hasRecurrence && payload.Rr != nil {
		templateUUID, tplPayload, instPayload, err := splitRecurringPayload(payload)
		if err != nil {
			return errResult(err.Error()), nil
		}
		envelopes = append(envelopes, writeEnvelope{id: templateUUID, action: 0, kind: "Task6", payload: tplPayload})
		envelopes = append(envelopes, writeEnvelope{id: taskUUID, action: 0, kind: "Task6", payload: instPayload})
	} else {
		envelopes = append(envelopes, writeEnvelope{id: taskUUID, action: 0, kind: "Task6", payload: payload})
	}

	// Checklist items (attach to instance, not template)
	if v, ok := opts["checklist"]; ok && v != "" {
		now := nowTs()
		for i, item := range strings.Split(v, ",") {
			itemUUID := generateUUID()
			clPayload := ChecklistItemCreatePayload{
				Cd: now, Md: nil, Tt: strings.TrimSpace(item), Ss: 0, Sp: nil,
				Ix: i, Ts: []string{taskUUID}, Lt: false, Xx: defaultExtension(),
			}
			envelopes = append(envelopes, writeEnvelope{id: itemUUID, action: 0, kind: "ChecklistItem3", payload: clPayload})
		}
	}

	if err := t.writeAndSync(envelopes...); err != nil {
		return errResult(fmt.Sprintf("create task: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": taskUUID, "title": title}), nil
```

**Step 3: Build and verify**

Run: `go build ./...`

**Step 4: Commit**

```
git add main.go
git commit -m "Create template+instance pair when creating recurring tasks"
```

---

## Task 3: Implement template+instance creation in `handleCreateProject`

**Files:**
- Modify: `main.go:~2687` (handleCreateProject)

Same pattern as Task 2. Uses `splitRecurringPayload` (shared helper from Task 2).

**Step 1: Apply the template+instance split**

Replace the single-write block in `handleCreateProject` (~lines 2701-2708):

```go
	projectUUID := generateUUID()
	payload := newTaskCreatePayload(title, opts, ix)

	var envelopes []thingscloud.Identifiable

	if _, hasRecurrence := opts["recurrence"]; hasRecurrence && payload.Rr != nil {
		templateUUID, tplPayload, instPayload, err := splitRecurringPayload(payload)
		if err != nil {
			return errResult(err.Error()), nil
		}
		envelopes = append(envelopes, writeEnvelope{id: templateUUID, action: 0, kind: "Task6", payload: tplPayload})
		envelopes = append(envelopes, writeEnvelope{id: projectUUID, action: 0, kind: "Task6", payload: instPayload})
	} else {
		envelopes = append(envelopes, writeEnvelope{id: projectUUID, action: 0, kind: "Task6", payload: payload})
	}

	if err := t.writeAndSync(envelopes...); err != nil {
		return errResult(fmt.Sprintf("create project: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": projectUUID, "title": title}), nil
```

**Step 2: Build and verify**

Run: `go build ./...`

**Step 3: Commit**

```
git add main.go
git commit -m "Create template+instance pair when creating recurring projects"
```

---

## Task 4: Implement recurrence editing in `handleEditTask`

**Files:**
- Modify: `main.go:~2888` (handleEditTask — add envelopes slice and rewrite recurrence block)

**Concept:** Three cases when user sets the `recurrence` parameter:

| Instance state | User action | Behavior |
|---|---|---|
| No `rt` (non-recurring) | `recurrence=daily` | Create new template, set `rt=[templateUUID]` on instance |
| Has `rt` (recurring) | `recurrence=weekly` | Trash old template, create new template, update `rt` |
| Has `rt` (recurring) | `recurrence=none` | Trash old template, clear `rt` on instance |

Note: `handleEditTask` is used for both tasks and projects (projects are Task entities with `tp=1`), so this covers both.

**Step 1: Add `envelopes` slice declaration**

At `main.go:~2909`, after `u := newTaskUpdate()`, add:

```go
var envelopes []thingscloud.Identifiable
```

**Step 2: Replace the recurrence block**

Replace `main.go:2976-2995` (the entire `if v := req.GetString("recurrence", "")` block):

```go
	if v := req.GetString("recurrence", ""); v != "" {
		// Look up the task to inspect current recurrence state
		// (Task 1 already syncs+guards against templates at the top of handleEditTask)
		task := t.findTask(taskUUID)
		if task == nil {
			return errResult(fmt.Sprintf("task not found: %s", taskUUID)), nil
		}

		hasExistingTemplate := len(task.RecurrenceIDs) > 0
		var oldTemplateUUID string
		if hasExistingTemplate {
			oldTemplateUUID = task.RecurrenceIDs[0]
		}

		if v == "none" {
			// Remove recurrence: clear rt on instance, trash old template
			u.fields["rt"] = []string{}
			u.ClearRecurrence()
			if hasExistingTemplate {
				trashUpdate := newTaskUpdate()
				trashUpdate.Trash(true)
				envelopes = append(envelopes, writeEnvelope{id: oldTemplateUUID, action: 1, kind: "Task6", payload: trashUpdate.build()})
			}
		} else {
			// Add or change recurrence
			recRef := time.Now()
			if schedStr := req.GetString("schedule", ""); schedStr != "" {
				if dt := parseDate(schedStr); dt != nil {
					recRef = *dt
				}
			} else if task.TodayIndexRefDate != nil {
				recRef = *task.TodayIndexRefDate
			} else if task.ScheduledDate != nil {
				recRef = *task.ScheduledDate
			}

			rr, err := parseRecurrence(v, recRef)
			if err != nil {
				return errResult(err.Error()), nil
			}
			if rr == nil {
				return errResult("invalid recurrence"), nil
			}

			// Compute next occurrence for template tir
			var rc thingscloud.RepeaterConfiguration
			if err := json.Unmarshal(*rr, &rc); err != nil {
				return errResult(fmt.Sprintf("internal: parse recurrence: %v", err)), nil
			}
			nextDate := rc.NextOccurrenceAfter(recRef)
			if nextDate.IsZero() {
				return errResult("could not compute next recurrence date"), nil
			}

			// Create new template
			newTemplateUUID := generateUUID()
			now := nowTs()
			nextTir := nextDate.Unix()
			today := todayMidnightUTC()
			tplPayload := TaskCreatePayload{
				Tp: int(task.Type), Tt: task.Title, Nt: textNote(task.Note),
				St: 1, Sr: &nextTir, Tir: &nextTir,
				Rr: rr, Icsd: &today,
				Rt: []string{}, Pr: task.ParentTaskIDs, Ar: task.AreaIDs,
				Agr: task.ActionGroupIDs, Tg: task.TagIDs,
				Cd: now, Ix: task.Index, Sb: 0,
				Dl: []string{}, Icp: false, Icc: 0, Lt: false,
				Do: task.DueOrder, Xx: defaultExtension(),
			}
			if task.DeadlineDate != nil {
				dd := task.DeadlineDate.Unix()
				tplPayload.Dd = &dd
			}
			envelopes = append(envelopes, writeEnvelope{id: newTemplateUUID, action: 0, kind: "Task6", payload: tplPayload})

			// Update instance: point to new template, remove rr/icsd
			u.fields["rt"] = []string{newTemplateUUID}
			u.ClearRecurrence()

			// Trash old template if changing recurrence
			if hasExistingTemplate {
				trashUpdate := newTaskUpdate()
				trashUpdate.Trash(true)
				envelopes = append(envelopes, writeEnvelope{id: oldTemplateUUID, action: 1, kind: "Task6", payload: trashUpdate.build()})
			}
		}
	}
```

**Step 3: Change the final write to use envelopes**

Replace the single `writeEnvelope` + `writeAndSync` at `main.go:~3013-3017`:

```go
	envelopes = append(envelopes, writeEnvelope{id: taskUUID, action: 1, kind: "Task6", payload: u.build()})
	if err := t.writeAndSync(envelopes...); err != nil {
		return errResult(fmt.Sprintf("edit task: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "updated", "uuid": taskUUID}), nil
```

**Step 4: Build and verify**

Run: `go build ./...`

**Step 5: Commit**

```
git add main.go
git commit -m "Support recurrence add/change/remove in handleEditTask via template+instance"
```

---

## Task 5: Update tool descriptions and landing page

**Files:**
- Modify: `main.go:~3237` (things_create_task description)
- Modify: `main.go:~3370` (things_edit_item description)
- Modify: `landing.go` (sync descriptions)

**Step 1: Update tool descriptions**

In `things_create_task` description, mention `isRecurring` in output.

In `things_edit_item` description, add: "Completing a recurring task completes only the current instance; the next instance appears automatically. Set recurrence=none to permanently stop a recurring task."

**Step 2: Sync landing.go**

Use the `/sync` skill to update landing.go to match main.go changes.

**Step 3: Commit**

```
git add main.go landing.go
git commit -m "Update tool descriptions for recurring task behavior"
```

---

## Task 6: Deploy and test end-to-end

**Step 1: Build for Linux and deploy**

```bash
GOOS=linux GOARCH=amd64 go build -o things-cloud-mcp .
scp things-cloud-mcp wenbo@e.wenbo.io:/home/wenbo/things-cloud-mcp/things-cloud-mcp
ssh wenbo@e.wenbo.io 'systemctl --user restart things-mcp'
```

**Step 2: Create a recurring task**

```bash
curl -s -X POST http://localhost:28063/mcp \
  -H "Authorization: Basic $(echo -n 'pixelwenbo@gmail.com:bmp6125388th' | base64)" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"things_create_task","arguments":{"title":"test recurring","schedule":"today","recurrence":"daily"}}}'
```

Expected: `{"status":"created","uuid":"...","title":"test recurring"}`

**Step 3: Verify template+instance via debug**

Use `things_debug_raw` on the returned UUID. Verify:
- Instance has `rt` pointing to a template UUID, NO `rr`
- `isRecurring: true` in normal output

Use `things_debug_raw` on the template UUID (from `rt`). Verify:
- Template has `rr` with daily config
- Template `tir`/`sr` = tomorrow
- Template `st=1`, `sb=0`

**Step 4: Complete the recurring task**

```bash
curl -s -X POST http://localhost:28063/mcp \
  -H "Authorization: Basic $(echo -n 'pixelwenbo@gmail.com:bmp6125388th' | base64)" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"things_edit_item","arguments":{"uuid":"<INSTANCE_UUID>","status":"completed"}}}'
```

Verify in Things app: task appears in Logbook, next instance appears for tomorrow.

**Step 5: Test recurrence change**

Find a recurring task, change its recurrence:
```json
{"name":"things_edit_item","arguments":{"uuid":"<UUID>","recurrence":"weekly"}}
```

Verify: old template trashed, new template created with weekly rule.

**Step 6: Test recurrence removal**

```json
{"name":"things_edit_item","arguments":{"uuid":"<UUID>","recurrence":"none"}}
```

Verify: task is no longer recurring, template trashed.

**Step 7: Test template UUID rejection**

Try editing a template UUID directly:
```json
{"name":"things_edit_item","arguments":{"uuid":"<TEMPLATE_UUID>","title":"hacked"}}
```

Expected: error "cannot edit recurring template directly".

**Step 8: Clean up test data**

Trash any test tasks created during testing.
