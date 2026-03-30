# Spec Audit: internal/config, internal/auth, internal/doctor, internal/lockfile, internal/sanitize

**Date:** 2026-03-29
**Specs:** specs/core/config.md, specs/core/refresh.md (Locking, OAuth Hardening, Data Sanitization)

---

## internal/config/config.go

### Config struct fields

- **[COVERED]** Core fields (Name, Palette, Calendar, GitHub, Todos, Granola, Slack, Gmail, ExternalPlugins) — config.md "Config Loading" + example YAML
- **[UNCOVERED-BEHAVIORAL]** `DisabledPlugins []string` — no spec describes the plugin enable/disable mechanism via config. **Intent question:** Should the config spec document the DisabledPlugins list and its interaction with PluginEnabled/SetPluginEnabled?
- **[UNCOVERED-BEHAVIORAL]** `AgentConfig` with budget caps, rate limiting, backoff fields — no spec covers agent config defaults or budget governance config. **Intent question:** Is there a separate agent governance spec, or should config.md document these fields?
- **[UNCOVERED-BEHAVIORAL]** `DaemonConfig` (RefreshInterval, SessionRetention) — not in config spec. **Intent question:** Should daemon config be documented here or in a separate daemon spec?
- **[UNCOVERED-BEHAVIORAL]** `RefreshConfig` (Enabled, Model) — the `RefreshEnabled()` method defaults to true when nil. Not in config spec. **Intent question:** Should the refresh toggle be documented?

### PluginEnabled / SetPluginEnabled

- **[UNCOVERED-BEHAVIORAL]** Checks both DisabledPlugins (built-in) and ExternalPlugins (by name + Enabled field). No spec describes this dual-lookup logic. **Intent question:** Should the config spec describe how plugin enablement works across built-in and external plugins?

### ParseRefreshInterval

- **[COVERED]** refresh.md "Configurable Refresh Interval": "parses the duration string, returning DefaultRefreshInterval (5m) if the string is empty, unparseable, or less than 1 minute"
- **[COVERED]** cli.md test cases: "ParseRefreshInterval with valid durations", "with empty/invalid → returns default", "with <1m → returns default"

### DefaultConfig

- **[COVERED]** config.md "Default Config": Name "Command Center" (code: "Claude Command"), palette aurora, todos enabled
- **[CONTRADICTS]** Spec says default name is "Command Center" but code returns "Claude Command". The code has `Name: "Claude Command"` on line 286.

### Path resolution (ConfigDir, ConfigPath, DataDir, DBPath, CredentialsDir)

- **[COVERED]** config.md "Path Resolution": all five functions match spec exactly

### Load

- **[COVERED]** config.md "Config Loading": reads ConfigPath, returns DefaultConfig if missing, errors on other failures

### Save

- **[COVERED]** config.md "Config Saving" (basic behavior)
- **[UNCOVERED-BEHAVIORAL]** Regression detection (`detectRegression`) — Save refuses to overwrite if it would lose user data (name, external plugins, calendar config, automations). Not in spec. **Intent question:** Should the save safety checks (regression detection, backup, atomic write) be documented?
- **[UNCOVERED-BEHAVIORAL]** Force parameter bypasses regression checks (used by settings UI). Not in spec.
- **[UNCOVERED-BEHAVIORAL]** Backup file creation (`.bak`) before writes. Not in spec.
- **[UNCOVERED-BEHAVIORAL]** Atomic write via temp file + rename. Not in spec.

### BannerVisible / SetShowBanner / GetBannerTopPadding / SetBannerTopPadding

- **[COVERED]** onboarding.md "Banner Visibility": `ShowBanner` is `*bool`, nil defaults to true
- **[UNCOVERED-BEHAVIORAL]** `BannerTopPadding` (defaults to 2, clamped to >=0). Not mentioned in any spec. **Intent question:** Should banner top padding be spec'd?

### Palettes

- **[COVERED]** config.md "Palettes": 5 built-in, GetPalette fallback to aurora, custom palette support, PaletteNames

