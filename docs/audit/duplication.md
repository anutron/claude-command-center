# Code Duplication Audit

## Summary

The codebase has significant duplication in three areas: (1) gradient/animation code is copy-pasted across three packages, (2) style struct construction repeats the same palette-to-lipgloss conversion in every plugin, and (3) LLM response parsing follows an identical extract-JSON-then-unmarshal pattern across five call sites. Plugin boilerplate is moderate -- each plugin re-stores the same Context fields and builds styles identically.

## Exact/Near Duplicates

### 1. `pulsingPointerStyle` -- identical function in 3 places

The pulsing pointer animation is copy-pasted with only type differences (`*GradientColors` vs `*gradientColors`):

**`internal/tui/effects.go:94-99`**
```go
func pulsingPointerStyle(g *GradientColors, frame int) lipgloss.Style {
    phase := float64(frame) / float64(pulsePeriod) * 2.0 * math.Pi
    brightness := 0.7 + 0.3*math.Sin(phase)
    c := g.DimCyan.BlendLab(g.BrightCyan, brightness)
    return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
}
```

**`internal/builtin/commandcenter/styles.go:129-134`**
```go
func pulsingPointerStyle(g *gradientColors, frame int) lipgloss.Style {
    phase := float64(frame) / float64(pulsePeriod) * 2.0 * math.Pi
    brightness := 0.7 + 0.3*math.Sin(phase)
    c := g.DimCyan.BlendLab(g.BrightCyan, brightness)
    return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
}
```

**`internal/builtin/sessions/sessions.go:131-136`**
```go
func pulsingPointerStyle(g *gradientColors, frame int) lipgloss.Style {
    phase := float64(frame) / float64(pulsePeriod) * 2.0 * math.Pi
    brightness := 0.7 + 0.3*math.Sin(phase)
    c := g.DimCyan.BlendLab(g.BrightCyan, brightness)
    return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
}
```

### 2. `gradientColors` struct and `newGradientColors` -- identical in 3 places

**`internal/tui/effects.go:22-49`** (exported as `GradientColors` / `NewGradientColors`)
**`internal/builtin/commandcenter/styles.go:96-121`** (unexported `gradientColors` / `newGradientColors`)
**`internal/builtin/sessions/sessions.go:116-129`** (unexported `gradientColors` / `newGradientColors`, subset with only DimCyan/BrightCyan)

The commandcenter version is a field-for-field copy of the tui version. The sessions version is a simplified subset. All three parse the same palette colors with `colorful.Hex()` and compute dim/bright variants identically.

### 3. `pulsePeriod` constant -- defined in 3 places

- `internal/tui/effects.go:18` -- `pulsePeriod = 54`
- `internal/builtin/commandcenter/styles.go:125` -- `pulsePeriod = 54`
- `internal/builtin/sessions/sessions.go:33` -- `pulsePeriod = 54`

### 4. `contentMaxWidth` constant -- defined in 4 places

- `internal/tui/styles.go:8` -- `contentMaxWidth = 120`
- `internal/builtin/commandcenter/commandcenter.go:23` -- `contentMaxWidth = 120`
- `internal/builtin/sessions/sessions.go:30` -- `contentMaxWidth = 120`
- `internal/builtin/settings/settings.go:18` -- `contentMaxWidth = 120`

### 5. `formatTodoContext` -- near-duplicate in 2 places

**`internal/builtin/sessions/sessions.go:753-775`**
```go
func formatTodoContext(todo db.Todo) string {
    var parts []string
    parts = append(parts, fmt.Sprintf("## Task: %s\n", todo.Title))
    if todo.Context != "" {
        parts = append(parts, fmt.Sprintf("**Context:** %s", todo.Context))
    }
    // ... identical structure ...
    if todo.Due != "" {
        parts = append(parts, fmt.Sprintf("**Due:** %s", todo.Due))
    }
    // ...
}
```

