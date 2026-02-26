package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
	memory "github.com/arthursoares/things-cloud-sdk/state/memory"
)

// ---------------------------------------------------------------------------
// Wire-format types (no omitempty — Things expects all fields on creates)
// ---------------------------------------------------------------------------

type WireNote struct {
	TypeTag  string `json:"_t"`
	Checksum int64  `json:"ch"`
	Value    string `json:"v"`
	Type     int    `json:"t"`
}

type WireExtension struct {
	Sn      map[string]any `json:"sn"`
	TypeTag string         `json:"_t"`
}

type writeEnvelope struct {
	id      string
	action  int
	kind    string
	payload any
}

func (w writeEnvelope) UUID() string { return w.id }

func (w writeEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		T int    `json:"t"`
		E string `json:"e"`
		P any    `json:"p"`
	}{w.action, w.kind, w.payload})
}

type TaskCreatePayload struct {
	Tp   int              `json:"tp"`
	Sr   *int64           `json:"sr"`
	Dds  *int64           `json:"dds"`
	Rt   []string         `json:"rt"`
	Rmd  *int64           `json:"rmd"`
	Ss   int              `json:"ss"`
	Tr   bool             `json:"tr"`
	Dl   []string         `json:"dl"`
	Icp  bool             `json:"icp"`
	St   int              `json:"st"`
	Ar   []string         `json:"ar"`
	Tt   string           `json:"tt"`
	Do   int              `json:"do"`
	Lai  *int64           `json:"lai"`
	Tir  *int64           `json:"tir"`
	Tg   []string         `json:"tg"`
	Agr  []string         `json:"agr"`
	Ix   int              `json:"ix"`
	Cd   float64          `json:"cd"`
	Lt   bool             `json:"lt"`
	Icc  int              `json:"icc"`
	Md   *float64         `json:"md"`
	Ti   int              `json:"ti"`
	Dd   *int64           `json:"dd"`
	Ato  *int             `json:"ato"`
	Nt   WireNote         `json:"nt"`
	Icsd *int64           `json:"icsd"`
	Pr   []string         `json:"pr"`
	Rp   *string          `json:"rp"`
	Acrd *int64           `json:"acrd"`
	Sp   *float64         `json:"sp"`
	Sb   int              `json:"sb"`
	Rr   *json.RawMessage `json:"rr"`
	Xx   WireExtension    `json:"xx"`
}

type ChecklistItemCreatePayload struct {
	Cd float64       `json:"cd"`
	Md *float64      `json:"md"`
	Tt string        `json:"tt"`
	Ss int           `json:"ss"`
	Sp *float64      `json:"sp"`
	Ix int           `json:"ix"`
	Ts []string      `json:"ts"`
	Lt bool          `json:"lt"`
	Xx WireExtension `json:"xx"`
}