### CalendarEntry.IsEnabled / SetEnabled

- **[UNCOVERED-IMPLEMENTATION]** Accessor methods, no spec needed

### GitHubConfig.IsTrackMyPRs / SetTrackMyPRs

- **[UNCOVERED-BEHAVIORAL]** `TrackMyPRs` defaults to true when nil. Not in config spec. **Intent question:** Should the "all my PRs" tracking mode be documented in config spec?

### SlackConfig.EffectiveToken

- **[UNCOVERED-BEHAVIORAL]** Token precedence: `Token` > `BotToken` (deprecated). Not in config spec but implicitly covered by validate.go's LoadSlackToken which has similar logic.

### GmailConfig

- **[UNCOVERED-BEHAVIORAL]** `TodoLabel` and `Advanced` fields not mentioned in config spec.

---

## internal/config/palettes.go

- **[COVERED]** All behavior matches config.md "Palettes" section

---

## internal/config/schedule.go

### IsScheduleInstalled / InstallSchedule / UninstallSchedule

- **[COVERED]** cli.md "ccc install-schedule" and "ccc uninstall-schedule" sections
- **[COVERED]** refresh.md "Background Scheduling" — crontab marker, env sourcing, log file, legacy cleanup

### cleanupLegacyPlist

- **[COVERED]** cli.md: "Cleans up legacy launchd plist if present"

---

## internal/config/setup.go (MCP)

### IsMCPBuilt / GenerateMCPConfig / BuildAndConfigureMCP

- **[COVERED]** onboarding.md "Step 3: Done" — MCP build during onboarding
- **[UNCOVERED-BEHAVIORAL]** `GenerateMCPConfig` writes to `~/.claude/mcp.json` with 0o600 permissions. Covered partially by config.md "File Permissions" for mcp.json.
- **[UNCOVERED-BEHAVIORAL]** `BuildAndConfigureMCP` runs `npm install && npm run build` for each MCP server if dist/index.js missing. Not explicitly spec'd outside onboarding context.

### findServersDir

- **[COVERED]** config.md "Repo Path Resolution" — symlink-aware resolution order

---

## internal/config/validate.go

### ValidateCalendar / ValidateGitHub / ValidateSlack / ValidateGmail / ValidateGranola

- **[COVERED]** cli.md "ccc doctor" lists all checks: calendar creds, GitHub CLI, Granola, Slack token
- **[COVERED]** onboarding.md "Step 2: Data Sources Hub" — auto-detection

### LoadSlackToken

- **[UNCOVERED-BEHAVIORAL]** Full precedence chain: Config.Token > Config.BotToken > SLACK_TOKEN env > SLACK_BOT_TOKEN env. No single spec documents this full chain. **Intent question:** Should credential resolution chains be documented in config spec?

---

## internal/config/shell.go

### IsShellHookInstalled / InstallShellHook / UninstallShellHook

- **[UNCOVERED-BEHAVIORAL]** The shell hook auto-launches CCC on interactive shell open and restores last-dir on exit. No spec documents this feature. **Intent question:** Should the shell hook (auto-launch + last-dir restore) have its own spec or a section in cli.md?

---

## internal/config/skills.go

### SkillNames / IsSkillInstalled / InstallSkills / UninstallSkills

- **[UNCOVERED-BEHAVIORAL]** Skill management (symlink repo skills to ~/.claude/skills). Not in any spec. **Intent question:** Should skill installation be spec'd in config.md or a new skills spec?

---

## internal/auth/config.go

### LoadGoogleOAuth2Config

- **[UNCOVERED-IMPLEMENTATION]** Thin wrapper around oauth2.Config creation

### LoadCalendarCredsFromClaudeConfig

- **[COVERED]** refresh.md mentions "Calendar cred chain: credentials.json -> token.json -> env vars -> ~/.claude.json"
- **[UNCOVERED-BEHAVIORAL]** Specific parsing of `~/.claude.json` MCP server config for calendar creds. Not explicitly spec'd. The credential chain is mentioned in project memory but not in a spec.

