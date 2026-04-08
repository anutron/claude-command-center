# Changelog

## v1.0.0 (2026-04-08)

Major release introducing the background daemon, agent governance, PR tracking, console observability, and unified sessions — transforming CCC from a read-only dashboard into an agentic productivity system.

### New Features

#### Background Daemon
- Add daemon process with Unix socket JSON-RPC communication
- Session registry with register, update, list, and liveness detection
- Event subscription system for live TUI updates
- Refresh loop with timer and on-demand RPC trigger
- Auto-start daemon from TUI; `ccc daemon start/stop` CLI subcommands
- Watch `~/.claude/session-topics/` to sync topics into sessions
- Daemon start/stop/pause controls in settings panel

#### Agent System
- Extract shared agent runner to `internal/agent/` package
- PTY-based agent runner with native log tailing (replaces pipe-based)
- Agent cost tracking and budget state DB tables
- BudgetTracker for spend enforcement with configurable caps
- RateLimiter for spawn loop prevention and cooldown
- GovernedRunner that enforces budget and rate limits on all agent launches
- Budget RPCs and emergency stop (`ctrl+x`) in daemon
- Agent max runtime timeout and `Shift+X` kill in console overlay
- ObservableLLM wrapper with context-based operation names
- Agent metadata: automation type and project_dir tracking
- Daemon-required for all agent operations (local fallback removed)

#### Console Overlay & Observability
- Console overlay with `~` toggle, list/detail views, and live agent updates
- Standalone `ccc console` streaming TUI
- LLM activity ring buffer with ReportLLMActivity and ListLLMActivity RPCs
- LLM activity rendering in console sidebar with origin and timestamps
- Instrument ai-cron and refresh pipeline LLM calls for daemon observability
- Agent history query joining costs with origin data (todo/PR context)
- StreamAgentOutput and ListAgentHistory daemon RPCs

#### Pull Request Tracking
- Dedicated PR tracking plugin with its own TUI tab
- GitHub PR fetching with headRefOid population
- Agent trigger detection and spawn logic for auto-review
- Agent status indicators on PR rows
- Watch key (`w`) for observing running PR agents
- Context-aware enter key for agent resume/attach/launch
- PR and repo ignore with `i`/`I` hotkeys
- PR plugin settings pane with ignored repos and PRs lists
- Flash message support for user feedback
- Merge-based upsert preserving agent state (replaces delete-all save)

#### Unified Sessions View
- Consolidate 3 session tab entries into single Sessions tab with sub-tabs
- Sub-tab bar with Active, Saved, Archive, and New Session views
- Auto-archive for ended sessions
- Session actions: resume, bookmark, dismiss
- Active sessions view with live daemon updates
- Session topic bridge via CCC_SESSION_ID env var
- Broadcast session and todo state changes to other TUI instances

#### Command Center Enhancements
- Command LLM delegation to real agents for external data requests
- `/todo` skill for capturing tasks directly from Claude Code
- Soft-delete for todos via `deleted_at` column
- Blocked status in todo detail status selector
- Author attribution on Slack todo extraction
- Mouse wheel scrolling for todo list
- Advance to next todo after marking done or dismissing

#### Settings & Configuration
- Agent sandbox configuration in settings
- Budget widget in upper-right corner overlay with agent count
- Budget settings pane with editable caps
- Split daemon controls into separate settings panel
- Daemon config section with refresh interval and session retention
- `ccc-refresh` binary renamed to `ai-cron`

### Bug Fixes

- Fix BUG-113: Down arrow past visible list should auto-expand instead of scrolling
- Fix BUG-114: Todo agent creation doesn't show in sub-agent tracker
- Fix BUG-115: Search filter enter should open selected item, not freeze filter
- Fix BUG-116: Todo disappears from list when agent runs
- Fix BUG-117: Agents tab shows completed agent as done instead of moving to review
- Fix BUG-118: Saved sessions appear under Active tab instead of their own section
- Fix BUG-119: Resume tab shows New Session content; tab switching corrupts all tabs
- Fix BUG-120: Cannot mouse-select text in TUI for copying
- Fix BUG-121: Active tab keybinding collision (a archives, A views archive)
- Fix BUG-122: Session viewer shows "Waiting for events" with no messages
- Fix BUG-123: Hiding banner also hides nav tab bar
- Fix BUG-124: Down arrow auto-expand shows wrong list and takes two presses
- Fix BUG-125: Budget widget agent count includes queued, daemon drains queue
- Fix BUG-126: Sessions launched from CCC don't appear in Active tab
- Fix BUG-127: Stabilize banner and tab bar horizontal position
- Fix BUG-128: Show project name in active sessions, fix resume session ID
- Fix BUG-129: Hide 's sessions' hint when already on sessions sub-tab
- Fix runaway PR agent spawning with three-layer dedup
- Fix agent session ID race and add log-based recovery
- Fix agent cost tracking: accumulate tokens instead of overwriting on finish
- Fix race: refresh no longer deletes todos created mid-cycle
- Fix data race in sessions Refresh() and event bus handlers
- Fix daemon connection not visible to bubbletea model copy
- Fix ghost artifact rendering when completing/dismissing todos in expanded view
- Fix ring buffer merge to preserve StartedAt on llm.finished updates
- Fix `?` key hiding command center instead of showing help overlay
- Fix `~` hotkey when plugin is in text input mode
- Fix console overlay detail scroll clamping
- Fix add-todo setting invalid 'active' status instead of 'backlog'
- Fix elapsed time display: use FinishedAt for completed agents
- Fix SendUserMessage for -p mode sessions using io.Pipe stdin
- Fix Google re-auth action labels to prevent accidental credential replacement
- Clean up orphaned agent rows on daemon restart

### Testing & Quality

- Add shared test infrastructure: view helpers, MockRunner, daemon helpers
- Add view-level tests for all plugins (command center, PRs, sessions, settings, TUI host)
- Add view-level regression tests for BUG-113 through BUG-123
- Add budget widget view tests
- Add agent-dependent view tests using MockRunner
- Spec audit: 69% coverage baseline, 19 contradictions resolved, 142 behavioral gaps documented and resolved
- Add specs for agent subsystem, daemon, console, PRs, unified sessions, Slack extraction

### Housekeeping

- Pre-push cleanup: scrub personal references from docs and code comments
- Update project structure in CLAUDE.md (11 missing directories added)
- Add Prerequisites, Quick Start, and Build Commands to README
- Align .gitignore with worktree convention
- Add .specs marker file for spec-driven development
- Add process-tree walk to shell hook for agent-spawned terminal detection

## v0.1.0 (2026-03-14)

Initial release. Terminal dashboard with command center, sessions, settings, external plugin system, and data connectors for Google Calendar, GitHub, Gmail, Slack, and Granola.
