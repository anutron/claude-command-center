# CCC Security Audit — 2026-03-14

## High Severity

### H1: Full config (with Slack token) sent to all external plugins

- **File:** `internal/external/external.go:72-77`
- `json.Marshal(ep.ctx.Config)` sends the entire Config struct to every plugin subprocess, including `Slack.Token`, `Slack.BotToken`, GitHub username, and all other plugin commands
- **Fix:** Plugin declares `config_scopes` (e.g., `["slack", "github"]`) in its `ready` response. Host only sends those top-level config sections — keeps plugin-specific knowledge in the plugin, not the core.

### H2: External plugin arbitrary SQL via migrations

- **Files:** `internal/plugin/migrations.go:45`, `internal/external/external.go:132-146`
- `tx.Exec(m.SQL)` runs raw SQL from plugin `ready` response with no restrictions
- A malicious plugin can DROP TABLE, read/modify other plugins' data, ATTACH DATABASE to exfiltrate
- Flagged by 4 of 5 audit agents independently
- **Fix:** Restrict migration SQL to namespaced DDL only (e.g., `CREATE TABLE IF NOT EXISTS <slug>_*`)

### H3: Launch action lets plugins chdir anywhere + write files

- **Files:** `internal/tui/model.go:420-439`, `internal/tui/launch.go:22-58`
- Plugin `action` response with `type: "launch"` can specify arbitrary `dir`, write `task-context.md`, and resume any Claude session
- **Fix:** Constrain launch action `dir` to directories listed in the Sessions learned paths list. Reject any path not in that list.

### H4: OAuth state parameter never validated

- **Files:** `internal/auth/flow.go:104-113`, `internal/refresh/sources/calendar/auth.go:96`
- Hardcoded `"state-token"` — callback ignores state entirely
- Enables CSRF: attacker crafts auth URL that associates their Google account with victim's CCC
- **Fix:** Generate crypto/rand state per flow, store it, verify on callback

### H5: Calendar OAuth binds `:3000` (all interfaces)

- **File:** `internal/refresh/sources/calendar/auth.go:117`
- Any device on the LAN can hit the callback endpoint — widens CSRF attack surface from H4
- The newer `flow.go` correctly binds to `127.0.0.1:0` — this is the legacy calendar auth
- **Fix:** Change to `127.0.0.1:3000` or use random port on loopback like `flow.go`

---

## Medium Severity

### M1: No PKCE in any OAuth flow

- **Files:** `internal/auth/flow.go`, `servers/gmail/src/auth.ts`
- Public/native OAuth clients should use PKCE to prevent authorization code interception
- Google supports PKCE
- **Fix:** Add PKCE code_verifier/code_challenge to all OAuth flows

### M2: Config file + backup written 0o644 (world-readable)

- **File:** `internal/config/config.go:343-353`
- Contains Slack token in plaintext — any local user can read it
- **Fix:** Write config.yaml and .bak with 0o600

### M3: DB + socket directories created 0o755

- **Files:** `internal/db/schema.go:15`, `internal/tui/notify.go:50`
- DB contains calendar events, email subjects, meeting transcripts
- Socket allows other users to send notify events to the TUI
- **Fix:** Use 0o700 for both directories

### M4: Gmail MCP token file written without explicit permissions

- **File:** `servers/gmail/src/auth.ts:67`
- `writeFileSync` defaults to 0o644 via umask — tokens are world-readable
- **Fix:** Pass `{ mode: 0o600 }` to `writeFileSync`

### M5: Terminal escape injection from external API data

- **Files:** `internal/builtin/commandcenter/cc_view.go`, `internal/builtin/sessions/sessions.go`
- Calendar event titles, PR titles, Slack messages rendered without ANSI stripping
- A malicious title could set terminal title or inject OSC sequences
- Lipgloss/bubbletea do NOT strip raw ANSI from input strings
- **Fix:** Strip ANSI escapes at the refresh/ingestion boundary using `ansi.Strip()`

### M6: Plugin slug self-declared — can spoof identity

- **File:** `internal/external/external.go:106-109`
- Plugin declares its own slug in `ready` response — no validation against built-in names or uniqueness
- Can impersonate `sessions`, `settings`, etc. and publish spoofed events
- **Fix:** Validate slug uniqueness, reject reserved built-in slugs

### M7: External plugin events published with no topic validation

