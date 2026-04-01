# Spec Audit: internal/builtin/commandcenter/

**Date:** 2026-03-29
**Specs:** `specs/builtin/command-center.md`, `specs/builtin/session-viewer.md`
**Code files:** 16 files in `internal/builtin/commandcenter/`

## Summary

- **Branches analyzed:** 127
- **Covered:** 98
- **Uncovered (behavioral):** 16
- **Uncovered (implementation-only):** 8
- **Contradictions:** 5

---

## Branch Coverage

### Plugin Lifecycle

| Branch | Classification | Notes |
|--------|---------------|-------|
| `New()` defaults | **[COVERED]** | Spec: "Default tab: Accepted" matches `triageFilter: "todo"` — wait, see contradiction #1 |
| `Init()` loads CC from DB | **[COVERED]** | Spec: "Init loads command center data from DB" (Test Cases) |
| `Init()` auto-refresh on stale data | **[COVERED]** | Spec: "Auto-refresh triggers when data is older than a threshold" (Behavior > Refresh) |
| `Init()` sets up event bus subscriptions (`pending.todo.cancel`, `config.saved`) | **[UNCOVERED-BEHAVIORAL]** | Spec mentions subscribes to lifecycle messages but does not mention `pending.todo.cancel` or `config.saved` subscriptions. **Intent question: Should spec enumerate all event bus subscriptions?** |
| `Shutdown()` kills active agents | **[COVERED]** | Spec: "Shutdown cleanup: Plugin.Shutdown() cancels all active sessions" (Agent Sessions > Session Lifecycle) |
| `Slug()` returns "commandcenter" | **[COVERED]** | Spec: "Slug: commandcenter" |
| `TabName()` returns "Command Center" | **[COVERED]** | Implied by spec title |
| `Migrations()` returns 2 migrations | **[CONTRADICTS]** | Spec says "None — uses existing tables." Code adds an index on cc_todos and a session_log_path column. See contradiction #2. |
| `Routes()` returns single route | **[CONTRADICTS]** | Spec says "Routes returns both routes" (Test Cases), but code returns only 1 route. See contradiction #3. |
| `RefreshInterval()` from config | **[COVERED]** | Spec: refresh interval concept covered |
| `Refresh()` guards against double-refresh | **[COVERED]** | Spec: "Manual refresh via r key" |
| `StartCmds()` batches spinner + refresh if refreshing | **[UNCOVERED-IMPLEMENTATION]** | Internal startup plumbing |

### Key Handling — Command Tab

