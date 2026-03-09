package commandcenter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	contentMaxWidth  = 120
	ccReloadInterval = 60 * time.Second
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

// tickMsg drives all frame-based animation.
type tickMsg time.Time

// Plugin implements the plugin.Plugin interface for the Command Center.
type Plugin struct {
	database *sql.DB
	cfg      *config.Config
	bus      plugin.EventBus
	logger   plugin.Logger
	styles   ccStyles
	grad     gradientColors

	// Command center state
	cc             *db.CommandCenter
	ccLastRead     time.Time
	ccCursor       int
	ccScrollOffset int
	showBacklog    bool
	ccExpanded     bool
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

	// Background claude processing
	claudeLoading     bool
	claudeLoadingMsg  string
	claudeLoadingTodo string

	// Background CC refresh
	ccRefreshing           bool
	ccLastRefreshTriggered time.Time

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
	return p.spinner.Tick
}

// Slug returns the plugin slug.
func (p *Plugin) Slug() string { return "commandcenter" }

// TabName returns the primary tab name.
func (p *Plugin) TabName() string { return "Command Center" }

// Migrations returns DB migrations (none needed, tables already exist).
func (p *Plugin) Migrations() []plugin.Migration { return nil }

// Routes returns the sub-routes for this plugin.
func (p *Plugin) Routes() []plugin.Route {
	return []plugin.Route{
		{Slug: "commandcenter", Description: "Command Center (calendar + todos)"},
		{Slug: "commandcenter/threads", Description: "Threads view"},
	}
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

// RefreshInterval returns the interval between background refreshes.
func (p *Plugin) RefreshInterval() time.Duration {
	return ccRefreshInterval
}

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
		{Key: "space", Description: "View todo detail", Promoted: true},
		{Key: "enter", Description: "Launch Claude session", Promoted: true},
		{Key: "x", Description: "Mark todo done", Promoted: true},
		{Key: "u", Description: "Undo last action", Promoted: true},
		{Key: "c", Description: "Create new todo", Promoted: true},
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

	pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
	p.styles = newCCStyles(pal)
	p.grad = newGradientColors(pal)

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

	// Set up spinner
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(p.styles.ColorCyan)
	p.spinner = s

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
	viewHeight := p.height - 14
	if viewHeight < 10 {
		viewHeight = 10
	}
	usedHeight := 16
	panelHeight := viewHeight - usedHeight
	if panelHeight < 8 {
		panelHeight = 8
	}
	max := (panelHeight - 3) / 2
	if max < 3 {
		max = 3
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

func (p *Plugin) expandedNumCols() int { return 2 }

func (p *Plugin) triggerFocusRefresh() tea.Cmd {
	if p.cc == nil || len(p.cc.ActiveTodos()) == 0 {
		return nil
	}
	p.claudeLoading = true
	p.claudeLoadingMsg = "Updating focus..."
	return claudeFocusCmd(buildFocusPrompt(p.cc))
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

// HandleKey handles key input and returns an action.
func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	// Help overlay
	if p.showHelp {
		p.showHelp = false
		return plugin.NoopAction()
	}

	// Detail view
	if p.detailView {
		return p.handleDetailView(msg)
	}

	// Rich todo creation
	if p.addingTodoRich {
		return p.handleAddingTodoRich(msg)
	}

	// Adding thread text input
	if p.addingThread {
		return p.handleTextInput(msg)
	}

	// Booking mode
	if p.bookingMode {
		return p.handleBooking(msg)
	}

	// Help toggle
	if msg.String() == "?" {
		p.showHelp = !p.showHelp
		return plugin.NoopAction()
	}

	// Esc handling
	if msg.String() == "esc" {
		if p.ccExpanded {
			p.ccExpanded = false
			p.ccExpandedOffset = 0
			p.ccScrollOffset = 0
			p.ccCursor = 0
			return plugin.NoopAction()
		}
		if p.pendingLaunchTodo != nil {
			p.pendingLaunchTodo = nil
			p.subView = "command"
			return plugin.NoopAction()
		}
		// Let host handle esc for quit
		return plugin.Action{Type: "unhandled"}
	}

	// Dispatch to sub-view
	switch p.subView {
	case "threads":
		return p.handleThreadsTab(msg)
	default:
		return p.handleCommandTab(msg)
	}
}

func (p *Plugin) handleCommandTab(msg tea.KeyMsg) plugin.Action {
	if p.cc == nil && msg.String() != "a" {
		return plugin.NoopAction()
	}
	var activeTodos []db.Todo
	if p.cc != nil {
		activeTodos = p.cc.ActiveTodos()
	}
	maxCursor := len(activeTodos) - 1
	if maxCursor < 0 {
		maxCursor = 0
	}

	todoViewHeight := p.normalMaxVisibleTodos()

	switch msg.String() {
	case "up", "k":
		if p.ccExpanded {
			if p.ccCursor > 0 {
				p.ccCursor--
				if p.ccCursor < p.ccExpandedOffset {
					rowsPerCol := p.expandedRowsPerCol()
					numCols := p.expandedNumCols()
					pageSize := rowsPerCol * numCols
					p.ccExpandedOffset -= pageSize
					if p.ccExpandedOffset < 0 {
						p.ccExpandedOffset = 0
					}
				}
			} else {
				p.ccExpanded = false
				p.ccExpandedOffset = 0
				p.ccScrollOffset = 0
				if p.ccCursor > todoViewHeight-1 {
					p.ccCursor = todoViewHeight - 1
				}
			}
		} else {
			if p.ccCursor > 0 {
				p.ccCursor--
				if p.ccCursor < p.ccScrollOffset {
					p.ccScrollOffset = p.ccCursor
				}
			}
		}
		return plugin.NoopAction()

	case "down", "j":
		if p.ccExpanded {
			if p.ccCursor < maxCursor {
				p.ccCursor++
				rowsPerCol := p.expandedRowsPerCol()
				numCols := p.expandedNumCols()
				pageSize := rowsPerCol * numCols
				if p.ccCursor >= p.ccExpandedOffset+pageSize {
					p.ccExpandedOffset += pageSize
				}
			}
		} else {
			if p.ccCursor < maxCursor {
				p.ccCursor++
				if p.ccCursor >= p.ccScrollOffset+todoViewHeight {
					p.ccExpanded = true
					p.ccExpandedOffset = 0
				}
			}
		}
		return plugin.NoopAction()

	case "left", "h":
		if p.ccExpanded {
			rowsPerCol := p.expandedRowsPerCol()
			relIdx := p.ccCursor - p.ccExpandedOffset
			col := relIdx / rowsPerCol
			row := relIdx % rowsPerCol
			if col > 0 {
				p.ccCursor = p.ccExpandedOffset + (col-1)*rowsPerCol + row
				if p.ccCursor > maxCursor {
					p.ccCursor = maxCursor
				}
			}
		}
		return plugin.NoopAction()

	case "right", "l":
		if p.ccExpanded {
			rowsPerCol := p.expandedRowsPerCol()
			numCols := p.expandedNumCols()
			relIdx := p.ccCursor - p.ccExpandedOffset
			col := relIdx / rowsPerCol
			row := relIdx % rowsPerCol
			if col < numCols-1 {
				target := p.ccExpandedOffset + (col+1)*rowsPerCol + row
				if target > maxCursor {
					target = maxCursor
				}
				p.ccCursor = target
			}
		}
		return plugin.NoopAction()

	case "x":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todo := activeTodos[p.ccCursor]
			p.undoStack = append(p.undoStack, undoEntry{
				todoID:     todo.ID,
				prevStatus: todo.Status,
				prevDoneAt: todo.CompletedAt,
				cursorPos:  p.ccCursor,
			})
			todoID := todo.ID
			p.cc.CompleteTodo(todoID)
			newLen := len(p.cc.ActiveTodos())
			if p.ccCursor >= newLen && newLen > 0 {
				p.ccCursor = newLen - 1
			}
			if p.ccScrollOffset > p.ccCursor {
				p.ccScrollOffset = p.ccCursor
			}
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBCompleteTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: "noop", TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case "X":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todo := activeTodos[p.ccCursor]
			p.undoStack = append(p.undoStack, undoEntry{
				todoID:     todo.ID,
				prevStatus: todo.Status,
				prevDoneAt: todo.CompletedAt,
				cursorPos:  p.ccCursor,
			})
			todoID := todo.ID
			p.cc.RemoveTodo(todoID)
			newLen := len(p.cc.ActiveTodos())
			if p.ccCursor >= newLen && newLen > 0 {
				p.ccCursor = newLen - 1
			}
			if p.ccScrollOffset > p.ccCursor {
				p.ccScrollOffset = p.ccCursor
			}
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBDismissTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: "noop", TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case "u":
		if len(p.undoStack) > 0 {
			entry := p.undoStack[len(p.undoStack)-1]
			p.undoStack = p.undoStack[:len(p.undoStack)-1]
			p.cc.RestoreTodo(entry.todoID, entry.prevStatus, entry.prevDoneAt)
			p.ccCursor = entry.cursorPos
			if p.ccCursor >= len(p.cc.ActiveTodos()) && len(p.cc.ActiveTodos()) > 0 {
				p.ccCursor = len(p.cc.ActiveTodos()) - 1
			}
			p.flashMessage = "Undid last action"
			p.flashMessageAt = time.Now()
			prevStatus := entry.prevStatus
			prevDoneAt := entry.prevDoneAt
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
				return db.DBRestoreTodo(database, entry.todoID, prevStatus, prevDoneAt)
			})
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: "noop", TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case "d":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todoID := activeTodos[p.ccCursor].ID
			p.cc.DeferTodo(todoID)
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBDeferTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: "noop", TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case "p":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todoID := activeTodos[p.ccCursor].ID
			p.cc.PromoteTodo(todoID)
			p.ccCursor = 0
			p.ccScrollOffset = 0
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBPromoteTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: "noop", TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case " ":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			p.detailView = true
			p.detailTodoIdx = p.ccCursor
			p.textInput.Reset()
			p.textInput.Placeholder = "Tell me what changed..."
			p.textInput.Focus()
			return plugin.Action{Type: "noop", TeaCmd: textinput.Blink}
		}
		return plugin.NoopAction()

	case "c":
		ensureCC(&p.cc)
		p.addingTodoRich = true
		p.flashMessage = ""
		p.commandConversation = nil
		p.todoTextArea.Reset()
		cmd := p.todoTextArea.Focus()
		return plugin.Action{Type: "noop", TeaCmd: cmd}

	case "b":
		p.showBacklog = !p.showBacklog
		return plugin.NoopAction()

	case "s":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			p.bookingMode = true
			p.bookingCursor = 2
		}
		return plugin.NoopAction()

	case "r":
		if !p.ccRefreshing {
			p.ccRefreshing = true
			p.ccLastRefreshTriggered = time.Now()
			return plugin.Action{Type: "noop", TeaCmd: refreshCCCmd()}
		}
		return plugin.NoopAction()

	case "enter":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todo := activeTodos[p.ccCursor]
			if todo.SessionID != "" {
				dir := todo.ProjectDir
				if dir == "" {
					home, _ := os.UserHomeDir()
					dir = home
				}
				return plugin.Action{
					Type: "launch",
					Args: map[string]string{
						"dir":       dir,
						"resume_id": todo.SessionID,
					},
				}
			}
			if todo.ProjectDir != "" {
				return plugin.Action{
					Type: "launch",
					Args: map[string]string{
						"dir":            todo.ProjectDir,
						"initial_prompt": formatTodoContext(todo),
					},
				}
			}
			p.pendingLaunchTodo = &todo
			return plugin.Action{
				Type:    "navigate",
				Payload: "sessions",
			}
		}
		return plugin.NoopAction()
	}

	return plugin.NoopAction()
}

