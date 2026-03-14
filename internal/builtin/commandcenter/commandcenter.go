package commandcenter

import (
	"database/sql"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	ccStaleThreshold = 2 * time.Second
)

var bookingDurations = []int{15, 30, 60, 120, 240}

type commandTurn struct {
	role string
	text string
}

type undoEntry struct {
	todoID     string
	prevStatus string
	prevDoneAt *time.Time
	cursorPos  int
}

// ccLoadedMsg is sent when CC data is loaded from DB.
type ccLoadedMsg struct {
	cc  *db.CommandCenter
	err error
}

// dbWriteResult is sent when a DB write completes.
type dbWriteResult struct {
	err error
}

// Plugin implements the plugin.Plugin interface for the Command Center.
type Plugin struct {
	database *sql.DB
	cfg      *config.Config
	bus      plugin.EventBus
	logger   plugin.Logger
	llm      llm.LLM
	styles   ccStyles
	grad     gradientColors

	// Command center state
	cc             *db.CommandCenter
	ccLastRead     time.Time
	ccCursor       int
	ccScrollOffset int
	showBacklog    bool
	ccExpanded       bool
	ccExpandedCols   int // 0 = use default (2), 1 = single column, 2 = two columns
	ccExpandedOffset int

	// Threads state
	threadCursor int

	// Input modes
	addingThread  bool
	bookingMode   bool
	bookingCursor int
	textInput     textinput.Model

	// Detail view
	detailView    bool
	detailTodoIdx int

	// Help overlay
	showHelp bool

	// Pending launch from todo
	pendingLaunchTodo *db.Todo

	// Rich todo creation
	addingTodoRich      bool
	todoTextArea        textarea.Model
	commandConversation []commandTurn

	// Quick todo entry (t key)
	addingTodoQuick    bool
	quickTodoTextArea  textarea.Model

	// Background claude processing
	claudeLoading     bool
	claudeLoadingMsg  string
	claudeLoadingTodo string

	// Background CC refresh
	ccRefreshing           bool
	ccLastRefreshTriggered time.Time
	lastRefreshAt          time.Time
	lastRefreshError       string

	// Undo stack
	undoStack []undoEntry

	// Flash message
	flashMessage   string
	flashMessageAt time.Time

	// Sub-view: "command" or "threads"
	subView string

	// Dimensions
	width, height int
	frame         int

	// Spinner
	spinner spinner.Model
}

// New creates a new commandcenter Plugin.
func New() *Plugin {
	return &Plugin{
		subView: "command",
	}
}

// StartCmds returns initial tea.Cmds (e.g., spinner tick) the host should run.
func (p *Plugin) StartCmds() tea.Cmd {
	if p.ccRefreshing {
		return tea.Batch(p.spinner.Tick, refreshCCCmd())
	}
	return p.spinner.Tick
}

// Slug returns the plugin slug.
func (p *Plugin) Slug() string { return "commandcenter" }

// TabName returns the primary tab name.
func (p *Plugin) TabName() string { return "Command Center" }

// Migrations returns DB migrations for the command center plugin.
func (p *Plugin) Migrations() []plugin.Migration {
	return []plugin.Migration{
		{
			Version: 1,
			SQL: `CREATE INDEX IF NOT EXISTS idx_cc_todos_status_sort ON cc_todos(status, sort_order);
CREATE INDEX IF NOT EXISTS idx_cc_threads_status_created ON cc_threads(status, created_at);`,
		},
	}
}

// Routes returns the sub-routes for this plugin.
func (p *Plugin) Routes() []plugin.Route {
	routes := []plugin.Route{
		{Slug: "commandcenter", Description: "Command Center (calendar + todos)"},
	}
	if p.cfg != nil && p.cfg.Threads.Enabled {
		routes = append(routes, plugin.Route{Slug: "commandcenter/threads", Description: "Threads view"})
	}
	return routes
}

// NavigateTo switches to the given sub-route.
func (p *Plugin) NavigateTo(route string, args map[string]string) {
	switch route {
	case "commandcenter/threads":
		p.subView = "threads"
	default:
		p.subView = "command"
	}
}

// RefreshInterval returns the configured interval between background refreshes.
func (p *Plugin) RefreshInterval() time.Duration {
	return ccRefreshInterval
}

