# CCC Roadmap

## Vision

Extract the Command Center TUI from AI-RON into a standalone, installable project that anyone can use. SQLite-only, plugin-driven, configurable identity.

## Completed Work

### Sprint 1: Portable Binary (done)

- [x] Create repo with standard Go layout
- [x] Move TUI + refresh code from AI-RON
- [x] Config system (`~/.config/ccc/config.yaml`)
- [x] Replace all hardcoded paths with config reads
- [x] Configurable name + 5 built-in color palettes + custom
- [x] `make install` works from clean clone
- [x] Calendar supports multiple calendars from config

### Sprint 2: Plugin Architecture (done)

- [x] Plugin interface with Init/Update/View/Routes lifecycle
- [x] Plugin registry, event bus, shared logger
- [x] Namespaced SQLite migrations per plugin
- [x] External plugin protocol (JSON-lines over stdin/stdout)
- [x] Crash recovery and subprocess lifecycle management
- [x] Python plugin SDK + pomodoro example
- [x] 3 built-in plugins: Sessions, Command Center, Settings
- [x] Settings detail views with per-item config screens
- [x] Eliminate command-center.json — refresh writes directly to SQLite
- [x] `ccc-refresh` standalone binary with `-v`, `--dry-run`, `--no-llm`
- [x] PID lockfile refresh locking
- [x] Data source validation on enable (calendar, GitHub, Granola)
- [x] Specs for all features

---

## Remaining Work

### Sprint 3+4: Polish, Hardening, MCP Consolidation (done)

**Goal:** Make CCC reliable enough to use every day. Fix bugs, smooth rough edges, consolidate MCP servers, add operational infrastructure.

- [x] Fix TUI glitches: DB nil safety, RunClaude error handling, signal handling
- [x] Refresh configurability: configurable interval, status indicator, error display
- [x] `ccc doctor` — diagnostic command checking config, credentials, connectivity
- [x] `ccc install-schedule` / `ccc uninstall-schedule` — launchd plist management
- [x] Move gmail + things MCP servers into monorepo under `servers/`
- [x] MCP config generation in `ccc setup` (output `.claude/mcp.json` snippets)
- [x] `make install` builds enabled MCP servers
- [x] Docs cleanup — remove stale Supabase/memory references, update specs
- [x] Auto-refresh on TUI startup if data is stale
- [x] Cross-instance notification (`ccc notify`) via unix sockets
- [x] Audit for personal content / hardcoded references — clean

### Sprint 5: Messaging Architecture & Plugin Lifecycle

**Goal:** Unify the three messaging layers, add plugin lifecycle events, and clean up large files. CCC currently has three overlapping messaging systems that need to be rationalized before adding more features.

**Study area: messaging layer unification.** Today there are three layers:

1. **Event Bus (`plugin.EventBus`)** — intra-process pub/sub between plugins. Topic-based, synchronous handlers. Defined in Sprint 2 but currently unused by any plugin. Best suited for plugin-to-plugin async signals where the publisher doesn't care about the response ("todo completed", "config changed").

2. **Bubbletea message broadcast** — the host's `broadcastMessage()` sends every `tea.Msg` to every plugin's `HandleMessage()`. This is how plugins learn about ticks, window resizes, refresh completions, and cross-instance notifications. Noisy — plugins ignore 95% of messages. But it supports returning `tea.Cmd`, which the event bus cannot.

3. **Cross-instance notification** — unix socket per PID, `ccc notify` sends a string, listener injects `plugin.NotifyMsg` into bubbletea. Currently only triggers DB reload in CC plugin.

**Target architecture** — three layers forming a clean stack:

```
External (unix socket)  →  NotifyMsg into bubbletea
Host lifecycle          →  tea.Msg types (TabViewMsg, LaunchMsg, ReturnMsg)
Plugin-to-plugin        →  EventBus topics
```

Key insight: lifecycle events should be `tea.Msg` types (not event bus topics) because they need to return `tea.Cmd` (e.g., "I just became visible, kick off a data fetch"). The event bus stays reserved for fire-and-forget plugin-to-plugin signals.

**Work items:**

- [ ] Define lifecycle `tea.Msg` types: `TabViewMsg{Slug}`, `TabLeaveMsg{Slug}`, `LaunchMsg{Dir, Args}`, `ReturnMsg`
- [ ] Host fires lifecycle messages at tab switch (replacing bare `NavigateTo` call), before/after Claude launch
- [ ] CC plugin uses `TabViewMsg` for lazy reload instead of polling on tick
- [ ] Data Source Plugins — extract fetchers into `DataSource` interface, enable third-party data sources
- [ ] SettingsProvider Interface — plugins own their settings UI
- [ ] First real EventBus usage — CC publishes "todo.completed" / "todo.created", other plugins can subscribe
- [ ] Multi-agent codebase review — break up `commandcenter.go` (1200+ lines) into focused files

### Sprint 6: Distribution & Onboarding

**Goal:** Someone else can `git clone` + `make install` + `ccc setup` and have a working system.

- [ ] README with setup guide, architecture overview, screenshots
- [ ] Skills distribution — ship bookmark/wind-down/wind-up, optional symlink to `~/.claude/skills/`
- [ ] Example external plugin documentation
- [ ] Verification: clean Mac install test
- [ ] Ship to 2-3 beta testers, iterate on feedback

### Future

These are ideas, not commitments. Build toward them incrementally when the need arises.

- **Full Daemon** — centralize refresh + event dispatch, TUI instances become thin clients

See `docs/ideas.md` for detailed exploration of each.

---

## Principles

- **Ship what you use** — CCC should be Aaron's daily driver before it ships to anyone else
- **Spec-driven** — every feature has a spec in `specs/`, tests validate the spec
- **Plugin architecture for extensibility, monolith for reliability** — built-in plugins are compiled in, external plugins are optional
- **SQLite is the platform** — single file, WAL mode, shared between TUI and MCP servers
- **Incremental extraction** — don't build abstractions until you need them (Data Source Plugins wait until someone actually wants to add a third-party source)