func (p *Plugin) handleDetailView(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		instruction := strings.TrimSpace(p.textInput.Value())
		if instruction == "" {
			return plugin.NoopAction()
		}
		activeTodos := p.cc.ActiveTodos()
		if p.detailTodoIdx >= len(activeTodos) {
			p.detailView = false
			p.textInput.Blur()
			return plugin.NoopAction()
		}
		todo := activeTodos[p.detailTodoIdx]
		prompt := buildEditPrompt(todo, instruction)
		p.detailView = false
		p.textInput.Blur()
		p.textInput.Reset()
		p.claudeLoading = true
		p.claudeLoadingMsg = "Updating todo..."
		p.claudeLoadingTodo = todo.ID
		return plugin.Action{Type: "noop", TeaCmd: claudeEditCmd(prompt, todo.ID)}

	case "esc":
		p.detailView = false
		p.textInput.Blur()
		p.textInput.Reset()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.textInput, cmd = p.textInput.Update(msg)
	return plugin.Action{Type: "noop", TeaCmd: cmd}
}

func (p *Plugin) handleAddingTodoRich(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "ctrl+d":
		text := strings.TrimSpace(p.todoTextArea.Value())
		if text == "" {
			p.addingTodoRich = false
			p.todoTextArea.Blur()
			p.commandConversation = nil
			return plugin.NoopAction()
		}
		p.commandConversation = append(p.commandConversation, commandTurn{role: "user", text: text})
		prompt := buildCommandPromptWithHistory(p.cc, p.cfg.Name, p.commandConversation)
		p.addingTodoRich = false
		p.todoTextArea.Blur()
		p.claudeLoading = true
		p.claudeLoadingMsg = "Processing..."
		return plugin.Action{Type: "noop", TeaCmd: claudeCommandCmd(prompt, "")}

	case "esc":
		p.addingTodoRich = false
		p.todoTextArea.Blur()
		p.commandConversation = nil
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.todoTextArea, cmd = p.todoTextArea.Update(msg)
	return plugin.Action{Type: "noop", TeaCmd: cmd}
}