**`internal/builtin/commandcenter/claude_exec.go:255-278`**
```go
func formatTodoContext(todo db.Todo) string {
    var parts []string
    parts = append(parts, fmt.Sprintf("## Task: %s\n", todo.Title))
    if todo.Context != "" {
        parts = append(parts, fmt.Sprintf("**Context:** %s", todo.Context))
    }
    // ... identical structure ...
    if todo.Due != "" {
        label := db.FormatDueLabel(todo.Due)
        parts = append(parts, fmt.Sprintf("**Due:** %s (%s)", todo.Due, label))
    }
    // ...
}
```

Nearly identical -- the commandcenter version adds a `FormatDueLabel` enrichment in the Due field, but the structure is the same.

### 6. `tickMsg` type -- defined in 2 places

- `internal/tui/effects.go:52` -- `type tickMsg time.Time`
- `internal/builtin/commandcenter/commandcenter.go:53` -- `type tickMsg time.Time`

The commandcenter plugin also uses the `tickMsg` from tui (they are different types despite identical definitions, but the host broadcasts to plugins via `HandleMessage`).

### 7. `extractJSON` vs `cleanJSON` -- similar JSON-cleaning logic

**`internal/builtin/commandcenter/claude_exec.go:230-253`** (`extractJSON`)
```go
func extractJSON(s string) string {
    s = strings.TrimSpace(s)
    if strings.HasPrefix(s, "```") {
        // strip markdown fences
    }
    start := strings.Index(s, "{")
    end := strings.LastIndex(s, "}")
    // ...
}
```

**`internal/refresh/llm.go:127-133`** (`cleanJSON`)
```go
func cleanJSON(s string) string {
    s = strings.TrimSpace(s)
    s = strings.TrimPrefix(s, "```json")
    s = strings.TrimPrefix(s, "```")
    s = strings.TrimSuffix(s, "```")
    return strings.TrimSpace(s)
}
```

Both attempt to strip markdown code fences from LLM output. The `extractJSON` version is more robust (handles multi-line fences and extracts by brace matching), but they serve the same purpose.

## Repeated Patterns

### 1. LLM prompt-then-parse pattern (5 occurrences)

Every LLM call follows an identical structure: build prompt string, call `l.Complete(ctx, prompt)`, strip markdown fences, `json.Unmarshal` into an anonymous struct, map fields into `db.Todo` or other types. Found in:

- `internal/refresh/llm.go:13-87` -- `extractCommitments` (granola)
- `internal/refresh/llm.go:89-125` -- `generateSuggestions`
- `internal/refresh/slack.go:205-283` -- `extractSlackCommitments`
- `internal/builtin/commandcenter/claude_exec.go:38-77` -- 4 separate `claude*Cmd` functions

The anonymous struct for LLM JSON responses is repeated:
```go
var items []struct {
    Title      string `json:"title"`
    SourceRef  string `json:"source_ref"`
    Context    string `json:"context"`
    Detail     string `json:"detail"`
    WhoWaiting string `json:"who_waiting"`
    Due        string `json:"due"`
}
```
This appears in `internal/refresh/llm.go:60-67` and `internal/refresh/slack.go:256-263` -- nearly identical.

### 2. `now := FormatTime(time.Now())` at start of every DB write (12+ occurrences)

Almost every DB write function in `internal/db/db.go` starts with:
```go
now := FormatTime(time.Now())
```

Found at lines: 355, 362, 368, 380, 387, 394, 417, 448, 471, 485, 492, 499, 506, 524, 625, 734, 745.

### 3. `completedAt *time.Time` nil-check-then-format pattern (4 occurrences)

```go
var completedAt *string
if t.CompletedAt != nil {
    s := FormatTime(*t.CompletedAt)
    completedAt = &s
}
```

Found in:
- `internal/db/db.go:399-403` (DBInsertTodo)
- `internal/db/db.go:418-421` (DBUpdateTodo)
- `internal/db/db.go:636-639` (DBSaveRefreshResult todos)
- `internal/db/db.go:663-666` (DBSaveRefreshResult threads, for `pausedAt` and `completedAt`)

### 4. Plugin Init storing Context fields (3 occurrences)

Every plugin's `Init` method unpacks the same Context fields:

**sessions/sessions.go:289-293**
```go
p.db = ctx.DB
p.cfg = ctx.Config
p.bus = ctx.Bus
p.logger = ctx.Logger
```

**commandcenter/commandcenter.go:219-222**
```go
p.database = ctx.DB
p.cfg = ctx.Config
p.bus = ctx.Bus
p.logger = ctx.Logger
```

**settings/settings.go:95-97**
```go
p.cfg = ctx.Config
p.logger = ctx.Logger
p.bus = ctx.Bus
```

### 5. Style construction from palette (3 occurrences)

Each plugin has its own styles struct and constructor that converts `config.Palette` fields to `lipgloss.Color` and builds identical styles:

- `internal/tui/styles.go:57-115` -- `NewStyles(p config.Palette) Styles`
- `internal/builtin/commandcenter/styles.go:55-93` -- `newCCStyles(p config.Palette) ccStyles`
- `internal/builtin/sessions/sessions.go:90-110` -- `newSessionStyles(p config.Palette) sessionStyles`
- `internal/builtin/settings/settings.go:1005-1022` -- `newSettingsStyles(p config.Palette) settingsStyles`

Many styles are repeated across these structs:
- `ActiveTab: lipgloss.NewStyle().Foreground(colorCyan).Bold(true)` -- in all 4
- `InactiveTab/Hint/Muted: lipgloss.NewStyle().Foreground(colorMuted)` -- in all 4
- `SectionHeader: lipgloss.NewStyle().Foreground(colorCyan).Bold(true)` -- in tui, commandcenter
- `SelectedItem: lipgloss.NewStyle().Foreground(colorWhite).Background(colorSelectedBg)` -- in tui, commandcenter, sessions
- `TitleBoldC/TitleBoldW` -- in tui, commandcenter, sessions
- `DescMuted` -- in tui, commandcenter, sessions
- `PanelBorder` with `#3b4261` hardcoded -- in tui, commandcenter, settings

