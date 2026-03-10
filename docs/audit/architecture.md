# Architecture & Coupling Audit

## Summary

The layering is mostly clean with `db` as a proper leaf package and `plugin` as the interface boundary. However, there are two notable violations: `commandcenter` imports `refresh` (crossing the TUI/data boundary), and `config` imports `db` in its doctor module (inverting the expected dependency direction). The `commandcenter` plugin is also a God object at 3,265 lines.

## Package Dependency Map

```
cmd/ccc           -> config, db, external, llm, plugin, tui
cmd/ccc-refresh   -> config, db, llm, refresh

tui               -> builtin/commandcenter, builtin/sessions, builtin/settings, config, db, plugin
  builtin/commandcenter -> config, db, llm, plugin, refresh  (!)
  builtin/sessions      -> config, db, plugin
  builtin/settings      -> config, plugin

plugin            -> config
config            -> db  (doctor.go only)
external          -> config, plugin
refresh           -> db, llm
llm               -> (no internal imports)
db                -> (no internal imports — leaf package)
```

## Layering Violations

### 1. `commandcenter` imports `refresh` — severity: HIGH

- **File:** `/Users/aaron/Personal/claude-command-center/internal/builtin/commandcenter/refresh.go`
- The `commandcenter` plugin (a TUI-side component) imports `internal/refresh` to call `refresh.IsLocked()`. This creates a dependency from the TUI layer into the data-fetching layer.
- The `commandcenter` plugin spawns `ccc-refresh` as a subprocess via `exec.Command`, but also links against the `refresh` package to check the lock file. This means the TUI binary (`cmd/ccc`) transitively depends on the entire `refresh` package, including all its data source implementations (calendar, gmail, github, slack, granola) and their dependencies.
- **Impact:** Increases the binary size of `ccc` and couples TUI to refresh internals. The lock check should be extracted to a shared utility or the `config` package.

### 2. `config` imports `db` — severity: MODERATE

- **File:** `/Users/aaron/Personal/claude-command-center/internal/config/doctor.go`
- The `config` package imports `db` to open the database and check data freshness in the `RunDoctor()` function. This inverts the expected dependency direction (`config` should be lower-level than `db`).
- `plugin` also depends on `config`, so this creates a longer chain: `plugin -> config -> db`.
- **Impact:** Minor in practice since `doctor.go` is a diagnostic tool, but it violates the stated architecture where `config` should not depend on `db`.

### 3. `tui` imports concrete builtin plugin packages — severity: LOW

- **File:** `/Users/aaron/Personal/claude-command-center/internal/tui/model.go`
- The `tui` package directly imports `builtin/commandcenter`, `builtin/sessions`, and `builtin/settings` to construct plugin instances. It also uses compile-time interface checks like `var _ plugin.Starter = (*sessions.Plugin)(nil)`.
- This is a pragmatic trade-off (the host needs to wire things up), but it means `tui` is tightly coupled to the specific set of built-in plugins rather than discovering them through the registry.

## High Coupling

### `builtin/commandcenter` — 5 internal imports, 3,265 lines

This is the highest fan-out package in the project:
- Imports: `config`, `db`, `llm`, `plugin`, `refresh`
- Files: 7 files totaling 3,265 lines
- `commandcenter.go` alone is 1,330 lines

This plugin does too much:
- Todo CRUD (create, complete, dismiss, defer, promote, undo)
- Thread management (create, pause, start, close)
- Calendar time-block booking
- LLM-powered command processing (natural language to todo)
- LLM-powered todo editing
- LLM-powered focus recommendations
- Background refresh orchestration (spawning `ccc-refresh`)
- Detail views, help overlays, flash messages
- Two distinct sub-views (command center + threads)

### `tui` — 6 internal imports

Imports: `builtin/commandcenter`, `builtin/sessions`, `builtin/settings`, `config`, `db`, `plugin`

The `tui` package has high fan-in because it wires everything together. The `db` import is only used for passing `*sql.DB` through, so it could potentially be removed by accepting a more generic type.

### `cmd/ccc` — 6 internal imports

Imports: `config`, `db`, `external`, `llm`, `plugin`, `tui`

This is the composition root, so high fan-out is expected and acceptable.

## Plugin Interface Quality

The `plugin.Plugin` interface at `/Users/aaron/Personal/claude-command-center/internal/plugin/plugin.go` is well-defined with 13 methods covering identity, lifecycle, database, display, input, routing, and scheduling.

**Strengths:**
- Clean separation of concerns in the interface design
- `plugin.Context` provides everything plugins need via dependency injection
- `plugin.Action` provides a clear return protocol for key/message handling
- Optional interfaces (`Starter`, `SetupFlow`) keep the core interface minimal
- Lifecycle messages (`TabViewMsg`, `TabLeaveMsg`, `LaunchMsg`, `ReturnMsg`) are defined in the `plugin` package to avoid circular imports

**Weaknesses:**
- `Context.Styles` and `Context.Grad` are typed as `interface{}` with comments like `// *tui.Styles -- interface to avoid circular import`. This is a code smell indicating the styles should either live in `plugin` or in a shared package.
- `Context.LLM` is also `interface{}` rather than `llm.LLM`. The `commandcenter` plugin has to type-assert it: `if l, ok := ctx.LLM.(llm.LLM); ok`. Since `llm` has no internal imports, `plugin` could safely import `llm` and type `LLM` properly.
- Plugins reach directly into `db` for data access (e.g., `db.LoadCommandCenterFromDB`, `db.DBCompleteTodo`). There is no data access layer abstraction — plugins call `db` functions directly with raw `*sql.DB`. This is pragmatic for a small project but limits testability.
- The `PendingLaunchTodo()` / `SetPendingLaunchTodo()` methods on both `commandcenter.Plugin` and `sessions.Plugin` are public methods outside the `plugin.Plugin` interface, suggesting a cross-plugin communication pattern that bypasses the event bus.

