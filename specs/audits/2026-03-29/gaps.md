# Spec Audit Gaps — 2026-03-29

All UNCOVERED-BEHAVIORAL and CONTRADICTS findings, sorted by severity.

---

## Contradictions (19 total)

### Threads removal — affects db.md, host.md, settings.md, refresh.md

The Threads feature was removed from code but specs still reference it extensively:

- `specs/core/db.md` — lists cc_threads table, Thread Operations section, ActiveThreads/PausedThreads views, in-memory thread mutations
- `specs/core/host.md` — Tab table lists "Threads" tab
- `specs/core/datasource.md` — combineResults references "Threads: concatenates" but code concatenates PullRequests
- `specs/builtin/settings.md` — PLUGINS category lists "Threads"

**Resolution:** Remove all Threads references from specs. Replace with PRs/Sessions where applicable.

---

### PR merge strategy — refresh.md

- **Spec:** "Merge-based upsert...each fresh PR upserted by ID...agent tracking columns preserved...missing PRs archived."
- **Code:** `PullRequests: fresh.PullRequests` (full replacement)
- **Impact:** Agent tracking data (agent_status, agent_session_id, etc.) is lost on every refresh

**Resolution:** Either update spec to match code, or fix code to preserve agent columns.

---

### New todo status — refresh.md

- **Spec:** "new items get generated IDs and 'active' status"
- **Code:** `Status: "new"`
- **Impact:** Status lifecycle starts differently than documented

**Resolution:** Update spec to say "new" status.

---

### Triage filter tabs — command-center.md

- **Spec:** Accepted/New/Review/Blocked/Active/All
- **Code:** todo/inbox/agents/review/all
- **Impact:** Tab names, filter logic, and navigation all differ

**Resolution:** Rewrite Triage Workflow section to match current code.

---

### Command center migrations — command-center.md

- **Spec:** "None — uses existing tables"
- **Code:** 2 migrations (index on cc_todos + session_log_path column)

**Resolution:** Document the migrations.

---

### Command center routes — command-center.md

- **Spec:** "Routes returns both routes"
- **Code:** Returns 1 route

**Resolution:** Update spec route count.

---

### DB schema table list — db.md

- **Spec:** 8 tables listed
- **Code:** ~16 tables (threads dropped, many added)

**Resolution:** Update table inventory.

---

### LoadCommandCenterFromDB return shape — db.md

- **Spec:** calendar, todos, threads, suggestions, pending actions, warnings, generated_at
- **Code:** loads PRs and merges instead of threads/warnings

**Resolution:** Update spec to match current fields.

---

### RoutingRule fields — db.md

- **Spec:** use_for and not_for string lists
- **Code:** also has PromptHint string

**Resolution:** Add PromptHint to spec.

---

### Event.Payload type — event-bus.md

- **Spec:** `map[string]interface{}`
- **Code:** `any`

**Resolution:** Update spec to `any`.

---

### ReturnMsg fields — lifecycle.md

- **Spec:** empty struct
- **Code:** `TodoID string`, `WasResumeJoin bool`

**Resolution:** Document the fields.

---

### Default config name — config.md

- **Spec:** "Command Center"
- **Code:** "Claude Command"

**Resolution:** Update spec.

---

### PrepareWorktree parameter — worktree.md

- **Spec:** `repoRoot`
- **Code:** `dir` (any path, internally resolves)

**Resolution:** Update spec parameter name and note auto-resolution.

---

### WorktreeInfo.CreatedAt source — worktree.md

- **Spec:** Parsed from branch name
- **Code:** Uses file mtime

**Resolution:** Update spec.

---

### PR Enter key behavior — prs.md

- **Spec:** "Agent failed: Resume session to see what went wrong"
- **Code:** Requires local repo dir, silently no-ops if missing

**Resolution:** Document the local-repo requirement and no-op fallback.

---

### TUI esc behavior — host.md

- **Spec:** Double-esc to quit
- **Code:** Single-esc quits

**Resolution:** Update spec.

---

### Host typed plugin references — host.md

- **Spec:** "no direct references to specific plugins"
- **Code:** Directly references commandcenter and sessions plugins by type

**Resolution:** Document the typed references or abstract them.

---

### Blocked session rendering — sessions.md

- **Code:** Yellow dot + "Blocked" text
- **Spec:** Only mentions blocked in tier table, no rendering detail

**Resolution:** Add rendering behavior to spec.

---

### ActiveTodos vs VisibleTodos — db.md

- **Spec:** "ActiveTodos" is a "filtered/sorted view"
- **Code:** Uses `VisibleTodos()` which also filters merge-hidden todos

**Resolution:** Document merge-based filtering.

---

## Behavioral Gaps by Cluster

### Cluster: CLI subcommands (cmd/ — 20 gaps)

