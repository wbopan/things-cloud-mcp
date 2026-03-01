# Design: `things_diagnose` MCP Tool

**Date:** 2026-03-01
**Issue:** https://github.com/wbopan/things-cloud-mcp/issues/1
**Problem:** Users report only fetching tasks up to 2020. No way to debug sync issues.

## Decision

Single MCP tool `things_diagnose` that performs a full end-to-end test of the sync pipeline, capturing detailed logs at each step. Returns a structured JSON diagnosis report.

## Tool Interface

- **Name:** `things_diagnose`
- **Parameters:** none
- **Annotations:** ReadOnlyHint=true, DestructiveHint=false, IdempotentHint=true

## Report Structure

The report runs 7 sequential diagnostic steps. Each step has:
- `step` (int) — order number
- `name` (string) — machine-readable identifier
- `description` (string) — human-readable description
- `status` (string) — `pass`, `warn`, `fail`, or `skipped`
- `durationMs` (int) — execution time
- `details` (object) — structured data specific to the step
- `log` ([]string) — human-readable log lines

If a step fails, subsequent steps are marked `skipped`.

### Step 1: credential_verification
- Call `client.Verify()`
- Report: masked email, auth method, HTTP response status

### Step 2: fetch_history
- Call `client.OwnHistory()`
- Report: whether history object was obtained

### Step 3: sync_history
- Call `history.Sync()`
- Report: success/failure

### Step 4: paginated_fetch
- Iterate `history.Items()` with pagination
- Report per-page: startIndex, itemsFetched, serverIndexAfter
- Report totals: totalItemsFetched, iterations count, finalServerIndex

### Step 5: rebuild_state
- Build in-memory state from fetched items
- Report: count of tasks, areas, tags, checklist items

### Step 6: data_integrity
- Analyze task distribution by status and year
- Detect anomalies: year gaps, missing recent data, unusual status ratios
- Report: tasksByStatus, tasksByYear, oldestTask, newestTask

### Step 7: query_tests
- Execute basic list operations (tasks, projects, areas, tags)
- Report: success/failure and counts for each

### Summary

Top-level summary with pass/warn/fail counts, total duration, and aggregated warnings/errors arrays.

## Implementation Notes

- Refactor `rebuildState()` to return a `DiagnosticInfo` struct with pagination details
- Create a new `handleDiagnose()` function that orchestrates all 7 steps
- Each step catches errors and continues to next (or skips if dependency failed)
- Mask email in output (show first char + `***` + domain)
- Register tool in `defineTools()` alongside existing tools