---

## internal/auth/env.go

### LoadEnvFile / ReadEnv / WriteEnvValue

- **[COVERED]** refresh.md "Load env vars from ~/.config/ccc/.env"
- **[UNCOVERED-BEHAVIORAL]** `LoadEnvFile` does not overwrite existing env vars. `WriteEnvValue` creates parent dirs, does in-place update or append. These behaviors are not spec'd. **Intent question:** Should env file handling have its own spec section?

---

## internal/auth/flow.go

### ValidateClientCredentials

- **[COVERED]** refresh.md "OAuth Hardening" — credential validation before browser launch
- **[UNCOVERED-BEHAVIORAL]** Specific placeholder detection (list of known fake client IDs). Not detailed in spec.

### AuthFlowCmd / runAuthFlow

- **[COVERED]** refresh.md "OAuth Hardening" — PKCE, random state, loopback binding
- **[COVERED]** refresh.md: "state parameter is random and validated on callback"
- **[COVERED]** refresh.md: "PKCE code verifier/challenge generated per flow"

### saveTokenFile

- **[COVERED]** config.md "File Permissions": "Go OAuth token files (token.json) already use 0o600"

---

## internal/auth/pkce.go

### GeneratePKCE / AuthURLParams / ExchangeParams

- **[COVERED]** refresh.md "OAuth Hardening" — "PKCE (S256): All OAuth2 flows use Proof Key for Code Exchange"

---

## internal/auth/types.go

### GoogleTokenFile / ToOAuth2Token

- **[UNCOVERED-IMPLEMENTATION]** Data type and conversion, no behavioral spec needed

---

## internal/doctor/doctor.go

### RunDoctor

- **[COVERED]** cli.md "ccc doctor" — lists all checks, exit code behavior
- **[UNCOVERED-BEHAVIORAL]** `DoctorProvider` interface delegation — doctor accepts plugin-provided checks. The provider mechanism itself isn't spec'd, though individual checks are.
- **[UNCOVERED-BEHAVIORAL]** `--live` flag enables live network checks. cli.md mentions `--live` in usage but doesn't describe what it controls.
- **[UNCOVERED-BEHAVIORAL]** `[??]` inconclusive status not mentioned in cli.md (only `[OK]` and `[!!]`)

### checkDataFreshness

- **[COVERED]** cli.md: "Data freshness — warns if generated_at > 30 minutes stale"

---

## internal/lockfile/lockfile.go

### AcquireLock / IsLocked

- **[COVERED]** refresh.md "Locking": flock-based advisory locking, ErrAlreadyLocked, 0o600 permissions
- **[COVERED]** refresh.md: "atomic and eliminates the TOCTOU race condition"

---

## internal/sanitize/sanitize.go

### StripANSI

- **[COVERED]** refresh.md "Data Sanitization": "stripped of ANSI escape sequences at the refresh boundary"

---

## Spec -> Code Direction Gaps

1. **config.md mentions `HomeDir`** with behavior "on Sessions plugin Init, if set and not already in learned paths, it is prepended." This is implemented in the sessions plugin, not in config — no code gap.
2. **config.md mentions `home_dir` in Default Config** as empty string. The code's DefaultConfig does not set HomeDir — consistent (zero value is "").
3. **config.md "Default Config" says Name: "Command Center"** but code says "Claude Command" — **CONTRADICTS**.

---

## Summary

- **CONTRADICTS: 1** — Default config name mismatch ("Command Center" in spec vs "Claude Command" in code)
- **UNCOVERED-BEHAVIORAL: 16** — DisabledPlugins, AgentConfig, DaemonConfig, RefreshConfig, Save safety, BannerTopPadding, TrackMyPRs, GmailConfig fields, shell hook, skills management, LoadSlackToken chain, env file handling, MCP build outside onboarding, placeholder detection, doctor --live, doctor inconclusive status
- **COVERED: ~25 behavioral paths**
