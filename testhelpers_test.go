package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
	memory "github.com/arthursoares/things-cloud-sdk/state/memory"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Fake Things Cloud HTTP server
// ---------------------------------------------------------------------------

type fakeCloud struct {
	server    *httptest.Server
	email     string
	historyID string
	items     []thingscloud.Item
	itemIndex int // current-item-index to report

	mu        sync.Mutex
	commitLog []json.RawMessage // captured POST bodies
}

func newFakeCloud(email string, items ...thingscloud.Item) *fakeCloud {
	fc := &fakeCloud{
		email:     email,
		historyID: "test-history-001",
		items:     items,
		itemIndex: len(items),
	}

	mux := http.NewServeMux()

	// GET /version/1/account/{email} — Verify
	mux.HandleFunc("GET /version/1/account/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "SYAccountStatusActive",
			"history-key": fc.historyID,
			"email":       fc.email,
		})
	})

	// GET /version/1/account/{email}/own-history-keys — Histories
	mux.HandleFunc("GET /version/1/account/{email}/own-history-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{fc.historyID})
	})

	// GET /version/1/history/{id} (no /items suffix) — History metadata
	mux.HandleFunc("GET /version/1/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Only match the exact path, not sub-paths like /items
		if strings.Contains(r.URL.Path, "/items") || strings.Contains(r.URL.Path, "/commit") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"latest-server-index": fc.itemIndex,
			"schema":              301,
		})
	})

	// GET /version/1/history/{id}/items — Items fetch
	mux.HandleFunc("GET /version/1/history/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		startIdx := r.URL.Query().Get("start-index")
		w.Header().Set("Content-Type", "application/json")

		if startIdx == "0" && len(fc.items) > 0 {
			// Return all items on first fetch
			wireItems := make([]map[string]any, len(fc.items))
			for i, item := range fc.items {
				itemData := map[string]any{
					"p": json.RawMessage(item.P),
					"e": item.Kind,
					"t": item.Action,
				}
				wireItems[i] = map[string]any{
					item.UUID: itemData,
				}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"items":              wireItems,
				"current-item-index": fc.itemIndex,
				"schema":             301,
			})
		} else {
			// Empty response (terminates sync/rebuild loops)
			json.NewEncoder(w).Encode(map[string]any{
				"items":              []any{},
				"current-item-index": fc.itemIndex,
				"schema":             301,
			})
		}
	})

	// POST /version/1/history/{id}/commit — Write
	mux.HandleFunc("POST /version/1/history/{id}/commit", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fc.mu.Lock()
		fc.commitLog = append(fc.commitLog, json.RawMessage(body))
		fc.itemIndex++
		fc.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"server-head-index": fc.itemIndex,
		})
	})

	fc.server = httptest.NewServer(mux)
	return fc
}

func (fc *fakeCloud) Close() {
	fc.server.Close()
}

func (fc *fakeCloud) getCommitLog() []json.RawMessage {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	out := make([]json.RawMessage, len(fc.commitLog))
	copy(out, fc.commitLog)
	return out
}

// ---------------------------------------------------------------------------
// Build ThingsMCP from fake server
// ---------------------------------------------------------------------------

func newTestThingsMCP(t *testing.T, fc *fakeCloud) *ThingsMCP {
	t.Helper()

	c := thingscloud.New(fc.server.URL, fc.email, "testpass")
	if _, err := c.Verify(); err != nil {
		t.Fatalf("fake Verify failed: %v", err)
	}

	history, err := bestHistory(c)
	if err != nil {
		t.Fatalf("fake bestHistory failed: %v", err)
	}
	if err := history.Sync(); err != nil {
		t.Fatalf("fake Sync failed: %v", err)
	}

	tmcp := &ThingsMCP{client: c, history: history}
	if err := tmcp.rebuildState(); err != nil {
		t.Fatalf("rebuildState failed: %v", err)
	}
	return tmcp
}

// newTestThingsMCPDirect creates a ThingsMCP with pre-built state (no HTTP).
// Useful for handler tests that don't need write operations.
func newTestThingsMCPDirect(state *memory.State) *ThingsMCP {
	return &ThingsMCP{state: state}
}

// ---------------------------------------------------------------------------
// Test data builders (functional options pattern)
// ---------------------------------------------------------------------------

type taskOption func(p *thingscloud.TaskActionItemPayload)

func withTitle(s string) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		p.Title = &s
	}
}