func (p *Plugin) handleTextInput(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(p.textInput.Value())
		var cmd tea.Cmd
		if title != "" && p.addingThread {
			ensureCC(&p.cc)
			thread := p.cc.AddThread(title, "manual")
			threadCopy := *thread
			cmd = p.dbWriteCmd(func(database *sql.DB) error { return db.DBInsertThread(database, threadCopy) })
		}
		p.addingThread = false
		p.textInput.Blur()
		return plugin.Action{Type: "noop", TeaCmd: cmd}

	case "esc":
		p.addingThread = false
		p.textInput.Blur()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.textInput, cmd = p.textInput.Update(msg)
	return plugin.Action{Type: "noop", TeaCmd: cmd}
}

func (p *Plugin) handleBooking(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "left", "h":
		if p.bookingCursor > 0 {
			p.bookingCursor--
		}
		return plugin.NoopAction()

	case "right", "l":
		if p.bookingCursor < len(bookingDurations)-1 {
			p.bookingCursor++
		}
		return plugin.NoopAction()

	case "enter":
		activeTodos := p.cc.ActiveTodos()
		if p.ccCursor < len(activeTodos) {
			todoID := activeTodos[p.ccCursor].ID
			dur := bookingDurations[p.bookingCursor]
			p.cc.AddPendingBooking(todoID, dur)
			action := db.PendingAction{
				Type:            "booking",
				TodoID:          todoID,
				DurationMinutes: dur,
				RequestedAt:     time.Now(),
			}
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBInsertPendingAction(database, action) })
			p.ccRefreshing = true
			p.flashMessage = fmt.Sprintf("Booking %dm for %s...", dur, activeTodos[p.ccCursor].Title)
			p.flashMessageAt = time.Now()
			p.bookingMode = false
			return plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmd, refreshCCCmd())}
		}
		p.bookingMode = false
		return plugin.NoopAction()

	case "esc":
		p.bookingMode = false
		return plugin.NoopAction()
	}

	return plugin.NoopAction()
}