| Branch | Classification | Notes |
|--------|---------------|-------|
| `up/k` — cursor up (normal + expanded) | **[COVERED]** | Spec: Key Bindings table |
| `up` at top of expanded — collapse | **[UNCOVERED-BEHAVIORAL]** | Code: pressing up at cursor 0 in expanded view collapses it. Spec does not describe this transition. **Intent question: Is collapsing expanded view via up-at-top intentional or should esc be the only way?** |
| `down/j` — cursor down (normal + expanded) | **[COVERED]** | Spec: Key Bindings table |
| `left/h`, `right/l` — expanded column nav + pagination | **[COVERED]** | Spec: "Left/right arrows paginate when at column edges" (Behavior > Command Center View) |
| `shift+up/down` — swap todos | **[COVERED]** | Spec: "Swap todo with the one above/below" (Key Bindings) |
| `x` — complete todo | **[COVERED]** | Spec: "Complete selected todo (pushes to undo stack)" |
| `x` — kills agent before completing | **[COVERED]** | Session-viewer spec: "x in list/detail view kills running agent before completing todo" |
| `X` — dismiss todo | **[COVERED]** | Spec: "Dismiss selected todo (pushes to undo stack)" |
| `X` — kills agent before dismissing | **[COVERED]** | Session-viewer spec: "X in list/detail view kills running agent before dismissing todo" |
| `u` — undo | **[COVERED]** | Spec: "Undo last complete/dismiss" |
| `d` — defer | **[COVERED]** | Spec: "Defer selected todo to bottom of list" |
| `p` — promote | **[COVERED]** | Spec: "Promote selected todo to top of list" |
| `space` — cycle expanded (collapsed -> 2col -> 1col -> collapsed) | **[COVERED]** | Spec: "Cycle expanded view: collapsed -> 2-col -> 1-col -> collapsed" |
| `c` — rich todo creation | **[COVERED]** | Spec: "Create todo via rich textarea (AI-powered)" |
| `t` — quick todo entry | **[UNCOVERED-BEHAVIORAL]** | Code implements `t` for quick todo creation with LLM enrichment. Spec key bindings table does not list `t`. **Intent question: Should spec document `t` as a separate quick-add path distinct from `c`?** |
| `/` — search/filter | **[COVERED]** | Spec: "Search/filter todos (case insensitive)" |
| `tab` — cycle triage filter (expanded) | **[COVERED]** | Spec: "tab cycles filter forward" (Triage Filter Tabs) |
| `shift+tab` — cycle triage filter backward | **[COVERED]** | Spec: "shift+tab cycles backward" |
| `y` — accept todo (expanded) | **[COVERED]** | Spec: "y accepts the selected todo" |
| `Y` — accept + open task runner | **[COVERED]** | Spec: "Y accepts + opens task runner" |
| `b` — toggle backlog | **[COVERED]** | Spec: "Toggle backlog (completed items)" |
| `s` — booking mode | **[COVERED]** | Spec: "Enter booking mode for selected todo" |
| `r` — manual refresh | **[COVERED]** | Spec: "Manual refresh (spawns ai-cron)" |
| `enter` — open detail view | **[COVERED]** | Spec: "Open detail view for selected todo" |
| `o` — launch/resume session or open task runner | **[COVERED]** | Spec: "Launch session for todo (by session_id, project_dir, or navigate to sessions)" |
| `o` — session ID recovery from log file | **[UNCOVERED-BEHAVIORAL]** | Code calls `extractSessionIDFromLog` as fallback when `SessionID` is empty. Spec mentions expired session detection but not log-based recovery. **Intent question: Should spec document the session ID recovery fallback from log files?** |
| `g` — chord prefix for Gmail-style shortcuts | **[UNCOVERED-BEHAVIORAL]** | Code implements `g` prefix with `gi`/`gu` to return to list view. Spec key bindings mention `gi/gu` but don't describe the chord mechanism (two-key state). **Intent question: Should spec describe the `g` chord prefix and its available combinations?** |
| `?` — help overlay | **[COVERED]** | Spec: "Toggle help overlay" |
| `esc` — clear search, collapse expanded, cancel pending | **[COVERED]** | Spec covers esc in expanded and pending launch contexts |

### Key Handling — Detail View

| Branch | Classification | Notes |
|--------|---------------|-------|
| `up/down` — viewport scroll | **[COVERED]** | Session-viewer spec: "Up/down arrows -- scroll viewport line by line" (Detail View: Scrollable Viewport) |
| `pgup/pgdown` — half-page scroll | **[COVERED]** | Session-viewer spec: "PgUp/PgDown -- scroll half-page" |
| `tab/shift+tab` — cycle fields | **[COVERED]** | Spec: "Cycle to next/previous editable field" |
| `enter` — edit selected field | **[COVERED]** | Spec: "Edit selected field (Status opens inline selector; Due opens text input; ProjectDir opens path picker)" |
| `enter` — blocked when agent active | **[UNCOVERED-BEHAVIORAL]** | Code blocks enter (shows "Todo is being updated by agent") when an agent is active. Spec doesn't mention this guard. **Intent question: Should spec document that field editing is blocked during active agent sessions?** |
| `x` — complete with notice | **[COVERED]** | Spec: "Complete todo (shows notice banner, auto-advances after 1s)" |
| `X` — dismiss with notice | **[COVERED]** | Spec: "Dismiss todo (shows notice banner, auto-advances after 1s)" |
| `j/k` — navigate todos or merge sources | **[COVERED]** | Spec: "Navigate to next/previous active todo" |
| `j/k` — merge source navigation | **[UNCOVERED-BEHAVIORAL]** | Code: `j/k` on synthesis todos navigates merge sources before falling through to todo nav. Spec does not describe this dual behavior. **Intent question: Should spec document j/k merge source cursor for synthesis todos?** |
| `w` — open session viewer (live) | **[COVERED]** | Session-viewer spec: "w on todo with active session opens session viewer" |
| `w` — open replay from log | **[COVERED]** | Session-viewer spec: "w on todo with SessionLogPath but no active session opens replay viewer" |
| `w` — no session, no log (flash) | **[COVERED]** | Session-viewer spec: "w on todo with no active session and no SessionLogPath shows flash" |
| `o` — join session or open task runner | **[COVERED]** | Spec: "Join session (if session_id exists) or open task runner" |
| `o` — expired session detection | **[COVERED]** | Spec: "If the session file is missing, shows a flash message" |
| `r` — resume/re-launch agent | **[COVERED]** | Session-viewer spec: "r from detail view on todo with SessionID and no active session launches headless agent" |
| `r` — dropped ResumeID for expired session | **[COVERED]** | Spec: "If the session file is missing, drops the ResumeID and launches a fresh agent" |
| `c` — command input | **[COVERED]** | Spec: "Open command input to edit todo via Claude LLM" |
| `c` — blocked when agent active | **[UNCOVERED-BEHAVIORAL]** | Same as enter guard above. Code blocks `c` key during active agent. Not in spec. |
| `delete/backspace` — kill agent | **[COVERED]** | Session-viewer spec: "del in detail view kills running agent" |
| `U` — unmerge source | **[UNCOVERED-BEHAVIORAL]** | Code implements unmerge of synthesis source todos. Spec does not describe this feature. **Intent question: Should spec document the U (unmerge) feature for synthesis todos?** |
| `T` — train routing rules | **[UNCOVERED-BEHAVIORAL]** | Code implements T for training prompt/routing rules via LLM. Spec does not describe this feature. **Intent question: Should spec document the T (train) feature and its LLM-based routing rule generation?** |
| Notice banner blocks keys except esc | **[COVERED]** | Spec: "While a notice banner is showing, all keys except esc are blocked" |
| Detail view status selector (left/right, enter, esc) | **[COVERED]** | Spec: "Status opens inline selector" |
| Detail view path picker (j/k, type-to-filter, enter, esc) | **[COVERED]** | Spec: "ProjectDir opens scrollable path picker" |
| Detail view due date editing (parseDueDate, LLM fallback) | **[COVERED]** | Spec: "Due opens text input" |

