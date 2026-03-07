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

### SetupFlow (optional)

Plugins that need onboarding also implement:
```go
type SetupFlow interface {
    RunSetup() (map[string]interface{}, error)
}
```

### Context

Provided to plugins during Init:
```go
type Context struct {
    DB       *sql.DB
    Config   *config.Config
    Styles   interface{}  // *tui.Styles
    Grad     interface{}  // *tui.GradientColors
    Bus      EventBus
    Logger   Logger
    DBPath   string
}
```

### Action

Returned by HandleKey/HandleMessage:
```go
type Action struct {
    Type    string            // "noop", "open_url", "flash", "launch", "quit", "navigate"
    Payload string
    Args    map[string]string
    TeaCmd  tea.Cmd
}
```

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

## Behavior

- All plugins are initialized with Init(ctx) before any other method is called
- Shutdown() is called when the TUI exits
- HandleKey receives key events when the plugin's tab is active
- HandleMessage receives all tea.Msg (for background command results)
- View is called every frame to render the plugin's content
- Migrations run once on first init, tracked in ccc_plugin_migrations table

## Test Cases

- Context fields are all non-nil after construction
- Action with Type "noop" is the zero-value behavior
- NoopAction() helper returns correct default
