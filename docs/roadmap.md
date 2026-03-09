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

### Sprint 3+4: Polish, Hardening, MCP Consolidation (in progress)

**Goal:** Make CCC reliable enough to use every day. Fix bugs, smooth rough edges, consolidate MCP servers, add operational infrastructure.

- [ ] Fix TUI glitches identified during first real usage
- [ ] Refresh configurability (intervals, retry behavior, staleness threshold)
- [ ] `ccc doctor` — diagnostic command checking config, credentials, connectivity
- [ ] `ccc install-schedule` — scheduled refresh via launchd plist
- [ ] Move gmail + things MCP servers into monorepo under `servers/`
- [ ] MCP config generation in `ccc setup` (output `.claude/mcp.json` snippets)
- [ ] `make install` builds enabled MCP servers
- [ ] Docs cleanup — remove stale Supabase/memory references, update specs
- [ ] Auto-refresh on TUI startup if data is stale (>5 min old)
- [ ] Cross-instance notification (unix socket) so multiple TUI instances stay in sync
- [ ] Remove any remaining personal content / hardcoded references

### Sprint 5: Architecture Evolution

**Goal:** Evolve the plugin architecture to support third-party data sources and richer plugin capabilities.

- [ ] Data Source Plugins — extract fetchers into `DataSource` interface, enable third-party data sources
- [ ] SettingsProvider Interface — plugins own their settings UI
- [ ] Plugin Lifecycle Events — onTabView, onLaunch, onReturn for lazy loading and analytics
- [ ] Multi-agent codebase review for large files

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
