package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

// ---------------------------------------------------------------------------
// handleListTasks
// ---------------------------------------------------------------------------

func TestHandleListTasks(t *testing.T) {
	area := makeAreaItem("area-1", "Work")
	tag := makeTagItem("tag-1", "important")
	project := makeTaskItem("proj-1",
		withTitle("My Project"),
		withTaskType(thingscloud.TaskTypeProject),
	)

	pendingTask := makeTaskItem("task-1",
		withTitle("Buy groceries"),
		withSchedule(thingscloud.TaskScheduleAnytime),
		withCreationDate(mustTime("2025-03-10")),
	)
	completedTask := makeTaskItem("task-2",
		withTitle("Finished task"),
		withStatus(thingscloud.TaskStatusCompleted),
		withSchedule(thingscloud.TaskScheduleAnytime),
		withCreationDate(mustTime("2025-03-05")),
	)
	trashedTask := makeTaskItem("task-3",
		withTitle("Trashed task"),
		withTrashed(),
		withCreationDate(mustTime("2025-03-01")),
	)
	heading := makeTaskItem("heading-1",
		withTitle("Section A"),
		withTaskType(thingscloud.TaskTypeHeading),
	)
	todayTask := makeTaskItem("task-4",
		withTitle("Today task"),
		withSchedule(thingscloud.TaskScheduleAnytime),
		withScheduledDate(time.Now().Add(-1*time.Hour)),
		withCreationDate(mustTime("2025-03-12")),
	)
	somedayTask := makeTaskItem("task-5",
		withTitle("Someday task"),
		withSchedule(thingscloud.TaskScheduleSomeday),
		withCreationDate(mustTime("2025-02-01")),
	)
	taggedTask := makeTaskItem("task-6",
		withTitle("Tagged task"),
		withTags("tag-1"),
		withCreationDate(mustTime("2025-03-15")),
	)
	areaTask := makeTaskItem("task-7",
		withTitle("Work task"),
		withArea("area-1"),
		withCreationDate(mustTime("2025-03-15")),
	)
	projectTask := makeTaskItem("task-8",
		withTitle("Project child"),
		withParent("proj-1"),
		withCreationDate(mustTime("2025-03-15")),
	)
	notedTask := makeTaskItem("task-9",
		withTitle("Task with note"),
		withNote("important details here"),
		withCreationDate(mustTime("2025-03-15")),
	)
	scheduledTask := makeTaskItem("task-10",
		withTitle("Scheduled future"),
		withSchedule(thingscloud.TaskScheduleSomeday),
		withScheduledDate(mustTime("2025-06-01")),
		withCreationDate(mustTime("2025-03-15")),
	)
	deadlineTask := makeTaskItem("task-11",
		withTitle("Has deadline"),
		withDeadline(mustTime("2025-04-01")),
		withCreationDate(mustTime("2025-03-15")),
	)

	allItems := []thingscloud.Item{
		area, tag, project, heading,
		pendingTask, completedTask, trashedTask, todayTask, somedayTask,
		taggedTask, areaTask, projectTask, notedTask, scheduledTask, deadlineTask,
	}

	fc := newFakeCloud("test@example.com", allItems...)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("default returns only pending tasks", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, err := tmcp.handleListTasks(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		// Should include pending tasks, not completed/trashed/projects/headings
		for _, task := range tasks {
			if task.Status != "pending" {
				t.Errorf("expected only pending tasks, got status %q for %q", task.Status, task.Title)
			}
			if task.UUID == "proj-1" || task.UUID == "heading-1" {
				t.Errorf("should not include projects/headings: %q", task.Title)
			}
			if task.UUID == "task-3" {
				t.Errorf("should not include trashed tasks")
			}
		}
		if len(tasks) == 0 {
			t.Error("expected at least one task")
		}
	})

	t.Run("schedule=today filter", func(t *testing.T) {
		req := makeReq(map[string]any{"schedule": "today"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		if len(tasks) == 0 {
			t.Fatal("expected at least one task for schedule=today")
		}
		// schedule=today uses isScheduledForTodayOrPast, which includes overdue tasks
		uuids := make(map[string]bool)
		for _, task := range tasks {
			uuids[task.UUID] = true
		}
		if !uuids["task-4"] {
			t.Errorf("expected task-4 (today task) in results")
		}
	})

	t.Run("status=completed filter", func(t *testing.T) {
		req := makeReq(map[string]any{"status": "completed"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		for _, task := range tasks {
			if task.Status != "completed" {
				t.Errorf("expected status=completed, got %q for %q", task.Status, task.Title)
			}
		}
		found := false
		for _, task := range tasks {
			if task.UUID == "task-2" {
				found = true
			}
		}
		if !found {
			t.Error("completed task (task-2) not found")
		}
	})

	t.Run("tag filter", func(t *testing.T) {
		req := makeReq(map[string]any{"tag": "important"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		if len(tasks) == 0 {
			t.Fatal("expected at least one tagged task")
		}
		for _, task := range tasks {
			if task.UUID != "task-6" {
				t.Errorf("unexpected task in tag filter: %q", task.Title)
			}
		}
	})

	t.Run("area filter", func(t *testing.T) {
		req := makeReq(map[string]any{"area": "Work"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		if len(tasks) == 0 {
			t.Fatal("expected at least one area task")
		}
		for _, task := range tasks {
			if task.UUID != "task-7" {
				t.Errorf("unexpected task in area filter: %q", task.Title)
			}
		}
	})

	t.Run("project filter", func(t *testing.T) {
		req := makeReq(map[string]any{"project": "My Project"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		if len(tasks) == 0 {
			t.Fatal("expected at least one project task")
		}
		for _, task := range tasks {
			if task.UUID != "task-8" {
				t.Errorf("unexpected task in project filter: %q", task.Title)
			}
		}
	})

	t.Run("contains_text in title", func(t *testing.T) {
		req := makeReq(map[string]any{"contains_text": "groceries"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].UUID != "task-1" {
			t.Errorf("expected task-1, got %s", tasks[0].UUID)
		}
	})

	t.Run("contains_text in note", func(t *testing.T) {
		req := makeReq(map[string]any{"contains_text": "important details"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].UUID != "task-9" {
			t.Errorf("expected task-9, got %s", tasks[0].UUID)
		}
	})

	t.Run("in_trash=true includes trashed", func(t *testing.T) {
		req := makeReq(map[string]any{"in_trash": true})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		found := false
		for _, task := range tasks {
			if task.UUID == "task-3" {
				found = true
			}
		}
		if !found {
			t.Error("trashed task (task-3) not found with in_trash=true")
		}
	})

	t.Run("scheduled_before/after filters", func(t *testing.T) {
		// task-10 has scheduledDate=2025-06-01
		req := makeReq(map[string]any{
			"scheduled_before": "2025-07-01",
			"scheduled_after":  "2025-05-01",
		})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		found := false
		for _, task := range tasks {
			if task.UUID == "task-10" {
				found = true
			}
		}
		if !found {
			t.Error("scheduled task (task-10) not found in date range")
		}
	})

	t.Run("created_before/after filters", func(t *testing.T) {
		// task-1 created 2025-03-10
		req := makeReq(map[string]any{
			"created_before": "2025-03-11",
			"created_after":  "2025-03-09",
		})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)

		tasks := resultJSON[[]TaskOutput](t, result)
		found := false
		for _, task := range tasks {
			if task.UUID == "task-1" {
				found = true
			}
		}
		if !found {
			t.Error("task-1 not found in created date range")
		}
	})

	t.Run("empty result returns empty array", func(t *testing.T) {
		req := makeReq(map[string]any{"tag": "nonexistent"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		// This should be an error since tag is not found
		assertIsError(t, result)
	})

	t.Run("nonexistent area returns error", func(t *testing.T) {
		req := makeReq(map[string]any{"area": "Nonexistent"})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertIsError(t, result)
	})
}

// ---------------------------------------------------------------------------
// handleShowTask
// ---------------------------------------------------------------------------

func TestHandleShowTask(t *testing.T) {
	task := makeTaskItem("ABCDEF123456",
		withTitle("Show me"),
		withNote("some notes"),
		withCreationDate(mustTime("2025-03-10")),
	)
	cl1 := makeChecklistItem("cl-1", "ABCDEF123456", "Step 1")
	cl2 := makeChecklistItem("cl-2", "ABCDEF123456", "Step 2")

	fc := newFakeCloud("test@example.com", task, cl1, cl2)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("exact UUID match", func(t *testing.T) {
		req := makeReq(map[string]any{"uuid": "ABCDEF123456"})
		result, err := tmcp.handleShowTask(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertNotError(t, result)

		detail := resultJSON[TaskDetailOutput](t, result)
		if detail.UUID != "ABCDEF123456" {
			t.Errorf("UUID: got %q, want %q", detail.UUID, "ABCDEF123456")
		}
		if detail.Title != "Show me" {
			t.Errorf("Title: got %q, want %q", detail.Title, "Show me")
		}
		if len(detail.Checklist) != 2 {
			t.Errorf("Checklist: got %d items, want 2", len(detail.Checklist))
		}
	})

	t.Run("prefix match", func(t *testing.T) {
		req := makeReq(map[string]any{"uuid": "ABCDEF"})
		result, _ := tmcp.handleShowTask(context.Background(), req)
		assertNotError(t, result)

		detail := resultJSON[TaskDetailOutput](t, result)
		if detail.UUID != "ABCDEF123456" {
			t.Errorf("UUID: got %q, want %q", detail.UUID, "ABCDEF123456")
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := makeReq(map[string]any{"uuid": "NONEXISTENT"})
		result, _ := tmcp.handleShowTask(context.Background(), req)
		assertIsError(t, result)
	})

	t.Run("missing uuid", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleShowTask(context.Background(), req)
		assertIsError(t, result)
	})
}

// ---------------------------------------------------------------------------
// handleListProjects
// ---------------------------------------------------------------------------

func TestHandleListProjects(t *testing.T) {
	proj1 := makeTaskItem("proj-1",
		withTitle("Active Project"),
		withTaskType(thingscloud.TaskTypeProject),
		withCreationDate(mustTime("2025-03-01")),
	)
	proj2 := makeTaskItem("proj-2",
		withTitle("Done Project"),
		withTaskType(thingscloud.TaskTypeProject),
		withStatus(thingscloud.TaskStatusCompleted),
		withCreationDate(mustTime("2025-02-01")),
	)
	task := makeTaskItem("task-1",
		withTitle("Regular task"),
		withCreationDate(mustTime("2025-03-01")),
	)

	fc := newFakeCloud("test@example.com", proj1, proj2, task)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("default returns only pending projects", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-1" {
			t.Errorf("expected proj-1, got %s", projects[0].UUID)
		}
	})

	t.Run("status=completed returns completed projects", func(t *testing.T) {
		req := makeReq(map[string]any{"status": "completed"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 completed project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-2" {
			t.Errorf("expected proj-2, got %s", projects[0].UUID)
		}
	})
}

// ---------------------------------------------------------------------------
// handleListProjects — filter tests
// ---------------------------------------------------------------------------

func TestHandleListProjectsFilters(t *testing.T) {
	area := makeAreaItem("area-1", "Work")
	tag := makeTagItem("tag-1", "urgent")

	projAnytime := makeTaskItem("proj-anytime",
		withTitle("Anytime Project"),
		withTaskType(thingscloud.TaskTypeProject),
		withSchedule(thingscloud.TaskScheduleAnytime),
		withScheduledDate(time.Now().Add(30*24*time.Hour)),
		withDeadline(time.Now().Add(60*24*time.Hour)),
		withArea("area-1"),
		withTags("tag-1"),
		withCreationDate(mustTime("2025-03-01")),
	)
	projSomeday := makeTaskItem("proj-someday",
		withTitle("Someday Project"),
		withTaskType(thingscloud.TaskTypeProject),
		withSchedule(thingscloud.TaskScheduleSomeday),
		withCreationDate(mustTime("2025-03-10")),
		withNote("important notes here"),
	)
	projToday := makeTaskItem("proj-today",
		withTitle("Today Project"),
		withTaskType(thingscloud.TaskTypeProject),
		withSchedule(thingscloud.TaskScheduleAnytime),
		withScheduledDate(time.Now().Add(-1*time.Hour)),
		withCreationDate(mustTime("2025-03-15")),
	)
	projTrashed := makeTaskItem("proj-trashed",
		withTitle("Trashed Project"),
		withTaskType(thingscloud.TaskTypeProject),
		withTrashed(),
		withCreationDate(mustTime("2025-01-01")),
	)

	fc := newFakeCloud("test@example.com", area, tag, projAnytime, projSomeday, projToday, projTrashed)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("schedule=someday", func(t *testing.T) {
		req := makeReq(map[string]any{"schedule": "someday"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-someday" {
			t.Errorf("expected proj-someday, got %s", projects[0].UUID)
		}
	})

	t.Run("schedule=today", func(t *testing.T) {
		req := makeReq(map[string]any{"schedule": "today"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-today" {
			t.Errorf("expected proj-today, got %s", projects[0].UUID)
		}
	})

	t.Run("scheduled_before filters by scheduled date", func(t *testing.T) {
		// projAnytime is scheduled +30d, projToday is scheduled ~now
		// Use +45d cutoff: both should match
		cutoff := time.Now().Add(45 * 24 * time.Hour).Format("2006-01-02")
		req := makeReq(map[string]any{"scheduled_before": cutoff})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		uuids := make(map[string]bool)
		for _, p := range projects {
			uuids[p.UUID] = true
		}
		if !uuids["proj-anytime"] {
			t.Errorf("expected proj-anytime in results")
		}
		if !uuids["proj-today"] {
			t.Errorf("expected proj-today in results")
		}
	})

	t.Run("deadline_before filters by deadline date", func(t *testing.T) {
		// projAnytime has deadline +60d; use +90d cutoff
		cutoff := time.Now().Add(90 * 24 * time.Hour).Format("2006-01-02")
		req := makeReq(map[string]any{"deadline_before": cutoff})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("deadline_after filters by deadline date", func(t *testing.T) {
		// projAnytime has deadline +60d; use +30d cutoff so it's after
		cutoff := time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02")
		req := makeReq(map[string]any{"deadline_after": cutoff})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("tag=urgent", func(t *testing.T) {
		req := makeReq(map[string]any{"tag": "urgent"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("area=Work", func(t *testing.T) {
		req := makeReq(map[string]any{"area": "Work"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("contains_text matches note", func(t *testing.T) {
		req := makeReq(map[string]any{"contains_text": "important"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-someday" {
			t.Errorf("expected proj-someday, got %s", projects[0].UUID)
		}
	})

	t.Run("contains_text matches title", func(t *testing.T) {
		req := makeReq(map[string]any{"contains_text": "anytime"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("in_trash=true includes trashed projects", func(t *testing.T) {
		req := makeReq(map[string]any{"in_trash": true})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		// All 4 pending projects (3 non-trashed + 1 trashed), default status=pending
		if len(projects) != 4 {
			t.Fatalf("expected 4 projects, got %d", len(projects))
		}
		uuids := make(map[string]bool)
		for _, p := range projects {
			uuids[p.UUID] = true
		}
		if !uuids["proj-trashed"] {
			t.Errorf("expected proj-trashed in results")
		}
	})

	t.Run("scheduled_after filters by scheduled date", func(t *testing.T) {
		// projAnytime is +30d, projToday is ~now; use +15d cutoff
		cutoff := time.Now().Add(15 * 24 * time.Hour).Format("2006-01-02")
		req := makeReq(map[string]any{"scheduled_after": cutoff})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		// Only projAnytime (+30d) is after cutoff (+15d); projToday (~now) is not
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("created_before=2025-03-05", func(t *testing.T) {
		req := makeReq(map[string]any{"created_before": "2025-03-05"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		// projAnytime (2025-03-01) matches; projTrashed (2025-01-01) excluded by default in_trash=false
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].UUID != "proj-anytime" {
			t.Errorf("expected proj-anytime, got %s", projects[0].UUID)
		}
	})

	t.Run("created_after=2025-03-05", func(t *testing.T) {
		req := makeReq(map[string]any{"created_after": "2025-03-05"})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)

		projects := resultJSON[[]TaskOutput](t, result)
		// projSomeday (2025-03-10) and projToday (2025-03-15) match
		if len(projects) != 2 {
			t.Fatalf("expected 2 projects, got %d", len(projects))
		}
		uuids := make(map[string]bool)
		for _, p := range projects {
			uuids[p.UUID] = true
		}
		if !uuids["proj-someday"] {
			t.Errorf("expected proj-someday in results")
		}
		if !uuids["proj-today"] {
			t.Errorf("expected proj-today in results")
		}
	})
}

// ---------------------------------------------------------------------------
// handleListAreas
// ---------------------------------------------------------------------------

func TestHandleListAreas(t *testing.T) {
	area1 := makeAreaItem("area-1", "Work")
	area2 := makeAreaItem("area-2", "Personal")

	fc := newFakeCloud("test@example.com", area1, area2)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	req := makeReq(map[string]any{})
	result, err := tmcp.handleListAreas(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNotError(t, result)

	type AreaOutput struct {
		UUID  string `json:"uuid"`
		Title string `json:"title"`
	}
	areas := resultJSON[[]AreaOutput](t, result)
	if len(areas) != 2 {
		t.Fatalf("expected 2 areas, got %d", len(areas))
	}

	titles := map[string]bool{}
	for _, a := range areas {
		titles[a.Title] = true
	}
	if !titles["Work"] || !titles["Personal"] {
		t.Errorf("expected Work and Personal areas, got %v", areas)
	}
}

// ---------------------------------------------------------------------------
// handleListTags
// ---------------------------------------------------------------------------

func TestHandleListTags(t *testing.T) {
	tag1 := makeTagItem("tag-1", "urgent")
	tag2 := makeTagItem("tag-2", "low-priority")

	fc := newFakeCloud("test@example.com", tag1, tag2)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	req := makeReq(map[string]any{})
	result, err := tmcp.handleListTags(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNotError(t, result)

	type TagOutput struct {
		UUID  string `json:"uuid"`
		Title string `json:"title"`
	}
	tags := resultJSON[[]TagOutput](t, result)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	titles := map[string]bool{}
	for _, tg := range tags {
		titles[tg.Title] = true
	}
	if !titles["urgent"] || !titles["low-priority"] {
		t.Errorf("expected urgent and low-priority tags, got %v", tags)
	}
}

// ---------------------------------------------------------------------------
// handleCreateTask
// ---------------------------------------------------------------------------

func TestHandleCreateTask(t *testing.T) {
	fc := newFakeCloud("test@example.com")
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("basic create", func(t *testing.T) {
		req := makeReq(map[string]any{
			"title":    "New task",
			"note":     "Task notes",
			"schedule": "today",
		})
		result, err := tmcp.handleCreateTask(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertNotError(t, result)

		out := resultJSON[map[string]string](t, result)
		if out["status"] != "created" {
			t.Errorf("status: got %q, want %q", out["status"], "created")
		}
		if out["title"] != "New task" {
			t.Errorf("title: got %q, want %q", out["title"], "New task")
		}
		if out["uuid"] == "" {
			t.Error("expected non-empty uuid")
		}

		// Verify commit was sent to fake server
		commits := fc.getCommitLog()
		if len(commits) == 0 {
			t.Fatal("expected at least one commit")
		}

		// Parse the commit body — it's a map of UUID → envelope
		var commitBody map[string]json.RawMessage
		if err := json.Unmarshal(commits[0], &commitBody); err != nil {
			t.Fatalf("unmarshal commit body: %v", err)
		}

		// Should have exactly one entry (the task)
		if len(commitBody) != 1 {
			t.Fatalf("expected 1 entry in commit, got %d", len(commitBody))
		}

		// Parse the envelope
		for _, raw := range commitBody {
			var env struct {
				T int             `json:"t"`
				E string          `json:"e"`
				P json.RawMessage `json:"p"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("unmarshal envelope: %v", err)
			}
			if env.T != 0 {
				t.Errorf("action: got %d, want 0 (create)", env.T)
			}
			if env.E != "Task6" {
				t.Errorf("kind: got %q, want %q", env.E, "Task6")
			}

			// Check payload fields
			var payload map[string]any
			json.Unmarshal(env.P, &payload)
			if payload["tt"] != "New task" {
				t.Errorf("tt: got %v, want %q", payload["tt"], "New task")
			}
			if payload["st"] != float64(1) {
				t.Errorf("st: got %v, want 1 (anytime/today)", payload["st"])
			}
			// Check note has CRC32
			if ntRaw, ok := payload["nt"]; ok {
				ntMap, _ := ntRaw.(map[string]any)
				if ntMap["v"] != "Task notes" {
					t.Errorf("nt.v: got %v, want %q", ntMap["v"], "Task notes")
				}
				if ntMap["ch"] == float64(0) {
					t.Error("nt.ch: expected non-zero CRC32 for non-empty note")
				}
			}
		}
	})

	t.Run("missing title returns error", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleCreateTask(context.Background(), req)
		assertIsError(t, result)
	})
}

// ---------------------------------------------------------------------------
// handleEditTask
// ---------------------------------------------------------------------------

func TestHandleEditTask(t *testing.T) {
	task := makeTaskItem("task-edit-1",
		withTitle("Original title"),
		withCreationDate(mustTime("2025-03-10")),
	)

	fc := newFakeCloud("test@example.com", task)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("edit title", func(t *testing.T) {
		req := makeReq(map[string]any{
			"uuid":  "task-edit-1",
			"title": "Updated title",
		})
		result, err := tmcp.handleEditTask(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertNotError(t, result)

		out := resultJSON[map[string]string](t, result)
		if out["status"] != "updated" {
			t.Errorf("status: got %q, want %q", out["status"], "updated")
		}

		// Verify commit payload
		commits := fc.getCommitLog()
		if len(commits) == 0 {
			t.Fatal("expected at least one commit")
		}
		lastCommit := commits[len(commits)-1]
		var commitBody map[string]json.RawMessage
		json.Unmarshal(lastCommit, &commitBody)

		for _, raw := range commitBody {
			var env struct {
				T int             `json:"t"`
				E string          `json:"e"`
				P json.RawMessage `json:"p"`
			}
			json.Unmarshal(raw, &env)
			if env.T != 1 {
				t.Errorf("action: got %d, want 1 (modify)", env.T)
			}
			var payload map[string]any
			json.Unmarshal(env.P, &payload)
			if payload["tt"] != "Updated title" {
				t.Errorf("tt: got %v, want %q", payload["tt"], "Updated title")
			}
		}
	})

	t.Run("edit nonexistent task", func(t *testing.T) {
		req := makeReq(map[string]any{
			"uuid":  "nonexistent-uuid",
			"title": "Won't work",
		})
		result, _ := tmcp.handleEditTask(context.Background(), req)
		assertIsError(t, result)
	})

	t.Run("missing uuid", func(t *testing.T) {
		req := makeReq(map[string]any{
			"title": "No uuid",
		})
		result, _ := tmcp.handleEditTask(context.Background(), req)
		assertIsError(t, result)
	})

	t.Run("complete task", func(t *testing.T) {
		req := makeReq(map[string]any{
			"uuid":   "task-edit-1",
			"status": "completed",
		})
		result, _ := tmcp.handleEditTask(context.Background(), req)
		assertNotError(t, result)

		commits := fc.getCommitLog()
		lastCommit := commits[len(commits)-1]
		var commitBody map[string]json.RawMessage
		json.Unmarshal(lastCommit, &commitBody)

		for _, raw := range commitBody {
			var env struct {
				P json.RawMessage `json:"p"`
			}
			json.Unmarshal(raw, &env)
			var payload map[string]any
			json.Unmarshal(env.P, &payload)
			if payload["ss"] != float64(3) {
				t.Errorf("ss: got %v, want 3 (completed)", payload["ss"])
			}
			if payload["sp"] == nil {
				t.Error("sp: expected non-nil stop date")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// handleListHeadings
// ---------------------------------------------------------------------------

func TestHandleListHeadings(t *testing.T) {
	project := makeTaskItem("proj-1",
		withTitle("My Project"),
		withTaskType(thingscloud.TaskTypeProject),
	)
	heading := makeTaskItem("heading-1",
		withTitle("Section A"),
		withTaskType(thingscloud.TaskTypeHeading),
		withParent("proj-1"),
	)
	heading2 := makeTaskItem("heading-2",
		withTitle("Section B"),
		withTaskType(thingscloud.TaskTypeHeading),
		withParent("proj-1"),
	)

	fc := newFakeCloud("test@example.com", project, heading, heading2)
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("lists headings for project", func(t *testing.T) {
		req := makeReq(map[string]any{"project_uuid": "proj-1"})
		result, err := tmcp.handleListHeadings(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertNotError(t, result)

		type HeadingOutput struct {
			UUID  string `json:"uuid"`
			Title string `json:"title"`
		}
		headings := resultJSON[[]HeadingOutput](t, result)
		if len(headings) != 2 {
			t.Fatalf("expected 2 headings, got %d", len(headings))
		}
	})

	t.Run("missing project_uuid returns error", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleListHeadings(context.Background(), req)
		assertIsError(t, result)
	})
}

// ---------------------------------------------------------------------------
// Empty state edge cases
// ---------------------------------------------------------------------------

func TestEmptyStateReturnsEmptyArrays(t *testing.T) {
	fc := newFakeCloud("test@example.com")
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	t.Run("list tasks empty", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleListTasks(context.Background(), req)
		assertNotError(t, result)
		text := resultText(t, result)
		if text != "[]" {
			t.Errorf("expected empty array, got: %s", text)
		}
	})

	t.Run("list projects empty", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleListProjects(context.Background(), req)
		assertNotError(t, result)
		text := resultText(t, result)
		if text != "[]" {
			t.Errorf("expected empty array, got: %s", text)
		}
	})

	t.Run("list areas empty", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleListAreas(context.Background(), req)
		assertNotError(t, result)
		text := resultText(t, result)
		if text != "[]" {
			t.Errorf("expected empty array, got: %s", text)
		}
	})

	t.Run("list tags empty", func(t *testing.T) {
		req := makeReq(map[string]any{})
		result, _ := tmcp.handleListTags(context.Background(), req)
		assertNotError(t, result)
		text := resultText(t, result)
		if text != "[]" {
			t.Errorf("expected empty array, got: %s", text)
		}
	})
}
