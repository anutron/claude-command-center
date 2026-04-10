package daemon

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// LLMActivityEvent represents a single LLM call lifecycle event.
type LLMActivityEvent struct {
	ID         string     `json:"id"`
	Operation  string     `json:"operation"`
	Source     string     `json:"source"`
	TodoID     string     `json:"todo_id,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMs int        `json:"duration_ms,omitempty"`
	Error      string     `json:"error,omitempty"`
	Status     string     `json:"status"` // "running", "completed", "failed"
}

// llmActivityBuffer is a fixed-size ring buffer of LLM activity events.
type llmActivityBuffer struct {
	mu      sync.Mutex
	entries []LLMActivityEvent
	max     int
}

// newLLMActivityBuffer creates a ring buffer that holds at most max events.
func newLLMActivityBuffer(max int) *llmActivityBuffer {
	return &llmActivityBuffer{
		entries: make([]LLMActivityEvent, 0, max),
		max:     max,
	}
}

// Report inserts a new event or updates an existing one (matched by ID).
// When inserting and the buffer is full, the oldest entry is evicted.
func (b *llmActivityBuffer) Report(evt LLMActivityEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Try to find and update an existing event with the same ID.
	for i := range b.entries {
		if b.entries[i].ID == evt.ID {
			// Merge: preserve StartedAt from original, update finish fields.
			if evt.StartedAt.IsZero() {
				evt.StartedAt = b.entries[i].StartedAt
			}
			if evt.Source == "" {
				evt.Source = b.entries[i].Source
			}
			if evt.TodoID == "" {
				evt.TodoID = b.entries[i].TodoID
			}
			b.entries[i] = evt
			return
		}
	}

	// Insert new event — evict oldest if at capacity.
	if len(b.entries) >= b.max {
		b.entries = b.entries[1:]
	}
	b.entries = append(b.entries, evt)
}

// List returns a copy of all events sorted newest-first (by StartedAt desc).
func (b *llmActivityBuffer) List() []LLMActivityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]LLMActivityEvent, len(b.entries))
	copy(result, b.entries)

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})

	return result
}

// ReportLLMActivity records an LLM activity event and broadcasts it to subscribers.
// This is the direct (non-RPC) entry point for in-process callers like the daemon refresh.
func (s *Server) ReportLLMActivity(evt LLMActivityEvent) {
	s.llmActivity.Report(evt)

	if evt.Status == "running" {
		data, _ := json.Marshal(map[string]interface{}{
			"id":        evt.ID,
			"operation": evt.Operation,
		})
		s.Broadcast(Event{Type: "llm.started", Data: data})
	} else {
		data, _ := json.Marshal(map[string]interface{}{
			"id":          evt.ID,
			"operation":   evt.Operation,
			"duration_ms": evt.DurationMs,
		})
		s.Broadcast(Event{Type: "llm.finished", Data: data})
	}
}

// handleReportLLMActivity handles the ReportLLMActivity RPC.
func (s *Server) handleReportLLMActivity(req *RPCRequest) (interface{}, *RPCError) {
	var evt LLMActivityEvent
	if err := json.Unmarshal(req.Params, &evt); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	s.ReportLLMActivity(evt)
	return map[string]bool{"ok": true}, nil
}

// handleListLLMActivity handles the ListLLMActivity RPC.
func (s *Server) handleListLLMActivity(req *RPCRequest) (interface{}, *RPCError) {
	return s.llmActivity.List(), nil
}
