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
- Big selling points to hit:
  - Fast access to Claude where you work
  - Bookmark and resume sessions
  - Automated todo list management
  - Agentic todo list resolution
- Feature highlights with screenshots (user will provide screenshots; leave placeholders in the README for each feature)
- Architecture at a glance: TUI binary (`ccc`) + data fetcher (`ccc-refresh`) + plugin system
- Quick pointer to AGENTS.md for setup
- No installation details — AGENTS.md owns that

### AGENTS.md (repo root)

The entry point for any Claude agent cloning the repo. Must be followable end-to-end by an LLM without ambiguity.

- System requirements: Go 1.25+, Node.js 18+, npm, macOS (darwin), git
- Step-by-step: `git clone` → `make install` → `ccc setup`
- What to expect on first launch (empty data sources, how to add them)
- Minimal `config.yaml` example showing structure with zero sources (fallback if `ccc setup` fails)
- Troubleshooting common failures (missing dependencies, build errors, `go mod download` network issues, permission issues)
- Links to per-source setup docs and plugin developer guide

### docs/sources/{calendar,github,gmail,slack,granola}.md

Listed and linked in the README as built-in connectors.

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
- Python helper module reference (`examples/pomodoro/ccc_plugin.py` — minimal wrapper, not a full SDK)
- Testing and debugging tips
- How to register plugins in config.yaml `externalPlugins` section

## Phase 2: Security Audit Validation

Re-check the 5 HIGH-severity findings from the March 14 security audit against current code:

1. Config file permissions (world-readable config with tokens)
2. Arbitrary SQL via plugin migrations
3. Unconstrained external plugin launch paths
4. Missing OAuth state validation
5. OAuth server binding to all interfaces

Expected disposition (verify against current code, adjust if needed):

1. **Config file permissions** — likely **defer** (coworkers own their machines)
2. **Arbitrary SQL via plugin migrations** — likely **defer** (only trusted plugins from coworkers)
3. **Unconstrained plugin launch paths** — likely **defer** (same trust model)
4. **Missing OAuth state validation** — likely **fix** (low effort, real attack surface even among coworkers)
5. **OAuth binding to all interfaces** — likely **fix** (low effort, real attack surface)

For each: verify current state, fix or document with rationale.

## Phase 3: Clean Environment QA

Claude guides the user through this QA process step-by-step (Claude does not perform it autonomously):

- **Prerequisite:** Threads removal must be merged before QA begins
- **"Clean environment"** = a directory that has never contained CCC, on a machine with Go 1.25+ and Node 18+ installed. Do not reuse existing `~/.config/ccc/`
- Guide user through: fresh `git clone` → `make install` → `ccc setup`
- Ask user to verify system requirement error messages are clear (missing Go, Node, etc.)
- Walk user through adding one data source (GitHub via `gh auth login` is simplest)
- Ask user to launch TUI, confirm it works with zero sources and with one source configured
- Guide user through loading the pomodoro example plugin, confirm external plugin flow works
- Ask user to follow AGENTS.md end-to-end as an LLM would — verify every step is unambiguous
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
