# Settings Plugin

**Package:** `internal/builtin/settings`
**Slug:** `settings`
**Tab:** "Settings"

## Purpose

Provides a UI for managing appearance, plugins, data sources, system actions, and logs. Uses a sidebar + content pane layout with huh forms for all interactive content.

## Layout

Sidebar (left) + content pane (right). The sidebar lists items grouped by category. The content pane shows a huh form (or logs viewer) for the selected item.

### Sidebar Categories

1. **APPEARANCE** — Banner, Palette
2. **PLUGINS** — Sessions, Command Center, PRs, Pomodoro, external plugins
3. **DATA SOURCES** — Calendar, GitHub, Granola, Slack, Gmail
4. **AGENT** — Daemon, Budget, Sandbox
5. **SYSTEM** — Automations, Schedule, MCP Servers, Skills, Shell Integration, Logs

### NavItem

Each sidebar entry is a `NavItem` with:
- `Label` — display name
- `Slug` — unique identifier
- `Kind` — "appearance", "plugin", "datasource", "agent", "system"
- `Description` — one-liner shown below the title in the content pane header
- `Enabled` — toggle state (nil = no toggle)
- `ValidationStatus` / `ValidationMsg` / `ValidHint` — tiered credential validation (data sources)
- `SyncStatus` — last sync time/error from database

### Sidebar Scrolling

When the terminal is short and sidebar items exceed the panel height, the sidebar scrolls to follow the cursor. A `navScrollOffset` tracks the scroll position. Moving the cursor past the visible area adjusts the offset to keep it in view.

### Content Pane Header

Every content pane renders a header: the item label in uppercase (e.g., "BANNER", "CALENDAR") with a description line in muted text below it. This header is rendered centrally in `viewContent`, not inside individual forms.

### Content Pane Preview

When the sidebar is focused (FocusNav), the content pane shows a read-only preview of the highlighted item's form. The preview builds a transient form via `buildFormForSlug`, calls `Init()` on it for proper field rendering, but does not store it as the active form. Moving the sidebar cursor updates the preview immediately.

## Focus Zones

Three focus zones control key routing:

| Zone | Purpose | Enter | Exit |
|------|---------|-------|------|
| FocusNav | Sidebar navigation | esc/left from form or logs | enter/right opens form |
| FocusForm | Active huh form in content pane | enter/right from nav | esc/left returns to nav |
| FocusLogs | Custom scrollable log viewer | enter/right on Logs item | esc/left returns to nav |

### Navigation Between Zones

- **Right arrow / enter / l** from FocusNav: builds a huh form for the selected item, sets FocusForm (or FocusLogs for logs)
- **Left arrow** from FocusForm: returns to FocusNav (auto-saves banner/palette). Does NOT intercept left arrow in text input fields (cursor movement).
- **Esc** from FocusForm: returns to FocusNav (auto-saves banner/palette). Auth forms cancel on esc.
- **Esc** from FocusLogs: returns to FocusNav

## Form-Based Content Panes

All content panes (except logs) use `charmbracelet/huh` forms. Each pane has:
- A `build*Form()` function that creates a huh.Form from current config values
- A `handle*FormCompletion()` function that reads bound values, saves config, and rebuilds the form
- Bound value structs (e.g., `bannerFormValues`, `paletteFormValues`) that huh updates in real-time

### Form Lifecycle

1. User presses enter/right on a nav item
2. `buildFormForSlug(item)` creates a form and calls `Init()`
3. Form stored as `p.activeForm` with `p.activeFormSlug`
4. User interacts (tab between fields, enter to submit, type in inputs)
5. On form completion (StateCompleted): `handleFormCompletion(slug)` saves values, rebuilds form
6. On exit (esc/left): auto-save for editable forms (banner, palette), then clear form

### Auto-Save Behavior

Forms with editable settings (banner, palette, agent-budget) auto-save on:
- **Pane exit** (esc, left arrow, tab leave)
- **Field transition** (tab, shift+tab, enter moving between fields)

The daemon form auto-saves by executing the selected action on field transition.

Action-based forms (system, datasource) do NOT auto-save — esc dismisses without executing.

### Huh Theme

A custom huh theme (`huhThemeFromPalette`) maps the CCC palette colors to huh's theme system:
- Titles/cursor: palette cyan
- Selected options: palette green
- Errors: red
- Select selector: `> ` prefix
- Selected/unselected prefixes: `[*] ` / `[ ] `

The theme rebuilds when the palette changes.

## Pane Details

### Banner (appearance)

Form fields:
- **Name** — text input, 20 char limit
- **Subtitle** — text input, 30 char limit
- **Show Banner** — confirm (yes/no)
- **Top Padding** — text input, validated 0-10

Saves to config on field transition and pane exit. Publishes `config.saved` event.

### Palette (appearance)

Form fields:
- **Color Palette** — select with palette names, "(active)" suffix on current
- **Preview** — note with live color swatches via `DescriptionFunc`

On completion: applies palette, rebuilds all styles (settings, shared, gradient), publishes `palette.changed` event.

### Data Sources (calendar, github, granola, slack, gmail)

Content layout (top to bottom):
1. Header (title + description)
2. Provider view (if provider exists — rendered outside the form for interactivity)
3. Validation status note (inside form)
4. Action select (inside form)

**Action options** (contextual):
- All sources: "Verify credentials" (live API check)
- Google sources: "Authenticate (enter client credentials + OAuth)", "Open Google Cloud Console"
- Slack: "Enter Slack token"

**Provider interactivity**: Data source providers (Calendar, GitHub, Granola) implement `SettingsProvider`. Their views are rendered above the form, and their `HandleSettingsKey` receives keys before the form. This preserves interactive features like calendar list toggles, GitHub repo selection, and color pickers.

