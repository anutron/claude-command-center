# Agent Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add agent observability via a `~` overlay in the TUI and a standalone `ccc console` streaming TUI.

**Architecture:** Two surfaces sharing a `ListAgentHistory` daemon RPC. The overlay is a state in `internal/tui/model.go` (like the budget widget). The console is a separate bubbletea program in `cmd/ccc/console.go` connecting to the daemon socket. Agent output streaming uses a new `StreamAgentOutput` RPC that pipes parsed JSONL events.

**Tech Stack:** Go, bubbletea, lipgloss, daemon Unix socket RPCs

---

## File Structure

### New files:
- `internal/tui/console_overlay.go` — overlay state, rendering, key handling
- `internal/tui/console_overlay_test.go` — overlay view tests
- `internal/ui/agent_format.go` — shared formatting helpers (statusIcon, statusColor, formatElapsed, formatDuration)
- `cmd/ccc/console.go` — `ccc console` subcommand and bubbletea program
- `internal/daemon/agent_history.go` — ListAgentHistory + StreamAgentOutput RPC handlers
- `internal/db/agent_history.go` — DB query: join agent_costs with todos/PRs for origin labels
- `internal/db/agent_history_test.go` — DB query tests

### Modified files:
- `internal/tui/model.go` — add overlay state fields, intercept `~` key, render overlay
- `internal/daemon/daemon.go` — register new RPCs in dispatch()
- `internal/daemon/client.go` — add client methods for new RPCs
- `internal/daemon/types.go` — add AgentHistoryEntry type (shared between client/server)
- `cmd/ccc/main.go` — add `console` subcommand case

---

### Task 1: Agent History DB Query

**Files:**
- Create: `internal/db/agent_history.go`
- Create: `internal/db/agent_history_test.go`

- [ ] **Step 1: Define AgentHistoryEntry type in `internal/db/agent_history.go`**

