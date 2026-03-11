// Package sessions implements the sessions plugin for CCC.
// It manages two sub-views: "New Session" (browse project paths) and
// "Resume Session" (bookmarked sessions).
package sessions

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)


// ---------------------------------------------------------------------------
// Local item types (no tui import needed)
// ---------------------------------------------------------------------------

// newItem represents a "new session" entry in the launcher list.
type newItem struct {
	path     string
	label    string
	isHome   bool
	isBrowse bool
}

func (i newItem) Title() string       { return i.label }
func (i newItem) Description() string { return i.path }
func (i newItem) FilterValue() string { return i.label + " " + i.path }

// sessionItem represents a paused/resumable session.
type sessionItem struct {
	session db.Session
}

func (i sessionItem) Title() string {
	return i.session.Repo + " (" + i.session.Branch + ")"
}

func (i sessionItem) Description() string {
	date := i.session.Created.Format("Jan 02 15:04")
	if i.session.Summary != "" {
		return date + " -- " + i.session.Summary
	}
	return date
}

func (i sessionItem) FilterValue() string {
	return i.session.Repo + " " + i.session.Branch + " " + i.session.Summary
}

// ---------------------------------------------------------------------------
// Local styles
// ---------------------------------------------------------------------------

type sessionStyles struct {
	activeTab    lipgloss.Style
	inactiveTab  lipgloss.Style
	hint         lipgloss.Style
	sectionHeader lipgloss.Style
	selectedItem lipgloss.Style
	titleBoldC   lipgloss.Style
	titleBoldW   lipgloss.Style
	descMuted    lipgloss.Style
	branchYellow lipgloss.Style
	colorCyan    lipgloss.Color
	colorWhite   lipgloss.Color
}

func newSessionStyles(p config.Palette) sessionStyles {
	colorCyan := lipgloss.Color(p.Cyan)
	colorMuted := lipgloss.Color(p.Muted)
	colorWhite := lipgloss.Color(p.White)
	colorYellow := lipgloss.Color(p.Yellow)
	colorSelectedBg := lipgloss.Color(p.SelectedBg)

	return sessionStyles{
		activeTab:     lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		inactiveTab:   lipgloss.NewStyle().Foreground(colorMuted),
		hint:          lipgloss.NewStyle().Foreground(colorMuted),
		sectionHeader: lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		selectedItem:  lipgloss.NewStyle().Foreground(colorWhite).Background(colorSelectedBg),
		titleBoldC:    lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		titleBoldW:    lipgloss.NewStyle().Foreground(colorWhite).Bold(true),
		descMuted:     lipgloss.NewStyle().Foreground(colorMuted),
		branchYellow:  lipgloss.NewStyle().Foreground(colorYellow),
		colorCyan:     colorCyan,
		colorWhite:    colorWhite,
	}
}


// ---------------------------------------------------------------------------
// Item delegate
// ---------------------------------------------------------------------------