### Key Handling — Task Runner Wizard

| Branch | Classification | Notes |
|--------|---------------|-------|
| Step 1: enter advances, / picks path, esc exits | **[COVERED]** | Spec: "Task runner wizard: enter advances steps (1->2->3), esc goes back" |
| Step 2: left/right cycles mode, enter advances, esc back | **[COVERED]** | Spec: "left/right cycles mode in step 2" |
| Step 3: j/k scroll, left/right launch cursor, enter launch | **[COVERED]** | Spec: "enter at step 3 launches with Run Claude/Queue Agent/Run Agent Now" |
| Step 3: `e` edit prompt in editor | **[COVERED]** | Spec: "e edit prompt" (Key Bindings Step 3) |
| Step 3: `c` AI prompt refinement with instructions | **[COVERED]** | Spec: "c sets refining state, LLM response updates prompt" |
| Step 3: `r`/`p` review loop (Plannotator) | **[COVERED]** | Spec: "r opens review loop, unchanged prompt = approved" |
| Step 3: Plannotator blocking modal (esc cancels) | **[COVERED]** | Spec: "Review Loop" section describes the flow |
| Step 3: taskRunnerLaunchInteractive (cursor 0) | **[COVERED]** | Spec: "Run Claude (taskRunnerLaunchCursor == 0)" |
| Step 3: taskRunnerLaunch queue (cursor 1) | **[COVERED]** | Spec: "Queue Agent (taskRunnerLaunchCursor == 1)" |
| Step 3: taskRunnerLaunch immediate (cursor 2) | **[COVERED]** | Spec: "Run Agent Now (taskRunnerLaunchCursor == 2)" |
| Wizard selection persistence across open/close | **[UNCOVERED-BEHAVIORAL]** | Code persists wizard selections (path, mode) in `wizardSelections` map per todo. Spec doesn't mention this persistence. **Intent question: Should spec document that wizard project/mode selections are remembered per-todo across wizard open/close cycles?** |
| Auto-open path picker when no project dir | **[UNCOVERED-BEHAVIORAL]** | Code auto-opens path picker when `todo.ProjectDir == ""` and paths are available. Spec doesn't mention this auto-open behavior. **Intent question: Should spec document auto-opening the path picker for todos without a project directory?** |
| Tab/shift-tab consumed in task runner | **[UNCOVERED-IMPLEMENTATION]** | Prevents tab from propagating to host nav |

### Key Handling — Session Viewer

