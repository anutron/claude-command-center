# Source: Granola

## What It Provides

- **Todos:** Extracts commitments and action items from meeting transcripts using LLM analysis
- **Context fetching:** Retrieves full meeting transcripts for todo detail views

Granola (https://granola.so) is a meeting notes app that records and transcribes meetings. CCC reads Granola's local credential store and uses its API to fetch meeting data.

## Prerequisites

1. The Granola desktop app installed and signed in on macOS
2. At least one recorded meeting in Granola
3. An LLM configured in CCC (required for commitment extraction from transcripts)

## Step-by-Step Setup

### 1. Install and Sign In to Granola

1. Download Granola from https://granola.so
2. Install and open the app
3. Sign in with your account
4. Record at least one meeting so there is data to process

### 2. Verify Credential File Exists

Granola stores its auth tokens automatically at:

```
~/Library/Application Support/Granola/stored-accounts.json
```

This file is created and maintained by the Granola desktop app. You do not need to create or edit it manually. The file structure is:

```json
{
  "accounts": "[{\"userId\":\"...\",\"email\":\"...\",\"tokens\":\"{\\\"access_token\\\":\\\"...\\\",\\\"refresh_token\\\":\\\"...\\\",\\\"expires_in\\\":3600}\",\"savedAt\":1710000000000}]"
}
```

Note: The `accounts` field is a JSON string containing a JSON array (double-encoded). CCC handles this parsing automatically.

### 3. Configure `config.yaml`

```yaml
granola:
  enabled: true
```

Fields:

- `enabled` (required): Set to `true` to activate the source. This is the only configuration needed.

There are no additional fields -- Granola auth is entirely handled by the desktop app's local credential store.

## How It Works

1. **Auth:** Reads the access token from `~/Library/Application Support/Granola/stored-accounts.json`, using the first account found
2. **Token expiry check:** Validates that the token hasn't expired based on `savedAt` + `expires_in`. If expired, returns an error directing the user to open Granola to refresh.
3. **Meeting list:** Calls `POST /v2/get-documents` to list meetings from the current week (Sunday start)
4. **Transcript fetch:** For each meeting, calls `POST /v1/get-document-transcript` to get the full transcript
5. **Incremental sync:** Only processes meetings newer than the last successful sync timestamp
6. **LLM extraction:** Sends new meeting transcripts to the LLM to extract commitments as structured todos
7. **Transcript format:** Transcripts are formatted with speaker labels (`[Aaron]` for microphone input, `[Other]` for system/remote participants)

## Verification

1. Run `ccc-refresh -verbose` and look for `granola:` log lines showing:
   - Number of meetings found
   - Number of new meetings to process (vs. skipped)
   - Per-meeting transcript and summary character counts
   - Number of todos extracted
2. Todos from Granola should appear in the TUI with source "granola"
3. In Settings > Granola, credentials should show "Token found"

## Common Issues

| Problem | Cause | Fix |
|---------|-------|-----|
| "no granola auth" | `stored-accounts.json` doesn't exist | Open the Granola desktop app and sign in |
| "no granola accounts found" | File exists but contains no accounts | Sign in to Granola again |
| "granola access token is empty" | Account entry has no access token | Sign out and back into Granola |
| "granola token expired at ..." | The stored access token has expired | Open the Granola desktop app -- it refreshes tokens automatically on launch |
| 0 meetings found | No meetings this week, or all meetings are deleted | Record a meeting in Granola, or check that meetings exist for the current week |
| Meetings found but 0 todos | LLM is nil or extraction failed | Ensure an LLM is configured; check for LLM error in logs |
| "granola /v2/get-documents: HTTP 401" | Token is valid but Granola API rejects it | Open Granola app to refresh the token |
| macOS-only path | CCC assumes `~/Library/Application Support/Granola/` | This source is macOS-only; the path is hardcoded |
