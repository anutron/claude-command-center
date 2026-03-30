# Spec Audit — 2026-03-29

## Summary

- **Modules analyzed:** 14 (across 9 agent dispatches)
- **Total behavioral branches:** 854
- **Covered by specs:** 587 (69%)
- **Behavioral gaps:** 142
- **Implementation detail (no spec needed):** 46
- **Contradictions:** 19
- **Unimplemented spec promises:** 3

## Coverage by Module

| Module | Branches | Covered | Gaps | Contradictions |
|--------|----------|---------|------|----------------|
| internal/agent/ | 114 | 97 (85%) | 7 | 0 |
| internal/daemon/ | 96 | 79 (82%) | 8 | 0 |
| internal/db/ | 97 | 68 (70%) | 25 | 4 |
| internal/plugin+external/ | 78 | 62 (79%) | 10 | 2 |
| internal/refresh/ | 98 | 62 (63%) | 24 | 4 |
| internal/builtin/commandcenter/ | 127 | 98 (77%) | 16 | 5 |
| internal/builtin/prs/ | 52 | 42 (81%) | 5 | 2 |
| internal/builtin/sessions/ | 38 | 33 (87%) | 5 | 1 |
| internal/builtin/settings/ | 54 | 46 (85%) | 8 | 1 |
| internal/config+auth+doctor | 41 | 25 (61%) | 16 | 1 |
| internal/tui+ui/ | 39 | 20 (51%) | 19 | 3 |
| internal/llm/ | 11 | 11 (100%) | 0 | 0 |
| internal/automation/ | 24 | 20 (83%) | 4 | 0 |
| internal/worktree/ | 17 | 15 (88%) | 3 | 2 |
| cmd/ | 32 | 12 (38%) | 20 | 0 |

## Top Contradictions (by severity)

### Critical — Spec and code actively disagree

1. **internal/db/** — Threads feature removed but spec still references cc_threads table, Thread Operations (DBInsertThread, DBPauseThread, etc.), ActiveThreads/PausedThreads views, and thread-related in-memory mutations. All gone from code.

2. **internal/refresh/** — PR merge strategy: spec says "Merge-based upsert...each fresh PR upserted by ID...agent tracking columns preserved...missing PRs archived." Code does full replacement (`PullRequests: fresh.PullRequests`). No upsert, no archival, no agent column preservation.

3. **internal/refresh/** — New todo status: spec says "active", code sets "new". Different status values at creation.

4. **internal/builtin/commandcenter/** — Triage filter tabs have diverged: spec describes Accepted/New/Review/Blocked/Active/All, code uses todo/inbox/agents/review/all with different filter logic.

5. **internal/builtin/commandcenter/** — Spec says migrations "None — uses existing tables." Code defines 2 migrations (index + session_log_path column).

### Moderate — Naming/shape mismatches

6. **internal/db/** — Schema table list stale: spec says 8 tables, code has ~16 (drops cc_threads, adds cc_source_sync, cc_todo_merges, cc_pull_requests, cc_sessions, cc_automation_runs, cc_agent_costs, cc_budget_state, cc_archived_sessions, cc_ignored_repos).

7. **internal/db/** — LoadCommandCenterFromDB: spec says it returns calendar, todos, threads, suggestions, pending actions, warnings, generated_at. Code loads PRs and merges instead of threads/warnings.

8. **internal/plugin/** — Event.Payload: spec says `map[string]interface{}`, code uses `any`.

9. **internal/plugin/** — ReturnMsg: spec says empty struct, code has `TodoID string` and `WasResumeJoin bool`.

10. **internal/tui/** — Tab table: spec lists "Threads" tab, code has "PRs" and "Sessions" tabs.

11. **internal/config/** — Default config name: spec says "Command Center", code returns "Claude Command".

12. **internal/worktree/** — PrepareWorktree param: spec says `repoRoot`, code takes `dir` (any path, internally resolves).

### Minor — Undocumented rendering/UX details

13. **internal/builtin/prs/** — Enter on agent-failed PR: spec says "Resume session to see what went wrong" but code requires local repo dir, silently no-ops if missing.

14. **internal/builtin/sessions/** — Blocked session visualization (yellow dot + "Blocked" text) not in spec.

15. **internal/builtin/settings/** — PLUGINS category lists "Threads" instead of actual registry slugs.

16. **internal/tui/** — Single-esc quit vs spec's double-esc requirement.

## Top Behavioral Gaps (by impact)

### Stale spec areas (high impact — entire features undocumented)

1. **cmd/** — 11+ CLI subcommands undocumented: `daemon start/stop/status/logs`, `register`, `update-session`, `stop-all`, `add-todo`, `add-bookmark`, `todo --get`, `paths` flags, `worktrees list/prune`. cli.md hasn't kept pace.

2. **internal/db/** — 25 gaps including: todo merges (DBLoadMerges, WerePreviouslyMergedAndVetoed, DBGetOriginalIDs), ignored repos/PRs (DBLoadIgnoredRepos, DBLoadIgnoredPRs, DBAddIgnoredRepo), agent costs (DBInsertAgentCost, etc.), archived sessions, source sync, routing rules PromptHint field. Spec's behavioral section needs a major refresh.

3. **internal/tui/** — 19 gaps: budget status widget, daemon auto-start/reconnect lifecycle, flash messages, Ctrl+Z background, Ctrl+X quit, tab dispatch order, stub plugins, onboarding skills/shell install steps. host.md covers the core but not the daemon integration layer.

### Feature evolution gaps (medium impact)

4. **internal/refresh/** — 24 gaps: source context TTL behavior, search query construction (Slack), label-based Gmail workflows, conversation context resolution, synthesis deduplication (WerePreviouslyMergedAndVetoed), todo routing prompt generation, calendar auto-accept PostMerge logic.

5. **internal/builtin/commandcenter/** — 16 gaps: undocumented keybindings (t quick-add, T train, U unmerge, g chord prefix), wizard selection persistence, agent edit guards, merge/synthesis detection, clarifying question UX, SIGINT graceful shutdown before session resume.

6. **internal/config/** — 16 gaps: new config struct fields (AgentConfig, DaemonConfig, RefreshConfig, DisabledPlugins), Save() safety semantics, shell hook installation, skills management, MCP build & configure outside onboarding.

### Minor gaps (low impact — implementation details leaking)

7. **internal/plugin+external/** — 10 gaps: ScopeConfig inclusion logic, DoctorProvider/DoctorCheck types, external plugin error UI rendering, loader skip-on-duplicate behavior.

8. **internal/builtin/settings/** — 8 gaps: 3 undocumented panes (Sandbox, Automations, PRs), RegisterProvider API, credential reuse optimization.

## Unimplemented Spec Promises

1. **specs/core/daemon.md** — "Socket file permissions checked on connect" — code doesn't verify socket ownership/permissions.
2. **specs/core/daemon.md** — "Graceful drain on SIGTERM" — code calls `Shutdown()` directly on signal, no drain.
3. **specs/core/daemon.md** — "Connection timeout" — no configurable timeout on client connections.

## Delta from Last Audit

First audit — no delta available.

---

Full module reports: `specs/audits/2026-03-29/modules/`
