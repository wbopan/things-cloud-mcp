# Persistent Sync Engine Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a SQLite-backed sync engine that tracks Things Cloud state and surfaces semantic change events for agent consumption.

**Architecture:** New `sync/` subpackage with `Syncer` as entry point. Uses `modernc.org/sqlite` (pure Go). Change detection compares incoming Items against stored state to generate typed events. All entities soft-deleted to preserve history.

**Tech Stack:** Go 1.18+, `database/sql`, `modernc.org/sqlite`, existing `thingscloud` types

---

## Phase 1: Foundation

### Task 1: Add SQLite dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run:
```bash
cd /Users/arthur.soares/Github/things-cloud-sdk && go get modernc.org/sqlite
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./...
```
Expected: No errors

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add modernc.org/sqlite for sync engine"
```

---

### Task 2: Create package structure and schema

**Files:**
- Create: `sync/sync.go`
- Create: `sync/schema.go`

**Step 1: Create the package with Syncer struct**

Create `sync/sync.go`:
```go
// Package sync provides a persistent sync engine for Things Cloud.
// It stores state in SQLite and surfaces semantic change events.
package sync

import (
	"database/sql"
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
	_ "modernc.org/sqlite"
)

// Syncer manages persistent sync with Things Cloud
type Syncer struct {
	db      *sql.DB
	client  *things.Client
	history *things.History
}

