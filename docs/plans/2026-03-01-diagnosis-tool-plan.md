# `things_diagnose` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `things_diagnose` MCP tool that performs a full end-to-end test of the Things Cloud sync pipeline with detailed per-step logging, helping users debug sync issues (e.g., only seeing tasks from 2020).

**Architecture:** A single `handleDiagnose()` method on `ThingsMCP` that creates a *fresh* Things Cloud client (separate from the cached one), walks through 7 sequential diagnostic steps (verify → history → sync → paginated fetch → rebuild state → data integrity → query tests), captures logs and timing for each step, and returns a structured JSON report. The diagnosis uses its own client to avoid interfering with the user's cached session.

**Tech Stack:** Go, mcp-go SDK, things-cloud-sdk

---

### Task 1: Add diagnostic types and email masking helper

**Files:**
- Modify: `main.go` (insert after the `errResult`/`jsonResult` helpers, around line 967)

**Step 1: Add the diagnostic output types and masking helper**

Insert the following code after the `errResult` function (line 967 in `main.go`):

```go
// ---------------------------------------------------------------------------
// Diagnosis types
// ---------------------------------------------------------------------------

type diagStep struct {
	Step        int    `json:"step"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"durationMs"`
	Details     any    `json:"details"`
	Log         []string `json:"log"`
}

type diagSummary struct {
	TotalSteps    int   `json:"totalSteps"`
	Passed        int   `json:"passed"`
	Warnings      int   `json:"warnings"`
	Failed        int   `json:"failed"`
	Skipped       int   `json:"skipped"`
	TotalDurationMs int64 `json:"totalDurationMs"`
}

type diagReport struct {
	Steps    []diagStep  `json:"steps"`
	Summary  diagSummary `json:"summary"`
	Warnings []string    `json:"warnings"`
	Errors   []string    `json:"errors"`
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	name := parts[0]
	if len(name) <= 1 {
		return name + "***@" + parts[1]
	}
	return string(name[0]) + "***@" + parts[1]
}
```

**Step 2: Verify it compiles**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: compiles without errors

**Step 3: Commit**

```bash
git add main.go
git commit -m "Add diagnostic types and maskEmail helper for things_diagnose"
```

---

### Task 2: Implement `handleDiagnose` — steps 1-3 (credential, history, sync)

**Files:**
- Modify: `main.go` (add `handleDiagnose` method after the diagnostic types)

**Step 1: Add the handleDiagnose method with steps 1-3**

Insert after the diagnostic types block:

