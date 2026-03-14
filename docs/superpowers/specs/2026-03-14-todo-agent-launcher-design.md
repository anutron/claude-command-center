# SPEC: Todo Agent Launcher

## Purpose

Transform the todo detail view into a launchpad for kicking off headless Claude Code agent sessions. Users review AI-generated prompts, configure launch parameters, and explicitly approve each session. Session status is tracked on todos and visible in the list view.

**Hard safety rule**: Todos never auto-launch into headless mode. Every session requires explicit human review and approval.

## Interface

- **Inputs**: Todo with source context (Granola transcript, Slack thread, GitHub issue), user edits to proposed prompt, launch configuration (mode, permission, budget)
- **Outputs**: Headless Claude session running in the target project directory, session status updates on the todo
- **Dependencies**: Claude CLI (`--print --output-format stream-json`), SQLite DB, event bus, LLM for prompt generation

## Data Model Changes

### New columns on `cc_todos`

```sql
proposed_prompt TEXT    -- agent-ready prompt, written by skill/refresh, editable by user
session_status TEXT     -- empty | "queued" | "active" | "blocked" | "review" | "reviewed"
```

### Config additions

```yaml
agent:
  default_budget: 5.00       # USD max per session
  default_permission: default # default | plan | auto
  default_mode: normal        # normal | worktree | sandbox
  max_concurrent: 3           # max simultaneous headless sessions
```

## Behavior

### Todo Detail View (Redesigned)

Opens in **viewing mode** (no auto-focus on text input). Three modes:

| Mode | Entry | Keys | Exit |
|------|-------|------|------|
| viewing | Default on open | tab/shift-tab cycle fields, enter → edit field, o → task runner, c → command input | esc → list |
| editingField | enter on highlighted field | Field-specific widget, enter commits | esc → viewing |
| commandInput | c from viewing | Text input, enter submits to LLM | esc → viewing |

**Layout**: Two-column metadata (left=editable, right=read-only), full-width Detail and truncated Prompt sections below.

**Editable fields** (left column): Status (dropdown), Due (text input, LLM normalizes), Project dir (path picker from `db.DBLoadPaths()`)

**Read-only fields** (right column): Source, Context, Who waiting, Created

**Tab/shift-tab** cycles editable fields without switching the host tab bar. Returns `ConsumedAction`.

### Task Runner View

Accessed via `o` from detail view or todo list. Separate zoom level for launch configuration.

**Shows**: Todo title, project dir, mode selector (Normal/Worktree/Sandbox), permission selector (default/plan/auto), budget ($5 default), queue option (Launch Now / Queue & Auto-start), full scrollable prompt.

**Keys**: enter → launch/queue, tab → cycle selectors, left/right → change option, c → LLM prompt refinement, p → Plannotator annotation loop, esc → back.

### Headless Session Management

**Launch**: `claude --print --output-format stream-json --verbose --input-format stream-json [--worktree] --permission-mode <mode> --max-budget-usd <budget> "<prompt>"`

**Monitoring**: Background goroutine reads stream-JSON events. Detects `SendUserMessage` tool use → "blocked" with question text. Process exit → "review".

**Concurrency**: Max 3 concurrent (configurable). Overflow goes to FIFO queue. Pre-approved tasks auto-start when slots open. Others wait for user approval.

**Status transitions**: queued → active → blocked/review → reviewed

### List View Indicators

Below todo title, colored status text:
- `queued` → dim "queued"
- `active` → cyan "agent working"
- `blocked` → yellow "needs input"
- `review` → green "ready for review"

Header shows: "N/3 agents running, M queued"

`a` key toggles filter to show only agent-status todos.

### Prompt Generation (Trainable Skill)

A `/todo-agent` Claude Code skill generates proposed prompts. Used by both:
1. **Refresh agent**: Auto-generates prompts for source-linked todos without one
2. **Manual invocation**: `/todo-agent <source-ref>` for targeted prompt generation

The skill fetches source context, inlines relevant excerpts (not just links), includes artifact references for attribution, and suggests a project directory.

**Training loop**: User reviews prompts → gives feedback → improves skill via `/write-skill` → next generation is better.

**Merge rule**: Refresh only writes proposed_prompt if currently empty (preserves user edits).

### Plannotator Integration

From task runner, `p` exports prompt to temp markdown file, suspends CCC via `tea.Exec`, launches Claude with Plannotator for annotation. User annotates → LLM refines → loop until approved. On exit, reads back file, updates prompt, returns to task runner.

## Safety Guardrails

1. No auto-launch — explicit user approval required for every session
2. Permission mode defaults to `default` (Claude asks before risky operations)
3. Budget cap via `--max-budget-usd` (default $5)
4. Default mode is Normal (not Worktree)
5. Max 3 concurrent sessions with queue
6. Full prompt always visible and editable before launch

## Test Cases

### Happy path
- Create todo from Granola source → refresh generates proposed_prompt → user opens detail → views two-column layout → presses o → task runner shows full prompt → user configures mode/budget → presses enter → session launches → todo shows "active" → session completes → todo shows "ready for review"

### Editing flow
- Tab through fields → edit status via dropdown → edit due via text → edit project dir via picker → all persist to DB

### Concurrency
- Launch 3 sessions → 4th queues → 1st completes → 4th auto-starts (if pre-approved)

### Blocked state
- Agent uses SendUserMessage → todo shows "needs input" with question text

### Prompt refinement
- Open task runner → press c → type refinement instruction → LLM updates prompt → press p → Plannotator opens → annotate → LLM refines → approve → back to task runner with updated prompt

### Error cases
- Launch with no project dir set → prompt to select one
- Launch with no proposed prompt → prompt to write one or generate via skill
- Claude process crashes → session_status → "review" (wrapper catches exit)
- Budget exceeded → Claude exits, session_status → "review"
