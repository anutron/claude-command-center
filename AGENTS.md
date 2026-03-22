# AGENTS.md -- Setup Guide for Claude Agents

This document is the entry point for any Claude agent setting up CCC (Claude Command Center) from a fresh clone. Follow every step in order. Do not skip steps or improvise.

## System Requirements

Before starting, verify these are installed:

| Dependency | Minimum Version | Check Command |
|-----------|----------------|---------------|
| Go | 1.25+ | `go version` |
| Node.js | 18+ | `node --version` |
| npm | (bundled with Node.js) | `npm --version` |
| git | any recent version | `git --version` |
| macOS | darwin (only supported OS) | `uname -s` |

If any dependency is missing, install it before proceeding:

```bash
# Go -- install from https://go.dev/dl/ or:
brew install go

# Node.js + npm:
brew install node
```

## Installation

### Step 1: Clone the Repository

```bash
git clone https://github.com/anutron/claude-command-center.git
cd claude-command-center
```

### Step 2: Build and Install

```bash
make install
```

This does the following:
- Compiles `ccc` (TUI binary) and `ai-cron` (data fetcher binary)
- Code-signs the `ccc` binary for macOS
- Builds MCP servers (Gmail -- requires Node.js)
- Symlinks `ccc` and `ai-cron` to `/usr/local/bin/`
- Symlinks the pomodoro plugin to `/usr/local/bin/ccc-pomodoro`
- Installs Claude skills to `~/.claude/skills/`

Verify the install succeeded:

```bash
ccc --help
```

You should see the help text listing available commands (setup, doctor, etc.).

### Step 3: Run Interactive Setup

```bash
ccc setup
```

This launches the TUI with an onboarding wizard that walks through initial configuration. It creates the config file at `~/.config/ccc/config.yaml` and the database at `~/.config/ccc/data/ccc.db`.

### Step 4: Install Background Refresh (Optional)

To keep data sources updated automatically via launchd:

```bash
ccc install-schedule
```

To remove it later: `ccc uninstall-schedule`.

## What to Expect on First Launch

After setup, run `ccc` to launch the TUI. With zero data sources configured:

- The **Sessions** tab shows your Claude coding sessions (populated as you use `ccc` to launch Claude)
- The **Command Center** tab shows todos (empty until you add some)
- The **Settings** tab lets you configure data sources and preferences
- Calendar, GitHub, Slack, Gmail, and Granola tabs appear only after you enable their respective sources in settings or `config.yaml`

Data sources are independent and additive. Enable them one at a time as needed. See "Adding Data Sources" below.

## Fallback: Manual config.yaml

If `ccc setup` fails or you prefer manual configuration, create the config file directly. This minimal config is valid and launches CCC with no data sources:

```yaml
name: "My Command Center"
palette: aurora

calendar:
  enabled: false
  calendars: []

github:
  enabled: false
  repos: []
  username: ""

gmail:
  enabled: false

slack:
  enabled: false

granola:
  enabled: false

todos:
  enabled: true

agent:
  default_budget: 5.0
  default_permission: default
  default_mode: normal
  max_concurrent: 3
```

Save this to `~/.config/ccc/config.yaml`. The directory `~/.config/ccc/data/` will be created automatically on first run.

You can also override these paths with environment variables:
- `CCC_CONFIG_DIR` -- overrides `~/.config/ccc` (config directory)
- `CCC_STATE_DIR` -- overrides `~/.config/ccc/data` (database and logs)

## Adding Data Sources

Each data source has its own setup requirements (OAuth credentials, API tokens, etc.). Enable them by setting `enabled: true` in `config.yaml` and providing the required credentials.

Detailed per-source setup guides:

- [Google Calendar](docs/sources/calendar.md) -- OAuth flow, calendar ID configuration
- [GitHub](docs/sources/github.md) -- GitHub CLI auth, repo tracking
- [Gmail](docs/sources/gmail.md) -- OAuth flow, MCP server setup
- [Slack](docs/sources/slack.md) -- Bot token setup
- [Granola](docs/sources/granola.md) -- Account connection

After enabling a source, run a manual refresh to pull data immediately:

