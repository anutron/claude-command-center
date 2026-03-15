# Design: Path Metadata for Todo-Agent Project Routing

## Purpose

The todo-agent needs to choose the right project directory for a task. Today, `cc_learned_paths` stores bare filesystem paths with no context about what each project does or what tools it offers. This design adds three layers of metadata that the todo-agent (and refresh agent) use to make informed routing decisions:

1. **Path descriptions** — LLM-generated summaries of what each project is
2. **Skills discovery** — cached inventory of project-local and global Claude Code skills
3. **Routing rules** — user-curated preferences learned through interactive corrections

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    /todo-agent #4                        │
│                                                         │
│  Gathers:                                               │
│  ├─ ccc paths --json  (descriptions + routing rules)    │
│  ├─ Project skills    (per-path, disk-cached 1hr)       │
│  ├─ Global skills     (~/.claude/skills/, disk-cached)  │
│  └─ Todo from DB                                        │
│                                                         │
│  Proposes project + prompt → user corrects →            │
│  correction saved as routing rule                       │
└─────────────────────────────────────────────────────────┘
```

## 1. Path Descriptions

### Storage

Existing `description TEXT` column on `cc_learned_paths` (already added). Contains a 1-2 sentence summary of what the project is and what tech stack it uses.

### Generation

**On new path add (sessions plugin):**

1. Path is inserted into DB immediately — user is not blocked
2. A background `tea.Cmd` fires an LLM call that:
   - Reads `README.md` (first ~200 lines)
   - Reads `CLAUDE.md` (first ~100 lines, focusing on Project Overview)
   - Produces a 1-2 sentence summary
3. On completion, writes the description back to DB via `DBUpdatePathDescription`
4. If LLM is unavailable, falls back to `AutoDescribePath` file heuristics (already implemented)

**LLM prompt template:**

```
Summarize this project in 1-2 sentences for someone deciding which project
directory to route a task to. Include: what the project does, primary tech
stack, and domain. Be specific — "Go TUI dashboard for personal productivity"
is better than "a software project."

README.md:
{readme_content}

CLAUDE.md (project instructions):
{claude_md_content}
```

**Background command pattern:** The sessions plugin must store an `llm llm.LLM` field (initialized from `ctx.LLM` in `Init`, falling back to `llm.NoopLLM{}`). The async `tea.Cmd` follows the same pattern as `claudeEditCmd` in the command center — returns a `pathDescribeFinishedMsg` handled in the sessions plugin's `HandleMessage`.

**CLI backfill:** `ccc paths --auto-describe` uses the same LLM path. The CLI initializes an LLM instance via `llm.Available()` check — if the `claude` binary is on PATH, use `llm.ClaudeCLI{}`; otherwise fall back to heuristics. The CLI call is synchronous (not async tea.Cmd) since it's a short-lived process.

### Functions

| Function | Location | Purpose |
|----------|----------|---------|
| `AutoDescribePath(dir)` | `internal/db/path_describe.go` | Heuristic fallback (already exists) |
| `LLMDescribePath(l llm.LLM, dir)` | `internal/builtin/sessions/describe.go` | Reads project files, calls LLM, returns description. Lives in sessions package (not `internal/db`) to avoid importing `internal/llm` from the data layer. |
| `pathDescribeCmd(l llm.LLM, path)` | `internal/builtin/sessions/describe.go` | Async tea.Cmd wrapper, returns `pathDescribeFinishedMsg` |

**Package layering note:** `internal/db` remains dependency-free from `internal/llm`. The heuristic `AutoDescribePath` stays in `internal/db/path_describe.go` (pure stdlib). The LLM-dependent `LLMDescribePath` lives in the sessions plugin package which already imports `internal/llm` transitively via `plugin.Context`.

## 2. Skills Discovery

### Design Principles

- Skills change frequently — user adds/removes skills as they iterate on automation
- Never stored in DB — read from filesystem
- Cached to disk with 1-hour TTL (survives process restarts — important since `ccc paths --json` is a short-lived CLI process)
- Manual refresh available via CLI

### Expected SKILL.md format

```yaml
---
name: wind-down
description: Save session context to disk for future resume
---

# Skill body (not parsed by discovery)
...
```

If frontmatter is absent or unparseable, the file is skipped.

### Data Types

```go
// SkillInfo holds frontmatter from a SKILL.md file.
type SkillInfo struct {
    Name        string `json:"name"`
    Description string `json:"description"`
}