### 6. `DueStyle` method -- duplicated between tui and commandcenter

**`internal/tui/styles.go:118-129`**
```go
func (s *Styles) DueStyle(urgency string) lipgloss.Style {
    switch urgency {
    case "overdue": return s.DueOverdue
    case "soon":    return s.DueSoon
    case "later":   return s.DueLater
    default:        return s.DueLater
    }
}
```

**`internal/builtin/commandcenter/styles.go:42-53`** -- identical logic, different receiver type.

### 7. `viewWidth` clamping pattern (3+ occurrences)

```go
viewWidth := contentMaxWidth
if width > 0 && width < viewWidth {
    viewWidth = width
}
```

Found in:
- `internal/builtin/commandcenter/commandcenter.go:1231-1234` (viewCommandTab)
- `internal/builtin/commandcenter/commandcenter.go:1278-1281` (viewThreadsTab)
- `internal/builtin/settings/settings.go:764-767` (View)

### 8. Spinner setup pattern (2 occurrences)

```go
s := spinner.New()
s.Spinner = spinner.MiniDot
s.Style = lipgloss.NewStyle().Foreground(colorCyan)
p.spinner = s
```

Found in:
- `internal/builtin/sessions/sessions.go:322-325`
- `internal/builtin/commandcenter/commandcenter.go:255-258`

### 9. `calendar_id TEXT NOT NULL DEFAULT ''` insert pattern for calendar events (3 occurrences)

The calendar insert SQL is repeated:
- `internal/db/db.go:449-458` (DBReplaceCalendar - today loop)
- `internal/db/db.go:461-465` (DBReplaceCalendar - tomorrow loop)
- `internal/db/db.go:686-699` (DBSaveRefreshResult - today and tomorrow loops)

