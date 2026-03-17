# SPEC: CCC v0.1.0 Release Plan

## Purpose

Prepare CCC for distribution to coworkers as both a usable terminal dashboard and an extensible plugin platform. The primary "user" of the documentation is a coworker's Claude agent — docs must be precise, actionable, and structured so an LLM can follow them step-by-step.

## Target Audience

- Coworkers who want a terminal productivity dashboard
- Coworkers who want to build custom plugins for their own workflows
- Their Claude agents, who will handle installation and configuration

## Design Decisions

- **Distribution:** GitHub repo clone (no Homebrew/binary releases)
- **First-run experience:** Core-first, sources a la carte. CCC launches and is useful with zero data sources configured. Each source is added independently when the user wants it.
- **Documentation structure:** AGENTS.md at root as the AI-agent entry point, modular per-source docs, separate plugin developer guide
- **Threads:** Excluded from all documentation (being removed in parallel)

## Phase 1: Documentation

### README.md (repo root)

Human-facing pitch document. Not a setup guide.

- What CCC is and why it exists (terminal dashboard aggregating calendar, GitHub, todos, meetings, Slack, Gmail)
- Feature highlights with screenshots
- Architecture at a glance: TUI binary (`ccc`) + data fetcher (`ccc-refresh`) + plugin system
- Quick pointer to AGENTS.md for setup
- No installation details — AGENTS.md owns that

### AGENTS.md (repo root)

The entry point for any Claude agent cloning the repo. Must be followable end-to-end by an LLM without ambiguity.

- System requirements: Go 1.25+, Node.js 18+, npm, macOS (darwin), git
- Step-by-step: `git clone` → `make install` → `ccc setup`
- What to expect on first launch (empty data sources, how to add them)
- Troubleshooting common failures (missing dependencies, build errors, permission issues)
- Links to per-source setup docs and plugin developer guide

### docs/sources/{calendar,github,gmail,slack,granola}.md

One file per data source. Each follows the same structure:

- **What it provides:** Which tabs/data this source powers
- **Prerequisites:** Credentials, tokens, accounts needed
- **Step-by-step setup:** Exact commands/config changes Claude should make
- **Verification:** How to confirm the source is working (what to look for in the TUI)
- **Common issues:** Known failure modes and fixes

Sources to document:
- `calendar.md` — Google Calendar OAuth flow
- `github.md` — GitHub CLI auth (`gh auth login`)
- `gmail.md` — Gmail OAuth flow (MCP server)
- `slack.md` — Slack bot token setup
- `granola.md` — Granola.so account connection

### docs/plugin-development.md

Plugin developer guide for coworkers who want to extend CCC.

- Architecture overview: JSON-lines over stdin/stdout, subprocess lifecycle
- Protocol reference: every message type in both directions (host→plugin, plugin→host)
- Two-phase init: init without config → plugin declares config_scopes → host sends scoped config
- "Build your first plugin" tutorial based on the pomodoro example
- Python SDK reference (bundled in repo)
- Testing and debugging tips
- How to register plugins in config.yaml `externalPlugins` section

## Phase 2: Security Audit Validation

Re-check the 5 HIGH-severity findings from the March 14 security audit against current code:

1. Config file permissions (world-readable config with tokens)
2. Arbitrary SQL via plugin migrations
3. Unconstrained external plugin launch paths
4. Missing OAuth state validation
5. OAuth server binding to all interfaces

For each:
- Verify whether the issue is still present in current code
- Fix if quick and straightforward
- Document with rationale if deferring (trusted coworker audience)

## Phase 3: Clean Environment QA

Test the full experience a coworker's Claude would have:

- Fresh `git clone` → `make install` → `ccc setup` on a clean-ish environment
- Verify system requirement error messages are clear (missing Go, Node, etc.)
- Walk through adding one data source (GitHub via `gh auth login` is simplest)
- Launch TUI, confirm it works with zero sources and with one source configured
- Load the pomodoro example plugin, confirm external plugin flow works
- Follow AGENTS.md end-to-end as an LLM would — verify every step is unambiguous
- Issues found feed back into docs or get fixed directly

## Phase 4: Release

- Tag version v0.1.0
- Ensure repo is in a clean state where `git clone` + follow AGENTS.md works
- Share with coworkers

## Success Criteria

- A coworker's Claude can go from `git clone` to a running CCC dashboard by following AGENTS.md alone
- Each data source can be added independently by following its source doc
- A coworker can build and register a custom plugin by following the plugin dev guide
- No HIGH-severity security issues remain unaddressed or undocumented
- Clean environment QA passes with no undocumented failures
