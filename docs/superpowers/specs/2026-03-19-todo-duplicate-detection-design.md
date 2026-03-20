# SPEC: Todo Duplicate Detection

## Purpose

Prevent duplicate todos from accumulating when the same commitment is captured multiple times — manually via `t`, from Slack extraction, from Granola meetings, or any combination. Uses LLM-based semantic matching with synthesis-based merges so no information is lost.

## Design Principles

- **Immutable originals** — original todos are never mutated by a merge. They are hidden, not modified.
- **Synthesis on merge** — each merge creates a new todo that combines all originals, with the newest entry as source of truth where data overlaps
- **Flattened, not chained** — when a new entry matches an existing synthesis, gather all originals and re-synthesize from scratch
- **Reversible** — unmerge deletes the synthesis and restores originals to visibility
- **No extra LLM calls** — dedup detection piggybacks on existing enrichment (manual intake) and routing (refresh) prompts. Synthesis requires one additional Haiku call.
- **Veto persistence** — once a user splits items, they stay split across future refresh cycles

## Schema Changes

### New table: `cc_todo_merges`

```sql
CREATE TABLE IF NOT EXISTS cc_todo_merges (
    synthesis_id TEXT NOT NULL,
    original_id TEXT NOT NULL,
    vetoed INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    PRIMARY KEY (synthesis_id, original_id)
);
```

All merge state lives in this one table:
- **Merge** → create synthesis S from originals A+B, insert `(S.id, A.id, 0, now)` and `(S.id, B.id, 0, now)`
- **New entry matches synthesis** → gather all originals of S, add new entry, create new synthesis S2, supersede S (delete old rows, insert new rows pointing at S2)
- **Unmerge one original** → set `vetoed = 1` on that row. If only one non-vetoed original remains, delete the synthesis and restore that original too.
- **Unmerge all** → set all rows to `vetoed = 1`, delete the synthesis
- **Re-merge after veto** → set `vetoed = 0` (user intent overrides prior split)
- **Veto check** → look for row with `vetoed = 1` for the pair of original IDs

Synthesis todos are identified by `source = 'merge'` on the `cc_todos` row.

Originals are hidden from default views:
```sql
WHERE id NOT IN (SELECT original_id FROM cc_todo_merges WHERE vetoed = 0)
```

## Behavior

### Manual Intake (pressing `t`)

The existing `claudeEnrich` LLM call is expanded:

**New input:** Active todo list (title + display_id + id) appended to the enrichment prompt. "Active" means status in (`backlog`, `active`, `blocked`, `new`) and not hidden by a non-vetoed merge. Capped at 50 items, ordered by most recently created.

**New output fields:**

```json
{
  "title": "Send report to Bob",
  "due": "2026-03-20",
  "merge_into": "abc-123",
  "merge_note": "Matches #12 'Send Bob the quarterly report' — same deliverable, new deadline"
}
```

**When `merge_into` is set:**

1. Create the new original todo B in DB normally
2. Determine if the match target is itself a synthesis or an original:
   - If target is a synthesis S: gather all originals of S from `cc_todo_merges`
   - If target is a plain todo A: the originals list is just `[A]`
3. Add B to the originals list
4. Call the synthesis LLM (Haiku) with all originals to produce a combined todo, newest entry (B) as source of truth where data overlaps
5. Create the synthesis todo S2 with `source = 'merge'`
6. If superseding an old synthesis S: delete S from `cc_todos` and its `cc_todo_merges` rows
7. Insert `cc_todo_merges` rows: `(S2.id, original.id, 0, now)` for each original
8. Flash message: `"Merged with #12: Send Bob the quarterly report"`

**When `merge_into` is empty/null:**

1. Create todo as today
2. Flash message: `"Added todo #183"` (include display_id)

**Veto check:** Before accepting a merge, check `cc_todo_merges` for vetoed rows between B and any of the target's originals. If any pair is vetoed, ignore the LLM's suggestion and create B as a new standalone todo.

### Refresh Path (External Sources)

The routing LLM in `buildRoutingPrompt` (Stage 3, Sonnet) is expanded:

**New input:** Same active todo list appended to the routing prompt.

**New output fields in `TodoPromptResult`:**

```json
{
  "project_dir": "/path/to/project",
  "proposed_prompt": "...",
  "merge_into": "abc-123",
  "merge_note": "Matches #12 — same commitment from Slack DM"
}
```

**When `merge_into` is set:**

1. Create todo B in DB with its own `source_ref` (preserving the dedup-by-source mechanism)
2. Follow the same synthesis flow as manual intake (steps 2-7)
3. Silent — no user interaction needed

**When `merge_into` is empty:** Existing behavior, no change.

**Veto check:** Same as manual intake.

### Synthesis LLM Call

A Haiku-level call that takes all originals and produces a combined todo. This is the one new LLM call in the flow (dedup detection is free, but synthesis requires generation).

**Input:**
```
Combine these related todos into one. The newest entry is the source of truth
where information overlaps. Fill gaps from older entries.

Originals (oldest first):
1. [#12] "do X" (manual, no due, no who_waiting)
2. [#14] "do X for Carol" (slack, due: monday, who_waiting: Carol)

Return a single combined todo as JSON.
```

