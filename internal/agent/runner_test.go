package agent

import (
	"strings"
	"testing"
)

func TestRunner_NewRunner(t *testing.T) {
	r := NewRunner(2)
	if len(r.Active()) != 0 {
		t.Error("expected empty active list")
	}
	if r.Status("nonexistent") != nil {
		t.Error("expected nil status for unknown ID")
	}
	if r.QueueLen() != 0 {
		t.Error("expected empty queue")
	}
}

func TestRunner_DefaultMaxConcurrent(t *testing.T) {
	// Passing 0 should default to 3
	r := NewRunner(0).(*defaultRunner)
	if r.maxConcurrent != 3 {
		t.Errorf("expected maxConcurrent=3, got %d", r.maxConcurrent)
	}
}

func TestRunner_KillNonexistent(t *testing.T) {
	r := NewRunner(2)
	if r.Kill("nonexistent") {
		t.Error("expected Kill to return false for nonexistent session")
	}
}

func TestRunner_SendMessageNonexistent(t *testing.T) {
	r := NewRunner(2)
	err := r.SendMessage("nonexistent", "hello")
	if err == nil {
		t.Error("expected error sending message to nonexistent session")
	}
}

func TestRunner_DrainQueueEmpty(t *testing.T) {
	r := NewRunner(2)
	_, ok := r.DrainQueue()
	if ok {
		t.Error("expected DrainQueue to return false on empty queue")
	}
}

func TestRunner_SessionNonexistent(t *testing.T) {
	r := NewRunner(2)
	if r.Session("nonexistent") != nil {
		t.Error("expected nil session for nonexistent ID")
	}
}

func TestRunner_CheckProcessesEmpty(t *testing.T) {
	r := NewRunner(2)
	cmd := r.CheckProcesses()
	if cmd != nil {
		t.Error("expected nil cmd from CheckProcesses with no active sessions")
	}
}

func TestDetectBlockingEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    map[string]interface{}
		expected bool
	}{
		{
			name: "tool_use SendUserMessage",
			event: map[string]interface{}{
				"type": "tool_use",
				"name": "SendUserMessage",
			},
			expected: true,
		},
		{
			name: "tool_use AskUser",
			event: map[string]interface{}{
				"type": "tool_use",
				"name": "AskUser",
			},
			expected: true,
		},
		{
			name: "tool_use other",
			event: map[string]interface{}{
				"type": "tool_use",
				"name": "Bash",
			},
			expected: false,
		},
		{
			name:     "empty event",
			event:    map[string]interface{}{},
			expected: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectBlockingEvent(tc.event); got != tc.expected {
				t.Errorf("DetectBlockingEvent() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestParseSessionEvent_AssistantText(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello world",
				},
			},
		},
	}
	events := ParseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "assistant_text" {
		t.Errorf("expected type assistant_text, got %s", events[0].Type)
	}
	if events[0].Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %s", events[0].Text)
	}
}

func TestParseSessionEvent_Error(t *testing.T) {
	raw := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"message": "something broke",
		},
	}
	events := ParseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "error" {
		t.Errorf("expected type error, got %s", events[0].Type)
	}
	if !events[0].IsError {
		t.Error("expected IsError to be true")
	}
}

func TestExtractBlockingQuestion(t *testing.T) {
	event := map[string]interface{}{
		"input": map[string]interface{}{
			"message": "What should I do?",
		},
	}
	q := ExtractBlockingQuestion(event)
	if q != "What should I do?" {
		t.Errorf("expected question 'What should I do?', got %s", q)
	}
}

func TestExtractSessionSummary(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		wantSub  string
		wantNot  string
	}{
		{
			name:     "empty output success",
			output:   "",
			exitCode: 0,
			wantSub:  "Session completed successfully",
		},
		{
			name:     "empty output failure",
			output:   "",
			exitCode: 1,
			wantSub:  "exited with code 1",
		},
		{
			name: "assistant text content",
			output: `{"type":"assistant","content":[{"type":"text","text":"I fixed the bug in main.go by updating the error handler."}]}
`,
			exitCode: 0,
			wantSub:  "I fixed the bug in main.go",
			wantNot:  `"type"`,
		},
		{
			name: "result event preferred over assistant",
			output: `{"type":"assistant","content":[{"type":"text","text":"Working on it..."}]}
{"type":"result","result":"Completed: fixed the login bug and added unit tests."}
`,
			exitCode: 0,
			wantSub:  "Completed: fixed the login bug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &Session{
				output:   &strings.Builder{},
				exitCode: tt.exitCode,
			}
			sess.output.WriteString(tt.output)

			got := ExtractSessionSummary(sess)
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("ExtractSessionSummary() = %q, want substring %q", got, tt.wantSub)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("ExtractSessionSummary() = %q, should not contain %q", got, tt.wantNot)
			}
		})
	}
}