// Open creates or opens a sync database and connects to Things Cloud
func Open(dbPath string, client *things.Client) (*Syncer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Syncer{
		db:     db,
		client: client,
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Close closes the database connection
func (s *Syncer) Close() error {
	return s.db.Close()
}
```

**Step 2: Create schema with migration**

Create `sync/schema.go`:
```go
package sync

const schemaVersion = 1

const schema = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

-- Sync metadata (singleton row)
CREATE TABLE IF NOT EXISTS sync_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    history_id TEXT NOT NULL,
    server_index INTEGER NOT NULL DEFAULT 0,
    last_sync_at INTEGER
);

-- Core entities
CREATE TABLE IF NOT EXISTS areas (
    uuid TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    "index" INTEGER DEFAULT 0,
    deleted INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tags (
    uuid TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    shortcut TEXT DEFAULT '',
    parent_uuid TEXT,
    "index" INTEGER DEFAULT 0,
    deleted INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tasks (
    uuid TEXT PRIMARY KEY,
    type INTEGER NOT NULL DEFAULT 0,
    title TEXT NOT NULL DEFAULT '',
    note TEXT DEFAULT '',
    status INTEGER NOT NULL DEFAULT 0,
    schedule INTEGER NOT NULL DEFAULT 0,
    scheduled_date INTEGER,
    deadline_date INTEGER,
    completion_date INTEGER,
    creation_date INTEGER,
    modification_date INTEGER,
    "index" INTEGER DEFAULT 0,
    today_index INTEGER DEFAULT 0,
    in_trash INTEGER DEFAULT 0,
    area_uuid TEXT,
    project_uuid TEXT,
    heading_uuid TEXT,
    alarm_time_offset INTEGER,
    recurrence_rule TEXT,
    deleted INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS checklist_items (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT,
    title TEXT NOT NULL DEFAULT '',
    status INTEGER NOT NULL DEFAULT 0,
    "index" INTEGER DEFAULT 0,
    creation_date INTEGER,
    completion_date INTEGER,
    deleted INTEGER DEFAULT 0
);

-- Junction tables
CREATE TABLE IF NOT EXISTS task_tags (
    task_uuid TEXT NOT NULL,
    tag_uuid TEXT NOT NULL,
    PRIMARY KEY (task_uuid, tag_uuid)
);

CREATE TABLE IF NOT EXISTS area_tags (
    area_uuid TEXT NOT NULL,
    tag_uuid TEXT NOT NULL,
    PRIMARY KEY (area_uuid, tag_uuid)
);

-- Change log
CREATE TABLE IF NOT EXISTS change_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_index INTEGER NOT NULL,
    synced_at INTEGER NOT NULL,
    change_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_uuid TEXT NOT NULL,
    payload TEXT
);

CREATE INDEX IF NOT EXISTS idx_change_log_synced_at ON change_log(synced_at);
CREATE INDEX IF NOT EXISTS idx_change_log_entity ON change_log(entity_type, entity_uuid);
`

func (s *Syncer) migrate() error {
	// Check current version
	var version int
	err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil {
		// Table doesn't exist or is empty, run full migration
		if _, err := s.db.Exec(schema); err != nil {
			return err
		}
		_, err = s.db.Exec("INSERT OR REPLACE INTO schema_version (version) VALUES (?)", schemaVersion)
		return err
	}

	// Already at current version
	if version >= schemaVersion {
		return nil
	}

	// Future: handle incremental migrations here
	return nil
}
```

**Step 3: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 4: Commit**

```bash
git add sync/
git commit -m "feat(sync): add package structure and SQLite schema"
```

---

### Task 3: Write test for Open/Close and schema creation

**Files:**
- Create: `sync/sync_test.go`

**Step 1: Write the test**

Create `sync/sync_test.go`:
```go
package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	t.Run("creates new database", func(t *testing.T) {
		t.Parallel()
		dbPath := filepath.Join(t.TempDir(), "test.db")

		syncer, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer syncer.Close()

		// Verify file was created
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Fatal("Database file was not created")
		}

		// Verify schema was applied by checking tables exist
		var tableName string
		err = syncer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&tableName)
		if err != nil {
			t.Fatalf("tasks table not created: %v", err)
		}
	})

	t.Run("reopens existing database", func(t *testing.T) {
		t.Parallel()
		dbPath := filepath.Join(t.TempDir(), "test.db")

		// Create and close
		syncer1, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("First Open failed: %v", err)
		}

		// Insert test data
		_, err = syncer1.db.Exec("INSERT INTO areas (uuid, title) VALUES ('test-uuid', 'Test Area')")
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
		syncer1.Close()

		// Reopen
		syncer2, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("Second Open failed: %v", err)
		}
		defer syncer2.Close()

		// Verify data persisted
		var title string
		err = syncer2.db.QueryRow("SELECT title FROM areas WHERE uuid = 'test-uuid'").Scan(&title)
		if err != nil {
			t.Fatalf("Data not persisted: %v", err)
		}
		if title != "Test Area" {
			t.Fatalf("Expected 'Test Area', got %q", title)
		}
	})
}
```

**Step 2: Run the test**

Run:
```bash
go test -v ./sync/...
```
Expected: PASS

**Step 3: Commit**

```bash
git add sync/sync_test.go
git commit -m "test(sync): add tests for Open/Close and schema creation"
```

---

## Phase 2: Change Types

### Task 4: Define Change interface and base types

**Files:**
- Create: `sync/changes.go`

**Step 1: Create the Change interface and common types**

Create `sync/changes.go`:
```go
package sync

import (
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

// Change represents a semantic change detected during sync
type Change interface {
	ChangeType() string
	EntityType() string
	EntityUUID() string
	ServerIndex() int
	Timestamp() time.Time
}

// TaskLocation represents where a task lives in the Things hierarchy
type TaskLocation int

const (
	LocationUnknown TaskLocation = iota
	LocationInbox
	LocationToday
	LocationAnytime
	LocationSomeday
	LocationUpcoming
	LocationProject
)

func (l TaskLocation) String() string {
	switch l {
	case LocationInbox:
		return "Inbox"
	case LocationToday:
		return "Today"
	case LocationAnytime:
		return "Anytime"
	case LocationSomeday:
		return "Someday"
	case LocationUpcoming:
		return "Upcoming"
	case LocationProject:
		return "Project"
	default:
		return "Unknown"
	}
}

// baseChange contains fields common to all changes
type baseChange struct {
	serverIndex int
	timestamp   time.Time
}

func (b baseChange) ServerIndex() int    { return b.serverIndex }
func (b baseChange) Timestamp() time.Time { return b.timestamp }

// --- Task Changes ---

type TaskCreated struct {
	baseChange
	Task *things.Task
}

func (c TaskCreated) ChangeType() string  { return "TaskCreated" }
func (c TaskCreated) EntityType() string  { return "task" }
func (c TaskCreated) EntityUUID() string  { return c.Task.UUID }

type TaskDeleted struct {
	baseChange
	Task *things.Task // last known state
}

func (c TaskDeleted) ChangeType() string  { return "TaskDeleted" }
func (c TaskDeleted) EntityType() string  { return "task" }
func (c TaskDeleted) EntityUUID() string  { return c.Task.UUID }

type TaskCompleted struct {
	baseChange
	Task *things.Task
}

func (c TaskCompleted) ChangeType() string  { return "TaskCompleted" }
func (c TaskCompleted) EntityType() string  { return "task" }
func (c TaskCompleted) EntityUUID() string  { return c.Task.UUID }

type TaskUncompleted struct {
	baseChange
	Task *things.Task
}

func (c TaskUncompleted) ChangeType() string  { return "TaskUncompleted" }
func (c TaskUncompleted) EntityType() string  { return "task" }
func (c TaskUncompleted) EntityUUID() string  { return c.Task.UUID }

type TaskCanceled struct {
	baseChange
	Task *things.Task
}

func (c TaskCanceled) ChangeType() string  { return "TaskCanceled" }
func (c TaskCanceled) EntityType() string  { return "task" }
func (c TaskCanceled) EntityUUID() string  { return c.Task.UUID }

type TaskTitleChanged struct {
	baseChange
	Task     *things.Task
	OldTitle string
}

func (c TaskTitleChanged) ChangeType() string  { return "TaskTitleChanged" }
func (c TaskTitleChanged) EntityType() string  { return "task" }
func (c TaskTitleChanged) EntityUUID() string  { return c.Task.UUID }

type TaskNoteChanged struct {
	baseChange
	Task    *things.Task
	OldNote string
}

func (c TaskNoteChanged) ChangeType() string  { return "TaskNoteChanged" }
func (c TaskNoteChanged) EntityType() string  { return "task" }
func (c TaskNoteChanged) EntityUUID() string  { return c.Task.UUID }

type TaskMovedToInbox struct {
	baseChange
	Task *things.Task
	From TaskLocation
}

func (c TaskMovedToInbox) ChangeType() string  { return "TaskMovedToInbox" }
func (c TaskMovedToInbox) EntityType() string  { return "task" }
func (c TaskMovedToInbox) EntityUUID() string  { return c.Task.UUID }

type TaskMovedToToday struct {
	baseChange
	Task *things.Task
	From TaskLocation
}

func (c TaskMovedToToday) ChangeType() string  { return "TaskMovedToToday" }
func (c TaskMovedToToday) EntityType() string  { return "task" }
func (c TaskMovedToToday) EntityUUID() string  { return c.Task.UUID }

type TaskMovedToAnytime struct {
	baseChange
	Task *things.Task
	From TaskLocation
}

func (c TaskMovedToAnytime) ChangeType() string  { return "TaskMovedToAnytime" }
func (c TaskMovedToAnytime) EntityType() string  { return "task" }
func (c TaskMovedToAnytime) EntityUUID() string  { return c.Task.UUID }

type TaskMovedToSomeday struct {
	baseChange
	Task *things.Task
	From TaskLocation
}

func (c TaskMovedToSomeday) ChangeType() string  { return "TaskMovedToSomeday" }
func (c TaskMovedToSomeday) EntityType() string  { return "task" }
func (c TaskMovedToSomeday) EntityUUID() string  { return c.Task.UUID }

type TaskMovedToUpcoming struct {
	baseChange
	Task         *things.Task
	From         TaskLocation
	ScheduledFor time.Time
}

func (c TaskMovedToUpcoming) ChangeType() string  { return "TaskMovedToUpcoming" }
func (c TaskMovedToUpcoming) EntityType() string  { return "task" }
func (c TaskMovedToUpcoming) EntityUUID() string  { return c.Task.UUID }

type TaskDeadlineChanged struct {
	baseChange
	Task        *things.Task
	OldDeadline *time.Time
}

func (c TaskDeadlineChanged) ChangeType() string  { return "TaskDeadlineChanged" }
func (c TaskDeadlineChanged) EntityType() string  { return "task" }
func (c TaskDeadlineChanged) EntityUUID() string  { return c.Task.UUID }

type TaskAssignedToProject struct {
	baseChange
	Task       *things.Task
	Project    *things.Task
	OldProject *things.Task
}

func (c TaskAssignedToProject) ChangeType() string  { return "TaskAssignedToProject" }
func (c TaskAssignedToProject) EntityType() string  { return "task" }
func (c TaskAssignedToProject) EntityUUID() string  { return c.Task.UUID }

type TaskAssignedToArea struct {
	baseChange
	Task    *things.Task
	Area    *things.Area
	OldArea *things.Area
}

func (c TaskAssignedToArea) ChangeType() string  { return "TaskAssignedToArea" }
func (c TaskAssignedToArea) EntityType() string  { return "task" }
func (c TaskAssignedToArea) EntityUUID() string  { return c.Task.UUID }

type TaskTrashed struct {
	baseChange
	Task *things.Task
}

func (c TaskTrashed) ChangeType() string  { return "TaskTrashed" }
func (c TaskTrashed) EntityType() string  { return "task" }
func (c TaskTrashed) EntityUUID() string  { return c.Task.UUID }

type TaskRestored struct {
	baseChange
	Task *things.Task
}

func (c TaskRestored) ChangeType() string  { return "TaskRestored" }
func (c TaskRestored) EntityType() string  { return "task" }
func (c TaskRestored) EntityUUID() string  { return c.Task.UUID }

type TaskTagsChanged struct {
	baseChange
	Task    *things.Task
	Added   []string
	Removed []string
}

func (c TaskTagsChanged) ChangeType() string  { return "TaskTagsChanged" }
func (c TaskTagsChanged) EntityType() string  { return "task" }
func (c TaskTagsChanged) EntityUUID() string  { return c.Task.UUID }
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/changes.go
git commit -m "feat(sync): add Change interface and task change types"
```

---

### Task 5: Add Project, Heading, Area, Tag, Checklist change types

**Files:**
- Modify: `sync/changes.go`

**Step 1: Add remaining change types**

Append to `sync/changes.go`:
```go

// --- Project Changes (projects are tasks with type=1) ---

type ProjectCreated struct {
	baseChange
	Project *things.Task
}

func (c ProjectCreated) ChangeType() string  { return "ProjectCreated" }
func (c ProjectCreated) EntityType() string  { return "project" }
func (c ProjectCreated) EntityUUID() string  { return c.Project.UUID }

type ProjectDeleted struct {
	baseChange
	Project *things.Task
}

func (c ProjectDeleted) ChangeType() string  { return "ProjectDeleted" }
func (c ProjectDeleted) EntityType() string  { return "project" }
func (c ProjectDeleted) EntityUUID() string  { return c.Project.UUID }

type ProjectCompleted struct {
	baseChange
	Project *things.Task
}

func (c ProjectCompleted) ChangeType() string  { return "ProjectCompleted" }
func (c ProjectCompleted) EntityType() string  { return "project" }
func (c ProjectCompleted) EntityUUID() string  { return c.Project.UUID }

type ProjectTitleChanged struct {
	baseChange
	Project  *things.Task
	OldTitle string
}

func (c ProjectTitleChanged) ChangeType() string  { return "ProjectTitleChanged" }
func (c ProjectTitleChanged) EntityType() string  { return "project" }
func (c ProjectTitleChanged) EntityUUID() string  { return c.Project.UUID }

type ProjectTrashed struct {
	baseChange
	Project *things.Task
}

func (c ProjectTrashed) ChangeType() string  { return "ProjectTrashed" }
func (c ProjectTrashed) EntityType() string  { return "project" }
func (c ProjectTrashed) EntityUUID() string  { return c.Project.UUID }

type ProjectRestored struct {
	baseChange
	Project *things.Task
}

func (c ProjectRestored) ChangeType() string  { return "ProjectRestored" }
func (c ProjectRestored) EntityType() string  { return "project" }
func (c ProjectRestored) EntityUUID() string  { return c.Project.UUID }

// --- Heading Changes (headings are tasks with type=2) ---

type HeadingCreated struct {
	baseChange
	Heading *things.Task
	Project *things.Task
}

func (c HeadingCreated) ChangeType() string  { return "HeadingCreated" }
func (c HeadingCreated) EntityType() string  { return "heading" }
func (c HeadingCreated) EntityUUID() string  { return c.Heading.UUID }

type HeadingDeleted struct {
	baseChange
	Heading *things.Task
}

func (c HeadingDeleted) ChangeType() string  { return "HeadingDeleted" }
func (c HeadingDeleted) EntityType() string  { return "heading" }
func (c HeadingDeleted) EntityUUID() string  { return c.Heading.UUID }

type HeadingTitleChanged struct {
	baseChange
	Heading  *things.Task
	OldTitle string
}

func (c HeadingTitleChanged) ChangeType() string  { return "HeadingTitleChanged" }
func (c HeadingTitleChanged) EntityType() string  { return "heading" }
func (c HeadingTitleChanged) EntityUUID() string  { return c.Heading.UUID }

// --- Area Changes ---

type AreaCreated struct {
	baseChange
	Area *things.Area
}

func (c AreaCreated) ChangeType() string  { return "AreaCreated" }
func (c AreaCreated) EntityType() string  { return "area" }
func (c AreaCreated) EntityUUID() string  { return c.Area.UUID }

type AreaDeleted struct {
	baseChange
	Area *things.Area
}

func (c AreaDeleted) ChangeType() string  { return "AreaDeleted" }
func (c AreaDeleted) EntityType() string  { return "area" }
func (c AreaDeleted) EntityUUID() string  { return c.Area.UUID }

type AreaRenamed struct {
	baseChange
	Area     *things.Area
	OldTitle string
}

func (c AreaRenamed) ChangeType() string  { return "AreaRenamed" }
func (c AreaRenamed) EntityType() string  { return "area" }
func (c AreaRenamed) EntityUUID() string  { return c.Area.UUID }

// --- Tag Changes ---

type TagCreated struct {
	baseChange
	Tag *things.Tag
}

func (c TagCreated) ChangeType() string  { return "TagCreated" }
func (c TagCreated) EntityType() string  { return "tag" }
func (c TagCreated) EntityUUID() string  { return c.Tag.UUID }

type TagDeleted struct {
	baseChange
	Tag *things.Tag
}

func (c TagDeleted) ChangeType() string  { return "TagDeleted" }
func (c TagDeleted) EntityType() string  { return "tag" }
func (c TagDeleted) EntityUUID() string  { return c.Tag.UUID }

type TagRenamed struct {
	baseChange
	Tag      *things.Tag
	OldTitle string
}

func (c TagRenamed) ChangeType() string  { return "TagRenamed" }
func (c TagRenamed) EntityType() string  { return "tag" }
func (c TagRenamed) EntityUUID() string  { return c.Tag.UUID }

type TagShortcutChanged struct {
	baseChange
	Tag         *things.Tag
	OldShortcut string
}

func (c TagShortcutChanged) ChangeType() string  { return "TagShortcutChanged" }
func (c TagShortcutChanged) EntityType() string  { return "tag" }
func (c TagShortcutChanged) EntityUUID() string  { return c.Tag.UUID }

// --- Checklist Item Changes ---

type ChecklistItemCreated struct {
	baseChange
	Item *things.CheckListItem
	Task *things.Task
}

func (c ChecklistItemCreated) ChangeType() string  { return "ChecklistItemCreated" }
func (c ChecklistItemCreated) EntityType() string  { return "checklist" }
func (c ChecklistItemCreated) EntityUUID() string  { return c.Item.UUID }

type ChecklistItemDeleted struct {
	baseChange
	Item *things.CheckListItem
}

func (c ChecklistItemDeleted) ChangeType() string  { return "ChecklistItemDeleted" }
func (c ChecklistItemDeleted) EntityType() string  { return "checklist" }
func (c ChecklistItemDeleted) EntityUUID() string  { return c.Item.UUID }

type ChecklistItemCompleted struct {
	baseChange
	Item *things.CheckListItem
	Task *things.Task
}

func (c ChecklistItemCompleted) ChangeType() string  { return "ChecklistItemCompleted" }
func (c ChecklistItemCompleted) EntityType() string  { return "checklist" }
func (c ChecklistItemCompleted) EntityUUID() string  { return c.Item.UUID }

type ChecklistItemUncompleted struct {
	baseChange
	Item *things.CheckListItem
	Task *things.Task
}

func (c ChecklistItemUncompleted) ChangeType() string  { return "ChecklistItemUncompleted" }
func (c ChecklistItemUncompleted) EntityType() string  { return "checklist" }
func (c ChecklistItemUncompleted) EntityUUID() string  { return c.Item.UUID }

type ChecklistItemTitleChanged struct {
	baseChange
	Item     *things.CheckListItem
	OldTitle string
}

func (c ChecklistItemTitleChanged) ChangeType() string  { return "ChecklistItemTitleChanged" }
func (c ChecklistItemTitleChanged) EntityType() string  { return "checklist" }
func (c ChecklistItemTitleChanged) EntityUUID() string  { return c.Item.UUID }

// --- Catchall ---

type UnknownChange struct {
	baseChange
	entityType string
	entityUUID string
	Details    string
}

func (c UnknownChange) ChangeType() string  { return "UnknownChange" }
func (c UnknownChange) EntityType() string  { return c.entityType }
func (c UnknownChange) EntityUUID() string  { return c.entityUUID }
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/changes.go
git commit -m "feat(sync): add project, heading, area, tag, checklist change types"
```

---

## Phase 3: Database Layer

### Task 6: Add entity storage functions (tasks)

**Files:**
- Create: `sync/store.go`

**Step 1: Create store with task CRUD**

Create `sync/store.go`:
```go
package sync

import (
	"database/sql"
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

// getTask retrieves a task from the database by UUID
func (s *Syncer) getTask(uuid string) (*things.Task, error) {
	row := s.db.QueryRow(`
		SELECT uuid, type, title, note, status, schedule,
		       scheduled_date, deadline_date, completion_date,
		       creation_date, modification_date, "index", today_index,
		       in_trash, area_uuid, project_uuid, heading_uuid,
		       alarm_time_offset, deleted
		FROM tasks WHERE uuid = ?
	`, uuid)

	var t things.Task
	var taskType, status, schedule int
	var scheduledDate, deadlineDate, completionDate, creationDate, modificationDate sql.NullInt64
	var idx, todayIdx int
	var inTrash, deleted int
	var areaUUID, projectUUID, headingUUID sql.NullString
	var alarmOffset sql.NullInt64

	err := row.Scan(
		&t.UUID, &taskType, &t.Title, &t.Note, &status, &schedule,
		&scheduledDate, &deadlineDate, &completionDate,
		&creationDate, &modificationDate, &idx, &todayIdx,
		&inTrash, &areaUUID, &projectUUID, &headingUUID,
		&alarmOffset, &deleted,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.Type = things.TaskType(taskType)
	t.Status = things.TaskStatus(status)
	t.Schedule = things.TaskSchedule(schedule)
	t.Index = idx
	t.TodayIndex = todayIdx
	t.InTrash = inTrash == 1

	if scheduledDate.Valid {
		tm := time.Unix(scheduledDate.Int64, 0)
		t.ScheduledDate = &tm
	}
	if deadlineDate.Valid {
		tm := time.Unix(deadlineDate.Int64, 0)
		t.DeadlineDate = &tm
	}
	if completionDate.Valid {
		tm := time.Unix(completionDate.Int64, 0)
		t.CompletionDate = &tm
	}
	if creationDate.Valid {
		t.CreationDate = time.Unix(creationDate.Int64, 0)
	}
	if modificationDate.Valid {
		tm := time.Unix(modificationDate.Int64, 0)
		t.ModificationDate = &tm
	}
	if areaUUID.Valid {
		t.AreaIDs = []string{areaUUID.String}
	}
	if projectUUID.Valid {
		t.ParentTaskIDs = []string{projectUUID.String}
	}
	if headingUUID.Valid {
		t.ActionGroupIDs = []string{headingUUID.String}
	}
	if alarmOffset.Valid {
		offset := int(alarmOffset.Int64)
		t.AlarmTimeOffset = &offset
	}

	// Load tags
	rows, err := s.db.Query("SELECT tag_uuid FROM task_tags WHERE task_uuid = ?", uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tagUUID string
		if err := rows.Scan(&tagUUID); err != nil {
			return nil, err
		}
		t.TagIDs = append(t.TagIDs, tagUUID)
	}

	return &t, nil
}

// saveTask inserts or updates a task in the database
func (s *Syncer) saveTask(t *things.Task) error {
	var scheduledDate, deadlineDate, completionDate, creationDate, modificationDate sql.NullInt64
	var areaUUID, projectUUID, headingUUID sql.NullString
	var alarmOffset sql.NullInt64

	if t.ScheduledDate != nil {
		scheduledDate = sql.NullInt64{Int64: t.ScheduledDate.Unix(), Valid: true}
	}
	if t.DeadlineDate != nil {
		deadlineDate = sql.NullInt64{Int64: t.DeadlineDate.Unix(), Valid: true}
	}
	if t.CompletionDate != nil {
		completionDate = sql.NullInt64{Int64: t.CompletionDate.Unix(), Valid: true}
	}
	if !t.CreationDate.IsZero() {
		creationDate = sql.NullInt64{Int64: t.CreationDate.Unix(), Valid: true}
	}
	if t.ModificationDate != nil {
		modificationDate = sql.NullInt64{Int64: t.ModificationDate.Unix(), Valid: true}
	}
	if len(t.AreaIDs) > 0 {
		areaUUID = sql.NullString{String: t.AreaIDs[0], Valid: true}
	}
	if len(t.ParentTaskIDs) > 0 {
		projectUUID = sql.NullString{String: t.ParentTaskIDs[0], Valid: true}
	}
	if len(t.ActionGroupIDs) > 0 {
		headingUUID = sql.NullString{String: t.ActionGroupIDs[0], Valid: true}
	}
	if t.AlarmTimeOffset != nil {
		alarmOffset = sql.NullInt64{Int64: int64(*t.AlarmTimeOffset), Valid: true}
	}

	inTrash := 0
	if t.InTrash {
		inTrash = 1
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO tasks (
			uuid, type, title, note, status, schedule,
			scheduled_date, deadline_date, completion_date,
			creation_date, modification_date, "index", today_index,
			in_trash, area_uuid, project_uuid, heading_uuid,
			alarm_time_offset, deleted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
	`, t.UUID, int(t.Type), t.Title, t.Note, int(t.Status), int(t.Schedule),
		scheduledDate, deadlineDate, completionDate,
		creationDate, modificationDate, t.Index, t.TodayIndex,
		inTrash, areaUUID, projectUUID, headingUUID,
		alarmOffset,
	)
	if err != nil {
		return err
	}

	// Update tags
	_, err = s.db.Exec("DELETE FROM task_tags WHERE task_uuid = ?", t.UUID)
	if err != nil {
		return err
	}
	for _, tagUUID := range t.TagIDs {
		_, err = s.db.Exec("INSERT INTO task_tags (task_uuid, tag_uuid) VALUES (?, ?)", t.UUID, tagUUID)
		if err != nil {
			return err
		}
	}

	return nil
}

// markTaskDeleted soft-deletes a task
func (s *Syncer) markTaskDeleted(uuid string) error {
	_, err := s.db.Exec("UPDATE tasks SET deleted = 1 WHERE uuid = ?", uuid)
	return err
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/store.go
git commit -m "feat(sync): add task storage functions"
```

---

### Task 7: Add area, tag, checklist storage functions

**Files:**
- Modify: `sync/store.go`

**Step 1: Add remaining entity storage**

Append to `sync/store.go`:
```go

// --- Area storage ---

func (s *Syncer) getArea(uuid string) (*things.Area, error) {
	row := s.db.QueryRow("SELECT uuid, title FROM areas WHERE uuid = ? AND deleted = 0", uuid)

	var a things.Area
	err := row.Scan(&a.UUID, &a.Title)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Syncer) saveArea(a *things.Area) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO areas (uuid, title, deleted)
		VALUES (?, ?, 0)
	`, a.UUID, a.Title)
	return err
}

func (s *Syncer) markAreaDeleted(uuid string) error {
	_, err := s.db.Exec("UPDATE areas SET deleted = 1 WHERE uuid = ?", uuid)
	return err
}

// --- Tag storage ---

func (s *Syncer) getTag(uuid string) (*things.Tag, error) {
	row := s.db.QueryRow("SELECT uuid, title, shortcut, parent_uuid FROM tags WHERE uuid = ? AND deleted = 0", uuid)

	var t things.Tag
	var parentUUID sql.NullString
	err := row.Scan(&t.UUID, &t.Title, &t.ShortHand, &parentUUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parentUUID.Valid {
		t.ParentTagIDs = []string{parentUUID.String}
	}
	return &t, nil
}

func (s *Syncer) saveTag(t *things.Tag) error {
	var parentUUID sql.NullString
	if len(t.ParentTagIDs) > 0 {
		parentUUID = sql.NullString{String: t.ParentTagIDs[0], Valid: true}
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO tags (uuid, title, shortcut, parent_uuid, deleted)
		VALUES (?, ?, ?, ?, 0)
	`, t.UUID, t.Title, t.ShortHand, parentUUID)
	return err
}

