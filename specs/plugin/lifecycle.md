# SPEC: Lifecycle Messages

## Purpose

Typed lifecycle messages replace implicit tab-switch handling. Instead of plugins
guessing when they become visible or when the user returns from a Claude session,
the host sends explicit messages that plugins can handle in `HandleMessage`.

This enables smarter data loading: plugins reload from DB only when they become
visible (and data is stale), rather than polling on a timer.

## Types

### TabViewMsg

```go
type TabViewMsg struct{ Route string }
```

Sent to a plugin when its tab becomes the active tab. The `Route` field contains
the tab's route slug (e.g., `"commandcenter"`, `"commandcenter/threads"`).

- **Sent by:** Host, during `activateTab`
- **Received by:** The plugin that owns the newly-active tab
- **Use case:** Reload data from DB if stale

### TabLeaveMsg

```go
type TabLeaveMsg struct{ Route string }
```

Sent to a plugin when its tab is being deactivated (user is switching away).
The `Route` field contains the route being left.

- **Sent by:** Host, during `activateTab`
- **Received by:** The plugin that owns the previously-active tab
- **Use case:** Cancel in-progress operations, save ephemeral state

### LaunchMsg

```go
type LaunchMsg struct{ Dir string; ResumeID string }
```

Broadcast to all plugins just before the TUI quits to launch a Claude session.

- **Sent by:** Host, when processing a "launch" action
- **Received by:** All plugins (broadcast)
- **Use case:** Save state before the TUI exits

### ReturnMsg

```go
type ReturnMsg struct{}
```

Broadcast to all plugins when the TUI starts up after returning from a Claude
session.

- **Sent by:** Host, during `Init()` when `returnedFromLaunch` is true
- **Received by:** All plugins (broadcast)
- **Use case:** Reload data that may have changed during the Claude session

## Behavior

### Tab Switching

Given the user presses Tab (or Shift+Tab, or a navigate action switches tabs):

1. Host determines the previous tab and the new tab
2. Host sends `TabLeaveMsg{Route: prevRoute}` to the previous plugin
3. Host calls `NavigateTo(newRoute, nil)` on the new plugin
4. Host sends `TabViewMsg{Route: newRoute}` to the new plugin
5. Host collects any `tea.Cmd` returned from steps 2 and 4

### Launch Flow

Given the user triggers a "launch" action:

1. Host broadcasts `LaunchMsg{Dir, ResumeID}` to all plugins
2. Host sets `Launch` field and quits

### Return Flow

Given the TUI restarts after a Claude session:

1. `main.go` creates the Model with `returnedFromLaunch = true`
2. `Init()` broadcasts `ReturnMsg{}` to all plugins
3. Plugins that care (e.g., command center) reload from DB

## Integration with Command Center Plugin

The command center plugin handles lifecycle messages to replace its 60-second
tick-based DB reload:

- **TabViewMsg:** If `time.Since(ccLastRead) > 2s`, reload from DB
- **ReturnMsg:** Always reload from DB
- **LaunchMsg:** No special handling needed (state is in DB)

The 5-minute `ccRefreshInterval` for spawning `ccc-refresh` is preserved as-is
in the tick handler.

## Test Cases

- TabViewMsg sent to new plugin on tab switch
- TabLeaveMsg sent to previous plugin on tab switch
- NavigateTo called between Leave and View
- LaunchMsg broadcast before quit on launch action
- ReturnMsg broadcast on Init when returnedFromLaunch is true
- No ReturnMsg on normal Init (first launch)
- Command center reloads on TabViewMsg when stale (>2s)
- Command center does not reload on TabViewMsg when fresh (<2s)
- Command center reloads on ReturnMsg
- 60s tick-based reload removed from command center
- 5m refresh interval preserved
