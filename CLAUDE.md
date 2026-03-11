# Claude Command Center — Development Guide

## Project Overview

CCC is a terminal-based productivity dashboard built in Go with bubbletea. It aggregates data from external services (calendar, GitHub, Granola, Slack, Gmail) into a single TUI with a plugin architecture.

**Key binaries:** `ccc` (TUI), `ccc-refresh` (data fetcher)
**Config:** `~/.config/ccc/config.yaml`
**Database:** `~/.config/ccc/data/ccc.db` (SQLite, WAL mode)

## Build & Test

```bash
make build     # Build ccc + ccc-refresh binaries
make test      # Run all tests (go test -v ./...)
make install   # Symlink binaries to /usr/local/bin
make servers   # Build MCP servers (gmail, things)
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
- `specs/builtin/` — sessions, command-center, settings, pomodoro

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

## Project Structure

```
cmd/ccc/              # TUI binary entrypoint
cmd/ccc-refresh/      # Refresh binary entrypoint
internal/
  builtin/            # Built-in plugins (sessions, command-center, settings)
  config/             # Config loading, validation
  db/                 # SQLite schema, queries, types (single source of truth for data types)
  external/           # External plugin subprocess adapter
  plugin/             # Plugin interface, registry, event bus, logger, migrations
  refresh/            # Data fetch, merge, locking
  tui/                # Bubbletea host, tab navigation, styles
specs/                # Feature specifications
docs/                 # Roadmap, ideas, product requirements
examples/             # Example external plugins
servers/              # MCP servers (gmail, things)
  gmail/              # Gmail MCP server (TypeScript)
  things/             # Things MCP server (TypeScript, macOS-only)
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
