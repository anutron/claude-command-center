# SPEC: TUI Onboarding Flow

## Purpose

Replace the CLI wizard (`ccc setup`) with a TUI onboarding flow that triggers on first run. Guides users through naming their dashboard, picking a color palette, and configuring data sources — all with live previews inside the TUI. Eliminates the separate setup step and provides a polished first-run experience.

## Interface

- **Inputs**: Config file existence (first-run detection), `ccc setup` flag (re-run), user keyboard input
- **Outputs**: Saved `config.yaml` with name, palette, and data source settings; built MCP servers; transition to normal TUI
- **Dependencies**: `config` package, `bubbletea`, `textinput` component, `spinner` component, calendar/github/granola credential validation

## Detection

### First-Run Detection

- On TUI startup, check if `config.ConfigPath()` exists
- If missing: enter onboarding mode automatically
- If present: skip onboarding, load config normally

### Manual Re-Run

- `ccc setup` flag sets `onboarding=true` in the Model regardless of config existence
- Allows users to re-run onboarding to change name, palette, or data sources

## Behavior

### Model State

The onboarding flow is a mode within the main TUI Model:

- `onboarding bool` — when true, the host renders the onboarding view instead of tabs
- `onboardingStep int` — tracks current step (0–3)
- `onboardingModel OnboardingModel` — holds all onboarding-specific state

### Step 0: Welcome + Name

**Display:**
- Welcome message explaining what CCC does
- Text input for dashboard name (pre-filled with `DefaultConfig().Name`)
- Live banner preview above the input — updates on every keystroke using the current name value

**Keys:**
- Type to edit name
- `enter` advances to Step 1

### Step 1: Palette Selection

**Display:**
- List of all 5 built-in palettes (aurora, ocean, ember, neon, mono)
- Cursor highlights the current selection
- Live gradient preview rendered using the selected palette's GradStart/GradMid/GradEnd colors
- The banner preview from Step 0 also updates to reflect the selected palette

**Keys:**
- `up`/`down` to cycle through palettes
- `enter` selects palette and advances to Step 2
- `esc` goes back to Step 0

### Step 2: Data Sources Hub

**Display:**
- List of configurable data sources: Calendar, GitHub, Granola
- Each row shows: name, enable/disable status, credential status (auto-detected)
- Credential status indicators: `✓ credentials found`, `✗ not configured`, `? checking...`

**Auto-Detection on Enter:**
- When Step 2 loads, spawn background checks for each source's credentials:
  - Calendar: check for Google OAuth token file via `auth.GoogleTokenFile()`
  - GitHub: check for `GITHUB_TOKEN` in env (loaded via `auth.LoadEnvFile()`)
  - Granola: check for Granola cookie file existence
- Update status indicators as checks complete

**Keys:**
- `up`/`down` to navigate sources
- `space` toggles enable/disable on selected source
- `enter` opens the sub-flow for the selected source
- `tab` or `n` advances to Step 3 (done)
- `esc` goes back to Step 1

### Sub-Flow: Calendar

**Display:**
- Credential status (valid/missing/expired)
- If credentials valid: list of available Google calendars fetched from API
- Each calendar shows: name, ID, selected/unselected status
- If credentials missing: instructions for setting up Google OAuth

**Keys:**
- `up`/`down` to navigate calendars
- `space` to toggle calendar selection
- `a` to add a calendar ID manually
- `x` to remove selected calendar
- `enter` to confirm and return to hub
- `esc` to cancel and return to hub

**Fetch Behavior:**
- On entering sub-flow with valid credentials, show spinner while fetching calendar list
- On fetch failure, show error and fall back to manual ID entry

### Sub-Flow: GitHub

**Display:**
- Credential status (token found/missing)
- Username field (editable)
- Repo list with add/remove

**Keys:**
- `u` to edit username (activates text input)
- `a` to add a repo (activates text input for `owner/repo`)
- `x` to remove selected repo
- `up`/`down` to navigate repos
- `enter` to confirm and return to hub
- `esc` to cancel and return to hub

### Sub-Flow: Granola

**Display:**
- Credential status (cookie found/missing)
- If missing: instructions for locating the Granola cookie
- If found: confirmation message with cookie file path

**Keys:**
- `enter` to confirm and return to hub
- `esc` to cancel and return to hub

### Step 3: Done

**Display:**
- Summary of configured settings (name, palette, enabled sources)
- Spinner while building MCP servers (gmail, things)
- Success/failure status for each MCP build
- "Press enter to start" prompt after builds complete

**MCP Build:**
- Run `make servers` or equivalent build commands for enabled MCP servers
- Show spinner with status text during build
- On failure: show error message but allow proceeding (MCP servers are optional)
- On success: show checkmark next to each built server

**Keys:**
- `enter` saves config and transitions to normal TUI (only enabled after builds complete or skipped)

### Transition to Normal TUI

When Step 3 completes:

1. Save config to `config.ConfigPath()` via `config.Save()`
2. Set `onboarding = false` in the Model
3. Fire deferred plugin init commands (initial data refresh, plugin registration)
4. Host renders normal tab view

## Navigation Summary

| Step | Enter | Esc | Other |
|------|-------|-----|-------|
| 0: Welcome + Name | Go to Step 1 | Quit TUI | Type to edit name |
| 1: Palette | Go to Step 2 | Back to Step 0 | up/down to cycle |
| 2: Data Sources Hub | Open sub-flow | Back to Step 1 | space toggle, tab/n to Step 3 |
| 2a: Sub-flow | Confirm, return to hub | Cancel, return to hub | Source-specific keys |
| 3: Done | Save + start TUI | Back to Step 2 | Wait for builds |

## Test Cases

### First-Run Detection
- Missing config file triggers onboarding mode
- Existing config file skips onboarding
- `ccc setup` flag triggers onboarding even with existing config

### Step 0: Name Input
- Typing updates the live banner preview
- Empty name falls back to default ("Command Center")
- Enter advances to Step 1 with name stored

### Step 1: Palette Selection
- All 5 palettes are listed and selectable
- Gradient preview updates when cycling palettes
- Banner preview reflects selected palette colors
- Esc returns to Step 0 with previous name preserved

### Step 2: Data Sources
- Auto-detection finds existing credentials
- Auto-detection handles missing credentials gracefully
- Space toggles source enabled/disabled
- Enter opens correct sub-flow for each source
- Tab/n advances to Step 3

### Sub-Flows
- Calendar: fetches calendar list with valid credentials
- Calendar: shows error and manual entry on fetch failure
- GitHub: validates `owner/repo` format on add
- Granola: detects cookie file presence
- Esc from sub-flow preserves hub state

### Step 3: Completion
- Config is saved with all settings from Steps 0–2
- MCP build failure shows error but allows proceeding
- MCP build success shows checkmarks
- Enter transitions to normal TUI with onboarding disabled

### Transition
- Onboarding flag is set to false after completion
- Deferred plugin init commands fire after transition
- Normal tab view renders correctly after onboarding
