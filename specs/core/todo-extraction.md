# SPEC: Todo Extraction

## Purpose

Defines what qualifies as an extractable todo from external sources (Gmail, Slack, Granola), how candidates are filtered before reaching the LLM, and how the LLM determines ownership. This spec governs the extraction and routing stages of the refresh pipeline.

## Interface

- **Inputs**: Gmail labeled emails, raw messages from Slack channels/DMs, Granola meeting transcripts
- **Outputs**: `[]db.Todo` with title, source, source_ref, context, detail, who_waiting, due, status
- **Dependencies**: Haiku LLM (extraction + Gmail title generation), Sonnet LLM (routing/validation)

## Behavior

### Gmail: Label-Based Title Generation (Haiku)

When emails are labeled for todo tracking, the email subject is a poor proxy for the action item. Instead, `fetchLabeledTodos` uses the LLM to generate actionable titles from the full email body.

#### Flow

1. Fetch metadata (Subject, From, To) for each labeled email — same as before
2. Fetch body via `GetMessageBody` for each labeled email
3. Batch all emails into a single LLM prompt
4. LLM returns a JSON map of `{message_id: generated_title}` for each email
5. If LLM is unavailable (nil), fall back to email subject (current behavior)

#### LLM Prompt Inputs

For each email, the prompt receives:
- Message ID
- Subject line
- Sender name
- Recipient (To header) — used to determine direction
- Email body (truncated to 2000 chars)

#### Title Generation Rules

The LLM generates a title that captures the job to be done:
- **Imperative mood, verb-first** (Send, Review, Follow up, Schedule, etc.)
- **20+ characters** — specific enough to be actionable
- **Received emails**: "What is this email asking me to do?"
- **Sent emails**: "What did I commit to doing?"
- Resolve pronouns and vague references using the email body
- Strip "Re:", "Fwd:", and other prefixes — the title should describe the action, not the email thread

#### Batching

All labeled emails are sent in a single LLM call to minimize API usage. The response is a JSON array of `{id, title}` objects.

#### Fallback

When `llm.LLM` is nil (no LLM configured), the email subject is used as the title — preserving current behavior.

### Stage 1: Pre-Filter (Keyword Match)

Before any LLM call, messages are filtered by keyword to reduce token cost. A message must contain at least one commitment phrase to be considered a candidate.

#### Self-Commitment Phrases

First-person language indicating Aaron made a commitment:

- "i'll", "i will", "i need to", "let me", "i'm going to"
- "action item", "i committed", "i promise", "follow up"
- "send you", "set up", "schedule", "i can do"
- Verb-specific: "i'll take/handle/get/send/look/check/follow/set/make/write/review/update/fix/create/put/share/reach out"

#### Assignment Phrases

Third-person language indicating someone else assigned work to Aaron:

- "aaron will", "aaron is going to", "aaron to follow/handle/send/review/set up/schedule"
- "aaron can", "aaron should", "aaron needs to"
- "and aaron will" (captures "Darren and Aaron will...")

#### Search Queries (Slack search.messages fallback)

When the conversations API is unavailable, `search.messages` runs these queries:

- Self: "i'll", "i will", "i promise", "action item", "follow up", "let me"
- Assignment: "aaron will", "aaron is going to", "aaron to follow"

### Stage 1b: Conversation Context

Before LLM extraction, each candidate is enriched with surrounding conversation context to help resolve pronouns and understand what was committed to.

- **Channel history path**: Up to 15 preceding same-day messages from the same channel are included as `ConversationContext`. Messages are presented in chronological order (oldest first). Stops at the calendar day boundary (Pacific time) to avoid pulling in stale context from previous days.
- **Thread replies**: If the candidate is part of a thread, replies are fetched and included as `ThreadContext`.
- **Search fallback path**: No conversation context available (search API returns individual matches). Thread context is also unavailable.

This context is critical for DM conversations where commitments often use pronouns ("I'll get this to you", "I'll handle it") that refer to earlier messages.

### Stage 2: LLM Extraction (Haiku)

Candidates that pass the pre-filter are sent to haiku for extraction.

#### Slack Extraction

A message is a todo if EITHER:

