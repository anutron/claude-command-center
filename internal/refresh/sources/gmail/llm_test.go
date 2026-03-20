package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// stubLLM returns a canned response for any prompt.
type stubLLM struct {
	response string
	err      error
}

func (s stubLLM) Complete(_ context.Context, _ string) (string, error) {
	return s.response, s.err
}

func TestGenerateTodoTitles_Success(t *testing.T) {
	emails := []emailForTitleGen{
		{ID: "msg1", Subject: "Re: Q2 Planning", From: "alice@example.com", To: "aaron@example.com", Body: "Can you prepare the slide deck for the Q2 planning session?"},
		{ID: "msg2", Subject: "Quick question", From: "aaron@example.com", To: "bob@example.com", Body: "I'll have the migration script ready by Friday"},
	}

	response, _ := json.Marshal([]struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}{
		{ID: "msg1", Title: "Prepare slide deck for Q2 planning session"},
		{ID: "msg2", Title: "Complete migration script by Friday"},
	})

	l := stubLLM{response: string(response)}
	titles, err := generateTodoTitles(context.Background(), l, emails)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := titles["msg1"]; got != "Prepare slide deck for Q2 planning session" {
		t.Errorf("msg1 title = %q, want %q", got, "Prepare slide deck for Q2 planning session")
	}
	if got := titles["msg2"]; got != "Complete migration script by Friday" {
		t.Errorf("msg2 title = %q, want %q", got, "Complete migration script by Friday")
	}
}

func TestGenerateTodoTitles_EmptyInput(t *testing.T) {
	titles, err := generateTodoTitles(context.Background(), stubLLM{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if titles != nil {
		t.Errorf("expected nil, got %v", titles)
	}
}

func TestGenerateTodoTitles_LLMError(t *testing.T) {
	emails := []emailForTitleGen{
		{ID: "msg1", Subject: "Test", From: "a@b.com", To: "c@d.com", Body: "body"},
	}
	l := stubLLM{err: fmt.Errorf("api error")}
	_, err := generateTodoTitles(context.Background(), l, emails)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGenerateTodoTitles_PartialResponse(t *testing.T) {
	// LLM only returns a title for one of two emails
	emails := []emailForTitleGen{
		{ID: "msg1", Subject: "Subject 1", From: "a@b.com", To: "c@d.com", Body: "body1"},
		{ID: "msg2", Subject: "Subject 2", From: "a@b.com", To: "c@d.com", Body: "body2"},
	}

	response, _ := json.Marshal([]struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}{
		{ID: "msg1", Title: "Do the thing"},
	})

	l := stubLLM{response: string(response)}
	titles, err := generateTodoTitles(context.Background(), l, emails)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := titles["msg1"]; !ok {
		t.Error("expected title for msg1")
	}
	if _, ok := titles["msg2"]; ok {
		t.Error("expected no title for msg2 (LLM didn't return one)")
	}
}

func TestGenerateTodoTitles_EmptyTitleSkipped(t *testing.T) {
	emails := []emailForTitleGen{
		{ID: "msg1", Subject: "Test", From: "a@b.com", To: "c@d.com", Body: "body"},
	}

	response, _ := json.Marshal([]struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}{
		{ID: "msg1", Title: ""},
	})

	l := stubLLM{response: string(response)}
	titles, err := generateTodoTitles(context.Background(), l, emails)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := titles["msg1"]; ok {
		t.Error("expected empty title to be skipped")
	}
}
