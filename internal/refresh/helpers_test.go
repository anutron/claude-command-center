package refresh

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestCleanJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"key": "value"}`, `{"key": "value"}`},
		{"```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"  {\"key\": \"value\"}  ", `{"key": "value"}`},
		{"", ""},
	}
	for _, tt := range tests {
		got := CleanJSON(tt.input)
		if got != tt.want {
			t.Errorf("CleanJSON(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestActiveTodos(t *testing.T) {
	todos := []db.Todo{
		{ID: "1", Title: "Active", Status: "active"},
		{ID: "2", Title: "Done", Status: "done"},
		{ID: "3", Title: "Dismissed", Status: "dismissed"},
		{ID: "4", Title: "Also Active", Status: "active"},
	}
	result := activeTodos(todos)
	if len(result) != 2 {
		t.Fatalf("expected 2 active todos, got %d", len(result))
	}
	if result[0].ID != "1" || result[1].ID != "4" {
		t.Error("unexpected active todo IDs")
	}
}

func TestCombineResults(t *testing.T) {
	cal := &db.CalendarData{
		Today: []db.CalendarEvent{{Title: "Meeting", Start: time.Now()}},
	}
	r1 := &SourceResult{
		Calendar: cal,
		Todos:    []db.Todo{{ID: "t1", Title: "Todo from source 1"}},
	}
	r2 := &SourceResult{
		Todos: []db.Todo{{ID: "t2", Title: "Todo from source 2"}},
	}

	result := combineResults([]*SourceResult{r1, nil, r2})

	if len(result.Calendar.Today) != 1 {
		t.Errorf("expected 1 calendar event, got %d", len(result.Calendar.Today))
	}
	if len(result.Todos) != 2 {
		t.Errorf("expected 2 todos, got %d", len(result.Todos))
	}
}

func TestCombineResults_StripsANSI(t *testing.T) {
	r := &SourceResult{
		Calendar: &db.CalendarData{
			Today:    []db.CalendarEvent{{Title: "\x1b[31mRed Meeting\x1b[0m"}},
			Tomorrow: []db.CalendarEvent{{Title: "\x1b]0;evil\x07Normal"}},
		},
		Todos: []db.Todo{{Title: "\x1b[1mBold Todo\x1b[0m", Context: "\x1b[32mgreen\x1b[0m", Detail: "\x1b[4munderline\x1b[0m"}},
	}

	result := combineResults([]*SourceResult{r})

	if result.Calendar.Today[0].Title != "Red Meeting" {
		t.Errorf("calendar today title not stripped: %q", result.Calendar.Today[0].Title)
	}
	if result.Calendar.Tomorrow[0].Title != "Normal" {
		t.Errorf("calendar tomorrow title not stripped: %q", result.Calendar.Tomorrow[0].Title)
	}
	if result.Todos[0].Title != "Bold Todo" {
		t.Errorf("todo title not stripped: %q", result.Todos[0].Title)
	}
	if result.Todos[0].Context != "green" {
		t.Errorf("todo context not stripped: %q", result.Todos[0].Context)
	}
	if result.Todos[0].Detail != "underline" {
		t.Errorf("todo detail not stripped: %q", result.Todos[0].Detail)
	}
}

func TestCombineResults_AllNil(t *testing.T) {
	result := combineResults([]*SourceResult{nil, nil})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Todos) != 0 {
		t.Errorf("expected 0 todos, got %d", len(result.Todos))
	}
}
