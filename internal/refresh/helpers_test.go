package refresh

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestMatchesDomain(t *testing.T) {
	tests := []struct {
		email   string
		domains []string
		want    bool
	}{
		{"user@example.com", []string{"example.com"}, true},
		{"user@example.com", []string{"other.com"}, false},
		{"user@example.com", []string{"other.com", "example.com"}, true},
		{"user@sub.example.com", []string{"example.com"}, false},
		{"", []string{"example.com"}, false},
		{"user@example.com", nil, false},
		{"user@example.com", []string{}, false},
	}
	for _, tt := range tests {
		got := matchesDomain(tt.email, tt.domains)
		if got != tt.want {
			t.Errorf("matchesDomain(%q, %v) = %v, want %v", tt.email, tt.domains, got, tt.want)
		}
	}
}

func TestHasCommitmentLanguage(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"I'll send you the report", true},
		{"I will handle this", true},
		{"Let me check on that", true},
		{"No commitments here", false},
		{"Just a regular message", false},
		{"I'LL SEND IT", true}, // case insensitive
		{"action item: review PR", true},
		{"", false},
	}
	for _, tt := range tests {
		got := hasCommitmentLanguage(tt.text)
		if got != tt.want {
			t.Errorf("hasCommitmentLanguage(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

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
		got := cleanJSON(tt.input)
		if got != tt.want {
			t.Errorf("cleanJSON(%q) = %q, want %q", tt.input, got, tt.want)
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

func TestActiveThreads(t *testing.T) {
	threads := []db.Thread{
		{ID: "1", Title: "Active", Status: "active"},
		{ID: "2", Title: "Paused", Status: "paused"},
		{ID: "3", Title: "Done", Status: "done"},
	}
	result := activeThreads(threads)
	if len(result) != 1 {
		t.Fatalf("expected 1 active thread, got %d", len(result))
	}
	if result[0].ID != "1" {
		t.Errorf("expected thread ID 1, got %s", result[0].ID)
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
		Threads: []db.Thread{{ID: "th1", Title: "Thread from source 2"}},
		Todos:   []db.Todo{{ID: "t2", Title: "Todo from source 2"}},
	}

	result := combineResults([]*SourceResult{r1, nil, r2})

	if len(result.Calendar.Today) != 1 {
		t.Errorf("expected 1 calendar event, got %d", len(result.Calendar.Today))
	}
	if len(result.Todos) != 2 {
		t.Errorf("expected 2 todos, got %d", len(result.Todos))
	}
	if len(result.Threads) != 1 {
		t.Errorf("expected 1 thread, got %d", len(result.Threads))
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