Undocumented subcommands added since cli.md was written:
- `ccc daemon start|stop|status|logs`
- `ccc register`
- `ccc update-session`
- `ccc stop-all`
- `ccc add-todo`
- `ccc add-bookmark`
- `ccc todo --get`
- `ccc paths --auto-describe|--list`
- `ccc worktrees list|prune`

### Cluster: Database operations (internal/db/ — 25 gaps)

Major undocumented DB operations:
- Todo merges: `DBLoadMerges`, `WerePreviouslyMergedAndVetoed`, `DBGetOriginalIDs`
- Ignored repos/PRs: `DBLoadIgnoredRepos`, `DBLoadIgnoredPRs`, `DBAddIgnoredRepo`, `DBRemoveIgnoredRepo`
- Agent costs: `DBInsertAgentCost`, `DBUpdateAgentCostFinished`, `DBSumCostsSince`, `DBCountLaunchesSince`, `DBLastAgentLaunch`, `DBCountRecentFailures`, `DBGetBudgetState`, `DBSetBudgetState`
- Archived sessions: `DBLoadArchivedSessions`
- Source sync: `DBLoadSourceSync`, `DBLoadAllSourceSync`
- Visible sessions: `DBLoadVisibleSessions`
- Session records: `DBLoadSessions`/`SessionRecord`

### Cluster: Data refresh pipeline (internal/refresh/ — 24 gaps)

- Source context: TTL behavior, FetchContextBestEffort fire-and-forget pattern
- Slack: search query construction (search.messages API), channel-based context fetching
- Gmail: label-based todo workflows, GetThread/GetMessageBody/ModifyLabels client methods
- Granola: speaker attribution in context, meeting transcript structure
- Todo synthesis: WerePreviouslyMergedAndVetoed dedup, BuildSynthesisTodo assembly
- Todo agent: routing prompt generation, path context assembly, learned paths

### Cluster: TUI host layer (internal/tui/ — 19 gaps)

- Daemon integration: auto-start, DaemonConn lifecycle, reconnect, DaemonEventMsg routing
- Budget widget in banner
- Flash messages system
- Keyboard: Ctrl+Z (background), Ctrl+X (quit), double-esc behavior
- Tab dispatch: plugin HandleKey called before host-level bindings
- Stub plugins for disabled plugins
- Onboarding: skills installation, shell hook installation
- Launch: dir validation, worktree launch, session dir resolution

### Cluster: Command center evolution (internal/builtin/commandcenter/ — 16 gaps)

- Undocumented keybindings: `t` (quick todo add), `T` (train/routing), `U` (unmerge), `g` (chord prefix)
- Wizard: selection persistence, AI prompt refinement response handling
- Agent sessions: edit guards, SIGINT before resume, clarifying question UX
- Merge/synthesis: auto-detection in Refresh, display of merged items

### Cluster: Config struct expansion (internal/config/ — 16 gaps)

- New config sections: AgentConfig, DaemonConfig, RefreshConfig, DisabledPlugins
- Config.Save() safety (atomic write semantics)
- Shell hook: IsShellHookInstalled, InstallShellHook, UninstallShellHook
- Skills: SkillNames, IsSkillInstalled, InstallSkills, UninstallSkills
- MCP: IsMCPBuilt, GenerateMCPConfig, BuildAndConfigureMCP
- Validate: placeholder detection for client IDs

### Cluster: Plugin framework (internal/plugin+external/ — 10 gaps)

- ScopeConfig inclusion logic
- DoctorProvider/DoctorCheck types and interface
- External plugin error state UI rendering
- Loader skip-on-duplicate behavior
- Logger Recent() ring buffer semantics

### Cluster: Settings panes (internal/builtin/settings/ — 8 gaps)

- 3 undocumented panes: Sandbox, Automations, PRs
- RegisterProvider API for external settings providers
- Credential reuse optimization in auth forms
- Todo event subscriptions for pane updates
- datasource.toggled event emission

### Cluster: Agent runner details (internal/agent/ — 7 gaps)

- Max output capture (100KB ring buffer)
- Session event channel pattern (buffered channel + goroutine)
- Log directory structure and cleanup
- Native log path resolution for IDE integration
- QueueLen/DrainQueue queue management

### Cluster: Sessions plugin (internal/builtin/sessions/ — 5 gaps)

- LLMDescribePath for path descriptions
- Browse flow (fzf selection + auto-add + LLM description)
- Daemon archive RPC on session dismiss
- NavigateTo args (pending_todo_title)
- Blocked session rendering details

### Cluster: Automation runner (internal/automation/ — 4 gaps)

- Log rotation (directory creation)
- LogPath field on RunResult
- Init-phase log messages
- Process group kill on timeout

### Cluster: Worktree (internal/worktree/ — 3 gaps)

- Symlink resolution in list/remove/gitRepoRoot
- Pre-existing symlink target handling in PrepareWorktree
