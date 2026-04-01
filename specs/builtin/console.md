# SPEC: Agent Console (overlay + standalone TUI)

## Purpose

Provide observability into all running and recently completed Claude Code agent sessions. Two surfaces: a `~` overlay inside the TUI for quick status checks, and a standalone `ccc console` command for real-time streaming monitoring.

## Interface

- **Inputs**: Agent session data from daemon (via DB + event bus for overlay, via daemon RPC for console)
- **Outputs**: Overlay renders agent list + detail view; console renders live streaming panes
- **Dependencies**: Daemon socket, `cc_agent_costs`, `cc_sessions`, `cc_todos`, `cc_pull_requests` tables, event bus

## Feature 1: `~` Overlay (Static Agent Dashboard)

### Trigger

- `~` key toggles overlay from any tab
- `~` or `Esc` dismisses overlay
- Works like the existing `?` help overlay — drawn on top of the active tab

### List View

Displays all agents from the last 24 hours plus currently active/queued agents. Queried from DB on open.

Columns:

| Column  | Description                                      | Example                        |
|---------|--------------------------------------------------|--------------------------------|
| Status  | Icon: `●` running, `◌` queued, `✓` completed, `✗` failed, `⊘` stopped | `●`               |
| Origin  | Source label: todo display ID + title, or PR ref + category | `TODO #113 — Fix auth bug` |
| Elapsed | Duration (running) or total time (completed)     | `3m12s`                        |
| Cost    | USD spent                                        | `$0.42`                        |

Sorting: active/queued first (by start time desc), then completed/failed (by end time desc).

### Navigation

- `j`/`k` or `↑`/`↓` to select row
- `Enter` to open detail view
- `Esc` to return to list (from detail) or dismiss overlay (from list)

### Detail View

Shows all available metadata for the selected agent:

- **Session ID** — Claude session UUID
- **Status** — running, queued, completed, failed, stopped
- **Origin** — full source reference (e.g., `todo:113`, `pr:47:review`)
- **Title** — todo title or PR title
- **Project dir** — working directory
- **Repo** — git remote URL
- **Branch** — git branch
- **Started at** — ISO8601 timestamp
- **Ended at** — ISO8601 timestamp (if completed)
- **Duration** — elapsed or total
- **Tokens in** — input token count
- **Tokens out** — output token count
- **Cost** — USD
- **Blocking state** — whether agent is waiting on user input
- **Agent category** — e.g., "review", "respond", "todo"
- **Summary** — agent-generated summary (if completed)

Scrollable with `j`/`k` if content exceeds viewport. Scroll offset is clamped so the user cannot scroll past the last row of content — when all content fits in the viewport, scrolling is disabled (offset stays at 0).

### Live Updates

While overlay is open, subscribe to event bus for `agent.started`, `agent.stopped`, `agent.finished` events. New agents appear in the list, status changes reflected without re-querying DB.

On `agent.cost_updated` daemon events (broadcast when `RecordCost` fires, throttled to ≤1 per 2s per agent), re-fetch `ListAgentHistory` to pick up live cost/token updates. This is event-driven — no polling needed. All TUI instances subscribed to the daemon receive the same updates.

The TUI host handles `agent.cost_updated` at the model level: when the console overlay is visible, it re-fetches agent history entries and updates the overlay's list. It also immediately re-polls the daemon budget status (bypassing the normal 5-second polling interval) so the budget widget stays current with live spend.

## Feature 2: `ccc console` (Live Streaming TUI)

### Entry Point

- `ccc console` — standalone bubbletea program
- Runs in its own terminal (e.g., tmux pane, split terminal)
- Connects to daemon via Unix socket (`~/.config/ccc/daemon.sock`)
- Read-only — no agent control (no stop/restart)

### Layout

Left sidebar (~25 chars) + right focus pane (remaining width).

```
┌─ Sidebar ──────────┬─ Focus Pane ───────────────────────────────┐
│ ● TODO #113 auth   │ TODO #113 — Fix auth bug                   │
│   PR #47 review    │ ──────────────────────────────────────────  │
│   TODO #98 pagi…   │ ⠋ Reading internal/auth/oauth.go           │
│ ○ TODO #102 slack  │ → Found token refresh logic at line 142    │
│                     │ → Token not refreshed when expired         │
│                     │ ⠋ Editing oauth.go:142-158                 │
│                     │ → Applied fix: check expiry before call    │
│                     │ ⠋ Running make test...                     │
│                     │ → 42 passed, 0 failed                      │
│ ── completed ──     │                                            │
│ ✓ TODO #91 refac   │                                            │
│ ✗ PR #39 respond   │                                            │
└─────────────────────┴────────────────────────────────────────────┘
```

### Sidebar

- Lists all agents: active/queued on top, completed/failed below a separator (dimmed)
- `↑`/`↓` to select agent, selected row highlighted
- Scrolls if agents exceed visible height
- Each item shows: status icon + short label (truncated to fit)
- Color-coded by status: green (running), yellow (queued), dim (completed), red (failed)