// ccReloadInterval is separate from refresh — it's how often the plugin re-reads from DB.


// Refresh returns a command that triggers a CC refresh.
func (p *Plugin) Refresh() tea.Cmd {
	if !p.ccRefreshing {
		p.ccRefreshing = true
		p.ccLastRefreshTriggered = time.Now()
		return refreshCCCmd()
	}
	return nil
}

// KeyBindings returns the key bindings for the current sub-view.
func (p *Plugin) KeyBindings() []plugin.KeyBinding {
	if p.subView == "threads" {
		return []plugin.KeyBinding{
			{Key: "up/k", Description: "Navigate threads", Promoted: true},
			{Key: "down/j", Description: "Navigate threads", Promoted: true},
			{Key: "enter", Description: "Launch Claude session", Promoted: true},
			{Key: "a", Description: "Add new thread", Promoted: true},
			{Key: "p", Description: "Pause active thread", Promoted: true},
			{Key: "s", Description: "Start paused thread", Promoted: true},
			{Key: "x", Description: "Close thread", Promoted: true},
		}
	}
	return []plugin.KeyBinding{
		{Key: "up/k", Description: "Navigate todos", Promoted: true},
		{Key: "down/j", Description: "Navigate todos", Promoted: true},
		{Key: "shift+up/down", Description: "Move todo up/down", Promoted: true},
		{Key: "enter", Description: "View todo detail", Promoted: true},
		{Key: "space", Description: "Cycle expanded view", Promoted: true},
		{Key: "o", Description: "Launch Claude session", Promoted: true},
		{Key: "x", Description: "Mark todo done", Promoted: true},
		{Key: "u", Description: "Undo last action", Promoted: true},
		{Key: "c", Description: "Command — tell Claude what to do", Promoted: true},
		{Key: "t", Description: "Quick add todos", Promoted: true},
		{Key: "X", Description: "Dismiss todo"},
		{Key: "d", Description: "Defer todo"},
		{Key: "p", Description: "Promote todo to top"},
		{Key: "s", Description: "Schedule time block"},
		{Key: "b", Description: "Toggle completed backlog"},
		{Key: "r", Description: "Refresh from all sources"},
	}
}

// Init initializes the plugin with the given context.
func (p *Plugin) Init(ctx plugin.Context) error {
	p.database = ctx.DB
	p.cfg = ctx.Config
	p.bus = ctx.Bus
	p.logger = ctx.Logger
	if ctx.LLM != nil {
		p.llm = ctx.LLM
	} else {
		p.llm = llm.NoopLLM{}
	}

	// Set refresh interval from config
	ccRefreshInterval = ctx.Config.ParseRefreshInterval()

	if ctx.Styles != nil {
		p.styles = *ctx.Styles
	} else {
		pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
		p.styles = ui.NewStyles(pal)
	}
	if ctx.Grad != nil {
		p.grad = *ctx.Grad
	} else {
		pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
		p.grad = ui.NewGradientColors(pal)
	}

	// Set up text input
	ti := textinput.New()
	ti.Placeholder = "Enter title..."
	ti.CharLimit = 120
	p.textInput = ti

	// Set up textarea
	ta := textarea.New()
	ta.Placeholder = "Tell " + p.cfg.Name + " what to do -- add todos, resolve conflicts, ask questions (ctrl+d submit, esc cancel)"
	ta.CharLimit = 2000
	ta.SetWidth(80)
	ta.SetHeight(5)
	ta.FocusedStyle.Base = ta.FocusedStyle.Base.Foreground(p.styles.ColorWhite)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(p.styles.ColorMuted)
	p.todoTextArea = ta

	// Set up quick todo textarea
	qta := textarea.New()
	qta.Placeholder = "One todo per line (ctrl+d submit, esc cancel)"
	qta.CharLimit = 2000
	qta.SetWidth(80)
	qta.SetHeight(5)
	qta.FocusedStyle.Base = qta.FocusedStyle.Base.Foreground(p.styles.ColorWhite)
	qta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
	qta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
	qta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(p.styles.ColorMuted)
	p.quickTodoTextArea = qta

	// Set up spinner
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(p.styles.ColorCyan)
	p.spinner = s

	// Subscribe to events
	if p.bus != nil {
		p.bus.Subscribe("pending.todo.cancel", func(e plugin.Event) {
			p.pendingLaunchTodo = nil
		})
		p.bus.Subscribe("config.saved", func(e plugin.Event) {
			// Re-read config on save (palette, refresh interval, etc.)
			if p.logger != nil {
				p.logger.Info("commandcenter", "config.saved event received")
			}
		})
	}

	// Load CC from DB
	if p.database != nil {
		cc, err := db.LoadCommandCenterFromDB(p.database)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("commandcenter", "failed to load CC from DB", "err", err)
			}
		}
		if cc != nil {
			p.cc = cc
			// Auto-refresh if data is stale (e.g., after machine sleep)
			if time.Since(cc.GeneratedAt) > ccRefreshInterval {
				p.ccRefreshing = true
				p.ccLastRefreshTriggered = time.Now()
			}
		}
		p.ccLastRead = time.Now()
	}

	return nil
}