type itemDelegate struct {
	frame  int
	styles *sessionStyles
	grad   *ui.GradientColors
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	selected := index == m.Index()
	width := m.Width() - 4

	var title, desc string
	pointer := "  "
	if selected && d.grad != nil {
		pointer = ui.PulsingPointerStyle(d.grad, d.frame).Render("> ")
	}

	switch it := item.(type) {
	case newItem:
		if it.isHome {
			title = d.styles.titleBoldC.Render("* " + it.Title())
			desc = "  " + d.styles.descMuted.Render(it.path)
		} else if it.isBrowse {
			title = d.styles.titleBoldC.Render("+ " + it.Title())
		} else {
			title = d.styles.titleBoldW.Render(it.Title())
			desc = "  " + d.styles.descMuted.Render(it.path)
		}

	case sessionItem:
		repo := d.styles.titleBoldW.Render(it.session.Repo)
		branch := d.styles.branchYellow.Render(it.session.Branch)
		title = fmt.Sprintf("%s (%s)", repo, branch)
		desc = "  " + d.styles.descMuted.Render(it.session.Created.Format("Jan 02")+" -- "+it.session.Summary)

	default:
		title = item.FilterValue()
	}

	line := title + desc
	line = truncate(line, width)

	if selected {
		line = d.styles.selectedItem.Render(line)
	}

	fmt.Fprintf(w, "%s%s", pointer, line)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	if ansi.StringWidth(s) > max {
		return ansi.Truncate(s, max-1, "...")
	}
	return s
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type sessionsLoadedMsg struct {
	sessions []db.Session
}

type fzfFinishedMsg struct {
	path string
	err  error
}

type fzfProcess struct {
	output string
	stdin  io.Reader
	stderr io.Writer
}

func (f *fzfProcess) SetStdin(r io.Reader)  { f.stdin = r }
func (f *fzfProcess) SetStdout(_ io.Writer) {}
func (f *fzfProcess) SetStderr(w io.Writer) { f.stderr = w }

func (f *fzfProcess) Run() error {
	home, _ := os.UserHomeDir()
	var buf bytes.Buffer
	cmd := exec.Command("fzf",
		"--walker=dir",
		"--walker-root="+home,
		"--walker-skip=.git,node_modules,.venv,__pycache__,.cache,.Trash,Library",
		"--scheme=path",
		"--exact",
		"--ansi",
		"--layout=reverse",
		"--prompt=  path: ",
	)
	cmd.Stdin = f.stdin
	cmd.Stdout = &buf
	cmd.Stderr = f.stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	f.output = strings.TrimSpace(buf.String())
	return nil
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

// Plugin implements plugin.Plugin for session management.
type Plugin struct {
	db     *sql.DB
	cfg    *config.Config
	bus    plugin.EventBus
	logger plugin.Logger

	styles sessionStyles
	grad   ui.GradientColors

	newList       list.Model
	resumeList    list.Model
	paths         []string
	confirming    bool
	confirmYes    bool
	confirmItem   newItem
	confirmResume *sessionItem
	loading       bool
	spinner       spinner.Model
	width         int
	height        int
	subTab        string // "new" or "resume"
	frame         int

	pendingLaunchTodo *db.Todo
}

// Slug returns the plugin identifier.
func (p *Plugin) Slug() string { return "sessions" }

// TabName returns the display name shown in the tab bar.
func (p *Plugin) TabName() string { return "Sessions" }

// Init initialises the plugin with context from the host.
func (p *Plugin) Init(ctx plugin.Context) error {
	p.db = ctx.DB
	p.cfg = ctx.Config
	p.bus = ctx.Bus
	p.logger = ctx.Logger

	pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
	p.styles = newSessionStyles(pal)
	if ctx.Grad != nil {
		p.grad = *ctx.Grad
	} else {
		p.grad = ui.NewGradientColors(pal)
	}

	p.subTab = "new"
	p.loading = true

	paths, _ := db.DBLoadPaths(p.db)
	p.paths = paths

	newItems := p.buildNewItems()

	delegate := itemDelegate{styles: &p.styles, grad: &p.grad}
	nl := list.New(newItems, delegate, 0, 10)
	nl.SetShowTitle(false)
	nl.SetShowStatusBar(false)
	nl.SetFilteringEnabled(true)
	nl.SetShowHelp(false)
	p.newList = nl

	rl := list.New([]list.Item{}, delegate, 0, 10)
	rl.SetShowTitle(false)
	rl.SetShowStatusBar(false)
	rl.SetFilteringEnabled(true)
	rl.SetShowHelp(false)
	p.resumeList = rl

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(p.styles.colorCyan)
	p.spinner = s

	// Subscribe to events
	if p.bus != nil {
		p.bus.Subscribe("pending.todo", func(e plugin.Event) {
			m, ok := e.Payload.(map[string]interface{})
			if !ok {
				return
			}
			title, _ := m["title"].(string)
			context, _ := m["context"].(string)
			detail, _ := m["detail"].(string)
			whoWaiting, _ := m["who_waiting"].(string)
			due, _ := m["due"].(string)
			effort, _ := m["effort"].(string)
			p.pendingLaunchTodo = &db.Todo{
				Title:      title,
				Context:    context,
				Detail:     detail,
				WhoWaiting: whoWaiting,
				Due:        due,
				Effort:     effort,
			}
			p.subTab = "new"
		})
		p.bus.Subscribe("data.refreshed", func(e plugin.Event) {
			// Reload bookmarks when data is refreshed
			if p.db != nil {
				sessions, _ := db.DBLoadBookmarks(p.db)
				p.resumeList.SetItems(buildSessionItems(sessions))
			}
		})
	}

	return nil
}

// StartCmds returns initial tea.Cmds (e.g., spinner tick) the host should run.
func (p *Plugin) StartCmds() tea.Cmd {
	return p.spinner.Tick
}

// Shutdown cleans up plugin resources.
func (p *Plugin) Shutdown() {}

// Migrations returns any DB migrations needed by this plugin.
func (p *Plugin) Migrations() []plugin.Migration { return nil }

// Routes returns navigable sub-routes.
func (p *Plugin) Routes() []plugin.Route {
	return []plugin.Route{
		{Slug: "new", Description: "New session sub-tab"},
		{Slug: "resume", Description: "Resume session sub-tab"},
	}
}

// NavigateTo switches to the requested sub-route.
func (p *Plugin) NavigateTo(route string, args map[string]string) {
	switch route {
	case "new":
		p.subTab = "new"
	case "resume":
		p.subTab = "resume"
	}
	if todoTitle, ok := args["pending_todo_title"]; ok {
		p.pendingLaunchTodo = &db.Todo{Title: todoTitle}
	}
}

// RefreshInterval returns how often the plugin should auto-refresh.
func (p *Plugin) RefreshInterval() time.Duration { return 0 }

// Refresh returns a tea.Cmd for refreshing session data.
func (p *Plugin) Refresh() tea.Cmd {
	return p.loadSessionsCmd()
}

// KeyBindings returns the key bindings for this plugin.
func (p *Plugin) KeyBindings() []plugin.KeyBinding {
	return []plugin.KeyBinding{
		{Key: "n", Description: "New session sub-tab", Promoted: true},
		{Key: "r", Description: "Resume session sub-tab", Promoted: true},
		{Key: "enter", Description: "Launch session", Promoted: true},
		{Key: "shift+up/down", Description: "Reorder paths", Promoted: true},
		{Key: "delete", Description: "Remove saved path/session", Promoted: true},
		{Key: "/", Description: "Filter list"},
		{Key: "esc", Description: "Quit or cancel"},
	}
}

// HandleKey processes key input and returns an action for the host.
func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	// Delegate to list when filtering
	if p.subTab == "new" && p.newList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		p.newList, cmd = p.newList.Update(msg)
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	if p.subTab == "resume" && p.resumeList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		p.resumeList, cmd = p.resumeList.Update(msg)
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}

	if p.confirming {
		return p.handleConfirming(msg)
	}

	switch msg.String() {
	case "n":
		p.subTab = "new"
		return plugin.NoopAction()
	case "r":
		p.subTab = "resume"
		return plugin.NoopAction()
	case "esc":
		if p.pendingLaunchTodo != nil {
			p.pendingLaunchTodo = nil
			if p.bus != nil {
				p.bus.Publish(plugin.Event{
					Source:  "sessions",
					Topic:   "pending.todo.cancel",
					Payload: map[string]interface{}{},
				})
			}
			return plugin.Action{Type: plugin.ActionNavigate, Payload: "command"}
		}
		return plugin.Action{Type: plugin.ActionQuit}
	}

	switch p.subTab {
	case "new":
		return p.handleNewTab(msg)
	case "resume":
		return p.handleResumeTab(msg)
	}
	return plugin.NoopAction()
}

