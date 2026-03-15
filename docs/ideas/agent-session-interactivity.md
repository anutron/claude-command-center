# Agent Session Interactivity — Future Work

## Problem

Once a headless agent is running, there's no way to:
1. "Step into" it to see what it's doing or answer a question
2. "Step out" and have it continue working headless
3. Answer a blocked agent's question without joining the full session

## Approaches to Explore

### Option A: tmux-based sessions

Launch headless agents in a tmux session instead of a raw subprocess. "Join" = attach to tmux pane. "Leave" = detach. Process keeps running either way.

**Pros:** General-purpose, works for any interaction pattern, user can see full context
**Cons:** Adds tmux dependency, more complex process management, CCC needs to track tmux session names

### Option B: stream-json input/output

Use Claude's `--input-format stream-json` alongside `--output-format stream-json` to send answers to blocked agents programmatically. User never "joins" — they answer questions from CCC's UI.

**Pros:** No external dependencies, clean separation, CCC stays in control
**Cons:** Limited to Q&A interaction (can't browse agent's full context), needs bidirectional stream-JSON pipe management

### Option C: Hybrid

Use stream-json for answering blocked questions (common case), tmux for full "drop-in" sessions (power user case).

## Open Questions

- Does `--input-format stream-json` support sending tool results / user messages to a running session?
- Can we pipe stdin to a running subprocess after launch?
- What's the UX for answering a blocked question inline in CCC vs opening a full terminal?
