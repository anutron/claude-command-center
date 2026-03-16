package commandcenter

import (
	"testing"
)

func TestParseSessionEvent_AssistantText(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "Hello, I'll help you with that.",
			},
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "assistant_text" {
		t.Errorf("expected type assistant_text, got %q", ev.Type)
	}
	if ev.Text != "Hello, I'll help you with that." {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_ToolUse(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type":  "tool_use",
				"name":  "Read",
				"id":    "tool_abc123",
				"input": map[string]interface{}{"file_path": "/tmp/test.go"},
			},
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %q", ev.Type)
	}
	if ev.ToolName != "Read" {
		t.Errorf("expected tool name Read, got %q", ev.ToolName)
	}
	if ev.ToolID != "tool_abc123" {
		t.Errorf("expected tool id tool_abc123, got %q", ev.ToolID)
	}
}

func TestParseSessionEvent_ToolResult(t *testing.T) {
	raw := map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "tool_abc123",
		"content":     "file contents here",
		"is_error":    false,
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "tool_result" {
		t.Errorf("expected type tool_result, got %q", ev.Type)
	}
	if ev.ResultToolID != "tool_abc123" {
		t.Errorf("expected result tool id tool_abc123, got %q", ev.ResultToolID)
	}
	if ev.ResultText != "file contents here" {
		t.Errorf("unexpected result text: %q", ev.ResultText)
	}
	if ev.IsError {
		t.Error("expected IsError to be false")
	}
}

func TestParseSessionEvent_ToolResultError(t *testing.T) {
	raw := map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "tool_xyz",
		"content":     "permission denied",
		"is_error":    true,
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "tool_result" {
		t.Errorf("expected type tool_result, got %q", ev.Type)
	}
	if !ev.IsError {
		t.Error("expected IsError to be true")
	}
	if ev.ResultText != "permission denied" {
		t.Errorf("unexpected result text: %q", ev.ResultText)
	}
}

func TestParseSessionEvent_ToolResultContentBlocks(t *testing.T) {
	raw := map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "tool_blk",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "block content",
			},
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "tool_result" {
		t.Errorf("expected type tool_result, got %q", ev.Type)
	}
	if ev.ResultText != "block content" {
		t.Errorf("unexpected result text: %q", ev.ResultText)
	}
}

func TestParseSessionEvent_Result(t *testing.T) {
	raw := map[string]interface{}{
		"type":   "result",
		"result": "Task completed successfully.",
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "assistant_text" {
		t.Errorf("expected type assistant_text, got %q", ev.Type)
	}
	if ev.Text != "Task completed successfully." {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_ResultWithContent(t *testing.T) {
	raw := map[string]interface{}{
		"type": "result",
		"result": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Final output text",
				},
			},
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "assistant_text" {
		t.Errorf("expected type assistant_text, got %q", ev.Type)
	}
	if ev.Text != "Final output text" {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_Error(t *testing.T) {
	raw := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"message": "rate limit exceeded",
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "error" {
		t.Errorf("expected type error, got %q", ev.Type)
	}
	if !ev.IsError {
		t.Error("expected IsError to be true")
	}
	if ev.Text != "rate limit exceeded" {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_ErrorFallbackMessage(t *testing.T) {
	raw := map[string]interface{}{
		"type":    "error",
		"message": "something went wrong",
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "error" {
		t.Errorf("expected type error, got %q", ev.Type)
	}
	if ev.Text != "something went wrong" {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_User(t *testing.T) {
	raw := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"content": "please fix the bug",
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "user" {
		t.Errorf("expected type user, got %q", ev.Type)
	}
	if ev.Text != "please fix the bug" {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_System(t *testing.T) {
	raw := map[string]interface{}{
		"type":    "system",
		"message": "session started",
	}

	events := parseSessionEvent(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != "system" {
		t.Errorf("expected type system, got %q", ev.Type)
	}
	if ev.Text != "session started" {
		t.Errorf("unexpected text: %q", ev.Text)
	}
}

func TestParseSessionEvent_Unknown(t *testing.T) {
	raw := map[string]interface{}{
		"type": "unknown_type",
	}

	events := parseSessionEvent(raw)
	if len(events) != 0 {
		t.Errorf("expected 0 events for unknown type, got %d", len(events))
	}
}

func TestParseSessionEvent_MixedContentBlocks(t *testing.T) {
	// When an assistant event has both text and tool_use, each becomes its own event
	raw := map[string]interface{}{
		"type": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "Let me read that file.",
			},
			map[string]interface{}{
				"type":  "tool_use",
				"name":  "Read",
				"id":    "tool_mixed",
				"input": map[string]interface{}{"file_path": "/tmp/test.go"},
			},
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 2 {
		t.Fatalf("expected 2 events for mixed content, got %d", len(events))
	}

	// First event: text
	if events[0].Type != "assistant_text" {
		t.Errorf("expected first event type assistant_text, got %q", events[0].Type)
	}
	if events[0].Text != "Let me read that file." {
		t.Errorf("unexpected first event text: %q", events[0].Text)
	}

	// Second event: tool_use
	if events[1].Type != "tool_use" {
		t.Errorf("expected second event type tool_use, got %q", events[1].Type)
	}
	if events[1].ToolName != "Read" {
		t.Errorf("expected tool name Read, got %q", events[1].ToolName)
	}
	if events[1].ToolID != "tool_mixed" {
		t.Errorf("expected tool id tool_mixed, got %q", events[1].ToolID)
	}
}

func TestParseSessionEvent_MultipleToolUseBlocks(t *testing.T) {
	// Assistant message with multiple tool_use blocks
	raw := map[string]interface{}{
		"type": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "I'll read both files.",
			},
			map[string]interface{}{
				"type":  "tool_use",
				"name":  "Read",
				"id":    "tool_1",
				"input": map[string]interface{}{"file_path": "/tmp/a.go"},
			},
			map[string]interface{}{
				"type":  "tool_use",
				"name":  "Read",
				"id":    "tool_2",
				"input": map[string]interface{}{"file_path": "/tmp/b.go"},
			},
		},
	}

	events := parseSessionEvent(raw)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != "assistant_text" {
		t.Errorf("expected first event assistant_text, got %q", events[0].Type)
	}
	if events[1].Type != "tool_use" || events[1].ToolID != "tool_1" {
		t.Errorf("expected second event tool_use with id tool_1, got type=%q id=%q", events[1].Type, events[1].ToolID)
	}
	if events[2].Type != "tool_use" || events[2].ToolID != "tool_2" {
		t.Errorf("expected third event tool_use with id tool_2, got type=%q id=%q", events[2].Type, events[2].ToolID)
	}
}

func TestParseSessionEvent_AssistantEmptyContent(t *testing.T) {
	raw := map[string]interface{}{
		"type":    "assistant",
		"content": []interface{}{},
	}

	events := parseSessionEvent(raw)
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty content, got %d", len(events))
	}
}

func TestTruncateForViewer(t *testing.T) {
	// Normal text
	if got := truncateForViewer("hello", 10); got != "hello" {
		t.Errorf("expected hello, got %q", got)
	}

	// Newlines replaced
	if got := truncateForViewer("line1\nline2", 20); got != "line1 line2" {
		t.Errorf("expected 'line1 line2', got %q", got)
	}

	// Truncation
	long := "abcdefghijklmnop"
	if got := truncateForViewer(long, 10); got != "abcdefghij..." {
		t.Errorf("expected truncated string, got %q", got)
	}
}