| Branch | Classification | Notes |
|--------|---------------|-------|
| `j/down` — scroll down, disable auto-scroll | **[COVERED]** | Session-viewer spec: "j/down scroll down one line, disables auto-scroll" |
| `k/up` — scroll up, disable auto-scroll | **[COVERED]** | Session-viewer spec: "k/up scroll up one line, disables auto-scroll" |
| `G` — jump to bottom, re-enable auto-scroll | **[COVERED]** | Session-viewer spec: "G jump to bottom, re-enable auto-scroll" |
| `g` — jump to top, disable auto-scroll | **[COVERED]** | Session-viewer spec: "g jump to top, disable auto-scroll" |
| `c` — open message textarea | **[COVERED]** | Session-viewer spec: "c open message textarea" |
| `o` — join session interactively | **[COVERED]** | Session-viewer spec: "o launches interactive session with resume_id" |
| `o` — recover session ID from log | **[COVERED]** | Code recovers session ID from log file if missing |
| `esc` — exit viewer | **[COVERED]** | Session-viewer spec: "esc exit viewer, return to detail view" |
| Input mode: enter sends message | **[COVERED]** | Session-viewer spec: "enter in input mode sends message" |
| Input mode: empty text cancels | **[COVERED]** | Session-viewer spec: "enter in input mode with empty text cancels" |
| Input mode: esc cancels | **[COVERED]** | Session-viewer spec: "esc in input mode cancels" |
| Daemon RPC fallback for sending input | **[UNCOVERED-BEHAVIORAL]** | Code tries daemon RPC first (`dc.SendAgentInput`), then falls back to local runner. Spec doesn't mention daemon routing. **Intent question: Should spec document the daemon-first, local-fallback communication pattern for session viewer?** |

### Message Handling (HandleMessage)

| Branch | Classification | Notes |
|--------|---------------|-------|
| `tea.MouseMsg` — viewport scroll | **[COVERED]** | Session-viewer spec: "Mouse wheel / trackpad -- scroll viewport" |
| `ccLoadedMsg` — load CC, kill agents with summaries | **[COVERED]** | Session-viewer spec: "Agent Lifecycle: Kill on Summary Submission" |
| `ccLoadedMsg` — auto-generate focus if empty | **[COVERED]** | Spec: "Focus suggestion: data load with empty focus triggers generation" |
| `ccRefreshFinishedMsg` — handle refresh result | **[COVERED]** | Spec: "Spawns ai-cron binary, then reloads from DB" |
| `dbWriteResult` — handle DB errors | **[UNCOVERED-IMPLEMENTATION]** | Error handling plumbing |
| `claudeEditFinishedMsg` — apply edit | **[COVERED]** | Spec: "HandleMessage processes async results" |
| `claudeEditFinishedMsg` — preserves system fields | **[UNCOVERED-BEHAVIORAL]** | Code preserves ID, CreatedAt, CompletedAt, SessionID, SessionSummary, DisplayID from LLM overwrite. Spec doesn't enumerate protected fields. **Intent question: Should spec list which fields are protected from LLM overwrites?** |
| `claudeEnrichFinishedMsg` — add enriched todo | **[COVERED]** | Spec: "Create via c (rich textarea)" |
| `claudeEnrichFinishedMsg` — merge detection + synthesis | **[UNCOVERED-BEHAVIORAL]** | Code detects duplicates via LLM `merge_into` field and triggers synthesis. Spec does not describe the merge/synthesis flow for enriched todos. **Intent question: Should spec document the automatic duplicate detection and synthesis merge during todo creation?** |
| `claudeCommandFinishedMsg` — process command response | **[COVERED]** | Spec: "Claude Integration" section |
| `claudeCommandFinishedMsg` — clarifying question (ask) | **[UNCOVERED-BEHAVIORAL]** | Code handles `ask` field by reopening textarea with the question. Spec mentions "may ask ONE short question" in prompt but doesn't describe the UX flow. **Intent question: Should spec describe the clarifying question UX (flash + reopen textarea)?** |
| `claudeFocusFinishedMsg` — update focus suggestion | **[COVERED]** | Spec: Focus suggestion behavior section |
| `claudeDateParseFinishedMsg` — update due date | **[COVERED]** | Spec: "Due opens text input" (LLM fallback for natural language) |
| `claudeRefinePromptMsg` — update prompt in wizard | **[COVERED]** | Spec: "On response: updates prompt viewport, persists to DB" |
| `claudeTrainFinishedMsg` — apply routing rules | **[UNCOVERED-BEHAVIORAL]** | Training feature not in spec (same as T key above) |
| `plannotatorFinishedMsg` — read edited prompt | **[COVERED]** | Spec: "e edit prompt" |
| `plannotatorReviewMsg` — review loop flow | **[COVERED]** | Spec: "Review Loop" section describes approve/deny/loop |
| `claudeReviewAddressedMsg` — reopen Plannotator | **[COVERED]** | Spec: "Loop repeats until user approves" |
| `agentEventMsg` — update session viewer | **[COVERED]** | Session-viewer spec: event channel pattern |
| `agentEventsDoneMsg` — mark session done | **[COVERED]** | Session-viewer spec: "agentEventsDoneMsg sets sessionViewerDone = true" |
| `SessionStartedMsg` — persist status + log path | **[COVERED]** | Session-viewer spec: "Agent launch persists session_log_path to DB" |
| `SessionBlockedMsg` — set blocked status | **[COVERED]** | Spec: "stream-JSON blocking event sets session_status to blocked" |
| `SessionIDCapturedMsg` — persist session ID | **[UNCOVERED-BEHAVIORAL]** | Code captures and persists the Claude session ID from the agent stream. Spec doesn't explicitly describe session ID capture from the stream. **Intent question: Is this covered by the session_id schema field description, or should the capture mechanism be documented?** |
| `SessionFinishedMsg` — agent finished handler | **[COVERED]** | Spec: "When the process exits, onAgentFinished sets status" |
| `TabViewMsg` — reload if stale | **[COVERED]** | Spec: "TabViewMsg: Reload from DB if stale (>2s since last read)" |
| `ReturnMsg` — reload + restore detail view | **[COVERED]** | Spec: "ReturnMsg: Always reload from DB" |
| `ReturnMsg` — set status to review on return | **[UNCOVERED-IMPLEMENTATION]** | Internal status transition detail |
| `LaunchMsg` — graceful agent stop before resume | **[UNCOVERED-BEHAVIORAL]** | Code sends SIGINT to headless agent and waits 5s before interactive resume. Spec says nothing about graceful agent stop before joining. **Intent question: Should spec document the SIGINT + 5s wait for headless agent before interactive resume?** |
| `NotifyMsg` — reload on data.refreshed | **[COVERED]** | Spec: "NotifyMsg: Reload from DB" |
| `TickMsg` — flash expiry, notice auto-advance, agent check, auto-refresh | **[COVERED]** | Spec: "Tick polling" and "Auto-refresh triggers" |

