# SPEC: Command Center Plugin (built-in)

## Purpose

The main productivity hub plugin. Manages todos, threads, calendar events, AI-powered suggestions, and Claude integration. Provides two routes: the command center view (calendar + todos) and the threads view.

## Slug: `commandcenter`

## Routes

- `commandcenter` — default view (calendar + todo panels)
- `commandcenter/threads` — threads sub-view (active + paused threads)

## File Organization

| File | Responsibility |
|------|---------------|
| `commandcenter.go` | Main plugin struct, Init, NavigateTo, HandleMessage, Refresh, state management |
| `cc_keys.go` | All key handling: `HandleKey`, sub-handlers for command tab, threads tab, detail view, rich todo creation, text input, booking mode |
| `cc_messages.go` | Message handling for async results (Claude responses, refresh finished, DB writes) |
| `cc_view.go` | Command center rendering: calendar panel, todo panel, warnings, suggestions, help overlay, detail view, booking UI |
| `threads_view.go` | Threads tab: active/paused sections with type prefixes |
| `styles.go` | Local style/gradient types populated from `config.Palette` (avoids circular imports with tui) |
| `refresh.go` | Background refresh command (finds and spawns `ccc-refresh` binary) |
| `claude.go` | Background Claude CLI/LLM commands (edit, enrich, command, focus), prompt builders |

**Related refresh files** (in `internal/refresh/`):

| File | Responsibility |
|------|---------------|
| `todo_agent.go` | `GenerateTodoPrompt` — LLM-based project routing and prompt generation for todos |
| `llm.go` | `generateSuggestions`, `generateProposedPrompts`, `loadPathContext` — orchestrates path metadata, skills, and routing rules into LLM calls |

## State

- `cc *db.CommandCenter` — loaded from DB, contains todos, threads, calendar, suggestions
- `ccCursor int` — selected todo index in command tab
- `threadCursor int` — selected thread index in threads tab
- `subView string` — active sub-view: `"command"` or `"threads"`
- `showHelp bool` — help overlay toggle
- `showBacklog bool` — show/hide completed todos
- `detailView bool` — viewing a single todo's detail with edit input
- `detailNotice string` — transient notice banner in detail view (auto-clears after 1s)
- `addingTodoRich bool` — rich textarea for AI-powered todo creation
- `addingThread bool` — text input for adding a new thread
- `bookingMode bool` — calendar event booking flow
- `ccExpanded bool` — expanded multi-column todo view
- `triageFilter string` — active triage filter tab in expanded view (default: "accepted")
- `undoStack []undoEntry` — stack of undo-able todo actions
- `pendingLaunchTodo *db.Todo` — todo awaiting session navigation

## Key Bindings

### Command Center Tab

| Key | Context | Description |
|-----|---------|-------------|
| `up`/`k` | normal | Move cursor up |
| `down`/`j` | normal | Move cursor down |
| `shift+up` | normal | Swap todo with the one above |
| `shift+down` | normal | Swap todo with the one below |
| `left`/`h` | expanded | Move cursor left; paginates to previous page at left edge |
| `right`/`l` | expanded | Move cursor right; paginates to next page at right edge |
| `x` | normal | Complete selected todo (pushes to undo stack) |
| `X` | normal | Dismiss selected todo (pushes to undo stack) |
| `u` | normal | Undo last complete/dismiss |
| `d` | normal | Defer selected todo to bottom of list |
| `p` | normal | Promote selected todo to top of list |
| `space` | normal | Open detail view with edit text input |
| `c` | normal | Create todo via rich textarea (AI-powered) |
| `/` | normal | Search/filter todos (case insensitive) |
| `b` | normal | Toggle backlog (completed items) |
| `s` | normal | Enter booking mode for selected todo |
| `r` | normal | Manual refresh (spawns ccc-refresh) |
| `enter` | normal | Open detail view for selected todo |
| `o` | normal | Launch session for todo (by session_id, project_dir, or navigate to sessions) |
| `?` | any | Toggle help overlay |
| `tab` | expanded | Cycle triage filter forward |
| `shift+tab` | expanded | Cycle triage filter backward |
| `y` | expanded | Accept selected todo (triage) |
| `Y` | expanded | Accept + open task runner for selected todo |
| `esc` | expanded | Collapse expanded view |
| `esc` | pending launch | Cancel pending launch, return to command view |