func (p *Plugin) handleThreadsTab(msg tea.KeyMsg) plugin.Action {
	if p.cc == nil {
		return plugin.NoopAction()
	}
	active := p.cc.ActiveThreads()
	paused := p.cc.PausedThreads()
	total := len(active) + len(paused)
	maxCursor := total - 1
	if maxCursor < 0 {
		maxCursor = 0
	}

	switch msg.String() {
	case "up", "k":
		if p.threadCursor > 0 {
			p.threadCursor--
		}
		return plugin.NoopAction()

	case "down", "j":
		if p.threadCursor < maxCursor {
			p.threadCursor++
		}
		return plugin.NoopAction()

	case "p":
		thread := p.threadAtCursor(active, paused)
		if thread != nil && thread.Status == "active" {
			threadID := thread.ID
			p.cc.PauseThread(threadID)
			return plugin.Action{Type: "noop", TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBPauseThread(database, threadID) })}
		}
		return plugin.NoopAction()

	case "s":
		thread := p.threadAtCursor(active, paused)
		if thread != nil && thread.Status == "paused" {
			threadID := thread.ID
			p.cc.StartThread(threadID)
			return plugin.Action{Type: "noop", TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBStartThread(database, threadID) })}
		}
		return plugin.NoopAction()

	case "x":
		thread := p.threadAtCursor(active, paused)
		if thread != nil {
			threadID := thread.ID
			p.cc.CloseThread(threadID)
			if p.threadCursor > 0 {
				newTotal := len(p.cc.ActiveThreads()) + len(p.cc.PausedThreads())
				if p.threadCursor >= newTotal {
					p.threadCursor = newTotal - 1
				}
				if p.threadCursor < 0 {
					p.threadCursor = 0
				}
			}
			return plugin.Action{Type: "noop", TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBCloseThread(database, threadID) })}
		}
		return plugin.NoopAction()

	case "a":
		p.addingThread = true
		p.textInput.Reset()
		p.textInput.Placeholder = "New thread..."
		p.textInput.Focus()
		return plugin.Action{Type: "noop", TeaCmd: textinput.Blink}

	case "enter":
		thread := p.threadAtCursor(active, paused)
		if thread != nil && thread.ProjectDir != "" {
			return plugin.Action{
				Type: "launch",
				Args: map[string]string{"dir": thread.ProjectDir},
			}
		}
		return plugin.NoopAction()
	}

	return plugin.NoopAction()
}