// Shutdown is called when the plugin is being shut down.
func (p *Plugin) Shutdown() {}

// dbWriteCmd creates a tea.Cmd that performs a DB write.
func (p *Plugin) dbWriteCmd(fn func(*sql.DB) error) tea.Cmd {
	if p.database == nil {
		return nil
	}
	database := p.database
	return func() tea.Msg {
		return dbWriteResult{err: fn(database)}
	}
}

// loadCCFromDBCmd creates a tea.Cmd that loads CC data from the DB.
func (p *Plugin) loadCCFromDBCmd() tea.Cmd {
	database := p.database
	if database == nil {
		return nil
	}
	return func() tea.Msg {
		cc, err := db.LoadCommandCenterFromDB(database)
		return ccLoadedMsg{cc: cc, err: err}
	}
}

func ensureCC(cc **db.CommandCenter) {
	if *cc == nil {
		*cc = &db.CommandCenter{GeneratedAt: time.Now()}
	}
}

func (p *Plugin) normalMaxVisibleTodos() int {
	// Must match the maxVisibleTodos calculation in renderCommandCenterView.
	// viewCommandTab passes height = p.height - 14 to the render function.
	// renderCommandCenterView uses usedHeight ~= 4 (base, no warnings/suggestions),
	// panelHeight = height - usedHeight, maxVisibleTodos = (panelHeight - 3) / 2, min 5.
	viewHeight := p.height - 14
	if viewHeight < 10 {
		viewHeight = 10
	}
	usedHeight := 4
	panelHeight := viewHeight - usedHeight
	if panelHeight < 10 {
		panelHeight = 10
	}
	max := (panelHeight - 3) / 2
	if max < 5 {
		max = 5
	}
	return max
}

func (p *Plugin) expandedRowsPerCol() int {
	rows := (p.height - 14 - 5) / 2
	if rows < 5 {
		rows = 5
	}
	return rows
}

func (p *Plugin) expandedNumCols() int {
	if p.ccExpandedCols == 1 {
		return 1
	}
	return 2
}

func (p *Plugin) triggerFocusRefresh() tea.Cmd {
	if p.cc == nil || len(p.cc.ActiveTodos()) == 0 {
		return nil
	}
	p.claudeLoading = true
	p.claudeLoadingMsg = "Updating focus..."
	return claudeFocusCmd(p.llm, buildFocusPrompt(p.cc))
}

func (p *Plugin) threadAtCursor(active, paused []db.Thread) *db.Thread {
	if p.threadCursor < len(active) {
		return &active[p.threadCursor]
	}
	pausedIdx := p.threadCursor - len(active)
	if pausedIdx < len(paused) {
		return &paused[pausedIdx]
	}
	return nil
}

// PendingLaunchTodo returns and clears the pending launch todo, if any.
func (p *Plugin) PendingLaunchTodo() *db.Todo {
	t := p.pendingLaunchTodo
	p.pendingLaunchTodo = nil
	return t
}

// SetPendingLaunchTodo sets a pending launch todo (used when navigating back from sessions).
func (p *Plugin) SetPendingLaunchTodo(todo *db.Todo) {
	p.pendingLaunchTodo = todo
}

