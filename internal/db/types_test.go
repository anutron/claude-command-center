package db

import (
	"testing"
)

func TestValidTransition(t *testing.T) {
	// All valid state-specific transitions from the spec.
	valid := []struct{ from, to string }{
		{StatusNew, StatusBacklog},
		{StatusBacklog, StatusEnqueued},
		{StatusBacklog, StatusRunning},
		{StatusEnqueued, StatusRunning},
		{StatusEnqueued, StatusBacklog},
		{StatusRunning, StatusBlocked},
		{StatusRunning, StatusReview},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusBacklog},
		{StatusBlocked, StatusRunning},
		{StatusBlocked, StatusBacklog},
		{StatusReview, StatusBacklog},
		{StatusReview, StatusEnqueued},
		{StatusReview, StatusRunning},
		{StatusFailed, StatusBacklog},
		{StatusFailed, StatusEnqueued},
		{StatusFailed, StatusRunning},
		{StatusCompleted, StatusBacklog},
		{StatusDismissed, StatusBacklog},
	}
	for _, tc := range valid {
		if !ValidTransition(tc.from, tc.to) {
			t.Errorf("expected %s -> %s to be valid", tc.from, tc.to)
		}
	}

	// Universal exits: any state -> completed or dismissed.
	allStatuses := []string{
		StatusNew, StatusBacklog, StatusEnqueued, StatusRunning,
		StatusBlocked, StatusReview, StatusFailed, StatusCompleted, StatusDismissed,
	}
	for _, s := range allStatuses {
		if !ValidTransition(s, StatusCompleted) {
			t.Errorf("expected %s -> completed to be valid (universal exit)", s)
		}
		if !ValidTransition(s, StatusDismissed) {
			t.Errorf("expected %s -> dismissed to be valid (universal exit)", s)
		}
	}
}

func TestValidTransitionRejectsInvalid(t *testing.T) {
	invalid := []struct{ from, to string }{
		{StatusNew, StatusRunning},       // must go through backlog
		{StatusNew, StatusEnqueued},      // must go through backlog
		{StatusNew, StatusBlocked},       // not a valid transition
		{StatusNew, StatusReview},        // not a valid transition
		{StatusNew, StatusFailed},        // not a valid transition
		{StatusCompleted, StatusRunning}, // must reopen to backlog first
		{StatusCompleted, StatusEnqueued},
		{StatusDismissed, StatusEnqueued}, // must reopen to backlog first
		{StatusDismissed, StatusRunning},
		{StatusBacklog, StatusBlocked},  // can't block without running
		{StatusBacklog, StatusReview},   // can't review without running
		{StatusBacklog, StatusFailed},   // can't fail without running
		{StatusEnqueued, StatusBlocked}, // can't block without running
		{StatusEnqueued, StatusReview},  // can't review without running
		{StatusBlocked, StatusReview},   // must go through running
		{StatusBlocked, StatusFailed},   // must go through running
	}
	for _, tc := range invalid {
		if ValidTransition(tc.from, tc.to) {
			t.Errorf("expected %s -> %s to be invalid", tc.from, tc.to)
		}
	}
}

func TestValidTransitionUnknownState(t *testing.T) {
	if ValidTransition("unknown", StatusBacklog) {
		t.Error("expected unknown -> backlog to be invalid")
	}
	// But universal exits still work even from unknown (by design they check to, not from)
	if !ValidTransition("unknown", StatusCompleted) {
		t.Error("expected unknown -> completed to be valid (universal exit)")
	}
}

func TestIsTerminalStatus(t *testing.T) {
	terminal := []string{StatusCompleted, StatusDismissed}
	for _, s := range terminal {
		if !IsTerminalStatus(s) {
			t.Errorf("expected %s to be terminal", s)
		}
	}

	nonTerminal := []string{StatusNew, StatusBacklog, StatusEnqueued, StatusRunning, StatusBlocked, StatusReview, StatusFailed}
	for _, s := range nonTerminal {
		if IsTerminalStatus(s) {
			t.Errorf("expected %s to be non-terminal", s)
		}
	}
}

func TestIsAgentStatus(t *testing.T) {
	agent := []string{StatusEnqueued, StatusRunning, StatusBlocked}
	for _, s := range agent {
		if !IsAgentStatus(s) {
			t.Errorf("expected %s to be agent status", s)
		}
	}

	nonAgent := []string{StatusNew, StatusBacklog, StatusReview, StatusFailed, StatusCompleted, StatusDismissed}
	for _, s := range nonAgent {
		if IsAgentStatus(s) {
			t.Errorf("expected %s to not be agent status", s)
		}
	}
}

func TestActiveTodos(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Status: StatusNew},
			{ID: "2", Status: StatusBacklog},
			{ID: "3", Status: StatusEnqueued},
			{ID: "4", Status: StatusRunning},
			{ID: "5", Status: StatusBlocked},
			{ID: "6", Status: StatusReview},
			{ID: "7", Status: StatusFailed},
			{ID: "8", Status: StatusCompleted},
			{ID: "9", Status: StatusDismissed},
		},
	}

	active := cc.ActiveTodos()
	if len(active) != 7 {
		t.Fatalf("expected 7 active todos, got %d", len(active))
	}
	// Verify no terminal statuses in active
	for _, todo := range active {
		if IsTerminalStatus(todo.Status) {
			t.Errorf("active todos should not include terminal status %q", todo.Status)
		}
	}
}

func TestCompletedTodos(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Status: StatusNew},
			{ID: "2", Status: StatusBacklog},
			{ID: "3", Status: StatusCompleted},
			{ID: "4", Status: StatusDismissed},
		},
	}

	completed := cc.CompletedTodos()
	if len(completed) != 2 {
		t.Fatalf("expected 2 completed todos, got %d", len(completed))
	}
	for _, todo := range completed {
		if !IsTerminalStatus(todo.Status) {
			t.Errorf("completed todos should only include terminal statuses, got %q", todo.Status)
		}
	}
}

func TestAddTodoSetsBacklogStatus(t *testing.T) {
	cc := &CommandCenter{}
	todo := cc.AddTodo("Test todo")
	if todo.Status != StatusBacklog {
		t.Errorf("expected new manual todo to have status %q, got %q", StatusBacklog, todo.Status)
	}
	if todo.Source != "manual" {
		t.Errorf("expected source 'manual', got %q", todo.Source)
	}
}

func TestAcceptTodoSetsBacklog(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Status: StatusNew},
		},
	}
	cc.AcceptTodo("1")
	if cc.Todos[0].Status != StatusBacklog {
		t.Errorf("expected status %q after accept, got %q", StatusBacklog, cc.Todos[0].Status)
	}
}