func withStatus(s thingscloud.TaskStatus) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		p.Status = &s
	}
}

func withSchedule(s thingscloud.TaskSchedule) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		p.Schedule = &s
	}
}

func withScheduledDate(t time.Time) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		ts := thingscloud.Timestamp(t)
		p.ScheduledDate = &ts
	}
}

func withDeadline(t time.Time) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		ts := thingscloud.Timestamp(t)
		p.DeadlineDate = &ts
	}
}

func withCreationDate(t time.Time) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		ts := thingscloud.Timestamp(t)
		p.CreationDate = &ts
	}
}

func withTaskType(tp thingscloud.TaskType) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		p.Type = &tp
	}
}

func withParent(uuid string) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		ids := []string{uuid}
		p.ParentTaskIDs = &ids
	}
}

func withArea(uuid string) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		ids := []string{uuid}
		p.AreaIDs = &ids
	}
}

func withTags(uuids ...string) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		p.TagIDs = uuids
	}
}

func withActionGroup(uuid string) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		ids := []string{uuid}
		p.ActionGroupIDs = &ids
	}
}

func withTrashed() taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		tr := true
		p.InTrash = &tr
	}
}

func withNote(text string) taskOption {
	return func(p *thingscloud.TaskActionItemPayload) {
		note := thingscloud.Note{
			TypeTag: "tx",
			Type:    thingscloud.NoteTypeFullText,
			Value:   text,
		}
		raw, _ := json.Marshal(note)
		p.Note = raw
	}
}

func makeTaskItem(uuid string, opts ...taskOption) thingscloud.Item {
	p := &thingscloud.TaskActionItemPayload{}
	// Defaults: pending task
	status := thingscloud.TaskStatusPending
	p.Status = &status
	tp := thingscloud.TaskTypeTask
	p.Type = &tp
	sched := thingscloud.TaskScheduleAnytime
	p.Schedule = &sched

	for _, opt := range opts {
		opt(p)
	}

	raw, _ := json.Marshal(p)
	return thingscloud.Item{
		UUID:   uuid,
		P:      raw,
		Kind:   thingscloud.ItemKindTask,
		Action: thingscloud.ItemActionCreated,
	}
}

func makeAreaItem(uuid, title string) thingscloud.Item {
	p := thingscloud.AreaActionItemPayload{Title: &title}
	raw, _ := json.Marshal(p)
	return thingscloud.Item{
		UUID:   uuid,
		P:      raw,
		Kind:   thingscloud.ItemKindArea,
		Action: thingscloud.ItemActionCreated,
	}
}

func makeTagItem(uuid, title string) thingscloud.Item {
	p := thingscloud.TagActionItemPayload{Title: &title}
	raw, _ := json.Marshal(p)
	return thingscloud.Item{
		UUID:   uuid,
		P:      raw,
		Kind:   thingscloud.ItemKindTag,
		Action: thingscloud.ItemActionCreated,
	}
}

func makeChecklistItem(uuid, taskUUID, title string) thingscloud.Item {
	taskIDs := []string{taskUUID}
	p := thingscloud.CheckListActionItemPayload{
		Title:   &title,
		TaskIDs: &taskIDs,
	}
	raw, _ := json.Marshal(p)
	return thingscloud.Item{
		UUID:   uuid,
		P:      raw,
		Kind:   thingscloud.ItemKindChecklistItem,
		Action: thingscloud.ItemActionCreated,
	}
}

// ---------------------------------------------------------------------------
// Request/result helpers
// ---------------------------------------------------------------------------

func makeReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func resultText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if len(r.Content) == 0 {
		t.Fatal("empty result content")
	}
	tc, ok := r.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", r.Content[0])
	}
	return tc.Text
}

func resultJSON[T any](t *testing.T, r *mcp.CallToolResult) T {
	t.Helper()
	text := resultText(t, r)
	var v T
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("unmarshal result: %v\ntext: %s", err, text)
	}
	return v
}

func assertIsError(t *testing.T, r *mcp.CallToolResult) {
	t.Helper()
	if !r.IsError {
		t.Fatalf("expected error result, got: %s", resultText(t, r))
	}
}

func assertNotError(t *testing.T, r *mcp.CallToolResult) {
	t.Helper()
	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, r))
	}
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T { return &v }

// mustTime parses a date string or panics.
func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(fmt.Sprintf("mustTime(%q): %v", s, err))
	}
	return t
}
