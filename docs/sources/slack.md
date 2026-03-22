# Source: Slack

## What It Provides

- **Todos:** Extracts commitments from Slack messages using LLM analysis. Scans channels, DMs, and group DMs for messages containing commitment language (e.g., "I'll", "I will", "let me", "follow up") and uses an LLM to determine which are real action items.
- **Context fetching:** Retrieves conversation context around a Slack message for todo detail views.

## Prerequisites

1. A Slack user token (preferred) or bot token with appropriate scopes
2. An LLM configured in CCC (required for commitment extraction -- without it, candidates are found but no todos are created)

## Step-by-Step Setup

### 1. Create a Slack App and Get a Token

The source uses the Slack Web API directly with a bearer token. You need either:

**Option A: User token (recommended)**
A user token (`xoxp-...`) with these scopes:
- `search:read` -- minimum required scope (search-based fallback)
- `channels:read` -- list channels the user is in
- `channels:history` -- read channel messages
- `groups:read` -- list private channels
- `groups:history` -- read private channel messages
- `im:read` -- list DM conversations
- `im:history` -- read DM messages
- `mpim:read` -- list group DM conversations
- `mpim:history` -- read group DM messages
- `users:read` -- resolve user IDs to display names

**Option B: Bot token**
A bot token (`xoxb-...`) with equivalent scopes. Note: bot tokens may have limited access to DMs.

The source gracefully degrades: if `channels:read`/`channels:history` scopes are missing, it falls back to `search.messages` which only requires `search:read`.

### 2. Configure `config.yaml`

```yaml
slack:
  enabled: true
  token: "xoxp-your-slack-token"
```

Fields:

- `enabled` (required): Set to `true` to activate the source.
- `token` (required): The Slack API token. Preferred field name.
- `bot_token` (deprecated): Legacy field name. If `token` is empty, `bot_token` is used as fallback.

### 3. Security Note

Store the token in `config.yaml` directly. The file should have restrictive permissions (`0600`). The token is read at refresh time and passed to the Slack API via `Authorization: Bearer` header.

## How It Works

1. **Channel discovery:** Fetches all channels the user belongs to (public, private, IM, group DM) via `users.conversations`
2. **Activity filter:** Only fetches history from channels with activity in the last 15 hours (all IMs are always included since Slack doesn't reliably report their `updated` timestamp)
3. **Commitment scanning:** Scans messages for commitment language phrases
4. **Search supplement:** Also runs `search.messages` queries to catch DM/group-DM messages that the channel-based scan might miss
5. **Deduplication:** Deduplicates candidates by permalink
6. **LLM extraction:** Sends candidates to the LLM to determine which contain real commitments and extracts structured todos
7. **Incremental sync:** Only processes messages newer than the last successful sync timestamp (with a 2-minute overlap buffer)

## Verification

1. Run `ai-cron -verbose` and look for `slack:` log lines showing:
   - Number of channels found and their type breakdown
   - Number of candidates found
   - Number of new candidates processed
   - Number of todos extracted
2. Todos from Slack should appear in the TUI with source "slack" and permalink as context

## Common Issues

| Problem | Cause | Fix |
|---------|-------|-----|
| "slack auth: bot token not configured" | Token field is empty in config | Set `token` in `config.yaml` |
| "missing_scope" errors in logs | Token lacks required OAuth scopes | Add the needed scopes to the Slack app and reinstall |
| 0 candidates found | No messages with commitment language in the last 15 hours, or token has no channel access | Verify token scopes; check that the token owner is a member of the target channels |
| Candidates found but 0 todos | LLM is nil or LLM extraction failed | Ensure an LLM is configured (`refresh.model` in config); check for LLM error in logs |
| Rate limiting (429) | Too many API calls | The source retries automatically with `Retry-After` backoff (up to 3 attempts). If persistent, reduce the number of channels or increase refresh interval |
| "auth.test error" | Invalid or expired token | Generate a new Slack token |
| DMs not scanned | Bot token without IM scope, or `search:read` fallback missing | Use a user token, or ensure `search:read` scope is present for the fallback path |