### Detail View

Title bar shows "TODO #N" using the todo's `display_id`.

The detail view tracks the todo by **ID** (not list index), so status changes (e.g. cycling active → waiting) don't cause the view to jump to a different todo.

Editable fields are cycled with `tab`/`shift+tab`: Status (0), Due (1), ProjectDir (2), Prompt (3).

| Key | Context | Description |
|-----|---------|-------------|
| `tab` | detail:viewing | Cycle to next editable field |
| `shift+tab` | detail:viewing | Cycle to previous editable field |
| `enter` | detail:viewing | Edit selected field (Status cycles value; Due/ProjectDir/Prompt open text input) |
| `enter` | detail:editing | Confirm field edit |
| `c` | detail:viewing | Open command input to edit todo via Claude LLM |
| `o` | detail:viewing | Open task runner view |
| `j` | detail:viewing | Navigate to next active todo |
| `k` | detail:viewing | Navigate to previous active todo |
| `x` | detail:viewing | Complete todo (shows notice banner, auto-advances after 1s) |
| `X` | detail:viewing | Dismiss todo (shows notice banner, auto-advances after 1s) |
| `esc` | detail:viewing | Return to list |
| `esc` | detail:editing | Cancel field edit |

While a notice banner is showing (1s after complete/dismiss), all keys except `esc` are blocked. After the notice clears, the view auto-advances to the next active todo.

### Rich Todo Creation

| Key | Context | Description |
|-----|---------|-------------|
| `ctrl+d` | rich | Submit text to Claude for processing |
| `esc` | rich | Cancel and return to list |

### Booking Mode

| Key | Context | Description |
|-----|---------|-------------|
| `left`/`h` | booking | Select shorter duration |
| `right`/`l` | booking | Select longer duration |
| `enter` | booking | Confirm booking and trigger refresh |
| `esc` | booking | Cancel booking |

### Threads Tab

| Key | Context | Description |
|-----|---------|-------------|
| `up`/`k` | threads | Move cursor up |
| `down`/`j` | threads | Move cursor down |
| `p` | threads | Pause active thread |
| `s` | threads | Start (resume) paused thread |
| `x` | threads | Close thread |
| `a` | threads | Add new thread (text input) |
| `enter` | threads | Launch session in thread's project_dir |

## Event Bus

- Publishes: `todo.completed`, `todo.dismissed`, `todo.deferred`, `todo.promoted`, `pending.todo`
- Subscribes to lifecycle messages: `TabViewMsg`, `ReturnMsg`, `NotifyMsg`, `LaunchMsg`

## Migrations

None — uses existing `cc_todos`, `cc_threads`, `cc_calendar_cache`, `cc_suggestions`, `cc_pending_actions`, `cc_meta`, `cc_source_sync` tables created by `db.migrateSchema`.

### Display IDs

Todos have a `display_id` column (auto-incrementing integer) for stable, human-readable references. Used in the detail view title ("TODO #N") and anywhere a short identifier is needed.

## Behavior

### Command Center View