```go
func (t *ThingsMCP) handleDiagnose(email, password string) *diagReport {
	report := &diagReport{}
	var allWarnings []string
	var allErrors []string
	failed := false

	// Step 1: Credential verification
	step1 := diagStep{Step: 1, Name: "credential_verification", Description: "Verify Things Cloud credentials"}
	step1.Log = append(step1.Log, fmt.Sprintf("Verifying credentials for %s...", maskEmail(email)))
	start := time.Now()

	c := thingscloud.New(thingscloud.APIEndpoint, email, password)
	verifyResp, err := c.Verify()
	step1.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		step1.Status = "fail"
		step1.Log = append(step1.Log, fmt.Sprintf("FAIL: %v", err))
		step1.Details = map[string]any{"email": maskEmail(email), "error": err.Error()}
		allErrors = append(allErrors, fmt.Sprintf("Credential verification failed: %v", err))
		failed = true
	} else {
		step1.Status = "pass"
		step1.Log = append(step1.Log, "Credentials valid")
		step1.Details = map[string]any{
			"email":         maskEmail(email),
			"accountStatus": string(verifyResp.Status),
			"historyKey":    verifyResp.HistoryKey,
		}
	}
	report.Steps = append(report.Steps, step1)

	// Step 2: Fetch history
	step2 := diagStep{Step: 2, Name: "fetch_history", Description: "Fetch user history from Things Cloud"}
	if failed {
		step2.Status = "skipped"
		step2.Log = append(step2.Log, "Skipped: previous step failed")
		report.Steps = append(report.Steps, step2)
	} else {
		step2.Log = append(step2.Log, "Calling OwnHistory()...")
		start = time.Now()
		history, err := c.OwnHistory()
		step2.DurationMs = time.Since(start).Milliseconds()

		if err != nil {
			step2.Status = "fail"
			step2.Log = append(step2.Log, fmt.Sprintf("FAIL: %v", err))
			step2.Details = map[string]any{"error": err.Error()}
			allErrors = append(allErrors, fmt.Sprintf("Fetch history failed: %v", err))
			failed = true
		} else {
			step2.Status = "pass"
			step2.Log = append(step2.Log, fmt.Sprintf("Got history object, key=%s", history.ID))
			step2.Details = map[string]any{"historyId": history.ID}
		}
		report.Steps = append(report.Steps, step2)

		// Step 3: Sync history
		step3 := diagStep{Step: 3, Name: "sync_history", Description: "Sync history data"}
		if failed {
			step3.Status = "skipped"
			step3.Log = append(step3.Log, "Skipped: previous step failed")
		} else {
			step3.Log = append(step3.Log, "Calling history.Sync()...")
			start = time.Now()
			err = history.Sync()
			step3.DurationMs = time.Since(start).Milliseconds()

			if err != nil {
				step3.Status = "fail"
				step3.Log = append(step3.Log, fmt.Sprintf("FAIL: %v", err))
				step3.Details = map[string]any{"error": err.Error()}
				allErrors = append(allErrors, fmt.Sprintf("Sync failed: %v", err))
				failed = true
			} else {
				step3.Status = "pass"
				step3.Log = append(step3.Log, fmt.Sprintf("Sync completed, latestServerIndex=%d", history.LatestServerIndex))
				step3.Details = map[string]any{
					"latestServerIndex": history.LatestServerIndex,
				}
			}
		}
		report.Steps = append(report.Steps, step3)

		// Steps 4-7 call helper methods, passing history
		if !failed {
			t.diagnoseSteps4to7(history, report, &allWarnings, &allErrors)
		} else {
			// Mark remaining steps as skipped
			for i, name := range []string{"paginated_fetch", "rebuild_state", "data_integrity", "query_tests"} {
				report.Steps = append(report.Steps, diagStep{
					Step: i + 4, Name: name, Status: "skipped",
					Description: []string{"Fetch all items via pagination", "Rebuild in-memory state", "Check data completeness and integrity", "Test basic list/query operations"}[i],
					Log: []string{"Skipped: previous step failed"},
				})
			}
		}
	}

	// Build summary
	var totalMs int64
	var passed, warnings, fails, skipped int
	for _, s := range report.Steps {
		totalMs += s.DurationMs
		switch s.Status {
		case "pass":
			passed++
		case "warn":
			warnings++
		case "fail":
			fails++
		case "skipped":
			skipped++
		}
	}
	report.Summary = diagSummary{
		TotalSteps: len(report.Steps), Passed: passed, Warnings: warnings,
		Failed: fails, Skipped: skipped, TotalDurationMs: totalMs,
	}
	report.Warnings = allWarnings
	report.Errors = allErrors
	return report
}
```

**Step 2: Verify it compiles**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: Will fail because `diagnoseSteps4to7` doesn't exist yet. Add a stub:

```go
func (t *ThingsMCP) diagnoseSteps4to7(history *thingscloud.History, report *diagReport, warnings *[]string, errors *[]string) {
	// TODO: implement steps 4-7
}
```

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: compiles without errors

**Step 3: Commit**

```bash
git add main.go
git commit -m "Add handleDiagnose with credential/history/sync steps"
```

---

### Task 3: Implement steps 4-5 (paginated fetch + rebuild state)

**Files:**
- Modify: `main.go` (replace `diagnoseSteps4to7` stub)

