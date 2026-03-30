# Spec Audit: cmd/ccc, cmd/ai-cron

**Date:** 2026-03-29
**Specs:** specs/core/cli.md, specs/core/refresh.md, specs/core/llm.md, specs/core/automations.md

---

## cmd/ccc/main.go

### Subcommand dispatch

- **[COVERED]** cli.md documents: default (TUI), setup, doctor, install-schedule, uninstall-schedule, notify, update-todo, sessions, help
- **[UNCOVERED-BEHAVIORAL]** `daemon` subcommand (start/stop/status) — not in cli.md. **Intent question:** Add daemon subcommand to cli spec?
- **[UNCOVERED-BEHAVIORAL]** `register` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `update-session` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `stop-all` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `refresh` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `add-todo` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `todo` subcommand (--get, --fetch-context) — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `add-bookmark` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `paths` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `worktrees` subcommand — not in cli.md
- **[UNCOVERED-BEHAVIORAL]** `--daemon-internal` flag for daemon process — not in cli.md

### First-run detection

- **[COVERED]** onboarding.md "First-Run Detection": config file existence check

### Config load error handling

- **[UNCOVERED-BEHAVIORAL]** On config load error, prints fix instructions and mentions .bak backup if present. Not in any spec.

### DB open error handling

- **[COVERED]** host.md "Database required": "exits with a clear error message"

### LLM construction (TUI)

- **[COVERED]** llm.md "cmd/ccc (TUI)": sandboxed ClaudeCLI with Timeout:90s, Tools:ptr(""), DisableSlashCommands:true

### External plugin loading

- **[COVERED]** host.md: external plugins loaded and passed to NewModel

### Signal handling

- **[COVERED]** host.md: "SIGINT and SIGTERM trigger graceful shutdown"

### TUI loop (launch cycle)

- **[COVERED]** host.md: TUI launches, user picks item, claude runs, returns to TUI
- **[UNCOVERED-BEHAVIORAL]** `last-dir` file written after RunClaude for shell hook cd. Not in any spec.

### printUsage

- **[COVERED]** cli.md "ccc help": prints usage. But usage text lists many more subcommands than spec documents.

---

## cmd/ccc/daemon_cmd.go

### runDaemon (start/stop/status)

- **[UNCOVERED-BEHAVIORAL]** Entire daemon lifecycle management — start (detach, PID file), stop (SIGTERM), status (PID check, socket ping). No spec. **Intent question:** Should there be a daemon spec?

### runDaemonInternal

- **[UNCOVERED-BEHAVIORAL]** Daemon main loop: loads config, opens DB, creates governed agent runner, serves via daemon.Server. No spec.

### runRefresh (daemon-internal)

- **[COVERED]** refresh.md: builds data sources from config, runs refresh, then automations
- **[COVERED]** automations.md: "run after main refresh data has been saved"

---

## cmd/ccc/session_cmd.go

### runRegister

- **[UNCOVERED-BEHAVIORAL]** Registers session with daemon (fallback: direct DB write). No spec.

### runUpdateSession

- **[UNCOVERED-BEHAVIORAL]** Updates session topic via daemon RPC. No spec.

### runRefreshCmd

- **[UNCOVERED-BEHAVIORAL]** Triggers refresh via daemon. No spec.

### runStopAll

- **[UNCOVERED-BEHAVIORAL]** Emergency stop via daemon RPC. No spec.

---

## cmd/ccc/worktrees.go

### runWorktreesList / runWorktreesPrune

- **[UNCOVERED-BEHAVIORAL]** CLI for listing and pruning worktrees across all learned paths. Not in cli.md. The worktree.md spec documents the library functions but not the CLI commands.
- **[UNCOVERED-BEHAVIORAL]** Prune requires user confirmation (y/N prompt). Not in any spec.

---

## cmd/ccc/paths.go

### runPaths

- **[UNCOVERED-BEHAVIORAL]** Lists learned project paths (plain text or --json). Not in cli.md.
- **[UNCOVERED-BEHAVIORAL]** `--auto-describe` generates LLM-based descriptions. Mentioned in llm.md "cmd/ccc paths --auto-describe" but not in cli.md.
- **[UNCOVERED-BEHAVIORAL]** `--add-rule` with `--use-for`, `--not-for`, `--prompt-hint` for routing rules. Not in cli.md.
- **[UNCOVERED-BEHAVIORAL]** `--refresh-skills` forces skill cache refresh. Not in any spec.

---

## cmd/ccc/update_todo.go

### runUpdateTodo

- **[COVERED]** cli.md "ccc update-todo": flags (--id, --session-summary, --session-status), stdin support, notify after update

---

## cmd/ccc/add_bookmark.go

### runAddBookmark

- **[UNCOVERED-BEHAVIORAL]** Saves session bookmark with session-id, project, repo, branch, summary, optional label/worktree-path/source-repo. Not in cli.md.

---

## cmd/ccc/add_todo.go

### runAddTodo

- **[UNCOVERED-BEHAVIORAL]** Adds a todo with title, source, source-ref, context, detail, who-waiting, project-dir, session-id, due, effort. Not in cli.md.

---

## cmd/ccc/todo.go

### runTodo (--get)

- **[UNCOVERED-BEHAVIORAL]** Gets todo by display_id, outputs JSON. Not in cli.md.

### runFetchContext (--fetch-context)

- **[COVERED]** refresh.md "CLI": `ccc todo --fetch-context <display_id>` — manually fetch and cache source context

---

## cmd/ai-cron/main.go

### main

- **[COVERED]** refresh.md "ai-cron Binary": flags (-v, --dry-run, --no-llm)
- **[COVERED]** llm.md "cmd/ai-cron": haiku for extraction, sonnet for routing, NoopLLM fallback
- **[COVERED]** refresh.md: builds DataSources from config, context registry
- **[COVERED]** refresh.md "Locking": acquires lock via lockfile.AcquireLock
- **[COVERED]** automations.md: runs automations after refresh

---

## Spec -> Code Direction Gaps

1. **cli.md lists "ccc sessions"** as "Alias for default (launches TUI)" — code confirms this, no gap.
2. **cli.md** is significantly behind on subcommand documentation. 11+ subcommands exist in code but are not in the spec.
3. **printUsage** in main.go lists all subcommands including those not in cli.md — the usage text is more complete than the spec.

---

## Summary

- **CONTRADICTS: 0**
- **UNCOVERED-BEHAVIORAL: 20+** — daemon subcommands, register, update-session, stop-all, refresh, add-todo, add-bookmark, todo --get, paths (all flags), worktrees (list/prune), last-dir file, config error handling, daemon internal loop, session registration
- **COVERED: ~12 behavioral paths** — cli.md covers the original subcommands well but has not kept pace with significant feature additions (daemon, session management, paths, worktrees, bookmarks, todos).