// SkillCache holds discovered skills with a TTL.
type SkillCache struct {
    Skills    []SkillInfo `json:"skills"`
    ScannedAt time.Time   `json:"scanned_at"`
}
```

### Functions

| Function | Purpose |
|----------|---------|
| `DiscoverSkills(dir string) []SkillInfo` | Scan `<dir>/.claude/skills/*/SKILL.md`, parse frontmatter |
| `DiscoverGlobalSkills() []SkillInfo` | Scan `~/.claude/skills/*/SKILL.md` |

Both are pure file I/O — read each `SKILL.md`, extract the `name` and `description` fields from YAML frontmatter, return the list. No LLM involved. If a `SKILL.md` has missing or malformed frontmatter, it is skipped silently.

### Caching

The cache is **disk-based** at `~/.config/ccc/cache/skills/` so it survives across short-lived CLI invocations of `ccc paths --json`. Each path gets its own cache file keyed by a hash of the directory path. Global skills cache is stored separately.

```
~/.config/ccc/cache/skills/
├── global.json              # global skills cache
├── a1b2c3d4.json            # project skills cache (hash of path)
└── ...
```

Each file contains a `SkillCache` struct with `scanned_at` timestamp. On read:
- If cache file exists and `scanned_at` is within 1 hour, return cached skills
- Otherwise, re-scan the filesystem and write updated cache

**Global skills path:** Currently hardcoded to `~/.claude/skills/`. Not config-driven — acceptable for now, may be made configurable later if needed.

### CLI

- `ccc paths --json` includes `"skills"` array per path and a top-level `"global_skills"` array
- `ccc paths --refresh-skills` forces a re-scan of all paths + global, ignoring TTL

### Output format for `ccc paths --json`

**Breaking change:** The current `--json` output is a flat `[]LearnedPath` array. This changes to a top-level object with `paths` and `global_skills`. Any existing consumers must be updated in the same commit.

```json
{
  "paths": [
    {
      "path": "/Users/aaron/Personal/claude-command-center/",
      "description": "Go TUI productivity dashboard...",
      "added_at": "2026-03-11T07:37:33Z",
      "sort_order": 2,
      "skills": [
        {"name": "bookmark", "description": "Save a reference to this session..."},
        {"name": "wind-down", "description": "Save session context to disk..."}
      ],
      "routing_rules": {
        "use_for": ["CCC feature development", "TUI plugin work"],
        "not_for": ["data investigations"]
      }
    }
  ],
  "global_skills": [
    {"name": "commit", "description": "Create a git commit"},
    {"name": "review", "description": "Quick code review shorthand"}
  ]
}
```

## 3. Routing Rules

### Storage

**File:** `~/.config/ccc/routing-rules.yaml`

```yaml
# Routing preferences learned from /todo-agent corrections.
# Managed by the todo-agent — edit manually if needed.

/Users/aaron/Development/thanx/merchant-ui/:
  use_for:
    - Thanx loyalty program UI bugs
    - merchant dashboard features
  not_for:
    - backend API work
    - data pipeline issues

/Users/aaron/Development/ai/sherlock/:
  use_for:
    - data investigations
    - Snowflake query work
    - dashboard analytics
```

### Lifecycle

- **Created by:** `/todo-agent` skill during interactive corrections
- **Updated by:** `/todo-agent` when user corrects a routing decision
- **Read by:** `ccc paths --json` (merged into output), todo-agent, refresh agent
- **Editable by:** User directly in any text editor

### Error handling

- **Missing file:** Return empty map (no rules yet — normal state)
- **Malformed YAML:** Log a warning, return empty map. Do not propagate parse errors to `ccc paths --json` — a corrupted rules file should degrade gracefully, not break path listing.
- **Orphaned entries** (rules for paths that have been removed from DB): Harmless — they'll never match a known path. No automatic cleanup. User can edit the file to remove stale entries if desired.

### Functions

| Function | Location | Purpose |
|----------|----------|---------|
| `LoadRoutingRules()` | `internal/db/routing_rules.go` | Parse YAML file, return `map[string]RoutingRule`. Returns empty map on missing file or parse error. |
| `SaveRoutingRules(rules)` | `internal/db/routing_rules.go` | Write rules back to YAML |
| `AddRoutingRule(path, ruleType, text)` | `internal/db/routing_rules.go` | Append a `use_for` or `not_for` entry. Creates file if missing. |

### Data type

```go
type RoutingRule struct {
    UseFor []string `yaml:"use_for,omitempty"`
    NotFor []string `yaml:"not_for,omitempty"`
}
```

## 4. Todo-Agent and Todo-Trainer

### Architecture Split

The todo prompt generation has two layers:

- **`/todo-agent`** — The shared production logic: takes a todo + project context, returns a project_dir + proposed_prompt. Used by both the refresh agent (`ccc-refresh`) and the trainer. This is a **Go function** in the refresh package, not a skill — it uses the same prompt template that `generateProposedPrompts()` currently uses, extended with project context and routing rules.

- **`/todo-trainer`** — Interactive testing harness (Claude Code skill). Lets the user simulate what the refresh agent produces, correct it, and capture routing rules. Scoped to project routing + prompt generation only (not commitment extraction from meetings/Slack — that gets its own trainer later).

### Why two layers?

The refresh agent runs headless with a cheap model. You need a way to see what it would produce, correct it, and verify the correction sticks — without running a full refresh cycle. The trainer simulates exactly the refresh agent's behavior (same model, same prompt template, same project context) but interactively.

### `/todo-agent` — Production Logic

A Go function (not a skill) that the refresh agent and trainer both call:

```go
// internal/refresh/todo_agent.go

