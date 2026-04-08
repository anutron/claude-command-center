package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// --- Context helpers ---

func TestWithOperation_RoundTrip(t *testing.T) {
	ctx := WithOperation(context.Background(), "summarize")
	got := OperationFrom(ctx)
	if got != "summarize" {
		t.Errorf("OperationFrom = %q, want %q", got, "summarize")
	}
}

func TestOperationFrom_EmptyWhenNotSet(t *testing.T) {
	got := OperationFrom(context.Background())
	if got != "" {
		t.Errorf("OperationFrom on bare context = %q, want empty", got)
	}
}

// --- ObservableLLM ---

type stubLLM struct {
	result string
	err    error
}

func (s *stubLLM) Complete(_ context.Context, _ string) (string, error) {
	return s.result, s.err
}

type capturedEvent struct {
	topic   string
	payload EventPayload
}

func TestObservableLLM_PublishesStartedAndFinished(t *testing.T) {
	inner := &stubLLM{result: "hello"}
	var events []capturedEvent
	pub := func(topic string, payload EventPayload) {
		events = append(events, capturedEvent{topic, payload})
	}

	obs := NewObservableLLM(inner, pub, "test-source")
	ctx := WithOperation(context.Background(), "greet")
	result, err := obs.Complete(ctx, "say hi")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %q, want %q", result, "hello")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Check started event
	started := events[0]
	if started.topic != "llm.started" {
		t.Errorf("first event topic = %q, want %q", started.topic, "llm.started")
	}
	if started.payload["operation"] != "greet" {
		t.Errorf("started operation = %v, want %q", started.payload["operation"], "greet")
	}
	if started.payload["source"] != "test-source" {
		t.Errorf("started source = %v, want %q", started.payload["source"], "test-source")
	}

	// Check finished event
	finished := events[1]
	if finished.topic != "llm.finished" {
		t.Errorf("second event topic = %q, want %q", finished.topic, "llm.finished")
	}
	if finished.payload["status"] != "completed" {
		t.Errorf("finished status = %v, want %q", finished.payload["status"], "completed")
	}
	if finished.payload["id"] != started.payload["id"] {
		t.Errorf("id mismatch: started=%v finished=%v", started.payload["id"], finished.payload["id"])
	}
	if _, ok := finished.payload["duration_ms"]; !ok {
		t.Error("finished event missing duration_ms")
	}
}

func TestObservableLLM_ErrorStatus(t *testing.T) {
	inner := &stubLLM{err: errors.New("boom")}
	var events []capturedEvent
	pub := func(topic string, payload EventPayload) {
		events = append(events, capturedEvent{topic, payload})
	}

	obs := NewObservableLLM(inner, pub, "err-source")
	_, err := obs.Complete(context.Background(), "fail")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	finished := events[1]
	if finished.payload["status"] != "failed" {
		t.Errorf("status = %v, want %q", finished.payload["status"], "failed")
	}
	if finished.payload["error"] != "boom" {
		t.Errorf("error = %v, want %q", finished.payload["error"], "boom")
	}
}

func TestObservableLLM_DefaultsToUnknownOperation(t *testing.T) {
	inner := &stubLLM{result: "ok"}
	var events []capturedEvent
	pub := func(topic string, payload EventPayload) {
		events = append(events, capturedEvent{topic, payload})
	}

	obs := NewObservableLLM(inner, pub, "src")
	// No WithOperation on context
	_, _ = obs.Complete(context.Background(), "test")

	if len(events) < 1 {
		t.Fatal("no events captured")
	}
	if events[0].payload["operation"] != "unknown" {
		t.Errorf("operation = %v, want %q", events[0].payload["operation"], "unknown")
	}
}

func TestObservableLLM_UniqueIDs(t *testing.T) {
	inner := &stubLLM{result: "ok"}
	var ids []string
	pub := func(topic string, payload EventPayload) {
		if topic == "llm.started" {
			ids = append(ids, payload["id"].(string))
		}
	}

	obs := NewObservableLLM(inner, pub, "src")
	for i := 0; i < 10; i++ {
		_, _ = obs.Complete(context.Background(), "test")
	}

	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id: %s", id)
		}
		seen[id] = true
		// Basic UUID format check: 8-4-4-4-12
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("id %q does not look like a UUID", id)
		}
	}
}

func TestObservableLLM_ImplementsLLMInterface(t *testing.T) {
	var _ LLM = &ObservableLLM{}
}
