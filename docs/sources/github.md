# Source: GitHub

## What It Provides

- **Context fetching:** Retrieves PR and issue details (title, body, comments, reviews) for todos that reference GitHub URLs
- **Settings UI:** Browse and select repos, configure username, toggle "Track My PRs" mode

Note: The GitHub source currently returns an empty result from `Fetch()` (the threads feature that surfaced PRs as items has been removed). It remains as infrastructure for future GitHub integration. The context fetcher and settings UI are fully functional.

## Prerequisites

1. The GitHub CLI (`gh`) installed and available on `$PATH`
2. `gh` authenticated with a GitHub account that has access to the target repos

## Step-by-Step Setup

### 1. Install the GitHub CLI

```bash
brew install gh
```

### 2. Authenticate

```bash
gh auth login
```

Follow the interactive prompts. Choose HTTPS protocol and authenticate via browser.

### 3. Verify Authentication

```bash
gh auth token
```

This should print a token without error. The doctor check uses this exact command.

### 4. Configure `config.yaml`

```yaml
github:
  enabled: true
  username: your-github-username
  # track_my_prs: true      # Default: true. Shows all PRs you're involved in.
  # repos:                   # Only used when track_my_prs is false.
  #   - owner/repo-name
  #   - org/another-repo
```

Fields:

- `enabled` (required): Set to `true` to activate the source.
- `username` (required for PR tracking): Your GitHub login. Auto-detected via `gh api /user` when opening Settings.
- `track_my_prs` (optional): Defaults to `true`. When on, tracks all PRs where you are author, assignee, or review-requested. When off, only tracks PRs in the explicitly listed `repos`.
- `repos` (optional): List of `owner/repo` strings. Only relevant when `track_my_prs` is `false`. Can be managed via the Settings TUI (press `f` to browse repos from GitHub).

## Verification

1. Run `gh auth token` -- should succeed without error
2. In the TUI Settings > GitHub, status should show "Authenticated"
3. Username should auto-populate if not set

## Common Issues

| Problem | Cause | Fix |
|---------|-------|-----|
| "GitHub CLI not authenticated" in doctor | `gh auth token` fails | Run `gh auth login` |
| Username not auto-detected | `gh api /user` fails | Set `username` manually in config or run `gh auth login` |
| Repo fetch returns empty | User has no repos, or auth token lacks `repo` scope | Check `gh auth status` for scopes |
| Context fetch fails for a PR | PR URL is not a valid `owner/repo#number` format | Verify the `source_ref` stored in the todo |
