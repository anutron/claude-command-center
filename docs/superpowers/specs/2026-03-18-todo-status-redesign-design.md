# SPEC: Todo Status Redesign

## Purpose

Replace the current three-field status model (`Status`, `TriageStatus`, `SessionStatus`) with a single `Status` field representing a finite state machine. The current model has 42 theoretical state combinations, most of which are invalid, and causes UI bugs where todos count toward "All" but appear in no filter tab.

## Current Problem

The existing model uses three independent fields:

- `Status`: `active` | `completed` | `dismissed`
- `TriageStatus`: `new` | `accepted`
- `SessionStatus`: `""` | `queued` | `active` | `review` | `blocked` | `failed` | `completed`

This creates impossible combinations (e.g., `new` + `running`) and orphaned states where a todo is `TriageStatus=accepted` with a `SessionStatus` like `failed` — counted in "All" but matching no filter tab.

## Design

### Single Status Field

One `status` column with eight valid states:

| State | Meaning | Set by |
|-------|---------|--------|
| `new` | Extracted by refresh, awaiting triage | System (refresh) |
| `backlog` | Accepted, not being worked on | User (triage accept, manual create, reopen) |
| `enqueued` | Waiting for an agent slot | System (agent queue) |
| `running` | Agent actively working | System (agent runner) |
| `review` | Agent finished successfully, needs human review | System (agent exit 0) |
| `failed` | Agent finished with error, needs human review | System (agent exit != 0) |
| `completed` | Done | User |
| `dismissed` | Discarded / not relevant | User |

Manual todos created via `t` enter as `backlog` directly (skip `new`).

### State Transitions

**Universal transitions (from any state):**

- → `dismissed` — User dismisses (`X`). If `running` or `enqueued`, agent is killed/dequeued as side effect.
- → `completed` — User marks done (`x`). If `running` or `enqueued`, agent is killed/dequeued as side effect.

**State-specific transitions:**

| From | To | Trigger |
|------|----|---------|
| `new` | `backlog` | User accepts (`y`) |
| `backlog` | `enqueued` | User launches agent (queue full) |
| `backlog` | `running` | User launches agent (slot available) |
| `enqueued` | `running` | System (slot opens) |
| `enqueued` | `backlog` | User cancels from queue |
| `running` | `review` | System (agent exits 0) |
| `running` | `failed` | System (agent exits != 0) |
| `running` | `backlog` | User cancels agent |
| `review` | `backlog` | User pushes back |
| `review` | `enqueued` | User retries (queue full) |
| `review` | `running` | User retries (slot available) |
| `failed` | `backlog` | User pushes back |
| `failed` | `enqueued` | User retries (queue full) |
| `failed` | `running` | User retries (slot available) |
| `completed` | `backlog` | User reopens |
| `dismissed` | `backlog` | User reopens |

### State Diagram

```
              ┌──────────────────────────────────────────────────┐
              │         x (complete) from any state              │
              │         X (dismiss) from any state               │
              └──────────────────────────────────────────────────┘

                  new ──→ backlog ──→ enqueued ──→ running
                             ↑  ↑       │            │
                             │  │       │            │
                             │  └───────┘            ▼
                             │  (cancel)          review
                             │                      │
                             │                      ▼
                             │                   failed
                             │                      │
                             ├──────────────────────┘
                             │  (push back from review/failed)
                             │
                             │     ┌─────────┐
                             ├─────┤completed│
                             │     └─────────┘
                             │     ┌─────────┐
                             └─────┤dismissed │
                                   └─────────┘
                              (reopen → backlog)

        review ──→ enqueued/running  (retry)
        failed ──→ enqueued/running  (retry)
```

### Tab Groupings (View Layer)

Tabs are a view-layer concept — groups of states for filtering:

| Tab | Label | Shows statuses |
|-----|-------|---------------|
| ToDo | ToDo | `backlog` |
| Inbox | Inbox | `new` |
| Agents | Agents | `enqueued`, `running` |
| Review | Review | `review`, `failed` (with visual distinction: green for review, red for failed) |
| All | All | Everything except `completed`, `dismissed` |

The collapsed (non-expanded) default view shows **ToDo** (`backlog`).

The `completed` and `dismissed` todos are accessible via the backlog toggle (`b` key).

The header count shows the count for the currently selected tab.

### Visual Treatment in Review Tab

Both `review` and `failed` appear in the same Review tab but with different visual indicators:

