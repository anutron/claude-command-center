package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockLLM is a test double that returns canned responses.
type mockLLM struct {
	output string
	err    error
}

func (m mockLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return m.output, m.err
}

func TestObservableLLM_PublishesStartedAndFinished(t *testing.T) {
	inner := mockLLM{output: `{"message":"ok"}`, err: nil}

	var mu sync.Mutex
	var events []struct {
		topic   string
		payload EventPayload
	}

	publish := func(topic string, payload EventPayload) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, struct {
			topic   string
			payload EventPayload
		}{topic, payload})
	}

	obs := NewObservableLLM(inner, publish, "command-center")
	ctx := WithOperation(context.Background(), "command")

	result, err := obs.Complete(ctx, "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"message":"ok"}` {
		t.Errorf("result = %q, want %q", result, `{"message":"ok"}`)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 events (started + finished), got %d", len(events))
	}

	if events[0].topic != "llm.started" {
		t.Errorf("event[0].topic = %q, want 'llm.started'", events[0].topic)
	}
	startedID, ok := events[0].payload["id"].(string)
	if !ok || startedID == "" {
		t.Error("started event should have a non-empty 'id' field")
	}
	if events[0].payload["operation"] != "command" {
		t.Errorf("started event operation = %v, want 'command'", events[0].payload["operation"])
	}
	if events[0].payload["source"] != "command-center" {
		t.Errorf("started event source = %v, want 'command-center'", events[0].payload["source"])
	}

	if events[1].topic != "llm.finished" {
		t.Errorf("event[1].topic = %q, want 'llm.finished'", events[1].topic)
	}
	finishedID, ok := events[1].payload["id"].(string)
	if !ok || finishedID == "" {
		t.Error("finished event should have a non-empty 'id' field")
	}
	if startedID != finishedID {
		t.Errorf("started id %q != finished id %q — should match", startedID, finishedID)
	}
	if events[1].payload["status"] != "completed" {
		t.Errorf("finished event status = %v, want 'completed'", events[1].payload["status"])
	}
	if _, ok := events[1].payload["duration_ms"]; !ok {
		t.Error("finished event should have 'duration_ms' field")
	}
}

func TestObservableLLM_PublishesErrorOnFailure(t *testing.T) {
	inner := mockLLM{output: "", err: errors.New("API overloaded")}

	var mu sync.Mutex
	var events []struct {
		topic   string
		payload EventPayload
	}

	publish := func(topic string, payload EventPayload) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, struct {
			topic   string
			payload EventPayload
		}{topic, payload})
	}

	obs := NewObservableLLM(inner, publish, "test")
	ctx := WithOperation(context.Background(), "edit")

	_, err := obs.Complete(ctx, "test prompt")
	if err == nil {
		t.Fatal("expected error from inner LLM")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 events even on error, got %d", len(events))
	}

	finished := events[1]
	if finished.topic != "llm.finished" {
		t.Errorf("event[1].topic = %q, want 'llm.finished'", finished.topic)
	}
	errField, ok := finished.payload["error"].(string)
	if !ok || errField == "" {
		t.Error("finished event on failure should have non-empty 'error' field")
	}
	if finished.payload["status"] != "failed" {
		t.Errorf("finished event status = %v, want 'failed'", finished.payload["status"])
	}
}

func TestObservableLLM_PassesThroughResult(t *testing.T) {
	inner := mockLLM{output: "hello world", err: nil}
	publish := func(topic string, payload EventPayload) {}

	obs := NewObservableLLM(inner, publish, "test")
	result, err := obs.Complete(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("result = %q, want 'hello world'", result)
	}

	innerErr := mockLLM{output: "", err: errors.New("fail")}
	obs2 := NewObservableLLM(innerErr, publish, "test")
	_, err = obs2.Complete(context.Background(), "prompt")
	if err == nil || err.Error() != "fail" {
		t.Errorf("expected error 'fail', got %v", err)
	}
}

func TestObservableLLM_DefaultOperationName(t *testing.T) {
	inner := mockLLM{output: "ok", err: nil}

	var mu sync.Mutex
	var events []struct {
		topic   string
		payload EventPayload
	}

	publish := func(topic string, payload EventPayload) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, struct {
			topic   string
			payload EventPayload
		}{topic, payload})
	}

	obs := NewObservableLLM(inner, publish, "test")
	_, err := obs.Complete(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) < 1 {
		t.Fatal("expected at least 1 event")
	}

	op, ok := events[0].payload["operation"].(string)
	if !ok || op == "" {
		t.Error("operation should have a default value when not set via context")
	}
	if op != "unknown" {
		t.Errorf("default operation = %q, want 'unknown'", op)
	}
}

func TestWithOperation_RoundTrip(t *testing.T) {
	ctx := WithOperation(context.Background(), "focus")
	got := OperationFrom(ctx)
	if got != "focus" {
		t.Errorf("OperationFrom = %q, want 'focus'", got)
	}
}

func TestOperationFrom_EmptyContext(t *testing.T) {
	got := OperationFrom(context.Background())
	if got != "" {
		t.Errorf("OperationFrom on empty context = %q, want empty string", got)
	}
}
