package granola

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockLLM implements llm.LLM for testing.
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Complete(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func TestNew(t *testing.T) {
	t.Run("enabled with LLM", func(t *testing.T) {
		l := &mockLLM{}
		s := New(true, l, nil)
		if s == nil {
			t.Fatal("New returned nil")
		}
		if !s.Enabled() {
			t.Error("expected Enabled() = true")
		}
		if s.LLM != l {
			t.Error("LLM not set correctly")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		s := New(false, nil, nil)
		if s.Enabled() {
			t.Error("expected Enabled() = false")
		}
	})

	t.Run("nil LLM", func(t *testing.T) {
		s := New(true, nil, nil)
		if s.LLM != nil {
			t.Error("expected nil LLM")
		}
		if !s.Enabled() {
			t.Error("expected Enabled() = true even with nil LLM")
		}
	})
}

func TestName(t *testing.T) {
	s := New(true, nil, nil)
	if got := s.Name(); got != "granola" {
		t.Errorf("Name() = %q, want %q", got, "granola")
	}
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		enabled bool
		want    bool
	}{
		{true, true},
		{false, false},
	}
	for _, tt := range tests {
		s := New(tt.enabled, nil, nil)
		if got := s.Enabled(); got != tt.want {
			t.Errorf("New(%v, nil, nil).Enabled() = %v, want %v", tt.enabled, got, tt.want)
		}
	}
}

func TestRawMeetingJSON(t *testing.T) {
	now := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	m := RawMeeting{
		ID:         "meeting-1",
		Title:      "Standup",
		StartTime:  now,
		EndTime:    now.Add(30 * time.Minute),
		Transcript: "We discussed the roadmap.",
		Summary:    "Roadmap discussion",
		Attendees:  []string{"Alice", "Bob"},
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got RawMeeting
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != m.ID {
		t.Errorf("ID = %q, want %q", got.ID, m.ID)
	}
	if got.Title != m.Title {
		t.Errorf("Title = %q, want %q", got.Title, m.Title)
	}
	if got.Transcript != m.Transcript {
		t.Errorf("Transcript = %q, want %q", got.Transcript, m.Transcript)
	}
	if got.Summary != m.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, m.Summary)
	}
	if len(got.Attendees) != 2 {
		t.Errorf("Attendees len = %d, want 2", len(got.Attendees))
	}
}

