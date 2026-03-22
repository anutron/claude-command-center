# Design: Google OAuth Credential Diagnosis & Resolution UX

## Problem

When Google OAuth credentials break (deleted client, revoked token, missing config), CCC gives no useful feedback:

- `validate.go` only checks if credential files exist and parse as JSON — settings shows a green checkmark even when the client_id is dead
- Refresh failures show raw error messages in the warning banner with no actionable guidance
- `ccc doctor` does the same shallow file-exists check
- There's no way to re-authenticate or fix credentials from within CCC
- The external google-calendar MCP server (not CCC) opens the browser on stale credentials, creating confusion

## Design

### 1. Structural Validation

Replace shallow file-exists checks in `validate.go` with field-level inspection. Return a structured result instead of just error/nil.

**New type:**

```go
type ValidationResult struct {
    Status  string // "ok", "missing", "incomplete", "no_client"
    Message string // human-readable explanation
    Hint    string // actionable fix instruction
}
```

**Calendar checks** (`ValidateCalendar() ValidationResult`):

Calendar has two file formats — `credentials.json` (new, contains client_id + client_secret + tokens) and `token.json` (legacy, tokens only). The validation checks whichever file is found:

1. Check `~/.config/google-calendar-mcp/credentials.json` exists
   - If found: check for `refresh_token` → if missing: Status `incomplete`
   - If found: check for `client_id` in file → if present: Status `ok`
   - If found but no `client_id`: fall through to client credential resolution (step 3)
2. If `credentials.json` missing, check `token.json`
   - If found: check for `refresh_token` → if missing: Status `incomplete`
   - Fall through to client credential resolution (step 3)
3. If neither file exists: Status `missing`, Hint "Press 'a' to set up"
4. Client credential resolution (when file exists but no embedded client_id):
   - Check `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` env vars
   - Check `~/.claude.json` → `mcpServers.google-calendar.env`
   - If found in any source: Status `ok`
   - If not found anywhere: Status `no_client`, Hint "Press 'a' to configure OAuth client"

**Gmail checks** (`ValidateGmail() ValidationResult`):

Gmail has a different credential chain than Calendar — no `~/.claude.json` fallback:

1. Check `~/.gmail-mcp/work.json` exists → if not: Status `missing`, Hint "Press 'a' to set up"
2. Check for `refresh_token` in `tokens` object → if missing: Status `incomplete`
3. Check for `clientId` in file → if present: Status `ok`
4. Check `GMAIL_CLIENT_ID` / `GMAIL_CLIENT_SECRET` env vars → if set: Status `ok`
5. If no client credentials anywhere: Status `no_client`, Hint "Press 'a' to configure OAuth client"

**Other data sources:**

- **Todos**: No credentials needed, always valid. Not shown in validation.
- **GitHub**: Keep current `gh auth token` check, return `ValidationResult`
- **Slack**: Keep current `SLACK_BOT_TOKEN` env var check, return `ValidationResult`
- **Granola**: Keep current file-exists check, return `ValidationResult`

The settings sidebar checkmark/X now reflects structural validity. The content pane shows the full `Message` and `Hint`.

### 2. Live Token Check (On-Demand in Settings)

When the user opens the settings content pane for a Google data source (calendar or gmail), fire an async `tea.Cmd` that verifies the token actually works.

**Flow:**

1. User navigates to Calendar or Gmail in settings → content pane renders immediately with structural validation result
2. Background `tea.Cmd` loads the token and calls `https://oauth2.googleapis.com/tokeninfo?access_token=...`
3. Content pane shows "Checking..." indicator next to credentials status
4. Result arrives as a `tea.Msg`, updates the content pane:
   - 200 OK → "Credentials valid" (green)
   - 401 → try refreshing token using refresh_token + client credentials
     - Refresh succeeds → "Credentials valid (token refreshed)"
     - Refresh fails with `invalid_client` → "OAuth client no longer exists — press 'a' to re-authenticate"
   - No client_id available → skip live check (structural validation already caught this)
   - Network error → "Could not verify (offline?)" — inconclusive, not marked broken