// HandleMessage processes non-key messages.
func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		p.loading = false
		p.resumeList.SetItems(buildSessionItems(msg.sessions))
		return true, plugin.NoopAction()

	case fzfFinishedMsg:
		if msg.err != nil || msg.path == "" {
			return true, plugin.NoopAction()
		}
		p.paths = db.AddPath(p.paths, msg.path)
		if p.db != nil {
			_ = db.DBAddPath(p.db, msg.path)
		}
		p.newList.SetItems(p.buildNewItems())
		return true, plugin.Action{
			Type: "launch",
			Args: map[string]string{"dir": msg.path},
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		listWidth := ui.ContentMaxWidth
		if p.width > 0 && p.width < listWidth {
			listWidth = p.width
		}
		listHeight := p.height - 14
		if listHeight < 5 {
			listHeight = 5
		}
		p.newList.SetSize(listWidth, listHeight)
		p.resumeList.SetSize(listWidth, listHeight)
		return true, plugin.NoopAction()
	}

	// Delegate to active list for unhandled messages
	switch p.subTab {
	case "new":
		var cmd tea.Cmd
		p.newList, cmd = p.newList.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
	case "resume":
		var cmd tea.Cmd
		p.resumeList, cmd = p.resumeList.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
	}
	return false, plugin.NoopAction()
}

// View renders the plugin's current view.
func (p *Plugin) View(width, height, frame int) string {
	p.frame = frame
	p.newList.SetDelegate(itemDelegate{frame: frame, styles: &p.styles, grad: &p.grad})
	p.resumeList.SetDelegate(itemDelegate{frame: frame, styles: &p.styles, grad: &p.grad})

	var content string
	switch p.subTab {
	case "new":
		content = p.viewNewTab()
	case "resume":
		content = p.viewResumeTab()
	}

	return content
}