- **File:** `internal/external/external.go:257-265`
- Plugins can publish events on any topic — could trigger actions in built-in plugins
- No rate limiting on event publishing
- **Fix:** Auto-prefix all plugin event topics with the plugin's slug (e.g., `pomodoro:timer-done`)

### M8: Unbounded io.ReadAll on API responses

- **Files:** `internal/refresh/sources/slack/slack.go:139`, `granola/granola.go:204`
- Granola also decompresses gzip before ReadAll — gzip bomb risk
- **Fix:** Use `io.LimitReader(resp.Body, 10*1024*1024)` (10MB cap)

### M9: Lock file TOCTOU race

- **File:** `internal/lockfile/lockfile.go:24-36`
- Read, check PID alive, write new PID is not atomic
- Two processes could both claim the lock, causing concurrent refresh
- **Fix:** Use `syscall.Flock()` for advisory file locking

---

## Low Severity

### L1: No resource limits on plugin subprocesses

- Plugin processes have full environment, filesystem, network access
- No CPU, memory, or file descriptor limits

### L2: Remove ccc- prefix resolution

- `resolveCommand("foo")` tries `ccc-foo` — PATH manipulation could shadow binaries
- **Fix:** Remove the `ccc-` prefix fallback entirely. Require full binary names in config.

### L3: No CLI input length/character validation

- `add-todo`, `add-bookmark` accept arbitrarily long strings

### L4: MCP config written 0o644

- `internal/config/setup.go:83` — `~/.claude/mcp.json` world-readable

### L5: Lock file written 0o644

- `internal/lockfile/lockfile.go:36`

### L8: Credential chain fallback widens trust boundary

- `~/.claude.json` fallback means Claude CLI compromise could inject malicious client IDs

### L9: Strip ANSI from plugin stderr before logging

- Malicious plugin could inject escape sequences into log output
- **Fix:** Apply `ansi.Strip()` to stderr lines before passing to logger

### L10: Worktree symlink entries can traverse out of repo

- `.ccc/config.yaml` symlink entries with `..` resolve outside repo root

---

## Removed from Scope

- ~~M10: Things MCP shell interpolation~~ — removing Things MCP entirely
- ~~M11: Malicious repo .ccc/config.yaml scripts~~ — out of scope
- ~~L6: HTTP client transport timeouts~~ — context timeout at refresh level is sufficient
- ~~L7: Gmail scope escalation in advanced mode~~ — out of scope

---

## Positive Findings

- All SQL queries parameterized — zero injection vectors in `internal/db/`
- Gmail SafeGmailClient properly enforced — no Send/Delete/Trash, unexported `svc`
- TLS enforced on all API calls, no SSRF vectors
- Good subprocess error handling — timeouts, crash detection, graceful degradation
- Token values not leaked to logs or event bus
- Go OAuth token files use correct 0o600 permissions

---

## Implementation Plan

### Phase 1: Quick wins (permissions + OAuth basics)

- [ ] 1. File permissions: 0o600 for config.yaml, config.yaml.bak, mcp.json, lock file
- [ ] 2. Directory permissions: 0o700 for DB dir, socket dir
- [ ] 3. Gmail MCP token: `{ mode: 0o600 }` in `writeFileSync`
- [ ] 4. Bind calendar OAuth to `127.0.0.1` only
- [ ] 5. Generate random OAuth state + validate on callback
- [ ] 6. `io.LimitReader` on Slack/Granola API responses
- [ ] 7. Remove Things MCP server entirely (`servers/things/`)

### Phase 2: Data sanitization + protocol hardening

- [ ] 8. Strip ANSI escapes from external API data at refresh boundary
- [ ] 9. Strip ANSI from plugin stderr before logging
- [ ] 10. Plugin config scoping: `config_scopes` in `ready` response, host filters config sections
- [ ] 11. Validate plugin slug uniqueness + reject reserved built-in slugs
- [ ] 12. Auto-prefix plugin event topics with slug
- [ ] 13. Remove `ccc-` prefix fallback in `resolveCommand`

### Phase 3: Deeper security hardening

- [ ] 14. Add PKCE to all OAuth flows (Go + TypeScript)
- [ ] 15. `syscall.Flock()` for atomic lock acquisition
- [ ] 16. Sandbox plugin migrations to namespaced DDL only
- [ ] 17. Constrain launch action `dir` to Sessions learned paths list
