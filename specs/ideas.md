# CCC Ideas & Future Explorations

Parking lot for ideas that come up during development. Not committed to — just captured.

---

## Plugin Lifecycle Events from User Actions

**Idea:** Add user-driven lifecycle events that the host fires and plugins can subscribe to via the EventBus.

**Proposed events:**
- `onStart` — TUI has launched, all plugins initialized
- `onTabView(slug)` — User navigated to a plugin's tab (becomes the active view)
- `onTabLeave(slug)` — User navigated away from a plugin's tab
- `onLaunch(dir, args)` — User is about to launch a Claude session
- `onReturn` — User returned from a Claude session back to the TUI
- `onShutdown` — TUI is exiting

**Why:** Plugins could react to user navigation patterns — e.g., refresh data when a tab becomes visible instead of on a timer, log usage analytics, animate entry/exit transitions, or lazy-load expensive data only when the tab is first viewed.

**Where it fits:** Extends the EventBus from Stage 1. The host fires these events at the appropriate points in model.go's Update/Init/shutdown flow. Plugins subscribe during their Init().

---

## Multi-Instance Coordination

**Problem:** Multiple CCC instances share the same SQLite DB. Several things can go wrong:
- Two instances both trigger `ccc-refresh` simultaneously (API thrashing, DB write races)
- Instance A completes a todo; instance B shows it active until next poll
- Auto-refresh timers can align across instances

**Refresh locking (do soon):** `ccc-refresh` should acquire a PID lockfile (`~/.config/ccc/data/refresh.lock`) on start, release on exit. Instances check before spawning. Detect stale locks by checking if PID is still alive. User pressing `r` should only refresh if no lock is held.

**Cross-instance notification (explore later):** Instances could register unix sockets on startup. A lightweight notify mechanism (`ccc notify refresh-complete`, `ccc notify todo-added`) pokes all running instances to reload from DB. External tools and scripts can call this too. This gives daemon-like coordination without an always-on process.

**Full daemon (maybe never, maybe eventually):** Centralizes refresh, locking, and event dispatch into one process. Instances become thin display layers that subscribe to change notifications. Unlocks external triggers (webhooks, cron, MCP tool calls pushing events). But adds operational complexity — process management, crash recovery, debugging. The unix socket approach may be sufficient.

**Design principle:** Each CCC instance should be treated like an external trigger. If the coordination mechanism works for "a script wants to tell CCC something happened," it works for multi-instance too. Don't build instance-to-instance communication — build a general "notify CCC" mechanism that instances also use.

---

## Data Source Plugins (Refresh as a Plugin System)

**Problem:** Today, data sources (Google Calendar, Gmail, GitHub, Slack, Granola) are hardcoded into the monolithic `ccc-refresh` binary. Adding a new data source requires a pull request to the main platform. There's no way for a third-party developer to say "here's a plugin that adds a Jira tab AND fetches Jira data during refresh."

**Current architecture:**
- `internal/refresh/` is a single package that knows about every data source
- Config flags (`calendar.enabled`, `github.enabled`, etc.) toggle which sources run
- The command center plugin displays data from refresh but has no control over what gets fetched
- A plugin can add a tab, but can't contribute data to refresh

**Idea:** Make data sources pluggable. A plugin could register both:
1. **A tab** (UI) — what the user sees and interacts with
2. **A data source** (refresh) — code that fetches data and writes to the DB during refresh

This creates a full-stack plugin: a Jira plugin would provide a Jira tab showing issues AND a refresh hook that fetches Jira data on schedule.

**Possible design:**
- Add a `DataSource` interface alongside the existing `Plugin` interface:
  ```
  DataSource {
      FetchData(ctx RefreshContext) error  // called during refresh
      MergeRules() MergeConfig             // how to merge fresh data with existing
  }
  ```
- Plugins optionally implement `DataSource` in addition to `Plugin`
- The refresh binary (or daemon) discovers all registered data sources and runs them in parallel
- Built-in data sources (calendar, gmail, github, slack, granola) get extracted into plugins that implement both interfaces
- External plugins could implement `DataSource` via the JSON-lines protocol with additional message types

**Migration path:**
1. Today: monolithic refresh, config flags toggle sources (this is what we have)
2. Near-term: extract each source into its own internal package within refresh, keeping the same binary
3. Mid-term: define the DataSource interface, built-in sources implement it
4. Long-term: external plugins can implement DataSource too

**Relationship to command center plugin:** Today the CC plugin displays all data from all sources in one view. With data source plugins, the CC plugin would depend on specific data source plugins (calendar, todos, threads). But a third-party plugin might bring its OWN tab and its OWN data — e.g., a Jira plugin that adds a "Jira" tab and fetches Jira issues independently.

**Relationship to daemon idea:** A daemon would be the natural place to run data source plugins on schedule, rather than spawning a separate refresh binary. The daemon manages plugin lifecycles (both UI and data) and notifies TUI instances when data changes.

**Why not now:** The current refresh architecture works fine for the built-in sources. This becomes valuable when external developers want to add data sources without forking the project. Build toward it incrementally — start by extracting sources into packages, then add the interface when the first external use case appears.

**Sprint 2 progress:** The prerequisite step is now complete. Types are consolidated in `internal/db/` as the single source of truth, refresh writes directly to SQLite via `db.DBSaveRefreshResult()`, and the `ccc-refresh` binary exists at `cmd/ccc-refresh/main.go`. The next step toward full Data Source Plugins is defining the `DataSource` interface and extracting each fetcher (calendar, github, granola, slack, gmail) into separate plugin packages.

---

## SettingsProvider Interface (Plugin-Owned Settings)

**Idea:** Plugins could implement an optional `SettingsProvider` interface so the Settings plugin delegates detail view rendering to each plugin rather than hardcoding it.

**Proposed interface:**
```go
type SettingsProvider interface {
    SettingsView(width, height int) string
    HandleSettingsKey(msg tea.KeyMsg) Action
}
```

The Settings plugin would check for this via type assertion on each plugin. If a plugin implements `SettingsProvider`, the Settings plugin calls `SettingsView()` to render the detail view and `HandleSettingsKey()` to handle keypresses, rather than using its own hardcoded detail view logic.

**Why:** This is the natural evolution after Sprint 2's hardcoded detail views. Currently the Settings plugin knows how to render detail views for Calendar, GitHub, Granola, and external plugins. As more plugins and data sources are added, hardcoding each one becomes unsustainable. With `SettingsProvider`, each plugin owns its own settings UI.

**Not implemented yet** — noted for when the first external plugin needs custom settings. The current hardcoded approach is fine for the built-in set.