**Output:**
```json
{
  "title": "Do X for Carol",
  "due": "monday",
  "who_waiting": "Carol",
  "detail": "Originally captured as 'do X', later confirmed via Slack with deadline and stakeholder.",
  "context": "",
  "effort": ""
}
```

The synthesis inherits from the newest original: `project_dir`, `proposed_prompt`, `source_context`. Fields not in the LLM output are taken from the newest original that has them.

### Splitting (Unmerge)

In the detail view of a synthesis todo:

- A "Sources" section appears at the bottom listing the originals with their source and original title
- `U` keybinding unmerges the selected original:
  1. Set `vetoed = 1` on the `cc_todo_merges` row for that original
  2. That original reappears in the todo list with its pristine data
  3. If 2+ non-vetoed originals remain: re-synthesize from remaining originals (new Haiku call), replace old synthesis with new one
  4. If only 1 non-vetoed original remains: delete the synthesis, restore that last original to visibility (delete its `cc_todo_merges` row)
  5. If 0 non-vetoed originals remain: delete the synthesis

**Conditional display:** The sources section and `U` keybinding only appear on synthesis todos (`source = 'merge'`). Regular todos never show this section.

### Edge Cases

- **Match target is a synthesis:** Handled by flattening — gather originals, add new entry, re-synthesize. The old synthesis is superseded.
- **Merge target invariant:** Only visible (non-merged) todos appear in the LLM's active list. A hidden original can never be suggested as a merge target.
- **Self-match:** LLM returns `merge_into` pointing to the todo being created → ignore, treat as new
- **Invalid ID:** LLM returns an ID that doesn't exist → ignore, treat as new
- **Dismissed synthesis:** If a synthesis was dismissed, its originals stay hidden (dismissed is a tombstone)
- **Synthesis inherits status:** New synthesis takes the status of the old synthesis (or the matched original if no prior synthesis). User's triage decisions are preserved.

## Query Changes

All queries that list todos for display exclude hidden originals:

```sql
WHERE id NOT IN (SELECT original_id FROM cc_todo_merges WHERE vetoed = 0)
```

This applies to:
- Command center todo list view
- Suggestions LLM input
- Enrichment prompt active todo list
- Routing prompt active todo list

The `source_ref` unique index is unaffected — original todos keep their `source_ref` so re-extraction doesn't create a third copy.

## UX Polish

### Flash message improvement

- Current: `"Added todo"`
- New (no merge): `"Added todo #183"`
- New (merge): `"Merged with #12: Send Bob the quarterly report"`

### Search by display ID

The existing `/` search/filter already matches todo titles. Extend it to also match display IDs:

- `/ 183` matches todo #183
- `/ report` matches todos with "report" in the title (existing behavior)
- When filter narrows to a single result, pressing enter opens its detail view

## Test Cases

### Duplicate Detection

- Manual "do X" then manual "do X by tomorrow" → synthesis created with title from B, due from B
- Manual "send report" then Slack extracts "I'll send the report tomorrow" → synthesis combines both, Slack entry is source of truth for overlapping fields
- Granola "follow up with Bob" then Slack "I'll follow up with Bob" → synthesis created from both originals
- Manual "do X" then manual "do Y" → no merge (semantically different)

### Progressive Merge

- A: "do X" → visible
- B: "do X due tomorrow" → synthesis S1 created from A+B, A and B hidden, S1 visible with due=tomorrow
- C: "do X for Carol, due monday" → S1 superseded, new synthesis S2 from A+B+C, C is source of truth: due=monday, who_waiting=Carol
- S1 deleted from DB, S2 is the visible todo

### Synthesis Mechanics

- Newest entry is source of truth: B says "due tomorrow", A says "due friday" → synthesis uses "due tomorrow"
- Gaps filled from older entries: A has who_waiting "Alice", B has none → synthesis keeps "Alice"
- All originals preserved pristine in DB — never modified

### Splitting

- Unmerge C from S2 (3 originals: A, B, C) → C reappears, new synthesis S3 from A+B replaces S2
- Unmerge B from S3 (2 originals: A, B) → B reappears, only A remains, synthesis deleted, A reappears
- All originals retain their exact original data after any sequence of merges and unmerges
- Next refresh finds same Slack message → creates todo but veto prevents re-merge
- Unmerge then manually merge again → veto is cleared (`vetoed = 0`) when the user explicitly creates a todo that the LLM matches to a vetoed pair

### Edge Cases

- LLM returns invalid merge_into ID → treated as new todo
- LLM returns merge_into pointing to self → treated as new todo
- Match target is a synthesis → flattened: gather originals, add new entry, re-synthesize
- Vetoed pair → LLM suggestion ignored, created as new
- Synthesis dismissed → originals stay hidden

### Query Filtering

- Hidden originals don't appear in list view
- Hidden originals don't appear in suggestions input
- Hidden originals don't appear in enrichment/routing prompt todo lists
- Hidden originals still have their source_ref (re-extraction blocked by unique index)
- Synthesis todos appear normally in all views

### Display

- Synthesis detail view shows sources section + U keybinding
- Non-synthesis detail view shows no sources section, no U hint
- Flash: "Added todo #183" on new, "Merged with #12: ..." on merge
- Search `/` matches display IDs and titles
