# Persistent Sync Engine Design

## Overview

A stateful sync engine that persists Things Cloud state to SQLite and surfaces semantic change events. Designed for agents that need to know "what changed since last sync" rather than just querying current state.

## Goals

1. **Persistent state** — SQLite storage that survives process restarts
2. **Incremental sync** — Only fetch new items from server using cursor
3. **Semantic changes** — Translate raw Items into meaningful events like "TaskCompleted", "TaskMovedToToday"
4. **Event ordering** — Preserve the sequence of changes (task completed → uncompleted shows both events)
5. **Queryable state** — Current aggregated state available for context lookups

## Non-Goals

- Polling/scheduling logic (agent decides when to call `Sync()`)
- Push notifications (Things Cloud API is request/reply only)
- Conflict resolution (read-only sync, writes go through existing SDK)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Agent                                │
│  (decides when to sync, reacts to changes)                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      thingsync.Syncer                        │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐  │
│  │ Sync()      │  │ State        │  │ ChangesSince()     │  │
│  │ returns     │  │ queries      │  │ query change log   │  │
│  │ []Change    │  │ current data │  │                    │  │
│  └─────────────┘  └──────────────┘  └────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
            ┌─────────────────┴─────────────────┐
            ▼                                   ▼
┌───────────────────────┐           ┌───────────────────────┐
│   Things Cloud API    │           │   SQLite Database     │
│   (via existing SDK)  │           │   - sync_state        │
│                       │           │   - tasks, areas...   │
│                       │           │   - change_log        │
└───────────────────────┘           └───────────────────────┘
```

## SQLite Schema

```sql
-- Sync metadata (singleton row)
CREATE TABLE sync_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    history_id TEXT NOT NULL,
    server_index INTEGER NOT NULL DEFAULT 0,
    last_sync_at INTEGER
);

-- Core entities
CREATE TABLE areas (
    uuid TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    "index" INTEGER,
    deleted INTEGER DEFAULT 0
);

CREATE TABLE tags (
    uuid TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    shortcut TEXT,
    parent_uuid TEXT,
    "index" INTEGER,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (parent_uuid) REFERENCES tags(uuid)
);

CREATE TABLE tasks (
    uuid TEXT PRIMARY KEY,
    type INTEGER NOT NULL DEFAULT 0,        -- 0=task, 1=project, 2=heading
    title TEXT NOT NULL,
    note TEXT,
    status INTEGER NOT NULL DEFAULT 0,      -- 0=pending, 2=canceled, 3=completed
    schedule INTEGER NOT NULL DEFAULT 0,    -- 0=inbox, 1=anytime, 2=someday
    scheduled_date INTEGER,
    deadline_date INTEGER,
    completion_date INTEGER,
    creation_date INTEGER,
    modification_date INTEGER,
    "index" INTEGER,
    today_index INTEGER,
    in_trash INTEGER DEFAULT 0,
    area_uuid TEXT,
    project_uuid TEXT,
    heading_uuid TEXT,
    alarm_time_offset INTEGER,
    recurrence_rule TEXT,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (area_uuid) REFERENCES areas(uuid),
    FOREIGN KEY (project_uuid) REFERENCES tasks(uuid),
    FOREIGN KEY (heading_uuid) REFERENCES tasks(uuid)
);

CREATE TABLE checklist_items (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    title TEXT NOT NULL,
    status INTEGER NOT NULL DEFAULT 0,
    "index" INTEGER,
    creation_date INTEGER,
    completion_date INTEGER,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (task_uuid) REFERENCES tasks(uuid)
);

-- Junction tables
CREATE TABLE task_tags (
    task_uuid TEXT NOT NULL,
    tag_uuid TEXT NOT NULL,
    PRIMARY KEY (task_uuid, tag_uuid)
);

CREATE TABLE area_tags (
    area_uuid TEXT NOT NULL,
    tag_uuid TEXT NOT NULL,
    PRIMARY KEY (area_uuid, tag_uuid)
);

-- Change log for "what changed" queries
CREATE TABLE change_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_index INTEGER NOT NULL,
    synced_at INTEGER NOT NULL,
    change_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_uuid TEXT NOT NULL,
    payload TEXT
);

