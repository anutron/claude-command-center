# SPEC: External Plugin Adapter (internal/external)

## Purpose

Adapts external subprocess plugins into the `plugin.Plugin` interface. Manages subprocess lifecycle, JSON-lines IPC, crash recovery, and protocol translation. This package has no Bubbletea imports in its protocol or process layers — only the adapter uses `tea.KeyMsg`/`tea.Cmd` at the boundary, making it forward-compatible with a future daemon host.

## Interface

- **Input**: Command string (e.g., `python3 pomodoro.py`), `plugin.Context`
- **Output**: `*ExternalPlugin` implementing `plugin.Plugin`
- **Entry point**: `LoadExternalPlugins(cfg, ctx)` returns initialized plugins from config
- **Dependencies**: `internal/plugin`, `internal/config`

## Architecture

### Files

- `protocol.go` — Wire format types (`HostMsg`, `PluginMsg`, `RouteMsg`, `KeyBindingMsg`, `MigrationMsg`). Flat structs with `json:"omitempty"`. Separate field names avoid JSON key collisions across message types.
- `process.go` — `Process` struct managing `exec.Cmd`, stdin/stdout/stderr pipes, reader goroutines, sync/async message channels.
- `external.go` — `ExternalPlugin` struct implementing `plugin.Plugin` on top of `Process`.
- `loader.go` — `LoadExternalPlugins` function reading config and initializing plugins.

### Process

```go
type Process struct {
    cmd      *exec.Cmd
    stdin    io.WriteCloser
    mu       sync.Mutex        // protects stdin writes
    syncResp chan PluginMsg     // capacity 1, for view/action/ready responses
    asyncCh  chan PluginMsg     // capacity 64, for events/logs
    done     chan struct{}      // closed when process exits
    err      error
    logger   plugin.Logger
    slug     string
}
```

- Launches subprocess via `sh -c <command>` with `PYTHONUNBUFFERED=1`
- Stdout reader goroutine routes messages: `view`/`action`/`ready` → `syncResp`, everything else → `asyncCh`
- Stderr reader goroutine logs each line as a warning
- `Send` is mutex-protected; checks process liveness before writing
- `Receive` blocks on `syncResp` with configurable timeout
- `DrainAsync` non-blocking drain of all pending async messages

### ExternalPlugin

Implements every method of `plugin.Plugin`:

| Method | Behavior |
|--------|----------|
| `Init(ctx)` | Start subprocess, send `init` (no config), wait for `ready` (5s), send scoped `config`, validate migrations, run migrations |
| `View(w,h,f)` | Send `render`, receive `view` (50ms timeout), return content or cached fallback |
| `HandleKey(msg)` | If crashed + "r" → restart. Otherwise send `key`, receive `action` (50ms timeout) |
| `HandleMessage(msg)` | Update dimensions on `WindowSizeMsg`. On any msg: drain async channel (events→bus with slug-prefixed topics, logs→logger) |
| `Refresh()` | Return `tea.Cmd` that sends `refresh` (fire-and-forget) |
| `NavigateTo(route, args)` | Send `navigate` message |
| `Shutdown()` | Send `shutdown`, wait 2s, kill |
| Identity methods | Return cached values from `ready` handshake |

### Key Design Decisions

