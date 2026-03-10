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
}
```

When the Settings plugin opens a detail view for an item:
1. It checks its `providers` map (for data source settings registered at init)
2. It checks if the registry plugin implements `SettingsProvider`
3. Falls back to generic views if neither provides one

Key handling flow:
- Settings plugin handles `esc` (close) and `space` (toggle) first
- Provider's `HandleSettingsKey` is called next
- If provider returns `ActionFlash`, settings plugin shows the flash message
- If provider returns `ActionUnhandled`, settings plugin applies generic navigation (up/down)

### Context

Provided to plugins during Init:
```go
type Context struct {
    DB     *sql.DB
    Config *config.Config
    Styles *ui.Styles
    Grad   *ui.GradientColors
    Bus    EventBus
    Logger Logger
    DBPath string
    LLM    llm.LLM
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

Action type constants: `ActionNoop`, `ActionOpenURL`, `ActionFlash`, `ActionLaunch`, `ActionQuit`, `ActionNavigate`, `ActionUnhandled`.

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
