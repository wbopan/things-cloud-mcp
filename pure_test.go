package main

import (
	"encoding/json"
	"hash/crc32"
	"testing"
	"time"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

// ---------------------------------------------------------------------------
// parseDate
// ---------------------------------------------------------------------------

func TestParseDate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantNil bool
		check  func(t *testing.T, got *time.Time)
	}{
		{
			name:  "YYYY-MM-DD returns UTC midnight",
			input: "2025-03-15",
			check: func(t *testing.T, got *time.Time) {
				want := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "RFC3339 with timezone",
			input: "2025-03-15T10:00:00+08:00",
			check: func(t *testing.T, got *time.Time) {
				want, _ := time.Parse(time.RFC3339, "2025-03-15T10:00:00+08:00")
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:    "empty string returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:    "invalid string returns nil",
			input:   "bad",
			wantNil: true,
		},
		{
			name:    "partial date returns nil",
			input:   "2025-13-01",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDate(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseTime
// ---------------------------------------------------------------------------

func TestParseTime(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSecs  int
		wantValid bool
	}{
		{"09:30", "09:30", 34200, true},
		{"23:59", "23:59", 86340, true},
		{"00:00", "00:00", 0, true},
		{"invalid hour", "25:00", 0, false},
		{"invalid minute", "12:60", 0, false},
		{"no colon", "1200", 0, false},
		{"negative hour", "-1:00", 0, false},
		{"empty", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secs, valid := parseTime(tt.input)
			if valid != tt.wantValid {
				t.Errorf("valid: got %v, want %v", valid, tt.wantValid)
			}
			if valid && secs != tt.wantSecs {
				t.Errorf("secs: got %d, want %d", secs, tt.wantSecs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseRecurrence
// ---------------------------------------------------------------------------

func TestParseRecurrence(t *testing.T) {
	ref := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC) // Saturday

	t.Run("empty returns nil", func(t *testing.T) {
		rr, err := parseRecurrence("", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rr != nil {
			t.Fatal("expected nil")
		}
	})

	t.Run("none returns nil", func(t *testing.T) {
		rr, err := parseRecurrence("none", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rr != nil {
			t.Fatal("expected nil")
		}
	})

	t.Run("daily", func(t *testing.T) {
		rr, err := parseRecurrence("daily", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rr == nil {
			t.Fatal("expected non-nil")
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		if m["fu"] != float64(16) {
			t.Errorf("fu: got %v, want 16", m["fu"])
		}
		if m["fa"] != float64(1) {
			t.Errorf("fa: got %v, want 1", m["fa"])
		}
	})

	t.Run("weekly:mon,wed", func(t *testing.T) {
		rr, err := parseRecurrence("weekly:mon,wed", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		if m["fu"] != float64(256) {
			t.Errorf("fu: got %v, want 256", m["fu"])
		}
		of, ok := m["of"].([]any)
		if !ok {
			t.Fatalf("of: expected array, got %T", m["of"])
		}
		if len(of) != 2 {
			t.Fatalf("of: expected 2 entries, got %d", len(of))
		}
		// Check weekday values: mon=1, wed=3
		entry0, _ := of[0].(map[string]any)
		entry1, _ := of[1].(map[string]any)
		if entry0["wd"] != float64(1) {
			t.Errorf("of[0].wd: got %v, want 1", entry0["wd"])
		}
		if entry1["wd"] != float64(3) {
			t.Errorf("of[1].wd: got %v, want 3", entry1["wd"])
		}
	})

	t.Run("monthly:15", func(t *testing.T) {
		rr, err := parseRecurrence("monthly:15", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		if m["fu"] != float64(8) {
			t.Errorf("fu: got %v, want 8", m["fu"])
		}
		of, _ := m["of"].([]any)
		entry, _ := of[0].(map[string]any)
		// monthly:15 → dy = 15-1 = 14
		if entry["dy"] != float64(14) {
			t.Errorf("of[0].dy: got %v, want 14", entry["dy"])
		}
	})

	t.Run("every 3 days", func(t *testing.T) {
		rr, err := parseRecurrence("every 3 days", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		if m["fu"] != float64(16) {
			t.Errorf("fu: got %v, want 16", m["fu"])
		}
		if m["fa"] != float64(3) {
			t.Errorf("fa: got %v, want 3", m["fa"])
		}
	})

	t.Run("every 2 weeks", func(t *testing.T) {
		rr, err := parseRecurrence("every 2 weeks", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		if m["fu"] != float64(256) {
			t.Errorf("fu: got %v, want 256", m["fu"])
		}
		if m["fa"] != float64(2) {
			t.Errorf("fa: got %v, want 2", m["fa"])
		}
	})

	t.Run("yearly", func(t *testing.T) {
		rr, err := parseRecurrence("yearly", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		if m["fu"] != float64(4) {
			t.Errorf("fu: got %v, want 4", m["fu"])
		}
	})

	t.Run("monthly:last", func(t *testing.T) {
		rr, err := parseRecurrence("monthly:last", ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var m map[string]any
		json.Unmarshal(*rr, &m)
		of, _ := m["of"].([]any)
		entry, _ := of[0].(map[string]any)
		if entry["dy"] != float64(-1) {
			t.Errorf("of[0].dy: got %v, want -1", entry["dy"])
		}
	})

	t.Run("bad returns error", func(t *testing.T) {
		_, err := parseRecurrence("bad", ref)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// parseWeekdays
// ---------------------------------------------------------------------------

func TestParseWeekdays(t *testing.T) {
	t.Run("mon,wed,fri", func(t *testing.T) {
		result, err := parseWeekdays("mon,wed,fri")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(result))
		}
		wds := []int{1, 3, 5}
		for i, want := range wds {
			if result[i]["wd"] != want {
				t.Errorf("result[%d][wd]: got %v, want %d", i, result[i]["wd"], want)
			}
		}
	})

	t.Run("bad weekday returns error", func(t *testing.T) {
		_, err := parseWeekdays("bad")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("all days", func(t *testing.T) {
		result, err := parseWeekdays("sun,mon,tue,wed,thu,fri,sat")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 7 {
			t.Fatalf("expected 7 entries, got %d", len(result))
		}
	})
}

// ---------------------------------------------------------------------------
// textNote / emptyNote — CRC32 correctness
// ---------------------------------------------------------------------------

func TestTextNote(t *testing.T) {
	text := "Hello, world!"
	note := textNote(text)

	if note.TypeTag != "tx" {
		t.Errorf("TypeTag: got %q, want %q", note.TypeTag, "tx")
	}
	if note.Type != 1 {
		t.Errorf("Type: got %d, want 1", note.Type)
	}
	if note.Value != text {
		t.Errorf("Value: got %q, want %q", note.Value, text)
	}
	expectedCRC := int64(crc32.ChecksumIEEE([]byte(text)))
	if note.Checksum != expectedCRC {
		t.Errorf("Checksum: got %d, want %d", note.Checksum, expectedCRC)
	}
}

func TestEmptyNote(t *testing.T) {
	note := emptyNote()

	if note.TypeTag != "tx" {
		t.Errorf("TypeTag: got %q, want %q", note.TypeTag, "tx")
	}
	if note.Checksum != 0 {
		t.Errorf("Checksum: got %d, want 0", note.Checksum)
	}
	if note.Value != "" {
		t.Errorf("Value: got %q, want empty", note.Value)
	}
	if note.Type != 1 {
		t.Errorf("Type: got %d, want 1", note.Type)
	}
}

// ---------------------------------------------------------------------------
// scheduleString
// ---------------------------------------------------------------------------

func TestScheduleString(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name     string
		schedule thingscloud.TaskSchedule
		date     *time.Time
		want     string
	}{
		{"st=0 → inbox", thingscloud.TaskScheduleInbox, nil, "inbox"},
		{"st=1 + no date → anytime", thingscloud.TaskScheduleAnytime, nil, "anytime"},
		{"st=1 + past date → anytime", thingscloud.TaskScheduleAnytime, &past, "anytime"},
		{"st=1 + future date → anytime", thingscloud.TaskScheduleAnytime, &future, "anytime"},
		{"st=2 + no date → someday", thingscloud.TaskScheduleSomeday, nil, "someday"},
		{"st=2 + date → upcoming", thingscloud.TaskScheduleSomeday, &future, "upcoming"},
		{"unknown schedule → inbox", thingscloud.TaskSchedule(99), nil, "inbox"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scheduleString(tt.schedule, tt.date)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// statusString
// ---------------------------------------------------------------------------

func TestStatusString(t *testing.T) {
	tests := []struct {
		status thingscloud.TaskStatus
		want   string
	}{
		{thingscloud.TaskStatusPending, "pending"},
		{thingscloud.TaskStatusCompleted, "completed"},
		{thingscloud.TaskStatusCanceled, "canceled"},
		{thingscloud.TaskStatus(99), "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := statusString(tt.status)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// noteChecksum
// ---------------------------------------------------------------------------

func TestNoteChecksum(t *testing.T) {
	text := "test note"
	got := noteChecksum(text)
	want := int64(crc32.ChecksumIEEE([]byte(text)))
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// offsetToTime
// ---------------------------------------------------------------------------

func TestOffsetToTime(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{34200, "09:30"},
		{0, "00:00"},
		{86340, "23:59"},
		{3600, "01:00"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := offsetToTime(tt.secs)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