1. **Ticks not forwarded** — Host calls `HandleMessage` at 18fps for ticks. ExternalPlugin does NOT forward these. Only `View()` triggers a render request, and View is only called for the active tab.
2. **Sync/async split** — The stdout reader routes responses to two channels. `view`/`action` are synchronous responses to host requests. `event`/`log` are async and drained during `HandleMessage`.
3. **50ms render timeout** — On timeout, return cached view (no flicker). On process death, set error state.
4. **Crash recovery** — Error state shows "press r to restart". Pressing `r` calls `startProcess()` which reinitializes the subprocess from scratch.
5. **No Bubbletea in protocol/process layers** — Only `external.go` imports Bubbletea (for `tea.KeyMsg`, `tea.Cmd`, `tea.Msg`). Protocol and process layers are pure Go, reusable by a future daemon host.
6. **Config scoping** — The `scopeConfig` function uses reflection to match requested scope names against the YAML struct tags of `config.Config` fields. Only matching top-level fields are included in the scoped config map. This ensures plugins cannot access credentials or settings outside their declared needs.
7. **Event topic prefixing** — When draining async messages in `HandleMessage`, event topics are prefixed with `<slug>:` before publishing to the bus. This is enforced by the host, not the plugin, so external plugins cannot bypass it.
8. **Migration SQL validation** — `plugin.ValidateExternalMigrationSQL` checks each SQL statement against a whitelist of DDL patterns namespaced to the plugin slug. Allowed patterns: `CREATE TABLE IF NOT EXISTS <slug>_*`, `CREATE [UNIQUE] INDEX IF NOT EXISTS <slug>_*`, `ALTER TABLE <slug>_*`, `DROP TABLE IF EXISTS <slug>_*`, `DROP INDEX IF EXISTS <slug>_*`. SQL comments are stripped before validation. Any non-matching statement causes the plugin to be rejected.

## Behavior

### Init Handshake (Two-Phase)

1. Start subprocess
2. Send `init` message with `db_path`, `width`, `height` — **no config included**
3. Wait up to 5s for `ready` response
4. Parse `ready`: extract slug, tab_name, routes, key_bindings, migrations, refresh_interval_ms, config_scopes
5. Scope the host config to only the sections listed in `config_scopes` (matched against YAML struct tags). If `config_scopes` is empty, use an empty map (secure by default)
6. Send `config` message with the scoped config JSON
7. Validate migration SQL: each statement must be namespaced DDL (tables/indexes prefixed with `<slug>_`). Reject plugin if any statement fails validation
8. Run `plugin.RunMigrations` for any declared migrations
9. Clear error state

### Error State

When the subprocess crashes or fails to respond:
- `errState` is set to a description of the failure
- `View()` returns an error panel: plugin name, error message, "press r to restart"
- `HandleKey()` only responds to "r" (restart), returns noop for everything else
- `HandleMessage()` skips async drain

### Loader

`LoadExternalPlugins(cfg, ctx)` iterates `cfg.ExternalPlugins`, skips disabled/empty entries, initializes each. Failures are logged but the plugin is **kept in the list** with its error state set — this ensures the error view ("press r to restart") is visible to the user rather than the plugin silently disappearing. If the slug wasn't set during init (process never responded), the plugin's configured name is used as the slug.

After successful init, the loader validates the plugin's slug:

- **Reserved slug check** — If the slug matches a built-in plugin name (`sessions`, `commandcenter`, `settings`), the plugin is shut down and excluded from the list entirely.
- **Uniqueness check** — If the slug is already in use by a previously loaded external plugin, the duplicate is shut down and excluded.

Both rejections are logged at error level. Rejected plugins are not kept in the list (unlike init failures, which are kept for the error view).

The error view includes the command that failed. For exit status 127, a hint is shown that the command was not found on PATH.

## Test Cases

- Two-phase init: init sent without config, ready parsed for config_scopes, config sent with scoped sections
- Config scoping: plugin requesting ["github"] only receives the github config section
- Config scoping: plugin with no config_scopes receives empty config
- Render: plugin responds with view content
- HandleKey: plugin responds with action, correctly converted
- Crash detection: process exits, error view shown
- Restart: press "r" in error state restarts process
- Shutdown: sends shutdown message, process exits cleanly
- Async events: events from plugin are published to bus with slug-prefixed topics
- Slug validation: reserved slug ("sessions") causes plugin rejection
- Slug validation: duplicate slug causes second plugin rejection
- Migration validation: CREATE TABLE with slug prefix passes
- Migration validation: CREATE TABLE without slug prefix is rejected
- Migration validation: INSERT/UPDATE/DELETE statements are rejected
