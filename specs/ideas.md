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
