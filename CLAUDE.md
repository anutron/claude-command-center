# Claude Command Center — Development Guide

## Project Overview

CCC is a terminal-based productivity dashboard built in Go with bubbletea. It aggregates data from external services (calendar, GitHub, Granola, Slack, Gmail) into a single TUI with a plugin architecture.

**Key binaries:** `ccc` (TUI), `ai-cron` (data fetcher)
**Config:** `~/.config/ccc/config.yaml`
**Database:** `~/.config/ccc/data/ccc.db` (SQLite, WAL mode)

## Build & Test

```bash
make build     # Build ccc + ai-cron binaries
make test      # Run all tests (go test -v ./...)
make install   # Symlink binaries to /usr/local/bin
make servers   # Build MCP servers (gmail)
```

## Development Workflow: Spec-Driven Development

Every behavioral change follows this process:

1. **Spec first** — Create or update the spec in `specs/` before or alongside implementation
2. **Review** — Show spec to Aaron for approval on new features (wait for approval)
3. **Tests from spec** — Write tests that validate the spec's behavior section
4. **Implement** — Write code until tests pass
5. **Commit** — Tests green, spec updated, commit together

### Spec Format

Specs live in `specs/` organized by area:
- `specs/core/` — config, db, host, refresh
- `specs/plugin/` — interface, event-bus, registry, protocol
- `specs/builtin/` — sessions, command-center, settings, prs, console

```markdown
# SPEC: Feature Name

## Purpose
Why this exists, what problem it solves

## Interface
- **Inputs**: What goes in
- **Outputs**: What comes out
- **Dependencies**: What it needs

## Behavior
Step-by-step: given X, system does Y, resulting in Z

## Test Cases
- Happy path
- Error cases
- Edge cases
```

### Key Rules

- **Spec is the source of truth** — if code and spec disagree, fix the code
- **Update spec on the same turn as behavioral changes** — never let them drift
- **Tests validate spec compliance** — not implementation details

### Required Process for Every Change

Every behavioral change — bug fix, feature, refactor — follows this order:

1. **Spec** — update the relevant spec in `specs/` (add the new behavior, fix the description, add test cases)
2. **Unit tests** — write or update tests that validate the spec's behavior section
3. **View tests** — if the change affects rendered UI output (key bindings, status indicators, tab content, hint bars, overlays), add a `TestView_*` test in the appropriate `*_view_test.go` file
4. **Implement** — write code to pass the tests
5. **Run tests** — `make test` must pass before committing

**View tests are mandatory for UI changes.** View tests exercise the full plugin: inject state, send keys via `HandleKey()`, render `View()`, and assert on the output with `strings.Contains`. They live in dedicated files:

- `internal/builtin/commandcenter/commandcenter_view_test.go`
- `internal/builtin/sessions/sessions_view_test.go`
- `internal/builtin/settings/settings_view_test.go`
- `internal/builtin/prs/prs_view_test.go`
- `internal/tui/model_view_test.go`

**Shared test infrastructure** in `internal/testutil/`:
- `view_helpers.go` — `AssertViewContains`, `KeyMsg`, `SendKeys`
- `mock_runner.go` — `MockRunner` implementing `agent.Runner` for agent-dependent tests
- `daemon_helpers.go` — `StartTestDaemon`, `InsertTestTodo`, `InsertTestPR`

**What needs a view test:**
- Key binding changes → verify the key produces expected view output
- Status indicator changes → verify the rendered badge/text
- Tab/navigation changes → verify correct content renders per tab
- Overlay/modal changes → verify overlay text appears in view
- Hint bar changes → verify hint text

**What doesn't need a view test:**
- DB-only changes (query optimization, schema migration)
- Daemon RPC internals (unit test the RPC handler directly)
- Config loading (unit test)

## Project Structure

```
cmd/ccc/              # TUI binary entrypoint
cmd/ai-cron/          # Refresh binary entrypoint
internal/
  agent/              # Agent runner, budget, rate limiting, PTY, log tailing
  auth/               # OAuth/authentication helpers
  automation/         # Automation process framework
  builtin/            # Built-in plugins (command-center, prs, sessions, settings)
  config/             # Config loading, validation
  daemon/             # Background daemon (session registry, RPCs, events)
  db/                 # SQLite schema, queries, types (single source of truth for data types)
  doctor/             # Diagnostic checks
  external/           # External plugin subprocess adapter
  llm/                # LLM client wrappers, observable instrumentation
  lockfile/           # File locking
  plugin/             # Plugin interface, registry, event bus, logger, migrations
  refresh/            # Data fetch, merge, locking
  sanitize/           # Output sanitization
  testutil/           # Shared test infrastructure (view helpers, mock runner, daemon helpers)
  tui/                # Bubbletea host, tab navigation, styles, console overlay
  ui/                 # Shared UI formatting helpers
  worktree/           # Git worktree utilities
specs/                # Feature specifications
docs/                 # Roadmap, ideas, product requirements
examples/             # Example external plugins
scripts/              # Utility scripts
sdk/                  # SDK (Python)
servers/              # MCP servers
  gmail/              # Gmail MCP server (TypeScript)
  things/             # Things MCP server
```

## Architecture Conventions

- **Types live in `internal/db/`** — shared schema contract between refresh (producer) and plugins (consumer). Avoids circular imports.
- **Plugins are the unit of UI** — each plugin owns its tab, routes, and key handling via the `plugin.Plugin` interface
- **External plugins speak JSON-lines over stdin/stdout** — any language, subprocess lifecycle managed by `internal/external/`
- **Event bus for cross-plugin communication** — publish/subscribe, no direct plugin-to-plugin imports
- **Namespaced migrations** — each plugin owns its SQLite tables via `plugin.Migration`

## Gmail Safety Rules

- NEVER add Send, Delete, or Trash methods to SafeGmailClient (`internal/refresh/sources/gmail/client.go`)
- NEVER expose the raw `*gmail.Service` outside the client wrapper
- These restrictions require explicit override from the user — do not infer permission from general instructions
- Future: settings-based read-only permission will further lock this down

## Code Style

- **Go standard formatting** — `gofmt`
- **No over-engineering** — solve the current problem, not hypothetical future ones
- **Prefer editing existing files** over creating new ones
- **Be opinionated** — when recommending approaches, analyze 2-3 options with tradeoffs, pick one, defend it

## Git Conventions

- **Commit frequently** — each completed chunk of work gets a commit
- **Imperative messages** — "Add X feature", "Fix Y bug", not "Added" or "Fixes"
- **Co-author line** for AI-assisted commits

## Documentation

- `specs/` — behavioral specifications (source of truth)
- `docs/` — roadmap, ideas, product requirements
- Inline comments only where logic isn't self-evident
- Update docs when adding features or changing behavior