```go
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// AgentHistoryEntry is a joined view of an agent run with its origin context.
type AgentHistoryEntry struct {
	// From cc_agent_costs
	AgentID      string    `json:"agent_id"`
	Automation   string    `json:"automation"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	DurationSec  int       `json:"duration_sec"`
	CostUSD      float64   `json:"cost_usd"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	ExitCode     *int      `json:"exit_code,omitempty"`

	// Origin context (from joins)
	OriginType  string `json:"origin_type"`  // "todo", "pr", "manual"
	OriginLabel string `json:"origin_label"` // e.g. "TODO #113 — Fix auth bug"
	OriginRef   string `json:"origin_ref"`   // e.g. "todo:113" or "pr:47:review"

	// From cc_sessions (if linked)
	ProjectDir string `json:"project_dir,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Branch     string `json:"branch,omitempty"`
	SessionID  string `json:"session_id,omitempty"` // Claude session UUID
}
```

- [ ] **Step 2: Write the failing test**

```go
package db

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestDBLoadAgentHistory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now()

	// Insert a todo with a session_id linking to an agent
	_, err := db.Exec(`INSERT INTO cc_todos (id, display_id, title, status, source, session_id)
		VALUES ('todo-1', 113, 'Fix auth bug', 'running', 'github', 'agent-abc')`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert an agent cost row
	_, err = db.Exec(`INSERT INTO cc_agent_costs (agent_id, automation, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('agent-abc', 'todo', ?, 'completed', 0.42, 1000, 500)`,
		FormatTime(now.Add(-10*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}

	// Insert a PR agent cost row
	_, err = db.Exec(`INSERT INTO cc_pull_requests (number, repo, title, agent_session_id, agent_status, agent_category)
		VALUES (47, 'owner/repo', 'Add feature', 'agent-pr-47', 'completed', 'review')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO cc_agent_costs (agent_id, automation, started_at, status, cost_usd)
		VALUES ('agent-pr-47', 'pr-review', ?, 'completed', 0.18)`,
		FormatTime(now.Add(-5*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}

	// Insert an old agent (>24h ago) that should NOT appear
	_, err = db.Exec(`INSERT INTO cc_agent_costs (agent_id, automation, started_at, status, cost_usd)
		VALUES ('agent-old', 'todo', ?, 'completed', 1.00)`,
		FormatTime(now.Add(-25*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	entries, err := DBLoadAgentHistory(db, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Most recent first (pr agent started 5m ago)
	if entries[0].AgentID != "agent-pr-47" {
		t.Errorf("expected first entry agent-pr-47, got %s", entries[0].AgentID)
	}
	if entries[0].OriginType != "pr" {
		t.Errorf("expected origin_type pr, got %s", entries[0].OriginType)
	}
	if entries[0].OriginLabel == "" {
		t.Error("expected non-empty origin label for PR agent")
	}

	if entries[1].AgentID != "agent-abc" {
		t.Errorf("expected second entry agent-abc, got %s", entries[1].AgentID)
	}
	if entries[1].OriginType != "todo" {
		t.Errorf("expected origin_type todo, got %s", entries[1].OriginType)
	}
	if entries[1].OriginLabel != "TODO #113 — Fix auth bug" {
		t.Errorf("unexpected origin label: %s", entries[1].OriginLabel)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	InitSchema(db)
	return db
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestDBLoadAgentHistory -v`
Expected: FAIL — `DBLoadAgentHistory` not defined

- [ ] **Step 4: Implement `DBLoadAgentHistory`**

```go
// DBLoadAgentHistory returns agent runs from the last `window` duration,
// joined with todo/PR origin data. Ordered by started_at DESC.
func DBLoadAgentHistory(database *sql.DB, window time.Duration) ([]AgentHistoryEntry, error) {
	since := FormatTime(time.Now().Add(-window))
	rows, err := database.Query(`
		SELECT
			ac.agent_id,
			ac.automation,
			ac.status,
			ac.started_at,
			ac.finished_at,
			ac.duration_sec,
			ac.cost_usd,
			ac.input_tokens,
			ac.output_tokens,
			ac.exit_code,
			-- Todo origin
			t.display_id,
			t.title,
			-- PR origin
			pr.number,
			pr.title,
			pr.agent_category,
			-- Session context
			s.session_id,
			s.project,
			s.repo,
			s.branch
		FROM cc_agent_costs ac
		LEFT JOIN cc_todos t ON t.session_id = ac.agent_id
		LEFT JOIN cc_pull_requests pr ON pr.agent_session_id = ac.agent_id
		LEFT JOIN cc_sessions s ON s.session_id = ac.agent_id
		WHERE ac.started_at >= ?
		ORDER BY ac.started_at DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("load agent history: %w", err)
	}
	defer rows.Close()

	var entries []AgentHistoryEntry
	for rows.Next() {
		var e AgentHistoryEntry
		var finishedAt, sessionID, project, repo, branch sql.NullString
		var exitCode sql.NullInt64
		var todoDisplayID sql.NullInt64
		var todoTitle, prTitle, prCategory sql.NullString
		var prNumber sql.NullInt64
		var startedAtStr string

		err := rows.Scan(
			&e.AgentID, &e.Automation, &e.Status, &startedAtStr,
			&finishedAt, &e.DurationSec, &e.CostUSD,
			&e.InputTokens, &e.OutputTokens, &exitCode,
			&todoDisplayID, &todoTitle,
			&prNumber, &prTitle, &prCategory,
			&sessionID, &project, &repo, &branch,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agent history row: %w", err)
		}

		e.StartedAt = ParseTime(startedAtStr)
		if finishedAt.Valid {
			t := ParseTime(finishedAt.String)
			e.FinishedAt = &t
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			e.ExitCode = &ec
		}
		if sessionID.Valid {
			e.SessionID = sessionID.String
		}
		if project.Valid {
			e.ProjectDir = project.String
		}
		if repo.Valid {
			e.Repo = repo.String
		}
		if branch.Valid {
			e.Branch = branch.String
		}

		// Determine origin
		switch {
		case todoDisplayID.Valid && todoTitle.Valid:
			e.OriginType = "todo"
			e.OriginLabel = fmt.Sprintf("TODO #%d — %s", todoDisplayID.Int64, todoTitle.String)
			e.OriginRef = fmt.Sprintf("todo:%d", todoDisplayID.Int64)
		case prNumber.Valid && prTitle.Valid:
			e.OriginType = "pr"
			cat := "review"
			if prCategory.Valid {
				cat = prCategory.String
			}
			e.OriginLabel = fmt.Sprintf("PR #%d — %s", prNumber.Int64, prTitle.String)
			e.OriginRef = fmt.Sprintf("pr:%d:%s", prNumber.Int64, cat)
		default:
			e.OriginType = "manual"
			e.OriginLabel = e.AgentID
			e.OriginRef = "manual"
		}

		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestDBLoadAgentHistory -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/db/agent_history.go internal/db/agent_history_test.go
git commit -m "Add DBLoadAgentHistory query joining agent costs with origin data"
```

---

### Task 2: Daemon RPCs — ListAgentHistory + StreamAgentOutput

**Files:**
- Create: `internal/daemon/agent_history.go`
- Modify: `internal/daemon/daemon.go` (dispatch table)
- Modify: `internal/daemon/client.go` (client methods)
- Modify: `internal/daemon/types.go` (shared types if needed)

- [ ] **Step 1: Write `ListAgentHistory` handler in `internal/daemon/agent_history.go`**

```go
package daemon

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/db"
)

// ListAgentHistoryParams are the parameters for the ListAgentHistory RPC.
type ListAgentHistoryParams struct {
	WindowHours int `json:"window_hours"` // default 24
}

// ListAgentHistoryResult wraps the response.
type ListAgentHistoryResult struct {
	Entries []db.AgentHistoryEntry `json:"entries"`
}

func (s *Server) handleListAgentHistory(req *RPCRequest) (interface{}, *RPCError) {
	var params ListAgentHistoryParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
	}
	if params.WindowHours <= 0 {
		params.WindowHours = 24
	}

	window := time.Duration(params.WindowHours) * time.Hour
	entries, err := db.DBLoadAgentHistory(s.db, window)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("load agent history: %v", err)}
	}

	// Enrich with live status from the runner for active agents.
	if s.runner != nil {
		for i := range entries {
			if entries[i].Status == "running" {
				if status := s.runner.Status(entries[i].AgentID); status != nil {
					entries[i].Status = status.Status
					if status.SessionID != "" {
						entries[i].SessionID = status.SessionID
					}
				}
			}
		}
	}

	return ListAgentHistoryResult{Entries: entries}, nil
}

// StreamAgentOutputParams are the parameters for the StreamAgentOutput RPC.
type StreamAgentOutputParams struct {
	AgentID string `json:"agent_id"`
}

// StreamAgentOutputResult returns the current event buffer for an agent.
type StreamAgentOutputResult struct {
	Events []agent.SessionEvent `json:"events"`
	Done   bool                 `json:"done"` // true if agent has finished
}

func (s *Server) handleStreamAgentOutput(req *RPCRequest) (interface{}, *RPCError) {
	var params StreamAgentOutputParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	if params.AgentID == "" {
		return nil, &RPCError{Code: -32602, Message: "agent_id required"}
	}

	if s.runner == nil {
		return nil, &RPCError{Code: -32000, Message: "agent runner not configured"}
	}

	session := s.runner.Session(params.AgentID)
	if session == nil {
		return StreamAgentOutputResult{Done: true}, nil
	}

	session.Mu.Lock()
	events := make([]agent.SessionEvent, len(session.Events))
	copy(events, session.Events)
	session.Mu.Unlock()

	// Check if done
	done := false
	select {
	case <-session.Done():
		done = true
	default:
	}

	return StreamAgentOutputResult{Events: events, Done: done}, nil
}
```

- [ ] **Step 2: Expose `Done()` on Session if not already public**

Check `internal/agent/runner.go` — the `done` channel is lowercase. Add a public accessor:

```go
// Done returns a channel that closes when the session's process exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}
```

- [ ] **Step 3: Register RPCs in dispatch table**

Edit `internal/daemon/daemon.go`, add cases in the `dispatch()` switch:

```go
case "ListAgentHistory":
	return s.handleListAgentHistory(req)
case "StreamAgentOutput":
	return s.handleStreamAgentOutput(req)
```

- [ ] **Step 4: Add client methods in `internal/daemon/client.go`**

```go
// ListAgentHistory returns agent history for the given time window.
func (c *Client) ListAgentHistory(windowHours int) ([]db.AgentHistoryEntry, error) {
	result, err := c.call("ListAgentHistory", ListAgentHistoryParams{WindowHours: windowHours})
	if err != nil {
		return nil, err
	}
	var resp ListAgentHistoryResult
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal agent history: %w", err)
	}
	return resp.Entries, nil
}

// StreamAgentOutput returns the current event buffer for a running agent.
func (c *Client) StreamAgentOutput(agentID string) (StreamAgentOutputResult, error) {
	result, err := c.call("StreamAgentOutput", StreamAgentOutputParams{AgentID: agentID})
	if err != nil {
		return StreamAgentOutputResult{}, err
	}
	var resp StreamAgentOutputResult
	if err := json.Unmarshal(result, &resp); err != nil {
		return StreamAgentOutputResult{}, fmt.Errorf("unmarshal agent output: %w", err)
	}
	return resp, nil
}
```

- [ ] **Step 5: Add `db` import to `client.go` if not present**

The `ListAgentHistory` client method returns `[]db.AgentHistoryEntry`, so add `"github.com/anutron/claude-command-center/internal/db"` to imports.

- [ ] **Step 6: Run tests**

Run: `make test`
Expected: PASS (all existing tests still pass)

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/agent_history.go internal/daemon/daemon.go internal/daemon/client.go internal/agent/runner.go
git commit -m "Add ListAgentHistory and StreamAgentOutput daemon RPCs"
```

---

### Task 3: Console Overlay — State and Key Toggle

**Files:**
- Create: `internal/tui/console_overlay.go`
- Modify: `internal/tui/model.go`

- [ ] **Step 1: Create `internal/tui/console_overlay.go` with overlay state**

```go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// consoleOverlay manages the agent console overlay state.
type consoleOverlay struct {
	visible  bool
	entries  []db.AgentHistoryEntry
	cursor   int
	detail   bool // true = showing detail view for selected entry
	scroll   int  // scroll offset in detail view
}

func (o *consoleOverlay) toggle(entries []db.AgentHistoryEntry) {
	o.visible = !o.visible
	if o.visible {
		o.entries = entries
		o.cursor = 0
		o.detail = false
		o.scroll = 0
	}
}

func (o *consoleOverlay) close() {
	o.visible = false
	o.detail = false
	o.scroll = 0
}

func (o *consoleOverlay) selected() *db.AgentHistoryEntry {
	if o.cursor >= 0 && o.cursor < len(o.entries) {
		return &o.entries[o.cursor]
	}
	return nil
}
```

- [ ] **Step 2: Add overlay field to Model in `internal/tui/model.go`**

Add to the `Model` struct:

```go
// Console overlay (agent observability).
console consoleOverlay
```

- [ ] **Step 3: Intercept `~` key in Model.Update**

In `internal/tui/model.go`, inside the `case tea.KeyMsg:` block, before the final `action := m.activePlugin().HandleKey(msg)` line (around line 473), add:

```go
// Console overlay: ~ toggles, keys handled when visible.
if m.console.visible {
	switch msg.String() {
	case "~", "esc":
		if m.console.detail {
			m.console.detail = false
			m.console.scroll = 0
		} else {
			m.console.close()
		}
		return m, nil
	case "j", "down":
		if m.console.detail {
			m.console.scroll++
		} else if m.console.cursor < len(m.console.entries)-1 {
			m.console.cursor++
		}
		return m, nil
	case "k", "up":
		if m.console.detail {
			if m.console.scroll > 0 {
				m.console.scroll--
			}
		} else if m.console.cursor > 0 {
			m.console.cursor--
		}
		return m, nil
	case "enter":
		if !m.console.detail && m.console.selected() != nil {
			m.console.detail = true
			m.console.scroll = 0
		}
		return m, nil
	default:
		return m, nil // consume all keys while overlay is open
	}
}

if msg.String() == "~" {
	var entries []db.AgentHistoryEntry
	if client := m.DaemonClient(); client != nil {
		entries, _ = client.ListAgentHistory(24)
	}
	m.console.toggle(entries)
	return m, nil
}
```

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/console_overlay.go internal/tui/model.go
git commit -m "Add console overlay state and ~ key toggle"
```

---

### Task 4: Console Overlay — List View Rendering

**Files:**
- Modify: `internal/tui/console_overlay.go`
- Modify: `internal/tui/model.go` (View method)
- Create: `internal/tui/console_overlay_test.go`

- [ ] **Step 1: Write the failing view test**

```go
package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestConsoleOverlay_ListView(t *testing.T) {
	now := time.Now()
	entries := []db.AgentHistoryEntry{
		{
			AgentID:     "agent-1",
			Status:      "running",
			StartedAt:   now.Add(-3 * time.Minute),
			CostUSD:     0.42,
			OriginType:  "todo",
			OriginLabel: "TODO #113 — Fix auth bug",
		},
		{
			AgentID:     "agent-2",
			Status:      "completed",
			StartedAt:   now.Add(-10 * time.Minute),
			CostUSD:     0.18,
			OriginType:  "pr",
			OriginLabel: "PR #47 — Review",
			DurationSec: 68,
		},
	}

	o := &consoleOverlay{visible: true, entries: entries, cursor: 0}
	view := o.renderList(80, 24)

	if !strings.Contains(view, "AGENT CONSOLE") {
		t.Error("expected title AGENT CONSOLE")
	}
	if !strings.Contains(view, "TODO #113") {
		t.Error("expected todo origin label")
	}
	if !strings.Contains(view, "PR #47") {
		t.Error("expected PR origin label")
	}
	if !strings.Contains(view, "$0.42") {
		t.Error("expected cost")
	}
	// Verify running status icon
	if !strings.Contains(view, "●") {
		t.Error("expected running status icon ●")
	}
	// Verify completed status icon
	if !strings.Contains(view, "✓") {
		t.Error("expected completed status icon ✓")
	}
}

func TestConsoleOverlay_EmptyState(t *testing.T) {
	o := &consoleOverlay{visible: true, entries: nil, cursor: 0}
	view := o.renderList(80, 24)

	if !strings.Contains(view, "No agents") {
		t.Error("expected empty state message")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestConsoleOverlay -v`
Expected: FAIL — `renderList` not defined

- [ ] **Step 3a: Create shared formatting helpers in `internal/ui/agent_format.go`**

These helpers are used by both the TUI overlay and the standalone console.

```go
package ui

import (
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// AgentStatusIcon returns a unicode icon for the agent status.
func AgentStatusIcon(status string) string {
	switch status {
	case "running", "processing":
		return "●"
	case "queued":
		return "◌"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "stopped":
		return "⊘"
	case "blocked":
		return "⏸"
	default:
		return "?"
	}
}

// AgentStatusColor returns a lipgloss color for the agent status.
func AgentStatusColor(status string) lipgloss.Color {
	switch status {
	case "running", "processing":
		return lipgloss.Color("#4ade80") // green
	case "queued":
		return lipgloss.Color("#f59e0b") // yellow
	case "completed":
		return lipgloss.Color("#565f89") // dim
	case "failed":
		return lipgloss.Color("#f7768e") // red
	case "stopped":
		return lipgloss.Color("#f7768e") // red
	case "blocked":
		return lipgloss.Color("#f59e0b") // yellow
	default:
		return lipgloss.Color("#565f89")
	}
}

// FormatAgentElapsed formats the elapsed/duration for an agent entry.
func FormatAgentElapsed(e db.AgentHistoryEntry) string {
	if e.Status == "queued" {
		return "queued"
	}
	if e.DurationSec > 0 {
		return FormatDuration(time.Duration(e.DurationSec) * time.Second)
	}
	if !e.StartedAt.IsZero() {
		return FormatDuration(time.Since(e.StartedAt))
	}
	return "—"
}

// FormatDuration formats a duration as "Xm YYs" or "Xs".
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}
```

- [ ] **Step 3b: Implement `renderList` and `renderDetail` in `console_overlay.go`**

Use `ui.AgentStatusIcon`, `ui.AgentStatusColor`, and `ui.FormatAgentElapsed` throughout instead of local functions.

```go
func (o *consoleOverlay) renderList(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5")).Render("AGENT CONSOLE")
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Render("Last 24 hours · ↑↓ select · Enter detail · ~ dismiss")

	var rows []string
	rows = append(rows, title, subtitle, "")

	if len(o.entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Render("  No agents in the last 24 hours"))
	}

	for i, e := range o.entries {
		icon := ui.AgentStatusIcon(e.Status)
		iconStyle := lipgloss.NewStyle().Foreground(ui.AgentStatusColor(e.Status))
		label := e.OriginLabel
		if len(label) > 35 {
			label = label[:32] + "..."
		}
		elapsed := ui.FormatAgentElapsed(e)
		cost := fmt.Sprintf("$%.2f", e.CostUSD)

		row := fmt.Sprintf("  %s  %-35s  %-8s  %s",
			iconStyle.Render(icon), label, elapsed, cost)

		if i == o.cursor {
			row = lipgloss.NewStyle().Background(lipgloss.Color("#1a1b26")).Render(row)
		}
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	boxWidth := 70
	if width < boxWidth+4 {
		boxWidth = width - 4
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3b4261")).
		Width(boxWidth).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (o *consoleOverlay) renderDetail(width, height int) string {
	e := o.selected()
	if e == nil {
		return o.renderList(width, height)
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5")).Render("AGENT DETAIL")
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Render("Esc back · ↑↓ scroll")

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a9b1d6"))

	field := func(label, value string) string {
		return fmt.Sprintf("  %s  %s", labelStyle.Width(16).Render(label), valStyle.Render(value))
	}

	var rows []string
	rows = append(rows, title, subtitle, "")

	rows = append(rows, field("Status:", fmt.Sprintf("%s %s", ui.AgentStatusIcon(e.Status), e.Status)))
	rows = append(rows, field("Origin:", e.OriginLabel))
	rows = append(rows, field("Origin Ref:", e.OriginRef))
	rows = append(rows, field("Agent ID:", e.AgentID))
	if e.SessionID != "" {
		rows = append(rows, field("Session ID:", e.SessionID))
	}
	rows = append(rows, field("Automation:", e.Automation))
	rows = append(rows, field("Started:", e.StartedAt.Format("15:04:05")))
	if e.FinishedAt != nil {
		rows = append(rows, field("Finished:", e.FinishedAt.Format("15:04:05")))
	}
	rows = append(rows, field("Duration:", ui.FormatAgentElapsed(*e)))
	rows = append(rows, field("Cost:", fmt.Sprintf("$%.4f", e.CostUSD)))
	rows = append(rows, field("Tokens In:", fmt.Sprintf("%d", e.InputTokens)))
	rows = append(rows, field("Tokens Out:", fmt.Sprintf("%d", e.OutputTokens)))
	if e.ExitCode != nil {
		rows = append(rows, field("Exit Code:", fmt.Sprintf("%d", *e.ExitCode)))
	}
	if e.ProjectDir != "" {
		rows = append(rows, field("Project:", e.ProjectDir))
	}
	if e.Repo != "" {
		rows = append(rows, field("Repo:", e.Repo))
	}
	if e.Branch != "" {
		rows = append(rows, field("Branch:", e.Branch))
	}

	// Apply scroll offset
	contentRows := rows
	if o.scroll > 0 && o.scroll < len(contentRows) {
		contentRows = contentRows[o.scroll:]
	}

	content := lipgloss.JoinVertical(lipgloss.Left, contentRows...)
	boxWidth := 70
	if width < boxWidth+4 {
		boxWidth = width - 4
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3b4261")).
		Width(boxWidth).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (o *consoleOverlay) render(width, height int) string {
	if o.detail {
		return o.renderDetail(width, height)
	}
	return o.renderList(width, height)
}
```

- [ ] **Step 4: Hook overlay rendering into `Model.View()`**

In `internal/tui/model.go`, in the `View()` method, after the line that builds the final `page` (around line 715, before the budget widget overlay), add:

```go
// Console overlay — render on top of everything.
if m.console.visible {
	page = m.console.render(m.width, m.height)
}
```

Place this BEFORE the budget widget overlay so the budget still shows on top.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -run TestConsoleOverlay -v`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `make test`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/tui/console_overlay.go internal/tui/console_overlay_test.go internal/tui/model.go
git commit -m "Add console overlay list and detail view rendering"
```

---

### Task 5: Console Overlay — Live Event Bus Updates

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/console_overlay.go`

- [ ] **Step 1: Refresh overlay entries on agent state change**

In `internal/tui/model.go`, in the `case plugin.AgentStateChangedMsg:` handler (around line 514), add a refresh of the overlay if it's visible:

```go
// Refresh console overlay if visible.
if m.console.visible {
	if client := m.DaemonClient(); client != nil {
		if entries, err := client.ListAgentHistory(24); err == nil {
			m.console.entries = entries
			// Clamp cursor
			if m.console.cursor >= len(entries) {
				m.console.cursor = max(0, len(entries)-1)
			}
		}
	}
}
```

- [ ] **Step 2: Also refresh on daemon events (agent.started, agent.finished, agent.stopped)**

In the `case DaemonEventMsg:` handler (around line 476), add similar refresh logic:

```go
// Refresh console overlay on agent lifecycle events.
if m.console.visible {
	switch msg.Event.Type {
	case "agent.started", "agent.finished", "agent.stopped":
		if client := m.DaemonClient(); client != nil {
			if entries, err := client.ListAgentHistory(24); err == nil {
				m.console.entries = entries
				if m.console.cursor >= len(entries) {
					m.console.cursor = max(0, len(entries)-1)
				}
			}
		}
	}
}
```

- [ ] **Step 3: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/tui/model.go
git commit -m "Refresh console overlay on agent lifecycle events"
```

---

### Task 6: `ccc console` — Subcommand Scaffold + Daemon Connection

**Files:**
- Create: `cmd/ccc/console.go`
- Modify: `cmd/ccc/main.go`

- [ ] **Step 1: Add `console` case to `main.go`**

In `cmd/ccc/main.go`, add a new case in the switch block (around line 31):

```go
case "console":
	if err := runConsole(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return
```

- [ ] **Step 2: Add to printUsage()**

```go
fmt.Fprintln(os.Stderr, "  console              Live agent streaming dashboard")
```

- [ ] **Step 3: Create `cmd/ccc/console.go` with basic bubbletea program**

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pollInterval controls how often the console refreshes agent data.
const consolePollInterval = 1 * time.Second

type consoleModel struct {
	client  *daemon.Client
	entries []db.AgentHistoryEntry
	cursor  int
	width   int
	height  int

	// Focus pane: events for selected agent
	events []agent.SessionEvent
	done   bool // selected agent is finished
}

type consoleTickMsg struct{}
type consoleDataMsg struct {
	entries []db.AgentHistoryEntry
	events  []agent.SessionEvent
	done    bool
}

func runConsole(args []string) error {
	socketPath := config.DaemonSocketPath()
	client, err := daemon.NewClient(socketPath)
	if err != nil {
		return fmt.Errorf("could not connect to daemon at %s: %w\nIs the daemon running? Try: ccc daemon start", socketPath, err)
	}

	m := consoleModel{client: client}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m consoleModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchData(),
		tea.Tick(consolePollInterval, func(time.Time) tea.Msg { return consoleTickMsg{} }),
	)
}

func (m consoleModel) fetchData() tea.Cmd {
	return func() tea.Msg {
		entries, _ := m.client.ListAgentHistory(24)

		var events []agent.SessionEvent
		done := false
		if len(entries) > 0 && m.cursor < len(entries) {
			agentID := entries[m.cursor].AgentID
			result, err := m.client.StreamAgentOutput(agentID)
			if err == nil {
				events = result.Events
				done = result.Done
			}
		}

		return consoleDataMsg{entries: entries, events: events, done: done}
	}
}

func (m consoleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case consoleTickMsg:
		return m, tea.Batch(
			m.fetchData(),
			tea.Tick(consolePollInterval, func(time.Time) tea.Msg { return consoleTickMsg{} }),
		)

	case consoleDataMsg:
		m.entries = msg.entries
		m.events = msg.events
		m.done = msg.done
		if m.cursor >= len(m.entries) {
			m.cursor = max(0, len(m.entries)-1)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				return m, m.fetchData() // immediate refresh for new selection
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				return m, m.fetchData()
			}
		}
	}
	return m, nil
}

func (m consoleModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	sidebar := m.renderSidebar()
	focus := m.renderFocusPane()

	sidebarWidth := 28
	if m.width < 60 {
		sidebarWidth = 20
	}

	sidebarStyle := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(m.height).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#3b4261"))

	focusStyle := lipgloss.NewStyle().
		Width(m.width - sidebarWidth - 2). // -2 for border
		Height(m.height)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		sidebarStyle.Render(sidebar),
		focusStyle.Render(focus),
	)
}

func (m consoleModel) renderSidebar() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5")).Render(" AGENTS")
	var rows []string
	rows = append(rows, title, "")

	if len(m.entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")).
			Render(" No agents running"))
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")).
			Render(" Watching..."))
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	// Split active and completed
	var active, completed []int
	for i, e := range m.entries {
		if e.Status == "running" || e.Status == "processing" || e.Status == "queued" || e.Status == "blocked" {
			active = append(active, i)
		} else {
			completed = append(completed, i)
		}
	}

	renderItem := func(idx int) string {
		e := m.entries[idx]
		icon := ui.AgentStatusIcon(e.Status)
		iconSt := lipgloss.NewStyle().Foreground(ui.AgentStatusColor(e.Status))

		label := e.OriginLabel
		maxLabel := 20
		if len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		row := fmt.Sprintf(" %s %s", iconSt.Render(icon), label)
		if idx == m.cursor {
			row = lipgloss.NewStyle().
				Background(lipgloss.Color("#1a1b26")).
				Bold(true).
				Render(row)
		}
		return row
	}

	for _, idx := range active {
		rows = append(rows, renderItem(idx))
	}

	if len(completed) > 0 {
		sep := lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4261")).Render(" ── completed ──")
		rows = append(rows, "", sep)
		for _, idx := range completed {
			rows = append(rows, renderItem(idx))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m consoleModel) renderFocusPane() string {
	if len(m.entries) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")).
			Padding(1, 2).
			Render("No agents running. Watching for activity...")
	}

	e := m.entries[m.cursor]
	header := lipgloss.NewStyle().Bold(true).Foreground(ui.AgentStatusColor(e.Status)).
		Render(fmt.Sprintf(" %s %s", ui.AgentStatusIcon(e.Status), e.OriginLabel))
	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).
		Render(fmt.Sprintf(" %s · $%.2f", ui.FormatAgentElapsed(e), e.CostUSD))
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4261")).
		Render(" " + strings.Repeat("─", m.width-32))

	var rows []string
	rows = append(rows, header, meta, sep, "")

	if len(m.events) == 0 {
		if m.done {
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).
				Render("  (no event data available)"))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).
				Render("  Waiting for events..."))
		}
	}

	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a9b1d6"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))

	for _, ev := range m.events {
		switch ev.Type {
		case "tool_use":
			input := ev.ToolInput
			if len(input) > 60 {
				input = input[:57] + "..."
			}
			rows = append(rows, fmt.Sprintf("  %s %s",
				toolStyle.Render("⠋ "+ev.ToolName),
				dimStyle.Render(input)))
		case "tool_result":
			text := ev.ResultText
			if len(text) > 80 {
				text = text[:77] + "..."
			}
			if ev.IsError {
				rows = append(rows, errStyle.Render("  ✗ "+text))
			} else {
				rows = append(rows, textStyle.Render("  → "+text))
			}
		case "assistant_text":
			text := ev.Text
			if len(text) > 80 {
				text = text[:77] + "..."
			}
			rows = append(rows, textStyle.Render("  "+text))
		case "error":
			rows = append(rows, errStyle.Render("  ERROR: "+ev.Text))
		}
	}

	// Only show the last N rows that fit in the viewport
	maxRows := m.height - 4
	if maxRows < 5 {
		maxRows = 5
	}
	if len(rows) > maxRows {
		rows = rows[len(rows)-maxRows:]
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
```

- [ ] **Step 4: Run build**

Run: `make build`
Expected: SUCCESS

- [ ] **Step 5: Test manually**

Run: `./ccc console`
Expected: Connects to daemon, shows "No agents running" or lists active agents. `q` to quit.

- [ ] **Step 6: Commit**

```bash
git add cmd/ccc/console.go cmd/ccc/main.go
git commit -m "Add ccc console standalone streaming TUI"
```

---

### Task 7: Integration Testing + Polish

**Files:**
- Modify: `internal/tui/console_overlay_test.go`

- [ ] **Step 1: Add detail view test**

```go
func TestConsoleOverlay_DetailView(t *testing.T) {
	now := time.Now()
	finished := now.Add(-2 * time.Minute)
	exitCode := 0
	entries := []db.AgentHistoryEntry{
		{
			AgentID:      "agent-abc",
			SessionID:    "sess-123",
			Status:       "completed",
			Automation:   "todo",
			StartedAt:    now.Add(-10 * time.Minute),
			FinishedAt:   &finished,
			DurationSec:  480,
			CostUSD:      1.23,
			InputTokens:  5000,
			OutputTokens: 2000,
			ExitCode:     &exitCode,
			OriginType:   "todo",
			OriginLabel:  "TODO #91 — Refactor db layer",
			OriginRef:    "todo:91",
			ProjectDir:   "/Users/aaron/project",
			Repo:         "owner/repo",
			Branch:       "feature-branch",
		},
	}

	o := &consoleOverlay{visible: true, entries: entries, cursor: 0, detail: true}
	view := o.renderDetail(80, 30)

	for _, want := range []string{
		"AGENT DETAIL",
		"agent-abc",
		"sess-123",
		"completed",
		"TODO #91",
		"todo:91",
		"$1.2300",
		"5000",
		"2000",
		"/Users/aaron/project",
		"owner/repo",
		"feature-branch",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run all tests**

Run: `make test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/tui/console_overlay_test.go
git commit -m "Add console overlay detail view test"
```

---

### Task 8: Spec Update

**Files:**
- Modify: `specs/builtin/console.md`

- [ ] **Step 1: Update spec with implementation details**

Add to the spec any implementation-specific details that diverged from the original (e.g., exact key bindings, exact file locations). Update test cases section to reflect actual tests written.

- [ ] **Step 2: Commit**

```bash
git add specs/builtin/console.md
git commit -m "Update console spec with implementation details"
```
