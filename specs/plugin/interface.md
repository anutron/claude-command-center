# SPEC: Plugin Interface

## Purpose

Define the core Plugin interface and supporting types that all plugins (built-in and external) must implement. This is the contract between the TUI host and plugins.

## Interface

### Plugin Interface

```go
type Plugin interface {
    Slug() string
    TabName() string
    Init(ctx Context) error
    Shutdown()
    Migrations() []Migration
    View(width, height, frame int) string
    KeyBindings() []KeyBinding
    HandleKey(msg tea.KeyMsg) Action
    HandleMessage(msg tea.Msg) (handled bool, action Action)
    Routes() []Route
    NavigateTo(route string, args map[string]string)
    RefreshInterval() time.Duration
    Refresh() tea.Cmd
}
```

### Starter (optional)

Plugins that need to run initial tea.Cmds (e.g., spinner ticks):
```go
type Starter interface {
    StartCmds() tea.Cmd
}
```

### SettingsProvider (optional)

Plugins or data sources that want to render their own settings detail view instead of the default generic view:
```go
type SettingsProvider interface {
    SettingsView(width, height int) string
    HandleSettingsKey(msg tea.KeyMsg) Action
    SettingsOpenCmd() tea.Cmd
    HandleSettingsMsg(msg tea.Msg) (bool, Action)
}
```

- `SettingsOpenCmd()` is called when the user navigates to this provider's settings pane. Returns a `tea.Cmd` for async initialization (e.g., a live credential check). Return nil if no async work is needed.
- `HandleSettingsMsg(msg)` routes a `tea.Msg` to the provider for async result handling. Returns `(handled, action)` -- `handled=true` means the message was consumed by this provider.

### DoctorProvider (optional)

Plugins that can diagnose their own credential and configuration health:
```go
type DoctorProvider interface {
    DoctorChecks(opts DoctorOpts) []DoctorCheck
}
```

Supporting types:
```go
type DoctorOpts struct {
    Live bool // hit network endpoints (tokeninfo, gh auth, etc.)
}

type DoctorCheck struct {
    Name         string
    Result       ValidationResult
    Inconclusive bool
}

type ValidationResult struct {
    Status  string // "ok", "missing", "incomplete", "no_client"
    Message string
    Hint    string
}
```

- When `Live` is true, checks may hit network endpoints (e.g., Google tokeninfo, GitHub auth status)
- When `Live` is false, checks are limited to local file/config inspection
- `Inconclusive` marks checks whose result could not be determined (e.g., network error during a live check)
- `ValidationResult.Status` values: `"ok"` (healthy), `"missing"` (credential/config not found), `"incomplete"` (partially configured), `"no_client"` (OAuth client ID not available)
- `ValidationResult.Hint` provides a user-facing suggestion for resolution (e.g., "Run `ccc setup gmail` to configure")

When the Settings plugin opens a detail view for an item:
1. It checks its `providers` map (for data source settings registered at init)
2. It checks if the registry plugin implements `SettingsProvider`
3. Falls back to generic views if neither provides one

Key handling flow:
- Settings plugin handles `esc` (close) and `space` (toggle) first
- Provider's `HandleSettingsKey` is called next
- If provider returns `ActionFlash`, settings plugin shows the flash message
- If provider returns `ActionUnhandled`, settings plugin applies generic navigation (up/down)

### ScopeConfig

```go
func ScopeConfig(cfg *config.Config, scopes []string) map[string]interface{}
```

Returns a map containing only the top-level config fields matching the requested scope names. Used by the external plugin adapter to send scoped config during the init handshake.

- Uses reflection to match scope names against the `yaml` struct tags of `config.Config` fields
- Tag parsing: extracts the field name before any comma options (e.g., `yaml:"github,omitempty"` matches scope `"github"`)
- Scope matching is case-insensitive (scopes are lowercased before comparison)
- Fields with empty or `"-"` yaml tags are skipped
- If `scopes` is empty or `cfg` is nil, returns an empty map (secure by default)
- Only top-level fields are matched -- nested config sections are not individually addressable

### Context

Provided to plugins during Init:
```go
type Context struct {
    DB          *sql.DB
    Config      *config.Config
    Styles      *ui.Styles
    Grad        *ui.GradientColors
    Bus         EventBus
    Logger      Logger
    DBPath      string
    LLM         llm.LLM
    AgentRunner agent.Runner
    NotifyPeers func(event string) // sends event to all other running TUI instances via notify sockets
}
```

### Action

Returned by HandleKey/HandleMessage:
```go
type Action struct {
    Type    string            // "noop", "open_url", "flash", "launch", "quit", "navigate", "unhandled"
    Payload string
    Args    map[string]string
    TeaCmd  tea.Cmd
}
```

Action type constants: `ActionNoop`, `ActionConsumed`, `ActionOpenURL`, `ActionFlash`, `ActionLaunch`, `ActionQuit`, `ActionNavigate`, `ActionUnhandled`.

- `ActionConsumed` signals that the plugin handled the key but needs no host action. Unlike `ActionNoop`, this prevents the host from applying its own default behavior for the key (e.g., Tab switching tabs).

### Route

```go
type Route struct {
    Slug        string
    Description string
    ArgKeys     []string
}
```

### Migration

```go
type Migration struct {
    Version int
    SQL     string
}
```

### KeyBinding

```go
type KeyBinding struct {
    Key         string
    Description string
    Mode        string // "" = normal
    Promoted    bool   // true = shown in tab footer
}
```

## Registry

```go
type Registry struct { ... }

func NewRegistry() *Registry
func (r *Registry) Register(p Plugin)
func (r *Registry) All() []Plugin
func (r *Registry) BySlug(slug string) (Plugin, bool)
func (r *Registry) Count() int
func (r *Registry) IndexOf(slug string) int
```

## Behavior

- All plugins are initialized with Init(ctx) before any other method is called
- Shutdown() is called when the TUI exits
- HandleKey receives key events when the plugin's tab is active
- HandleMessage receives all tea.Msg (for background command results)
- View is called every frame to render the plugin's content
- Migrations run once on first init, tracked in ccc_plugin_migrations table
- SettingsProvider implementations are registered with the Settings plugin at init time; they receive the config and palette for rendering

## Test Cases

- Context fields are all non-nil after construction
- Action with Type "noop" is the zero-value behavior
- NoopAction() helper returns correct default
- Registry BySlug returns (nil, false) for unknown slugs
- Registry Register replaces existing plugin with same slug
- SettingsProvider.HandleSettingsKey returning ActionUnhandled falls through to generic handling
- SettingsProvider.HandleSettingsKey returning ActionFlash sets flash message on settings plugin
- SettingsProvider.SettingsOpenCmd returns a Cmd for async init (or nil)
- SettingsProvider.HandleSettingsMsg routes async results to the correct provider
- DoctorProvider.DoctorChecks with Live=true may hit network endpoints
- DoctorProvider.DoctorChecks with Live=false checks only local config
- DoctorCheck.Inconclusive=true marks indeterminate results
- ScopeConfig with empty scopes returns empty map
- ScopeConfig with nil config returns empty map
- ScopeConfig matches yaml tag names case-insensitively
- ScopeConfig skips fields with empty or "-" yaml tags
- ScopeConfig extracts tag name before comma options (e.g., "github,omitempty" matches "github")
- ConsumedAction prevents host default key handling (unlike NoopAction)