### Focus Pane

- Streams real-time output of the selected agent
- Shows tool calls, file reads/edits, thinking indicators as they happen
- Auto-scrolls to bottom
- Scrollback buffer for reviewing earlier output
- When selecting a completed agent, shows their full output history

### Data Source

- New daemon RPC `StreamAgentOutput` — streams JSONL events for a specific agent session
- The daemon already parses agent stdout events (`internal/agent/impl.go`); this RPC pipes them to the console client
- New daemon RPC `ListAgentHistory` — returns agents from last 24h with metadata (status, cost, origin, timestamps). Used on startup and for sidebar population.
- Polls for new agents periodically (or subscribes to daemon events via the socket)

### Empty State

"No agents running. Watching for activity..." with a spinner. Updates as soon as an agent starts.

### Agent Lifecycle in Console

- New agent starts → appears in sidebar, auto-selected if nothing else is focused
- Agent completes/fails → moves below separator, dimmed, still selectable
- Agents persist in sidebar for the console session (not just 24h — anything that happened while console was open)

## Shared Infrastructure

### Agent Origin Labeling

When an agent launches, tag it with its source for display purposes:

- `todo:<display_id>` — agent launched from a todo
- `pr:<number>:<category>` — agent launched for PR review/respond
- `manual` — agent launched manually

This data partially exists scattered across tables (`cc_todos.session_id`, `cc_pull_requests.agent_session_id`). Consolidate into a queryable form — either a new column on `cc_agent_costs` or a join query.

### Daemon RPCs (new)

- `ListAgentHistory` — returns agent metadata for last 24h. Both overlay and console use this.
- `StreamAgentOutput` — streams JSONL events for a given agent session. Console-only.

### No New Database Tables

All data comes from existing tables via joins:
- `cc_agent_costs` — cost and token data
- `cc_sessions` — session metadata (project, repo, branch, state)
- `cc_todos` — todo origin (display_id, title, session_id)
- `cc_pull_requests` — PR origin (number, agent_session_id, agent_category)

## Implementation Notes

### Shared Formatting Helpers (`internal/ui/agent_format.go`)

Shared formatting functions used by both the overlay and standalone console:
- `AgentStatusIcon(status)` — returns status character icon
- `AgentStatusColor(status)` — returns lipgloss color for status
- `FormatAgentElapsed(entry)` — returns human-readable elapsed time
- `FormatDuration(d)` — formats a duration as short string

### Standalone Console Entry Point (`cmd/ccc/console.go`)

`runConsole(args)` is the entry point for `ccc console`. It:
1. Dials the daemon Unix socket via `daemon.NewClient(config.ConfigDir() + "/daemon.sock")`
2. Creates a `consoleModel` and runs `tea.NewProgram` with `tea.WithAltScreen()`
3. Polls every 1 second via `tea.Tick` — calls `ListAgentHistory(24)` for sidebar, `StreamAgentOutput(agentID)` for focus pane
4. Key bindings: `q`/`ctrl+c` quit, `j`/`down` and `k`/`up` move cursor
5. Layout: `lipgloss.JoinHorizontal` with sidebar (28 chars, or 20 if width < 60) + focus pane (remaining)
6. If daemon connection fails, prints helpful error: "Is the daemon running? Try: ccc daemon start"

### Overlay State (`internal/tui/console_overlay.go`)

`consoleOverlay` struct holds: `visible`, `entries`, `cursor`, `detail`, `scroll`.
- `toggle(entries)` — flip visible, load entries, reset cursor/detail/scroll
- `close()` — hide, reset state
- `selected()` — return entry at cursor
- `renderList(w, h)` — list view with status icon, origin (35 chars), elapsed, cost
- `renderDetail(w, h)` — all metadata fields, scrollable
- `render(w, h)` — dispatches to list or detail

Box: 70 chars wide (or width-4 if narrow), rounded border `#3b4261`, padding 1,2, centered via `lipgloss.Place`.

## Out of Scope

- Agent control (stop/restart/requeue) from either surface
- Changes to agent launch or governance
- Changes to the refresh pipeline
- New database tables
- The console does not access the DB directly — all data comes via daemon RPCs

## Test Cases

- Overlay opens/closes with `~` key from any tab
- `~` key is ignored (not intercepted) when the active plugin is in text input mode (editing a todo title, command mode, editing a setting value); the plugin's `HandleKey` gets first chance at the key, and the overlay only opens if the plugin returns `ActionUnhandled` or `ActionNoop`
- Overlay list shows running agents with correct status icons
- Overlay list shows completed agents from last 24h
- Overlay detail view displays all metadata fields
- Overlay updates when agent status changes (via event bus)
- `ccc console` connects to daemon socket and shows agent list
- Console sidebar reflects agent start/complete/fail in real time
- Console focus pane streams live output for selected agent
- Console handles 0 agents (empty state)
- Console handles 5+ agents (sidebar scrolls)
- Selecting a completed agent in console shows its output history
- Origin labels display correctly for todo-sourced and PR-sourced agents