**Caching:** Result stored on the settings `Plugin` struct in a `map[string]*ValidationResult` keyed by data source slug. Navigating away and back doesn't re-check. User can force re-check with `r` (clears the cache entry and fires a new `tea.Cmd`).

**SettingsProvider interface extension:**

The current `SettingsProvider` interface cannot fire async `tea.Cmd`s or handle `tea.Msg` results. Extend it:

```go
type SettingsProvider interface {
    SettingsView(width, height int) string
    HandleSettingsKey(msg tea.KeyMsg) Action
    // New: return a tea.Cmd to fire when this provider's pane is opened.
    // Return nil if no async work needed.
    SettingsOpenCmd() tea.Cmd
    // New: handle async results. Return true if the message was consumed.
    HandleSettingsMsg(msg tea.Msg) (bool, Action)
}
```

The settings plugin calls `SettingsOpenCmd()` when the user navigates to a data source's content pane, and routes `tea.Msg`s to the active provider via `HandleSettingsMsg`. Providers that don't need async (GitHub, Granola) return nil/false.

For data sources without a `SettingsProvider` (Calendar and Gmail currently don't implement one from a plugin), the settings plugin itself owns the live check logic — it fires the `tea.Cmd` directly and handles the result in its own `HandleMessage`.

### 3. In-TUI Re-authentication & Credential Setup

When the user presses `a` on a data source with broken credentials, the content pane enters an auth flow. The flow adapts to what's broken:

**Scenario 1 — Token expired/revoked, client credentials valid:**

- Open browser for OAuth consent, listen for callback on localhost
- Content pane shows spinner: "Waiting for authorization... (esc to cancel)"
- On callback: save tokens to credentials file (with client_id/secret embedded), flash success, refresh validation

**Scenario 2 — No client credentials:**

- Show numbered step-by-step guide inline in the content pane
- Press `o` to open Google Cloud Console in browser
- huh form appears inline with two fields: Client ID, Client Secret
- On submit: write values to `~/.config/ccc/.env` (create/update without clobbering)
- Automatically chain into Scenario 1's OAuth flow

**Scenario 3 — Gmail:**

- Same structure as Calendar but with:
  - Gmail scopes (`gmail.GmailReadonlyScope`, or `gmail.GmailModifyScope` + `gmail.GmailComposeScope` if `config.Gmail.Advanced`)
  - Random port on `127.0.0.1:0`, redirect URI set dynamically
  - Credentials written to `~/.gmail-mcp/work.json`
  - Client credential env vars: `GMAIL_CLIENT_ID` / `GMAIL_CLIENT_SECRET` (not `GOOGLE_*`)

**OAuth callback as a tea.Cmd:**

The existing `RunCalendarAuth` is a blocking function. For the TUI, the OAuth flow is wrapped as a non-blocking `tea.Cmd`:

```go
// authFlowCmd returns a tea.Cmd that runs the OAuth flow in a goroutine.
// It starts an HTTP server, opens the browser, waits for the callback,
// and returns an authFlowResultMsg with the token or error.
func authFlowCmd(conf *oauth2.Config, port int, credsPath string) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        defer cancel()
        // Start HTTP server, open browser, wait for callback or timeout
        // Returns authFlowResultMsg{tok, err}
    }
}
```

The settings plugin handles `authFlowResultMsg` in `HandleMessage`: on success, save credentials, clear live check cache, flash "Connected". On error/timeout/cancel, flash the error and return to content view.

Esc cancellation: the `tea.Cmd` uses a `context.Context`. When the user presses esc during `FocusForm` or while the spinner is showing, the settings plugin cancels the context (stored on the Plugin struct), which causes the HTTP server to shut down and the `tea.Cmd` to return an error.

**OAuth callback mechanics:**

- Both Calendar and Gmail: HTTP server on `127.0.0.1:0` (random available port), redirect URI set dynamically
- Random port avoids "port in use" conflicts with other processes
- 5-minute timeout, cancellable with esc
- On success: credentials file includes client_id + client_secret + tokens (no env fallback needed after this)

### 4. Tiered Fix Instructions

The content pane adapts its guidance based on fix complexity:

**Tier 1 — Press a key** (fixable in-TUI):

```
  Fix:  Press 'a' to re-authenticate
```

**Tier 2 — Run a command** (needs separate terminal):

```
  Fix:  Run 'gh auth login' in another terminal
```

**Tier 3 — Step-by-step guide** (requires browser steps + TUI input):

```
  Problem:  OAuth client no longer exists in Google Cloud

  To fix:
  1. Go to console.cloud.google.com/apis/credentials
     (press 'o' to open in browser)
  2. Create an OAuth 2.0 Client ID (type: Desktop app)
  3. Copy the Client ID and Client Secret
  4. Paste them below:

  > Client ID:     [____________________________]
    Client Secret:  ____________________________

  enter next field · esc cancel
```

Content pane is scrollable (up/down) when displaying longer guides.

### 5. huh Form Integration

**New dependency:** `github.com/charmbracelet/huh`

Used for multi-field credential input forms. Phase 1 adds huh for the new auth forms only. Phase 2 (follow-up) migrates existing textinput usage throughout the app.

**Tab key conflict resolution:** Modal approach — when a huh form is active (`FocusForm` zone), Tab/Shift+Tab navigate between form fields. When no form is active, Tab/Shift+Tab switch host tabs as usual. The plugin handles the key first; if it returns `ActionUnhandled`, the host processes it. No keybinding changes needed.

**New focus zone:**

```go
const (
    FocusNav FocusZone = iota
    FocusContent
    FocusEditing
    FocusForm  // new: huh form active, captures Tab/Shift+Tab
)
```

**Key handling chain for FocusForm:**

In `settings.go HandleKey`, when `focusZone == FocusForm`:

1. All key events are forwarded to the huh form model's `Update(tea.Msg)` method
2. If the form completes (submitted), process the result, switch to `FocusContent`
3. If esc is pressed, cancel the form, switch to `FocusContent`
4. All other keys (including `q`, Tab, Shift+Tab) are consumed by the form — they do NOT propagate to the host

The huh form model is stored on the settings `Plugin` struct. The settings `HandleMessage` method also forwards `tea.Msg`s to the huh form (it needs non-key messages for internal state management like blinking cursors).

**Content pane key bindings (when data source selected):**

| Key | Context | Action |
|-----|---------|--------|
| `a` | Credentials broken | Start auth/setup flow |
| `o` | Guide showing URL | Open URL in browser |
| `r` | Any | Re-run live credential check |
| `esc` | Form active | Cancel form/auth flow |
| `Tab`/`Shift+Tab` | Form active | Next/prev form field |

### 6. Doctor Improvements

`ccc doctor` uses the same `ValidationResult` checks as the settings pane:

- **Structural validation** for Calendar and Gmail (shared functions)
- **Live token check** for Calendar and Gmail (same endpoint, same logic)
- Fix hints printed inline with failures
- Network errors show `[??]` for inconclusive (distinct from `[!!]` for definite failures)

```
  [OK] Config file
  [OK] Database
  [!!] Calendar credentials
       OAuth client not found — open Settings > Calendar and press 'a' to fix
  [OK] GitHub CLI
  [!!] Gmail credentials
       No credentials file — open Settings > Gmail and press 'a' to set up
  [OK] Granola
  [OK] ai-cron binary
  [OK] claude CLI
  [OK] Data freshness (2m ago)

  7/9 checks passed
```

## Architecture

### New/Modified Files

| File | Changes |
|------|---------|
| `internal/plugin/doctor.go` | New. `DoctorProvider` interface, `DoctorCheck`, `DoctorOpts`, `ValidationResult` types. |
| `internal/plugin/plugin.go` | Extend `SettingsProvider` with `SettingsOpenCmd()` and `HandleSettingsMsg()`. |
| `internal/auth/types.go` | New. `GoogleTokenFile`, `ToOAuth2Token()` — extracted from `refresh/auth.go`. |
| `internal/auth/config.go` | New. `LoadGoogleOAuth2Config`, `LoadCalendarCredsFromClaudeConfig`. |
| `internal/auth/env.go` | New. `LoadEnvFile`, `ReadEnv`, `WriteEnvValue` — atomic write to avoid races with `ai-cron`. |
| `internal/auth/flow.go` | New. `AuthFlowCmd` — generic non-blocking OAuth flow as `tea.Cmd`. Random port on `127.0.0.1:0`. |
| `internal/refresh/sources/calendar/doctor.go` | New. `ValidateCalendarResult()`, `DoctorChecks()` with live tokeninfo check. |
| `internal/refresh/sources/gmail/doctor.go` | New. `GmailDoctor`, `ValidateGmailResult()`, `DoctorChecks()` with live tokeninfo check. |
| `internal/refresh/sources/github/settings.go` | Added `DoctorChecks()` wrapping `gh auth token` check. |
| `internal/refresh/sources/granola/settings.go` | Added `DoctorChecks()` wrapping account check. |
| `internal/config/validate.go` | Kept for backward compat — providers are the primary validation source now. |
| `internal/builtin/settings/content_datasources.go` | Tiered validation status, fix instructions, `o`/`a`/`r` key handling. Live check on pane open. |
| `internal/builtin/settings/auth_form.go` | New. huh form for client credential input. |
| `internal/doctor/doctor.go` | Uses `DoctorProvider` loop, `--live` flag, `[??]` for inconclusive. |
| `internal/refresh/auth.go` | Deleted — contents moved to `internal/auth/`. All callers migrated directly. |

### Dependency Graph

```
internal/auth/          ← new package, no CCC dependencies (only oauth2, net/http, os)
  ↑              ↑
internal/config/    internal/refresh/
  ↑
internal/builtin/settings/
  ↑
internal/doctor/
```

`internal/auth/` is a leaf package that both `config` (for validation/live checks) and `refresh` (for token loading during data fetch) can import without circular dependencies.

### Data Flow

```
Settings pane opened
  → structural validate → show status + checkmark/X
  → async tea.Cmd → live token check → update status

User presses 'a':
  Has client creds? ─── yes ──→ OAuth flow (browser + callback)
    │ no                              ↓
    ↓                           Save tokens → flash success
  Step-by-step guide
    → press 'o' to open GCP Console
    → huh form (Client ID, Client Secret)
    → submit → write .env → load into env
    → chain into OAuth flow
    → Save tokens → flash success
```

## Phase 2 (Follow-Up)

Migrate existing `bubbles/textinput` usage to huh forms:

- Onboarding wizard (name, subtitle, calendar, GitHub)
- Settings banner editing
- Calendar settings (add calendar)
- GitHub settings (add repo, edit username)
- Command center text input

Each is an isolated migration. Not part of this design's scope.

## Test Cases

### Structural Validation
- Calendar: missing both files → `missing` status
- Calendar: `credentials.json` exists with all fields → `ok` status
- Calendar: `token.json` exists, client_id in env → `ok` status
- Calendar: `token.json` exists, no client_id anywhere → `no_client` status
- Calendar: file exists but no refresh_token → `incomplete` status
- Gmail: missing file → `missing` status
- Gmail: file exists with clientId and tokens → `ok` status
- Gmail: file exists, no clientId, `GMAIL_CLIENT_ID` set → `ok` status
- Gmail: file exists, no client credentials anywhere → `no_client` status

### Live Check
- Valid token → `ok`
- Expired token, refresh succeeds → `ok` with "refreshed" message
- Expired token, refresh fails with `invalid_client` → specific error about dead OAuth client
- Network error → inconclusive, not marked broken
- No client_id → skip live check, return structural result only

### Auth Flow
- Port in use → clear error message, no hang
- Timeout (5 min) → cancellable, returns to content pane
- Esc during auth → cancels context, shuts down HTTP server, returns to content
- Success → credentials saved with client_id embedded, live check cache cleared
- Client credential form submit → chains into OAuth flow without extra user action

### Integration
- Doctor: uses same ValidationResult as settings, shows consistent output
- Doctor: network error shows `[??]` marker
- huh form: Tab navigates fields, host Tab blocked during FocusForm
- huh form: `q` key does not quit app during form entry
- .env write: creates file if missing, updates existing key without clobbering others
- .env write: atomic (temp file + rename) to avoid partial reads by ai-cron
