package agent

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNativeLogPath(t *testing.T) {
	// NativeLogPath uses os.UserHomeDir internally, so we just verify the
	// structure relative to whatever home it resolves.
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}

	tests := []struct {
		name       string
		projectDir string
		sessionID  string
		wantSuffix string // relative to home
	}{
		{
			name:       "simple path",
			projectDir: "/Users/aaron/project",
			sessionID:  "abc-123",
			wantSuffix: filepath.Join(".claude", "projects", "Users-aaron-project", "abc-123.jsonl"),
		},
		{
			name:       "deep nested path",
			projectDir: "/Users/aaron/Personal/claude-command-center",
			sessionID:  "sess-456",
			wantSuffix: filepath.Join(".claude", "projects", "Users-aaron-Personal-claude-command-center", "sess-456.jsonl"),
		},
		{
			name:       "root-level path",
			projectDir: "/tmp",
			sessionID:  "root-sess",
			wantSuffix: filepath.Join(".claude", "projects", "tmp", "root-sess.jsonl"),
		},
		{
			name:       "single depth",
			projectDir: "/opt",
			sessionID:  "s1",
			wantSuffix: filepath.Join(".claude", "projects", "opt", "s1.jsonl"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NativeLogPath(tc.projectDir, tc.sessionID)
			want := filepath.Join(home, tc.wantSuffix)
			if got != want {
				t.Errorf("NativeLogPath(%q, %q)\n  got:  %s\n  want: %s", tc.projectDir, tc.sessionID, got, want)
			}
		})
	}
}

func TestTailNativeLog_ParsesJSONL(t *testing.T) {
	// Write fake JSONL to a temp file, verify events come through the channel.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.jsonl")

	// Create the file with initial content.
	events := []map[string]interface{}{
		{"type": "system", "message": "hello"},
		{"type": "assistant", "message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "world"},
			},
		}},
	}

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		line, _ := json.Marshal(ev)
		f.Write(line)
		f.Write([]byte("\n"))
	}
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh := make(chan map[string]interface{}, 64)
	go tailNativeLog(ctx, logPath, 0, eventCh)

	// Collect the two events.
	var received []map[string]interface{}
	for i := 0; i < 2; i++ {
		select {
		case ev := <-eventCh:
			received = append(received, ev)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for event %d", i)
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0]["type"] != "system" {
		t.Errorf("event 0: expected type=system, got %v", received[0]["type"])
	}
	if received[1]["type"] != "assistant" {
		t.Errorf("event 1: expected type=assistant, got %v", received[1]["type"])
	}
}

