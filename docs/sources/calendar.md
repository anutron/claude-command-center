# Source: Google Calendar

## What It Provides

- **Sessions tab:** Today's and tomorrow's calendar events with start/end times, titles, and declined status
- **Auto-accept:** Available via the `calendar-accept` automation (a separate scheduled script, not a built-in calendar feature). See [docs/automations.md](../automations.md)
- **Actions:** Supports pending calendar actions (e.g., booking time blocks) executed after data merge

## Prerequisites

1. A Google Cloud Platform (GCP) project with the Google Calendar API enabled
2. An OAuth 2.0 client ID (type: Desktop app) from that GCP project
3. The client ID and client secret available via one of these methods:
   - Environment variables `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`
   - `~/.claude.json` under `mcpServers.google-calendar.env`
   - Stored directly in `~/.config/google-calendar-mcp/credentials.json`

## Step-by-Step Setup

### 1. Obtain Google OAuth Client Credentials

If the user does not already have a GCP project with Calendar API enabled:

1. Go to https://console.cloud.google.com/
2. Create a project (or select an existing one)
3. Enable the Google Calendar API
4. Go to Credentials > Create Credentials > OAuth client ID
5. Application type: Desktop app
6. Copy the Client ID and Client Secret

### 2. Store Client Credentials

Choose one of these methods:

**Method A: Environment variables in `~/.config/ccc/.env`**

```
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
```

**Method B: In `~/.claude.json` (if using Claude Desktop MCP)**

The auth loader reads from `mcpServers.google-calendar.env.GOOGLE_CLIENT_ID` and `mcpServers.google-calendar.env.GOOGLE_CLIENT_SECRET`.

### 3. Run the OAuth Flow

The calendar source includes a built-in OAuth flow (`RunCalendarAuth`). When triggered:

1. It starts a local HTTP server on a random port on `127.0.0.1`
2. Opens the Google consent URL in the default browser
3. After the user approves, captures the authorization code via callback
4. Exchanges the code for tokens using PKCE
5. Saves the complete credentials (client ID, secret, access token, refresh token) to `~/.config/google-calendar-mcp/credentials.json` with mode `0600`

### 4. Configure `config.yaml`

```yaml
calendar:
  enabled: true
  calendars:
    - id: primary
      label: Personal
    - id: team-calendar@group.calendar.google.com
      label: Team
      color: "#7aa2f7"
```

Each calendar entry has:

- `id` (required): The Google Calendar ID. Use `primary` for the user's main calendar, or the full calendar ID for shared/additional calendars.
- `label` (optional): Display name in the TUI. Defaults to the calendar ID.
- `color` (optional): Hex color for visual differentiation in the TUI.
- `enabled` (optional): Defaults to `true`. Set to `false` to hide without removing.

If `calendars` is empty or omitted, the source defaults to fetching only `primary`.

## Credential File Format

The canonical credential file at `~/.config/google-calendar-mcp/credentials.json`:

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

### Legacy Format Migration

If only `~/.config/google-calendar-mcp/token.json` exists (older format without embedded client credentials), the source auto-migrates it to `credentials.json` on first run, provided client credentials are available via environment variables or `~/.claude.json`.

## Verification

1. Run `ccc-refresh -verbose` and check for calendar fetch output
2. In the TUI, the Sessions tab should show today's events
3. In the TUI Settings > Calendar, credentials should show "Configured"
4. The doctor check (`Settings > Calendar > r` to refresh) validates:
   - Structural check: `credentials.json` or `token.json` exists with required fields
   - Live check: Hits `https://oauth2.googleapis.com/tokeninfo` to verify the access token is valid

## Common Issues

| Problem | Cause | Fix |
|---------|-------|-----|
| "no calendar token found" | Neither `credentials.json` nor `token.json` exists | Run the OAuth flow or place credentials manually |
| "no Google Calendar client credentials" | Token exists but no client ID/secret available | Set `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET` env vars or add to `credentials.json` |
| "invalid_client" error | The GCP OAuth client ID has been deleted or disabled | Create a new OAuth client ID in GCP Console and re-run auth |
| Token expired, no refresh | The refresh token was revoked or the GCP app's consent was withdrawn | Re-run the full OAuth flow to get a new refresh token |
| Events missing | Calendar ID not in config, or per-calendar `enabled: false` | Check `config.yaml` calendar entries; use Settings TUI to browse and toggle |
| "Token invalid (HTTP 401)" in doctor | Access token expired and refresh failed | Re-run the OAuth flow |