- `review` — success styling (agent completed its work, ready for human approval)
- `failed` — error styling (agent encountered an error, needs human decision to retry or take over)

## Data Migration

### Column Changes

- The `status` column is reused with new valid values
- The `triage_status` column is dropped
- The `session_status` column is dropped
- `session_id`, `session_summary`, `session_log_path` are retained (agent run metadata, not status)

### Migration Mapping

| Current state | New `status` |
|---------------|-------------|
| `Status=active`, `TriageStatus=new`, `SessionStatus=""` | `new` |
| `Status=active`, `TriageStatus=accepted`, `SessionStatus=""` | `backlog` |
| `Status=active`, `SessionStatus="queued"` | `enqueued` |
| `Status=active`, `SessionStatus="active"` | `running` |
| `Status=active`, `SessionStatus="review"` | `review` |
| `Status=active`, `SessionStatus="failed"` | `failed` |
| `Status=active`, `SessionStatus="completed"` | `review` |
| `Status=active`, `SessionStatus="blocked"` | `backlog` |
| `Status=completed` | `completed` |
| `Status=dismissed` | `dismissed` |

## Impact

### Files affected

- `internal/db/types.go` — Remove `TriageStatus`, `SessionStatus` fields from `Todo` struct; update `ActiveTodos()`, `CompletedTodos()`, mutation methods
- `internal/db/read.go` — Update queries to stop reading `triage_status`, `session_status`
- `internal/db/write.go` — Update queries to stop writing `triage_status`, `session_status`
- `internal/db/schema.go` — Migration to drop columns and remap status values
- `internal/builtin/commandcenter/commandcenter.go` — Replace `filteredTodos()`, `triageCounts()` with single-field logic
- `internal/builtin/commandcenter/cc_view.go` — Update tab definitions, header counts, status bar
- `internal/builtin/commandcenter/cc_keys.go` — Update key handlers for new status values
- `internal/builtin/commandcenter/cc_keys_detail.go` — Update detail view status display
- `internal/builtin/commandcenter/cc_messages.go` — Update message handlers
- `internal/builtin/commandcenter/agent_runner.go` — Replace `setTodoSessionStatus` with direct status updates
- `internal/refresh/merge.go` — Update merge logic for new status values
- `internal/refresh/llm.go` — Update LLM-related status checks

### Transition Validation

Transitions should be validated — a helper function `ValidTransition(from, to string) bool` should enforce the state machine, preventing illegal transitions. Universal exits (`completed`, `dismissed`) are always valid. State-specific transitions follow the table above.

## Test Cases

### State Machine

- Manual todo creation → status is `backlog`
- Extracted todo from refresh → status is `new`
- Accept `new` todo → status becomes `backlog`
- Launch agent on `backlog` todo with slot available → status becomes `running`
- Launch agent on `backlog` todo with queue full → status becomes `enqueued`
- Agent exits 0 → status becomes `review`
- Agent exits non-zero → status becomes `failed`
- Dismiss from any state → status becomes `dismissed`
- Complete from any state → status becomes `completed`
- Reopen `completed` → status becomes `backlog`
- Reopen `dismissed` → status becomes `backlog`
- Cancel `enqueued` → status becomes `backlog`
- Cancel `running` → status becomes `backlog`, agent killed

### Tab Filtering

- `backlog` todo appears in ToDo tab and All tab
- `new` todo appears in Inbox tab and All tab
- `enqueued` todo appears in Agents tab and All tab
- `running` todo appears in Agents tab and All tab
- `review` todo appears in Review tab and All tab
- `failed` todo appears in Review tab and All tab
- `completed` todo appears in none of the tabs (backlog toggle only)
- `dismissed` todo appears in none of the tabs (backlog toggle only)
- All tab count = sum of non-terminal todos

### Tab Counts

- Every non-terminal todo is counted in exactly one named tab
- The sum of ToDo + Inbox + Agents + Review always equals All
- No todo can be "orphaned" (counted in All but not in any named tab)

### Migration

- Existing `active`/`accepted`/`""` todos become `backlog`
- Existing `active`/`new`/`""` todos become `new`
- Existing `active`/`accepted`/`"failed"` todos become `failed` (the original bug)
- Existing `completed` todos become `completed`

### Invalid Transitions

- `new` → `running` is rejected (must go through `backlog` first)
- `completed` → `running` is rejected (must reopen to `backlog` first)
- `dismissed` → `enqueued` is rejected (must reopen to `backlog` first)
