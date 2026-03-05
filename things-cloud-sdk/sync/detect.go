package sync

import (
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
)

// detectTaskChanges compares old and new task state and returns semantic changes
func detectTaskChanges(old, new *things.Task, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	// Created
	if old == nil && new != nil {
		switch new.Type {
		case things.TaskTypeProject:
			changes = append(changes, ProjectCreated{projectChange: projectChange{baseChange: base, Project: new}})
		case things.TaskTypeHeading:
			changes = append(changes, HeadingCreated{headingChange: headingChange{baseChange: base, Heading: new}})
		default:
			changes = append(changes, TaskCreated{taskChange: taskChange{baseChange: base, Task: new}})
		}
		return changes
	}

	// Deleted
	if old != nil && new == nil {
		switch old.Type {
		case things.TaskTypeProject:
			changes = append(changes, ProjectDeleted{projectChange: projectChange{baseChange: base, Project: old}})
		case things.TaskTypeHeading:
			changes = append(changes, HeadingDeleted{headingChange: headingChange{baseChange: base, Heading: old}})
		default:
			changes = append(changes, TaskDeleted{taskChange: taskChange{baseChange: base, Task: old}})
		}
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	// Title changed
	if old.Title != new.Title {
		switch new.Type {
		case things.TaskTypeProject:
			changes = append(changes, ProjectTitleChanged{projectChange: projectChange{baseChange: base, Project: new}, OldTitle: old.Title})
		case things.TaskTypeHeading:
			changes = append(changes, HeadingTitleChanged{headingChange: headingChange{baseChange: base, Heading: new}, OldTitle: old.Title})
		default:
			changes = append(changes, TaskTitleChanged{taskChange: taskChange{baseChange: base, Task: new}, OldTitle: old.Title})
		}
	}

	// Note changed (not for headings)
	if old.Note != new.Note && new.Type != things.TaskTypeHeading {
		changes = append(changes, TaskNoteChanged{taskChange: taskChange{baseChange: base, Task: new}, OldNote: old.Note})
	}

	// Status changed
	if old.Status != new.Status {
		switch {
		case new.Status == things.TaskStatusCompleted:
			if new.Type == things.TaskTypeProject {
				changes = append(changes, ProjectCompleted{projectChange: projectChange{baseChange: base, Project: new}})
			} else {
				changes = append(changes, TaskCompleted{taskChange: taskChange{baseChange: base, Task: new}})
			}
		case new.Status == things.TaskStatusCanceled:
			changes = append(changes, TaskCanceled{taskChange: taskChange{baseChange: base, Task: new}})
		case old.Status == things.TaskStatusCompleted && new.Status == things.TaskStatusPending:
			changes = append(changes, TaskUncompleted{taskChange: taskChange{baseChange: base, Task: new}})
		}
	}

	// Trash changed
	if old.InTrash != new.InTrash {
		if new.InTrash {
			if new.Type == things.TaskTypeProject {
				changes = append(changes, ProjectTrashed{projectChange: projectChange{baseChange: base, Project: new}})
			} else {
				changes = append(changes, TaskTrashed{taskChange: taskChange{baseChange: base, Task: new}})
			}
		} else {
			if new.Type == things.TaskTypeProject {
				changes = append(changes, ProjectRestored{projectChange: projectChange{baseChange: base, Project: new}})
			} else {
				changes = append(changes, TaskRestored{taskChange: taskChange{baseChange: base, Task: new}})
			}
		}
	}

	// Schedule/location changed (only for regular tasks)
	if new.Type == things.TaskTypeTask {
		oldLoc := taskLocation(old)
		newLoc := taskLocation(new)
		if oldLoc != newLoc {
			tc := taskChange{baseChange: base, Task: new}
			switch newLoc {
			case LocationInbox:
				changes = append(changes, TaskMovedToInbox{taskChange: tc, From: oldLoc})
			case LocationToday:
				changes = append(changes, TaskMovedToToday{taskChange: tc, From: oldLoc})
			case LocationAnytime:
				changes = append(changes, TaskMovedToAnytime{taskChange: tc, From: oldLoc})
			case LocationSomeday:
				changes = append(changes, TaskMovedToSomeday{taskChange: tc, From: oldLoc})
			case LocationUpcoming:
				var scheduledFor time.Time
				if new.ScheduledDate != nil {
					scheduledFor = *new.ScheduledDate
				}
				changes = append(changes, TaskMovedToUpcoming{taskChange: tc, From: oldLoc, ScheduledFor: scheduledFor})
			}
		}
	}

	// Deadline changed
	if !timeEqual(old.DeadlineDate, new.DeadlineDate) {
		changes = append(changes, TaskDeadlineChanged{taskChange: taskChange{baseChange: base, Task: new}, OldDeadline: old.DeadlineDate})
	}

	// Tags changed
	added, removed := diffStringSlices(old.TagIDs, new.TagIDs)
	if len(added) > 0 || len(removed) > 0 {
		changes = append(changes, TaskTagsChanged{taskChange: taskChange{baseChange: base, Task: new}, Added: added, Removed: removed})
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

// detectAreaChanges compares old and new area state
func detectAreaChanges(old, new *things.Area, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	if old == nil && new != nil {
		changes = append(changes, AreaCreated{areaChange: areaChange{baseChange: base, Area: new}})
		return changes
	}

	if old != nil && new == nil {
		changes = append(changes, AreaDeleted{areaChange: areaChange{baseChange: base, Area: old}})
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	if old.Title != new.Title {
		changes = append(changes, AreaRenamed{areaChange: areaChange{baseChange: base, Area: new}, OldTitle: old.Title})
	}

	return changes
}

// detectTagChanges compares old and new tag state
func detectTagChanges(old, new *things.Tag, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	if old == nil && new != nil {
		changes = append(changes, TagCreated{tagChange: tagChange{baseChange: base, Tag: new}})
		return changes
	}

	if old != nil && new == nil {
		changes = append(changes, TagDeleted{tagChange: tagChange{baseChange: base, Tag: old}})
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	if old.Title != new.Title {
		changes = append(changes, TagRenamed{tagChange: tagChange{baseChange: base, Tag: new}, OldTitle: old.Title})
	}

	if old.ShortHand != new.ShortHand {
		changes = append(changes, TagShortcutChanged{tagChange: tagChange{baseChange: base, Tag: new}, OldShortcut: old.ShortHand})
	}

	return changes
}

// detectChecklistChanges compares old and new checklist item state
func detectChecklistChanges(old, new *things.CheckListItem, task *things.Task, serverIndex int, ts time.Time) []Change {
	var changes []Change
	base := baseChange{serverIndex: serverIndex, timestamp: ts}

	if old == nil && new != nil {
		changes = append(changes, ChecklistItemCreated{checklistItemChange: checklistItemChange{baseChange: base, Item: new}, Task: task})
		return changes
	}

	if old != nil && new == nil {
		changes = append(changes, ChecklistItemDeleted{checklistItemChange: checklistItemChange{baseChange: base, Item: old}})
		return changes
	}

	if old == nil || new == nil {
		return changes
	}

	if old.Title != new.Title {
		changes = append(changes, ChecklistItemTitleChanged{checklistItemChange: checklistItemChange{baseChange: base, Item: new}, OldTitle: old.Title})
	}

	if old.Status != new.Status {
		if new.Status == things.TaskStatusCompleted {
			changes = append(changes, ChecklistItemCompleted{checklistItemChange: checklistItemChange{baseChange: base, Item: new}, Task: task})
		} else if old.Status == things.TaskStatusCompleted {
			changes = append(changes, ChecklistItemUncompleted{checklistItemChange: checklistItemChange{baseChange: base, Item: new}, Task: task})
		}
	}

	return changes
}
