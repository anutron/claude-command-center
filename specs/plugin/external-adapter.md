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
| `Init(ctx)` | Start subprocess, send `init`, wait for `ready` (5s), cache metadata, run migrations |
| `View(w,h,f)` | Send `render`, receive `view` (50ms timeout), return content or cached fallback |
| `HandleKey(msg)` | If crashed + "r" → restart. Otherwise send `key`, receive `action` (50ms timeout) |
| `HandleMessage(msg)` | Update dimensions on `WindowSizeMsg`. On any msg: drain async channel (events→bus, logs→logger) |
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

## Behavior

### Init Handshake

1. Start subprocess
2. Marshal config as JSON, send `init` message with config, db_path, width, height
3. Wait up to 5s for `ready` response
4. Parse `ready`: extract slug, tab_name, routes, key_bindings, migrations, refresh_interval_ms
5. Run `plugin.RunMigrations` for any declared migrations
6. Clear error state

### Error State

When the subprocess crashes or fails to respond:
- `errState` is set to a description of the failure
- `View()` returns an error panel: plugin name, error message, "press r to restart"
- `HandleKey()` only responds to "r" (restart), returns noop for everything else
- `HandleMessage()` skips async drain

### Loader

`LoadExternalPlugins(cfg, ctx)` iterates `cfg.ExternalPlugins`, skips disabled/empty entries, initializes each. Failures are logged and skipped — one bad plugin doesn't block the rest.

## Test Cases

- Init handshake: plugin responds with ready, metadata cached correctly
- Render: plugin responds with view content
- HandleKey: plugin responds with action, correctly converted
- Crash detection: process exits, error view shown
- Restart: press "r" in error state restarts process
- Shutdown: sends shutdown message, process exits cleanly
- Async events: events from plugin are published to bus