// HandleMessage handles non-key messages and returns whether it was handled.
func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case ccLoadedMsg:
		if msg.cc != nil {
			p.cc = msg.cc
		}
		p.ccLastRead = time.Now()
		return true, plugin.NoopAction()

	case ccRefreshFinishedMsg:
		p.ccRefreshing = false
		if p.database != nil {
			return true, plugin.Action{Type: "noop", TeaCmd: p.loadCCFromDBCmd()}
		}
		return true, plugin.NoopAction()

	case dbWriteResult:
		if msg.err != nil {
			fmt.Fprintf(os.Stderr, "DB write error: %v\n", msg.err)
			if p.database != nil {
				return true, plugin.Action{Type: "noop", TeaCmd: p.loadCCFromDBCmd()}
			}
		}
		return true, plugin.NoopAction()

	case claudeEditFinishedMsg:
		p.claudeLoading = false
		p.claudeLoadingTodo = ""
		if msg.err == nil && msg.output != "" && p.cc != nil {
			jsonStr := extractJSON(msg.output)
			var updated db.Todo
			if err := json.Unmarshal([]byte(jsonStr), &updated); err == nil {
				todoID := msg.todoID
				for i := range p.cc.Todos {
					if p.cc.Todos[i].ID == todoID {
						updated.ID = p.cc.Todos[i].ID
						if updated.CreatedAt.IsZero() {
							updated.CreatedAt = p.cc.Todos[i].CreatedAt
						}
						p.cc.Todos[i] = updated
						break
					}
				}
				return true, plugin.Action{Type: "noop", TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBUpdateTodo(database, todoID, updated)
				})}
			}
		}
		return true, plugin.NoopAction()

	case claudeEnrichFinishedMsg:
		p.claudeLoading = false
		if msg.err == nil && msg.output != "" {
			jsonStr := extractJSON(msg.output)
			var enriched struct {
				Title      string `json:"title"`
				Due        string `json:"due"`
				WhoWaiting string `json:"who_waiting"`
				Effort     string `json:"effort"`
				Context    string `json:"context"`
				Detail     string `json:"detail"`
				ProjectDir string `json:"project_dir"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &enriched); err == nil && enriched.Title != "" {
				ensureCC(&p.cc)
				todo := p.cc.AddTodo(enriched.Title)
				todo.Due = enriched.Due
				todo.WhoWaiting = enriched.WhoWaiting
				todo.Effort = enriched.Effort
				todo.Context = enriched.Context
				todo.Detail = enriched.Detail
				todo.ProjectDir = enriched.ProjectDir
				todoCopy := *todo
				return true, plugin.Action{Type: "noop", TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBInsertTodo(database, todoCopy)
				})}
			}
		}
		return true, plugin.NoopAction()

	case claudeCommandFinishedMsg:
		p.claudeLoading = false
		if msg.err == nil && msg.output != "" {
			jsonStr := extractJSON(msg.output)
			var resp struct {
				Message         string `json:"message"`
				Ask             string `json:"ask"`
				Todos           []struct {
					Title      string `json:"title"`
					Due        string `json:"due"`
					WhoWaiting string `json:"who_waiting"`
					Effort     string `json:"effort"`
					Context    string `json:"context"`
					Detail     string `json:"detail"`
					ProjectDir string `json:"project_dir"`
				} `json:"todos"`
				CompleteTodoIDs []string `json:"complete_todo_ids"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &resp); err == nil {
				if resp.Ask != "" {
					p.commandConversation = append(p.commandConversation, commandTurn{role: "assistant", text: resp.Ask})
					p.flashMessage = resp.Ask
					p.flashMessageAt = time.Now()
					p.addingTodoRich = true
					p.todoTextArea.Reset()
					return true, plugin.Action{Type: "noop", TeaCmd: p.todoTextArea.Focus()}
				}

				if resp.Message != "" {
					p.flashMessage = resp.Message
					p.flashMessageAt = time.Now()
				}
				mutated := len(resp.Todos) > 0 || len(resp.CompleteTodoIDs) > 0
				if mutated {
					ensureCC(&p.cc)
					var dbCmds []tea.Cmd
					for _, item := range resp.Todos {
						if item.Title == "" {
							continue
						}
						todo := p.cc.AddTodo(item.Title)
						todo.Due = item.Due
						todo.WhoWaiting = item.WhoWaiting
						todo.Effort = item.Effort
						todo.Context = item.Context
						todo.Detail = item.Detail
						todo.ProjectDir = item.ProjectDir
						t := *todo
						dbCmds = append(dbCmds, p.dbWriteCmd(func(database *sql.DB) error { return db.DBInsertTodo(database, t) }))
					}
					for _, id := range resp.CompleteTodoIDs {
						p.cc.CompleteTodo(id)
						cid := id
						dbCmds = append(dbCmds, p.dbWriteCmd(func(database *sql.DB) error { return db.DBCompleteTodo(database, cid) }))
					}
					if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
						dbCmds = append(dbCmds, focusCmd)
					}
					p.commandConversation = nil
					return true, plugin.Action{Type: "noop", TeaCmd: tea.Batch(dbCmds...)}
				}
				p.commandConversation = nil
			} else {
				p.flashMessage = strings.TrimSpace(msg.output)
				p.flashMessageAt = time.Now()
				p.commandConversation = nil
			}
		} else if msg.err != nil {
			p.flashMessage = "Command failed: " + msg.err.Error()
			p.flashMessageAt = time.Now()
			p.commandConversation = nil
		}
		return true, plugin.NoopAction()

	case claudeFocusFinishedMsg:
		p.claudeLoading = false
		if msg.err == nil && msg.output != "" {
			focus := strings.TrimSpace(msg.output)
			focus = strings.Trim(focus, "\"")
			if focus != "" && p.cc != nil {
				p.cc.Suggestions.Focus = focus
				return true, plugin.Action{Type: "noop", TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBSaveFocus(database, focus) })}
			}
		}
		return true, plugin.NoopAction()

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return false, plugin.NoopAction() // Let host also handle this

	case tickMsg:
		p.frame++
		if p.flashMessage != "" && time.Since(p.flashMessageAt) > 15*time.Second {
			p.flashMessage = ""
		}
		var cmds []tea.Cmd
		if p.cc != nil && time.Since(p.ccLastRead) > ccReloadInterval {
			if p.database != nil {
				cmds = append(cmds, p.loadCCFromDBCmd())
			}
		}
		if p.cc != nil && !p.ccRefreshing && time.Since(p.cc.GeneratedAt) > ccRefreshInterval {
			if time.Since(p.ccLastRefreshTriggered) > ccRefreshInterval {
				p.ccRefreshing = true
				p.ccLastRefreshTriggered = time.Now()
				cmds = append(cmds, refreshCCCmd())
			}
		}
		if len(cmds) > 0 {
			return true, plugin.Action{Type: "noop", TeaCmd: tea.Batch(cmds...)}
		}
		return false, plugin.NoopAction() // Tick is shared, don't claim exclusive ownership

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		if cmd != nil {
			return false, plugin.Action{Type: "noop", TeaCmd: cmd}
		}
		return false, plugin.NoopAction()
	}

	// Pass through to textarea / textinput if active
	if p.addingTodoRich {
		var cmd tea.Cmd
		p.todoTextArea, cmd = p.todoTextArea.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: "noop", TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}
	if p.detailView {
		var cmd tea.Cmd
		p.textInput, cmd = p.textInput.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: "noop", TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}

	return false, plugin.NoopAction()
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
	viewWidth := contentMaxWidth
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

	view := renderCommandCenterView(&p.styles, &p.grad, p.cc, p.cfg.Calendar.Calendars, p.cfg.Calendar.Enabled, viewWidth, viewHeight, p.ccCursor, p.ccScrollOffset, p.frame, p.claudeLoadingTodo, p.showBacklog, p.ccRefreshing)

	if p.claudeLoading {
		loadingLine := "  " + p.spinner.View() + " " + p.claudeLoadingMsg
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", loadingLine)
	}
	if p.flashMessage != "" {
		flash := lipgloss.NewStyle().Foreground(p.styles.ColorGreen).Render("  > " + p.flashMessage)
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", flash)
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
	viewWidth := contentMaxWidth
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

// SubView returns the current sub-view name.
func (p *Plugin) SubView() string {
	return p.subView
}

// SetSubView sets the current sub-view.
func (p *Plugin) SetSubView(v string) {
	p.subView = v
}