// ---------------------------------------------------------------------------
// Internal: key handling
// ---------------------------------------------------------------------------

func (p *Plugin) handleNewTab(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		item, ok := p.newList.SelectedItem().(newItem)
		if !ok {
			return plugin.NoopAction()
		}
		if item.isBrowse {
			proc := &fzfProcess{}
			cmd := tea.Exec(proc, func(err error) tea.Msg {
				return fzfFinishedMsg{path: proc.output, err: err}
			})
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		args := map[string]string{"dir": item.path}
		if p.pendingLaunchTodo != nil {
			args["initial_prompt"] = formatTodoContext(*p.pendingLaunchTodo)
			p.pendingLaunchTodo = nil
		}
		return plugin.Action{Type: plugin.ActionLaunch, Args: args}

	case "shift+up":
		return p.movePathUp()
	case "shift+down":
		return p.movePathDown()

	case "delete", "backspace":
		item, ok := p.newList.SelectedItem().(newItem)
		if !ok || item.isBrowse {
			return plugin.NoopAction()
		}
		p.confirming = true
		p.confirmYes = false
		p.confirmItem = item
		p.confirmResume = nil
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.newList, cmd = p.newList.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleResumeTab(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		item, ok := p.resumeList.SelectedItem().(sessionItem)
		if !ok {
			return plugin.NoopAction()
		}
		return plugin.Action{
			Type: "launch",
			Args: map[string]string{
				"dir":        item.session.Project,
				"resume_id":  item.session.SessionID,
			},
		}

	case "delete", "backspace":
		item, ok := p.resumeList.SelectedItem().(sessionItem)
		if !ok {
			return plugin.NoopAction()
		}
		p.confirming = true
		p.confirmYes = false
		p.confirmItem = newItem{}
		p.confirmResume = &item
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.resumeList, cmd = p.resumeList.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleConfirming(msg tea.KeyMsg) plugin.Action {
	doDelete := func() {
		if p.confirmResume != nil {
			if p.db != nil {
				_ = db.DBRemoveBookmark(p.db, p.confirmResume.session.SessionID)
			}
			sessions, _ := db.DBLoadBookmarks(p.db)
			p.resumeList.SetItems(buildSessionItems(sessions))
		} else {
			p.paths = db.RemovePath(p.paths, p.confirmItem.path)
			if p.db != nil {
				_ = db.DBRemovePath(p.db, p.confirmItem.path)
			}
			p.newList.SetItems(p.buildNewItems())
		}
	}

	switch msg.String() {
	case "y":
		doDelete()
		p.confirming = false
		return plugin.NoopAction()
	case "enter":
		if p.confirmYes {
			doDelete()
		}
		p.confirming = false
		return plugin.NoopAction()
	case "n", "esc":
		p.confirming = false
		return plugin.NoopAction()
	case "left", "right", "tab":
		p.confirmYes = !p.confirmYes
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

// ---------------------------------------------------------------------------
// Internal: views
// ---------------------------------------------------------------------------

func (p *Plugin) viewNewTab() string {
	var banner string
	if p.pendingLaunchTodo != nil {
		banner = p.styles.sectionHeader.Render("Select project for: ") +
			lipgloss.NewStyle().Foreground(p.styles.colorWhite).Bold(true).Render(p.pendingLaunchTodo.Title) +
			p.styles.hint.Render("  (esc to cancel)")
	}
	listView := p.newList.View()
	hints := p.renderHints()
	if banner != "" {
		return lipgloss.JoinVertical(lipgloss.Left, banner, "", listView, "", hints)
	}
	return lipgloss.JoinVertical(lipgloss.Left, listView, "", hints)
}

func (p *Plugin) viewResumeTab() string {
	var listView string
	if p.loading {
		listView = "  " + p.spinner.View() + " Loading sessions..."
	} else {
		listView = p.resumeList.View()
	}
	hints := p.renderHints()
	return lipgloss.JoinVertical(lipgloss.Left, listView, "", hints)
}

func (p *Plugin) renderHints() string {
	var hints string
	if p.confirming {
		var label string
		if p.confirmResume != nil {
			label = p.confirmResume.session.Repo + " (" + p.confirmResume.session.Branch + ")"
		} else {
			label = p.confirmItem.label
		}
		yesStr := "yes"
		noStr := "no"
		if p.confirmYes {
			yesStr = p.styles.activeTab.Render("> yes")
			noStr = p.styles.inactiveTab.Render("no")
		} else {
			yesStr = p.styles.inactiveTab.Render("yes")
			noStr = p.styles.activeTab.Render("> no")
		}
		hints = p.styles.hint.Render(fmt.Sprintf("Remove %q from saved list?  ", label)) + yesStr + p.styles.hint.Render("  |  ") + noStr
	} else {
		hints = p.styles.hint.Render("n/r sub-tab   up/down navigate   shift+up/down reorder   enter launch   del remove   / filter   esc quit")
	}
	return lipgloss.PlaceHorizontal(ui.ContentMaxWidth, lipgloss.Center, hints)
}

// ---------------------------------------------------------------------------
// Internal: helpers
// ---------------------------------------------------------------------------

func (p *Plugin) buildNewItems() []list.Item {
	items := []list.Item{
		newItem{
			path:   p.cfg.ResolveHomeDir(),
			label:  p.cfg.Name,
			isHome: true,
		},
	}
	for _, path := range p.paths {
		items = append(items, newItem{
			path:  path,
			label: filepath.Base(path),
		})
	}
	items = append(items, newItem{
		label:    "Browse...",
		isBrowse: true,
	})
	return items
}

func (p *Plugin) dbWriteCmd(fn func(*sql.DB) error) tea.Cmd {
	database := p.db
	return func() tea.Msg {
		if database != nil {
			_ = fn(database)
		}
		return nil
	}
}

// movePathUp swaps the selected path with the one above it.
func (p *Plugin) movePathUp() plugin.Action {
	idx := p.newList.Index()
	// Items: [home, ...paths..., browse]
	// Path indices in p.paths are offset by 1 (home is index 0)
	pathIdx := idx - 1
	if pathIdx <= 0 {
		// Can't move the first path (or home) up further
		return plugin.NoopAction()
	}
	p.paths[pathIdx-1], p.paths[pathIdx] = p.paths[pathIdx], p.paths[pathIdx-1]
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(idx - 1)
	pathA, pathB := p.paths[pathIdx-1], p.paths[pathIdx]
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBSwapPathOrder(database, pathA, pathB)
	})}
}

// movePathDown swaps the selected path with the one below it.
func (p *Plugin) movePathDown() plugin.Action {
	idx := p.newList.Index()
	pathIdx := idx - 1
	if pathIdx < 0 || pathIdx >= len(p.paths)-1 {
		// Can't move the last path (or home/browse) down further
		return plugin.NoopAction()
	}
	p.paths[pathIdx], p.paths[pathIdx+1] = p.paths[pathIdx+1], p.paths[pathIdx]
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(idx + 1)
	pathA, pathB := p.paths[pathIdx], p.paths[pathIdx+1]
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBSwapPathOrder(database, pathA, pathB)
	})}
}