```bash
ai-cron -v
```

## Running Tests

```bash
make test
```

This runs `go test -v ./...` across the entire project.

## Health Check

To verify data source connectivity and configuration:

```bash
ccc doctor         # Offline checks only
ccc doctor --live  # Include network connectivity checks
```

## Troubleshooting

### Build fails: `go build` errors

**Symptom:** `make build` or `make install` fails with Go compilation errors.

**Fix:**
1. Verify Go version: `go version` (must be 1.25+)
2. Download dependencies: `go mod download`
3. If `go mod download` fails with network errors, check your internet connection and any proxy settings (`GOPROXY`, `GONOSUMCHECK`)
4. Try clearing the module cache: `go clean -modcache` then `go mod download`

### Build fails: `npm install` or `npm run build` errors

**Symptom:** `make install` fails during the MCP server build step.

**Fix:**
1. Verify Node.js version: `node --version` (must be 18+)
2. Try building servers separately: `cd servers/gmail && npm install && npm run build`
3. If you do not need Gmail integration, you can skip this by running `make build` instead of `make install`, then manually symlinking the binaries

### Permission denied on `/usr/local/bin`

**Symptom:** `make install` fails with "Permission denied" when creating symlinks.

**Fix:**
```bash
sudo make install
```

Or create the symlinks manually:
```bash
ln -sf $(pwd)/ccc /usr/local/bin/ccc
ln -sf $(pwd)/ai-cron /usr/local/bin/ai-cron
```

### Config file parse error

**Symptom:** `Error: could not load config` on startup.

**Fix:**
1. Check YAML syntax in `~/.config/ccc/config.yaml`
2. A backup may exist at `~/.config/ccc/config.yaml.bak`
3. To start fresh, delete the config file and run `ccc setup`

### Database errors

**Symptom:** `Error: could not open database` on startup.

**Fix:**
1. Check that `~/.config/ccc/data/` exists and is writable
2. If the database is corrupted, delete `~/.config/ccc/data/ccc.db` and restart (data will be re-fetched on next refresh)

### codesign fails

**Symptom:** `codesign -s -` fails during `make build`.

**Fix:** This is macOS-specific code signing. If it fails, you can still run the binary -- the signing is for clean integration with macOS security. Try running `make build` again. If it persists, check that Xcode command line tools are installed: `xcode-select --install`.

## Plugin Development

CCC supports external plugins that run as subprocesses communicating over JSON-lines (stdin/stdout). See the plugin developer guide:

- [Plugin Development Guide](docs/plugin-development.md) -- architecture, protocol reference, tutorial
- [Pomodoro Example](examples/pomodoro/) -- working reference plugin in Python
- [Plugin Protocol Spec](specs/plugin/protocol.md) -- formal protocol specification

To register an external plugin, add it to `config.yaml`:

```yaml
external_plugins:
  - name: my-plugin
    command: /path/to/my-plugin
    description: "What my plugin does"
    enabled: true
```

## Automations

Automations are headless scripts that run during `ai-cron` cycles. Unlike plugins (which have a TUI tab), automations operate in the background with no UI footprint. A Python SDK is included for writing automations.

To register an automation, add it to `config.yaml`:

```yaml
automations:
  - name: calendar-accept
    schedule: "*/5 * * * *"          # cron expression
    command: python3
    args: ["/path/to/calendar_accept.py"]
    enabled: true
    env:
      ACCEPT_DOMAINS: "mycompany.com,partner.com"
```

For full details — SDK usage, scheduling, environment variables, and the included `calendar-accept` example — see [docs/automations.md](docs/automations.md).

## Key Paths Reference

| Path | Purpose |
|------|---------|
| `~/.config/ccc/config.yaml` | Main configuration file |
| `~/.config/ccc/data/ccc.db` | SQLite database (WAL mode) |
| `~/.config/ccc/data/ccc.log` | Application log |
| `~/.config/ccc/credentials/` | OAuth credentials directory |
| `/usr/local/bin/ccc` | TUI binary (symlink) |
| `/usr/local/bin/ai-cron` | Data fetcher binary (symlink) |
