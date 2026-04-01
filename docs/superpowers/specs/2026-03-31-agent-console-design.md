# Agent Console — Design Spec

**Date:** 2026-03-31
**Status:** Approved

## Problem

No visibility into what background agents are doing. When CCC launches agents for todos, PR reviews, etc., the user has no way to see what's running, what finished, or what's happening in real time.

## Solution

Two complementary surfaces:

### 1. `~` Overlay (Static Dashboard)

Toggle from any tab with `~`. Shows a list of all agents from the last 24 hours plus currently active/queued. Select with `j`/`k`, `Enter` for detail view showing all metadata (session ID, tokens, cost, project, timestamps, summary).

Data comes from DB queries + event bus subscription for live updates.

### 2. `ccc console` (Live Streaming TUI)

Standalone bubbletea app for a dedicated terminal. Left sidebar lists agents, right pane streams the focused agent's live output (tool calls, file edits, thinking indicators).

- Sidebar: `↑`/`↓` to select, active on top, completed dimmed below separator
- Focus pane: real-time JSONL stream from daemon, auto-scroll, scrollback buffer
- Connects via daemon Unix socket, read-only

### Shared Infrastructure

- Agent origin labeling: tag each agent with `todo:<id>`, `pr:<num>:<category>`, or `manual`
- New daemon RPCs: `ListAgentHistory` (both surfaces), `StreamAgentOutput` (console only)
- No new DB tables — joins existing `cc_agent_costs`, `cc_sessions`, `cc_todos`, `cc_pull_requests`

## Out of Scope

- Agent control (stop/restart) from either surface
- Changes to agent launch/governance
- Direct DB access from console (all via daemon RPCs)

## Design Decisions

- **Overlay, not tab** — `~` overlay matches "always one hotkey away" without adding tab clutter
- **Sidebar on left** — focus pane gets more width, sidebar is scannable
- **24-hour history** — survives daemon restarts, bounded growth
- **Read-only** — observability first, control can come later