- **A) Self-commitment**: Aaron explicitly committed to a specific deliverable with a concrete next action
- **B) Third-party assignment**: Someone else assigned work to Aaron — e.g., "Aaron will...", "Darren and Aaron will follow-up on...", "Aaron is going to..."

In both cases:

- There must be a concrete next action with a clear outcome
- The title must be actionable, starting with a verb (Send, Review, Schedule, Build, Write, Follow up, etc.)

REJECT:

- Conversational responses ("done", "good process!", "sounds good")
- Observations, tips, shared links, compliments
- Descriptions of past actions ("I just...", "I found that...")
- Vague intentions without a specific deliverable
- Assignments to other people that don't include Aaron

#### Granola Extraction

Transcripts use speaker labels: `[Aaron]` = the user, `[Other]` = other participants.

A commitment is Aaron's if:

- Aaron states he will do something in an `[Aaron]` block
- Aaron agrees/affirms in an `[Aaron]` block when asked by `[Other]`
- Someone in an `[Other]` block assigns work to Aaron by name (e.g., "Aaron will follow up on...", "Darren and Aaron will handle...")

REJECT:

- Commitments made by others about THEMSELVES in `[Other]` blocks (e.g., `[Other]: "I will handle that"`)
- General discussion points without a clear commitment involving Aaron
- Action items assigned to other people that don't mention Aaron by name

### Stage 3: Routing Validation (Sonnet)

After extraction, the routing LLM validates ownership before assigning a project directory.

A task is Aaron's if ANY of these are true:

- **a)** Aaron stated he would do it or explicitly agreed to do it
- **b)** Someone else assigned the work to Aaron by name (e.g., "Aaron will...", "Darren and Aaron will follow-up on...")

REJECT only if:

- The commitment was made by someone else about themselves (not Aaron)
- Aaron is not mentioned or involved in the commitment

If rejected, `project_dir` is set to `"REJECT"` and the todo is auto-dismissed.

## Test Cases

### Gmail Title Generation

- Subject "Re: Q2 Planning", body asks Aaron to prepare a slide deck → title: "Prepare slide deck for Q2 planning"
- Subject "Quick question", body asks Aaron to review a contract → title: "Review contract" (not "Quick question")
- Sent email where Aaron says "I'll have the migration script ready by Friday" → title: "Complete migration script by Friday"
- No LLM available → title falls back to email subject
- Body is empty or fetch fails → title falls back to email subject
- LLM returns no title for a given message ID → title falls back to email subject

### Pre-Filter

- "I'll send the report tomorrow" matches on "i'll"
- "Aaron will follow-up on card tokens" matches on "aaron will"
- "Darren and Aaron will handle the integration" matches on "and aaron will"
- "Great meeting everyone!" does not match any phrase
- "Sarah will handle that" does not match (no Aaron phrase)

### Extraction — Self-Commitments

- Aaron says "I'll review the PR by Friday" → extracted with title "Review the PR", due date
- Aaron says "sounds good" → rejected (conversational)
- Aaron says "I just finished the deploy" → rejected (past action)

### Extraction — Third-Party Assignments

- Zach says "Darren and Aaron will follow-up on card tokens from Qu" → extracted with title "Follow up on card tokens and unknown identity from Qu"
- Manager says "Aaron is going to set up the demo environment" → extracted
- Colleague says "Sarah will handle the frontend" → rejected (not Aaron)

### Routing Validation

- Todo from Slack where someone said "Aaron will..." → accepted (category b)
- Todo from Granola where `[Other]` said "I will handle that" → rejected (other's commitment)
- Todo from Granola where `[Other]` said "Aaron, can you follow up on X?" and `[Aaron]` said "Yes" → accepted (category a)
- Todo from Granola where `[Other]` said "Aaron will take care of the integration" → accepted (category b)

### Conversation Context Resolution

- Preceding DM: "Can you send me the mockup?" → Aaron: "I'll get this to you by EOD" → extracted with title referencing the mockup, not just "this"
- Preceding channel message about a PR → Aaron: "I'll handle it" → extracted with title referencing the PR

### Edge Cases

- "Aaron can you check on this?" without an affirmative response — should NOT be extracted (question, not assignment)
- "Aaron should probably look at this" — extracted as low-confidence assignment (matches "aaron should")
- Multiple assignments in one message — each extracted as separate todo