func buildSessionItems(sessions []db.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{session: s}
	}
	return items
}

func (p *Plugin) loadSessionsCmd() tea.Cmd {
	database := p.db
	return func() tea.Msg {
		sessions, _ := db.DBLoadBookmarks(database)
		return sessionsLoadedMsg{sessions: sessions}
	}
}

// SetPendingLaunchTodo sets a todo whose context will be written before launch.
func (p *Plugin) SetPendingLaunchTodo(todo *db.Todo) {
	p.pendingLaunchTodo = todo
	if todo != nil {
		p.subTab = "new"
	}
}

// formatTodoContext builds a markdown context string for a todo.
func formatTodoContext(todo db.Todo) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("## Task: %s\n", todo.Title))
	if todo.Context != "" {
		parts = append(parts, fmt.Sprintf("**Context:** %s", todo.Context))
	}
	if todo.WhoWaiting != "" {
		parts = append(parts, fmt.Sprintf("**Who's waiting:** %s", todo.WhoWaiting))
	}
	if todo.Due != "" {
		parts = append(parts, fmt.Sprintf("**Due:** %s", todo.Due))
	}
	if todo.Effort != "" {
		parts = append(parts, fmt.Sprintf("**Effort:** %s", todo.Effort))
	}
	if todo.Source != "" && todo.Source != "manual" {
		parts = append(parts, fmt.Sprintf("**Source:** %s", todo.Source))
	}
	if todo.Detail != "" {
		parts = append(parts, fmt.Sprintf("\n### Detail\n%s", todo.Detail))
	}
	return strings.Join(parts, "\n")
}