// GenerateTodoPrompt takes a todo and project context, returns a
// project_dir assignment and a proposed prompt.
func GenerateTodoPrompt(l llm.LLM, todo db.Todo, paths PathContext) (*TodoPromptResult, error)

type PathContext struct {
    Paths        []PathEntry  // from ccc paths --json
    GlobalSkills []SkillInfo  // from ~/.claude/skills/
}

type PathEntry struct {
    Path         string
    Description  string
    Skills       []SkillInfo
    RoutingRules *RoutingRule
}

type TodoPromptResult struct {
    ProjectDir     string `json:"project_dir"`
    ProposedPrompt string `json:"proposed_prompt"`
    Reasoning      string `json:"reasoning"`
}
```

This function:
1. Builds the routing prompt (see template below)
2. Calls the LLM
3. Parses the JSON response
4. Returns the result

The existing `generateProposedPrompts()` in `internal/refresh/llm.go` is refactored to call `GenerateTodoPrompt` per-todo instead of batching (or batching calls to it).

### `/todo-trainer <display_id>` — Interactive Skill

A Claude Code skill (`.claude/skills/todo-trainer/SKILL.md`) that:

1. Fetches todo by display_id from DB: `ccc add-todo --get <id>` (or a new `ccc todo --get <id>` CLI)
2. Gathers project context: `ccc paths --json`
3. Spins up a **sub-agent with the same model the refresh agent uses** (configurable in config.yaml, e.g. `refresh.model: haiku`)
4. Tells the sub-agent: "You are the refresh agent's prompt generator. Given this todo and these available projects, assign a project_dir and generate a proposed_prompt." Uses the **exact same prompt template** as `GenerateTodoPrompt`.
5. Shows the user: proposed project, proposed prompt, reasoning
6. User corrects: "No, this should go to sherlock because it's a data question"
7. Trainer proposes a routing rule update, shows the YAML diff
8. On confirm: updates `routing-rules.yaml` via `ccc paths --add-rule` (new CLI subcommand)
9. Re-runs the sub-agent with updated rules to verify the correction works
10. Repeat until satisfied

### Prompt template (shared by both)

```
You are choosing which project directory is most appropriate for a task,
and generating a prompt to execute it there.

## Task
Title: {todo.Title}
Detail: {todo.Detail}
Context: {todo.Context}
Source: {todo.Source}
Who waiting: {todo.WhoWaiting}
Due: {todo.Due}

## Available Projects
{for each path}
### {path}
Description: {description}
Project skills: {skill names + descriptions}
Routing preferences:
  Use for: {use_for rules}
  Not for: {not_for rules}
{end}

## Global Skills (available in ALL projects)
{global skill names + descriptions}
Note: Do not prefer a project just because it has skills that are also
available globally. Focus on whether the project's PURPOSE matches the task.

## Instructions
1. Choose the best project directory for this task. Explain your reasoning briefly.
2. Generate an actionable prompt for a Claude Code agent working in that directory.
   The prompt should:
   - State the objective clearly in imperative mood
   - Include relevant context from the todo detail
   - Mention who is waiting (for attribution)
   - Suggest what "done" looks like

Return ONLY JSON:
{
  "project_dir": "/path/to/project",
  "proposed_prompt": "## Objective\n...",
  "reasoning": "One sentence explaining why this project"
}
```

### Model configuration

```yaml
# ~/.config/ccc/config.yaml
refresh:
  model: haiku  # model used by ccc-refresh for prompt generation
