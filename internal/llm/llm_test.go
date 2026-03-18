package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNoopLLM_Complete(t *testing.T) {
	var l NoopLLM
	result, err := l.Complete(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("NoopLLM.Complete returned error: %v", err)
	}
	if result != "" {
		t.Errorf("NoopLLM.Complete returned %q, want empty string", result)
	}
}

func TestLLMInterfaceCompliance(t *testing.T) {
	var impls []LLM
	impls = append(impls, NoopLLM{}, ClaudeCLI{})
	if len(impls) != 2 {
		t.Fatalf("expected 2 implementations, got %d", len(impls))
	}
}

func TestBuildArgs_Default(t *testing.T) {
	c := ClaudeCLI{}
	args := c.buildArgs()
	expected := []string{"-p", "-", "--output-format", "text"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(expected), args)
	}
	for i, a := range expected {
		if args[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, args[i], a)
		}
	}
}

func TestBuildArgs_WithModel(t *testing.T) {
	c := ClaudeCLI{Model: "haiku"}
	args := c.buildArgs()
	if !contains(args, "--model") || !contains(args, "haiku") {
		t.Errorf("expected --model haiku in args: %v", args)
	}
}

func TestBuildArgs_WithTimeout(t *testing.T) {
	// Timeout wraps context, doesn't change args
	c := ClaudeCLI{Timeout: 90 * time.Second}
	args := c.buildArgs()
	expected := []string{"-p", "-", "--output-format", "text"}
	if len(args) != len(expected) {
		t.Fatalf("timeout should not add args, got %v", args)
	}
}

func TestBuildArgs_WithSandbox(t *testing.T) {
	noTools := ""
	c := ClaudeCLI{
		Tools:                &noTools,
		DisableSlashCommands: true,
	}
	args := c.buildArgs()
	if !contains(args, "--tools") {
		t.Errorf("expected --tools in args: %v", args)
	}
	if !contains(args, "--disable-slash-commands") {
		t.Errorf("expected --disable-slash-commands in args: %v", args)
	}
	// Verify --tools value is empty string
	for i, a := range args {
		if a == "--tools" && i+1 < len(args) {
			if args[i+1] != "" {
				t.Errorf("--tools value = %q, want empty string", args[i+1])
			}
		}
	}
}

func TestBuildArgs_WithModelAndSandbox(t *testing.T) {
	noTools := ""
	c := ClaudeCLI{
		Model:                "sonnet",
		Tools:                &noTools,
		DisableSlashCommands: true,
	}
	args := c.buildArgs()
	if !contains(args, "--model") || !contains(args, "sonnet") {
		t.Errorf("expected --model sonnet in args: %v", args)
	}
	if !contains(args, "--tools") || !contains(args, "--disable-slash-commands") {
		t.Errorf("expected sandbox flags in args: %v", args)
	}
}

func TestParseClaudeError_API500(t *testing.T) {
	stderr := `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	result := ParseClaudeError(stderr)
	if !strings.Contains(result, "overloaded_error") {
		t.Errorf("expected overloaded_error in result, got %q", result)
	}
}

func TestParseClaudeError_PlainText500(t *testing.T) {
	result := ParseClaudeError("API Error: 500 Internal Server Error")
	if !strings.Contains(result, "500") {
		t.Errorf("expected 500 in result, got %q", result)
	}
}

func TestParseClaudeError_Empty(t *testing.T) {
	result := ParseClaudeError("")
	if result != "unknown error" {
		t.Errorf("expected 'unknown error', got %q", result)
	}
}

func TestParseClaudeError_LongMessage(t *testing.T) {
	long := strings.Repeat("x", 100)
	result := ParseClaudeError(long)
	if len(result) > 80 {
		t.Errorf("expected truncated result, got length %d", len(result))
	}
}

func TestLogFailure(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "failures.jsonl")

	entry := FailureEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Operation: "edit",
		Prompt:    "test prompt",
		Error:     "test error",
		TodoID:    "todo-123",
	}

	if err := LogFailure(logPath, entry); err != nil {
		t.Fatalf("LogFailure returned error: %v", err)
	}

	// Write a second entry
	entry2 := FailureEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Operation: "focus",
		Prompt:    "another prompt",
		Error:     "another error",
	}
	if err := LogFailure(logPath, entry2); err != nil {
		t.Fatalf("LogFailure second call returned error: %v", err)
	}

	// Read back and verify JSON-lines format
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("could not read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var parsed FailureEntry
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("could not parse first line: %v", err)
	}
	if parsed.Operation != "edit" {
		t.Errorf("operation = %q, want 'edit'", parsed.Operation)
	}
	if parsed.TodoID != "todo-123" {
		t.Errorf("todo_id = %q, want 'todo-123'", parsed.TodoID)
	}
	if parsed.Prompt != "test prompt" {
		t.Errorf("prompt = %q, want 'test prompt'", parsed.Prompt)
	}

	var parsed2 FailureEntry
	if err := json.Unmarshal([]byte(lines[1]), &parsed2); err != nil {
		t.Fatalf("could not parse second line: %v", err)
	}
	if parsed2.Operation != "focus" {
		t.Errorf("operation = %q, want 'focus'", parsed2.Operation)
	}
	if parsed2.TodoID != "" {
		t.Errorf("todo_id = %q, want empty", parsed2.TodoID)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