**Step 1: Implement the full `diagnoseSteps4to7` method with steps 4 and 5**

Replace the stub with the real implementation:

```go
func (t *ThingsMCP) diagnoseSteps4to7(history *thingscloud.History, report *diagReport, allWarnings *[]string, allErrors *[]string) {
	failed := false

	// Step 4: Paginated fetch
	step4 := diagStep{Step: 4, Name: "paginated_fetch", Description: "Fetch all items via pagination"}
	step4.Log = append(step4.Log, "Starting paginated fetch at index 0")

	type pageInfo struct {
		Page             int `json:"page"`
		StartIndex       int `json:"startIndex"`
		ItemsFetched     int `json:"itemsFetched"`
		ServerIndexAfter int `json:"serverIndexAfter"`
	}
	var pages []pageInfo
	var allItems []thingscloud.Item
	startIndex := 0
	pageNum := 0
	start := time.Now()

	for {
		pageNum++
		items, _, err := history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			step4.Status = "fail"
			step4.Log = append(step4.Log, fmt.Sprintf("FAIL on page %d: %v", pageNum, err))
			step4.Details = map[string]any{"error": err.Error(), "failedAtPage": pageNum}
			*allErrors = append(*allErrors, fmt.Sprintf("Paginated fetch failed on page %d: %v", pageNum, err))
			failed = true
			break
		}
		if len(items) == 0 {
			step4.Log = append(step4.Log, fmt.Sprintf("Page %d: fetched 0 items — pagination complete", pageNum))
			break
		}

		pi := pageInfo{
			Page: pageNum, StartIndex: startIndex,
			ItemsFetched: len(items), ServerIndexAfter: history.LatestServerIndex,
		}
		pages = append(pages, pi)
		step4.Log = append(step4.Log, fmt.Sprintf("Page %d: startIndex=%d, fetched %d items, serverIndex now %d",
			pageNum, startIndex, len(items), history.LatestServerIndex))

		allItems = append(allItems, items...)
		startIndex = history.LatestServerIndex
	}

	step4.DurationMs = time.Since(start).Milliseconds()
	if !failed {
		step4.Status = "pass"
		step4.Details = map[string]any{
			"totalItemsFetched": len(allItems),
			"paginationPages":   len(pages),
			"pages":             pages,
			"finalServerIndex":  history.LatestServerIndex,
		}
		step4.Log = append(step4.Log, fmt.Sprintf("Total: %d items in %d pages", len(allItems), len(pages)))
	}
	report.Steps = append(report.Steps, step4)

	// Step 5: Rebuild state
	step5 := diagStep{Step: 5, Name: "rebuild_state", Description: "Rebuild in-memory state from fetched items"}
	if failed {
		step5.Status = "skipped"
		step5.Log = append(step5.Log, "Skipped: previous step failed")
		report.Steps = append(report.Steps, step5)
	} else {
		step5.Log = append(step5.Log, fmt.Sprintf("Processing %d items into state...", len(allItems)))
		start = time.Now()

		state := memory.NewState()
		state.Update(allItems...)

		step5.DurationMs = time.Since(start).Milliseconds()
		step5.Status = "pass"
		step5.Details = map[string]any{
			"tasks":          len(state.Tasks),
			"areas":          len(state.Areas),
			"tags":           len(state.Tags),
			"checklistItems": len(state.CheckListItems),
		}
		step5.Log = append(step5.Log, fmt.Sprintf("State rebuilt: %d tasks, %d areas, %d tags, %d checklist items",
			len(state.Tasks), len(state.Areas), len(state.Tags), len(state.CheckListItems)))
		report.Steps = append(report.Steps, step5)

		// Steps 6-7 use the rebuilt state
		t.diagnoseDataIntegrity(state, report, allWarnings)
		t.diagnoseQueryTests(report, allWarnings, allErrors)
	}

	if failed {
		// Mark remaining steps as skipped
		for i, name := range []string{"rebuild_state", "data_integrity", "query_tests"} {
			skip := false
			for _, existing := range report.Steps {
				if existing.Name == name {
					skip = true
					break
				}
			}
			if !skip {
				report.Steps = append(report.Steps, diagStep{
					Step: i + 5, Name: name, Status: "skipped",
					Description: []string{"Rebuild in-memory state", "Check data completeness and integrity", "Test basic list/query operations"}[i],
					Log: []string{"Skipped: previous step failed"},
				})
			}
		}
	}
}
```

