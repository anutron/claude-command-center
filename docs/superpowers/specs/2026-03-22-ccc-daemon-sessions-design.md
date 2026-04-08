# DESIGN: CCC Daemon + Session Registry + Agent Runner Migration

**Date:** 2026-03-22
**Origin:** Comparison with [drn/argus](https://github.com/drn/argus) — Darren's terminal-native LLM orchestrator
**Notion doc:** [Argus vs CCC — Architecture Comparison](https://www.notion.so/32ba84ed4024815b8955fb47c31e5d87)

## Overview

Three-phase evolution of CCC's architecture, inspired by Argus's daemon model:

1. **Thin daemon** — persistent process replacing ai-cron, with session registry
2. **Sessions UI** — "Active Sessions" view in the sessions plugin
3. **Agent runner migration** — move agent lifecycle into daemon so agents survive TUI restarts

Each phase depends on the previous one. The daemon starts small and grows.

---

## Phase 1: Daemon Core

### New Package

`internal/daemon/` — persistent background process.

### Lifecycle

- `ccc daemon start` — forks a detached process (`fork + Setsid`). Writes PID to `~/.config/ccc/daemon.pid`.
- `ccc daemon stop` — sends SIGTERM via PID file.
- TUI auto-starts daemon on launch if not running. Connects over socket. If daemon dies mid-session, TUI attempts automatic restart. Shows error only if restart fails.

### Protocol

JSON-RPC over Unix socket at `~/.config/ccc/daemon.sock`.

Clients: TUI, CLI commands (`ccc register`, `ccc refresh`, `ccc update-session`).

### Security

- Socket created with `0600` permissions — only the owning user can connect.
- PID file same permissions.
- No TCP listener, ever. Unix socket only.
- Phase 1 RPC surface is low-risk: `Refresh`, `RegisterSession`, `UnregisterSession`, `UpdateSession`, `ListSessions`, `Subscribe`. No shell execution, no file mutation.
- Phase 3 (agent runner) adds process spawning — agents must match configured command allowlist, not arbitrary strings. Existing sandbox profiles apply.

### Event Subscription

After connecting, TUI sends a `Subscribe` RPC. Daemon pushes events over the connection:

- `data.refreshed` — refresh cycle completed
- `session.registered` — new session appeared
- `session.updated` — topic or status changed
- `session.ended` — PID gone, session moved to ended state

On reconnect (after daemon restart or network blip), the TUI does a full state reload via `ListSessions` rather than attempting event replay. Simple and correct — event replay adds complexity for minimal benefit given that session state is small.

Replaces the current "spawn ai-cron, wait for exit, reload DB" pattern with live push.

### Refresh Loop

Daemon runs the refresh cycle on a configurable timer (default 5m, matching the current ai-cron default). Same logic as current `cmd/ai-cron/main.go`, hosted in a long-lived process. Benefits:

- No cold start per cycle
- Holds state between cycles (last-fetched timestamps for incremental fetches)
- `ccc refresh` CLI triggers an immediate cycle via RPC instead of running its own process

### Migration from ai-cron

The daemon replaces ai-cron. On first `ccc daemon start`, the daemon removes the existing crontab entry (`ccc uninstall-schedule`). During the transition, the daemon holds `flock` on `refresh.lock` for backward compatibility — if an old ai-cron process is still scheduled, it will fail to acquire the lock and exit cleanly. Once the daemon is stable, remove `flock`. Track this with a `TODO(daemon-stable)` comment in the flock code and a skipped test: `TestFlockRemovedAfterDaemonMigration` that fails if `flock` is still referenced — unskip it when ready to clean up.

---

## Phase 1 (cont): Session Registry

### Registration Flow

Two integration points:

1. **Session start → Claude Code hook.** A `session_start` hook in Claude Code settings calls `ccc register --session-id $ID --pid $PPID --project $PWD`. This registers the session immediately, even before a topic is set. If the daemon is not running, `ccc register` starts it automatically (same auto-start behavior as the TUI). If auto-start fails, `ccc register` writes directly to `cc_sessions` in SQLite as a fallback — the daemon will pick it up on next start.

2. **Topic update → CLAUDE.md instruction.** The user's CLAUDE.md snippet (managed via [claude-skills](https://github.com/anutron/claude-skills)) instructs Claude to call `ccc update-session --topic "..."` after setting the topic via `/set-topic`. This keeps `/set-topic` generic and open-sourceable. CCC documents this as an optional setup step for users who want named sessions.

### Session Record

| Field | Source | Purpose |
|---|---|---|
| `session_id` | Claude's UUID | Resume with `--resume` |
| `topic` | `/set-topic` → CLAUDE.md instruction | Display name in TUI |
| `pid` | `$PPID` | Liveness detection |
| `project` | Working directory | Group by project |
| `repo` | Git remote | Context |
| `branch` | Git branch | Context |
| `worktree_path` | If in a worktree | Resume in correct dir |
| `state` | Daemon-managed | `active` / `ended` / `archived` |
| `registered_at` | Timestamp | Sort/age display |

### Liveness Detection

Daemon periodically checks if `pid` is still alive (`kill -0`). If gone, transitions session to `ended`. No explicit unregister hook needed.

### Storage

Daemon holds active sessions in memory, persists to `cc_sessions` table in existing SQLite DB. Writes are synchronous — every `RegisterSession`, `UpdateSession`, and state transition hits the DB immediately. No batching. If the daemon crashes, no session data is lost.

On daemon restart, loads from DB, prunes dead PIDs.

### Schema

```sql
CREATE TABLE cc_sessions (
    session_id TEXT PRIMARY KEY,
    topic TEXT,
    pid INTEGER,
    project TEXT,
    repo TEXT,
    branch TEXT,
    worktree_path TEXT,
    state TEXT NOT NULL DEFAULT 'active',  -- active | ended | archived
    registered_at TEXT NOT NULL,
    ended_at TEXT
);
```

Requires a `plugin.Migration` entry. `repo` and `branch` are derived server-side from `project` (working directory) via git commands when the daemon processes the `RegisterSession` RPC — the CLI client only passes `--project`.

### Session vs Bookmark

Sessions and bookmarks are different concerns:

- **Session** = "currently running or recently ended" — automatic, ephemeral
- **Bookmark** = "I want to come back to this" — intentional, permanent

A session can be bookmarked from the TUI. The existing bookmark system stays as-is.

### Session → Bookmark Lifecycle

When a session is bookmarked (action `b` in Phase 2), a row is copied from `cc_sessions` to `cc_bookmarks`. Both tables reference the same `session_id`. The session continues its normal lifecycle — it can end and eventually archive. The bookmark persists independently. Resuming from either the session list or the bookmark list uses the same `claude --resume <session_id>`.

### Retention

Ended sessions remain visible for 7 days (configurable via `session_retention` config key, e.g. `session_retention: "7d"`). After that, they move to `archived` state — hidden from the TUI list but preserved in the DB for future querying.

---

## Phase 2: Sessions Plugin Upgrade

### New View: "Active Sessions"

Added to the sessions plugin. Shows all registered sessions grouped by status:

- **Running** — PID alive
- **Ended** — PID gone, available for resume

Note: "Blocked" status (agent needs input) becomes available in Phase 3 when the agent runner moves to the daemon and stream-JSON monitoring is accessible via RPC. In Phase 2, CCC-spawned agents show as "Running" until they exit.

### List Layout

Each row: topic (or project/branch fallback), age, status indicator, project. Grouped by project, sorted by recency within each group.

### Actions

| Key | Action |
|---|---|
| `Enter` | Open in new iTerm tab — runs `claude --resume <session_id>` in the session's directory |
| `b` | Bookmark the session (copy to `cc_bookmarks`) |
| `d` | Dismiss — only available on ended/archived sessions (live sessions are truth, not dismissable) |
| `w` | Open session viewer (CCC-spawned agents only) |

### Data Flow

TUI subscribes to daemon events — no polling. `session.registered` adds to list. `session.updated` updates topic/status. `session.ended` moves to ended group.

---

## Phase 3: Agent Runner Migration to Daemon

### What Moves

The shared agent runner in `internal/agent/` (already extracted from command center, commit `4360403`):

- Session spawning (headless Claude via stream-JSON)
- Queue management (FIFO, max concurrent)
- Stream-JSON monitoring (status, blocked detection, event capture)
- JSONL logging
- Blocked detection (`SendUserMessage` / `AskUser` tool calls)
- Summary extraction

### Why

Agents survive TUI restarts. Today if CCC crashes or you quit, headless agents die. With daemon ownership, they keep running. TUI reconnects and picks up where it left off.

### New Daemon RPCs

| RPC | Purpose |
|---|---|
| `LaunchAgent(req)` | Queue or start an agent session |
| `StopAgent(id)` | Kill a running agent |
| `AgentStatus(id)` | Status, question if blocked, event count |
| `AgentEvents(id, since)` | Stream-JSON events since offset |
| `SendInput(id, message)` | Respond to blocked agent |
| `ListAgents()` | All active/queued agents |

### TUI as Thin Client

The command center plugin no longer spawns processes. It sends RPCs and renders responses. The session viewer connects to `AgentEvents` instead of reading from a local stdout pipe. Queue depth, concurrent limit, and auto-launch logic all live in the daemon.

### CCC-Spawned vs External Sessions

The daemon distinguishes:

- **CCC-spawned agents** — have full agent records with status, events, input capabilities
- **Externally registered sessions** — just session registry entries with PID liveness

The sessions list shows both. Only CCC-spawned agents support the full status/events/input surface.

### Migration Path

1. Daemon imports `internal/agent/` and hosts the runner (already extracted, commit `4360403`)
3. Command center plugin switches from direct `internal/agent/` calls to daemon RPC calls
4. PR plugin switches from direct `internal/agent/` calls to daemon RPC calls
5. Automation framework migrates to daemon (automations run post-refresh, which is already in the daemon)
6. Session viewer connects to daemon's `AgentEvents` stream

### Risk

This is the hardest phase — threading, process lifecycle, reconnection after TUI restart, replaying missed events. Phases 1-2 prove the socket/RPC/event machinery before adding process ownership on top.

---

## Implementation Notes

### CLAUDE.md Integration

The plan should look at the user's claude-skills repo (see [github.com/anutron/claude-skills](https://github.com/anutron/claude-skills)) to find the `/set-topic` invocation and add the appropriate CCC `update-session` call right before it.

### Session Registration Setup

CCC's README should document the CLAUDE.md snippet needed for session topic registration. Optionally, `ccc setup` could check `~/.claude/CLAUDE.md` for the snippet and offer to append it if missing. But this is a convenience — the README is sufficient.

### Example Hook

Include an example session-start hook in `examples/hooks/` showing the `ccc register` setup.

### Relationship to PR Automation Design

The agent runner extraction to `internal/agent/` is already done (commit `4360403`). Phase 3 moves this package behind the daemon's RPC layer — the `internal/agent/` package becomes the implementation that the daemon hosts, and plugins switch from calling it directly to calling it via RPC. The extraction is done; only the call path changes in Phase 3.

### Ideas Backlog

"CCC as a Global MCP Server" is captured in `docs/ideas.md`. The daemon from Phase 1 would be the natural host for the MCP HTTP server. This is a separate design cycle.
