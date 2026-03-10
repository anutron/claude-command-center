# SPEC: TUI Onboarding Flow

## Purpose

Replace the CLI wizard (`ccc setup`) with a TUI onboarding flow that triggers on first run. Guides users through naming their dashboard, picking a color palette, and configuring data sources — all with live previews inside the TUI. Eliminates the separate setup step and provides a polished first-run experience.

## Interface

- **Inputs**: Config file existence (first-run detection), `ccc setup` flag (re-run), user keyboard input
- **Outputs**: Saved `config.yaml` with name, palette, show_banner, and data source settings; built MCP servers; transition to normal TUI
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

### Step 0: Welcome + Banner Subtitle

**Display:**
- Welcome message
- Explanation that the text input sets the banner subtitle (the spaced-out text below the CCC logo)
- Text input for banner subtitle (pre-filled with `DefaultConfig().Name`)
- Live banner preview above the input — subtitle updates on every keystroke
- Banner visibility toggle showing current state (`[on]`/`[off]`)

**Keys:**
- Type to edit subtitle text
- `ctrl+b` toggles banner visibility (`cfg.ShowBanner`)
- `enter` advances to Step 1
- `esc` quits

**Banner Visibility:**
- `Config.ShowBanner` is a `*bool` (nil defaults to true for backwards compat)
- `Config.BannerVisible()` returns the effective value
- When `ShowBanner` is false, `Model.View()` skips rendering the gradient banner entirely (both during onboarding and normal mode)
- The toggle preview is live — toggling immediately hides/shows the banner above the onboarding panel

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
- `enter` opens the sub-flow for the selected source (auto-fetches GitHub username if entering GitHub)
- `tab` or `n` advances to Step 3 (done)
- `esc` goes back to Step 1

### Sub-Flow: Calendar

**Display:**
- Credential status (valid/missing/expired)
- If credentials valid: list of configured calendars with add/remove/edit/fetch
- If credentials missing: instructions for setting up Google OAuth

**Keys:**
- `up`/`down` to navigate calendars
- `a` to add a calendar ID manually (two-phase: ID then label)
- `x` to remove selected calendar
- `e` to edit selected calendar's label
- `f` to fetch calendars from Google (enters selection mode)
- `r` to re-check credentials
- `esc` to return to hub

**Fetch with Select-to-Add:**
- Pressing `f` fetches all calendars from Google via `calendar.ListAvailableCalendars()`
- Calendars already in config are filtered out
- Results enter a selection view with checkboxes (`[x]`/`[ ]`)
- Primary calendar is auto-checked, others unchecked by default
- `space` toggles selection, `up`/`down` navigates
- `enter` adds selected calendars to config and returns to normal calendar view
- `esc` cancels selection and returns without adding

### Sub-Flow: GitHub

**Display:**
- Auth status (`gh CLI authenticated` / `gh CLI not authenticated`)
- Username field (auto-populated or editable)
- Repo list with add/remove

**Auto-Fetch Username:**
- When entering the GitHub sub-flow or pressing `r` to re-check, if auth is valid and username is empty, automatically run `gh api user -q .login` via a background `tea.Cmd`
- Result arrives as `githubUsernameMsg` and populates both `cfg.GitHub.Username` and the text input

**Keys:**
- `u` to edit username (activates text input)
- `a` to add a repo (activates text input for `owner/repo`)
- `x` to remove selected repo
- `up`/`down` to navigate repos
- `r` to re-check auth (also triggers username auto-fetch)
- `esc` to return to hub

### Sub-Flow: Granola

**Display:**
- Brief description: "Granola records and summarizes your meetings."
- Credential status (stored-accounts.json found/missing)
- If missing: step-by-step setup instructions:
  1. Install from granola.ai
  2. Open Granola and sign in
  3. CCC reads Granola's local data automatically
  4. Shows the specific path checked: `~/Library/Application Support/Granola/stored-accounts.json`
- If found: confirmation message

**Keys:**
- `r` to re-check credentials
- `esc` to return to hub

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
| 0: Welcome + Subtitle | Go to Step 1 | Quit TUI | Type to edit subtitle, ctrl+b toggle banner |
| 1: Palette | Go to Step 2 | Back to Step 0 | up/down to cycle |
| 2: Data Sources Hub | Open sub-flow | Back to Step 1 | space toggle, tab/n to Step 3 |
| 2a: Calendar sub-flow | Confirm (or add selected) | Cancel/back | a/x/e/f/r, selection mode keys |
| 2b: GitHub sub-flow | Confirm | Back to hub | a/x/u/r, auto-fetch username |
| 2c: Granola sub-flow | - | Back to hub | r re-check |
| 3: Done | Save + start TUI | Back to Step 2 | Wait for builds |

## Test Cases

### First-Run Detection
- Missing config file triggers onboarding mode
- Existing config file skips onboarding
- `ccc setup` flag triggers onboarding even with existing config

### Step 0: Banner Subtitle + Visibility
- Typing updates the live banner subtitle preview
- Empty name falls back to default ("Command Center")
- Enter advances to Step 1 with name stored
- `ctrl+b` toggles banner visibility in real-time
- Banner hidden when `ShowBanner` is false
- `ShowBanner` defaults to true when nil (backwards compat)

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
- Calendar: fetch enters selection mode with checkboxes
- Calendar: primary calendar auto-checked in selection
- Calendar: enter in selection mode adds only checked calendars
- Calendar: esc in selection mode discards fetched results
- Calendar: already-configured calendars filtered from fetch results
- Calendar: shows error and manual entry on fetch failure
- GitHub: auto-fetches username when entering sub-flow with valid auth
- GitHub: auto-fetches username on `r` re-check with valid auth and empty username
- GitHub: does not auto-fetch if username already set
- GitHub: validates `owner/repo` format on add
- Granola: shows setup instructions with path when not configured
- Granola: detects stored-accounts.json presence
- Esc from sub-flow preserves hub state

### Step 3: Completion
- Config is saved with all settings from Steps 0-2
- MCP build failure shows error but allows proceeding
- MCP build success shows checkmarks
- Enter transitions to normal TUI with onboarding disabled

### Transition
- Onboarding flag is set to false after completion
- Deferred plugin init commands fire after transition
- Normal tab view renders correctly after onboarding
