# SPEC: Slack Todo Extraction

## Purpose

Extract actionable commitments from Slack messages and create todos. Runs during the data refresh cycle. Uses LLM to classify messages as real commitments vs. noise.

## Interface

- **Input**: Slack API token (user token `xoxp-*`), configured channels/search queries
- **Output**: `[]db.Todo` with source="slack", populated title/context/detail/who_waiting/due
- **Dependencies**: Slack API (`conversations.history`, `search.messages`), LLM for classification

## Behavior

### Message Fetching

Two fetch strategies, tried in order:

1. **Conversations-based** (`fetchSlackCandidates`): Iterates configured channels, fetches recent messages, resolves user display names via `users.info` API
2. **Search-based fallback** (`fetchSlackCandidatesViaSearch`): Uses `search.messages` API with query terms. Author comes from `match.Username` field (may be username not display name)

Both produce `slackCandidate` structs with: Message, Author, Permalink, Channel, ChannelID, ConversationContext, ThreadContext.

### Author Attribution

Each candidate includes an `Author` field — the display name of the message author. The LLM uses this to determine attribution:

- **Author is Aaron + first-person** ("I will..."): Aaron's commitment → ACCEPT
- **Author is NOT Aaron + first-person** ("I will..."): Someone else's commitment → REJECT
- **Author is NOT Aaron + assigns Aaron** ("Aaron will...", "Aaron to..."): Delegation to Aaron → ACCEPT
- **Author is NOT Aaron + assigns someone else**: Not Aaron's concern → REJECT

This prevents todos being created from other people's commitments that happen to appear in Aaron's Slack channels.

### LLM Classification

Messages are batched and sent to the LLM with a structured prompt. The LLM returns a JSON array of extracted commitments. The classification bar is intentionally high — expect 0-3 results from a batch of candidates.

**Accept criteria:**
- Concrete next action with a clear outcome
- Can be expressed as an actionable title starting with a verb
- Either Aaron's own commitment OR work assigned to Aaron by someone else

**Reject criteria:**
- First-person commitments by non-Aaron authors
- Conversational responses ("done", "sounds good")
- Observations, tips, shared links, compliments
- Past actions ("I just...", "I found that...")
- Vague intentions without specific deliverables
- Assignments to other people that don't include Aaron

### Todo Fields

Each extracted commitment maps to a `db.Todo`:
- `Title`: Actionable, verb-first, 20+ characters. Resolved from conversation context (not just the short message)
- `Source`: "slack"
- `SourceRef`: Slack permalink to the original message
- `Context`: Channel name and topic area
- `Detail`: Full context — participants, discussion summary, expectations
- `WhoWaiting`: Person(s) waiting on this deliverable
- `Due`: YYYY-MM-DD if mentioned, empty string otherwise
- `Status`: empty (new/unprocessed)

## Test Cases

- Message from Aaron with "I will send the report" → ACCEPT, title starts with "Send"
- Message from coworker with "I will send the report" → REJECT (not Aaron's commitment)
- Message from coworker with "Aaron will handle the migration" → ACCEPT (delegation to Aaron)
- Conversational response "sounds good!" → REJECT
- Past action "I just deployed the fix" → REJECT
- Vague "we should look into this" → REJECT
- Empty candidate list → returns empty todo list, no LLM call