// View renders the plugin's current view.
func (p *Plugin) View(width, height, frame int) string {
	p.width = width
	p.height = height
	p.frame = frame

	if p.showHelp {
		return renderHelpOverlay(&p.styles, p.subView, width, height)
	}

	switch p.subView {
	case "threads":
		return p.viewThreadsTab(width, height)
	default:
		return p.viewCommandTab(width, height)
	}
}

func (p *Plugin) viewCommandTab(width, height int) string {
	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}
	viewHeight := height - 14
	if viewHeight < 10 {
		viewHeight = 10
	}

	if p.detailView && p.cc != nil {
		activeTodos := p.cc.ActiveTodos()
		if p.detailTodoIdx < len(activeTodos) {
			return renderDetailView(&p.styles, activeTodos[p.detailTodoIdx], p.textInput.View(), viewWidth)
		}
	}

	if p.ccExpanded && p.cc != nil {
		view := renderExpandedTodoView(&p.styles, &p.grad, p.cc.ActiveTodos(), p.ccCursor, p.ccExpandedOffset, p.expandedRowsPerCol(), p.expandedNumCols(), viewWidth, viewHeight, p.frame, p.claudeLoadingTodo, p.ccRefreshing)
		if p.claudeLoading {
			loadingLine := "  " + p.spinner.View() + " " + p.claudeLoadingMsg
			view = lipgloss.JoinVertical(lipgloss.Left, view, "", loadingLine)
		}
		return view
	}

	view := renderCommandCenterView(&p.styles, &p.grad, p.cc, p.cfg.Calendar.Calendars, p.cfg.Calendar.Enabled, viewWidth, viewHeight, p.ccCursor, p.ccScrollOffset, p.frame, p.claudeLoadingTodo, p.showBacklog, p.ccRefreshing, p.lastRefreshError)

	if p.claudeLoading {
		loadingLine := "  " + p.spinner.View() + " " + p.claudeLoadingMsg
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", loadingLine)
	}
	if p.flashMessage != "" {
		flash := lipgloss.NewStyle().Foreground(p.styles.ColorGreen).Render("  > " + p.flashMessage)
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", flash)
	}
	if p.addingTodoQuick {
		inputLine := p.styles.SectionHeader.Render("QUICK TODO (one per line, ctrl+d submit, esc cancel):") + "\n" + p.quickTodoTextArea.View()
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", inputLine)
	}
	if p.addingTodoRich {
		inputLine := p.styles.SectionHeader.Render("COMMAND (ctrl+d submit, esc cancel):") + "\n" + p.todoTextArea.View()
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", inputLine)
	}
	if p.bookingMode {
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", p.renderBookingPicker())
	}

	return view
}

func (p *Plugin) viewThreadsTab(width, height int) string {
	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}
	viewHeight := height - 14
	if viewHeight < 10 {
		viewHeight = 10
	}

	view := renderThreadsView(&p.styles, &p.grad, p.cc, viewWidth, viewHeight, p.threadCursor, p.frame)

	if p.addingThread {
		inputLine := p.styles.SectionHeader.Render("Add thread: ") + p.textInput.View()
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", inputLine)
	}

	return view
}

func (p *Plugin) renderBookingPicker() string {
	labels := []string{"15m", "30m", "1h", "2h", "4h"}
	var parts []string
	for i, label := range labels {
		if i == p.bookingCursor {
			parts = append(parts, p.styles.ActiveTab.Render("> "+label))
		} else {
			parts = append(parts, p.styles.InactiveTab.Render(label))
		}
	}
	picker := strings.Join(parts, "  ")
	return p.styles.SectionHeader.Render("Book time: ") + picker + p.styles.Hint.Render("  (<-> select, enter confirm, esc cancel)")
}

// publishEvent is a helper for publishing events to the bus.
func (p *Plugin) publishEvent(topic string, payload map[string]interface{}) {
	if p.bus != nil {
		p.bus.Publish(plugin.Event{
			Source:  "commandcenter",
			Topic:   topic,
			Payload: payload,
		})
	}
}

// SubView returns the current sub-view name.
func (p *Plugin) SubView() string {
	return p.subView
}

// SetSubView sets the current sub-view.
func (p *Plugin) SetSubView(v string) {
	p.subView = v
}