func TestTailNativeLog_FileAppearsAfterDelay(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "delayed.jsonl")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh := make(chan map[string]interface{}, 64)
	go tailNativeLog(ctx, logPath, 0, eventCh)

	// Create the file after a short delay.
	time.Sleep(500 * time.Millisecond)
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	ev := map[string]interface{}{"type": "system", "message": "delayed"}
	line, _ := json.Marshal(ev)
	f.Write(line)
	f.Write([]byte("\n"))
	f.Close()

	select {
	case got := <-eventCh:
		if got["type"] != "system" {
			t.Errorf("expected type=system, got %v", got["type"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for delayed event")
	}
}

func TestTailNativeLog_IncrementalWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "incremental.jsonl")

	// Create file with one event.
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}

	ev1, _ := json.Marshal(map[string]interface{}{"type": "first"})
	f.Write(ev1)
	f.Write([]byte("\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh := make(chan map[string]interface{}, 64)
	go tailNativeLog(ctx, logPath, 0, eventCh)

	// Read first event.
	select {
	case got := <-eventCh:
		if got["type"] != "first" {
			t.Errorf("expected type=first, got %v", got["type"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for first event")
	}

	// Write a second event after a poll interval.
	time.Sleep(300 * time.Millisecond)
	ev2, _ := json.Marshal(map[string]interface{}{"type": "second"})
	f.Write(ev2)
	f.Write([]byte("\n"))
	f.Sync()

	select {
	case got := <-eventCh:
		if got["type"] != "second" {
			t.Errorf("expected type=second, got %v", got["type"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for second event")
	}

	f.Close()
}

func TestTailNativeLog_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "cancel.jsonl")

	// Create an empty file so tailNativeLog opens it and enters the poll loop.
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	eventCh := make(chan map[string]interface{}, 64)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tailNativeLog(ctx, logPath, 0, eventCh)
	}()

	// Cancel and verify the goroutine returns promptly.
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good — goroutine exited.
	case <-time.After(2 * time.Second):
		t.Fatal("tailNativeLog did not exit after context cancellation")
	}
}

func TestExtractUsageFromEvent(t *testing.T) {
	tests := []struct {
		name       string
		event      map[string]interface{}
		wantInput  int
		wantOutput int
		wantOK     bool
	}{
		{
			name: "valid assistant event with usage",
			event: map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"stop_reason": "end_turn",
					"usage": map[string]interface{}{
						"input_tokens":  float64(1500),
						"output_tokens": float64(350),
					},
				},
			},
			wantInput:  1500,
			wantOutput: 350,
			wantOK:     true,
		},
		{
			name: "no message field",
			event: map[string]interface{}{
				"type": "system",
			},
			wantOK: false,
		},
		{
			name: "no stop_reason",
			event: map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"usage": map[string]interface{}{
						"input_tokens":  float64(100),
						"output_tokens": float64(50),
					},
				},
			},
			wantOK: false,
		},
		{
			name: "nil stop_reason",
			event: map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"stop_reason": nil,
					"usage": map[string]interface{}{
						"input_tokens":  float64(100),
						"output_tokens": float64(50),
					},
				},
			},
			wantOK: false,
		},
		{
			name: "no usage field",
			event: map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"stop_reason": "end_turn",
				},
			},
			wantOK: false,
		},
		{
			name: "missing output_tokens",
			event: map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"stop_reason": "end_turn",
					"usage": map[string]interface{}{
						"input_tokens": float64(100),
					},
				},
			},
			wantOK: false,
		},
		{
			name:   "empty event",
			event:  map[string]interface{}{},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input, output, ok := extractUsageFromEvent(tc.event)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok {
				if input != tc.wantInput {
					t.Errorf("inputTokens = %d, want %d", input, tc.wantInput)
				}
				if output != tc.wantOutput {
					t.Errorf("outputTokens = %d, want %d", output, tc.wantOutput)
				}
			}
		})
	}
}

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name     string
		event    map[string]interface{}
		input    int
		output   int
		wantCost float64
	}{
		{
			name: "opus model",
			event: map[string]interface{}{
				"message": map[string]interface{}{
					"model": "claude-opus-4-20250514",
				},
			},
			input:  1000,
			output: 500,
			// (1000 * 15 / 1_000_000) + (500 * 75 / 1_000_000) = 0.015 + 0.0375 = 0.0525
			wantCost: 0.0525,
		},
		{
			name: "sonnet model",
			event: map[string]interface{}{
				"message": map[string]interface{}{
					"model": "claude-sonnet-4-20250514",
				},
			},
			input:  1000,
			output: 500,
			// (1000 * 3 / 1_000_000) + (500 * 15 / 1_000_000) = 0.003 + 0.0075 = 0.0105
			wantCost: 0.0105,
		},
		{
			name: "unknown model defaults to sonnet pricing",
			event: map[string]interface{}{
				"message": map[string]interface{}{
					"model": "claude-haiku-3-20250514",
				},
			},
			input:    1000,
			output:   500,
			wantCost: 0.0105, // same as sonnet
		},
		{
			name:  "no model field defaults to sonnet pricing",
			event: map[string]interface{}{},
			input: 1000, output: 500,
			wantCost: 0.0105,
		},
		{
			name: "zero tokens",
			event: map[string]interface{}{
				"message": map[string]interface{}{
					"model": "claude-opus-4-20250514",
				},
			},
			input:    0,
			output:   0,
			wantCost: 0.0,
		},
		{
			name: "large token counts",
			event: map[string]interface{}{
				"message": map[string]interface{}{
					"model": "claude-opus-4-20250514",
				},
			},
			input:  100000,
			output: 10000,
			// (100000 * 15 / 1_000_000) + (10000 * 75 / 1_000_000) = 1.5 + 0.75 = 2.25
			wantCost: 2.25,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := estimateCost(tc.event, tc.input, tc.output)
			if math.Abs(got-tc.wantCost) > 0.000001 {
				t.Errorf("estimateCost() = %f, want %f", got, tc.wantCost)
			}
		})
	}
}

func TestSendUserMessage_PlainText(t *testing.T) {
	// SendUserMessage now writes plain text + newline to the PTY.
	// We use an os.Pipe to simulate the PTY file descriptor.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	sess := &Session{
		Pty:    w,
		Status: "blocked",
		done:   make(chan struct{}),
	}

	if err := SendUserMessage(sess, "hello world"); err != nil {
		t.Fatalf("SendUserMessage failed: %v", err)
	}

	// Read what was written.
	buf := make([]byte, 256)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}
	got := string(buf[:n])
	want := "hello world\n"
	if got != want {
		t.Errorf("written data = %q, want %q", got, want)
	}

	// Verify status was reset from blocked to processing.
	sess.Mu.Lock()
	status := sess.Status
	question := sess.Question
	sess.Mu.Unlock()

	if status != "processing" {
		t.Errorf("status = %q, want %q", status, "processing")
	}
	if question != "" {
		t.Errorf("question = %q, want empty", question)
	}
}

func TestSendUserMessage_NilPty(t *testing.T) {
	sess := &Session{
		Pty:  nil,
		done: make(chan struct{}),
	}
	err := SendUserMessage(sess, "hello")
	if err == nil {
		t.Error("expected error when PTY is nil")
	}
	if !strings.Contains(err.Error(), "PTY") {
		t.Errorf("error should mention PTY, got: %v", err)
	}
}