## Event Bus Usage

### Event Topics in Use

| Topic | Publisher | Subscriber(s) |
|---|---|---|
| `todo.completed` | commandcenter | settings |
| `todo.created` | commandcenter | settings |
| `todo.dismissed` | commandcenter | settings |
| `todo.deferred` | commandcenter | settings |
| `todo.promoted` | commandcenter | settings |
| `todo.edited` | commandcenter | settings |
| `pending.todo` | commandcenter | sessions |
| `pending.todo.cancel` | sessions | commandcenter |
| `data.refreshed` | commandcenter | sessions |
| `config.saved` | settings | commandcenter |
| `datasource.toggled` | settings | (none) |
| `palette.changed` | settings | (none) |

### Assessment

**Strengths:**
- The event bus is used consistently for cross-plugin communication. No plugin imports another plugin directly.
- Event topics follow a clear `noun.verb` naming convention.
- All bus interactions are nil-guarded (`if p.bus != nil`).

**Weaknesses:**
- Two published events (`datasource.toggled`, `palette.changed`) have no subscribers. They may be intended for future use or external plugins.
- The `pending.todo` / `pending.todo.cancel` pair creates a request-response pattern over a pub/sub bus. The `commandcenter` publishes a `pending.todo` event containing todo fields as `map[string]interface{}`, and `sessions` reconstructs a `db.Todo` from it. This is fragile — field additions or renames will silently break. A shared event payload type would be safer.
- The `settings` plugin subscribes to 6 todo events solely for logging. This is verbose; a wildcard subscription or a single `todo.*` topic would reduce boilerplate.
- External plugins can publish events through the bus (via `external.go` line 247), but there is no documentation of the event contract for external plugin developers.

### Backdoor Communications

There is one backdoor: `commandcenter.Plugin` exposes `PendingLaunchTodo()` and `SetPendingLaunchTodo()` as public methods, and `sessions.Plugin` has the same. While these are not currently called cross-plugin (the event bus handles it), they exist as a public API that could bypass the bus.

## Binary Separation

### `cmd/ccc` (TUI binary)

- Imports: `config`, `db`, `external`, `llm`, `plugin`, `tui`
- Transitively imports: `builtin/*`, `refresh` (via `commandcenter`)
- Responsibilities: Config loading, DB setup, plugin wiring, TUI loop, Claude session launching

### `cmd/ccc-refresh` (data fetcher binary)

- Imports: `config`, `db`, `llm`, `refresh`
- Responsibilities: Config loading, DB setup, lock acquisition, data source fetching, merge, save

### Assessment

The two binaries share `config`, `db`, and `llm` as intended. They communicate solely through the SQLite database (WAL mode), which is a clean boundary.

**Problem:** Because `commandcenter` imports `refresh` (for `IsLocked()`), the `ccc` binary transitively links against the entire `refresh` package and all its data source implementations. This means `ccc` includes code for fetching from Google Calendar, Gmail, GitHub, Slack, and Granola APIs — none of which it uses. The `ccc` binary should only need the lock-check function, not the full refresh pipeline.

## Recommendations

### Priority 1: Break `commandcenter`'s dependency on `refresh`

Extract `refresh.IsLocked()` and `refresh.AcquireLock()` into a shared `internal/lockfile` package (or put them in `config`). This eliminates the layering violation and reduces the `ccc` binary's dependency set.

**Files to change:**
- `/Users/aaron/Personal/claude-command-center/internal/builtin/commandcenter/refresh.go`
- `/Users/aaron/Personal/claude-command-center/internal/refresh/lock.go`

### Priority 2: Split the `commandcenter` plugin

The 3,265-line plugin handles too many responsibilities. Consider splitting into:
- `commandcenter` — calendar + todos view, cursor management, rendering
- `commandcenter/llmops` or similar — LLM command processing, prompt building, response parsing
- Move thread management into its own plugin or at least its own file with clearer boundaries

### Priority 3: Type `Context.LLM` properly

Change `Context.LLM` from `interface{}` to `llm.LLM`. The `llm` package has zero internal imports, so `plugin` can safely import it. This eliminates the fragile type assertion in `commandcenter.Init()`.

### Priority 4: Move `doctor.go` out of `config`

Move `RunDoctor()` to `cmd/ccc` (since it is only called from the CLI entry point) or to a new `internal/doctor` package. This removes the `config -> db` dependency inversion.

**File to move:** `/Users/aaron/Personal/claude-command-center/internal/config/doctor.go`

### Priority 5: Strengthen event bus payload types

Replace `map[string]interface{}` event payloads with typed structs for the `pending.todo` request-response pattern. Define a `PendingTodoEvent` struct in the `plugin` package that both `commandcenter` and `sessions` use.

### Priority 6: Extract styles to a shared package

The `interface{}` types for `Context.Styles` and `Context.Grad` indicate that styles need to be accessible without creating circular imports. Consider a `internal/theme` or `internal/styles` package that both `tui` and `plugin` can import.
