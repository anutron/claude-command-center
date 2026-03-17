# CCC v0.1.0 Security Audit Validation

Validation of the 5 HIGH-severity findings from the [March 14 security audit](security-audit-2026-03-14.md), performed 2026-03-16.

## Finding Dispositions

### H1: Full config (with Slack token) sent to all external plugins

- **Status:** FIXED (prior to this review)
- **Evidence:** `internal/external/protocol.go` — plugins declare `config_scopes` in their `ready` response. Host filters config to only requested sections. Plugins with no declared scopes receive no config (secure by default).

### H2: External plugin arbitrary SQL via migrations

- **Status:** MITIGATED (prior to this review), DEFERRED for further hardening
- **Evidence:** `internal/plugin/migrations.go` — `ValidateExternalMigrationSQL()` restricts external plugin migrations to namespaced DDL only (`CREATE TABLE IF NOT EXISTS <slug>_*`, `ALTER TABLE <slug>_*`, etc.). Arbitrary SQL is rejected.
- **Residual risk:** Trusted coworker plugins only. The regex-based validation could theoretically be bypassed with crafted SQL, but the threat model assumes trusted plugin authors.
- **Decision:** Acceptable for v0.1.0. Only trusted plugins from coworkers are loaded.

### H3: Unconstrained external plugin launch paths

- **Status:** DEFERRED
- **Evidence:** `internal/external/loader.go` — `resolveCommand()` passes the command through as-is from config. Reserved slug validation prevents impersonating built-in plugins, and duplicate slug detection is in place.
- **Residual risk:** A plugin config entry could specify any executable on PATH. This is by design — the user explicitly configures plugins in `config.yaml`.
- **Decision:** Acceptable for v0.1.0. Users control their own config files and only add plugins they trust. Same trust model as shell aliases or PATH entries.

### H4: OAuth state parameter never validated

- **Status:** FIXED (prior to this review)
- **Evidence:**
  - `internal/auth/flow.go:113-116` — generates 16 bytes of crypto/rand, hex-encoded as state parameter
  - `internal/auth/flow.go:129-133` — validates state in callback, returns HTTP 403 on mismatch
  - `internal/auth/flow.go:158` — passes state to `AuthCodeURL()`
  - `internal/refresh/sources/calendar/auth.go:107-112` — same pattern in legacy calendar auth
  - `internal/refresh/sources/calendar/auth.go:135-139` — validates state in callback
- **Additional hardening:** Both flows also implement PKCE (RFC 7636) via `auth.GeneratePKCE()`, adding code_challenge/code_verifier to prevent authorization code interception.

### H5: Calendar OAuth binds to all interfaces

- **Status:** FIXED (prior to this review)
- **Evidence:**
  - `internal/auth/flow.go:101` — `net.Listen("tcp", "127.0.0.1:0")` (localhost only, random port)
  - `internal/refresh/sources/calendar/auth.go:92` — `net.Listen("tcp", "127.0.0.1:0")` (localhost only, random port)
- Both OAuth callback servers bind exclusively to the loopback interface, preventing LAN access.

## Summary

| Finding | Severity | Status | Action |
|---------|----------|--------|--------|
| H1: Config leak to plugins | HIGH | Fixed | Config scoping via `config_scopes` |
| H2: Arbitrary SQL migrations | HIGH | Mitigated | DDL validation + trusted plugins only |
| H3: Unconstrained plugin paths | HIGH | Deferred | Trusted config, user-controlled |
| H4: Missing OAuth state | HIGH | Fixed | Crypto/rand state + PKCE |
| H5: OAuth binds all interfaces | HIGH | Fixed | Bound to 127.0.0.1 |

All 5 HIGH findings are either fixed or have an explicit accept-risk decision appropriate for the v0.1.0 trust model (single user + trusted coworkers).