func (s *Syncer) markTagDeleted(uuid string) error {
	_, err := s.db.Exec("UPDATE tags SET deleted = 1 WHERE uuid = ?", uuid)
	return err
}

// --- ChecklistItem storage ---

func (s *Syncer) getChecklistItem(uuid string) (*things.CheckListItem, error) {
	row := s.db.QueryRow(`
		SELECT uuid, task_uuid, title, status, "index", creation_date, completion_date
		FROM checklist_items WHERE uuid = ? AND deleted = 0
	`, uuid)

	var c things.CheckListItem
	var taskUUID sql.NullString
	var status, idx int
	var creationDate, completionDate sql.NullInt64

	err := row.Scan(&c.UUID, &taskUUID, &c.Title, &status, &idx, &creationDate, &completionDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.Status = things.TaskStatus(status)
	c.Index = idx
	if taskUUID.Valid {
		c.TaskIDs = []string{taskUUID.String}
	}
	if creationDate.Valid {
		c.CreationDate = time.Unix(creationDate.Int64, 0)
	}
	if completionDate.Valid {
		tm := time.Unix(completionDate.Int64, 0)
		c.CompletionDate = &tm
	}

	return &c, nil
}

func (s *Syncer) saveChecklistItem(c *things.CheckListItem) error {
	var taskUUID sql.NullString
	var creationDate, completionDate sql.NullInt64

	if len(c.TaskIDs) > 0 {
		taskUUID = sql.NullString{String: c.TaskIDs[0], Valid: true}
	}
	if !c.CreationDate.IsZero() {
		creationDate = sql.NullInt64{Int64: c.CreationDate.Unix(), Valid: true}
	}
	if c.CompletionDate != nil {
		completionDate = sql.NullInt64{Int64: c.CompletionDate.Unix(), Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO checklist_items (uuid, task_uuid, title, status, "index", creation_date, completion_date, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
	`, c.UUID, taskUUID, c.Title, int(c.Status), c.Index, creationDate, completionDate)
	return err
}

func (s *Syncer) markChecklistItemDeleted(uuid string) error {
	_, err := s.db.Exec("UPDATE checklist_items SET deleted = 1 WHERE uuid = ?", uuid)
	return err
}

// --- Sync state storage ---

func (s *Syncer) getSyncState() (historyID string, serverIndex int, err error) {
	row := s.db.QueryRow("SELECT history_id, server_index FROM sync_state WHERE id = 1")
	err = row.Scan(&historyID, &serverIndex)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return
}

func (s *Syncer) saveSyncState(historyID string, serverIndex int) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO sync_state (id, history_id, server_index, last_sync_at)
		VALUES (1, ?, ?, ?)
	`, historyID, serverIndex, time.Now().Unix())
	return err
}

// --- Change log storage ---

func (s *Syncer) logChange(serverIndex int, change Change, payload string) error {
	_, err := s.db.Exec(`
		INSERT INTO change_log (server_index, synced_at, change_type, entity_type, entity_uuid, payload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, serverIndex, time.Now().Unix(), change.ChangeType(), change.EntityType(), change.EntityUUID(), payload)
	return err
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/store.go
git commit -m "feat(sync): add area, tag, checklist, sync state storage functions"
```

---

### Task 8: Write tests for storage functions

**Files:**
- Create: `sync/store_test.go`

**Step 1: Write storage tests**

Create `sync/store_test.go`:
```go
package sync

import (
	"path/filepath"
	"testing"
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

func TestTaskStorage(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	t.Run("save and retrieve task", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		task := &things.Task{
			UUID:         "task-123",
			Title:        "Test Task",
			Note:         "Some notes",
			Status:       things.TaskStatusPending,
			Schedule:     things.TaskScheduleAnytime,
			Type:         things.TaskTypeTask,
			CreationDate: now,
			TagIDs:       []string{"tag-1", "tag-2"},
		}

		if err := syncer.saveTask(task); err != nil {
			t.Fatalf("saveTask failed: %v", err)
		}

		retrieved, err := syncer.getTask("task-123")
		if err != nil {
			t.Fatalf("getTask failed: %v", err)
		}
		if retrieved == nil {
			t.Fatal("task not found")
		}
		if retrieved.Title != "Test Task" {
			t.Errorf("Title mismatch: got %q", retrieved.Title)
		}
		if retrieved.Note != "Some notes" {
			t.Errorf("Note mismatch: got %q", retrieved.Note)
		}
		if len(retrieved.TagIDs) != 2 {
			t.Errorf("TagIDs mismatch: got %v", retrieved.TagIDs)
		}
	})

	t.Run("get non-existent task returns nil", func(t *testing.T) {
		retrieved, err := syncer.getTask("non-existent")
		if err != nil {
			t.Fatalf("getTask failed: %v", err)
		}
		if retrieved != nil {
			t.Error("expected nil for non-existent task")
		}
	})

	t.Run("soft delete task", func(t *testing.T) {
		task := &things.Task{UUID: "to-delete", Title: "Delete Me"}
		syncer.saveTask(task)

		if err := syncer.markTaskDeleted("to-delete"); err != nil {
			t.Fatalf("markTaskDeleted failed: %v", err)
		}

		// Task still exists but is marked deleted
		var deleted int
		syncer.db.QueryRow("SELECT deleted FROM tasks WHERE uuid = 'to-delete'").Scan(&deleted)
		if deleted != 1 {
			t.Error("task not marked as deleted")
		}
	})
}

func TestAreaStorage(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	area := &things.Area{UUID: "area-123", Title: "Work"}
	if err := syncer.saveArea(area); err != nil {
		t.Fatalf("saveArea failed: %v", err)
	}

	retrieved, err := syncer.getArea("area-123")
	if err != nil {
		t.Fatalf("getArea failed: %v", err)
	}
	if retrieved.Title != "Work" {
		t.Errorf("Title mismatch: got %q", retrieved.Title)
	}
}

func TestTagStorage(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	tag := &things.Tag{UUID: "tag-123", Title: "Urgent", ShortHand: "u"}
	if err := syncer.saveTag(tag); err != nil {
		t.Fatalf("saveTag failed: %v", err)
	}

	retrieved, err := syncer.getTag("tag-123")
	if err != nil {
		t.Fatalf("getTag failed: %v", err)
	}
	if retrieved.Title != "Urgent" {
		t.Errorf("Title mismatch: got %q", retrieved.Title)
	}
	if retrieved.ShortHand != "u" {
		t.Errorf("ShortHand mismatch: got %q", retrieved.ShortHand)
	}
}

func TestSyncStateStorage(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	if err := syncer.saveSyncState("history-abc", 42); err != nil {
		t.Fatalf("saveSyncState failed: %v", err)
	}

	historyID, serverIndex, err := syncer.getSyncState()
	if err != nil {
		t.Fatalf("getSyncState failed: %v", err)
	}
	if historyID != "history-abc" {
		t.Errorf("historyID mismatch: got %q", historyID)
	}
	if serverIndex != 42 {
		t.Errorf("serverIndex mismatch: got %d", serverIndex)
	}
}
```

**Step 2: Run tests**

Run:
```bash
go test -v ./sync/...
```
Expected: All tests PASS

**Step 3: Commit**

```bash
git add sync/store_test.go
git commit -m "test(sync): add storage function tests"
```

---

## Phase 4: Change Detection

### Task 9: Add change detection for tasks

**Files:**
- Create: `sync/detect.go`

**Step 1: Create change detection logic**

Create `sync/detect.go`:
```go
package sync

import (
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

// detectTaskChanges compares old and new task state and returns semantic changes
func detectTaskChanges(old, new *things.Task, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	// Created
	if old == nil && new != nil {
		switch new.Type {
		case things.TaskTypeProject:
			changes = append(changes, ProjectCreated{baseChange: base, Project: new})
		case things.TaskTypeHeading:
			changes = append(changes, HeadingCreated{baseChange: base, Heading: new})
		default:
			changes = append(changes, TaskCreated{baseChange: base, Task: new})
		}
		return changes
	}

	// Deleted
	if old != nil && new == nil {
		switch old.Type {
		case things.TaskTypeProject:
			changes = append(changes, ProjectDeleted{baseChange: base, Project: old})
		case things.TaskTypeHeading:
			changes = append(changes, HeadingDeleted{baseChange: base, Heading: old})
		default:
			changes = append(changes, TaskDeleted{baseChange: base, Task: old})
		}
		return changes
	}

	// Modified - detect specific changes
	if old == nil || new == nil {
		return changes
	}

	// Title changed
	if old.Title != new.Title {
		switch new.Type {
		case things.TaskTypeProject:
			changes = append(changes, ProjectTitleChanged{baseChange: base, Project: new, OldTitle: old.Title})
		case things.TaskTypeHeading:
			changes = append(changes, HeadingTitleChanged{baseChange: base, Heading: new, OldTitle: old.Title})
		default:
			changes = append(changes, TaskTitleChanged{baseChange: base, Task: new, OldTitle: old.Title})
		}
	}

	// Note changed
	if old.Note != new.Note && new.Type != things.TaskTypeHeading {
		changes = append(changes, TaskNoteChanged{baseChange: base, Task: new, OldNote: old.Note})
	}

	// Status changed
	if old.Status != new.Status {
		switch {
		case new.Status == things.TaskStatusCompleted:
			if new.Type == things.TaskTypeProject {
				changes = append(changes, ProjectCompleted{baseChange: base, Project: new})
			} else {
				changes = append(changes, TaskCompleted{baseChange: base, Task: new})
			}
		case new.Status == things.TaskStatusCanceled:
			changes = append(changes, TaskCanceled{baseChange: base, Task: new})
		case old.Status == things.TaskStatusCompleted && new.Status == things.TaskStatusPending:
			changes = append(changes, TaskUncompleted{baseChange: base, Task: new})
		}
	}

	// Trash changed
	if old.InTrash != new.InTrash {
		if new.InTrash {
			if new.Type == things.TaskTypeProject {
				changes = append(changes, ProjectTrashed{baseChange: base, Project: new})
			} else {
				changes = append(changes, TaskTrashed{baseChange: base, Task: new})
			}
		} else {
			if new.Type == things.TaskTypeProject {
				changes = append(changes, ProjectRestored{baseChange: base, Project: new})
			} else {
				changes = append(changes, TaskRestored{baseChange: base, Task: new})
			}
		}
	}

	// Schedule/location changed (only for regular tasks)
	if new.Type == things.TaskTypeTask {
		oldLoc := taskLocation(old)
		newLoc := taskLocation(new)
		if oldLoc != newLoc {
			switch newLoc {
			case LocationInbox:
				changes = append(changes, TaskMovedToInbox{baseChange: base, Task: new, From: oldLoc})
			case LocationToday:
				changes = append(changes, TaskMovedToToday{baseChange: base, Task: new, From: oldLoc})
			case LocationAnytime:
				changes = append(changes, TaskMovedToAnytime{baseChange: base, Task: new, From: oldLoc})
			case LocationSomeday:
				changes = append(changes, TaskMovedToSomeday{baseChange: base, Task: new, From: oldLoc})
			case LocationUpcoming:
				var scheduledFor time.Time
				if new.ScheduledDate != nil {
					scheduledFor = *new.ScheduledDate
				}
				changes = append(changes, TaskMovedToUpcoming{baseChange: base, Task: new, From: oldLoc, ScheduledFor: scheduledFor})
			}
		}
	}

	// Deadline changed
	if !timeEqual(old.DeadlineDate, new.DeadlineDate) {
		changes = append(changes, TaskDeadlineChanged{baseChange: base, Task: new, OldDeadline: old.DeadlineDate})
	}

	// Tags changed
	added, removed := diffStringSlices(old.TagIDs, new.TagIDs)
	if len(added) > 0 || len(removed) > 0 {
		changes = append(changes, TaskTagsChanged{baseChange: base, Task: new, Added: added, Removed: removed})
	}

	return changes
}

// taskLocation determines where a task lives based on schedule and dates
func taskLocation(t *things.Task) TaskLocation {
	if t == nil {
		return LocationUnknown
	}

	switch t.Schedule {
	case things.TaskScheduleInbox:
		return LocationInbox
	case things.TaskScheduleAnytime:
		if t.ScheduledDate != nil && isToday(t.ScheduledDate) {
			return LocationToday
		}
		return LocationAnytime
	case things.TaskScheduleSomeday:
		if t.ScheduledDate != nil && t.ScheduledDate.After(time.Now()) {
			return LocationUpcoming
		}
		return LocationSomeday
	}
	return LocationUnknown
}

func isToday(t *time.Time) bool {
	if t == nil {
		return false
	}
	now := time.Now()
	return t.Year() == now.Year() && t.YearDay() == now.YearDay()
}

func timeEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func diffStringSlices(old, new []string) (added, removed []string) {
	oldSet := make(map[string]bool)
	newSet := make(map[string]bool)
	for _, s := range old {
		oldSet[s] = true
	}
	for _, s := range new {
		newSet[s] = true
	}
	for _, s := range new {
		if !oldSet[s] {
			added = append(added, s)
		}
	}
	for _, s := range old {
		if !newSet[s] {
			removed = append(removed, s)
		}
	}
	return
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/detect.go
git commit -m "feat(sync): add task change detection"
```

---

### Task 10: Add change detection for areas, tags, checklists

**Files:**
- Modify: `sync/detect.go`

**Step 1: Add remaining change detection**

Append to `sync/detect.go`:
```go

// detectAreaChanges compares old and new area state
func detectAreaChanges(old, new *things.Area, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	if old == nil && new != nil {
		changes = append(changes, AreaCreated{baseChange: base, Area: new})
		return changes
	}

	if old != nil && new == nil {
		changes = append(changes, AreaDeleted{baseChange: base, Area: old})
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	if old.Title != new.Title {
		changes = append(changes, AreaRenamed{baseChange: base, Area: new, OldTitle: old.Title})
	}

	return changes
}

// detectTagChanges compares old and new tag state
func detectTagChanges(old, new *things.Tag, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	if old == nil && new != nil {
		changes = append(changes, TagCreated{baseChange: base, Tag: new})
		return changes
	}

	if old != nil && new == nil {
		changes = append(changes, TagDeleted{baseChange: base, Tag: old})
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	if old.Title != new.Title {
		changes = append(changes, TagRenamed{baseChange: base, Tag: new, OldTitle: old.Title})
	}

	if old.ShortHand != new.ShortHand {
		changes = append(changes, TagShortcutChanged{baseChange: base, Tag: new, OldShortcut: old.ShortHand})
	}

	return changes
}

// detectChecklistChanges compares old and new checklist item state
func detectChecklistChanges(old, new *things.CheckListItem, task *things.Task, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	if old == nil && new != nil {
		changes = append(changes, ChecklistItemCreated{baseChange: base, Item: new, Task: task})
		return changes
	}

	if old != nil && new == nil {
		changes = append(changes, ChecklistItemDeleted{baseChange: base, Item: old})
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	if old.Title != new.Title {
		changes = append(changes, ChecklistItemTitleChanged{baseChange: base, Item: new, OldTitle: old.Title})
	}

	if old.Status != new.Status {
		if new.Status == things.TaskStatusCompleted {
			changes = append(changes, ChecklistItemCompleted{baseChange: base, Item: new, Task: task})
		} else if old.Status == things.TaskStatusCompleted {
			changes = append(changes, ChecklistItemUncompleted{baseChange: base, Item: new, Task: task})
		}
	}

	return changes
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/detect.go
git commit -m "feat(sync): add area, tag, checklist change detection"
```

---

### Task 11: Write tests for change detection

**Files:**
- Create: `sync/detect_test.go`

**Step 1: Write detection tests**

Create `sync/detect_test.go`:
```go
package sync

import (
	"testing"
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

func TestDetectTaskChanges(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("task created", func(t *testing.T) {
		t.Parallel()
		task := &things.Task{UUID: "t1", Title: "New Task", Type: things.TaskTypeTask}
		changes := detectTaskChanges(nil, task, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		if _, ok := changes[0].(TaskCreated); !ok {
			t.Errorf("expected TaskCreated, got %T", changes[0])
		}
	})

	t.Run("project created", func(t *testing.T) {
		t.Parallel()
		project := &things.Task{UUID: "p1", Title: "New Project", Type: things.TaskTypeProject}
		changes := detectTaskChanges(nil, project, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		if _, ok := changes[0].(ProjectCreated); !ok {
			t.Errorf("expected ProjectCreated, got %T", changes[0])
		}
	})

	t.Run("task completed", func(t *testing.T) {
		t.Parallel()
		old := &things.Task{UUID: "t1", Title: "Task", Status: things.TaskStatusPending}
		new := &things.Task{UUID: "t1", Title: "Task", Status: things.TaskStatusCompleted}
		changes := detectTaskChanges(old, new, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		if _, ok := changes[0].(TaskCompleted); !ok {
			t.Errorf("expected TaskCompleted, got %T", changes[0])
		}
	})

	t.Run("task uncompleted", func(t *testing.T) {
		t.Parallel()
		old := &things.Task{UUID: "t1", Title: "Task", Status: things.TaskStatusCompleted}
		new := &things.Task{UUID: "t1", Title: "Task", Status: things.TaskStatusPending}
		changes := detectTaskChanges(old, new, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		if _, ok := changes[0].(TaskUncompleted); !ok {
			t.Errorf("expected TaskUncompleted, got %T", changes[0])
		}
	})

	t.Run("task title changed", func(t *testing.T) {
		t.Parallel()
		old := &things.Task{UUID: "t1", Title: "Old Title"}
		new := &things.Task{UUID: "t1", Title: "New Title"}
		changes := detectTaskChanges(old, new, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		tc, ok := changes[0].(TaskTitleChanged)
		if !ok {
			t.Fatalf("expected TaskTitleChanged, got %T", changes[0])
		}
		if tc.OldTitle != "Old Title" {
			t.Errorf("expected OldTitle 'Old Title', got %q", tc.OldTitle)
		}
	})

	t.Run("task trashed", func(t *testing.T) {
		t.Parallel()
		old := &things.Task{UUID: "t1", Title: "Task", InTrash: false}
		new := &things.Task{UUID: "t1", Title: "Task", InTrash: true}
		changes := detectTaskChanges(old, new, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		if _, ok := changes[0].(TaskTrashed); !ok {
			t.Errorf("expected TaskTrashed, got %T", changes[0])
		}
	})

	t.Run("task moved to today", func(t *testing.T) {
		t.Parallel()
		today := time.Now()
		old := &things.Task{UUID: "t1", Title: "Task", Schedule: things.TaskScheduleInbox}
		new := &things.Task{UUID: "t1", Title: "Task", Schedule: things.TaskScheduleAnytime, ScheduledDate: &today}
		changes := detectTaskChanges(old, new, 1, now)

		found := false
		for _, c := range changes {
			if _, ok := c.(TaskMovedToToday); ok {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected TaskMovedToToday change")
		}
	})

	t.Run("multiple changes at once", func(t *testing.T) {
		t.Parallel()
		old := &things.Task{UUID: "t1", Title: "Old", Status: things.TaskStatusPending, InTrash: false}
		new := &things.Task{UUID: "t1", Title: "New", Status: things.TaskStatusCompleted, InTrash: true}
		changes := detectTaskChanges(old, new, 1, now)

		// Should have: title changed, completed, trashed
		if len(changes) != 3 {
			t.Fatalf("expected 3 changes, got %d: %v", len(changes), changes)
		}
	})

	t.Run("tags changed", func(t *testing.T) {
		t.Parallel()
		old := &things.Task{UUID: "t1", Title: "Task", TagIDs: []string{"tag1", "tag2"}}
		new := &things.Task{UUID: "t1", Title: "Task", TagIDs: []string{"tag2", "tag3"}}
		changes := detectTaskChanges(old, new, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		tc, ok := changes[0].(TaskTagsChanged)
		if !ok {
			t.Fatalf("expected TaskTagsChanged, got %T", changes[0])
		}
		if len(tc.Added) != 1 || tc.Added[0] != "tag3" {
			t.Errorf("expected Added ['tag3'], got %v", tc.Added)
		}
		if len(tc.Removed) != 1 || tc.Removed[0] != "tag1" {
			t.Errorf("expected Removed ['tag1'], got %v", tc.Removed)
		}
	})
}

func TestDetectAreaChanges(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("area created", func(t *testing.T) {
		t.Parallel()
		area := &things.Area{UUID: "a1", Title: "Work"}
		changes := detectAreaChanges(nil, area, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		if _, ok := changes[0].(AreaCreated); !ok {
			t.Errorf("expected AreaCreated, got %T", changes[0])
		}
	})

	t.Run("area renamed", func(t *testing.T) {
		t.Parallel()
		old := &things.Area{UUID: "a1", Title: "Work"}
		new := &things.Area{UUID: "a1", Title: "Office"}
		changes := detectAreaChanges(old, new, 1, now)

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}
		ar, ok := changes[0].(AreaRenamed)
		if !ok {
			t.Fatalf("expected AreaRenamed, got %T", changes[0])
		}
		if ar.OldTitle != "Work" {
			t.Errorf("expected OldTitle 'Work', got %q", ar.OldTitle)
		}
	})
}
```

**Step 2: Run tests**

Run:
```bash
go test -v ./sync/...
```
Expected: All tests PASS

**Step 3: Commit**

```bash
git add sync/detect_test.go
git commit -m "test(sync): add change detection tests"
```

---

## Phase 5: Sync Engine Core

### Task 12: Implement the Sync() method

**Files:**
- Modify: `sync/sync.go`

**Step 1: Add Sync method**

Add to `sync/sync.go` (after the Close method):
```go

// Sync fetches new items from Things Cloud, updates local state,
// and returns the list of changes in order
func (s *Syncer) Sync() ([]Change, error) {
	// Ensure we have a history
	if s.history == nil {
		h, err := s.client.OwnHistory()
		if err != nil {
			return nil, err
		}
		s.history = h
	}

	// Get current sync state
	storedHistoryID, startIndex, err := s.getSyncState()
	if err != nil {
		return nil, err
	}

	// If history changed, start fresh
	if storedHistoryID != "" && storedHistoryID != s.history.ID {
		startIndex = 0
	}

	// Fetch items from server
	var allChanges []Change
	hasMore := true

	for hasMore {
		items, more, err := s.history.Items(things.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			return nil, err
		}
		hasMore = more

		// Process each item
		changes, err := s.processItems(items, startIndex)
		if err != nil {
			return nil, err
		}
		allChanges = append(allChanges, changes...)

		startIndex = s.history.LoadedServerIndex
	}

	// Save sync state
	if err := s.saveSyncState(s.history.ID, s.history.LatestServerIndex); err != nil {
		return nil, err
	}

	return allChanges, nil
}

// LastSyncedIndex returns the server index we've synced up to
func (s *Syncer) LastSyncedIndex() int {
	_, idx, _ := s.getSyncState()
	return idx
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors (will have undefined processItems, we'll add it next)

**Step 3: Commit**

```bash
git add sync/sync.go
git commit -m "feat(sync): add Sync() method skeleton"
```

---

### Task 13: Implement item processing

**Files:**
- Create: `sync/process.go`

**Step 1: Create item processing logic**

Create `sync/process.go`:
```go
package sync

import (
	"encoding/json"
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

// processItems processes a batch of items and returns detected changes
func (s *Syncer) processItems(items []things.Item, baseIndex int) ([]Change, error) {
	var changes []Change
	ts := time.Now()

	for i, item := range items {
		serverIndex := baseIndex + i

		itemChanges, err := s.processItem(item, serverIndex, ts)
		if err != nil {
			// Log but continue processing
			continue
		}
		changes = append(changes, itemChanges...)
	}

	return changes, nil
}

func (s *Syncer) processItem(item things.Item, serverIndex int, ts time.Time) ([]Change, error) {
	switch item.Kind {
	case things.ItemKindTask, things.ItemKindTask4, things.ItemKindTask3, things.ItemKindTaskPlain:
		return s.processTaskItem(item, serverIndex, ts)
	case things.ItemKindArea, things.ItemKindArea3, things.ItemKindAreaPlain:
		return s.processAreaItem(item, serverIndex, ts)
	case things.ItemKindTag, things.ItemKindTag4, things.ItemKindTagPlain:
		return s.processTagItem(item, serverIndex, ts)
	case things.ItemKindChecklistItem, things.ItemKindChecklistItem2, things.ItemKindChecklistItem3:
		return s.processChecklistItem(item, serverIndex, ts)
	case things.ItemKindTombstone:
		return s.processTombstone(item, serverIndex, ts)
	default:
		return nil, nil
	}
}

func (s *Syncer) processTaskItem(item things.Item, serverIndex int, ts time.Time) ([]Change, error) {
	var payload things.TaskActionItemPayload
	if err := json.Unmarshal(item.P, &payload); err != nil {
		return nil, err
	}

	// Get current state from DB
	old, err := s.getTask(item.UUID)
	if err != nil {
		return nil, err
	}

	// Handle deletion
	if item.Action == things.ItemActionDeleted {
		if old != nil {
			if err := s.markTaskDeleted(item.UUID); err != nil {
				return nil, err
			}
			return detectTaskChanges(old, nil, serverIndex, ts), nil
		}
		return nil, nil
	}

	// Build new state by applying payload to old state
	new := applyTaskPayload(old, item.UUID, payload)

	// Detect changes
	changes := detectTaskChanges(old, new, serverIndex, ts)

	// Save new state
	if err := s.saveTask(new); err != nil {
		return nil, err
	}

	// Log changes
	for _, c := range changes {
		s.logChange(serverIndex, c, "")
	}

	return changes, nil
}

func applyTaskPayload(old *things.Task, uuid string, p things.TaskActionItemPayload) *things.Task {
	t := &things.Task{
		UUID:     uuid,
		Schedule: things.TaskScheduleAnytime,
	}
	if old != nil {
		*t = *old // copy existing state
	}

	if p.Title != nil {
		t.Title = *p.Title
	}
	if p.Type != nil {
		t.Type = *p.Type
	}
	if p.Status != nil {
		t.Status = *p.Status
	}
	if p.Index != nil {
		t.Index = *p.Index
	}
	if p.InTrash != nil {
		t.InTrash = *p.InTrash
	}
	if p.Schedule != nil {
		t.Schedule = *p.Schedule
	}
	if p.ScheduledDate != nil {
		t.ScheduledDate = p.ScheduledDate.Time()
	}
	if p.CompletionDate != nil {
		t.CompletionDate = p.CompletionDate.Time()
	}
	if p.DeadlineDate != nil {
		t.DeadlineDate = p.DeadlineDate.Time()
	}
	if p.CreationDate != nil {
		cd := p.CreationDate.Time()
		if cd != nil {
			t.CreationDate = *cd
		}
	}
	if p.ModificationDate != nil {
		t.ModificationDate = p.ModificationDate.Time()
	}
	if p.AreaIDs != nil {
		t.AreaIDs = *p.AreaIDs
	}
	if p.ParentTaskIDs != nil {
		t.ParentTaskIDs = *p.ParentTaskIDs
	}
	if p.ActionGroupIDs != nil {
		t.ActionGroupIDs = *p.ActionGroupIDs
	}
	if p.TagIDs != nil {
		t.TagIDs = p.TagIDs
	}
	if p.TaskIndex != nil {
		t.TodayIndex = *p.TaskIndex
	}
	if p.AlarmTimeOffset != nil {
		t.AlarmTimeOffset = p.AlarmTimeOffset
	}
	if p.Note != nil {
		var noteStr string
		if err := json.Unmarshal(p.Note, &noteStr); err == nil {
			t.Note = noteStr
		} else {
			var note things.Note
			if err := json.Unmarshal(p.Note, &note); err == nil {
				switch note.Type {
				case things.NoteTypeFullText:
					t.Note = note.Value
				case things.NoteTypeDelta:
					t.Note = things.ApplyPatches(t.Note, note.Patches)
				}
			}
		}
	}

	return t
}

func (s *Syncer) processAreaItem(item things.Item, serverIndex int, ts time.Time) ([]Change, error) {
	var payload things.AreaActionItemPayload
	if err := json.Unmarshal(item.P, &payload); err != nil {
		return nil, err
	}

	old, err := s.getArea(item.UUID)
	if err != nil {
		return nil, err
	}

	if item.Action == things.ItemActionDeleted {
		if old != nil {
			if err := s.markAreaDeleted(item.UUID); err != nil {
				return nil, err
			}
			return detectAreaChanges(old, nil, serverIndex, ts), nil
		}
		return nil, nil
	}

	new := &things.Area{UUID: item.UUID}
	if old != nil {
		*new = *old
	}
	if payload.Title != nil {
		new.Title = *payload.Title
	}

	changes := detectAreaChanges(old, new, serverIndex, ts)

	if err := s.saveArea(new); err != nil {
		return nil, err
	}

	for _, c := range changes {
		s.logChange(serverIndex, c, "")
	}

	return changes, nil
}

func (s *Syncer) processTagItem(item things.Item, serverIndex int, ts time.Time) ([]Change, error) {
	var payload things.TagActionItemPayload
	if err := json.Unmarshal(item.P, &payload); err != nil {
		return nil, err
	}

	old, err := s.getTag(item.UUID)
	if err != nil {
		return nil, err
	}

	if item.Action == things.ItemActionDeleted {
		if old != nil {
			if err := s.markTagDeleted(item.UUID); err != nil {
				return nil, err
			}
			return detectTagChanges(old, nil, serverIndex, ts), nil
		}
		return nil, nil
	}

	new := &things.Tag{UUID: item.UUID}
	if old != nil {
		*new = *old
	}
	if payload.Title != nil {
		new.Title = *payload.Title
	}
	if payload.ShortHand != nil {
		new.ShortHand = *payload.ShortHand
	}
	if payload.ParentTagIDs != nil {
		new.ParentTagIDs = *payload.ParentTagIDs
	}

	changes := detectTagChanges(old, new, serverIndex, ts)

	if err := s.saveTag(new); err != nil {
		return nil, err
	}

	for _, c := range changes {
		s.logChange(serverIndex, c, "")
	}

	return changes, nil
}

func (s *Syncer) processChecklistItem(item things.Item, serverIndex int, ts time.Time) ([]Change, error) {
	var payload things.CheckListActionItemPayload
	if err := json.Unmarshal(item.P, &payload); err != nil {
		return nil, err
	}

	old, err := s.getChecklistItem(item.UUID)
	if err != nil {
		return nil, err
	}

	if item.Action == things.ItemActionDeleted {
		if old != nil {
			if err := s.markChecklistItemDeleted(item.UUID); err != nil {
				return nil, err
			}
			return detectChecklistChanges(old, nil, nil, serverIndex, ts), nil
		}
		return nil, nil
	}

	new := &things.CheckListItem{UUID: item.UUID}
	if old != nil {
		*new = *old
	}
	if payload.Title != nil {
		new.Title = *payload.Title
	}
	if payload.Status != nil {
		new.Status = *payload.Status
	}
	if payload.Index != nil {
		new.Index = *payload.Index
	}
	if payload.TaskIDs != nil {
		new.TaskIDs = *payload.TaskIDs
	}
	if payload.CreationDate != nil {
		cd := payload.CreationDate.Time()
		if cd != nil {
			new.CreationDate = *cd
		}
	}
	if payload.CompletionDate != nil {
		new.CompletionDate = payload.CompletionDate.Time()
	}

	// Get parent task for context
	var task *things.Task
	if len(new.TaskIDs) > 0 {
		task, _ = s.getTask(new.TaskIDs[0])
	}

	changes := detectChecklistChanges(old, new, task, serverIndex, ts)

	if err := s.saveChecklistItem(new); err != nil {
		return nil, err
	}

	for _, c := range changes {
		s.logChange(serverIndex, c, "")
	}

	return changes, nil
}

func (s *Syncer) processTombstone(item things.Item, serverIndex int, ts time.Time) ([]Change, error) {
	var payload things.TombstoneActionItemPayload
	if err := json.Unmarshal(item.P, &payload); err != nil {
		return nil, err
	}

	var changes []Change
	oid := payload.DeletedObjectID

	// Try each entity type
	if task, _ := s.getTask(oid); task != nil {
		s.markTaskDeleted(oid)
		changes = append(changes, detectTaskChanges(task, nil, serverIndex, ts)...)
	}
	if area, _ := s.getArea(oid); area != nil {
		s.markAreaDeleted(oid)
		changes = append(changes, detectAreaChanges(area, nil, serverIndex, ts)...)
	}
	if tag, _ := s.getTag(oid); tag != nil {
		s.markTagDeleted(oid)
		changes = append(changes, detectTagChanges(tag, nil, serverIndex, ts)...)
	}
	if item, _ := s.getChecklistItem(oid); item != nil {
		s.markChecklistItemDeleted(oid)
		changes = append(changes, detectChecklistChanges(item, nil, nil, serverIndex, ts)...)
	}

	for _, c := range changes {
		s.logChange(serverIndex, c, "")
	}

	return changes, nil
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/process.go
git commit -m "feat(sync): implement item processing and state updates"
```

---

## Phase 6: State Queries

### Task 14: Add State type with query methods

**Files:**
- Create: `sync/state.go`

**Step 1: Create State type**

Create `sync/state.go`:
```go
package sync

import (
	"database/sql"
	"time"

	things "github.com/nicolai86/things-cloud-sdk"
)

// State provides read-only access to the synced Things state
type State struct {
	db *sql.DB
}

// State returns a read-only view of the current aggregated state
func (s *Syncer) State() *State {
	return &State{db: s.db}
}

// QueryOpts controls filtering for state queries
type QueryOpts struct {
	IncludeCompleted bool
	IncludeTrashed   bool
}

// Task retrieves a task by UUID
func (st *State) Task(uuid string) (*things.Task, error) {
	return (&Syncer{db: st.db}).getTask(uuid)
}

// Area retrieves an area by UUID
func (st *State) Area(uuid string) (*things.Area, error) {
	return (&Syncer{db: st.db}).getArea(uuid)
}

// Tag retrieves a tag by UUID
func (st *State) Tag(uuid string) (*things.Tag, error) {
	return (&Syncer{db: st.db}).getTag(uuid)
}

// AllTasks returns all tasks matching the query options
func (st *State) AllTasks(opts QueryOpts) ([]*things.Task, error) {
	query := `SELECT uuid FROM tasks WHERE type = 0 AND deleted = 0`
	if !opts.IncludeCompleted {
		query += " AND status != 3"
	}
	if !opts.IncludeTrashed {
		query += " AND in_trash = 0"
	}
	query += ` ORDER BY "index"`

	return st.queryTasks(query)
}

// AllProjects returns all projects
func (st *State) AllProjects(opts QueryOpts) ([]*things.Task, error) {
	query := `SELECT uuid FROM tasks WHERE type = 1 AND deleted = 0`
	if !opts.IncludeCompleted {
		query += " AND status != 3"
	}
	if !opts.IncludeTrashed {
		query += " AND in_trash = 0"
	}
	query += ` ORDER BY "index"`

	return st.queryTasks(query)
}

// AllAreas returns all areas
func (st *State) AllAreas() ([]*things.Area, error) {
	rows, err := st.db.Query(`SELECT uuid, title FROM areas WHERE deleted = 0 ORDER BY "index"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []*things.Area
	for rows.Next() {
		var a things.Area
		if err := rows.Scan(&a.UUID, &a.Title); err != nil {
			return nil, err
		}
		areas = append(areas, &a)
	}
	return areas, nil
}

// AllTags returns all tags
func (st *State) AllTags() ([]*things.Tag, error) {
	rows, err := st.db.Query(`SELECT uuid, title, shortcut FROM tags WHERE deleted = 0 ORDER BY "index"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*things.Tag
	for rows.Next() {
		var t things.Tag
		if err := rows.Scan(&t.UUID, &t.Title, &t.ShortHand); err != nil {
			return nil, err
		}
		tags = append(tags, &t)
	}
	return tags, nil
}

// TasksInInbox returns tasks in the Inbox
func (st *State) TasksInInbox(opts QueryOpts) ([]*things.Task, error) {
	query := `SELECT uuid FROM tasks WHERE type = 0 AND schedule = 0 AND deleted = 0`
	if !opts.IncludeCompleted {
		query += " AND status != 3"
	}
	if !opts.IncludeTrashed {
		query += " AND in_trash = 0"
	}
	query += ` ORDER BY "index"`

	return st.queryTasks(query)
}

// TasksInToday returns tasks scheduled for today
func (st *State) TasksInToday(opts QueryOpts) ([]*things.Task, error) {
	today := time.Now().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	query := `SELECT uuid FROM tasks WHERE type = 0 AND schedule = 1
		AND scheduled_date >= ? AND scheduled_date < ? AND deleted = 0`
	if !opts.IncludeCompleted {
		query += " AND status != 3"
	}
	if !opts.IncludeTrashed {
		query += " AND in_trash = 0"
	}
	query += ` ORDER BY today_index, "index"`

	rows, err := st.db.Query(query, today.Unix(), tomorrow.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return st.scanTaskUUIDs(rows)
}

// TasksInProject returns tasks belonging to a project
func (st *State) TasksInProject(projectUUID string, opts QueryOpts) ([]*things.Task, error) {
	query := `SELECT uuid FROM tasks WHERE type = 0 AND project_uuid = ? AND deleted = 0`
	if !opts.IncludeCompleted {
		query += " AND status != 3"
	}
	if !opts.IncludeTrashed {
		query += " AND in_trash = 0"
	}
	query += ` ORDER BY "index"`

	rows, err := st.db.Query(query, projectUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return st.scanTaskUUIDs(rows)
}

// TasksInArea returns tasks belonging to an area
func (st *State) TasksInArea(areaUUID string, opts QueryOpts) ([]*things.Task, error) {
	query := `SELECT uuid FROM tasks WHERE type = 0 AND area_uuid = ? AND deleted = 0`
	if !opts.IncludeCompleted {
		query += " AND status != 3"
	}
	if !opts.IncludeTrashed {
		query += " AND in_trash = 0"
	}
	query += ` ORDER BY "index"`

	rows, err := st.db.Query(query, areaUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return st.scanTaskUUIDs(rows)
}

// ChecklistItems returns checklist items for a task
func (st *State) ChecklistItems(taskUUID string) ([]*things.CheckListItem, error) {
	rows, err := st.db.Query(`
		SELECT uuid, title, status, "index"
		FROM checklist_items
		WHERE task_uuid = ? AND deleted = 0
		ORDER BY "index"
	`, taskUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*things.CheckListItem
	for rows.Next() {
		var c things.CheckListItem
		var status int
		if err := rows.Scan(&c.UUID, &c.Title, &status, &c.Index); err != nil {
			return nil, err
		}
		c.Status = things.TaskStatus(status)
		c.TaskIDs = []string{taskUUID}
		items = append(items, &c)
	}
	return items, nil
}

// Helper methods

func (st *State) queryTasks(query string) ([]*things.Task, error) {
	rows, err := st.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return st.scanTaskUUIDs(rows)
}

func (st *State) scanTaskUUIDs(rows *sql.Rows) ([]*things.Task, error) {
	var tasks []*things.Task
	syncer := &Syncer{db: st.db}

	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		task, err := syncer.getTask(uuid)
		if err != nil {
			return nil, err
		}
		if task != nil {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/state.go
git commit -m "feat(sync): add State type with query methods"
```

---

### Task 15: Add change log query methods

**Files:**
- Modify: `sync/sync.go`

**Step 1: Add change log query methods**

Add to `sync/sync.go`:
```go

// ChangesSince returns changes that occurred after the given timestamp
func (s *Syncer) ChangesSince(timestamp time.Time) ([]Change, error) {
	rows, err := s.db.Query(`
		SELECT id, server_index, synced_at, change_type, entity_type, entity_uuid, payload
		FROM change_log
		WHERE synced_at > ?
		ORDER BY id
	`, timestamp.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanChangeLog(rows)
}

// ChangesSinceIndex returns changes that occurred after the given server index
func (s *Syncer) ChangesSinceIndex(serverIndex int) ([]Change, error) {
	rows, err := s.db.Query(`
		SELECT id, server_index, synced_at, change_type, entity_type, entity_uuid, payload
		FROM change_log
		WHERE server_index > ?
		ORDER BY id
	`, serverIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanChangeLog(rows)
}

// ChangesForEntity returns all changes for a specific entity
func (s *Syncer) ChangesForEntity(entityUUID string) ([]Change, error) {
	rows, err := s.db.Query(`
		SELECT id, server_index, synced_at, change_type, entity_type, entity_uuid, payload
		FROM change_log
		WHERE entity_uuid = ?
		ORDER BY id
	`, entityUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanChangeLog(rows)
}

func (s *Syncer) scanChangeLog(rows *sql.Rows) ([]Change, error) {
	var changes []Change

	for rows.Next() {
		var id int
		var serverIndex int
		var syncedAt int64
		var changeType, entityType, entityUUID string
		var payload sql.NullString

		if err := rows.Scan(&id, &serverIndex, &syncedAt, &changeType, &entityType, &entityUUID, &payload); err != nil {
			return nil, err
		}

		base := baseChange{
			serverIndex: serverIndex,
			timestamp:   time.Unix(syncedAt, 0),
		}

		// Reconstruct change from stored data
		// For now, return UnknownChange with details - a more complete implementation
		// would reconstruct the full typed change
		changes = append(changes, UnknownChange{
			baseChange: base,
			entityType: entityType,
			entityUUID: entityUUID,
			Details:    changeType,
		})
	}

	return changes, nil
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/sync.go
git commit -m "feat(sync): add change log query methods"
```

---

## Phase 7: Integration

### Task 16: Write integration test with mock data

**Files:**
- Create: `sync/integration_test.go`

**Step 1: Write integration test**

Create `sync/integration_test.go`:
```go
package sync

import (
	"encoding/json"
	"path/filepath"
	"testing"

	things "github.com/nicolai86/things-cloud-sdk"
)

// TestIntegration tests the full sync flow with mock items
func TestIntegration(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	// Simulate processing items directly (bypassing network)
	t.Run("process task creation", func(t *testing.T) {
		payload := things.TaskActionItemPayload{}
		title := "Buy groceries"
		payload.Title = &title
		tp := things.TaskTypeTask
		payload.Type = &tp

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "task-001",
			Kind:   things.ItemKindTask,
			Action: things.ItemActionCreated,
			P:      payloadBytes,
		}

		changes, err := syncer.processItems([]things.Item{item}, 0)
		if err != nil {
			t.Fatalf("processItems failed: %v", err)
		}

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}

		created, ok := changes[0].(TaskCreated)
		if !ok {
			t.Fatalf("expected TaskCreated, got %T", changes[0])
		}
		if created.Task.Title != "Buy groceries" {
			t.Errorf("expected title 'Buy groceries', got %q", created.Task.Title)
		}

		// Verify task was persisted
		state := syncer.State()
		task, err := state.Task("task-001")
		if err != nil {
			t.Fatalf("Task lookup failed: %v", err)
		}
		if task == nil {
			t.Fatal("task not persisted")
		}
		if task.Title != "Buy groceries" {
			t.Errorf("persisted title mismatch: %q", task.Title)
		}
	})

	t.Run("process task completion", func(t *testing.T) {
		// Complete the task we created
		payload := things.TaskActionItemPayload{}
		status := things.TaskStatusCompleted
		payload.Status = &status

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "task-001",
			Kind:   things.ItemKindTask,
			Action: things.ItemActionModified,
			P:      payloadBytes,
		}

		changes, err := syncer.processItems([]things.Item{item}, 1)
		if err != nil {
			t.Fatalf("processItems failed: %v", err)
		}

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}

		_, ok := changes[0].(TaskCompleted)
		if !ok {
			t.Fatalf("expected TaskCompleted, got %T", changes[0])
		}
	})

	t.Run("process area creation", func(t *testing.T) {
		payload := things.AreaActionItemPayload{}
		title := "Work"
		payload.Title = &title

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "area-001",
			Kind:   things.ItemKindArea3,
			Action: things.ItemActionCreated,
			P:      payloadBytes,
		}

		changes, err := syncer.processItems([]things.Item{item}, 2)
		if err != nil {
			t.Fatalf("processItems failed: %v", err)
		}

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}

		_, ok := changes[0].(AreaCreated)
		if !ok {
			t.Fatalf("expected AreaCreated, got %T", changes[0])
		}

		// Verify via State
		state := syncer.State()
		areas, _ := state.AllAreas()
		if len(areas) != 1 {
			t.Errorf("expected 1 area, got %d", len(areas))
		}
	})

	t.Run("query change log", func(t *testing.T) {
		changes, err := syncer.ChangesSinceIndex(0)
		if err != nil {
			t.Fatalf("ChangesSinceIndex failed: %v", err)
		}

		// Should have: TaskCreated, TaskCompleted, AreaCreated
		if len(changes) < 3 {
			t.Errorf("expected at least 3 changes in log, got %d", len(changes))
		}
	})
}

func TestStateQueries(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	// Create test data
	syncer.saveTask(&things.Task{
		UUID:     "inbox-1",
		Title:    "Inbox Task",
		Schedule: things.TaskScheduleInbox,
		Status:   things.TaskStatusPending,
	})
	syncer.saveTask(&things.Task{
		UUID:     "anytime-1",
		Title:    "Anytime Task",
		Schedule: things.TaskScheduleAnytime,
		Status:   things.TaskStatusPending,
	})
	syncer.saveTask(&things.Task{
		UUID:     "completed-1",
		Title:    "Completed Task",
		Schedule: things.TaskScheduleAnytime,
		Status:   things.TaskStatusCompleted,
	})
	syncer.saveTask(&things.Task{
		UUID:    "trashed-1",
		Title:   "Trashed Task",
		InTrash: true,
	})
	syncer.saveTask(&things.Task{
		UUID:  "project-1",
		Title: "Test Project",
		Type:  things.TaskTypeProject,
	})

	state := syncer.State()

	t.Run("TasksInInbox excludes completed by default", func(t *testing.T) {
		tasks, err := state.TasksInInbox(QueryOpts{})
		if err != nil {
			t.Fatalf("TasksInInbox failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("expected 1 inbox task, got %d", len(tasks))
		}
	})

	t.Run("AllTasks excludes trashed by default", func(t *testing.T) {
		tasks, err := state.AllTasks(QueryOpts{})
		if err != nil {
			t.Fatalf("AllTasks failed: %v", err)
		}
		for _, task := range tasks {
			if task.InTrash {
				t.Error("trashed task should be excluded")
			}
		}
	})

	t.Run("AllTasks includes trashed when requested", func(t *testing.T) {
		tasks, err := state.AllTasks(QueryOpts{IncludeTrashed: true})
		if err != nil {
			t.Fatalf("AllTasks failed: %v", err)
		}
		found := false
		for _, task := range tasks {
			if task.UUID == "trashed-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("trashed task should be included")
		}
	})

	t.Run("AllProjects returns only projects", func(t *testing.T) {
		projects, err := state.AllProjects(QueryOpts{})
		if err != nil {
			t.Fatalf("AllProjects failed: %v", err)
		}
		if len(projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(projects))
		}
		if projects[0].Type != things.TaskTypeProject {
			t.Error("returned task is not a project")
		}
	})
}
```

**Step 2: Run tests**

Run:
```bash
go test -v ./sync/...
```
Expected: All tests PASS

**Step 3: Commit**

```bash
git add sync/integration_test.go
git commit -m "test(sync): add integration tests"
```

---

### Task 17: Add example usage

**Files:**
- Create: `sync/example_test.go`

**Step 1: Write example**

Create `sync/example_test.go`:
```go
package sync_test

import (
	"fmt"
	"log"
	"os"

	things "github.com/nicolai86/things-cloud-sdk"
	"github.com/nicolai86/things-cloud-sdk/sync"
)

func Example() {
	// Create Things Cloud client
	client := things.New(
		things.APIEndpoint,
		os.Getenv("THINGS_USERNAME"),
		os.Getenv("THINGS_PASSWORD"),
	)

	// Open sync database (creates if doesn't exist)
	syncer, err := sync.Open("~/.things-agent/state.db", client)
	if err != nil {
		log.Fatal(err)
	}
	defer syncer.Close()

	// Sync and get changes
	changes, err := syncer.Sync()
	if err != nil {
		log.Fatal(err)
	}

	// React to changes
	for _, c := range changes {
		switch e := c.(type) {
		case *sync.TaskCreated:
			fmt.Printf("New task: %s\n", e.Task.Title)
		case *sync.TaskCompleted:
			fmt.Printf("Completed: %s\n", e.Task.Title)
		case *sync.TaskMovedToToday:
			fmt.Printf("Scheduled for today: %s (was in %s)\n", e.Task.Title, e.From)
		case *sync.AreaCreated:
			fmt.Printf("New area: %s\n", e.Area.Title)
		}
	}

	// Query current state
	state := syncer.State()

	inbox, _ := state.TasksInInbox(sync.QueryOpts{})
	fmt.Printf("\nInbox has %d items\n", len(inbox))

	projects, _ := state.AllProjects(sync.QueryOpts{})
	fmt.Printf("You have %d active projects\n", len(projects))
}
```

**Step 2: Verify it compiles**

Run:
```bash
go build ./sync/...
```
Expected: No errors

**Step 3: Commit**

```bash
git add sync/example_test.go
git commit -m "docs(sync): add usage example"
```

---

### Task 18: Run all tests and final verification

**Step 1: Run all tests**

Run:
```bash
go test -v ./...
```
Expected: All tests PASS

**Step 2: Build verification**

Run:
```bash
go build -v ./...
```
Expected: No errors

**Step 3: Final commit**

```bash
git add -A
git commit -m "feat(sync): complete persistent sync engine implementation"
```

---

## Summary

The implementation is complete with:

1. **Package structure**: `sync/` subpackage with SQLite storage
2. **Schema**: Tables for tasks, areas, tags, checklists, sync state, change log
3. **Change types**: ~40 semantic event types (TaskCreated, TaskCompleted, etc.)
4. **Change detection**: Compares old/new state to generate typed events
5. **State queries**: TasksInInbox, AllProjects, etc.
6. **Change log**: ChangesSince, ChangesForEntity for historical queries

Total: 18 tasks across 7 phases
