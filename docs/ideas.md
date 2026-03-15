# Ideas & Future Explorations

Parking lot for ideas that come up during development. Not committed to — just captured.

---

## Plugin Lifecycle Events

Add user-driven lifecycle events that the host fires and plugins can subscribe to via the EventBus.

**Proposed events:**
- `onStart` — TUI launched, all plugins initialized
- `onTabView(slug)` — user navigated to a plugin's tab
- `onTabLeave(slug)` — user navigated away
- `onLaunch(dir, args)` — about to launch a Claude session
- `onReturn` — returned from a Claude session
- `onShutdown` — TUI exiting

**Value:** Lazy-load expensive data on tab view, animate transitions, refresh on visibility instead of timer.

---

## Cross-Instance Notification

**Problem:** Multiple CCC instances share the same SQLite DB. Completing a todo in one instance doesn't reflect in others until next poll.

**Approach:** Unix socket notification mechanism. Instances register on startup. `ccc notify <event>` pokes all running instances to reload from DB. External scripts can use the same mechanism.

**Design principle:** Build a general "notify CCC" mechanism. Don't build instance-to-instance communication — if it works for "a script wants to tell CCC something happened," it works for multi-instance too.

**Progression:**
1. Unix socket notify (near-term, solves the immediate problem)
2. Full daemon (maybe eventually) — centralizes refresh, locking, event dispatch. Instances become thin display layers.

---

## Data Source Plugins

**Problem:** Data sources (calendar, Gmail, GitHub, Slack, Granola) are hardcoded into `ccc-refresh`. No way for a third-party to add a data source without forking.

**Idea:** `DataSource` interface alongside the `Plugin` interface. A plugin could register both a tab (UI) and a data source (refresh hook). Full-stack plugins — e.g., a Jira plugin provides a Jira tab AND fetches Jira data during refresh.

**Migration path:**
1. ~~Monolithic refresh with config flags~~ (done, Sprint 2)
2. Extract each source into its own internal package within refresh
3. Define `DataSource` interface, built-in sources implement it
4. External plugins can implement `DataSource` via JSON-lines protocol

**Sprint 2 completed prerequisites:** Types consolidated in `internal/db/`, refresh writes to SQLite via `db.DBSaveRefreshResult()`, `ccc-refresh` binary exists.

---

## SettingsProvider Interface

Let plugins own their settings UI instead of hardcoding detail views in the Settings plugin.

```go
type SettingsProvider interface {
    SettingsView(width, height int) string
    HandleSettingsKey(msg tea.KeyMsg) Action
}
```

**When:** When the first external plugin needs custom settings. Current hardcoded approach is fine for built-in set.

---

## `ccc setup` Wizard

Interactive first-run experience:
1. "What do you want to call your command center?" — free text, defaults to "Command Center"
2. "Pick a color palette" — live preview cycling through aurora, ocean, ember, neon, mono
3. "Enable Google Calendar?" → OAuth flow → "Which calendars?"
4. "Enable GitHub?" → username → repos to watch
5. "Enable Granola?" → yes/no
6. Writes config + credentials

---

## `ccc doctor` Command

Diagnostic command that checks:
- Config file exists and parses
- Credentials present for enabled data sources
- `gh` CLI authenticated (if GitHub enabled)
- SQLite DB accessible and schema current
- External plugin binaries exist and are executable
- `claude` CLI available (for LLM features)

---

## Scheduled Refresh (launchd/cron)

Auto-run `ccc-refresh` on a schedule so data is fresh when the TUI opens. Options:
- launchd plist (macOS native, survives reboots)
- `ccc install-schedule` command to set it up
- Default interval: 5 minutes
- Respect locking (already implemented)

---

## MCP Server Distribution

Fold MCP servers into the monorepo under `servers/`:
- `servers/gmail/` — Gmail MCP (already generic)

`ccc setup` generates `.claude/mcp.json` snippets. `make install` builds enabled servers.

---

## Memory as External Plugin

Memory-MCP lives in AI-RON — it's Aaron's personal memory system (workouts, sleep, observations, people). It is not part of CCC.

CCC could surface memory data via an external CCC plugin that talks to Supabase (or whatever backend AI-RON uses). This would be a read-only display plugin: "here's what memory-mcp knows about today."

This would be an AI-RON-side plugin, not a CCC built-in. AI-RON would ship the plugin, CCC would just run it via the external plugin protocol.

---

## Skills Distribution

Ship session management skills with the repo:
- `/bookmark` — save session reference
- `/wind-down` — save session context
- `/wind-up` — resume paused session

`ccc setup` optionally symlinks to `~/.claude/skills/`.

---

## Slack Channel as Todo Agent Intake

Private Slack channel dedicated to the user's todo agent. The user forwards threads, drops links, or types natural language commands into this channel.

**Flow:**
1. User forwards a Slack thread to their private todo-agent channel
2. Refresh agent picks it up (like the todo label in email)
3. Refresh interprets it as a command — could create a todo, book a calendar event, etc.
4. Agent posts a reply in the Slack thread with: "I have this todo with the following prompt: `<prompt>` in project `<dir>`"
5. User gives an emoji response (e.g., thumbs up) to approve
6. Next refresh cycle detects the emoji → kicks off the headless session
7. When the agent finishes, CCC posts back to the thread that it's ready for review

**Safety**: 100% of these funnel into todos with prompts the user reviews. No autonomous execution without prompt review. The Slack interaction is just a more convenient approval interface.

**Related**: Todo Agent Launcher (implemented 2026-03-14) provides the headless session infrastructure this builds on.

---

## Session Join for Agent Review

After a headless Claude session completes a todo, the user should be able to "join" the session — like session restore but for reviewing agent work. The user sees the full conversation history, can ask follow-up questions, and discuss the work Claude did.

**Prerequisite**: Todo Agent Launcher's session tracking (SessionID on todos).

---

## Interactive Headless Sessions (PTY-based)

For tasks where the agent should be able to ask clarifying questions during execution. Instead of stream-JSON monitoring, use a PTY to detect when Claude is waiting for input. Surface the question in CCC's todo list as a "blocked" state. User can answer from within CCC or join the session.

**Challenge**: Reliably distinguishing "Claude is thinking" from "Claude is waiting for input." Requires stable prompt-pattern detection or structured output format from Claude CLI.

---

## Smart Launch Mode Suggestion

The prompt-generation skill (`/todo-agent`) suggests Worktree mode when it detects the task involves code modifications to the target repo. Normal mode for everything else (research, docs, external API calls). User always overrides in the task runner.

---

## Status Line Updates for Spawned Claude Instances

When a Claude instance is spawned from a todo (headless or interactive), update the user's Claude Code status line to reflect it. This gives visibility into agent activity without switching tabs or checking the todo list.

**Possible display:**
- `🤖 Running: "Fix auth bug" (2m)` — active agent with task name and elapsed time
- `🤖 2 agents running` — summary when multiple are active
- Show in CCC's own status bar and/or pipe to Claude Code's status line config

**Prerequisite**: Todo Agent Launcher's session tracking. Needs a way to detect agent start/completion events — could use the EventBus or poll session status.