**Step 2: Add stub methods for steps 6-7**

```go
func (t *ThingsMCP) diagnoseDataIntegrity(state *memory.State, report *diagReport, allWarnings *[]string) {
	// TODO
}

func (t *ThingsMCP) diagnoseQueryTests(report *diagReport, allWarnings *[]string, allErrors *[]string) {
	// TODO
}
```

**Step 3: Verify it compiles**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: compiles without errors

**Step 4: Commit**

```bash
git add main.go
git commit -m "Add paginated fetch and rebuild state diagnosis steps"
```

---

### Task 4: Implement step 6 (data integrity analysis)

**Files:**
- Modify: `main.go` (replace `diagnoseDataIntegrity` stub)

**Step 1: Implement data integrity analysis**

Replace the stub:

```go
func (t *ThingsMCP) diagnoseDataIntegrity(state *memory.State, report *diagReport, allWarnings *[]string) {
	step := diagStep{Step: 6, Name: "data_integrity", Description: "Check data completeness and integrity"}
	start := time.Now()
	step.Log = append(step.Log, fmt.Sprintf("Analyzing %d tasks...", len(state.Tasks)))

	// Count tasks by status
	statusCounts := map[string]int{}
	yearCounts := map[int]int{}
	var oldest, newest time.Time
	totalTasks := 0

	for _, task := range state.Tasks {
		if task.Type != thingscloud.TaskTypeTask {
			continue
		}
		totalTasks++

		switch {
		case task.InTrash:
			statusCounts["trashed"]++
		case task.Status == thingscloud.TaskStatusCompleted:
			statusCounts["completed"]++
		case task.Status == thingscloud.TaskStatusCanceled:
			statusCounts["canceled"]++
		default:
			statusCounts["active"]++
		}

		if !task.CreationDate.IsZero() && task.CreationDate.Year() > 1970 {
			year := task.CreationDate.Year()
			yearCounts[year]++
			if oldest.IsZero() || task.CreationDate.Before(oldest) {
				oldest = task.CreationDate
			}
			if newest.IsZero() || task.CreationDate.After(newest) {
				newest = task.CreationDate
			}
		}
	}

	step.Details = map[string]any{
		"totalTasks":    totalTasks,
		"tasksByStatus": statusCounts,
		"tasksByYear":   yearCounts,
		"oldestTask":    oldest.Format(time.RFC3339),
		"newestTask":    newest.Format(time.RFC3339),
	}

	step.Log = append(step.Log, fmt.Sprintf("Date range: %s to %s",
		oldest.Format("2006-01-02"), newest.Format("2006-01-02")))

	// Build year distribution log
	if len(yearCounts) > 0 {
		years := make([]int, 0, len(yearCounts))
		for y := range yearCounts {
			years = append(years, y)
		}
		sort.Ints(years)
		var parts []string
		for _, y := range years {
			parts = append(parts, fmt.Sprintf("%d=%d", y, yearCounts[y]))
		}
		step.Log = append(step.Log, fmt.Sprintf("Year distribution: %s", strings.Join(parts, ", ")))
	}

	step.Status = "pass"

	// Detect anomalies
	currentYear := time.Now().Year()
	if !newest.IsZero() && newest.Year() < currentYear-1 {
		warning := fmt.Sprintf("No tasks found after %d — possible incomplete sync", newest.Year())
		step.Log = append(step.Log, "WARNING: "+warning)
		*allWarnings = append(*allWarnings, warning)
		step.Status = "warn"
	}

	// Check for year gaps (missing years in the middle)
	if len(yearCounts) > 1 {
		years := make([]int, 0, len(yearCounts))
		for y := range yearCounts {
			years = append(years, y)
		}
		sort.Ints(years)
		for i := 1; i < len(years); i++ {
			gap := years[i] - years[i-1]
			if gap > 1 {
				warning := fmt.Sprintf("Gap detected: no tasks in years %d–%d", years[i-1]+1, years[i]-1)
				step.Log = append(step.Log, "WARNING: "+warning)
				*allWarnings = append(*allWarnings, warning)
				step.Status = "warn"
			}
		}
	}

	// Check active task ratio
	if totalTasks > 0 {
		activeCount := statusCounts["active"]
		step.Log = append(step.Log, fmt.Sprintf("Status: %d active, %d completed, %d canceled, %d trashed",
			activeCount, statusCounts["completed"], statusCounts["canceled"], statusCounts["trashed"]))
	}

	step.DurationMs = time.Since(start).Milliseconds()
	report.Steps = append(report.Steps, step)
}
```

