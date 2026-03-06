package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strconv"
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

// ---------------------------------------------------------------------------
// Recurrence parser: user-friendly string → wire-format JSON
// ---------------------------------------------------------------------------

func parseRecurrence(s string, refDate time.Time) (*json.RawMessage, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "none" {
		return nil, nil
	}

	ref := time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, time.UTC).Unix()
	base := map[string]any{
		"rrv": 4,
		"tp":  0,
		"rc":  0,
		"ts":  0,
		"ed":  64092211200,
		"ia":  ref,
		"sr":  ref,
	}

	switch {
	case s == "daily":
		base["fu"] = 16
		base["fa"] = 1
		base["of"] = []map[string]any{{"dy": 0}}

	case strings.HasPrefix(s, "every ") && strings.HasSuffix(s, " days"):
		numStr := strings.TrimSuffix(strings.TrimPrefix(s, "every "), " days")
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid recurrence: %s", s)
		}
		base["fu"] = 16
		base["fa"] = n
		base["of"] = []map[string]any{{"dy": 0}}

	case s == "weekly":
		base["fu"] = 256
		base["fa"] = 1
		base["of"] = []map[string]any{{"wd": int(refDate.Weekday())}}

	case strings.HasPrefix(s, "weekly:"):
		dayStr := strings.TrimPrefix(s, "weekly:")
		days, err := parseWeekdays(dayStr)
		if err != nil {
			return nil, err
		}
		base["fu"] = 256
		base["fa"] = 1
		base["of"] = days

	case strings.HasPrefix(s, "every ") && strings.HasSuffix(s, " weeks"):
		numStr := strings.TrimSuffix(strings.TrimPrefix(s, "every "), " weeks")
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid recurrence: %s", s)
		}
		base["fu"] = 256
		base["fa"] = n
		base["of"] = []map[string]any{{"wd": int(refDate.Weekday())}}

	case s == "monthly":
		base["fu"] = 8
		base["fa"] = 1
		base["of"] = []map[string]any{{"dy": 0}}

	case strings.HasPrefix(s, "monthly:"):
		detail := strings.TrimPrefix(s, "monthly:")
		if detail == "last" {
			base["fu"] = 8
			base["fa"] = 1
			base["of"] = []map[string]any{{"dy": -1}}
		} else {
			day, err := strconv.Atoi(detail)
			if err != nil || day < 1 || day > 31 {
				return nil, fmt.Errorf("invalid monthly day: %s", detail)
			}
			base["fu"] = 8
			base["fa"] = 1
			base["of"] = []map[string]any{{"dy": day - 1}}
		}

	case s == "yearly":
		base["fu"] = 4
		base["fa"] = 1
		base["of"] = []map[string]any{{"dy": 0, "mo": 0}}

	default:
		return nil, fmt.Errorf("unsupported recurrence format: %s (try: daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days, every N weeks)", s)
	}

	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	msg := json.RawMessage(raw)
	return &msg, nil
}

func parseWeekdays(s string) ([]map[string]any, error) {
	dayMap := map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	}
	parts := strings.Split(s, ",")
	var result []map[string]any
	for _, p := range parts {
		p = strings.TrimSpace(p)
		wd, ok := dayMap[p]
		if !ok {
			return nil, fmt.Errorf("unknown weekday: %s (use: sun,mon,tue,wed,thu,fri,sat)", p)
		}
		result = append(result, map[string]any{"wd": wd})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no weekdays specified")
	}
	return result, nil
}

func parseDate(s string) *time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func parseTime(s string) (int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*3600 + m*60, true
}

