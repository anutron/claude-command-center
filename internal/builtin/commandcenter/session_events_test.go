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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
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

	ev := parseSessionEvent(raw)
	if ev.Type != "" {
		t.Errorf("expected empty type for unknown event, got %q", ev.Type)
	}
}

func TestParseSessionEvent_MixedContentBlocks(t *testing.T) {
	// When an assistant event has both text and tool_use, the last block wins
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

	ev := parseSessionEvent(raw)
	// The last content block (tool_use) should win
	if ev.Type != "tool_use" {
		t.Errorf("expected type tool_use for mixed content, got %q", ev.Type)
	}
	if ev.ToolName != "Read" {
		t.Errorf("expected tool name Read, got %q", ev.ToolName)
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
