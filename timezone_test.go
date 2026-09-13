package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

// Things stores date-only fields (sr, tir, dd) as UTC midnight of the calendar
// date, but "today" is the user's calendar date, not the server's UTC date.

func pinClock(t *testing.T, now time.Time, tz *time.Location) {
	t.Helper()
	oldClock, oldTZ := wallClock, userTZ
	wallClock = func() time.Time { return now.UTC() } // servers run on UTC
	userTZ = tz
	t.Cleanup(func() { wallClock, userTZ = oldClock, oldTZ })
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func calendarDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func anytimeTaskOn(date time.Time) *thingscloud.Task {
	return &thingscloud.Task{Schedule: thingscloud.TaskScheduleAnytime, ScheduledDate: &date}
}

// Between 00:00 and 02:00 CEST the server's UTC clock is still on the previous
// day. Today's tasks must already count as today.
func TestTodayFollowsUserCalendarEastOfUTC(t *testing.T) {
	berlin := mustLocation(t, "Europe/Berlin")
	pinClock(t, time.Date(2026, 9, 14, 0, 30, 0, 0, berlin), berlin) // 13 Sep 22:30 UTC

	if got, want := todayMidnightUTC(), calendarDate(2026, 9, 14).Unix(); got != want {
		t.Errorf("todayMidnightUTC = %s, want %s", time.Unix(got, 0).UTC(), time.Unix(want, 0).UTC())
	}
	if !isToday(calendarDate(2026, 9, 14)) || isToday(calendarDate(2026, 9, 13)) {
		t.Error("for a Berlin user at 00:30 on 14 Sep, 14 Sep is today and 13 Sep is not")
	}
	if !isScheduledForTodayOrPast(anytimeTaskOn(calendarDate(2026, 9, 14))) {
		t.Error("a task dated 14 Sep must be in Today")
	}
}

// West of UTC the error flips: at 21:00 in New York, UTC is already on the next
// day. Converting a stored UTC-midnight date into the user's zone lands on the
// previous evening, and tomorrow's tasks leak into Today.
func TestTodayFollowsUserCalendarWestOfUTC(t *testing.T) {
	newYork := mustLocation(t, "America/New_York")
	pinClock(t, time.Date(2026, 9, 13, 21, 0, 0, 0, newYork), newYork) // 14 Sep 01:00 UTC

	if got, want := todayMidnightUTC(), calendarDate(2026, 9, 13).Unix(); got != want {
		t.Errorf("todayMidnightUTC = %s, want %s", time.Unix(got, 0).UTC(), time.Unix(want, 0).UTC())
	}
	if !isToday(calendarDate(2026, 9, 13)) {
		t.Error("13 Sep must be today for a New York user at 21:00 on 13 Sep")
	}
	if isScheduledForTodayOrPast(anytimeTaskOn(calendarDate(2026, 9, 14))) {
		t.Error("a task dated 14 Sep must not be in Today on 13 Sep")
	}
	if !isScheduledForTodayOrPast(anytimeTaskOn(calendarDate(2026, 9, 13))) {
		t.Error("a task dated 13 Sep must be in Today on 13 Sep")
	}
}

// things_overview lists upcoming tasks from the end of today through the
// lookahead window. Anchored on the server's UTC date, the window ends a day
// early between 00:00 and 02:00 CEST and drops the last day's tasks.
func TestOverviewUpcomingWindowFollowsUserCalendar(t *testing.T) {
	berlin := mustLocation(t, "Europe/Berlin")
	pinClock(t, time.Date(2026, 9, 14, 0, 30, 0, 0, berlin), berlin)

	fc := newFakeCloud("test@example.com", makeTaskItem("task-in-a-week",
		withTitle("Due in seven days"),
		withSchedule(thingscloud.TaskScheduleSomeday),
		withScheduledDate(calendarDate(2026, 9, 21)),
	))
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)

	result, err := tmcp.handleOverview(context.Background(), makeReq(map[string]any{"lookahead_days": 7}))
	if err != nil {
		t.Fatal(err)
	}
	assertNotError(t, result)
	got := resultJSON[struct {
		UpcomingTasks []TaskOutput `json:"upcomingTasks"`
	}](t, result)
	if len(got.UpcomingTasks) != 1 {
		t.Fatalf("upcoming at 00:30 CEST on 14 Sep with lookahead 7 = %d tasks, want the task dated 21 Sep", len(got.UpcomingTasks))
	}
}

// "weekly" without a schedule repeats on today's weekday. At 00:30 CEST on a
// Monday that is Monday, not the Sunday the server's UTC clock still shows.
func TestWeeklyRecurrenceWithoutScheduleUsesUsersWeekday(t *testing.T) {
	berlin := mustLocation(t, "Europe/Berlin")
	pinClock(t, time.Date(2026, 9, 14, 0, 30, 0, 0, berlin), berlin) // Monday

	fc := newFakeCloud("test@example.com")
	defer fc.Close()
	tmcp := newTestThingsMCP(t, fc)
	result, err := tmcp.handleCreateTask(context.Background(), makeReq(map[string]any{
		"title":      "weekly review",
		"recurrence": "weekly",
	}))
	if err != nil {
		t.Fatal(err)
	}
	assertNotError(t, result)

	commits := fc.getCommitLog()
	if len(commits) != 1 {
		t.Fatalf("commit count = %d, want 1", len(commits))
	}
	var body map[string]struct {
		Payload struct {
			Rr *struct {
				Of []struct {
					Wd *int `json:"wd"`
				} `json:"of"`
			} `json:"rr"`
		} `json:"p"`
	}
	if err := json.Unmarshal(commits[0], &body); err != nil {
		t.Fatal(err)
	}
	for _, item := range body {
		if rr := item.Payload.Rr; rr != nil {
			if len(rr.Of) != 1 || rr.Of[0].Wd == nil || *rr.Of[0].Wd != int(time.Monday) {
				t.Fatalf("weekly rule = %s, want wd=%d (Monday)", commits[0], time.Monday)
			}
			return
		}
	}
	t.Fatalf("no recurrence template in commit %s", commits[0])
}

// The zone comes from deployment config. MCP_TIMEZONE wins over TZ, which a
// base image may set; a typo must degrade to UTC instead of blocking startup.
func TestLoadUserTimeZone(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"MCP_TIMEZONE wins", map[string]string{"MCP_TIMEZONE": "Europe/Berlin", "TZ": "America/New_York"}, "Europe/Berlin"},
		{"TZ fallback", map[string]string{"TZ": "America/New_York"}, "America/New_York"},
		{"invalid MCP_TIMEZONE falls through to TZ", map[string]string{"MCP_TIMEZONE": "Mars/Olympus", "TZ": "Asia/Tokyo"}, "Asia/Tokyo"},
		{"invalid only is UTC", map[string]string{"MCP_TIMEZONE": "Mars/Olympus"}, "UTC"},
		{"unset is UTC", map[string]string{}, "UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadUserTimeZone(func(key string) string { return tc.env[key] }); got.String() != tc.want {
				t.Errorf("loadUserTimeZone = %s, want %s", got, tc.want)
			}
		})
	}
}