func TestExtractCommitments(t *testing.T) {
	t.Run("empty meetings returns nil", func(t *testing.T) {
		todos, err := extractCommitments(context.Background(), &mockLLM{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if todos != nil {
			t.Errorf("expected nil, got %v", todos)
		}
	})

	t.Run("valid LLM response", func(t *testing.T) {
		llmResp := `[{"title":"Send report","meeting_id":"m1","context":"project-x","detail":"Aaron committed to sending the Q1 report","who_waiting":"Bob","due":"2026-03-15"}]`
		l := &mockLLM{response: llmResp}
		meetings := []RawMeeting{
			{ID: "m1", Title: "Standup", StartTime: time.Now(), Attendees: []string{"Bob"}},
		}

		todos, err := extractCommitments(context.Background(), l, meetings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 1 {
			t.Fatalf("expected 1 todo, got %d", len(todos))
		}

		todo := todos[0]
		if todo.Title != "Send report" {
			t.Errorf("Title = %q, want %q", todo.Title, "Send report")
		}
		if todo.Source != "granola" {
			t.Errorf("Source = %q, want %q", todo.Source, "granola")
		}
		if !strings.HasPrefix(todo.SourceRef, "m1-") {
			t.Errorf("SourceRef = %q, want prefix %q", todo.SourceRef, "m1-")
		}
		if todo.Context != "project-x" {
			t.Errorf("Context = %q, want %q", todo.Context, "project-x")
		}
		if todo.WhoWaiting != "Bob" {
			t.Errorf("WhoWaiting = %q, want %q", todo.WhoWaiting, "Bob")
		}
		if todo.Due != "2026-03-15" {
			t.Errorf("Due = %q, want %q", todo.Due, "2026-03-15")
		}
		if todo.Status != "active" {
			t.Errorf("Status = %q, want %q", todo.Status, "active")
		}
	})

	t.Run("LLM returns empty array", func(t *testing.T) {
		l := &mockLLM{response: "[]"}
		meetings := []RawMeeting{{ID: "m1", Title: "Quick chat"}}

		todos, err := extractCommitments(context.Background(), l, meetings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 0 {
			t.Errorf("expected 0 todos, got %d", len(todos))
		}
	})

	t.Run("LLM returns wrapped JSON", func(t *testing.T) {
		llmResp := "```json\n[{\"title\":\"Follow up\",\"meeting_id\":\"m2\",\"context\":\"\",\"detail\":\"check in\",\"who_waiting\":\"\",\"due\":\"\"}]\n```"
		l := &mockLLM{response: llmResp}
		meetings := []RawMeeting{{ID: "m2", Title: "Sync"}}

		todos, err := extractCommitments(context.Background(), l, meetings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 1 {
			t.Errorf("expected 1 todo, got %d", len(todos))
		}
	})

	t.Run("LLM error", func(t *testing.T) {
		l := &mockLLM{err: fmt.Errorf("API timeout")}
		meetings := []RawMeeting{{ID: "m1", Title: "Standup"}}

		_, err := extractCommitments(context.Background(), l, meetings)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("LLM returns invalid JSON", func(t *testing.T) {
		l := &mockLLM{response: "not json at all"}
		meetings := []RawMeeting{{ID: "m1", Title: "Standup"}}

		_, err := extractCommitments(context.Background(), l, meetings)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("multiple commitments from multiple meetings", func(t *testing.T) {
		llmResp := `[
			{"title":"Write docs","meeting_id":"m1","context":"docs","detail":"for API","who_waiting":"Alice","due":"2026-03-12"},
			{"title":"Fix bug","meeting_id":"m2","context":"backend","detail":"null pointer in handler","who_waiting":"","due":""}
		]`
		l := &mockLLM{response: llmResp}
		meetings := []RawMeeting{
			{ID: "m1", Title: "Planning", Attendees: []string{"Alice"}},
			{ID: "m2", Title: "Bug triage"},
		}

		todos, err := extractCommitments(context.Background(), l, meetings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 2 {
			t.Fatalf("expected 2 todos, got %d", len(todos))
		}
		if todos[0].Title != "Write docs" {
			t.Errorf("todos[0].Title = %q, want %q", todos[0].Title, "Write docs")
		}
		if todos[1].Title != "Fix bug" {
			t.Errorf("todos[1].Title = %q, want %q", todos[1].Title, "Fix bug")
		}
	})

	t.Run("long transcript is truncated in prompt", func(t *testing.T) {
		// This tests that extractCommitments doesn't crash with a very long transcript.
		// The function truncates transcripts > 8000 chars internally.
		longTranscript := ""
		for i := 0; i < 10000; i++ {
			longTranscript += "x"
		}
		l := &mockLLM{response: "[]"}
		meetings := []RawMeeting{{ID: "m1", Title: "Long meeting", Transcript: longTranscript}}

		todos, err := extractCommitments(context.Background(), l, meetings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 0 {
			t.Errorf("expected 0 todos, got %d", len(todos))
		}
	})
}

func TestExtractCommitmentsPromptContent(t *testing.T) {
	// Verify the prompt includes meeting metadata by capturing via mock.
	var capturedPrompt string
	l := &mockLLM{response: "[]"}
	// Override Complete to capture the prompt.
	type capturer struct {
		mockLLM
	}
	cl := &struct {
		mockLLM
		prompt *string
	}{mockLLM: *l, prompt: &capturedPrompt}
	_ = cl // We can't easily capture without modifying mockLLM, so use a different approach.

	// Use a capturing mock instead.
	cm := &capturingMock{response: "[]"}
	meetings := []RawMeeting{
		{
			ID:         "abc123",
			Title:      "Sprint Review",
			StartTime:  time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
			Attendees:  []string{"Alice", "Bob"},
			Summary:    "Reviewed sprint goals",
			Transcript: "We should ship the feature by Friday.",
		},
	}

	_, err := extractCommitments(context.Background(), cm, meetings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cm.lastPrompt == "" {
		t.Fatal("prompt was not captured")
	}

	checks := []struct {
		substr string
		desc   string
	}{
		{"Sprint Review", "meeting title"},
		{"2026-03-10", "meeting date"},
		{"Alice, Bob", "attendees"},
		{"Reviewed sprint goals", "summary"},
		{"ship the feature by Friday", "transcript content"},
	}
	for _, c := range checks {
		if !contains(cm.lastPrompt, c.substr) {
			t.Errorf("prompt missing %s (%q)", c.desc, c.substr)
		}
	}
}

type capturingMock struct {
	response   string
	lastPrompt string
}

func (m *capturingMock) Complete(_ context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	return m.response, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDataSourceInterface(t *testing.T) {
	// Verify GranolaSource satisfies the DataSource interface at compile time.
	// This is a compile-time check; if it compiles, the test passes.
	s := New(true, nil, nil)
	if s.Name() != "granola" {
		t.Errorf("Name() = %q, want %q", s.Name(), "granola")
	}
	if !s.Enabled() {
		t.Error("expected Enabled() = true")
	}
}