**Step 2: Add `sort` import if not already present**

The `sort` package should be added to the import block. Check if it's already there; if not, add `"sort"` to the imports.

**Step 3: Verify it compiles**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: compiles without errors

**Step 4: Commit**

```bash
git add main.go
git commit -m "Add data integrity analysis step to diagnosis"
```

---

### Task 5: Implement step 7 (query tests)

**Files:**
- Modify: `main.go` (replace `diagnoseQueryTests` stub)

**Step 1: Implement query tests**

Replace the stub. This step uses the existing cached state (via `syncAndRebuild`) to test that basic operations work:

```go
func (t *ThingsMCP) diagnoseQueryTests(report *diagReport, allWarnings *[]string, allErrors *[]string) {
	step := diagStep{Step: 7, Name: "query_tests", Description: "Test basic list/query operations"}
	start := time.Now()
	step.Log = append(step.Log, "Running query tests against synced state...")

	type queryResult struct {
		Name  string `json:"name"`
		OK    bool   `json:"ok"`
		Count int    `json:"count,omitempty"`
		Error string `json:"error,omitempty"`
	}
	var results []queryResult
	allOK := true

	// Test: sync and rebuild
	step.Log = append(step.Log, "Syncing and rebuilding state...")
	err := t.syncAndRebuild()
	if err != nil {
		r := queryResult{Name: "syncAndRebuild", OK: false, Error: err.Error()}
		results = append(results, r)
		step.Log = append(step.Log, fmt.Sprintf("syncAndRebuild... FAIL: %v", err))
		allOK = false
	} else {
		results = append(results, queryResult{Name: "syncAndRebuild", OK: true})
		step.Log = append(step.Log, "syncAndRebuild... OK")
	}

	state := t.getState()

	// Test: list active tasks
	activeTasks := 0
	for _, task := range state.Tasks {
		if task.Type == thingscloud.TaskTypeTask && !task.InTrash &&
			task.Status != thingscloud.TaskStatusCompleted &&
			task.Status != thingscloud.TaskStatusCanceled {
			activeTasks++
		}
	}
	results = append(results, queryResult{Name: "listActiveTasks", OK: true, Count: activeTasks})
	step.Log = append(step.Log, fmt.Sprintf("listActiveTasks... OK (%d tasks)", activeTasks))

	// Test: list projects
	projectCount := 0
	for _, task := range state.Tasks {
		if task.Type == thingscloud.TaskTypeProject && !task.InTrash &&
			task.Status != thingscloud.TaskStatusCompleted {
			projectCount++
		}
	}
	results = append(results, queryResult{Name: "listProjects", OK: true, Count: projectCount})
	step.Log = append(step.Log, fmt.Sprintf("listProjects... OK (%d projects)", projectCount))

	// Test: list areas
	areaCount := len(state.Areas)
	results = append(results, queryResult{Name: "listAreas", OK: true, Count: areaCount})
	step.Log = append(step.Log, fmt.Sprintf("listAreas... OK (%d areas)", areaCount))

	// Test: list tags
	tagCount := len(state.Tags)
	results = append(results, queryResult{Name: "listTags", OK: true, Count: tagCount})
	step.Log = append(step.Log, fmt.Sprintf("listTags... OK (%d tags)", tagCount))

	step.DurationMs = time.Since(start).Milliseconds()
	step.Details = map[string]any{"results": results}

	if !allOK {
		step.Status = "fail"
		*allErrors = append(*allErrors, "Some query tests failed")
	} else {
		step.Status = "pass"
	}

	report.Steps = append(report.Steps, step)
}
```