```

The trainer reads this config to match the refresh agent's behavior. If unset, defaults to whatever `claude` CLI defaults to.

## 5. Changes to Existing Code

### `ccc paths` CLI (`cmd/ccc/paths.go`)

- `--json` output restructured: top-level object with `paths` array + `global_skills` array (**breaking change** from current flat array — no known external consumers yet)
- Each path entry gains `skills` (from disk cache) and `routing_rules` (from YAML)
- New flag: `--refresh-skills` to force cache invalidation
- `--auto-describe` upgraded: initializes LLM via `llm.Available()` check, uses `llm.ClaudeCLI{}` if available, otherwise falls back to heuristics. CLI call is synchronous (not async tea.Cmd).
- New flag: `--add-rule <path> --use-for "description"` and `--not-for "description"` for programmatic rule updates (used by todo-trainer skill)

### `ccc todo` CLI (new subcommand)

New `ccc todo --get <display_id>` subcommand that outputs a todo as JSON. Used by the `/todo-trainer` skill to fetch todo context.

```
ccc todo --get 6
{"id":"abc123","display_id":6,"title":"...","detail":"...","context":"...","source":"granola",...}
```

### Sessions plugin (`internal/builtin/sessions/sessions.go`)

- Add `llm llm.LLM` field to `Plugin` struct, initialized from `ctx.LLM` in `Init` (fallback to `llm.NoopLLM{}`)
- On `fzfFinishedMsg`: after `DBAddPath` (with heuristic description), fire background `pathDescribeCmd` for LLM upgrade
- Handle `pathDescribeFinishedMsg`: write LLM-generated description to DB (overwrites heuristic)

### Refresh agent (`internal/refresh/llm.go`)

- Refactor `generateProposedPrompts()` to call the new `GenerateTodoPrompt()` function, which includes project context and routing rules in its prompt
- Add `refresh.model` config key (default: unset, uses CLI default)

### New files

| File | Purpose |
|------|---------|
| `internal/builtin/sessions/describe.go` | `LLMDescribePath`, `pathDescribeCmd`, `pathDescribeFinishedMsg` |
| `internal/db/skill_discover.go` | `SkillInfo`, `SkillCache`, `DiscoverSkills`, `DiscoverGlobalSkills`, disk cache read/write |
| `internal/db/routing_rules.go` | `RoutingRule`, YAML load/save/update |
| `internal/refresh/todo_agent.go` | `GenerateTodoPrompt`, `PathContext`, `TodoPromptResult` — shared logic used by both refresh and trainer |
| `.claude/skills/todo-trainer/SKILL.md` | Interactive training harness skill |
| `cmd/ccc/todo.go` | `ccc todo --get <id>` CLI subcommand |

### Modified files

| File | Change |
|------|--------|
| `cmd/ccc/paths.go` | Restructure JSON output, add `--refresh-skills`, `--add-rule` |
| `cmd/ccc/main.go` | Wire up `todo` subcommand |
| `internal/builtin/sessions/sessions.go` | Add `llm` field, background LLM description on path add, handle result message |
| `internal/refresh/llm.go` | Refactor `generateProposedPrompts()` to use `GenerateTodoPrompt()` |
| `internal/config/config.go` | Add `Refresh.Model` config field |

## 6. Test Cases

### Skills Discovery
- `DiscoverSkills` finds skills in a test directory with SKILL.md files
- `DiscoverSkills` returns empty for directory with no `.claude/skills/`
- `DiscoverSkills` skips SKILL.md with missing or malformed frontmatter
- `DiscoverGlobalSkills` reads from `~/.claude/skills/`
- Disk cache returns cached results within TTL
- Disk cache re-scans after TTL expires
- `--refresh-skills` bypasses cache

### Routing Rules
- `LoadRoutingRules` parses valid YAML
- `LoadRoutingRules` returns empty map for missing file
- `LoadRoutingRules` returns empty map for malformed YAML (logs warning)
- `AddRoutingRule` creates file if missing, appends rule
- `SaveRoutingRules` round-trips through load

### Path Descriptions
- `LLMDescribePath` produces description from README + CLAUDE.md
- `LLMDescribePath` falls back to heuristic when LLM returns error
- Background description generation doesn't block path addition in sessions plugin
- `ccc paths --auto-describe` uses LLM when available, heuristics otherwise

### Todo Agent
- `GenerateTodoPrompt` returns valid result with project_dir and prompt
- `GenerateTodoPrompt` respects routing rules (not_for excludes, use_for prefers)
- `GenerateTodoPrompt` includes global skills context without over-indexing on them

### Integration
- `ccc paths --json` includes skills, routing rules, and global_skills in output
- `ccc paths --json` output is valid JSON even when routing-rules.yaml is missing/malformed
- `ccc todo --get <id>` returns valid JSON for existing todo
- `ccc todo --get <id>` returns error for nonexistent display_id
