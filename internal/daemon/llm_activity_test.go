package daemon

import (
	"fmt"
	"testing"
	"time"
)

func TestLLMActivityBuffer_Insert(t *testing.T) {
	buf := newLLMActivityBuffer(100)

	evt := LLMActivityEvent{
		ID:        "evt-1",
		Operation: "command",
		Source:    "command-center",
		StartedAt: time.Now(),
		Status:    "running",
	}

	buf.Report(evt)

	events := buf.List()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "evt-1" {
		t.Errorf("event ID = %q, want 'evt-1'", events[0].ID)
	}
	if events[0].Operation != "command" {
		t.Errorf("event operation = %q, want 'command'", events[0].Operation)
	}
	if events[0].Status != "running" {
		t.Errorf("event status = %q, want 'running'", events[0].Status)
	}
}

func TestLLMActivityBuffer_UpdateOnFinish(t *testing.T) {
	buf := newLLMActivityBuffer(100)

	started := time.Now()
	buf.Report(LLMActivityEvent{
		ID:        "evt-2",
		Operation: "edit",
		Source:    "command-center",
		StartedAt: started,
		Status:    "running",
	})

	finished := started.Add(150 * time.Millisecond)
	buf.Report(LLMActivityEvent{
		ID:         "evt-2",
		Operation:  "edit",
		Source:     "command-center",
		StartedAt:  started,
		FinishedAt: &finished,
		DurationMs: 150,
		Status:     "completed",
	})

	events := buf.List()
	if len(events) != 1 {
		t.Fatalf("expected 1 event (updated, not duplicated), got %d", len(events))
	}

	evt := events[0]
	if evt.Status != "completed" {
		t.Errorf("status = %q, want 'completed'", evt.Status)
	}
	if evt.FinishedAt == nil {
		t.Fatal("FinishedAt should be set after finish")
	}
	if evt.DurationMs != 150 {
		t.Errorf("DurationMs = %d, want 150", evt.DurationMs)
	}
}

func TestLLMActivityBuffer_MaxCapacity(t *testing.T) {
	buf := newLLMActivityBuffer(100)

	for i := 0; i < 101; i++ {
		buf.Report(LLMActivityEvent{
			ID:        fmt.Sprintf("evt-%d", i),
			Operation: "command",
			Source:    "test",
			StartedAt: time.Now(),
			Status:    "running",
		})
	}

	events := buf.List()
	if len(events) != 100 {
		t.Errorf("expected max 100 events, got %d", len(events))
	}

	for _, evt := range events {
		if evt.ID == "evt-0" {
			t.Error("evt-0 should have been evicted (oldest)")
		}
	}
}

func TestLLMActivityBuffer_UpdateNotFound(t *testing.T) {
	buf := newLLMActivityBuffer(100)

	finished := time.Now()
	buf.Report(LLMActivityEvent{
		ID:         "nonexistent",
		Operation:  "edit",
		Source:     "test",
		StartedAt:  time.Now(),
		FinishedAt: &finished,
		DurationMs: 50,
		Status:     "completed",
	})

	events := buf.List()
	if len(events) != 1 {
		t.Errorf("expected 1 event (inserted as new), got %d", len(events))
	}
}

func TestLLMActivityBuffer_ListReturnsNewestFirst(t *testing.T) {
	buf := newLLMActivityBuffer(100)

	buf.Report(LLMActivityEvent{
		ID:        "first",
		Operation: "command",
		Source:    "test",
		StartedAt: time.Now(),
		Status:    "running",
	})

	buf.Report(LLMActivityEvent{
		ID:        "second",
		Operation: "edit",
		Source:    "test",
		StartedAt: time.Now().Add(time.Second),
		Status:    "running",
	})

	events := buf.List()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "second" {
		t.Errorf("events[0].ID = %q, want 'second' (newest first)", events[0].ID)
	}
	if events[1].ID != "first" {
		t.Errorf("events[1].ID = %q, want 'first'", events[1].ID)
	}
}
