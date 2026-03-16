---
name: todo-trainer
description: Interactive testing harness for todo-agent routing — simulate refresh agent behavior, correct routing, capture rules
---

# Todo Trainer

Interactive training harness that simulates the refresh agent's routing decision for a specific todo, lets the user correct it, and captures routing rules.

## Arguments

- `$ARGUMENTS` - Required: display_id of the todo to train on (e.g., `6`)

## Context

- Config file: !`cat ~/.config/ccc/config.yaml 2>/dev/null | grep -A2 'refresh:' || echo "NO_CONFIG"`
- Refresh model: !`cat ~/.config/ccc/config.yaml 2>/dev/null | grep 'model:' | head -1 | awk '{print $2}' || echo "haiku"`
- Todo instructions: !`cat todo_instructions.md 2>/dev/null || echo "NO_INSTRUCTIONS_FILE"`

## Instructions

### Step 1: Validate Arguments

If `$ARGUMENTS` is empty or not a number, tell the user:
"Usage: `/todo-trainer <display_id>` — e.g., `/todo-trainer 6`"
Stop here.

### Step 2: Fetch the Todo

Run:
```bash
ccc todo --get $ARGUMENTS
```

This returns JSON with fields: `title`, `detail`, `context`, `source`, `who_waiting`, `due`, `display_id`, etc.

If the command fails (no todo with that ID), relay the error and stop.

### Step 3: Gather Project Context

Run:
```bash
ccc paths --json
```

This returns JSON with:
- `paths[]` — each with `path`, `description`, `skills[]`, `routing_rules`
- `global_skills[]` — skills available in all projects

### Step 4: Determine the Refresh Model

Read the `refresh.model` value from context above. If not found, default to `haiku`.

### Step 5: Run the Routing Sub-Agent

Use the **Agent tool** with the `model` parameter set to the refresh model from Step 4.

Give the sub-agent this exact prompt, filling in the todo fields and project context from Steps 2-3:

```
You are choosing which project directory is most appropriate for a task,
and generating a prompt to execute it there.

## Task
Title: {todo.title}
Detail: {todo.detail}
Context: {todo.context}
Source: {todo.source}
Who waiting: {todo.who_waiting}
Due: {todo.due}

{if todo.source_context is not empty}
## Source Context
Source: {todo.source} (ref: {todo.source_ref})
Fetched: {todo.source_context_at}

<source_context>
{todo.source_context}
</source_context>
{end if}

## Available Projects
{for each path from ccc paths --json output}
### {path}
Description: {description}
Project skills: {skill names and descriptions, comma-separated}
Routing preferences — use for: {use_for rules, semicolon-separated}
Routing preferences — not for: {not_for rules, semicolon-separated}
{end for each}

## Global Skills (available in ALL projects)
{for each global skill}
- {name}: {description}
{end}
Note: Do not prefer a project just because it has skills that are also available globally. Focus on whether the project's PURPOSE matches the task.

{if todo_instructions from context is not "NO_INSTRUCTIONS_FILE"}
## User Instructions
{paste the full todo_instructions content here}
{end if}

## Instructions
1. Choose the best project directory for this task. Explain your reasoning briefly.
2. Generate an actionable prompt for a Claude Code agent working in that directory.
   The prompt should:
   - State the objective clearly in imperative mood
   - Include relevant context from the todo detail
   - Mention who is waiting (for attribution)
   - Suggest what "done" looks like

Return ONLY JSON:
{"project_dir": "/path/to/project", "proposed_prompt": "## Objective\n...", "reasoning": "One sentence explaining why this project"}
```

Only include non-empty todo fields (skip fields like Detail or Context if they are empty strings).

Parse the JSON response from the sub-agent.

### Step 6: Present the Result

Show the user:

```
## Routing Decision

**Todo:** {todo title} (#{display_id})
**Proposed project:** {project_dir}
**Reasoning:** {reasoning}

### Proposed Prompt
{proposed_prompt}
```

Then ask:
"Is this the right project? If not, which project should this go to and why?"

### Step 7: Handle Corrections

Corrections fall into two categories:
- **Routing corrections** — wrong project was chosen → apply routing rules via `ccc paths --add-rule`
- **Prompt/behavior corrections** — right project but the prompt content needs improvement → update `todo_instructions.md`

**If the user says it's correct:** Say "Great, no changes needed." and stop.

**If the user corrects the routing (wrong project):**

1. Based on their feedback, propose one or both of:
   - A `use_for` rule on the correct project (why it SHOULD go there)
   - A `not_for` rule on the incorrectly chosen project (why it should NOT go there)

2. Show the proposed rules as a diff:
   ```
   Proposed rule changes:
   + /path/to/correct-project: use_for "tasks about X"
   + /path/to/wrong-project: not_for "tasks about X"
   ```

3. Ask: "Apply these rules? (yes/no/edit)"

4. **On confirm:** Run the appropriate commands:
   ```bash
   ccc paths --add-rule "/path/to/project" --use-for "description"
   ```
   and/or:
   ```bash
   ccc paths --add-rule "/path/to/project" --not-for "description"
   ```

5. **On edit:** Let the user modify the rule text, then apply.

**If the user gives prompt/behavior feedback (right project, but prompt needs work):**

1. Read the current `todo_instructions.md` file in the project root
2. Propose an addition or edit to the instructions based on the user's feedback
3. Show the proposed change as a diff
4. Ask: "Update todo_instructions.md? (yes/no/edit)"
5. **On confirm:** Write the updated content to `todo_instructions.md` using the Edit or Write tool
6. **On edit:** Let the user modify the text, then write it

### Step 8: Verify the Correction

After applying rules, re-run the routing sub-agent (repeat Step 3 to get fresh context with updated rules, then Step 5) to verify the new rules produce the correct routing.

Show the new result and confirm:
- If the routing now matches the user's correction: "Verified -- the updated rules produce the correct routing."
- If it still routes wrong: "The rules didn't fix it. Let's try a stronger rule." Go back to Step 7.

### Step 9: Repeat

Ask: "Want to train on another todo? Provide a display_id or say done."

If the user provides another ID, go back to Step 2 with the new ID.
