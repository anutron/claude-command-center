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
	"github.com/charmbracelet/bubbles/viewport"
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

// wizardSelection stores wizard choices for a todo so they persist across open/close cycles.
type wizardSelection struct {
	pathCursor int    // selected path index (-1 = use todo's original project dir)
	mode       string // "normal", "worktree", "sandbox"
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
	detailView          bool
	detailTodoID        string // ID of the todo being viewed (stable across status changes)
	detailMode          string // "viewing", "editingField", "commandInput"
	detailSelectedField int    // 0=Status, 1=Due, 2=ProjectDir, 3=Prompt
	detailFieldInput    textinput.Model
	detailPaths         []string
	detailPathCursor    int
	detailPathFilter    string
	detailStatusCursor  int
	detailNotice        string    // flash notice shown after done/remove
	detailNoticeType    string    // "done" or "removed" — controls notice color
	detailNoticeAt      time.Time // when the notice was set

	// Task runner view (3-step wizard: 1=Project, 2=Mode, 3=Prompt)
	taskRunnerView        bool
	taskRunnerStep        int     // 1=Project, 2=Mode, 3=Prompt
	taskRunnerMode        string  // "normal", "worktree", "sandbox"
	taskRunnerPerm        string  // "default", "plan", "auto"
	taskRunnerBudget      float64
	taskRunnerPrompt      viewport.Model
	taskRunnerRefining      bool   // true when AI refine is active
	taskRunnerReviewing     bool   // true when Plannotator is open in browser
	taskRunnerInputting     bool   // true when user is typing instructions for c key
	taskRunnerInstructInput textinput.Model
	taskRunnerReviewClean   string // clean prompt text before review edits
	taskRunnerPathCursor   int    // index into detailPaths for task runner project override
	taskRunnerLaunchCursor int    // 0=Run Claude, 1=Queue Agent, 2=Run Agent Now
	taskRunnerPickingPath  bool   // true when scrollable path picker is open
	taskRunnerPathFilter   string // type-to-filter string for path picker

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

	// Agent sessions
	activeSessions map[string]*agentSession
	sessionQueue   []queuedSession

	// Wizard selections per-todo (persisted across open/close cycles)
	wizardSelections map[string]wizardSelection

	// Triage filter for expanded view tabs
	triageFilter string

	// Search filter
	searchActive bool
	searchInput  textinput.Model

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
		subView:          "command",
		triageFilter:     "accepted",
		wizardSelections: make(map[string]wizardSelection),
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
		{Key: "/", Description: "Search/filter todos"},
		{Key: "y", Description: "Accept todo (triage)"},
		{Key: "tab", Description: "Cycle triage filter (expanded)"},
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

	// Initialize agent session tracking
	p.activeSessions = make(map[string]*agentSession)

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
	ti.CharLimit = 0
	p.textInput = ti

	// Set up detail field input
	dfi := textinput.New()
	dfi.Placeholder = ""
	dfi.CharLimit = 120
	p.detailFieldInput = dfi
	p.detailMode = "viewing"

	// Load paths for project dir picker
	if ctx.DB != nil {
		paths, err := db.DBLoadPaths(ctx.DB)
		if err == nil {
			p.detailPaths = paths
		}
	}

	// Set up textarea
	ta := textarea.New()
	ta.Placeholder = "Tell " + p.cfg.Name + " what to do -- add todos, resolve conflicts, ask questions (ctrl+d submit, esc cancel)"
	ta.CharLimit = 0
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
	qta.CharLimit = 0
	qta.SetWidth(80)
	qta.SetHeight(5)
	qta.FocusedStyle.Base = qta.FocusedStyle.Base.Foreground(p.styles.ColorWhite)
	qta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
	qta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
	qta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(p.styles.ColorMuted)
	p.quickTodoTextArea = qta

	// Set up search input
	si := textinput.New()
	si.Placeholder = "Search todos..."
	si.CharLimit = 80
	p.searchInput = si

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
// It cancels all active agent sessions to prevent zombie processes.
func (p *Plugin) Shutdown() {
	for _, sess := range p.activeSessions {
		if sess.Cancel != nil {
			sess.Cancel()
		}
	}
}

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

// textareaWidth returns the appropriate width for textareas based on the current terminal width.
func (p *Plugin) textareaWidth() int {
	viewWidth := ui.ContentMaxWidth
	if p.width > 0 && p.width < viewWidth {
		viewWidth = p.width
	}
	w := viewWidth - 4
	if w < 40 {
		w = 40
	}
	return w
}

func (p *Plugin) expandedRowsPerCol() int {
	// p.height is the content area (terminal height minus TUI header).
	// The expanded view needs space for its own chrome:
	// header(1) + tabBar(1) + blank(1) + columns + blank(1) + hints(1) + footer(1) = 6 lines,
	// plus 2 lines buffer for optional loading/flash messages.
	// Each todo item takes 2 lines (title + details).
	rows := (p.height - 8) / 2
	if rows < 3 {
		rows = 3
	}
	return rows
}

func (p *Plugin) expandedNumCols() int {
	if p.ccExpandedCols == 1 {
		return 1
	}
	return 2
}

// filteredTodos returns the subset of todos based on the current view mode, triage filter, and search query.
func (p *Plugin) filteredTodos() []db.Todo {
	if p.cc == nil {
		return nil
	}
	allActive := p.cc.ActiveTodos()

	var result []db.Todo
	if !p.ccExpanded {
		// Normal view: accepted todos with no session_status
		for _, t := range allActive {
			if t.TriageStatus == "accepted" && t.SessionStatus == "" {
				result = append(result, t)
			}
		}
	} else {
		// Expanded view: filter based on triageFilter
		switch p.triageFilter {
		case "accepted":
			for _, t := range allActive {
				if t.TriageStatus == "accepted" && t.SessionStatus == "" {
					result = append(result, t)
				}
			}
		case "new":
			for _, t := range allActive {
				if t.TriageStatus == "new" {
					result = append(result, t)
				}
			}
		case "review":
			for _, t := range allActive {
				if t.SessionStatus == "review" {
					result = append(result, t)
				}
			}
		case "blocked":
			for _, t := range allActive {
				if t.SessionStatus == "blocked" {
					result = append(result, t)
				}
			}
		case "active":
			for _, t := range allActive {
				if t.SessionStatus == "active" {
					result = append(result, t)
				}
			}
		default:
			result = allActive
		}
	}

	// Apply search filter on top of triage filter
	query := strings.TrimSpace(p.searchInput.Value())
	if query == "" {
		return result
	}
	lower := strings.ToLower(query)
	var filtered []db.Todo
	for _, t := range result {
		if strings.Contains(strings.ToLower(flattenTitle(t.Title)), lower) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// triageCounts returns the count of todos matching each filter category.
func (p *Plugin) triageCounts() map[string]int {
	counts := map[string]int{
		"accepted": 0,
		"new":      0,
		"review":   0,
		"blocked":  0,
		"active":   0,
		"all":      0,
	}
	if p.cc == nil {
		return counts
	}
	for _, t := range p.cc.ActiveTodos() {
		counts["all"]++
		if t.TriageStatus == "accepted" && t.SessionStatus == "" {
			counts["accepted"]++
		}
		if t.TriageStatus == "new" {
			counts["new"]++
		}
		if t.SessionStatus == "review" {
			counts["review"]++
		}
		if t.SessionStatus == "blocked" {
			counts["blocked"]++
		}
		if t.SessionStatus == "active" {
			counts["active"]++
		}
	}
	return counts
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

	if p.taskRunnerView && p.detailView && p.cc != nil {
		if todo := p.detailTodo(); todo != nil {
			// Determine the effective project dir for display
		taskRunnerProjectDir := todo.ProjectDir
		if p.taskRunnerPathCursor >= 0 && p.taskRunnerPathCursor < len(p.detailPaths) {
			taskRunnerProjectDir = p.detailPaths[p.taskRunnerPathCursor]
		}
		// Resize viewport to match current available space on every render.
		// Step 3 has ~13 lines of chrome (header, labels, divider, selector, etc.).
		vpWidth := viewWidth - 10
		if vpWidth < 40 {
			vpWidth = 40
		}
		vpHeight := viewHeight - 13
		if vpHeight < 5 {
			vpHeight = 5
		}
		p.taskRunnerPrompt.Width = vpWidth
		p.taskRunnerPrompt.Height = vpHeight
		return renderTaskRunner(&p.styles, *todo, p.taskRunnerMode, p.taskRunnerBudget, p.taskRunnerStep, p.taskRunnerPrompt, viewWidth, viewHeight, taskRunnerProjectDir, p.taskRunnerLaunchCursor, p.taskRunnerPickingPath, p.taskRunnerFilteredPaths(), p.taskRunnerPathCursor, p.taskRunnerPathFilter, p.taskRunnerRefining, p.taskRunnerReviewing, p.taskRunnerInputting, p.taskRunnerInstructInput)
		}
	}

	if p.detailView && p.cc != nil {
		if todo := p.detailTodo(); todo != nil {
			return renderDetailView(&p.styles, *todo, p.detailMode, p.detailSelectedField, p.detailFieldInput.View(), p.textInput.View(), viewWidth, p.detailNotice, p.detailNoticeType, p.detailStatusCursor, p.filteredPaths(), p.detailPathCursor, p.detailPathFilter)
		}
		// Notice showing but no more active todos — render just the notice
		if p.detailNotice != "" {
			bgColor := p.styles.ColorGreen
			icon := "\u2713"
			if p.detailNoticeType == "removed" {
				bgColor = p.styles.ColorYellow
				icon = "\u2717"
			}
			notice := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#000000")).
				Background(bgColor).
				Bold(true).
				Padding(0, 1).
				Render(icon + " " + p.detailNotice)
			empty := p.styles.DescMuted.Render("No more active todos")
			content := lipgloss.JoinVertical(lipgloss.Left, "", "  "+notice, "", "  "+empty)
			return p.styles.PanelBorder.Width(viewWidth - 4).Render(content)
		}
	}

	if p.ccExpanded && p.cc != nil {
		filtered := p.filteredTodos()
		counts := p.triageCounts()
		view := renderExpandedTodoView(&p.styles, &p.grad, filtered, p.ccCursor, p.ccExpandedOffset, p.expandedRowsPerCol(), p.expandedNumCols(), viewWidth, viewHeight, p.frame, p.claudeLoadingTodo, p.ccRefreshing, p.triageFilter, counts)
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
		if p.searchActive {
			searchLine := p.styles.SectionHeader.Render("/") + " " + p.searchInput.View() + "  " + p.styles.Hint.Render("enter keep filter \u00b7 esc clear")
			view = lipgloss.JoinVertical(lipgloss.Left, view, "", searchLine)
		} else if strings.TrimSpace(p.searchInput.Value()) != "" {
			filterLabel := lipgloss.NewStyle().Foreground(p.styles.ColorCyan).Bold(true).Render("filter: " + p.searchInput.Value())
			filterHint := p.styles.Hint.Render("  / to edit \u00b7 esc to clear")
			view = lipgloss.JoinVertical(lipgloss.Left, view, "", "  "+filterLabel+filterHint)
		}
		return view
	}

	view := renderCommandCenterView(&p.styles, &p.grad, p.cc, p.cfg.Calendar.Calendars, p.cfg.Calendar.Enabled, viewWidth, viewHeight, p.ccCursor, p.ccScrollOffset, p.frame, p.claudeLoadingTodo, p.showBacklog, p.ccRefreshing, p.lastRefreshError, p.filteredTodos(), p.triageCounts())

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
	if p.searchActive {
		searchLine := p.styles.SectionHeader.Render("/") + " " + p.searchInput.View() + "  " + p.styles.Hint.Render("enter keep filter \u00b7 esc clear")
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", searchLine)
	} else if strings.TrimSpace(p.searchInput.Value()) != "" {
		filterLabel := lipgloss.NewStyle().Foreground(p.styles.ColorCyan).Bold(true).Render("filter: " + p.searchInput.Value())
		filterHint := p.styles.Hint.Render("  / to edit \u00b7 esc to clear")
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", "  "+filterLabel+filterHint)
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

// detailTodo returns the todo currently shown in the detail view, looked up by ID.
// Returns nil if the todo is not found (e.g. deleted).
func (p *Plugin) detailTodo() *db.Todo {
	if p.cc == nil || p.detailTodoID == "" {
		return nil
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == p.detailTodoID {
			return &p.cc.Todos[i]
		}
	}
	return nil
}

// detailTodoActiveIndex returns the index of the detail todo within ActiveTodos(), or -1.
func (p *Plugin) detailTodoActiveIndex() int {
	if p.detailTodoID == "" {
		return -1
	}
	for i, t := range p.cc.ActiveTodos() {
		if t.ID == p.detailTodoID {
			return i
		}
	}
	return -1
}

// SubView returns the current sub-view name.
func (p *Plugin) SubView() string {
	return p.subView
}

// SetSubView sets the current sub-view.
func (p *Plugin) SetSubView(v string) {
	p.subView = v
}