CREATE INDEX idx_change_log_synced_at ON change_log(synced_at);
CREATE INDEX idx_change_log_entity ON change_log(entity_type, entity_uuid);
```

## Change Types

### Task/Project/Heading

**Lifecycle:**
- `TaskCreated` / `ProjectCreated` / `HeadingCreated`
- `TaskDeleted` / `ProjectDeleted` / `HeadingDeleted`

**Status:**
- `TaskCompleted` / `ProjectCompleted`
- `TaskUncompleted` / `ProjectUncompleted`
- `TaskCanceled` / `ProjectCanceled`

**Content:**
- `TaskTitleChanged` / `ProjectTitleChanged` / `HeadingTitleChanged`
- `TaskNoteChanged` / `ProjectNoteChanged`

**Location/Scheduling:**
- `TaskMovedToInbox`
- `TaskMovedToToday`
- `TaskMovedToAnytime`
- `TaskMovedToSomeday`
- `TaskMovedToUpcoming`
- `TaskDeadlineChanged`
- `TaskScheduledDateChanged`

**Organization:**
- `TaskAssignedToProject`
- `TaskAssignedToArea`
- `TaskAssignedToHeading`
- `TaskTagsChanged`
- `TaskReordered`

**Trash:**
- `TaskTrashed` / `ProjectTrashed`
- `TaskRestored` / `ProjectRestored`

**Reminders/Recurrence:**
- `TaskReminderSet` / `TaskReminderCleared`
- `TaskRecurrenceChanged`

### Area

- `AreaCreated`
- `AreaDeleted`
- `AreaRenamed`
- `AreaTagsChanged`
- `AreaReordered`

### Tag

- `TagCreated`
- `TagDeleted`
- `TagRenamed`
- `TagShortcutChanged`
- `TagParentChanged`

### ChecklistItem

- `ChecklistItemCreated`
- `ChecklistItemDeleted`
- `ChecklistItemCompleted`
- `ChecklistItemUncompleted`
- `ChecklistItemTitleChanged`
- `ChecklistItemReordered`
- `ChecklistItemMovedToTask`

### Meta

- `UnknownChange` — catchall for unrecognized field changes

## Go API

```go
package thingsync

// Syncer manages persistent sync with Things Cloud
type Syncer struct { ... }

// Open creates or opens a sync database
func Open(dbPath string, client *things.Client) (*Syncer, error)

// Close closes the database connection
func (s *Syncer) Close() error

// Sync fetches new items and returns ordered changes
func (s *Syncer) Sync() ([]Change, error)

// State returns read-only view of current state
func (s *Syncer) State() *State

// LastSyncedIndex returns the server cursor position
func (s *Syncer) LastSyncedIndex() int

// ChangesSince queries the change log
func (s *Syncer) ChangesSince(timestamp time.Time) ([]Change, error)
func (s *Syncer) ChangesSinceIndex(serverIndex int) ([]Change, error)
func (s *Syncer) ChangesForEntity(entityUUID string) ([]Change, error)

// Change interface
type Change interface {
    ChangeType() string
    EntityType() string
    EntityUUID() string
    ServerIndex() int
    Timestamp() time.Time
}

// State queries
type State struct { ... }

func (st *State) Task(uuid string) (*things.Task, error)
func (st *State) AllTasks(opts QueryOpts) ([]*things.Task, error)
func (st *State) TasksInInbox(opts QueryOpts) ([]*things.Task, error)
func (st *State) TasksInToday(opts QueryOpts) ([]*things.Task, error)
// ... etc
```

## Usage Example

```go
syncer, _ := thingsync.Open("~/.things-agent/state.db", client)
defer syncer.Close()

changes, _ := syncer.Sync()

for _, c := range changes {
    switch e := c.(type) {
    case *thingsync.TaskCompleted:
        fmt.Printf("You completed: %s\n", e.Task.Title)
    case *thingsync.TaskMovedToToday:
        fmt.Printf("Scheduled for today: %s\n", e.Task.Title)
    }
}

// Query current state for context
state := syncer.State()
inbox, _ := state.TasksInInbox(thingsync.QueryOpts{})
```

## Implementation Notes

- Package location: `sync/` or `thingsync/` subpackage
- Uses `database/sql` with `modernc.org/sqlite` (pure Go, no CGO)
- Change detection compares incoming Item payload against stored state
- Soft deletes preserve entity data for "what was deleted" context
- All Change types embed common fields and implement the Change interface