**Step 2: Verify it compiles**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: compiles without errors

**Step 3: Commit**

```bash
git add main.go
git commit -m "Add query tests step to diagnosis"
```

---

### Task 6: Register the tool in defineTools and wire the handler

**Files:**
- Modify: `main.go` — add tool definition in `defineTools()` and connect it to `handleDiagnose`

**Step 1: Add the tool to `defineTools()`**

At the end of the `return []server.ServerTool{` array (before the closing `}` of `defineTools`, around line 2080), add:

```go
		// --- Diagnosis tool ---
		{
			Tool: mcp.NewTool("things_diagnose",
				mcp.WithDescription("Run a full diagnostic of the Things Cloud sync pipeline. Tests credentials, fetches history, paginates through all items, rebuilds state, checks data integrity, and runs query tests. Returns a detailed step-by-step report with logs, timing, and any warnings or errors. Use this to debug sync issues like missing or stale tasks."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
			),
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				val := ctx.Value(userContextKey)
				if val == nil {
					return errResult("authentication required"), nil
				}
				info, ok := val.(*UserInfo)
				if !ok {
					return errResult("invalid user context"), nil
				}

				var email, password string
				if info.Token != "" {
					if um.oauth == nil {
						return errResult("Bearer token authentication not configured"), nil
					}
					var err error
					email, password, err = um.oauth.ResolveBearer(info.Token)
					if err != nil {
						return errResult(fmt.Sprintf("Bearer auth failed: %v", err)), nil
					}
				} else {
					email = info.Email
					password = info.Password
				}

				if email == "" || password == "" {
					return errResult("invalid credentials"), nil
				}

				t, err := getUserFromContext(ctx, um)
				if err != nil {
					return errResult(err.Error()), nil
				}

				report := t.handleDiagnose(email, password)
				return jsonResult(report), nil
			},
		},
```

Note: This handler needs direct access to email/password (for creating a fresh client in the diagnosis), so it doesn't use the `wrap()` helper and instead resolves credentials directly. It also gets the `ThingsMCP` instance for query tests.

**Step 2: Verify it compiles**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build ./...`
Expected: compiles without errors

**Step 3: Commit**

```bash
git add main.go
git commit -m "Register things_diagnose tool in defineTools"
```

---

### Task 7: Verify and clean up

**Step 1: Run go vet**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go vet ./...`
Expected: no issues

**Step 2: Final build**

Run: `cd /home/wenbo/Repos/things-cloud-mcp && go build -o things-cloud-mcp ./...`
Expected: builds successfully

**Step 3: Final commit (if any cleanup needed)**

```bash
git add main.go
git commit -m "feat: add things_diagnose tool for debugging sync issues

Addresses #1. The new tool runs 7 diagnostic steps:
1. Credential verification
2. Fetch history
3. Sync history
4. Paginated fetch with per-page logging
5. State rebuild
6. Data integrity analysis (year gaps, status distribution)
7. Query tests (list tasks/projects/areas/tags)

Returns a structured JSON report with status, timing, details,
and human-readable logs for each step."
```
