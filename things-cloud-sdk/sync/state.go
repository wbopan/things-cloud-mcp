package sync

import (
	"database/sql"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
)

// State provides read-only access to the synced Things state
type State struct {
	db dbExecutor
}

// State returns a read-only view of the current aggregated state
func (s *Syncer) State() *State {
	return &State{db: s.rawDB}
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