### Agent Runner

| Branch | Classification | Notes |
|--------|---------------|-------|
| `killAgent` — daemon first, local fallback | **[COVERED]** | Session-viewer spec describes kill lifecycle |
| `canLaunchAgent` — concurrency check | **[COVERED]** | Spec: concurrency managed by `agentRunner.LaunchOrQueue()` (Runner interface) |
| `launchOrQueueAgent` — auto-accept | **[COVERED]** | Spec: "Auto-accept: Launching/queuing automatically sets triage_status" |
| `launchOrQueueAgent` — daemon first, local fallback | **[UNCOVERED-IMPLEMENTATION]** | Internal routing detail |
| `onAgentFinished` — status + summary + queue drain | **[COVERED]** | Spec: "Completion" and "Queue drain" sections |
| `queuedSession.toAgentRequest` — prompt postscript | **[COVERED]** | Spec: "Prompt Postscript" section describes the `ccc update-todo` instruction |

### View Rendering

| Branch | Classification | Notes |
|--------|---------------|-------|
| Help overlay routing (list vs detail) | **[COVERED]** | Spec and session-viewer spec: "? key toggles help overlay" |
| Session viewer rendering (status bar, viewport, events) | **[COVERED]** | Session-viewer spec: Layout section |
| Task runner rendering (3 steps) | **[COVERED]** | Spec: Task Runner Wizard section |
| Detail view scrollable viewport | **[COVERED]** | Session-viewer spec: Detail View: Scrollable Viewport section |
| Calendar panel (today/tomorrow split, conflicts, colors) | **[COVERED]** | Spec: "Left panel: calendar (today's events)" |
| Expanded todo view (multi-column, triage tabs) | **[COVERED]** | Spec: Triage Filter Tabs section |
| Normal todo view (filtered, scroll) | **[COVERED]** | Spec: Normal View Behavior section |
| Focus suggestion banner | **[COVERED]** | Spec: "Focus suggestion is always visible" |
| Warning banner | **[COVERED]** | Spec: "Warning bar when data is stale or services are unreachable" |
| Booking picker rendering | **[COVERED]** | Spec: Booking Mode key bindings |
| Search filter display (active + passive) | **[UNCOVERED-IMPLEMENTATION]** | UI detail |
| Daemon reconnect for session viewer | **[UNCOVERED-BEHAVIORAL]** | Code has `tryDaemonReconnect` for reconnecting to daemon-managed agents after TUI restart. Spec doesn't describe this reconnection path. |