**Async message routing**: Provider async results (CalendarFetchResultMsg, ghRepoFetchResult) are routed to providers BEFORE the form's Update, so fetch results arrive even when a form is active.

**Credential verification**: "Verify credentials" always does a live API check. For Slack, calls `auth.test`. Results respect the `Live` flag — a live "ok" skips the sync-aware downgrade that would otherwise show stale DB errors.

**After credential save**: The datasource form is rebuilt so the pane stays fully populated (not just title/subtitle).

### Slack Token

The Slack integration uses a **user token** (`xoxp-`), not a bot token. The token form says "Slack User Token" with "Starts with xoxp-". Config supports both `token` (preferred) and `bot_token` (backwards compat) fields. Env vars: `SLACK_TOKEN` and `SLACK_BOT_TOKEN`.

The Slack refresh source gracefully degrades: if `conversations.list` fails with `missing_scope`, it falls back to `search.messages` (which only requires `search:read`).

### Daemon (agent-daemon)

Shows the daemon process status and provides lifecycle controls. Queries the daemon via `daemonClientFunc` on each form build.

Form layout (single group, plus optional Live Info group):
- **Status** — note showing "Running" (green), "Paused" (yellow), or "Stopped" (red)
- **Action** — select with contextual options:
  - Running: "Pause", "Stop"
  - Paused: "Resume", "Stop"
  - Stopped: "Start"
- **Live Info** (shown only when daemon is reachable) — read-only note showing active agent count, hourly spend vs limit, daily spend vs limit, and emergency stop status if active

On completion: executes the selected action (`daemon.StartProcess()` for start, client RPC for pause/resume/stop), waits 300ms for the daemon to transition, then rebuilds the form to reflect the new state. Flash message confirms the action result.

Auto-save on field transition calls `saveDaemonValues()` which applies the action immediately (same as form completion).

### Budget (agent-budget)

Editable budget configuration with live spend display and read-only rate limits. Three form groups:

**Group 1 — Current Spend** (read-only, from daemon):
- **Current Spend** — note showing hourly spend/limit, daily spend/limit, active agent count. Falls back to muted "daemon not connected" / "daemon not available" messages when unreachable.
- **Emergency Stop** — note showing "ACTIVE — all agents stopped" (red) or "off" (green). Shows "N/A" when daemon is unreachable.

**Group 2 — Editable Fields:**
- **Max Concurrent Agents** — text input, 4 char limit, validated 1-100
- **Hourly Budget ($)** — text input, 8 char limit, validated non-negative float
- **Daily Budget ($)** — text input, 8 char limit, validated non-negative float
- **Warning Threshold (%)** — text input, 3 char limit, validated 0-100 integer

**Group 3 — Rate Limits** (read-only):
- **Rate Limits** — note showing max launches per automation per hour, budget cooldown minutes, failure backoff initial/max seconds. Values come from `config.Agent` fields.

Saves to `config.Agent.*` on field transition (auto-save via `saveBudgetValues()`) and on form completion. Publishes `config.saved` event with source `"agent-budget"`. Warning threshold is stored as a 0-1 float internally but displayed/edited as a 0-100 percentage.

### System Panes (schedule, mcp, skills, shell)

Each system pane has:
- **Status note** — current install/build status
- **ACTIONS select** — Install/Uninstall (or Build & Configure for MCP)

Actions execute immediately on form completion (no confirmation). The form rebuilds after the async action completes to show updated status.

### Plugins

Plugin panes show info via a huh Note. Plugins implementing `SettingsProvider` get their provider view rendered above the form.

### Logs

The **only** non-form pane. Uses FocusLogs zone with custom scrollable view:
- `j/k` — scroll line by line
- `f/b` — page forward/back
- `d/u` — half-page down/up
- `/` — enter filter mode
- `esc` — clear filter or return to nav

## Sidebar Toggle Behavior

- `space` toggles enable/disable on the selected item in FocusNav
- Built-in plugins: saves to `config.DisabledPlugins`, flashes "Restart CCC to apply"
- External plugins: saves config, flashes "Restart CCC to apply"
- Data sources: validates credentials first; reverts on failure

Enabled states sync from live config at the start of each `View()` call.

## Quit Behavior

Double-escape to quit (applies to all tabs, not just settings):
1. First esc at top level: shows "Press esc again to quit" flash, starts 2-second timer
2. Second esc within 2 seconds: quits CCC
3. Any other key or timer expiry: cancels pending quit

## Key Bindings

| Key | Context | Description |
|-----|---------|-------------|
| up/down | FocusNav | Navigate sidebar |
| space | FocusNav | Toggle enable/disable |
| enter/right/l | FocusNav | Open content pane (FocusForm or FocusLogs) |
| left/esc | FocusForm | Return to sidebar (auto-saves banner/palette) |
| tab | FocusForm | Next field (auto-saves banner/palette) |
| enter | FocusForm | Submit/advance field |
| left/esc | FocusLogs | Return to sidebar |
| j/k/f/b/d/u | FocusLogs | Scroll logs |
| / | FocusLogs | Enter filter mode |
| esc ×2 | Top level | Quit CCC |

## Constructor

```go
settings.New(registry *plugin.Registry) *Plugin
```

## Dependencies

- `plugin.Registry` — for listing all plugins
- `plugin.Logger` — for log viewer
- `plugin.EventBus` — for palette change and config saved events
- `config.Config` — for reading/writing all settings
- `charmbracelet/huh` — form framework for all content panes
- `charmbracelet/lipgloss` — styling
- `plugin.SettingsProvider` — delegated views for data sources and plugins
- `daemon` — daemon process lifecycle (start) and client RPC (pause, resume, stop, status, budget)