1. Left panel: calendar (today's events with times, colors from config)
2. Right panel: todos sorted by sort_order, with status indicators
3. Focus suggestion banner at top when available
4. Warning bar when data is stale or services are unreachable
5. Help overlay toggled with `?`
6. Expanded multi-column view when scrolling past visible todos. Rows per column use `(height - 8) / 2` to maximize vertical space (the extra line accounts for the triage tab bar). Left/right arrows paginate when at column edges. A triage filter tab bar appears below the header.

### Todo Lifecycle

- Create via `c` (rich textarea, `ctrl+d` submits to Claude LLM for structured todo creation)
- Complete with `x` (moves to completed, undo with `u`)
- Dismiss with `X` (tombstoned, never recreated by refresh)
- Defer with `d` (moves to bottom of list)
- Promote with `p` (moves to top of list)
- Detail view with `space` (shows full context, edit input for Claude-powered enrichment)
- Launch with `enter` (resumes session_id, launches in project_dir, or navigates to sessions)

### Triage Workflow

Todos have a `triage_status` field that controls inbox behavior. This allows refresh-created todos to arrive in a "new" inbox for review before appearing in the main working list.

#### Triage Status Values

- `"new"` — unreviewed, appears in the New inbox tab
- `"accepted"` — reviewed and accepted into the working list

#### Defaults

- **Refresh-created todos**: `triage_status = "new"` (set in `mergeTodos` for fresh external todos)
- **Manually created todos**: `triage_status = "accepted"` (set in `AddTodo` and `DBInsertTodo`)
- **Refresh merge**: never overwrites `triage_status` on existing todos (preserves user's triage decisions)

#### Triage Filter Tabs (Expanded View)

When the expanded multi-column view is active, a tab bar appears below the header showing filter categories:

| Tab | Filter Logic |
|-----|-------------|
| Accepted | `triage_status == "accepted"` AND `session_status == ""` |
| New | `triage_status == "new"` |
| Review | `session_status == "review"` |
| Blocked | `session_status == "blocked"` |
| Active | `session_status == "active"` |
| All | All active todos (no filter) |

- **Tab order**: Accepted, New, Review, Blocked, Active, All
- **Default tab**: Accepted
- `tab` cycles filter forward, `shift+tab` cycles backward
- Switching tabs resets cursor and scroll offset to 0
- `y` accepts the selected todo (sets `triage_status = "accepted"`, persists to DB)
- `Y` accepts the selected todo AND opens the task runner (detail view)

#### Normal View Behavior

In the normal (collapsed) view:

- **Todo list** shows only accepted todos with no agent session (`triage_status == "accepted"` AND `session_status == ""`)
- **Triage status bar** appears below the todo list showing counts: `New(N) · Review(N) · Blocked(N) · Active(N)` — only displayed if any count is non-zero

#### Auto-Accept Rules

- **Launching an agent** (`launchOrQueueAgent`) automatically accepts the todo, so it moves out of the "new" filter into the accepted working list
- The old `agentFilterActive` toggle (key `a`) is replaced by the triage tab system

### Thread Lifecycle

- Active threads shown with type prefix (PR, issue, conversation)
- Pause with `p`, start/resume with `s`, close with `x`, add with `a`
- Launch in thread's project_dir with `enter`

### Claude Integration

- `c` key opens rich textarea; `ctrl+d` submits text to Claude LLM for todo creation
- `space` on todo opens detail view with edit input for Claude-powered enrichment
- Focus suggestion auto-refreshes after todo mutations
- All Claude calls run as background `tea.Cmd` (non-blocking)
- Uses `LLM` abstraction layer (not direct CLI calls)

### Data Loading (Lifecycle Messages)

Instead of polling on a timer, the command center uses lifecycle messages to reload data from the DB at the right moments:

- **TabViewMsg:** Reload from DB if stale (>2s since last read)
- **ReturnMsg:** Always reload from DB (returning from a Claude session)
- **NotifyMsg:** Reload from DB (cross-instance notifications)

### Refresh (ccc-refresh)

- Auto-refresh triggers when data is older than a threshold (tick-based)
- Manual refresh via `r` key
- Spawns `ccc-refresh` binary, then reloads from DB
- Refresh binary located next to running executable, then falls back to PATH
- **Incremental sync**: Granola and Slack sources check `cc_source_sync` for their last successful sync time and skip already-processed meetings/messages, reducing LLM calls
- **Deterministic source_ref (Granola)**: Source refs use `{meeting_id}-{sha256(title)[:8]}` instead of LLM-generated values, making deduplication reliable
- **Merge preserves completed todos**: Refresh merge logic preserves completed todos as-is rather than overwriting them with fresh data

### Cross-Plugin Navigation

When a todo has a `project_dir`, pressing enter launches a Claude session there. When a todo has no project_dir, the plugin sets `pendingLaunchTodo` and navigates to the sessions plugin via the host's "navigate" action.

### Agent Sessions

CCC can launch, monitor, and manage headless Claude Code sessions that work on todos in the background. Sessions run as subprocesses with stream-JSON output, allowing CCC to track progress without blocking the UI.

#### Schema Fields on Todo

| Field | Type | Description |
|-------|------|-------------|
| `proposed_prompt` | `string` | The prompt to send to the Claude agent. Editable via task runner wizard. Falls back to `formatTodoContext(todo)` if empty. |
| `session_status` | `string` | Current agent session state. Empty string means no session. |
| `session_summary` | `string` | Summary of agent output after session completes. |
| `session_id` | `string` | Claude session ID for resuming an existing interactive session (predates headless agent sessions). |

#### Session Status Values

| Status | Meaning |
|--------|---------|
| `""` (empty) | No agent session associated with this todo |
| `"queued"` | Session is waiting to launch (concurrency limit reached) |
| `"active"` | Agent is running |
| `"blocked"` | Agent is waiting for user input (detected via stream-JSON tool_use events) |
| `"review"` | Agent finished successfully (exit code 0), output ready for review |
| `"failed"` | Agent exited with non-zero exit code |

#### Session Lifecycle

1. **Launch or queue**: User presses `enter` in task runner step 3. `launchOrQueueAgent` either starts the session immediately or queues it based on `cfg.Agent.MaxConcurrent` (default 3).
2. **Auto-accept**: Launching/queuing automatically sets the todo's `triage_status` to `"accepted"` so it leaves the "new" inbox.
3. **Process start**: `launchAgent` spawns `claude --print --output-format stream-json --verbose [flags] <prompt>` as a subprocess. The session's `done` channel and exit code are managed by a background goroutine.
4. **Monitoring**: A background goroutine reads stdout line-by-line, parsing stream-JSON events. It detects blocking events (tool_use with `SendUserMessage` or `AskUser`) and updates `sess.Status` to `"blocked"` with the question text.
5. **Tick polling**: `checkAgentProcesses` runs on every UI tick. It checks the `done` channel for finished processes and reads `sess.Status` (protected by mutex) for status changes like `"blocked"`.
6. **Completion**: When the process exits, `onAgentFinished` sets status to `"review"` (exit 0) or `"failed"` (non-zero), extracts a summary from the last ~500 chars of output, and persists both to DB.
7. **Queue drain**: After a session finishes, `onAgentFinished` checks the queue and auto-launches the next `AutoStart` session if capacity is available.
8. **Shutdown cleanup**: `Plugin.Shutdown()` cancels all active sessions to prevent zombie processes.

#### Launch Options

Sessions are configured via the task runner wizard (step 3) with two launch modes:

- **Queue** (`taskRunnerLaunchCursor == 0`): `AutoStart = false` — session launches immediately if under concurrency limit, otherwise queues without auto-start
- **Run Now** (`taskRunnerLaunchCursor == 1`): `AutoStart = true` — session launches immediately or queues with auto-start when capacity frees up

#### CLI Flags

The `claude` command is invoked with:

- `--print` — headless mode (no interactive TUI)
- `--output-format stream-json` — structured output for monitoring
- `--verbose` — detailed output
- `--permission-mode <perm>` — if perm is not "default" (options: "plan", "auto")
- `--max-budget-usd <budget>` — if budget >= $0.50
- `--worktree` — if mode is "worktree"

#### Join/Resume Existing Sessions

From the detail view or list view, pressing `o` on a todo with a `session_id` launches an interactive session with `resume_id` (not a headless agent — this resumes a previous interactive Claude session). If no `session_id` exists, the task runner wizard opens instead.

#### Review Completed Sessions

Completed sessions (`session_status == "review"` or `"failed"`) show:

- **In the todo list**: styled status indicator (`● ready for review` in green, or `⏳ queued` in muted)
- **In the detail view**: a session status indicator (`● Session: running`, `● Session: completed`, `● Session: failed`) and a `SESSION SUMMARY` section with wrapped output text
- **In the expanded view triage tabs**: the "Review" and "Blocked" tabs filter todos by `session_status`

#### Status Indicators in Todo List

| Status | Indicator | Color |
|--------|-----------|-------|
| `active` | `● agent working` | Cyan |
| `blocked` | `● needs input` | Yellow |
| `review` | `● ready for review` | Green |
| `queued` | `⏳ queued` | Muted |

An agent status header line also appears when sessions are running: `"2/3 agents running, 1 queued"`.

#### Concurrency Management

- `cfg.Agent.MaxConcurrent` controls the max number of simultaneous sessions (default 3)
- `canLaunchAgent()` checks `len(activeSessions) < maxConcurrent`
- Excess sessions are pushed to `sessionQueue` with status `"queued"`
- Queue is drained FIFO as sessions complete

#### Event Bus Integration

- `agent.started` — published when a session begins running
- `agent.queued` — published when a session is added to the queue
- `agent.blocked` — published when stream-JSON detects a blocking event (includes `question` in payload)
- `agent.completed` — published when a session finishes (includes `exit_code` and `status`)

#### Stream-JSON Monitoring

The background goroutine parses each stdout line as JSON. It detects blocking events by checking:

1. Top-level events with `type == "tool_use"` and `name` of `"SendUserMessage"` or `"AskUser"`
2. `type == "assistant"` events containing `content` blocks with tool_use entries matching the same names

When a blocking event is detected, the question text is extracted from `input.message` or `input.question` fields. The goroutine updates `sess.Status` and `sess.Question` under a mutex. The main UI thread picks up the change on the next tick via `checkAgentProcesses`.

### Todo-Agent Prompt Generation Pipeline

During refresh, the system generates `proposed_prompt` values and assigns `project_dir` for eligible todos (active, has a source other than "manual", no prompt yet). This pipeline runs in `generateProposedPrompts` within the refresh cycle.

#### Path Context Assembly (`loadPathContext`)

1. Load all learned paths with descriptions from `cc_learned_paths` via `DBLoadPathsFull`
2. Load routing rules from `~/.config/ccc/routing-rules.yaml` via `LoadRoutingRules`
3. Load global skills from `~/.claude/skills/*/SKILL.md` via `GetGlobalSkills` (cached, 1hr TTL)
4. For each learned path, load project-specific skills from `<path>/.claude/skills/*/SKILL.md` via `GetProjectSkills` (cached, 1hr TTL)
5. Attach routing rules to paths where a match exists
6. Assemble into `PathContext` struct: `Paths []PathWithMeta` + `GlobalSkills []SkillInfo`
7. Errors at any step are logged but not fatal — the pipeline works with partial context

#### Routing Prompt (`buildRoutingPrompt`)

For each eligible todo, builds a prompt containing:

1. **Task section** — todo title, detail, context, source, who_waiting, due date
2. **Available Projects section** — for each path: path, description, project skills (name + description), routing preferences (use_for / not_for)
3. **Global Skills section** — skills available in all projects, with a note not to prefer a project just because it shares global skills
4. **Instructions** — choose best project, generate an actionable prompt in imperative mood, include context, mention who is waiting, suggest what "done" looks like

The LLM returns JSON: `{"project_dir": "...", "proposed_prompt": "...", "reasoning": "..."}`

#### Fallback (Legacy Batch Mode)

If no learned paths exist (empty `PathContext.Paths`), falls back to `generateProposedPromptsLegacy`, which batches all eligible todos into a single LLM call that returns prompt-only results (no project assignment). Returns a map of `{todo_id: prompt_string}`.

#### Types

- `PathContext` — `Paths []PathWithMeta` + `GlobalSkills []SkillInfo`
- `PathWithMeta` — path, description, skills (per-project), routing_rules (optional)
- `TodoPromptResult` — project_dir, proposed_prompt, reasoning

### Task Runner Wizard

The task runner is a 3-step linear wizard for configuring and launching a Claude agent session on a todo. Accessed via `o` from the detail view or `Y` from triage.

#### Steps

1. **Project** (Step 1/3) — Shows the current project directory. `/` opens a scrollable path picker to change it. `enter` accepts and advances. `esc` exits the wizard.
2. **Mode** (Step 2/3) — Shows a reminder of the selected project. Inline mode selector cycles through Normal / Worktree / Sandbox with `←→`. `enter` advances. `esc` goes back to step 1.
3. **Prompt** (Step 3/3) — Shows project + mode reminder. Scrollable prompt viewport (`j/k` to scroll). Launch selector at bottom: `[ Queue ] Run Now` toggled with `←→`. `enter` launches. `esc` goes back to step 2.

#### Defaults

- **Budget**: $5 (hardcoded)
- **Permission**: "auto"
- **Launch cursor**: 0 (Queue)

#### Key Bindings (Step 3)

| Key | Description |
|-----|-------------|
| `j`/`k` | Scroll prompt viewport |
| `←`/`→` | Toggle launch cursor (Queue / Run Now) |
| `enter` | Launch agent with selected options |
| `e` | Open prompt in external editor |
| `c` | AI prompt refinement (LLM improves prompt clarity and structure) |
| `r`/`p` | Review loop (Plannotator annotation → LLM revision cycle) |
| `esc` | Back to step 2 |

#### AI Prompt Refinement (`c`)

1. Sets `taskRunnerRefining = true` (shows spinner in UI)
2. Sends current prompt to LLM asking it to improve clarity, structure, and actionability
3. On response: updates prompt viewport, persists to DB, flashes "Prompt refined", clears spinner

#### Review Loop (`r`)

1. Stores current prompt as clean baseline
2. Opens Plannotator with prompt for user annotation
3. On return:
   - If unchanged → "Prompt approved" flash, done
   - If annotated → sends original + annotated to LLM to address feedback, sets refining spinner
4. On LLM response: updates prompt, stores as new clean baseline, reopens Plannotator (loop continues)
5. Loop repeats until user approves (makes no changes)

#### Path Picker

Reused from previous implementation. `/` opens picker, type to filter, `j/k` or `↑/↓` to navigate, `enter` to select, `esc` to cancel.

## Test Cases

- Slug and tab name are correct
- Routes returns both routes
- Init loads command center data from DB
- Navigation (up/down) moves cursor correctly
- Complete todo updates status and pushes undo entry
- Dismiss todo (X) updates status and pushes undo entry
- Undo (u) restores previous state from undo stack
- Create todo (c) enters rich mode
- Enter on todo with session_id returns launch action with resume_id
- Enter on todo with project_dir returns launch action
- Enter on todo without project_dir navigates to sessions
- Sub-view switching between command and threads
- Thread navigation works independently
- Thread pause/start/close operations
- Defer (d) moves todo to bottom
- Promote (p) moves todo to top
- Shift+up/down swaps todo with neighbor, persists via DB sort_order swap (transaction-based)
- Toggle backlog (b) shows/hides completed items
- Booking mode enter/exit and duration selection
- View renders without panic (with and without data)
- Help overlay toggles
- HandleMessage processes async results
- Add thread creates new thread
- Close thread updates status
- Expanded view navigation (left/right columns)
- Expanded view left/right paginates at column edges
- Detail view shows "TODO #N" title with display_id
- Detail view tracks todo by ID (not index) — status changes don't jump to different todo
- Detail view `enter` edits selected field (Status cycles, others open text input)
- Detail view `x` completes todo with notice banner, auto-advances after 1s
- Detail view `X` dismisses todo with notice banner, auto-advances after 1s
- Detail view `j`/`k` navigates between active todos
- Detail view blocks keys (except esc) while notice banner is showing
- Granola/Slack incremental sync skips already-processed items via `cc_source_sync`
- Granola source_ref is deterministic (`{meeting_id}-{sha256(title)[:8]}`)
- Refresh merge preserves completed todos
- `DBSwapPathOrder` and `DBSwapTodoOrder` use transactions for atomicity
- Triage: refresh-created todos default to triage_status "new"
- Triage: manually created todos default to triage_status "accepted"
- Triage: normal view shows only accepted todos with no session_status
- Triage: tab/shift+tab cycles triage filter in expanded view
- Triage: y accepts a todo, Y accepts + opens task runner
- Triage: launching agent auto-accepts the todo
- Triage: refresh merge preserves existing triage_status
- Task runner wizard: enter advances steps (1→2→3), esc goes back (3→2→1)
- Task runner wizard: esc at step 1 exits wizard
- Task runner wizard: left/right cycles mode in step 2
- Task runner wizard: enter at step 3 launches with queue (cursor 0) or run now (cursor 1)
- Task runner wizard: `c` sets refining state, LLM response updates prompt
- Task runner wizard: `r` opens review loop, unchanged prompt = approved
- Task runner wizard: `r` annotated prompt triggers LLM revision and reopens Plannotator
- Agent sessions: launching sets session_status to "active" and auto-accepts the todo
- Agent sessions: queuing sets session_status to "queued" when at max concurrency
- Agent sessions: stream-JSON blocking event sets session_status to "blocked" with question text
- Agent sessions: successful completion (exit 0) sets session_status to "review" with summary
- Agent sessions: failed completion (non-zero exit) sets session_status to "failed" with summary
- Agent sessions: queue drains FIFO — next AutoStart session launches when capacity frees
- Agent sessions: `o` on todo with session_id returns launch action with resume_id
- Agent sessions: `o` on todo without session_id opens task runner wizard
- Agent sessions: checkAgentProcesses detects finished sessions via done channel on tick
- Agent sessions: checkAgentProcesses detects blocked status change on tick
- Agent sessions: Shutdown cancels all active sessions
- Agent sessions: status indicators render correctly in todo list (active/blocked/review/queued)
- Agent sessions: detail view shows session status and summary sections
- Agent sessions: triage "Review" tab filters todos with session_status "review"
- Agent sessions: triage "Blocked" tab filters todos with session_status "blocked"
- Agent sessions: triage "Active" tab filters todos with session_status "active"
- Agent sessions: normal view excludes todos with non-empty session_status from accepted list
- Agent sessions: concurrency respects cfg.Agent.MaxConcurrent (default 3)
- Todo-agent pipeline: eligible todos are active, have a source != "manual", and no proposed_prompt
- Todo-agent pipeline: with learned paths, calls GenerateTodoPrompt per todo (sets project_dir + proposed_prompt)
- Todo-agent pipeline: without learned paths, falls back to legacy batch prompt (prompt-only, no project_dir)
- Todo-agent pipeline: loadPathContext assembles path descriptions, project skills, global skills, and routing rules
- Todo-agent pipeline: partial context failures (missing skills, missing rules) are logged but don't block other paths
- Todo-agent pipeline: LLM parse failure for one todo is logged and skipped, other todos still processed