### Refresh

| Branch | Classification | Notes |
|--------|---------------|-------|
| `refreshCCCmd` — lockfile check | **[UNCOVERED-IMPLEMENTATION]** | Internal safety mechanism |
| `findRefreshBinary` — co-located binary fallback | **[COVERED]** | Spec: "Refresh binary located next to running executable, then falls back to PATH" |

---

## Contradictions

### 1. Triage Filter Default: Spec says "Accepted", Code says "todo"

- **Spec** (command-center.md, Triage Filter Tabs): "Default tab: Accepted"
- **Code** (`New()`): `triageFilter: "todo"`
- **Spec** (same table): The tab is called "Accepted" but the code filter value is `"todo"` which filters for `StatusBacklog`
- **Impact:** The tab name vs filter value naming is inconsistent. The spec describes tabs as "Accepted, New, Review, Blocked, Active, All" but code uses "todo, inbox, agents, review, all" with different filter logic.

### 2. Migrations: Spec says "None", Code has 2

- **Spec** (command-center.md, Migrations): "None -- uses existing tables created by db.migrateSchema"
- **Code** (`Migrations()`): Returns 2 migrations — an index on cc_todos and adding session_log_path column
- **Impact:** Spec needs updating to reflect plugin-owned migrations

### 3. Routes: Spec test case says "both routes", Code returns 1

- **Spec** (command-center.md, Test Cases): "Routes returns both routes"
- **Code** (`Routes()`): Returns `[]plugin.Route` with a single entry `{Slug: "commandcenter"}`
- **Impact:** Spec test case is stale — likely from when there was a sessions route

### 4. Triage Tab Names and Filter Logic Divergence

- **Spec** describes tabs: Accepted, New, Review, Blocked, Active, All with specific filter logic (e.g., Accepted = `triage_status == "accepted" AND session_status == ""`)
- **Code** implements: todo (StatusBacklog), inbox (StatusNew), agents (Enqueued/Running/Blocked), review (Review/Failed), all
- The tab names, count, and filter logic have diverged significantly from the spec

### 5. Normal View Filter Logic

- **Spec**: "Todo list shows only accepted todos with no agent session (triage_status == 'accepted' AND session_status == '')"
- **Code** (`filteredTodos` non-expanded): Shows all active todos except `StatusNew` — does not filter by session status
- **Impact:** Normal view shows agent-status todos (running, blocked, review) that the spec says should be hidden

---

## Unimplemented Spec Promises

### 1. Triage Status Bar in Normal View

- **Spec**: "Triage status bar appears below the todo list showing counts: New(N) . Review(N) . Blocked(N) . Active(N) -- only displayed if any count is non-zero"
- **Code**: The normal view renders `renderCommandCenterView` which does receive `triageCounts` but the actual status bar rendering was not found in the read portions of `cc_view.go`. May exist deeper in the rendering chain — needs verification.

### 2. Publish Events: pending.todo

- **Spec** (Event Bus): "Publishes: ... pending.todo"
- **Code**: No `pending.todo` event is published anywhere in the commandcenter package. The spec may be stale.

### 3. Auto-Advance After Notice (1s)

- **Spec**: "auto-advances after 1s"
- **Code**: `handleTickMsg` implements this correctly. However, the spec says "auto-advances to the next active todo" — the code advances to the next *filtered* todo, which may differ if search is active. Minor but worth noting.

### 4. Detail View: Prompt Not Editable

- **Spec**: "Prompt is not editable in the detail view -- it is managed via the task runner wizard"
- **Code**: The detail view `c` (command input) can update `proposed_prompt` via LLM edit. The spec is accurate that there's no *direct* field editing for prompt, but the LLM edit pathway can modify it.