## Plugin Boilerplate

### Common boilerplate every plugin must implement

Each plugin repeats:
1. **Slug/TabName** one-liners -- unavoidable interface methods
2. **Init** unpacking Context fields into local struct fields
3. **Style struct + constructor** converting palette to lipgloss
4. **Gradient colors struct + constructor** for animations
5. **Spinner setup** (sessions + commandcenter)
6. **`Migrations() []plugin.Migration { return nil }`** -- 2 of 3 plugins return nil
7. **`Shutdown() {}`** -- all 3 plugins are no-ops
8. **`RefreshInterval() time.Duration { return 0 }`** -- 2 of 3 return zero
9. **WindowSizeMsg handling** storing width/height -- in all 3 plugins' HandleMessage

### Specific boilerplate sizes

| Plugin | Lines | Style struct | Gradient struct | Style constructor |
|--------|-------|-------------|-----------------|-------------------|
| sessions | 776 | sessionStyles (12 fields) | gradientColors (2 fields) | newSessionStyles |
| commandcenter | 1331 | ccStyles (18 fields) | gradientColors (6 fields) | newCCStyles |
| settings | 1023 | settingsStyles (12 fields) | N/A | newSettingsStyles |
| tui (host) | 357 | Styles (26 fields) | GradientColors (6 fields) | NewStyles |

The tui `Styles` struct is a superset that could serve all plugins, but plugins can't import it due to circular dependency (`tui` imports `plugin`, plugins can't import `tui`). The current workaround is that `plugin.Context.Styles` is `interface{}`, which plugins could type-assert -- but none do. Instead, they all build their own.

## Recommendations

### High Impact

1. **Extract gradient/animation code into a shared package** (e.g., `internal/ui/anim`). Move `gradientColors`, `newGradientColors`, `pulsingPointerStyle`, and `pulsePeriod` to a single location. All three packages import the same thing. This eliminates ~100 lines of duplication.

2. **Create a shared styles package** (e.g., `internal/ui/styles`). Move the palette-to-lipgloss conversion out of `tui` (which causes the circular dep) and into a leaf package that both `tui` and plugins can import. This would let plugins use a single `Styles` struct instead of defining their own. Eliminates ~200 lines of boilerplate.

3. **Move `contentMaxWidth` to a shared constant** in the styles package or `internal/ui/layout`. Defined 4 times.

4. **Unify `formatTodoContext`** into `internal/db/` (where `Todo` lives) since both sessions and commandcenter need it. The commandcenter version is slightly richer -- use that one.

5. **Unify `extractJSON` and `cleanJSON`** into a single helper (e.g., in an `internal/llm/parse.go` or `internal/util/json.go`). Both strip markdown fences from LLM output.

### Medium Impact

6. **Extract the LLM response parsing pattern** into a generic helper:
   ```go
   func ParseLLMResponse[T any](raw string) (T, error)
   ```
   This would handle fence-stripping and unmarshaling, eliminating the repeated anonymous struct + unmarshal + error formatting across 5 call sites.

7. **Add a `FormatTimePtr` helper** to `internal/db/` that handles the nil-check pattern:
   ```go
   func FormatTimePtr(t *time.Time) *string
   ```
   This would simplify the 4 occurrences of the completedAt/pausedAt format pattern.

8. **Create a `BasePlugin` embed struct** with default no-op implementations for `Shutdown()`, `Migrations()`, and `RefreshInterval()`. This would remove ~15 lines of boilerplate per plugin.

### Low Impact

9. **Consolidate `tickMsg`** -- the commandcenter version shadows the tui version. Since the host broadcasts tui's tickMsg to plugins, the commandcenter shouldn't need its own type. It could react to the host-broadcast tick via HandleMessage instead.

10. **Share spinner setup** as a helper function since sessions and commandcenter use identical code.
