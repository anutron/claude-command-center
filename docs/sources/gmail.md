# Source: Gmail

## What It Provides

- **Todos:** Emails with a configured Gmail label are synced as active todos (subject line becomes todo title, sender becomes context)
- **Auto-labeling (advanced mode):** LLM-based commitment detection scans recent emails and auto-applies the todo label
- **Label cleanup (advanced mode):** When a todo is marked completed, the label is automatically removed from the email on the next refresh

CCC's Gmail integration is **read-only** in standard mode. In advanced mode, it can modify labels and create drafts, but it **never sends, deletes, or trashes emails**. This is an enforced safety constraint.

## Prerequisites

1. A Google Cloud Platform (GCP) project with the Gmail API enabled
2. An OAuth 2.0 client ID (type: Desktop app) from that GCP project
3. A Gmail label to use as the todo trigger (e.g., "CCC/Todo")
4. OAuth tokens stored at `~/.gmail-mcp/work.json`

## Step-by-Step Setup

### 1. Obtain Google OAuth Client Credentials

Same process as Calendar -- create a Desktop OAuth client in GCP Console with Gmail API enabled. The client ID and secret can be the same as Calendar's or separate.

### 2. Run the Gmail MCP Server Auth

The Gmail MCP server (in `servers/gmail/`) has its own auth flow:

```bash
cd servers/gmail
npm install
npm run auth work
```

This runs an OAuth flow that saves credentials to `~/.gmail-mcp/work.json`.

### 3. Credential File Format

The token file at `~/.gmail-mcp/work.json`:

```json
{
  "clientId": "your-client-id.apps.googleusercontent.com",
  "clientSecret": "your-client-secret",
  "access_token": "ya29...",
  "refresh_token": "1//...",
  "token_type": "Bearer",
  "expiry_date": 1710000000000
}
```

If `clientId` is not embedded in the file, the source falls back to the `GMAIL_CLIENT_ID` and `GMAIL_CLIENT_SECRET` environment variables.

### 4. Configure `config.yaml`

**Standard mode (read-only):**

```yaml
gmail:
  enabled: true
  todo_label: "CCC/Todo"
```

**Advanced mode (label modification + commitment detection):**

```yaml
gmail:
  enabled: true
  todo_label: "CCC/Todo"
  advanced: true
```

Fields:

- `enabled` (required): Set to `true` to activate the source.
- `todo_label` (optional): The Gmail label name whose emails become todos. If empty, the label-based todo feature is disabled.
- `advanced` (optional): Defaults to `false`. When `true`, requests `gmail.modify` and `gmail.compose` scopes instead of `gmail.readonly`. Enables:
  - Automatic removal of the todo label from emails when their corresponding todo is marked completed
  - LLM-based commitment detection that auto-labels emails containing commitments

### OAuth Scopes

- Standard mode: `gmail.readonly`
- Advanced mode: `gmail.modify`, `gmail.compose`

## Verification

1. Run `ai-cron -verbose` and look for `gmail:` log lines
2. Emails with the configured label should appear as todos in the TUI
3. In Settings, Gmail credentials should show "Token found" / "Configured"
4. The doctor check validates:
   - Structural: `~/.gmail-mcp/work.json` exists and contains valid JSON with client credentials
   - Live: Hits `https://oauth2.googleapis.com/tokeninfo` to verify token validity

## Common Issues

| Problem | Cause | Fix |
|---------|-------|-----|
| "no gmail token at ~/.gmail-mcp/work.json" | Token file missing | Run the Gmail MCP auth flow: `cd servers/gmail && npm run auth work` |
| "gmail token missing clientId and GMAIL_CLIENT_ID not set" | Token file exists but has no client credentials and env var not set | Add `clientId`/`clientSecret` to `work.json` or set `GMAIL_CLIENT_ID`/`GMAIL_CLIENT_SECRET` env vars |
| "invalid_client" | GCP OAuth client ID deleted or disabled | Create a new OAuth client in GCP and re-run auth |
| No todos appearing | `todo_label` not set, or no emails have that label | Set `todo_label` in config and apply the label to at least one email in Gmail |
| "label modification requires advanced mode" | Tried to modify labels without `advanced: true` | Set `advanced: true` in config and re-auth with modify scope |
| Label not found | The label name in config doesn't match any Gmail label (case-insensitive match) | Verify the exact label name in Gmail settings |
