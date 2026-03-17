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

func TestMerge_CompletedTodoNotOverwritten(t *testing.T) {
	existing := &db.CommandCenter{
		Todos: []db.Todo{
			{ID: "abc", Title: "Create Google Slides", Status: "completed", SourceRef: "meeting-123-abc"},
		},
	}
	fresh := &FreshData{
		Todos: []db.Todo{
			{Title: "Create Google Slides for data team", Source: "granola", SourceRef: "meeting-123-abc"},
		},
	}

	result := Merge(existing, fresh)
	for _, todo := range result.Todos {
		if todo.SourceRef == "meeting-123-abc" {
			if todo.Status != "completed" {
				t.Errorf("completed todo was overwritten, status = %q", todo.Status)
			}
			if todo.Title != "Create Google Slides" {
				t.Errorf("completed todo title was overwritten to %q", todo.Title)
			}
			return
		}
	}
	t.Error("completed todo was dropped entirely")
}

func TestMergeTriageStatus(t *testing.T) {
	t.Run("new external todo gets triage_status new", func(t *testing.T) {
		existing := &db.CommandCenter{}
		fresh := &FreshData{
			Todos: []db.Todo{
				{Title: "Review PR", Source: "github", SourceRef: "gh-999"},
			},
		}

		result := Merge(existing, fresh)
		if len(result.Todos) != 1 {
			t.Fatalf("expected 1 todo, got %d", len(result.Todos))
		}
		if result.Todos[0].TriageStatus != "new" {
			t.Errorf("expected triage_status 'new', got %q", result.Todos[0].TriageStatus)
		}
	})

	t.Run("fresh todo with no source_ref keeps its triage_status", func(t *testing.T) {
		existing := &db.CommandCenter{}
		fresh := &FreshData{
			Todos: []db.Todo{
				{Title: "Loose item", Source: "manual", TriageStatus: "accepted"},
			},
		}

		result := Merge(existing, fresh)
		if len(result.Todos) != 1 {
			t.Fatalf("expected 1 todo, got %d", len(result.Todos))
		}
		if result.Todos[0].TriageStatus != "accepted" {
			t.Errorf("expected triage_status 'accepted', got %q", result.Todos[0].TriageStatus)
		}
	})

	t.Run("existing todo triage_status preserved on merge", func(t *testing.T) {
		existing := &db.CommandCenter{
			Todos: []db.Todo{
				{ID: "t1", Title: "Old", Status: "active", SourceRef: "ref-1", TriageStatus: "accepted"},
			},
		}
		fresh := &FreshData{
			Todos: []db.Todo{
				{Title: "Updated", SourceRef: "ref-1", TriageStatus: "new"},
			},
		}

		result := Merge(existing, fresh)
		for _, todo := range result.Todos {
			if todo.SourceRef == "ref-1" {
				if todo.TriageStatus != "accepted" {
					t.Errorf("expected triage_status preserved as 'accepted', got %q", todo.TriageStatus)
				}
				return
			}
		}
		t.Error("todo with ref-1 not found")
	})

	t.Run("completed todo preserved as-is including triage_status", func(t *testing.T) {
		existing := &db.CommandCenter{
			Todos: []db.Todo{
				{ID: "t2", Title: "Done", Status: "completed", SourceRef: "ref-2", TriageStatus: "accepted"},
			},
		}
		fresh := &FreshData{
			Todos: []db.Todo{
				{Title: "Done Updated", SourceRef: "ref-2", TriageStatus: "new"},
			},
		}

		result := Merge(existing, fresh)
		for _, todo := range result.Todos {
			if todo.SourceRef == "ref-2" {
				if todo.Status != "completed" {
					t.Errorf("expected status 'completed', got %q", todo.Status)
				}
				if todo.TriageStatus != "accepted" {
					t.Errorf("expected triage_status preserved as 'accepted', got %q", todo.TriageStatus)
				}
				return
			}
		}
		t.Error("completed todo not found")
	})

	t.Run("dismissed todo remains tombstoned", func(t *testing.T) {
		existing := &db.CommandCenter{
			Todos: []db.Todo{
				{ID: "t3", Title: "Gone", Status: "dismissed", SourceRef: "ref-3", TriageStatus: "new"},
			},
		}
		fresh := &FreshData{
			Todos: []db.Todo{
				{Title: "Gone Recreated", Source: "granola", SourceRef: "ref-3"},
			},
		}

		result := Merge(existing, fresh)
		for _, todo := range result.Todos {
			if todo.SourceRef == "ref-3" && todo.Status != "dismissed" {
				t.Error("dismissed todo was recreated as non-dismissed")
			}
		}
	})
}

func TestMerge_SuggestionsPreserved(t *testing.T) {
	existing := &db.CommandCenter{
		Suggestions: db.Suggestions{
			Focus:         "Ship the calendar fix",
			RankedTodoIDs: []string{"todo-1", "todo-2"},
			Reasons:       map[string]string{"todo-1": "urgent deadline"},
		},
	}
	fresh := &FreshData{
		Todos: []db.Todo{
			{Title: "New task", Source: "granola", SourceRef: "g-1"},
		},
	}

	result := Merge(existing, fresh)
	if result.Suggestions.Focus != "Ship the calendar fix" {
		t.Errorf("expected suggestions.Focus preserved, got %q", result.Suggestions.Focus)
	}
	if len(result.Suggestions.RankedTodoIDs) != 2 {
		t.Errorf("expected 2 ranked todo IDs preserved, got %d", len(result.Suggestions.RankedTodoIDs))
	}
	if result.Suggestions.Reasons["todo-1"] != "urgent deadline" {
		t.Errorf("expected reasons preserved, got %v", result.Suggestions.Reasons)
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
