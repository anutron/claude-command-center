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
- `servers/things/` — Things MCP (macOS-only, optional)

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