func offsetToTime(secs int) string {
	return fmt.Sprintf("%02d:%02d", secs/3600, (secs%3600)/60)
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
			st = 1
		case "heading":
			tp = 2
			st = 1
		}
	}
	sb := 0
	if v, ok := opts["schedule"]; ok {
		switch v {
		case "today":
			st = 1
			today := todayMidnightUTC()
			sr = &today
			tir = &today
		case "tonight":
			st = 1
			today := todayMidnightUTC()
			sr = &today
			tir = &today
			sb = 1
		case "anytime":
			st = 1
		case "someday":
			st = 2
		case "inbox":
			st = 0
		default:
			// Try parsing as YYYY-MM-DD date
			if t := parseDate(v); t != nil {
				ts := t.Unix()
				sr = &ts
				tir = &ts
				st = 2 // Upcoming; Things auto-moves overdue tasks to Today with date shown
			}
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
	if v, ok := opts["project_uuid"]; ok && v != "" {
		pr = []string{v}
		if _, hasSchedule := opts["schedule"]; !hasSchedule {
			st = 1
		}
	}
	if v, ok := opts["heading_uuid"]; ok && v != "" {
		agr = []string{v}
		if _, hasSchedule := opts["schedule"]; !hasSchedule {
			st = 1
		}
	}
	if v, ok := opts["area_uuid"]; ok && v != "" {
		ar = []string{v}
		if _, hasSchedule := opts["schedule"]; !hasSchedule {
			st = 1
		}
	}
	if v, ok := opts["tags"]; ok && v != "" {
		tg = strings.Split(v, ",")
	}

	var rmd *int64
	var ato *int
	if rmdStr, ok := opts["reminder_date"]; ok {
		if timeStr, ok2 := opts["reminder_time"]; ok2 {
			if dt := parseDate(rmdStr); dt != nil {
				if offset, valid := parseTime(timeStr); valid {
					ts := dt.Unix()
					rmd = &ts
					ato = &offset
				}
			}
		}
	}

	var rr *json.RawMessage
	var icsd *int64
	if v, ok := opts["recurrence"]; ok && v != "" {
		// Use schedule date as reference for weekday, fall back to today
		recRef := time.Now()
		if schedStr, ok := opts["schedule"]; ok {
			if dt := parseDate(schedStr); dt != nil {
				recRef = *dt
			}
		}
		parsed, err := parseRecurrence(v, recRef)
		if err == nil && parsed != nil {
			rr = parsed
			today := todayMidnightUTC()
			icsd = &today
		}
	}

	return TaskCreatePayload{
		Tp: tp, Sr: sr, Dds: nil, Rt: []string{}, Rmd: rmd,
		Ss: 0, Tr: false, Dl: []string{}, Icp: false, St: st,
		Ar: ar, Tt: title, Do: 0, Lai: nil, Tir: tir,
		Tg: tg, Agr: agr, Ix: 0, Cd: now, Lt: false,
		Icc: 0, Md: nil, Ti: 0, Dd: dd, Ato: ato,
		Nt: nt, Icsd: icsd, Pr: pr, Rp: nil, Acrd: nil,
		Sp: nil, Sb: sb, Rr: rr, Xx: defaultExtension(),
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
func (u *taskUpdate) Reminder(rmd int64) *taskUpdate    { u.fields["rmd"] = rmd; return u }
func (u *taskUpdate) AlarmOffset(ato int) *taskUpdate   { u.fields["ato"] = ato; return u }
func (u *taskUpdate) ClearReminder() *taskUpdate        { u.fields["rmd"] = nil; u.fields["ato"] = nil; return u }
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
func (u *taskUpdate) StartBucket(sb int) *taskUpdate { u.fields["sb"] = sb; return u }
func (u *taskUpdate) Recurrence(rr json.RawMessage) *taskUpdate {
	u.fields["rr"] = rr
	return u
}
func (u *taskUpdate) ClearRecurrence() *taskUpdate {
	u.fields["rr"] = nil
	u.fields["icsd"] = nil
	return u
}
func (u *taskUpdate) InstanceCreationStartDate(icsd int64) *taskUpdate {
	u.fields["icsd"] = icsd
	return u
}
func (u *taskUpdate) build() map[string]any { return u.fields }

// ---------------------------------------------------------------------------
// JSON output types
// ---------------------------------------------------------------------------

type Ref struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type TaskOutput struct {
	UUID             string  `json:"uuid"`
	Title            string  `json:"title"`
	Note             string  `json:"note,omitempty"`
	Status           string  `json:"status"`
	Schedule         string  `json:"schedule"`
	ScheduledDate    *string `json:"scheduledDate,omitempty"`
	DeadlineDate     *string `json:"deadlineDate,omitempty"`
	ReminderTime     *string `json:"reminderTime,omitempty"`
	Recurrence       *string `json:"recurrence,omitempty"`
	CreationDate     *string `json:"creationDate,omitempty"`
	ModificationDate *string `json:"modificationDate,omitempty"`
	CompletionDate   *string `json:"completionDate,omitempty"`
	Areas            []Ref   `json:"areas,omitempty"`
	Project          *Ref    `json:"project,omitempty"`
	Tags             []Ref   `json:"tags,omitempty"`
}

type ChecklistOutput struct {
	UUID   string `json:"uuid"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type TaskDetailOutput struct {
	TaskOutput
	Checklist []ChecklistOutput `json:"checklist,omitempty"`
}

func statusString(s thingscloud.TaskStatus) string {
	switch s {
	case 3:
		return "completed"
	case 2:
		return "canceled"
	default:
		return "pending"
	}
}

func scheduleString(st thingscloud.TaskSchedule, scheduledDate *time.Time, startBucket int) string {
	switch st {
	case 0:
		return "inbox"
	case 1:
		if scheduledDate != nil && isToday(*scheduledDate) {
			if startBucket == 1 {
				return "tonight"
			}
			return "today"
		}
		return "anytime"
	case 2:
		if scheduledDate != nil {
			return "upcoming"
		}
		return "someday"
	default:
		return "inbox"
	}
}

// isToday returns true if t falls on today's date (UTC).
func isToday(t time.Time) bool {
	now := time.Now().UTC()
	return t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day()
}

// isScheduledForTodayOrPast returns true if the task should appear in
// the Today filter: either scheduled for today or overdue (past date).
func isScheduledForTodayOrPast(st thingscloud.TaskSchedule, scheduledDate *time.Time) bool {
	if scheduledDate == nil {
		return false
	}
	if st != 1 && st != 2 {
		return false
	}
	now := time.Now().UTC()
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	return !scheduledDate.After(todayEnd)
}

func (t *ThingsMCP) taskToOutput(task *thingscloud.Task) TaskOutput {
	state := t.getState()
	out := TaskOutput{
		UUID:     task.UUID,
		Title:    task.Title,
		Note:     task.Note,
		Status:   statusString(task.Status),
		Schedule: scheduleString(task.Schedule, task.ScheduledDate, task.StartBucket),
	}
	if task.ScheduledDate != nil && task.ScheduledDate.Year() > 1970 {
		s := task.ScheduledDate.Format("2006-01-02")
		out.ScheduledDate = &s
	}
	if task.DeadlineDate != nil && task.DeadlineDate.Year() > 1970 {
		s := task.DeadlineDate.Format("2006-01-02")
		out.DeadlineDate = &s
	}
	if task.AlarmTimeOffset != nil {
		s := offsetToTime(*task.AlarmTimeOffset)
		out.ReminderTime = &s
	}
	if len(task.RecurrenceIDs) > 0 {
		s := "recurring"
		out.Recurrence = &s
	}
	const isoFormat = "2006-01-02T15:04:05Z"
	if !task.CreationDate.IsZero() && task.CreationDate.Year() > 1970 {
		s := task.CreationDate.UTC().Format(isoFormat)
		out.CreationDate = &s
	}
	if task.ModificationDate != nil && task.ModificationDate.Year() > 1970 {
		s := task.ModificationDate.UTC().Format(isoFormat)
		out.ModificationDate = &s
	}
	if task.CompletionDate != nil && task.CompletionDate.Year() > 1970 {
		s := task.CompletionDate.UTC().Format(isoFormat)
		out.CompletionDate = &s
	}
	for _, areaID := range task.AreaIDs {
		if area, ok := state.Areas[areaID]; ok {
			out.Areas = append(out.Areas, Ref{UUID: areaID, Name: area.Title})
		}
	}
	if len(task.ParentTaskIDs) > 0 {
		pid := task.ParentTaskIDs[0]
		if parent, ok := state.Tasks[pid]; ok {
			out.Project = &Ref{UUID: pid, Name: parent.Title}
		}
	}
	for _, tagID := range task.TagIDs {
		if tag, ok := state.Tags[tagID]; ok {
			out.Tags = append(out.Tags, Ref{UUID: tagID, Name: tag.Title})
		}
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

// bestHistory fetches all history keys for the account and returns the one
// with the highest LatestServerIndex (i.e. the most recently active sync
// stream). For accounts with multiple devices over many years, OwnHistory()
// may return a stale/legacy history; this function picks the current one.
func bestHistory(c *thingscloud.Client) (*thingscloud.History, error) {
	histories, err := c.Histories()
	if err != nil || len(histories) == 0 {
		log.Printf("Histories() unavailable, falling back to OwnHistory()")
		return c.OwnHistory()
	}
	if len(histories) == 1 {
		full, err := c.History(histories[0].ID)
		if err != nil {
			log.Printf("Single history metadata fetch failed, using stub: %v", err)
			return histories[0], nil
		}
		log.Printf("Single history found: %s (serverIndex=%d)", full.ID, full.LatestServerIndex)
		return full, nil
	}
	// Multiple histories — pick the one with the highest server index.
	var best *thingscloud.History
	bestIdx := -1
	for _, h := range histories {
		full, err := c.History(h.ID)
		if err != nil {
			log.Printf("Skipping history %s: %v", h.ID, err)
			continue
		}
		log.Printf("History %s: LatestServerIndex=%d", full.ID, full.LatestServerIndex)
		if full.LatestServerIndex > bestIdx {
			best = full
			bestIdx = full.LatestServerIndex
		}
	}
	if best == nil {
		log.Printf("No valid histories found, falling back to OwnHistory()")
		return c.OwnHistory()
	}
	log.Printf("Selected best history: %s (index=%d, out of %d histories)", best.ID, best.LatestServerIndex, len(histories))
	return best, nil
}

// NewThingsMCPForUser creates a ThingsMCP instance for a specific user.
func NewThingsMCPForUser(email, password string) (*ThingsMCP, error) {
	c := thingscloud.New(thingscloud.APIEndpoint, email, password)
	if os.Getenv("THINGS_DEBUG") != "" {
		c.Debug = true
	}

	log.Printf("Verifying Things Cloud credentials for %s...", email)
	if _, err := c.Verify(); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	log.Printf("Credentials verified for %s.", email)

	log.Printf("Fetching history for %s...", email)
	history, err := bestHistory(c)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	if err := history.Sync(); err != nil {
		return nil, fmt.Errorf("sync history: %w", err)
	}
	log.Printf("History synced for %s (id=%s, serverIndex=%d).", email, history.ID, history.LatestServerIndex)

	t := &ThingsMCP{client: c, history: history}
	if err := t.fullRebuild(); err != nil {
		return nil, err
	}

	return t, nil
}

// ---------------------------------------------------------------------------
// Multi-user management
// ---------------------------------------------------------------------------

type contextKey string

const userContextKey contextKey = "things_user"
const baseURLContextKey contextKey = "base_url"

func getBaseURLFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(baseURLContextKey).(string); ok {
		return v
	}
	return ""
}

type UserInfo struct {
	Email    string
	Password string
	Token    string // raw Bearer token (JWT validation added by OAuth task)
}

type UserManager struct {
	users     map[string]*ThingsMCP // keyed by email
	oauth     *OAuthServer          // set after OAuthServer is created
	diagStore *DiagStore            // set after OAuthServer is created
	mu        sync.RWMutex
}

func NewUserManager() *UserManager {
	return &UserManager{users: make(map[string]*ThingsMCP)}
}

func (um *UserManager) GetOrCreateUser(email, password string) (*ThingsMCP, error) {
	um.mu.RLock()
	if t, ok := um.users[email]; ok {
		um.mu.RUnlock()
		return t, nil
	}
	um.mu.RUnlock()

	// Create new user instance (outside lock to avoid blocking other users)
	t, err := NewThingsMCPForUser(email, password)
	if err != nil {
		return nil, err
	}

	um.mu.Lock()
	// Double-check after acquiring write lock
	if existing, ok := um.users[email]; ok {
		um.mu.Unlock()
		return existing, nil
	}
	um.users[email] = t
	um.mu.Unlock()

	return t, nil
}

// httpContextFunc extracts user identity from the HTTP request and stores it in context.
func (um *UserManager) httpContextFunc(ctx context.Context, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, baseURLContextKey, getBaseURL(r))

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ctx
	}

	if strings.HasPrefix(authHeader, "Bearer ") {
		// Store raw token for JWT validation (implemented by OAuth task)
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return context.WithValue(ctx, userContextKey, &UserInfo{Token: token})
	}

	if strings.HasPrefix(authHeader, "Basic ") {
		encoded := strings.TrimPrefix(authHeader, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return ctx
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return ctx
		}
		return context.WithValue(ctx, userContextKey, &UserInfo{
			Email:    parts[0],
			Password: parts[1],
		})
	}

	return ctx
}

// getUserFromContext extracts the user's ThingsMCP instance from the request context.
func getUserFromContext(ctx context.Context, um *UserManager) (*ThingsMCP, error) {
	val := ctx.Value(userContextKey)
	if val == nil {
		return nil, fmt.Errorf("authentication required: provide Authorization header (Basic or Bearer)")
	}
	info, ok := val.(*UserInfo)
	if !ok {
		return nil, fmt.Errorf("invalid user context")
	}

	// Bearer token path — resolve JWT via OAuthServer
	if info.Token != "" {
		if um.oauth == nil {
			return nil, fmt.Errorf("Bearer token authentication not configured")
		}
		email, password, err := um.oauth.ResolveBearer(info.Token)
		if err != nil {
			return nil, fmt.Errorf("Bearer auth failed: %w", err)
		}
		return um.GetOrCreateUser(email, password)
	}

	if info.Email == "" || info.Password == "" {
		return nil, fmt.Errorf("invalid credentials")
	}

	return um.GetOrCreateUser(info.Email, info.Password)
}

// fullRebuild fetches ALL items from index 0 and creates a fresh state.
// Used for initial sync and as fallback when incremental sync fails.
func (t *ThingsMCP) fullRebuild() error {
	t.history.LoadedServerIndex = 0
	startIndex := 0
	var allItems []thingscloud.Item
	for {
		items, hasMore, err := t.history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			return fmt.Errorf("fetch items: %w", err)
		}
		if len(items) == 0 {
			break
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

	log.Printf("Full rebuild: %d tasks, %d areas, %d tags",
		len(state.Tasks), len(state.Areas), len(state.Tags))
	return nil
}

// incrementalSync fetches only commits newer than LoadedServerIndex
// and applies them to the existing state.
func (t *ThingsMCP) incrementalSync() error {
	startIndex := t.history.LoadedServerIndex
	var delta []thingscloud.Item
	for {
		items, hasMore, err := t.history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			log.Printf("Incremental fetch failed at index %d, falling back to full rebuild: %v", startIndex, err)
			return t.fullRebuild()
		}
		if len(items) == 0 {
			break
		}
		delta = append(delta, items...)
		if !hasMore {
			break
		}
		startIndex = t.history.LoadedServerIndex
	}

	if len(delta) == 0 {
		return nil
	}

	t.mu.Lock()
	t.state.Update(delta...)
	t.mu.Unlock()

	log.Printf("Incremental sync: applied %d new items", len(delta))
	return nil
}

func (t *ThingsMCP) getState() *memory.State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// syncAndRebuild checks for new history commits and updates state.
// Called from wrap() which serializes all MCP tool calls per-user,
// so t.state and t.history fields are safe to read without a lock here.
func (t *ThingsMCP) syncAndRebuild() error {
	if err := t.history.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	// First time (no state built yet) → full rebuild
	if t.state == nil {
		return t.fullRebuild()
	}

	// Already up to date → skip
	if t.history.LoadedServerIndex >= t.history.LatestServerIndex {
		return nil
	}

	// Delta available → incremental sync
	return t.incrementalSync()
}

func (t *ThingsMCP) writeAndSync(items ...thingscloud.Identifiable) error {
	if err := t.history.Sync(); err != nil {
		return fmt.Errorf("pre-write sync: %w", err)
	}
	if err := t.history.Write(items...); err != nil {
		return err
	}
	return t.fullRebuild()
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

func (t *ThingsMCP) findTagUUID(name string) string {
	state := t.getState()
	for _, tag := range state.Tags {
		if strings.EqualFold(tag.Title, name) {
			return tag.UUID
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

func (t *ThingsMCP) validateTagUUID(uuid string) error {
	state := t.getState()
	for _, tag := range state.Tags {
		if tag.UUID == uuid {
			return nil
		}
	}
	return fmt.Errorf("tag not found: %s", uuid)
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
// Diagnostic types
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
	TotalSteps      int   `json:"totalSteps"`
	Passed          int   `json:"passed"`
	Warnings        int   `json:"warnings"`
	Failed          int   `json:"failed"`
	Skipped         int   `json:"skipped"`
	TotalDurationMs int64 `json:"totalDurationMs"`
}

type diagReport struct {
	Steps    []diagStep  `json:"steps"`
	Summary  diagSummary `json:"summary"`
	Warnings []string    `json:"warnings"`
	Errors   []string    `json:"errors"`
}

// DiagStore handles persistence of shareable diagnosis reports.
type DiagStore struct {
	db *sql.DB
}

func (ds *DiagStore) Store(email string, report *diagReport) (string, error) {
	token := uuid.New().String()
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = ds.db.Exec(
		`INSERT INTO diagnoses (token, report_json, email, created_at) VALUES (?, ?, ?, ?)`,
		token, string(reportJSON), maskEmail(email), now,
	)
	if err != nil {
		return "", fmt.Errorf("store report: %w", err)
	}
	// Lazy cleanup: delete reports older than 7 days
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	ds.db.Exec(`DELETE FROM diagnoses WHERE created_at < ?`, cutoff)
	return token, nil
}

func (ds *DiagStore) Load(token string) (reportJSON string, email string, createdAt time.Time, err error) {
	var createdAtStr string
	err = ds.db.QueryRow(
		`SELECT report_json, email, created_at FROM diagnoses WHERE token = ?`, token,
	).Scan(&reportJSON, &email, &createdAtStr)
	if err != nil {
		return "", "", time.Time{}, err
	}
	createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if time.Since(createdAt) > 7*24*time.Hour {
		return "", "", time.Time{}, fmt.Errorf("report expired")
	}
	return reportJSON, email, createdAt, nil
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || len(parts[0]) == 0 {
		return "***"
	}
	return string(parts[0][0]) + "***@" + parts[1]
}

var diagStepDefs = []struct{ num int; name, desc string }{
	{1, "credential_verification", "Verify Things Cloud credentials and account status"},
	{2, "fetch_history", "Fetch all account histories and select best one"},
	{3, "sync_history", "Sync selected history to get latest server index"},
	{4, "paginated_fetch", "Fetch all items via paginated API"},
	{5, "rebuild_state", "Rebuild in-memory state from items"},
	{6, "data_integrity", "Check data completeness and integrity"},
	{7, "query_tests", "Test basic list/query operations"},
}

func addSkippedSteps(report *diagReport, fromStep int) {
	for _, sd := range diagStepDefs {
		if sd.num < fromStep {
			continue
		}
		report.Steps = append(report.Steps, diagStep{
			Step: sd.num, Name: sd.name, Description: sd.desc,
			Status: "skipped",
			Details: map[string]any{"reason": "previous step failed"},
		})
	}
}

func extractCredentials(ctx context.Context, um *UserManager) (string, string, error) {
	val := ctx.Value(userContextKey)
	if val == nil {
		return "", "", fmt.Errorf("authentication required: provide Authorization header (Basic or Bearer)")
	}
	info, ok := val.(*UserInfo)
	if !ok {
		return "", "", fmt.Errorf("invalid user context")
	}
	if info.Token != "" {
		if um.oauth == nil {
			return "", "", fmt.Errorf("Bearer token authentication not configured")
		}
		email, password, err := um.oauth.ResolveBearer(info.Token)
		if err != nil {
			return "", "", fmt.Errorf("Bearer auth failed: %w", err)
		}
		return email, password, nil
	}
	if info.Email == "" || info.Password == "" {
		return "", "", fmt.Errorf("invalid credentials")
	}
	return info.Email, info.Password, nil
}

// ---------------------------------------------------------------------------
// Diagnostic handler: steps 1-3
// ---------------------------------------------------------------------------

func (t *ThingsMCP) handleDiagnose(email, password string) *diagReport {
	report := &diagReport{}
	var allWarnings []string
	var allErrors []string

	// Step 1: credential_verification
	step1 := diagStep{
		Step:        1,
		Name:        "credential_verification",
		Description: "Verify Things Cloud credentials and account status",
		Log:         []string{},
	}
	step1.Log = append(step1.Log, fmt.Sprintf("Verifying credentials for %s", maskEmail(email)))

	start := time.Now()
	client := thingscloud.New(thingscloud.APIEndpoint, email, password)
	verifyResp, err := client.Verify()
	step1.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		step1.Status = "fail"
		step1.Details = map[string]any{"error": err.Error()}
		step1.Log = append(step1.Log, fmt.Sprintf("Credential verification failed: %v", err))
		allErrors = append(allErrors, fmt.Sprintf("Step 1: credential verification failed: %v", err))
		report.Steps = append(report.Steps, step1)

		addSkippedSteps(report, 2)
		report.Warnings = allWarnings
		report.Errors = allErrors
		report.Summary = buildDiagSummary(report.Steps)
		return report
	}

	step1.Status = "pass"
	step1.Details = map[string]any{
		"accountStatus": string(verifyResp.Status),
		"historyKey":    verifyResp.HistoryKey,
		"email":         maskEmail(verifyResp.Email),
	}
	step1.Log = append(step1.Log, fmt.Sprintf("Account status: %s", verifyResp.Status))
	step1.Log = append(step1.Log, fmt.Sprintf("History key from /verify: %s", verifyResp.HistoryKey))
	report.Steps = append(report.Steps, step1)

	// Step 2: fetch_history — select best history via bestHistory(), then
	// enumerate all histories for the diagnostic report.
	step2 := diagStep{
		Step:        2,
		Name:        "fetch_history",
		Description: "Fetch all account histories and select best one",
		Log:         []string{},
	}

	start = time.Now()
	ownHistoryID := verifyResp.HistoryKey // from step 1, no extra API call

	history, err := bestHistory(client)
	if err != nil {
		step2.DurationMs = time.Since(start).Milliseconds()
		step2.Status = "fail"
		step2.Details = map[string]any{"error": err.Error()}
		step2.Log = append(step2.Log, fmt.Sprintf("bestHistory failed: %v", err))
		allErrors = append(allErrors, fmt.Sprintf("Step 2: bestHistory failed: %v", err))
		report.Steps = append(report.Steps, step2)
		addSkippedSteps(report, 3)
		report.Warnings = allWarnings
		report.Errors = allErrors
		report.Summary = buildDiagSummary(report.Steps)
		return report
	}

	// Enumerate all histories for the diagnostic report.
	type historyInfo struct {
		ID                string `json:"id"`
		LatestServerIndex int    `json:"latestServerIndex"`
		IsOwnHistory      bool   `json:"isOwnHistory"`
		Selected          bool   `json:"selected"`
		NewestItemDate    string `json:"newestItemDate,omitempty"`
	}
	var allHistories []historyInfo
	histories, historiesErr := client.Histories()
	if historiesErr != nil {
		w := fmt.Sprintf("Histories() returned error: %v", historiesErr)
		step2.Log = append(step2.Log, fmt.Sprintf("Warning: %s", w))
		allWarnings = append(allWarnings, fmt.Sprintf("Step 2: %s", w))
	}
	if historiesErr == nil && len(histories) == 0 {
		w := "own-history-keys returned 0 keys; falling back to account history key from /verify"
		step2.Log = append(step2.Log, fmt.Sprintf("Warning: %s", w))
		allWarnings = append(allWarnings, fmt.Sprintf("Step 2: %s", w))
	}
	ownKeyFound := false
	for _, h := range histories {
		full, ferr := client.History(h.ID)
		if ferr != nil {
			step2.Log = append(step2.Log, fmt.Sprintf("History %s: failed to fetch metadata: %v", h.ID, ferr))
			continue
		}
		hi := historyInfo{
			ID:                full.ID,
			LatestServerIndex: full.LatestServerIndex,
			IsOwnHistory:      full.ID == ownHistoryID,
			Selected:          full.ID == history.ID,
		}
		if hi.IsOwnHistory {
			ownKeyFound = true
		}
		// Peek at recent items to find newest creation date for this history key
		if full.LatestServerIndex > 0 {
			peekStart := full.LatestServerIndex - 5
			if peekStart < 0 {
				peekStart = 0
			}
			items, _, ierr := full.Items(thingscloud.ItemsOptions{StartIndex: peekStart})
			if ierr == nil {
				var newestDate time.Time
				for _, item := range items {
					if item.Kind != thingscloud.ItemKindTask {
						continue
					}
					var payload thingscloud.TaskActionItemPayload
					if uerr := json.Unmarshal(item.P, &payload); uerr != nil {
						continue
					}
					if payload.CreationDate != nil {
						cd := payload.CreationDate.Time()
						if cd.After(newestDate) {
							newestDate = *cd
						}
					}
				}
				if !newestDate.IsZero() {
					hi.NewestItemDate = newestDate.Format("2006-01-02")
				}
			} else {
				step2.Log = append(step2.Log, fmt.Sprintf("History %s: failed to peek items: %v", full.ID, ierr))
			}
		}
		allHistories = append(allHistories, hi)
		logLine := fmt.Sprintf("History %s: serverIndex=%d, isOwnHistory=%v, selected=%v", full.ID, hi.LatestServerIndex, hi.IsOwnHistory, hi.Selected)
		if hi.NewestItemDate != "" {
			logLine += fmt.Sprintf(", newestItemDate=%s", hi.NewestItemDate)
		}
		step2.Log = append(step2.Log, logLine)
	}

	// Check if account history key appears in own-history-keys list
	if len(histories) > 0 && !ownKeyFound {
		w := fmt.Sprintf("Account history key (%s) not found in own-history-keys list", ownHistoryID)
		step2.Log = append(step2.Log, fmt.Sprintf("Warning: %s", w))
		allWarnings = append(allWarnings, fmt.Sprintf("Step 2: %s", w))
	}

	// If Histories() failed, still try to fetch metadata for the selected history
	var selectedServerIndex int
	if len(histories) == 0 {
		meta, metaErr := client.History(history.ID)
		if metaErr == nil {
			selectedServerIndex = meta.LatestServerIndex
			step2.Log = append(step2.Log, fmt.Sprintf("Selected history metadata: serverIndex=%d", meta.LatestServerIndex))
		} else {
			step2.Log = append(step2.Log, fmt.Sprintf("Selected history metadata fetch failed: %v", metaErr))
		}
	} else {
		selectedServerIndex = history.LatestServerIndex
	}

	step2.DurationMs = time.Since(start).Milliseconds()
	step2.Status = "pass"
	step2.Details = map[string]any{
		"historyCount":            len(histories),
		"selectedHistory":         history.ID,
		"selectedServerIndexMeta": selectedServerIndex,
		"ownHistoryKey":           ownHistoryID,
		"selectedIsSameAsOwn":     history.ID == ownHistoryID,
		"allHistories":            allHistories,
	}
	if history.ID != ownHistoryID && len(histories) > 1 {
		step2.Log = append(step2.Log, fmt.Sprintf("WARNING: Selected history %s differs from OwnHistory %s — account may have multiple sync streams", history.ID, ownHistoryID))
		allWarnings = append(allWarnings, "Selected history differs from OwnHistory — multi-device account detected")
	}
	step2.Log = append(step2.Log, fmt.Sprintf("Selected history: %s (serverIndex=%d)", history.ID, history.LatestServerIndex))
	report.Steps = append(report.Steps, step2)

	// Step 3: sync_history
	step3 := diagStep{
		Step:        3,
		Name:        "sync_history",
		Description: "Sync history to get latest server index",
		Log:         []string{},
	}

	preSyncIndex := history.LatestServerIndex
	start = time.Now()
	err = history.Sync()
	step3.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		step3.Status = "fail"
		step3.Details = map[string]any{"error": err.Error()}
		step3.Log = append(step3.Log, fmt.Sprintf("History sync failed: %v", err))
		allErrors = append(allErrors, fmt.Sprintf("Step 3: history sync failed: %v", err))
		report.Steps = append(report.Steps, step3)

		addSkippedSteps(report, 4)
		report.Warnings = allWarnings
		report.Errors = allErrors
		report.Summary = buildDiagSummary(report.Steps)
		return report
	}

	step3.Status = "pass"
	step3.Details = map[string]any{
		"preSyncServerIndex":  preSyncIndex,
		"postSyncServerIndex": history.LatestServerIndex,
	}
	step3.Log = append(step3.Log, fmt.Sprintf("Server index: %d → %d (after sync)", preSyncIndex, history.LatestServerIndex))
	report.Steps = append(report.Steps, step3)

	// Steps 4-7: delegated
	t.diagnoseSteps4to7(history, report, &allWarnings, &allErrors)

	report.Warnings = allWarnings
	report.Errors = allErrors
	report.Summary = buildDiagSummary(report.Steps)
	return report
}

func (t *ThingsMCP) diagnoseSteps4to7(history *thingscloud.History, report *diagReport, warnings *[]string, errors *[]string) {
	// Step 4: paginated_fetch
	step4 := diagStep{
		Step:        4,
		Name:        "paginated_fetch",
		Description: "Fetch all items via paginated API",
		Log:         []string{},
	}

	type pageInfo struct {
		Page             int `json:"page"`
		StartIndex       int `json:"startIndex"`
		ItemsFetched     int `json:"itemsFetched"`
		ServerIndexAfter int `json:"serverIndexAfter"`
	}

	var allItems []thingscloud.Item
	var pages []pageInfo
	startIndex := 0
	pageNum := 0
	step4Failed := false

	start := time.Now()
	for {
		pageNum++
		items, _, err := history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		serverIndexAfter := history.LatestServerIndex
		if err != nil {
			step4.DurationMs = time.Since(start).Milliseconds()
			step4.Status = "fail"
			step4.Details = map[string]any{
				"error":             err.Error(),
				"totalItemsFetched": len(allItems),
				"paginationPages":   pageNum,
				"pages":             pages,
				"finalServerIndex":  serverIndexAfter,
			}
			step4.Log = append(step4.Log, fmt.Sprintf("Page %d failed at startIndex %d: %v", pageNum, startIndex, err))
			*errors = append(*errors, fmt.Sprintf("Step 4: paginated fetch failed on page %d: %v", pageNum, err))
			report.Steps = append(report.Steps, step4)
			step4Failed = true
			break
		}
		if len(items) == 0 {
			break
		}
		allItems = append(allItems, items...)
		pi := pageInfo{
			Page:             pageNum,
			StartIndex:       startIndex,
			ItemsFetched:     len(items),
			ServerIndexAfter: serverIndexAfter,
		}
		pages = append(pages, pi)
		step4.Log = append(step4.Log, fmt.Sprintf("Page %d: startIndex=%d, fetched=%d, serverIndex=%d", pageNum, startIndex, len(items), serverIndexAfter))
		startIndex = serverIndexAfter
	}

	if !step4Failed {
		step4.DurationMs = time.Since(start).Milliseconds()
		step4.Status = "pass"
		finalServerIndex := 0
		if len(pages) > 0 {
			finalServerIndex = pages[len(pages)-1].ServerIndexAfter
		}
		step4.Details = map[string]any{
			"totalItemsFetched": len(allItems),
			"paginationPages":   len(pages),
			"pages":             pages,
			"finalServerIndex":  finalServerIndex,
		}
		step4.Log = append(step4.Log, fmt.Sprintf("Total items fetched: %d across %d pages", len(allItems), len(pages)))

		// Report item type distribution and tail items
		kindCounts := map[string]int{}
		for _, item := range allItems {
			kindCounts[string(item.Kind)]++
		}
		step4.Log = append(step4.Log, fmt.Sprintf("Item types: %v", kindCounts))
		step4.Details.(map[string]any)["itemTypes"] = kindCounts

		// Report last 5 items (the tail of the stream)
		type tailItem struct {
			Kind         string `json:"kind"`
			Action       int    `json:"action"`
			CreationDate string `json:"creationDate,omitempty"`
			Title        string `json:"title,omitempty"`
		}
		var tail []tailItem
		tailStart := len(allItems) - 5
		if tailStart < 0 {
			tailStart = 0
		}
		for _, item := range allItems[tailStart:] {
			ti := tailItem{Kind: string(item.Kind), Action: int(item.Action)}
			if item.Kind == thingscloud.ItemKindTask {
				var payload thingscloud.TaskActionItemPayload
				if uerr := json.Unmarshal(item.P, &payload); uerr == nil {
					if payload.CreationDate != nil {
						cd := payload.CreationDate.Time()
						ti.CreationDate = cd.Format("2006-01-02T15:04:05Z")
					}
					if payload.Title != nil {
						t := *payload.Title
						if len(t) > 30 {
							t = t[:30] + "..."
						}
						ti.Title = t
					}
				}
			}
			tail = append(tail, ti)
		}
		step4.Details.(map[string]any)["tailItems"] = tail
		for _, ti := range tail {
			desc := ti.Kind
			if ti.CreationDate != "" {
				desc += " created=" + ti.CreationDate
			}
			if ti.Title != "" {
				desc += " title=" + ti.Title
			}
			step4.Log = append(step4.Log, fmt.Sprintf("Tail item: %s (action=%d)", desc, ti.Action))
		}

		report.Steps = append(report.Steps, step4)
	}

	if step4Failed {
		addSkippedSteps(report, 5)
		return
	}

	// Step 5: rebuild_state
	step5 := diagStep{
		Step:        5,
		Name:        "rebuild_state",
		Description: "Rebuild in-memory state from items",
		Log:         []string{},
	}

	start = time.Now()
	state := memory.NewState()
	err := state.Update(allItems...)
	step5.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		step5.Status = "fail"
		step5.Details = map[string]any{"error": err.Error()}
		step5.Log = append(step5.Log, fmt.Sprintf("State rebuild failed: %v", err))
		*errors = append(*errors, fmt.Sprintf("Step 5: state rebuild failed: %v", err))
		report.Steps = append(report.Steps, step5)
		addSkippedSteps(report, 6)
		return
	}

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

	// Steps 6-7: delegated
	t.diagnoseDataIntegrity(state, report, warnings)
	t.diagnoseQueryTests(report, warnings, errors)
}

func (t *ThingsMCP) diagnoseDataIntegrity(state *memory.State, report *diagReport, allWarnings *[]string) {
	step := diagStep{
		Step:        6,
		Name:        "data_integrity",
		Description: "Check data integrity and referential consistency",
		Log:         []string{},
	}

	start := time.Now()

	// Count tasks by status (only TaskTypeTask, not projects/headings)
	var active, completed, canceled, trashed int
	yearCounts := map[int]int{}
	var oldest, newest time.Time
	first := true

	for _, task := range state.Tasks {
		if task.Type != thingscloud.TaskTypeTask {
			continue
		}
		if task.InTrash {
			trashed++
		} else {
			switch task.Status {
			case thingscloud.TaskStatusCompleted:
				completed++
			case thingscloud.TaskStatusCanceled:
				canceled++
			default:
				active++
			}
		}

		if task.CreationDate.IsZero() || task.CreationDate.Year() < 2000 {
			continue
		}

		y := task.CreationDate.Year()
		yearCounts[y]++

		if first || task.CreationDate.Before(oldest) {
			oldest = task.CreationDate
		}
		if first || task.CreationDate.After(newest) {
			newest = task.CreationDate
		}
		first = false
	}

	// Build sorted year distribution
	years := make([]int, 0, len(yearCounts))
	for y := range yearCounts {
		years = append(years, y)
	}
	sort.Ints(years)

	var yearParts []string
	for _, y := range years {
		yearParts = append(yearParts, fmt.Sprintf("%d=%d", y, yearCounts[y]))
	}
	yearDist := strings.Join(yearParts, ", ")

	step.Log = append(step.Log, fmt.Sprintf("Task status counts — active=%d, completed=%d, canceled=%d, trashed=%d", active, completed, canceled, trashed))
	if !first {
		step.Log = append(step.Log, fmt.Sprintf("Oldest task: %s", oldest.Format("2006-01-02")))
		step.Log = append(step.Log, fmt.Sprintf("Newest task: %s", newest.Format("2006-01-02")))
	}
	step.Log = append(step.Log, fmt.Sprintf("Year distribution: %s", yearDist))

	// Detect anomalies
	var stepWarnings []string
	var daysSinceNewest int

	if !first {
		daysSinceNewest = int(time.Since(newest).Hours() / 24)
		step.Log = append(step.Log, fmt.Sprintf("Days since newest task: %d", daysSinceNewest))
		if daysSinceNewest > 3 {
			w := fmt.Sprintf("Newest task created %d days ago (%s)",
				daysSinceNewest, newest.Format("2006-01-02"))
			stepWarnings = append(stepWarnings, w)
			step.Log = append(step.Log, fmt.Sprintf("Warning: %s", w))
		}
	}

	if len(years) >= 2 {
		for i := 1; i < len(years); i++ {
			if years[i]-years[i-1] > 1 {
				gapStart := years[i-1] + 1
				gapEnd := years[i] - 1
				var w string
				if gapStart == gapEnd {
					w = fmt.Sprintf("Gap detected: no tasks in year %d", gapStart)
				} else {
					w = fmt.Sprintf("Gap detected: no tasks in years %d–%d", gapStart, gapEnd)
				}
				stepWarnings = append(stepWarnings, w)
				step.Log = append(step.Log, fmt.Sprintf("Warning: %s", w))
			}
		}
	}

	step.DurationMs = time.Since(start).Milliseconds()

	details := map[string]any{
		"active":           active,
		"completed":        completed,
		"canceled":         canceled,
		"trashed":          trashed,
		"yearDistribution": yearCounts,
	}
	if !first {
		details["oldestTask"] = oldest.Format("2006-01-02")
		details["newestTask"] = newest.Format("2006-01-02")
		details["daysSinceNewest"] = daysSinceNewest
	}
	if len(stepWarnings) > 0 {
		details["warnings"] = stepWarnings
	}
	step.Details = details

	if len(stepWarnings) > 0 {
		step.Status = "warn"
		for _, w := range stepWarnings {
			*allWarnings = append(*allWarnings, fmt.Sprintf("Step 6: %s", w))
		}
	} else {
		step.Status = "pass"
	}

	report.Steps = append(report.Steps, step)
}

func (t *ThingsMCP) diagnoseQueryTests(report *diagReport, allWarnings *[]string, allErrors *[]string) {
	type queryResult struct {
		Name  string `json:"name"`
		OK    bool   `json:"ok"`
		Count int    `json:"count"`
		Error string `json:"error,omitempty"`
	}

	step := diagStep{
		Step:        7,
		Name:        "query_tests",
		Description: "Run sample queries against rebuilt state",
		Log:         []string{},
	}

	start := time.Now()

	// Test sync
	syncErr := t.syncAndRebuild()
	var results []queryResult

	if syncErr != nil {
		step.Log = append(step.Log, fmt.Sprintf("syncAndRebuild failed: %v", syncErr))
		results = append(results, queryResult{
			Name:  "syncAndRebuild",
			OK:    false,
			Count: 0,
			Error: syncErr.Error(),
		})
	} else {
		step.Log = append(step.Log, "syncAndRebuild succeeded")
		results = append(results, queryResult{
			Name:  "syncAndRebuild",
			OK:    true,
			Count: 0,
		})
	}

	state := t.getState()

	// Count tasks and projects in a single pass
	var activeTasks, activeProjects int
	if state != nil {
		for _, task := range state.Tasks {
			if task.InTrash {
				continue
			}
			switch task.Type {
			case thingscloud.TaskTypeTask:
				if task.Status != thingscloud.TaskStatusCompleted && task.Status != thingscloud.TaskStatusCanceled {
					activeTasks++
				}
			case thingscloud.TaskTypeProject:
				if task.Status != thingscloud.TaskStatusCompleted {
					activeProjects++
				}
			}
		}
	}

	results = append(results,
		queryResult{Name: "activeTasks", OK: true, Count: activeTasks},
		queryResult{Name: "activeProjects", OK: true, Count: activeProjects},
	)
	step.Log = append(step.Log, fmt.Sprintf("Active tasks: %d", activeTasks))
	step.Log = append(step.Log, fmt.Sprintf("Active projects: %d", activeProjects))

	// Count areas and tags
	var areaCount, tagCount int
	if state != nil {
		areaCount = len(state.Areas)
		tagCount = len(state.Tags)
	}
	results = append(results,
		queryResult{Name: "areas", OK: true, Count: areaCount},
		queryResult{Name: "tags", OK: true, Count: tagCount},
	)
	step.Log = append(step.Log, fmt.Sprintf("Areas: %d", areaCount))
	step.Log = append(step.Log, fmt.Sprintf("Tags: %d", tagCount))

	step.DurationMs = time.Since(start).Milliseconds()

	step.Details = map[string]any{
		"queryResults": results,
	}

	if syncErr != nil {
		step.Status = "fail"
		*allErrors = append(*allErrors, fmt.Sprintf("Step 7: syncAndRebuild failed: %v", syncErr))
	} else {
		step.Status = "pass"
	}

	report.Steps = append(report.Steps, step)
}

func buildDiagSummary(steps []diagStep) diagSummary {
	s := diagSummary{TotalSteps: len(steps)}
	for _, step := range steps {
		s.TotalDurationMs += step.DurationMs
		switch step.Status {
		case "pass":
			s.Passed++
		case "warn":
			s.Warnings++
		case "fail":
			s.Failed++
		case "skipped":
			s.Skipped++
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// MCP Tool handlers
// ---------------------------------------------------------------------------

func (t *ThingsMCP) handleListTasks(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}
	state := t.getState()

	schedule := req.GetString("schedule", "")
	scheduledBefore := req.GetString("scheduled_before", "")
	scheduledAfter := req.GetString("scheduled_after", "")
	deadlineBefore := req.GetString("deadline_before", "")
	deadlineAfter := req.GetString("deadline_after", "")
	createdBefore := req.GetString("created_before", "")
	createdAfter := req.GetString("created_after", "")
	tagName := req.GetString("tag", "")
	areaName := req.GetString("area", "")
	projectName := req.GetString("project", "")
	containsText := strings.ToLower(req.GetString("contains_text", ""))
	inTrash := req.GetBool("in_trash", false)
	statusFilter := req.GetString("status", "pending")

	// Pre-resolve names to UUIDs
	var areaUUID, projectUUID, tagUUID string
	if areaName != "" {
		areaUUID = t.findAreaUUID(areaName)
		if areaUUID == "" {
			return errResult(fmt.Sprintf("area not found: %s", areaName)), nil
		}
	}
	if projectName != "" {
		projectUUID = t.findProjectUUID(projectName)
		if projectUUID == "" {
			return errResult(fmt.Sprintf("project not found: %s", projectName)), nil
		}
	}
	if tagName != "" {
		tagUUID = t.findTagUUID(tagName)
		if tagUUID == "" {
			return errResult(fmt.Sprintf("tag not found: %s", tagName)), nil
		}
	}

	// Parse date filters
	var scheduledBeforeDate, scheduledAfterDate, deadlineBeforeDate, deadlineAfterDate *time.Time
	if scheduledBefore != "" {
		scheduledBeforeDate = parseDate(scheduledBefore)
		if scheduledBeforeDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", scheduledBefore)), nil
		}
	}
	if scheduledAfter != "" {
		scheduledAfterDate = parseDate(scheduledAfter)
		if scheduledAfterDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", scheduledAfter)), nil
		}
	}
	if deadlineBefore != "" {
		deadlineBeforeDate = parseDate(deadlineBefore)
		if deadlineBeforeDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", deadlineBefore)), nil
		}
	}
	if deadlineAfter != "" {
		deadlineAfterDate = parseDate(deadlineAfter)
		if deadlineAfterDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", deadlineAfter)), nil
		}
	}
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

	var tasks []TaskOutput
	for _, task := range state.Tasks {
		// Skip headings and projects
		if task.Type == thingscloud.TaskTypeProject || task.Type == thingscloud.TaskTypeHeading {
			continue
		}
		// Default: exclude trashed and completed
		if !inTrash && task.InTrash {
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
		default: // "pending"
			if task.Status != 0 {
				continue
			}
		}

		// Schedule filter
		if schedule != "" {
			if schedule == "today" {
				if !isScheduledForTodayOrPast(task.Schedule, task.ScheduledDate) {
					continue
				}
			} else if schedule == "tonight" {
				if !isScheduledForTodayOrPast(task.Schedule, task.ScheduledDate) || task.StartBucket != 1 {
					continue
				}
			} else {
				taskSchedule := scheduleString(task.Schedule, task.ScheduledDate, task.StartBucket)
				if taskSchedule != schedule {
					continue
				}
			}
		}

		// Date range filters (exclusive)
		if scheduledBeforeDate != nil {
			if task.ScheduledDate == nil || !task.ScheduledDate.Before(*scheduledBeforeDate) {
				continue
			}
		}
		if scheduledAfterDate != nil {
			if task.ScheduledDate == nil || !task.ScheduledDate.After(*scheduledAfterDate) {
				continue
			}
		}
		if deadlineBeforeDate != nil {
			if task.DeadlineDate == nil || !task.DeadlineDate.Before(*deadlineBeforeDate) {
				continue
			}
		}
		if deadlineAfterDate != nil {
			if task.DeadlineDate == nil || !task.DeadlineDate.After(*deadlineAfterDate) {
				continue
			}
		}
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
		// Name-based filters
		if areaUUID != "" && !containsStr(task.AreaIDs, areaUUID) {
			continue
		}
		if projectUUID != "" && !containsStr(task.ParentTaskIDs, projectUUID) {
			continue
		}
		if tagUUID != "" && !containsStr(task.TagIDs, tagUUID) {
			continue
		}
		if containsText != "" && !strings.Contains(strings.ToLower(task.Title), containsText) && !strings.Contains(strings.ToLower(task.Note), containsText) {
			continue
		}

		tasks = append(tasks, t.taskToOutput(task))
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
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}

	state := t.getState()
	for _, task := range state.Tasks {
		if strings.HasPrefix(task.UUID, uuidPrefix) {
			out := TaskDetailOutput{TaskOutput: t.taskToOutput(task)}
			// Add checklist items
			for _, cli := range state.CheckListItems {
				if containsStr(cli.TaskIDs, task.UUID) {
					out.Checklist = append(out.Checklist, ChecklistOutput{
						UUID:   cli.UUID,
						Title:  cli.Title,
						Status: statusString(cli.Status),
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
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}

	state := t.getState()
	statusFilter := req.GetString("status", "pending")

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
			continue
		}
		placed := false
		for _, hid := range task.ActionGroupIDs {
			if h, ok := headingMap[hid]; ok {
				h.Tasks = append(h.Tasks, t.taskToOutput(task))
				placed = true
				break
			}
		}
		if !placed {
			unfiled = append(unfiled, t.taskToOutput(task))
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
		TaskOutput:   t.taskToOutput(project),
		Headings:     headings,
		UnfiledTasks: unfiled,
	}
	return jsonResult(out), nil
}

func (t *ThingsMCP) handleListProjects(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}
	state := t.getState()

	schedule := req.GetString("schedule", "")
	scheduledBefore := req.GetString("scheduled_before", "")
	scheduledAfter := req.GetString("scheduled_after", "")
	deadlineBefore := req.GetString("deadline_before", "")
	deadlineAfter := req.GetString("deadline_after", "")
	createdBefore := req.GetString("created_before", "")
	createdAfter := req.GetString("created_after", "")
	tagName := req.GetString("tag", "")
	areaName := req.GetString("area", "")
	containsText := strings.ToLower(req.GetString("contains_text", ""))
	inTrash := req.GetBool("in_trash", false)
	statusFilter := req.GetString("status", "pending")

	// Pre-resolve names to UUIDs
	var areaUUID, tagUUID string
	if areaName != "" {
		areaUUID = t.findAreaUUID(areaName)
		if areaUUID == "" {
			return errResult(fmt.Sprintf("area not found: %s", areaName)), nil
		}
	}
	if tagName != "" {
		tagUUID = t.findTagUUID(tagName)
		if tagUUID == "" {
			return errResult(fmt.Sprintf("tag not found: %s", tagName)), nil
		}
	}

	// Parse date filters
	var scheduledBeforeDate, scheduledAfterDate, deadlineBeforeDate, deadlineAfterDate *time.Time
	if scheduledBefore != "" {
		scheduledBeforeDate = parseDate(scheduledBefore)
		if scheduledBeforeDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", scheduledBefore)), nil
		}
	}
	if scheduledAfter != "" {
		scheduledAfterDate = parseDate(scheduledAfter)
		if scheduledAfterDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", scheduledAfter)), nil
		}
	}
	if deadlineBefore != "" {
		deadlineBeforeDate = parseDate(deadlineBefore)
		if deadlineBeforeDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", deadlineBefore)), nil
		}
	}
	if deadlineAfter != "" {
		deadlineAfterDate = parseDate(deadlineAfter)
		if deadlineAfterDate == nil {
			return errResult(fmt.Sprintf("invalid date: %s", deadlineAfter)), nil
		}
	}
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
		if task.Type != thingscloud.TaskTypeProject {
			continue
		}
		if !inTrash && task.InTrash {
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
		default: // "pending"
			if task.Status != 0 {
				continue
			}
		}

		// Schedule filter
		if schedule != "" {
			if schedule == "today" {
				if !isScheduledForTodayOrPast(task.Schedule, task.ScheduledDate) {
					continue
				}
			} else if schedule == "tonight" {
				if !isScheduledForTodayOrPast(task.Schedule, task.ScheduledDate) || task.StartBucket != 1 {
					continue
				}
			} else {
				taskSchedule := scheduleString(task.Schedule, task.ScheduledDate, task.StartBucket)
				if taskSchedule != schedule {
					continue
				}
			}
		}

		// Date range filters (exclusive)
		if scheduledBeforeDate != nil {
			if task.ScheduledDate == nil || !task.ScheduledDate.Before(*scheduledBeforeDate) {
				continue
			}
		}
		if scheduledAfterDate != nil {
			if task.ScheduledDate == nil || !task.ScheduledDate.After(*scheduledAfterDate) {
				continue
			}
		}
		if deadlineBeforeDate != nil {
			if task.DeadlineDate == nil || !task.DeadlineDate.Before(*deadlineBeforeDate) {
				continue
			}
		}
		if deadlineAfterDate != nil {
			if task.DeadlineDate == nil || !task.DeadlineDate.After(*deadlineAfterDate) {
				continue
			}
		}
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
		// Name-based filters
		if areaUUID != "" && !containsStr(task.AreaIDs, areaUUID) {
			continue
		}
		if tagUUID != "" && !containsStr(task.TagIDs, tagUUID) {
			continue
		}
		if containsText != "" && !strings.Contains(strings.ToLower(task.Title), containsText) && !strings.Contains(strings.ToLower(task.Note), containsText) {
			continue
		}

		projects = append(projects, t.taskToOutput(task))
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
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
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
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}
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
	if err := t.syncAndRebuild(); err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}
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
	for _, key := range []string{"note", "schedule", "deadline", "project_uuid", "heading_uuid", "area_uuid", "tags", "checklist", "reminder_date", "reminder_time", "recurrence"} {
		if v := req.GetString(key, ""); v != "" {
			opts[key] = v
		}
	}

	if err := t.validateOpts(opts); err != nil {
		return errResult(err.Error()), nil
	}

	// Validate recurrence format early
	if v, ok := opts["recurrence"]; ok && v != "" {
		if _, err := parseRecurrence(v, time.Now()); err != nil {
			return errResult(err.Error()), nil
		}
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
	for _, key := range []string{"note", "schedule", "deadline", "area_uuid", "tags", "reminder_date", "reminder_time", "recurrence"} {
		if v := req.GetString(key, ""); v != "" {
			opts[key] = v
		}
	}

	if err := t.validateOpts(opts); err != nil {
		return errResult(err.Error()), nil
	}

	// Validate recurrence format early
	if v, ok := opts["recurrence"]; ok && v != "" {
		if _, err := parseRecurrence(v, time.Now()); err != nil {
			return errResult(err.Error()), nil
		}
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

func (t *ThingsMCP) handleEditArea(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	areaUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}
	if err := t.validateAreaUUID(areaUUID); err != nil {
		return errResult(err.Error()), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return errResult("name is required"), nil
	}

	payload := map[string]any{"tt": name}
	env := writeEnvelope{id: areaUUID, action: 1, kind: "Area3", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("edit area: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "updated", "uuid": areaUUID}), nil
}

func (t *ThingsMCP) handleDeleteArea(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	areaUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}
	if err := t.validateAreaUUID(areaUUID); err != nil {
		return errResult(err.Error()), nil
	}

	payload := map[string]any{}
	env := writeEnvelope{id: areaUUID, action: 2, kind: "Area3", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("delete area: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "deleted", "uuid": areaUUID}), nil
}

func (t *ThingsMCP) handleEditTag(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tagUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}
	if err := t.validateTagUUID(tagUUID); err != nil {
		return errResult(err.Error()), nil
	}

	payload := map[string]any{}
	if v := req.GetString("name", ""); v != "" {
		payload["tt"] = v
	}
	if v := req.GetString("shorthand", ""); v != "" {
		payload["sh"] = v
	}
	if v := req.GetString("parent_uuid", ""); v != "" {
		payload["pn"] = []string{v}
	}

	if len(payload) == 0 {
		return errResult("at least one field (name, shorthand, parent_uuid) must be provided"), nil
	}

	env := writeEnvelope{id: tagUUID, action: 1, kind: "Tag4", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("edit tag: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "updated", "uuid": tagUUID}), nil
}

func (t *ThingsMCP) handleDeleteTag(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tagUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}
	if err := t.validateTagUUID(tagUUID); err != nil {
		return errResult(err.Error()), nil
	}

	payload := map[string]any{}
	env := writeEnvelope{id: tagUUID, action: 2, kind: "Tag4", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("delete tag: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "deleted", "uuid": tagUUID}), nil
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
	sched := req.GetString("schedule", "")
	if sched != "" {
		switch sched {
		case "today":
			today := todayMidnightUTC()
			u.Schedule(1, today, today).StartBucket(0)
		case "tonight":
			today := todayMidnightUTC()
			u.Schedule(1, today, today).StartBucket(1)
		case "anytime":
			u.Schedule(1, nil, nil).StartBucket(0)
		case "someday":
			u.Schedule(2, nil, nil).StartBucket(0)
		case "inbox":
			u.Schedule(0, nil, nil).StartBucket(0)
		default:
			// Try parsing as YYYY-MM-DD date
			if dt := parseDate(sched); dt != nil {
				ts := dt.Unix()
				u.Schedule(2, ts, ts)
			}
		}
	}
	if v := req.GetString("deadline", ""); v != "" {
		if dt := parseDate(v); dt != nil {
			u.Deadline(dt.Unix())
		}
	}
	if v := req.GetString("area_uuid", ""); v != "" {
		u.Area(v)
		if sched == "" {
			u.Schedule(1, nil, nil)
		}
	}
	if v := req.GetString("project_uuid", ""); v != "" {
		u.Project(v)
		if sched == "" {
			u.Schedule(1, nil, nil)
		}
	}
	if v := req.GetString("heading_uuid", ""); v != "" {
		u.Heading(v)
		if sched == "" {
			u.Schedule(1, nil, nil)
		}
	}
	if v := req.GetString("tags", ""); v != "" {
		u.Tags(strings.Split(v, ","))
	}
	if rmdStr := req.GetString("reminder_date", ""); rmdStr != "" {
		if rmdStr == "none" {
			u.ClearReminder()
		} else if timeStr := req.GetString("reminder_time", ""); timeStr != "" {
			if dt := parseDate(rmdStr); dt != nil {
				if offset, valid := parseTime(timeStr); valid {
					u.Reminder(dt.Unix()).AlarmOffset(offset)
				}
			}
		}
	}
	if v := req.GetString("recurrence", ""); v != "" {
		if v == "none" {
			u.ClearRecurrence()
		} else {
			// Use schedule date as reference for weekday, fall back to today
			recRef := time.Now()
			if schedStr := req.GetString("schedule", ""); schedStr != "" {
				if dt := parseDate(schedStr); dt != nil {
					recRef = *dt
				}
			}
			rr, err := parseRecurrence(v, recRef)
			if err != nil {
				return errResult(err.Error()), nil
			}
			if rr != nil {
				u.Recurrence(*rr)
				u.InstanceCreationStartDate(todayMidnightUTC())
			}
		}
	}
	if v := req.GetString("status", ""); v != "" {
		switch v {
		case "completed":
			u.Status(3).StopDate(nowTs())
		case "canceled":
			u.Status(2).StopDate(nowTs())
		case "pending":
			u.Status(0)
			u.fields["sp"] = nil
		case "trashed":
			u.Trash(true)
		case "restored":
			u.Trash(false)
		}
	}

	env := writeEnvelope{id: taskUUID, action: 1, kind: "Task6", payload: u.build()}
	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("edit task: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "updated", "uuid": taskUUID}), nil
}



// ---------------------------------------------------------------------------
// Checklist item operations
// ---------------------------------------------------------------------------

func (t *ThingsMCP) handleAddChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskUUID, err := req.RequireString("task_uuid")
	if err != nil {
		return errResult("task_uuid is required"), nil
	}
	title, err := req.RequireString("title")
	if err != nil {
		return errResult("title is required"), nil
	}

	if err := t.validateTaskUUID(taskUUID); err != nil {
		return errResult(err.Error()), nil
	}

	ix := req.GetInt("index", 0)
	itemUUID := generateUUID()
	now := nowTs()
	payload := ChecklistItemCreatePayload{
		Cd: now, Md: nil, Tt: title, Ss: 0, Sp: nil,
		Ix: ix, Ts: []string{taskUUID}, Lt: false, Xx: defaultExtension(),
	}
	env := writeEnvelope{id: itemUUID, action: 0, kind: "ChecklistItem3", payload: payload}

	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("add checklist item: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "created", "uuid": itemUUID, "task_uuid": taskUUID}), nil
}

func (t *ThingsMCP) handleEditChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	payload := map[string]any{"md": nowTs()}
	if v := req.GetString("title", ""); v != "" {
		payload["tt"] = v
	}
	if ix := req.GetInt("index", -1); ix >= 0 {
		payload["ix"] = ix
	}
	if completed, ok := req.GetArguments()["completed"]; ok {
		if b, _ := completed.(bool); b {
			payload["ss"] = 3
			payload["sp"] = nowTs()
		} else {
			payload["ss"] = 0
			payload["sp"] = nil
		}
	}

	env := writeEnvelope{id: itemUUID, action: 1, kind: "ChecklistItem3", payload: payload}
	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("edit checklist item: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "updated", "uuid": itemUUID}), nil
}

func (t *ThingsMCP) handleDeleteChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemUUID, err := req.RequireString("uuid")
	if err != nil {
		return errResult("uuid is required"), nil
	}

	env := writeEnvelope{id: itemUUID, action: 2, kind: "ChecklistItem3", payload: map[string]any{}}
	if err := t.writeAndSync(env); err != nil {
		return errResult(fmt.Sprintf("delete checklist item: %v", err)), nil
	}
	return jsonResult(map[string]string{"status": "deleted", "uuid": itemUUID}), nil
}

// ---------------------------------------------------------------------------
// Batch operations
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// MCP tool definitions
// ---------------------------------------------------------------------------

func defineTools(um *UserManager) []server.ServerTool {
	// wrap creates a handler closure that extracts ThingsMCP from context via UserManager.
	wrap := func(fn func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			t, err := getUserFromContext(ctx, um)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return fn(t, ctx, req)
		}
	}

	return []server.ServerTool{
		// --- Read tools ---
		{
			Tool: mcp.NewTool("things_list_tasks",
				mcp.WithDescription("List tasks from Things 3 with optional filters. Returns an array of task objects, each containing uuid, title, status (pending/completed/canceled), schedule (inbox/today/tonight/anytime/someday/upcoming), and optional fields: note, scheduledDate, deadlineDate, reminderTime, recurrence, areas, project, tags. Default: only pending (active) tasks. Use status parameter to query completed or canceled tasks. Note: schedule=today includes both regular and tonight tasks; use schedule=tonight to filter only tonight tasks."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("schedule", mcp.Description("Filter by schedule"), mcp.Enum("inbox", "today", "tonight", "anytime", "someday", "upcoming")),
				mcp.WithString("scheduled_before", mcp.Description("Return tasks scheduled before this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("scheduled_after", mcp.Description("Return tasks scheduled after this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("deadline_before", mcp.Description("Return tasks with deadline before this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("deadline_after", mcp.Description("Return tasks with deadline after this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("created_before", mcp.Description("Return tasks created before this date/time (YYYY-MM-DD or RFC3339 e.g. 2025-03-01T00:00:00+08:00, exclusive)")),
				mcp.WithString("created_after", mcp.Description("Return tasks created after this date/time (YYYY-MM-DD or RFC3339 e.g. 2025-03-01T00:00:00+08:00, exclusive)")),
				mcp.WithString("tag", mcp.Description("Filter by tag name (case-insensitive)")),
				mcp.WithString("area", mcp.Description("Filter by area name (case-insensitive)")),
				mcp.WithString("project", mcp.Description("Filter by project name (case-insensitive)")),
				mcp.WithString("contains_text", mcp.Description("Filter tasks whose title or note contains this text (case-insensitive)")),
				mcp.WithBoolean("in_trash", mcp.Description("When true, include trashed items in results (default false)")),
				mcp.WithString("status", mcp.Description("Filter by task status (default: pending — only active tasks)"), mcp.Enum("pending", "completed", "canceled")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleListTasks(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_show_task",
				mcp.WithDescription("Show full details of a single Things 3 task, including its checklist items. Returns a task object with uuid, title, status, schedule, note, dates, areas, project, tags, and a checklist array (each with uuid, title, status). Accepts a UUID prefix for convenience."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Task UUID or unique prefix of the UUID")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleShowTask(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_show_project",
				mcp.WithDescription("Show full details of a Things 3 project, including its headings and tasks grouped by heading. Returns the project info plus a headings array (each with uuid, title, and nested tasks) and an unfiledTasks array for tasks not under any heading."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("Project UUID")),
				mcp.WithString("status", mcp.Description("Filter child tasks by status (default: pending — only active tasks)"), mcp.Enum("pending", "completed", "canceled")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleShowProject(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_list_projects",
				mcp.WithDescription("List projects from Things 3 with optional filters. Returns an array of project objects, each containing uuid, title, status (pending/completed/canceled), schedule (inbox/today/tonight/anytime/someday/upcoming), and optional fields: note, scheduledDate, deadlineDate, areas, tags. Default: only pending (active) projects. Use status parameter to query completed or canceled projects."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("schedule", mcp.Description("Filter by schedule"), mcp.Enum("inbox", "today", "tonight", "anytime", "someday", "upcoming")),
				mcp.WithString("scheduled_before", mcp.Description("Return projects scheduled before this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("scheduled_after", mcp.Description("Return projects scheduled after this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("deadline_before", mcp.Description("Return projects with deadline before this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("deadline_after", mcp.Description("Return projects with deadline after this date (YYYY-MM-DD, exclusive)")),
				mcp.WithString("created_before", mcp.Description("Return projects created before this date/time (YYYY-MM-DD or RFC3339 e.g. 2025-03-01T00:00:00+08:00, exclusive)")),
				mcp.WithString("created_after", mcp.Description("Return projects created after this date/time (YYYY-MM-DD or RFC3339 e.g. 2025-03-01T00:00:00+08:00, exclusive)")),
				mcp.WithString("tag", mcp.Description("Filter by tag name (case-insensitive)")),
				mcp.WithString("area", mcp.Description("Filter by area name (case-insensitive)")),
				mcp.WithString("contains_text", mcp.Description("Filter projects whose title or note contains this text (case-insensitive)")),
				mcp.WithBoolean("in_trash", mcp.Description("When true, include trashed items in results (default false)")),
				mcp.WithString("status", mcp.Description("Filter by project status (default: pending — only active projects)"), mcp.Enum("pending", "completed", "canceled")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleListProjects(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_list_headings",
				mcp.WithDescription("List all headings within a Things 3 project. Returns an array of heading objects, each containing uuid and title. Use things_show_project to also see the tasks grouped under each heading."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("project_uuid", mcp.Required(), mcp.Description("UUID of the project to list headings from")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleListHeadings(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_list_areas",
				mcp.WithDescription("List all areas in Things 3. Areas are top-level organizational containers for projects and tasks. Returns an array of area objects, each containing uuid and title."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleListAreas(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_list_tags",
				mcp.WithDescription("List all tags in Things 3. Returns an array of tag objects, each containing uuid, title, and optional fields: shorthand (abbreviation) and parentIds (for nested tags)."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleListTags(ctx, req)
			}),
		},

		// --- Create tools ---
		{
			Tool: mcp.NewTool("things_create_task",
				mcp.WithDescription("Create a new task in Things 3. Returns {status: \"created\", uuid, title}. The task is placed in Inbox by default; set schedule or project_uuid/area_uuid to organize it. Use things_list_projects and things_list_areas first to get valid UUIDs for assignment."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("title", mcp.Required(), mcp.Description("Task title")),
				mcp.WithString("note", mcp.Description("Markdown-compatible note or description for the task")),
				mcp.WithString("schedule", mcp.Description("When to schedule: today, tonight (today's tonight), anytime, someday, inbox, or a date (YYYY-MM-DD). Dates go to Upcoming and auto-move to Today when due.")),
				mcp.WithString("deadline", mcp.Description("Deadline date in YYYY-MM-DD format")),
				mcp.WithString("project_uuid", mcp.Description("UUID of the project to add this task to. Use things_list_projects to find project UUIDs.")),
				mcp.WithString("heading_uuid", mcp.Description("UUID of the heading to place this task under within a project. Use things_list_headings to find heading UUIDs.")),
				mcp.WithString("area_uuid", mcp.Description("UUID of the area to assign this task to. Use things_list_areas to find area UUIDs.")),
				mcp.WithString("tags", mcp.Description("Comma-separated tag UUIDs to apply. Use things_list_tags to find tag UUIDs.")),
				mcp.WithString("checklist", mcp.Description("Comma-separated checklist item titles to create within the task")),
				mcp.WithString("reminder_date", mcp.Description("Reminder date in YYYY-MM-DD format. Must be used together with reminder_time.")),
				mcp.WithString("reminder_time", mcp.Description("Reminder time in HH:MM 24-hour format (e.g. 09:00, 14:30). Must be used together with reminder_date.")),
				mcp.WithString("recurrence", mcp.Description("Recurrence rule: daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days, every N weeks. Use \"none\" to clear.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleCreateTask(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_create_heading",
				mcp.WithDescription("Create a new heading within a Things 3 project. Headings are section dividers used to group tasks inside a project. Returns {status: \"created\", uuid, title}."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("title", mcp.Required(), mcp.Description("Heading title")),
				mcp.WithString("project_uuid", mcp.Required(), mcp.Description("UUID of the project to add the heading to. Use things_list_projects to find project UUIDs.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleCreateHeading(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_create_project",
				mcp.WithDescription("Create a new project in Things 3. Projects are containers that hold tasks and headings. Returns {status: \"created\", uuid, title}. Defaults to Anytime schedule."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("title", mcp.Required(), mcp.Description("Project title")),
				mcp.WithString("note", mcp.Description("Markdown-compatible note or description for the project")),
				mcp.WithString("schedule", mcp.Description("When to schedule: today, anytime (default), someday, or a date (YYYY-MM-DD).")),
				mcp.WithString("deadline", mcp.Description("Deadline date in YYYY-MM-DD format")),
				mcp.WithString("area_uuid", mcp.Description("UUID of the area to assign this project to. Use things_list_areas to find area UUIDs.")),
				mcp.WithString("tags", mcp.Description("Comma-separated tag UUIDs to apply. Use things_list_tags to find tag UUIDs.")),
				mcp.WithString("reminder_date", mcp.Description("Reminder date in YYYY-MM-DD format. Must be used together with reminder_time.")),
				mcp.WithString("reminder_time", mcp.Description("Reminder time in HH:MM 24-hour format (e.g. 09:00, 14:30). Must be used together with reminder_date.")),
				mcp.WithString("recurrence", mcp.Description("Recurrence rule: daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days, every N weeks.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleCreateProject(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_create_area",
				mcp.WithDescription("Create a new area in Things 3. Areas are top-level organizational containers (e.g. \"Work\", \"Personal\") that group projects and tasks. Returns {status: \"created\", uuid, name}."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("name", mcp.Required(), mcp.Description("Area name")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleCreateArea(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_create_tag",
				mcp.WithDescription("Create a new tag in Things 3. Tags can be applied to tasks and projects for cross-cutting categorization. Tags support nesting via parent_uuid. Returns {status: \"created\", uuid, name}."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("name", mcp.Required(), mcp.Description("Tag name")),
				mcp.WithString("shorthand", mcp.Description("Short abbreviation for the tag")),
				mcp.WithString("parent_uuid", mcp.Description("UUID of the parent tag for nesting. Use things_list_tags to find existing tag UUIDs.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleCreateTag(ctx, req)
			}),
		},

		// --- Area/Tag edit & delete tools ---
		{
			Tool: mcp.NewTool("things_edit_area",
				mcp.WithDescription("Rename an existing area in Things 3. Returns {status: \"updated\", uuid}."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the area to rename. Use things_list_areas to find area UUIDs.")),
				mcp.WithString("name", mcp.Required(), mcp.Description("New area name")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleEditArea(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_delete_area",
				mcp.WithDescription("Permanently delete an area from Things 3. Tasks and projects in this area will become unassigned. This action cannot be undone. Returns {status: \"deleted\", uuid}."),
				mcp.WithDestructiveHintAnnotation(true),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the area to delete. Use things_list_areas to find area UUIDs.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleDeleteArea(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_edit_tag",
				mcp.WithDescription("Edit an existing tag in Things 3. Only provided fields are updated; omitted fields remain unchanged. Returns {status: \"updated\", uuid}."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the tag to edit. Use things_list_tags to find tag UUIDs.")),
				mcp.WithString("name", mcp.Description("New tag name")),
				mcp.WithString("shorthand", mcp.Description("New short abbreviation for the tag")),
				mcp.WithString("parent_uuid", mcp.Description("UUID of the new parent tag for nesting. Use things_list_tags to find tag UUIDs.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleEditTag(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_delete_tag",
				mcp.WithDescription("Permanently delete a tag from Things 3. The tag will be removed from all tasks and projects that use it. This action cannot be undone. Returns {status: \"deleted\", uuid}."),
				mcp.WithDestructiveHintAnnotation(true),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the tag to delete. Use things_list_tags to find tag UUIDs.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleDeleteTag(ctx, req)
			}),
		},

		// --- Modify tools ---
		{
			Tool: mcp.NewTool("things_edit_item",
				mcp.WithDescription("Edit an existing task or project in Things 3. Only provided fields are updated; omitted fields remain unchanged. Can also change status to complete, cancel, trash, or restore items. Returns {status: \"updated\", uuid}."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the task or project to edit")),
				mcp.WithString("title", mcp.Description("New title")),
				mcp.WithString("note", mcp.Description("New note content (replaces existing note)")),
				mcp.WithString("schedule", mcp.Description("When to schedule: today, tonight (today's tonight), anytime, someday, inbox, or a date (YYYY-MM-DD). Dates go to Upcoming and auto-move to Today when due.")),
				mcp.WithString("deadline", mcp.Description("Deadline date in YYYY-MM-DD format")),
				mcp.WithString("area_uuid", mcp.Description("UUID of the area to assign to. Use things_list_areas to find area UUIDs.")),
				mcp.WithString("project_uuid", mcp.Description("UUID of the project to move to. Use things_list_projects to find project UUIDs.")),
				mcp.WithString("heading_uuid", mcp.Description("UUID of the heading to place under. Use things_list_headings to find heading UUIDs.")),
				mcp.WithString("tags", mcp.Description("Comma-separated tag UUIDs (replaces all existing tags). Use things_list_tags to find tag UUIDs.")),
				mcp.WithString("reminder_date", mcp.Description("Reminder date in YYYY-MM-DD format, or \"none\" to clear. Must be used together with reminder_time.")),
				mcp.WithString("reminder_time", mcp.Description("Reminder time in HH:MM 24-hour format (e.g. 09:00, 14:30). Must be used together with reminder_date.")),
				mcp.WithString("recurrence", mcp.Description("Recurrence rule: daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days, every N weeks. Use \"none\" to clear.")),
				mcp.WithString("status", mcp.Description("Set item status: pending, completed, canceled, trashed (move to trash), restored (restore from trash)"), mcp.Enum("pending", "completed", "canceled", "trashed", "restored")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleEditTask(ctx, req)
			}),
		},

		// --- Checklist tools ---
		{
			Tool: mcp.NewTool("things_add_checklist_item",
				mcp.WithDescription("Add a checklist item to an existing Things 3 task. Checklist items are sub-steps within a task. Returns {status: \"created\", uuid, task_uuid}. Use things_show_task to see existing checklist items."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("task_uuid", mcp.Required(), mcp.Description("UUID of the parent task to add the checklist item to")),
				mcp.WithString("title", mcp.Required(), mcp.Description("Checklist item title")),
				mcp.WithNumber("index", mcp.Description("Sort position within the checklist (default 0, lower values appear first)")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleAddChecklistItem(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_edit_checklist_item",
				mcp.WithDescription("Edit an existing checklist item in Things 3. Only provided fields are updated; omitted fields remain unchanged. Returns {status: \"updated\", uuid}. Use things_show_task to find checklist item UUIDs."),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the checklist item to edit. Use things_show_task to find checklist item UUIDs.")),
				mcp.WithString("title", mcp.Description("New checklist item title")),
				mcp.WithNumber("index", mcp.Description("New sort position within the checklist")),
				mcp.WithBoolean("completed", mcp.Description("Set true to mark as completed, false to mark as pending")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleEditChecklistItem(ctx, req)
			}),
		},
		{
			Tool: mcp.NewTool("things_delete_checklist_item",
				mcp.WithDescription("Permanently delete a checklist item from a Things 3 task. This action cannot be undone. Returns {status: \"deleted\", uuid}. Use things_show_task to find checklist item UUIDs."),
				mcp.WithDestructiveHintAnnotation(true),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
				mcp.WithString("uuid", mcp.Required(), mcp.Description("UUID of the checklist item to delete. Use things_show_task to find checklist item UUIDs.")),
			),
			Handler: wrap(func(t *ThingsMCP, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return t.handleDeleteChecklistItem(ctx, req)
			}),
		},

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
				email, password, err := extractCredentials(ctx, um)
				if err != nil {
					return errResult(err.Error()), nil
				}
				t, err := getUserFromContext(ctx, um)
				if err != nil {
					return errResult(err.Error()), nil
				}
				report := t.handleDiagnose(email, password)

				// Store report and generate shareable URL
				type diagResponse struct {
					*diagReport
					ShareURL string `json:"shareUrl,omitempty"`
				}
				resp := diagResponse{diagReport: report}
				if um.diagStore != nil {
					if token, storeErr := um.diagStore.Store(email, report); storeErr == nil {
						if base := getBaseURLFromContext(ctx); base != "" {
							resp.ShareURL = base + "/d/" + token
						}
					}
				}
				return jsonResult(resp), nil
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Diagnosis report page
// ---------------------------------------------------------------------------

func serveDiagReportPage(w http.ResponseWriter, reportJSON string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(reportJSON))
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[things-mcp] ")

	um := NewUserManager()

	// Initialize OAuth server with persistent state
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	oauth := NewOAuthServer(um, dataDir)
	um.oauth = oauth
	um.diagStore = &DiagStore{db: oauth.db}

	hooks := &server.Hooks{}
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		result.ServerInfo.Icons = []mcp.Icon{
			{Src: "https://thingscloudmcp.com/favicon.svg", MIMEType: "image/svg+xml"},
		}
	})

	mcpServer := server.NewMCPServer(
		"Things Cloud MCP",
		"1.1.7",
		server.WithToolCapabilities(false),
		server.WithHooks(hooks),
		server.WithInstructions("Things Cloud MCP server for managing Things 3 tasks, projects, areas, and tags. "+
			"Use list_tasks with filters (area, project, status, tag) to find tasks. "+
			"Use edit_item to modify any item's title, notes, dates, tags, or status. "+
			"Use edit_item with status=completed to complete tasks, or status=canceled to cancel them. "+
			"Use create_task, create_project, create_area, or create_tag to create new items. "+
			"Use batch_create for creating multiple items at once efficiently. "+
			"Use move_item to reorganize tasks between projects and areas. "+
			"All changes sync to Things 3 apps (Mac, iPhone, iPad) in real-time via Things Cloud."),
	)
	mcpServer.AddTools(defineTools(um)...)

	streamServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
		server.WithHTTPContextFunc(um.httpContextFunc),
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

	// Wrap /mcp handler with 401 WWW-Authenticate for unauthenticated requests
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			base := getBaseURL(r)
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		streamServer.ServeHTTP(w, r)
	})

	// OAuth 2.1 routes (path-aware per RFC 9728: client appends resource path)
	mux.HandleFunc("/.well-known/oauth-protected-resource", oauth.handleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", oauth.handleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", oauth.handleAuthServerMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server/mcp", oauth.handleAuthServerMetadata)
	mux.HandleFunc("/register", oauth.handleRegister)
	mux.HandleFunc("/authorize", oauth.handleAuthorize)
	mux.HandleFunc("/token", oauth.handleToken)

	mux.HandleFunc("/docs", handleDocsPage)
	mux.HandleFunc("/how-it-works", handleHowItWorksPage)
	mux.HandleFunc("/favicon.ico", handleFavicon)
	mux.HandleFunc("/favicon.svg", handleFavicon)
	mux.HandleFunc("/d/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := strings.TrimPrefix(r.URL.Path, "/d/")
		if _, err := uuid.Parse(token); err != nil {
			http.NotFound(w, r)
			return
		}
		reportJSON, _, _, err := um.diagStore.Load(token)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		serveDiagReportPage(w, reportJSON)
	})

	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("Things Cloud MCP server listening on %s", addr)
	log.Printf("  Landing page: http://localhost%s/", addr)
	log.Printf("  MCP endpoint: http://localhost%s/mcp", addr)
	log.Printf("  OAuth metadata: http://localhost%s/.well-known/oauth-authorization-server", addr)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
