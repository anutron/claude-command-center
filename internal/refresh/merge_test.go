package refresh

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestMerge_CalendarReplacedEntirely(t *testing.T) {
	existing := &db.CommandCenter{
		Calendar: db.CalendarData{
			Today: []db.CalendarEvent{{Title: "Old Meeting"}},
		},
	}
	fresh := &FreshData{
		Calendar: db.CalendarData{
			Today: []db.CalendarEvent{{Title: "New Meeting"}},
		},
	}

	result := Merge(existing, fresh)
	if len(result.Calendar.Today) != 1 || result.Calendar.Today[0].Title != "New Meeting" {
		t.Errorf("expected calendar to be replaced, got %v", result.Calendar.Today)
	}
}

func TestMerge_DismissedTodoNeverRecreated(t *testing.T) {
	existing := &db.CommandCenter{
		Todos: []db.Todo{
			{ID: "abc", Title: "Old", Status: "dismissed", SourceRef: "granola-123"},
		},
	}
	fresh := &FreshData{
		Todos: []db.Todo{
			{Title: "Old Recreated", Source: "granola", SourceRef: "granola-123"},
		},
	}

	result := Merge(existing, fresh)
	active := 0
	for _, t := range result.Todos {
		if t.SourceRef == "granola-123" && t.Status != "dismissed" {
			active++
		}
	}
	if active != 0 {
		t.Errorf("dismissed todo was recreated as active")
	}
}

func TestMerge_ExistingTodoUpdated(t *testing.T) {
	existing := &db.CommandCenter{
		Todos: []db.Todo{
			{ID: "abc", Title: "Old Title", Status: "active", SourceRef: "ref-1",
				Detail: "old detail", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	fresh := &FreshData{
		Todos: []db.Todo{
			{Title: "New Title", SourceRef: "ref-1", Detail: "new detail", WhoWaiting: "Bob"},
		},
	}

	result := Merge(existing, fresh)
	found := false
	for _, todo := range result.Todos {
		if todo.SourceRef == "ref-1" {
			found = true
			if todo.ID != "abc" {
				t.Errorf("expected ID preserved as 'abc', got %q", todo.ID)
			}
			if todo.Title != "New Title" {
				t.Errorf("expected title updated to 'New Title', got %q", todo.Title)
			}
			if todo.Detail != "new detail" {
				t.Errorf("expected detail updated, got %q", todo.Detail)
			}
			if todo.Status != "active" {
				t.Errorf("expected status preserved as 'active', got %q", todo.Status)
			}
			if todo.CreatedAt.Year() != 2026 {
				t.Errorf("expected created_at preserved, got %v", todo.CreatedAt)
			}
		}
	}
	if !found {
		t.Error("todo with ref-1 not found in merged result")
	}
}

func TestMerge_NewTodoGetsID(t *testing.T) {
	existing := &db.CommandCenter{}
	fresh := &FreshData{
		Todos: []db.Todo{
			{Title: "Brand New", Source: "slack", SourceRef: "slack-456"},
		},
	}

	result := Merge(existing, fresh)
	if len(result.Todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(result.Todos))
	}
	if result.Todos[0].ID == "" {
		t.Error("expected new todo to get an ID")
	}
	if result.Todos[0].Status != "active" {
		t.Errorf("expected status 'active', got %q", result.Todos[0].Status)
	}
}

func TestMerge_ManualTodosPreserved(t *testing.T) {
	existing := &db.CommandCenter{
		Todos: []db.Todo{
			{ID: "manual-1", Title: "My Task", Status: "active", Source: "manual"},
		},
	}
	fresh := &FreshData{
		Todos: []db.Todo{
			{Title: "From Granola", Source: "granola", SourceRef: "g-1"},
		},
	}

	result := Merge(existing, fresh)
	found := false
	for _, todo := range result.Todos {
		if todo.ID == "manual-1" {
			found = true
		}
	}
	if !found {
		t.Error("manual todo was not preserved")
	}
}

func TestMerge_ThreadDismissedNotRecreated(t *testing.T) {
	existing := &db.CommandCenter{
		Threads: []db.Thread{
			{ID: "t1", Title: "Old PR", Status: "dismissed", URL: "https://github.com/pr/1"},
		},
	}
	fresh := &FreshData{
		Threads: []db.Thread{
			{Title: "Old PR Updated", URL: "https://github.com/pr/1", Type: "pr"},
		},
	}

	result := Merge(existing, fresh)
	active := 0
	for _, th := range result.Threads {
		if th.URL == "https://github.com/pr/1" && th.Status != "dismissed" && th.Status != "completed" {
			active++
		}
	}
	if active != 0 {
		t.Error("dismissed thread was recreated")
	}
}

func TestMerge_ThreadPauseStatePreserved(t *testing.T) {
	pausedAt := time.Now()
	existing := &db.CommandCenter{
		Threads: []db.Thread{
			{ID: "t1", Title: "PR", Status: "paused", URL: "https://github.com/pr/2",
				PausedAt: &pausedAt},
		},
	}
	fresh := &FreshData{
		Threads: []db.Thread{
			{Title: "PR Updated", URL: "https://github.com/pr/2", Summary: "3 comments"},
		},
	}

	result := Merge(existing, fresh)
	for _, th := range result.Threads {
		if th.URL == "https://github.com/pr/2" {
			if th.Status != "paused" {
				t.Errorf("expected status preserved as 'paused', got %q", th.Status)
			}
			if th.PausedAt == nil {
				t.Error("expected paused_at preserved")
			}
			if th.Summary != "3 comments" {
				t.Errorf("expected summary updated to '3 comments', got %q", th.Summary)
			}
			return
		}
	}
	t.Error("thread not found in result")
}

func TestMerge_PendingActionsPreserved(t *testing.T) {
	existing := &db.CommandCenter{
		PendingActions: []db.PendingAction{
			{Type: "booking", TodoID: "abc", DurationMinutes: 30},
		},
	}
	fresh := &FreshData{}

	result := Merge(existing, fresh)
	if len(result.PendingActions) != 1 {
		t.Errorf("expected pending actions preserved, got %d", len(result.PendingActions))
	}
}

func TestMerge_NilExisting(t *testing.T) {
	fresh := &FreshData{
		Calendar: db.CalendarData{
			Today: []db.CalendarEvent{{Title: "Meeting"}},
		},
		Todos: []db.Todo{
			{Title: "Task", Source: "granola", SourceRef: "g-1"},
		},
	}

	result := Merge(nil, fresh)
	if len(result.Calendar.Today) != 1 {
		t.Error("expected calendar data")
	}
	if len(result.Todos) != 1 {
		t.Error("expected 1 todo")
	}
}