type TagCreatePayload struct {
	Tt string        `json:"tt"`
	Ix int           `json:"ix"`
	Sh *string       `json:"sh"`
	Pn []string      `json:"pn"`
	Xx WireExtension `json:"xx"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func emptyNote() WireNote {
	return WireNote{TypeTag: "tx", Checksum: 0, Value: "", Type: 1}
}

func noteChecksum(s string) int64 {
	return int64(crc32.ChecksumIEEE([]byte(s)))
}

func textNote(s string) WireNote {
	return WireNote{TypeTag: "tx", Checksum: noteChecksum(s), Value: s, Type: 1}
}

func defaultExtension() WireExtension {
	return WireExtension{Sn: map[string]any{}, TypeTag: "oo"}
}

func generateUUID() string {
	u := uuid.New()
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	n := new(big.Int).SetBytes(u[:])
	base := big.NewInt(58)
	mod := new(big.Int)
	var encoded []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

func nowTs() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

func todayMidnightUTC() int64 {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

func parseDate(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// ---------------------------------------------------------------------------
// Payload builders
// ---------------------------------------------------------------------------

func newTaskCreatePayload(title string, opts map[string]string) TaskCreatePayload {
	now := nowTs()
	var st int
	var sr *int64
	var tir *int64
	var dd *int64
	tp := 0
	pr := []string{}
	agr := []string{}
	ar := []string{}
	tg := []string{}
	nt := emptyNote()

	if v, ok := opts["type"]; ok {
		switch v {
		case "project":
			tp = 1
		case "heading":
			tp = 2
			st = 1
		}
	}
	if v, ok := opts["when"]; ok {
		switch v {
		case "today":
			st = 1
			today := todayMidnightUTC()
			sr = &today
			tir = &today
		case "anytime":
			st = 1
		case "someday":
			st = 2
		case "inbox":
			st = 0
		}
	}
	if v, ok := opts["note"]; ok && v != "" {
		nt = textNote(v)
	}
	if v, ok := opts["deadline"]; ok {
		if t := parseDate(v); t != nil {
			ts := t.Unix()
			dd = &ts
		}
	}
	if v, ok := opts["scheduled"]; ok {
		if t := parseDate(v); t != nil {
			ts := t.Unix()
			sr = &ts
			tir = &ts
			if _, hasWhen := opts["when"]; !hasWhen {
				st = 1
			}
		}
	}
	if v, ok := opts["project_uuid"]; ok && v != "" {
		pr = []string{v}
		if _, hasWhen := opts["when"]; !hasWhen {
			st = 1
		}
	}
	if v, ok := opts["heading_uuid"]; ok && v != "" {
		agr = []string{v}
		if _, hasWhen := opts["when"]; !hasWhen {
			st = 1
		}
	}
	if v, ok := opts["area_uuid"]; ok && v != "" {
		ar = []string{v}
		if _, hasWhen := opts["when"]; !hasWhen {
			st = 1
		}
	}
	if v, ok := opts["tags"]; ok && v != "" {
		tg = strings.Split(v, ",")
	}

	return TaskCreatePayload{
		Tp: tp, Sr: sr, Dds: nil, Rt: []string{}, Rmd: nil,
		Ss: 0, Tr: false, Dl: []string{}, Icp: false, St: st,
		Ar: ar, Tt: title, Do: 0, Lai: nil, Tir: tir,
		Tg: tg, Agr: agr, Ix: 0, Cd: now, Lt: false,
		Icc: 0, Md: nil, Ti: 0, Dd: dd, Ato: nil,
		Nt: nt, Icsd: nil, Pr: pr, Rp: nil, Acrd: nil,
		Sp: nil, Sb: 0, Rr: nil, Xx: defaultExtension(),
	}
}

// ---------------------------------------------------------------------------
// Fluent update builder
// ---------------------------------------------------------------------------

type taskUpdate struct {
	fields map[string]any
}

func newTaskUpdate() *taskUpdate {
	return &taskUpdate{fields: map[string]any{"md": nowTs()}}
}

func (u *taskUpdate) Title(s string) *taskUpdate       { u.fields["tt"] = s; return u }
func (u *taskUpdate) Note(text string) *taskUpdate      { u.fields["nt"] = textNote(text); return u }
func (u *taskUpdate) ClearNote() *taskUpdate            { u.fields["nt"] = emptyNote(); return u }
func (u *taskUpdate) Status(ss int) *taskUpdate         { u.fields["ss"] = ss; return u }
func (u *taskUpdate) StopDate(ts float64) *taskUpdate   { u.fields["sp"] = ts; return u }
func (u *taskUpdate) Trash(b bool) *taskUpdate          { u.fields["tr"] = b; return u }
func (u *taskUpdate) Deadline(dd int64) *taskUpdate     { u.fields["dd"] = dd; return u }
func (u *taskUpdate) Scheduled(sr, tir int64) *taskUpdate {
	u.fields["sr"] = sr
	u.fields["tir"] = tir
	return u
}
func (u *taskUpdate) Area(uuid string) *taskUpdate    { u.fields["ar"] = []string{uuid}; return u }
func (u *taskUpdate) Project(uuid string) *taskUpdate { u.fields["pr"] = []string{uuid}; return u }
func (u *taskUpdate) Heading(uuid string) *taskUpdate { u.fields["agr"] = []string{uuid}; return u }
func (u *taskUpdate) Tags(uuids []string) *taskUpdate { u.fields["tg"] = uuids; return u }
func (u *taskUpdate) Schedule(st int, sr, tir any) *taskUpdate {
	u.fields["st"] = st
	u.fields["sr"] = sr
	u.fields["tir"] = tir
	return u
}
func (u *taskUpdate) build() map[string]any { return u.fields }

// ---------------------------------------------------------------------------
// JSON output types
// ---------------------------------------------------------------------------

type TaskOutput struct {
	UUID          string   `json:"uuid"`
	Title         string   `json:"title"`
	Note          string   `json:"note,omitempty"`
	Status        int      `json:"status"`
	InTrash       bool     `json:"inTrash"`
	IsProject     bool     `json:"isProject"`
	Schedule      int      `json:"schedule"`
	ScheduledDate *string  `json:"scheduledDate,omitempty"`
	DeadlineDate  *string  `json:"deadlineDate,omitempty"`
	AreaIDs       []string `json:"areaIds,omitempty"`
	ParentIDs     []string `json:"parentIds,omitempty"`
	TagIDs        []string `json:"tagIds,omitempty"`
}

type ChecklistOutput struct {
	UUID   string `json:"uuid"`
	Title  string `json:"title"`
	Status int    `json:"status"`
}

type TaskDetailOutput struct {
	TaskOutput
	Checklist []ChecklistOutput `json:"checklist,omitempty"`
}

func taskToOutput(t *thingscloud.Task) TaskOutput {
	out := TaskOutput{
		UUID:      t.UUID,
		Title:     t.Title,
		Note:      t.Note,
		Status:    int(t.Status),
		InTrash:   t.InTrash,
		IsProject: t.Type == thingscloud.TaskTypeProject,
		Schedule:  int(t.Schedule),
		AreaIDs:   t.AreaIDs,
		ParentIDs: t.ParentTaskIDs,
		TagIDs:    t.TagIDs,
	}
	if t.ScheduledDate != nil {
		s := t.ScheduledDate.Format("2006-01-02")
		out.ScheduledDate = &s
	}
	if t.DeadlineDate != nil {
		s := t.DeadlineDate.Format("2006-01-02")
		out.DeadlineDate = &s
	}
	return out
}

// ---------------------------------------------------------------------------
// ThingsMCP — server state
// ---------------------------------------------------------------------------

type ThingsMCP struct {
	client  *thingscloud.Client
	history *thingscloud.History
	state   *memory.State
	mu      sync.RWMutex
}

func NewThingsMCP() (*ThingsMCP, error) {
	username := os.Getenv("THINGS_USERNAME")
	password := os.Getenv("THINGS_PASSWORD")
	if username == "" || password == "" {
		return nil, fmt.Errorf("THINGS_USERNAME and THINGS_PASSWORD are required")
	}

	c := thingscloud.New(thingscloud.APIEndpoint, username, password)
	if os.Getenv("THINGS_DEBUG") != "" {
		c.Debug = true
	}

	log.Println("Verifying Things Cloud credentials...")
	if _, err := c.Verify(); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	log.Println("Credentials verified.")

	log.Println("Fetching history...")
	history, err := c.OwnHistory()
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	if err := history.Sync(); err != nil {
		return nil, fmt.Errorf("sync history: %w", err)
	}
	log.Println("History synced.")

	t := &ThingsMCP{client: c, history: history}
	if err := t.rebuildState(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *ThingsMCP) rebuildState() error {
	var allItems []thingscloud.Item
	startIndex := 0
	for {
		items, hasMore, err := t.history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			return fmt.Errorf("fetch items: %w", err)
		}
		allItems = append(allItems, items...)
		if !hasMore {
			break
		}
		startIndex = t.history.LoadedServerIndex
	}

	state := memory.NewState()
	state.Update(allItems...)

	t.mu.Lock()
	t.state = state
	t.mu.Unlock()

	taskCount := len(state.Tasks)
	areaCount := len(state.Areas)
	tagCount := len(state.Tags)
	log.Printf("State rebuilt: %d tasks, %d areas, %d tags", taskCount, areaCount, tagCount)
	return nil
}

func (t *ThingsMCP) getState() *memory.State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

func (t *ThingsMCP) writeAndSync(items ...thingscloud.Identifiable) error {
	if err := t.history.Write(items...); err != nil {
		return err
	}
	return t.rebuildState()
}

// ---------------------------------------------------------------------------
// Helper: find by name
// ---------------------------------------------------------------------------

func (t *ThingsMCP) findAreaUUID(name string) string {
	state := t.getState()
	for _, area := range state.Areas {
		if strings.EqualFold(area.Title, name) {
			return area.UUID
		}
	}
	return ""
}

func (t *ThingsMCP) findProjectUUID(name string) string {
	state := t.getState()
	for _, task := range state.Tasks {
		if task.Type == thingscloud.TaskTypeProject && strings.EqualFold(task.Title, name) {
			return task.UUID
		}
	}
	return ""
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// UUID validation
// ---------------------------------------------------------------------------

func (t *ThingsMCP) validateTaskUUID(uuid string) error {
	state := t.getState()
	for _, task := range state.Tasks {
		if task.UUID == uuid && !task.InTrash {
			return nil
		}
	}
	return fmt.Errorf("task not found: %s", uuid)
}

func (t *ThingsMCP) validateProjectUUID(uuid string) error {
	state := t.getState()
	for _, task := range state.Tasks {
		if task.UUID == uuid && task.Type == thingscloud.TaskTypeProject && !task.InTrash {
			return nil
		}
	}
	return fmt.Errorf("project not found: %s", uuid)
}

func (t *ThingsMCP) validateHeadingUUID(uuid string) error {
	state := t.getState()
	for _, task := range state.Tasks {
		if task.UUID == uuid && task.Type == thingscloud.TaskTypeHeading && !task.InTrash {
			return nil
		}
	}
	return fmt.Errorf("heading not found: %s", uuid)
}

func (t *ThingsMCP) validateAreaUUID(uuid string) error {
	state := t.getState()
	for _, area := range state.Areas {
		if area.UUID == uuid {
			return nil
		}
	}
	return fmt.Errorf("area not found: %s", uuid)
}

func (t *ThingsMCP) validateTagUUIDs(csv string) error {
	state := t.getState()
	tagSet := make(map[string]bool)
	for _, tag := range state.Tags {
		tagSet[tag.UUID] = true
	}
	for _, id := range strings.Split(csv, ",") {
		id = strings.TrimSpace(id)
		if !tagSet[id] {
			return fmt.Errorf("tag not found: %s", id)
		}
	}
	return nil
}

// validateOpts checks that any UUID references in opts point to existing items.
func (t *ThingsMCP) validateOpts(opts map[string]string) error {
	if v, ok := opts["project_uuid"]; ok && v != "" {
		if err := t.validateProjectUUID(v); err != nil {
			return err
		}
	}
	if v, ok := opts["heading_uuid"]; ok && v != "" {
		if err := t.validateHeadingUUID(v); err != nil {
			return err
		}
	}
	if v, ok := opts["area_uuid"]; ok && v != "" {
		if err := t.validateAreaUUID(v); err != nil {
			return err
		}
	}
	if v, ok := opts["tags"]; ok && v != "" {
		if err := t.validateTagUUIDs(v); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tool result helpers
// ---------------------------------------------------------------------------

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("json marshal: %v", err))
	}
	return mcp.NewToolResultText(string(b))
}

func errResult(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultError(msg)
}

// ---------------------------------------------------------------------------
// MCP Tool handlers
// ---------------------------------------------------------------------------

func (t *ThingsMCP) handleListTasks(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := t.getState()
	filter := req.GetString("filter", "")
	areaName := req.GetString("area", "")
	projectName := req.GetString("project", "")

	// Parse filter: keywords or date/date-range
	var filterFn func(*thingscloud.Task) bool
	switch filter {
	case "":
		filterFn = nil
	case "today":
		todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.UTC)
		filterFn = func(task *thingscloud.Task) bool {
			return task.Schedule == thingscloud.TaskScheduleAnytime && task.ScheduledDate != nil && task.ScheduledDate.Equal(todayStart)
		}
	case "inbox":
		filterFn = func(task *thingscloud.Task) bool {
			return task.Schedule == thingscloud.TaskScheduleInbox
		}
	case "anytime":
		filterFn = func(task *thingscloud.Task) bool {
			return task.Schedule == thingscloud.TaskScheduleAnytime
		}
	case "someday":
		filterFn = func(task *thingscloud.Task) bool {
			return task.Schedule == 2
		}
	default:
		// Try date range: YYYY-MM-DD..YYYY-MM-DD or single date: YYYY-MM-DD
		if parts := strings.SplitN(filter, "..", 2); len(parts) == 2 {
			from := parseDate(parts[0])
			to := parseDate(parts[1])
			if from == nil || to == nil {
				return errResult(fmt.Sprintf("invalid date range: %s", filter)), nil
			}
			filterFn = func(task *thingscloud.Task) bool {
				if task.ScheduledDate == nil {
					return false
				}
				return !task.ScheduledDate.Before(*from) && !task.ScheduledDate.After(*to)
			}
		} else {
			date := parseDate(filter)
			if date == nil {
				return errResult(fmt.Sprintf("invalid filter: %s (use today/inbox/anytime/someday/YYYY-MM-DD/YYYY-MM-DD..YYYY-MM-DD)", filter)), nil
			}
			filterFn = func(task *thingscloud.Task) bool {
				return task.ScheduledDate != nil && task.ScheduledDate.Equal(*date)
			}
		}
	}

	var tasks []TaskOutput
	for _, task := range state.Tasks {
		if task.InTrash || task.Status == 3 || task.Type == thingscloud.TaskTypeProject || task.Type == thingscloud.TaskTypeHeading {
			continue
		}

		if filterFn != nil && !filterFn(task) {
			continue
		}

		if areaName != "" {
			areaUUID := t.findAreaUUID(areaName)
			if areaUUID == "" || !containsStr(task.AreaIDs, areaUUID) {
				continue
			}
		}
		if projectName != "" {
			projectUUID := t.findProjectUUID(projectName)
			if projectUUID == "" || !containsStr(task.ParentTaskIDs, projectUUID) {
				continue
			}
		}

		tasks = append(tasks, taskToOutput(task))
	}

	if tasks == nil {
		tasks = []TaskOutput{}
	}
	return jsonResult(tasks), nil
}

func (t *ThingsMCP) handleShowTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuidPrefix, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	state := t.getState()
	for _, task := range state.Tasks {
		if strings.HasPrefix(task.UUID, uuidPrefix) {
			out := TaskDetailOutput{TaskOutput: taskToOutput(task)}
			// Add checklist items
			for _, cli := range state.CheckListItems {
				if containsStr(cli.TaskIDs, task.UUID) {
					out.Checklist = append(out.Checklist, ChecklistOutput{
						UUID:   cli.UUID,
						Title:  cli.Title,
						Status: int(cli.Status),
					})
				}
			}
			return jsonResult(out), nil
		}
	}
	return errResult(fmt.Sprintf("task not found: %s", uuidPrefix)), nil
}

func (t *ThingsMCP) handleShowProject(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	state := t.getState()

	// Find the project
	var project *thingscloud.Task
	for _, task := range state.Tasks {
		if task.UUID == projectUUID && task.Type == thingscloud.TaskTypeProject {
			project = task
			break
		}
	}
	if project == nil {
		return errResult(fmt.Sprintf("project not found: %s", projectUUID)), nil
	}

	type HeadingWithTasks struct {
		UUID  string       `json:"uuid"`
		Title string       `json:"title"`
		Tasks []TaskOutput `json:"tasks"`
	}
	type ProjectDetailOutput struct {
		TaskOutput
		Headings     []HeadingWithTasks `json:"headings"`
		UnfiledTasks []TaskOutput       `json:"unfiledTasks"`
	}

	// Collect headings in this project
	headingMap := make(map[string]*HeadingWithTasks)
	var headingOrder []string
	for _, task := range state.Tasks {
		if task.Type == thingscloud.TaskTypeHeading && !task.InTrash && containsStr(task.ParentTaskIDs, projectUUID) {
			h := &HeadingWithTasks{UUID: task.UUID, Title: task.Title, Tasks: []TaskOutput{}}
			headingMap[task.UUID] = h
			headingOrder = append(headingOrder, task.UUID)
		}
	}

	// Collect tasks in this project, group by heading
	var unfiled []TaskOutput
	for _, task := range state.Tasks {
		if task.InTrash || task.Status == 3 || task.Type == thingscloud.TaskTypeProject || task.Type == thingscloud.TaskTypeHeading {
			continue
		}
		if !containsStr(task.ParentTaskIDs, projectUUID) {
			continue
		}
		placed := false
		for _, hid := range task.ActionGroupIDs {
			if h, ok := headingMap[hid]; ok {
				h.Tasks = append(h.Tasks, taskToOutput(task))
				placed = true
				break
			}
		}
		if !placed {
			unfiled = append(unfiled, taskToOutput(task))
		}
	}

	if unfiled == nil {
		unfiled = []TaskOutput{}
	}
	var headings []HeadingWithTasks
	for _, hid := range headingOrder {
		headings = append(headings, *headingMap[hid])
	}
	if headings == nil {
		headings = []HeadingWithTasks{}
	}

	out := ProjectDetailOutput{
		TaskOutput:   taskToOutput(project),
		Headings:     headings,
		UnfiledTasks: unfiled,
	}
	return jsonResult(out), nil
}

func (t *ThingsMCP) handleListProjects(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := t.getState()
	var projects []TaskOutput
	for _, task := range state.Tasks {
		if task.Type == thingscloud.TaskTypeProject && !task.InTrash && task.Status != 3 {
			projects = append(projects, taskToOutput(task))
		}
	}
	if projects == nil {
		projects = []TaskOutput{}
	}
	return jsonResult(projects), nil
}

func (t *ThingsMCP) handleListHeadings(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectUUID, err := req.RequireString("project_uuid")
	if err != nil {
		return errResult("project_uuid is required"), nil
	}

	state := t.getState()
	type HeadingOutput struct {
		UUID  string `json:"uuid"`
		Title string `json:"title"`
	}
	var headings []HeadingOutput
	for _, task := range state.Tasks {
		if task.Type == thingscloud.TaskTypeHeading && !task.InTrash && containsStr(task.ParentTaskIDs, projectUUID) {
			headings = append(headings, HeadingOutput{UUID: task.UUID, Title: task.Title})
		}
	}
	if headings == nil {
		headings = []HeadingOutput{}
	}
	return jsonResult(headings), nil
}

func (t *ThingsMCP) handleListAreas(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := t.getState()
	type AreaOutput struct {
		UUID  string `json:"uuid"`
		Title string `json:"title"`
	}
	var areas []AreaOutput
	for _, area := range state.Areas {
		areas = append(areas, AreaOutput{UUID: area.UUID, Title: area.Title})
	}
	if areas == nil {
		areas = []AreaOutput{}
	}
	return jsonResult(areas), nil
}

func (t *ThingsMCP) handleListTags(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := t.getState()
	type TagOutput struct {
		UUID      string   `json:"uuid"`
		Title     string   `json:"title"`
		Shorthand string   `json:"shorthand,omitempty"`
		ParentIDs []string `json:"parentIds,omitempty"`
	}
	var tags []TagOutput
	for _, tag := range state.Tags {
		tags = append(tags, TagOutput{
			UUID:      tag.UUID,
			Title:     tag.Title,
			Shorthand: tag.ShortHand,
			ParentIDs: tag.ParentTagIDs,
		})
	}
	if tags == nil {
		tags = []TagOutput{}
	}
	return jsonResult(tags), nil
}

func (t *ThingsMCP) handleCreateTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return errResult("title is required"), nil
	}

	opts := make(map[string]string)
	for _, key := range []string{"note", "when", "deadline", "scheduled", "project_uuid", "heading_uuid", "area_uuid", "tags", "checklist"} {
		if v := req.GetString(key, ""); v != "" {
			opts[key] = v
		}
	}

	if err := t.validateOpts(opts); err != nil {
		return errResult(err.Error()), nil
	}

	taskUUID := generateUUID()
	payload := newTaskCreatePayload(title, opts)
	env := writeEnvelope{id: taskUUID, action: 0, kind: "Task6", payload: payload}

	var envelopes []thingscloud.Identifiable
	envelopes = append(envelopes, env)

	// Checklist items
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
}

func (t *ThingsMCP) handleCreateProject(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return errResult("title is required"), nil
	}

	opts := map[string]string{"type": "project"}
	for _, key := range []string{"note", "when", "deadline", "scheduled", "area_uuid", "tags"} {
		if v := req.GetString(key, ""); v != "" {
			opts[key] = v
		}
	}

	if err := t.validateOpts(opts); err != nil {
		return errResult(err.Error()), nil
	}

	projectUUID := generateUUID()
	payload := newTaskCreatePayload(title, opts)
	env := writeEnvelope{id: projectUUID, action: 0, kind: "Task6", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("create project: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": projectUUID, "title": title}), nil
}

func (t *ThingsMCP) handleCreateHeading(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return errResult("title is required"), nil
	}
	projectUUID, err := req.RequireString("project_uuid")
	if err != nil {
		return errResult("project_uuid is required"), nil
	}

	opts := map[string]string{"type": "heading", "project_uuid": projectUUID}

	if err := t.validateOpts(opts); err != nil {
		return errResult(err.Error()), nil
	}

	headingUUID := generateUUID()
	payload := newTaskCreatePayload(title, opts)
	env := writeEnvelope{id: headingUUID, action: 0, kind: "Task6", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("create heading: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": headingUUID, "title": title}), nil
}

func (t *ThingsMCP) handleCreateArea(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return errResult("name is required"), nil
	}

	areaUUID := generateUUID()
	payload := map[string]any{
		"tt": name,
		"ix": 0,
		"tg": []string{},
		"xx": defaultExtension(),
	}
	env := writeEnvelope{id: areaUUID, action: 0, kind: "Area3", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("create area: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": areaUUID, "name": name}), nil
}

func (t *ThingsMCP) handleCreateTag(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return errResult("name is required"), nil
	}

	tagUUID := generateUUID()
	var sh *string
	if v := req.GetString("shorthand", ""); v != "" {
		sh = &v
	}
	pn := []string{}
	if v := req.GetString("parent_uuid", ""); v != "" {
		pn = []string{v}
	}

	payload := TagCreatePayload{Tt: name, Ix: -1237, Sh: sh, Pn: pn, Xx: defaultExtension()}
	env := writeEnvelope{id: tagUUID, action: 0, kind: "Tag4", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("create tag: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": tagUUID, "name": name}), nil
}

func (t *ThingsMCP) handleEditTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	if err := t.validateTaskUUID(taskUUID); err != nil {
		return errResult(err.Error()), nil
	}

	// Validate referenced UUIDs
	editOpts := make(map[string]string)
	for _, key := range []string{"project_uuid", "heading_uuid", "area_uuid", "tags"} {
		if v := req.GetString(key, ""); v != "" {
			editOpts[key] = v
		}
	}
	if err := t.validateOpts(editOpts); err != nil {
		return errResult(err.Error()), nil
	}

	u := newTaskUpdate()
	if v := req.GetString("title", ""); v != "" {
		u.Title(v)
	}
	if v := req.GetString("note", ""); v != "" {
		u.Note(v)
	}
	when := req.GetString("when", "")
	if when != "" {
		switch when {
		case "today":
			today := todayMidnightUTC()
			u.Schedule(1, today, today)
		case "anytime":
			u.Schedule(1, nil, nil)
		case "someday":
			u.Schedule(2, nil, nil)
		case "inbox":
			u.Schedule(0, nil, nil)
		}
	}
	if v := req.GetString("deadline", ""); v != "" {
		if dt := parseDate(v); dt != nil {
			u.Deadline(dt.Unix())
		}
	}
	if v := req.GetString("scheduled", ""); v != "" {
		if dt := parseDate(v); dt != nil {
			ts := dt.Unix()
			u.Scheduled(ts, ts)
			if when == "" {
				u.Schedule(1, ts, ts)
			}
		}
	}
	if v := req.GetString("area_uuid", ""); v != "" {
		u.Area(v)
		if when == "" {
			u.Schedule(1, 0, 0)
		}
	}
	if v := req.GetString("project_uuid", ""); v != "" {
		u.Project(v)
		if when == "" {
			u.Schedule(1, 0, 0)
		}
	}
	if v := req.GetString("heading_uuid", ""); v != "" {
		u.Heading(v)
		if when == "" {
			u.Schedule(1, 0, 0)
		}
	}
	if v := req.GetString("tags", ""); v != "" {
		u.Tags(strings.Split(v, ","))
	}

	env := writeEnvelope{id: taskUUID, action: 1, kind: "Task6", payload: u.build()}
	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("edit task: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "updated", "uuid": taskUUID}), nil
}

func (t *ThingsMCP) handleCompleteTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	if err := t.validateTaskUUID(taskUUID); err != nil {
		return errResult(err.Error()), nil
	}

	ts := nowTs()
	u := newTaskUpdate().Status(3).StopDate(ts)
	env := writeEnvelope{id: taskUUID, action: 1, kind: "Task6", payload: u.build()}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("complete task: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "completed", "uuid": taskUUID}), nil
}

func (t *ThingsMCP) handleTrashTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	if err := t.validateTaskUUID(taskUUID); err != nil {
		return errResult(err.Error()), nil
	}

	u := newTaskUpdate().Trash(true)
	env := writeEnvelope{id: taskUUID, action: 1, kind: "Task6", payload: u.build()}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("trash task: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "trashed", "uuid": taskUUID}), nil
}

func (t *ThingsMCP) handleMoveToToday(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	if err := t.validateTaskUUID(taskUUID); err != nil {
		return errResult(err.Error()), nil
	}

	today := todayMidnightUTC()
	u := newTaskUpdate().Schedule(1, today, today)
	env := writeEnvelope{id: taskUUID, action: 1, kind: "Task6", payload: u.build()}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("move to today: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "moved-to-today", "uuid": taskUUID}), nil
}

// ---------------------------------------------------------------------------
// Batch operations
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// MCP tool definitions
// ---------------------------------------------------------------------------

func defineTools(t *ThingsMCP) []server.ServerTool {
	return []server.ServerTool{
		// --- Read tools ---
		{
			Tool: mcp.NewTool("list_tasks",
				mcp.WithDescription("List tasks with optional filters. Returns JSON array of tasks."),
				mcp.WithString("filter", mcp.Description("Filter: today, inbox, anytime, someday, YYYY-MM-DD, or YYYY-MM-DD..YYYY-MM-DD for date range")),
				mcp.WithString("area", mcp.Description("Filter by area name")),
				mcp.WithString("project", mcp.Description("Filter by project name")),
			),
			Handler: t.handleListTasks,
		},
		{
			Tool: mcp.NewTool("show_task",
				mcp.WithDescription("Show detailed info for a task by UUID (or UUID prefix). Includes checklist items."),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Task UUID or prefix")),
			),
			Handler: t.handleShowTask,
		},
		{
			Tool: mcp.NewTool("show_project",
				mcp.WithDescription("Show project detail: info, headings, and tasks grouped by heading."),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Project UUID")),
			),
			Handler: t.handleShowProject,
		},
		{
			Tool: mcp.NewTool("list_projects",
				mcp.WithDescription("List all active projects. Returns JSON array."),
			),
			Handler: t.handleListProjects,
		},
		{
			Tool: mcp.NewTool("list_headings",
				mcp.WithDescription("List headings (section dividers) in a project."),
				mcp.WithString("project_uuid", mcp.Required(), mcp.Description("Project UUID")),
			),
			Handler: t.handleListHeadings,
		},
		{
			Tool: mcp.NewTool("list_areas",
				mcp.WithDescription("List all areas. Returns JSON array."),
			),
			Handler: t.handleListAreas,
		},
		{
			Tool: mcp.NewTool("list_tags",
				mcp.WithDescription("List all tags. Returns JSON array."),
			),
			Handler: t.handleListTags,
		},

		// --- Create tools ---
		{
			Tool: mcp.NewTool("create_task",
				mcp.WithDescription("Create a new task in Things 3."),
				mcp.WithString("title", mcp.Required(), mcp.Description("Task title")),
				mcp.WithString("note", mcp.Description("Task note/description")),
				mcp.WithString("when", mcp.Description("Schedule: today, anytime, someday, inbox"), mcp.Enum("today", "anytime", "someday", "inbox")),
				mcp.WithString("deadline", mcp.Description("Deadline date (YYYY-MM-DD)")),
				mcp.WithString("scheduled", mcp.Description("Scheduled date (YYYY-MM-DD)")),
				mcp.WithString("project_uuid", mcp.Description("Project UUID to add task to")),
				mcp.WithString("heading_uuid", mcp.Description("Heading UUID to add task under")),
				mcp.WithString("area_uuid", mcp.Description("Area UUID to add task to")),
				mcp.WithString("tags", mcp.Description("Comma-separated tag UUIDs")),
				mcp.WithString("checklist", mcp.Description("Comma-separated checklist items")),
			),
			Handler: t.handleCreateTask,
		},
		{
			Tool: mcp.NewTool("create_heading",
				mcp.WithDescription("Create a heading (section divider) inside a project."),
				mcp.WithString("title", mcp.Required(), mcp.Description("Heading title")),
				mcp.WithString("project_uuid", mcp.Required(), mcp.Description("Project UUID to add heading to")),
			),
			Handler: t.handleCreateHeading,
		},
		{
			Tool: mcp.NewTool("create_project",
				mcp.WithDescription("Create a new project in Things 3."),
				mcp.WithString("title", mcp.Required(), mcp.Description("Project title")),
				mcp.WithString("note", mcp.Description("Project note/description")),
				mcp.WithString("when", mcp.Description("Schedule: today, anytime, someday, inbox"), mcp.Enum("today", "anytime", "someday", "inbox")),
				mcp.WithString("deadline", mcp.Description("Deadline date (YYYY-MM-DD)")),
				mcp.WithString("scheduled", mcp.Description("Scheduled date (YYYY-MM-DD)")),
				mcp.WithString("area_uuid", mcp.Description("Area UUID to add project to")),
				mcp.WithString("tags", mcp.Description("Comma-separated tag UUIDs")),
			),
			Handler: t.handleCreateProject,
		},
		{
			Tool: mcp.NewTool("create_area",
				mcp.WithDescription("Create a new area in Things 3."),
				mcp.WithString("name", mcp.Required(), mcp.Description("Area name")),
			),
			Handler: t.handleCreateArea,
		},
		{
			Tool: mcp.NewTool("create_tag",
				mcp.WithDescription("Create a new tag in Things 3."),
				mcp.WithString("name", mcp.Required(), mcp.Description("Tag name")),
				mcp.WithString("shorthand", mcp.Description("Tag shorthand/abbreviation")),
				mcp.WithString("parent_uuid", mcp.Description("Parent tag UUID for nesting")),
			),
			Handler: t.handleCreateTag,
		},

		// --- Modify tools ---
		{
			Tool: mcp.NewTool("edit_item",
				mcp.WithDescription("Edit an existing task or project. Only provided fields are updated."),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Task or project UUID")),
				mcp.WithString("title", mcp.Description("New title")),
				mcp.WithString("note", mcp.Description("New note")),
				mcp.WithString("when", mcp.Description("Schedule: today, anytime, someday, inbox"), mcp.Enum("today", "anytime", "someday", "inbox")),
				mcp.WithString("deadline", mcp.Description("Deadline date (YYYY-MM-DD)")),
				mcp.WithString("scheduled", mcp.Description("Scheduled date (YYYY-MM-DD)")),
				mcp.WithString("area_uuid", mcp.Description("Area UUID")),
				mcp.WithString("project_uuid", mcp.Description("Project UUID")),
				mcp.WithString("heading_uuid", mcp.Description("Heading UUID")),
				mcp.WithString("tags", mcp.Description("Comma-separated tag UUIDs")),
			),
			Handler: t.handleEditTask,
		},
		{
			Tool: mcp.NewTool("complete_item",
				mcp.WithDescription("Mark a task or project as complete."),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Task or project UUID")),
			),
			Handler: t.handleCompleteTask,
		},
		{
			Tool: mcp.NewTool("trash_item",
				mcp.WithDescription("Move a task, project, or heading to trash."),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Task, project, or heading UUID")),
			),
			Handler: t.handleTrashTask,
		},
		{
			Tool: mcp.NewTool("move_to_today",
				mcp.WithDescription("Move a task or project to Today list."),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Task or project UUID")),
			),
			Handler: t.handleMoveToToday,
		},

	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[things-mcp] ")

	t, err := NewThingsMCP()
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	mcpServer := server.NewMCPServer(
		"things-cloud-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
	)
	mcpServer.AddTools(defineTools(t)...)

	streamServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	// Use a custom mux so we can serve both the landing page and MCP endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.Method == http.MethodGet {
			handleLandingPage(w, r)
			return
		}
		// For any other path at root, 404
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		// This shouldn't be reached since /mcp is handled below, but just in case
		streamServer.ServeHTTP(w, r)
	})
	mux.Handle("/mcp", streamServer)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("Things Cloud MCP server listening on %s", addr)
	log.Printf("  Landing page: http://localhost%s/", addr)
	log.Printf("  MCP endpoint: http://localhost%s/mcp", addr)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
