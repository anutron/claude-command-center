package tui

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tab int

const (
	tabNew tab = iota
	tabResume
	tabCommand
	tabThreads
)

const ccReloadInterval = 60 * time.Second

var bookingDurations = []int{15, 30, 60, 120, 240}

// Model is the main Bubbletea model for the TUI.
type Model struct {
	cfg     *config.Config
	styles  Styles
	grad    GradientColors
	palette config.Palette

	activeTab     tab
	newList       list.Model
	resumeList    list.Model
	width         int
	height        int
	confirming    bool
	confirmYes    bool
	confirmItem   newItem
	confirmResume *sessionItem
	paths         []string
	Launch        *LaunchAction
	frame         int
	loading       bool
	spinner       spinner.Model

	// Database
	db *sql.DB

	// Command center state
	cc             *db.CommandCenter
	ccLastRead     time.Time
	ccCursor         int
	ccScrollOffset   int
	showBacklog      bool
	ccExpanded       bool
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

	// Rich todo creation (textarea)
	addingTodoRich      bool
	todoTextArea        textarea.Model
	commandConversation []commandTurn

	// Background claude processing
	claudeLoading      bool
	claudeLoadingMsg   string
	claudeLoadingTodo  string

	// Background CC refresh
	ccRefreshing          bool
	ccLastRefreshTriggered time.Time

	// Undo stack for todo actions
	undoStack []undoEntry

	// Flash message from command responses
	flashMessage   string
	flashMessageAt time.Time
}

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

type sessionsLoadedMsg struct {
	sessions []db.Session
}

type ccLoadedMsg struct {
	cc  *db.CommandCenter
	err error
}

func loadSessionsCmd(database *sql.DB) tea.Cmd {
	return func() tea.Msg {
		sessions, _ := db.DBLoadBookmarks(database)
		return sessionsLoadedMsg{sessions: sessions}
	}
}

func loadCCFromDBCmd(database *sql.DB) tea.Cmd {
	return func() tea.Msg {
		cc, err := db.LoadCommandCenterFromDB(database)
		return ccLoadedMsg{cc: cc, err: err}
	}
}

type dbWriteResult struct {
	err error
}

func dbWriteCmd(database *sql.DB, fn func(*sql.DB) error) tea.Cmd {
	if database == nil {
		return nil
	}
	return func() tea.Msg {
		return dbWriteResult{err: fn(database)}
	}
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

// NewModel creates the main TUI model.
func NewModel(database *sql.DB, cfg *config.Config) Model {
	pal := config.GetPalette(cfg.Palette, cfg.Colors)
	styles := NewStyles(pal)
	grad := NewGradientColors(pal)

	paths, _ := db.DBLoadPaths(database)

	newItems := buildNewItems(cfg.Name, paths)

	delegate := itemDelegate{styles: &styles, grad: &grad}
	nl := list.New(newItems, delegate, 0, 10)
	nl.SetShowTitle(false)
	nl.SetShowStatusBar(false)
	nl.SetFilteringEnabled(true)
	nl.SetShowHelp(false)

	rl := list.New([]list.Item{}, delegate, 0, 10)
	rl.SetShowTitle(false)
	rl.SetShowStatusBar(false)
	rl.SetFilteringEnabled(true)
	rl.SetShowHelp(false)

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(styles.ColorCyan)

	ti := textinput.New()
	ti.Placeholder = "Enter title..."
	ti.CharLimit = 120

	ta := textarea.New()
	ta.Placeholder = "Tell " + cfg.Name + " what to do -- add todos, resolve conflicts, ask questions (ctrl+d submit, esc cancel)"
	ta.CharLimit = 2000
	ta.SetWidth(80)
	ta.SetHeight(5)
	ta.FocusedStyle.Base = ta.FocusedStyle.Base.Foreground(styles.ColorWhite)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(styles.ColorWhite)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(styles.ColorWhite)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)

	return Model{
		cfg:      cfg,
		styles:   styles,
		grad:     grad,
		palette:  pal,
		activeTab: tabNew,
		newList:   nl,
		resumeList: rl,
		db:        database,
		paths:     paths,
		loading:   true,
		spinner:   s,
		textInput: ti,
		todoTextArea: ta,
	}
}

func buildNewItems(name string, paths []string) []list.Item {
	items := []list.Item{
		newItem{
			label:  name,
			isHome: true,
		},
	}
	for _, p := range paths {
		items = append(items, newItem{
			path:  p,
			label: filepath.Base(p),
		})
	}
	items = append(items, newItem{
		label:    "Browse...",
		isBrowse: true,
	})
	return items
}

func buildSessionItems(sessions []db.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{session: s}
	}
	return items
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, tickCmd(), m.spinner.Tick)
	if m.db != nil {
		cmds = append(cmds, loadSessionsCmd(m.db), loadCCFromDBCmd(m.db))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.frame++
		m.newList.SetDelegate(itemDelegate{frame: m.frame, styles: &m.styles, grad: &m.grad})
		m.resumeList.SetDelegate(itemDelegate{frame: m.frame, styles: &m.styles, grad: &m.grad})
		if m.flashMessage != "" && time.Since(m.flashMessageAt) > 15*time.Second {
			m.flashMessage = ""
		}
		if m.cc != nil && time.Since(m.ccLastRead) > ccReloadInterval {
			if m.db != nil {
				return m, tea.Batch(tickCmd(), loadCCFromDBCmd(m.db))
			}
		}
		if m.cc != nil && !m.ccRefreshing && time.Since(m.cc.GeneratedAt) > ccRefreshInterval {
			if time.Since(m.ccLastRefreshTriggered) > ccRefreshInterval {
				m.ccRefreshing = true
				m.ccLastRefreshTriggered = time.Now()
				return m, tea.Batch(tickCmd(), refreshCCCmd())
			}
		}
		return m, tickCmd()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case sessionsLoadedMsg:
		m.loading = false
		m.resumeList.SetItems(buildSessionItems(msg.sessions))
		return m, nil

	case ccLoadedMsg:
		if msg.cc != nil {
			m.cc = msg.cc
		}
		m.ccLastRead = time.Now()
		return m, nil

	case ccRefreshFinishedMsg:
		m.ccRefreshing = false
		if m.db != nil {
			return m, loadCCFromDBCmd(m.db)
		}
		return m, nil

	case dbWriteResult:
		if msg.err != nil {
			fmt.Fprintf(os.Stderr, "DB write error: %v\n", msg.err)
			if m.db != nil {
				return m, loadCCFromDBCmd(m.db)
			}
		}
		return m, nil

	case claudeEditFinishedMsg:
		m.claudeLoading = false
		m.claudeLoadingTodo = ""
		if msg.err == nil && msg.output != "" && m.cc != nil {
			jsonStr := extractJSON(msg.output)
			var updated db.Todo
			if err := json.Unmarshal([]byte(jsonStr), &updated); err == nil {
				todoID := msg.todoID
				for i := range m.cc.Todos {
					if m.cc.Todos[i].ID == todoID {
						updated.ID = m.cc.Todos[i].ID
						if updated.CreatedAt.IsZero() {
							updated.CreatedAt = m.cc.Todos[i].CreatedAt
						}
						m.cc.Todos[i] = updated
						break
					}
				}
				return m, dbWriteCmd(m.db, func(database *sql.DB) error {
					return db.DBUpdateTodo(database, todoID, updated)
				})
			}
		}
		return m, nil

	case claudeEnrichFinishedMsg:
		m.claudeLoading = false
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
				ensureCC(&m.cc)
				todo := m.cc.AddTodo(enriched.Title)
				todo.Due = enriched.Due
				todo.WhoWaiting = enriched.WhoWaiting
				todo.Effort = enriched.Effort
				todo.Context = enriched.Context
				todo.Detail = enriched.Detail
				todo.ProjectDir = enriched.ProjectDir
				todoCopy := *todo
				return m, dbWriteCmd(m.db, func(database *sql.DB) error {
					return db.DBInsertTodo(database, todoCopy)
				})
			}
		}
		return m, nil

	case claudeCommandFinishedMsg:
		m.claudeLoading = false
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
					m.commandConversation = append(m.commandConversation, commandTurn{role: "assistant", text: resp.Ask})
					m.flashMessage = resp.Ask
					m.flashMessageAt = time.Now()
					m.addingTodoRich = true
					m.todoTextArea.Reset()
					return m, m.todoTextArea.Focus()
				}

				if resp.Message != "" {
					m.flashMessage = resp.Message
					m.flashMessageAt = time.Now()
				}
				mutated := len(resp.Todos) > 0 || len(resp.CompleteTodoIDs) > 0
				if mutated {
					ensureCC(&m.cc)
					var dbCmds []tea.Cmd
					for _, item := range resp.Todos {
						if item.Title == "" {
							continue
						}
						todo := m.cc.AddTodo(item.Title)
						todo.Due = item.Due
						todo.WhoWaiting = item.WhoWaiting
						todo.Effort = item.Effort
						todo.Context = item.Context
						todo.Detail = item.Detail
						todo.ProjectDir = item.ProjectDir
						t := *todo
						dbCmds = append(dbCmds, dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBInsertTodo(database, t) }))
					}
					for _, id := range resp.CompleteTodoIDs {
						m.cc.CompleteTodo(id)
						cid := id
						dbCmds = append(dbCmds, dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBCompleteTodo(database, cid) }))
					}
					if focusCmd := m.triggerFocusRefresh(); focusCmd != nil {
						dbCmds = append(dbCmds, focusCmd)
					}
					m.commandConversation = nil
					return m, tea.Batch(dbCmds...)
				}
				m.commandConversation = nil
			} else {
				m.flashMessage = strings.TrimSpace(msg.output)
				m.flashMessageAt = time.Now()
				m.commandConversation = nil
			}
		} else if msg.err != nil {
			m.flashMessage = "Command failed: " + msg.err.Error()
			m.flashMessageAt = time.Now()
			m.commandConversation = nil
		}
		return m, nil

	case claudeFocusFinishedMsg:
		m.claudeLoading = false
		if msg.err == nil && msg.output != "" {
			focus := strings.TrimSpace(msg.output)
			focus = strings.Trim(focus, "\"")
			if focus != "" && m.cc != nil {
				m.cc.Suggestions.Focus = focus
				return m, dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBSaveFocus(database, focus) })
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listHeight := m.height - 14
		if listHeight < 3 {
			listHeight = 3
		}
		m.newList.SetSize(msg.Width, listHeight)
		m.resumeList.SetSize(msg.Width, listHeight)
		return m, nil

	case fzfFinishedMsg:
		if msg.err != nil || msg.path == "" {
			return m, nil
		}
		m.paths = db.AddPath(m.paths, msg.path)
		if m.db != nil {
			_ = db.DBAddPath(m.db, msg.path)
		}
		m.Launch = &LaunchAction{Dir: msg.path}
		return m, tea.Quit

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.detailView {
			return m.updateDetailView(msg)
		}
		if m.addingTodoRich {
			return m.updateAddingTodoRich(msg)
		}
		if m.addingThread {
			return m.updateTextInput(msg)
		}
		if m.bookingMode {
			return m.updateBooking(msg)
		}
		if m.activeTab == tabNew && m.newList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.newList, cmd = m.newList.Update(msg)
			return m, cmd
		}
		if m.activeTab == tabResume && m.resumeList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.resumeList, cmd = m.resumeList.Update(msg)
			return m, cmd
		}
		if m.confirming {
			return m.updateConfirming(msg)
		}
		return m.updateNormal(msg)
	}

	var cmd tea.Cmd
	if m.addingTodoRich {
		m.todoTextArea, cmd = m.todoTextArea.Update(msg)
	} else if m.detailView {
		m.textInput, cmd = m.textInput.Update(msg)
	} else if m.activeTab == tabNew {
		m.newList, cmd = m.newList.Update(msg)
	} else if m.activeTab == tabResume {
		m.resumeList, cmd = m.resumeList.Update(msg)
	}
	return m, cmd
}

func (m Model) nextTab() tab { return (m.activeTab + 1) % 4 }
func (m Model) prevTab() tab { return (m.activeTab + 3) % 4 }

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.activeTab = m.nextTab()
		return m, nil
	case "shift+tab":
		m.activeTab = m.prevTab()
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "esc":
		if m.ccExpanded {
			m.ccExpanded = false
			m.ccExpandedOffset = 0
			m.ccScrollOffset = 0
			m.ccCursor = 0
			return m, nil
		}
		if m.pendingLaunchTodo != nil {
			m.pendingLaunchTodo = nil
			m.activeTab = tabCommand
			return m, nil
		}
		return m, tea.Quit
	}

	switch m.activeTab {
	case tabNew:
		return m.updateNewTab(msg)
	case tabResume:
		return m.updateResumeTab(msg)
	case tabCommand:
		return m.updateCommandTab(msg)
	case tabThreads:
		return m.updateThreadsTab(msg)
	}
	return m, nil
}

func (m Model) updateNewTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		item, ok := m.newList.SelectedItem().(newItem)
		if !ok {
			return m, nil
		}
		if item.isBrowse {
			proc := &fzfProcess{}
			return m, tea.Exec(proc, func(err error) tea.Msg {
				return fzfFinishedMsg{path: proc.output, err: err}
			})
		}
		action := &LaunchAction{Dir: item.path}
		if m.pendingLaunchTodo != nil {
			action.InitialPrompt = formatTodoContext(*m.pendingLaunchTodo)
			m.pendingLaunchTodo = nil
		}
		m.Launch = action
		return m, tea.Quit

	case "delete", "backspace":
		item, ok := m.newList.SelectedItem().(newItem)
		if !ok || item.isHome || item.isBrowse {
			return m, nil
		}
		m.confirming = true
		m.confirmYes = false
		m.confirmItem = item
		m.confirmResume = nil
		return m, nil
	}

	var cmd tea.Cmd
	m.newList, cmd = m.newList.Update(msg)
	return m, cmd
}

func (m Model) updateResumeTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		item, ok := m.resumeList.SelectedItem().(sessionItem)
		if !ok {
			return m, nil
		}
		m.Launch = &LaunchAction{
			Dir:  item.session.Project,
			Args: []string{"-r", item.session.SessionID},
		}
		return m, tea.Quit

	case "delete", "backspace":
		item, ok := m.resumeList.SelectedItem().(sessionItem)
		if !ok {
			return m, nil
		}
		m.confirming = true
		m.confirmYes = false
		m.confirmItem = newItem{}
		m.confirmResume = &item
		return m, nil
	}

	var cmd tea.Cmd
	m.resumeList, cmd = m.resumeList.Update(msg)
	return m, cmd
}

func ensureCC(cc **db.CommandCenter) {
	if *cc == nil {
		*cc = &db.CommandCenter{GeneratedAt: time.Now()}
	}
}

func (m Model) normalMaxVisibleTodos() int {
	viewHeight := m.height - 14
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

func (m Model) expandedRowsPerCol() int {
	rows := (m.height - 14 - 5) / 2
	if rows < 5 {
		rows = 5
	}
	return rows
}

func (m Model) expandedNumCols() int { return 2 }

func (m Model) updateCommandTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cc == nil && msg.String() != "a" {
		return m, nil
	}
	var activeTodos []db.Todo
	if m.cc != nil {
		activeTodos = m.cc.ActiveTodos()
	}
	maxCursor := len(activeTodos) - 1
	if maxCursor < 0 {
		maxCursor = 0
	}

	todoViewHeight := m.normalMaxVisibleTodos()

	switch msg.String() {
	case "up", "k":
		if m.ccExpanded {
			if m.ccCursor > 0 {
				m.ccCursor--
				if m.ccCursor < m.ccExpandedOffset {
					rowsPerCol := m.expandedRowsPerCol()
					numCols := m.expandedNumCols()
					pageSize := rowsPerCol * numCols
					m.ccExpandedOffset -= pageSize
					if m.ccExpandedOffset < 0 {
						m.ccExpandedOffset = 0
					}
				}
			} else {
				m.ccExpanded = false
				m.ccExpandedOffset = 0
				m.ccScrollOffset = 0
				if m.ccCursor > todoViewHeight-1 {
					m.ccCursor = todoViewHeight - 1
				}
			}
		} else {
			if m.ccCursor > 0 {
				m.ccCursor--
				if m.ccCursor < m.ccScrollOffset {
					m.ccScrollOffset = m.ccCursor
				}
			}
		}
		return m, nil

	case "down", "j":
		if m.ccExpanded {
			if m.ccCursor < maxCursor {
				m.ccCursor++
				rowsPerCol := m.expandedRowsPerCol()
				numCols := m.expandedNumCols()
				pageSize := rowsPerCol * numCols
				if m.ccCursor >= m.ccExpandedOffset+pageSize {
					m.ccExpandedOffset += pageSize
				}
			}
		} else {
			if m.ccCursor < maxCursor {
				m.ccCursor++
				if m.ccCursor >= m.ccScrollOffset+todoViewHeight {
					m.ccExpanded = true
					m.ccExpandedOffset = 0
				}
			}
		}
		return m, nil

	case "left", "h":
		if m.ccExpanded {
			rowsPerCol := m.expandedRowsPerCol()
			relIdx := m.ccCursor - m.ccExpandedOffset
			col := relIdx / rowsPerCol
			row := relIdx % rowsPerCol
			if col > 0 {
				m.ccCursor = m.ccExpandedOffset + (col-1)*rowsPerCol + row
				if m.ccCursor > maxCursor {
					m.ccCursor = maxCursor
				}
			}
		}
		return m, nil

	case "right", "l":
		if m.ccExpanded {
			rowsPerCol := m.expandedRowsPerCol()
			numCols := m.expandedNumCols()
			relIdx := m.ccCursor - m.ccExpandedOffset
			col := relIdx / rowsPerCol
			row := relIdx % rowsPerCol
			if col < numCols-1 {
				target := m.ccExpandedOffset + (col+1)*rowsPerCol + row
				if target > maxCursor {
					target = maxCursor
				}
				m.ccCursor = target
			}
		}
		return m, nil

	case "x":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			todo := activeTodos[m.ccCursor]
			m.undoStack = append(m.undoStack, undoEntry{
				todoID:     todo.ID,
				prevStatus: todo.Status,
				prevDoneAt: todo.CompletedAt,
				cursorPos:  m.ccCursor,
			})
			todoID := todo.ID
			m.cc.CompleteTodo(todoID)
			newLen := len(m.cc.ActiveTodos())
			if m.ccCursor >= newLen && newLen > 0 {
				m.ccCursor = newLen - 1
			}
			if m.ccScrollOffset > m.ccCursor {
				m.ccScrollOffset = m.ccCursor
			}
			dbCmd := dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBCompleteTodo(database, todoID) })
			if focusCmd := m.triggerFocusRefresh(); focusCmd != nil {
				return m, tea.Batch(dbCmd, focusCmd)
			}
			return m, dbCmd
		}
		return m, nil

	case "X":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			todo := activeTodos[m.ccCursor]
			m.undoStack = append(m.undoStack, undoEntry{
				todoID:     todo.ID,
				prevStatus: todo.Status,
				prevDoneAt: todo.CompletedAt,
				cursorPos:  m.ccCursor,
			})
			todoID := todo.ID
			m.cc.RemoveTodo(todoID)
			newLen := len(m.cc.ActiveTodos())
			if m.ccCursor >= newLen && newLen > 0 {
				m.ccCursor = newLen - 1
			}
			if m.ccScrollOffset > m.ccCursor {
				m.ccScrollOffset = m.ccCursor
			}
			dbCmd := dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBDismissTodo(database, todoID) })
			if focusCmd := m.triggerFocusRefresh(); focusCmd != nil {
				return m, tea.Batch(dbCmd, focusCmd)
			}
			return m, dbCmd
		}
		return m, nil

	case "u":
		if len(m.undoStack) > 0 {
			entry := m.undoStack[len(m.undoStack)-1]
			m.undoStack = m.undoStack[:len(m.undoStack)-1]
			m.cc.RestoreTodo(entry.todoID, entry.prevStatus, entry.prevDoneAt)
			m.ccCursor = entry.cursorPos
			if m.ccCursor >= len(m.cc.ActiveTodos()) && len(m.cc.ActiveTodos()) > 0 {
				m.ccCursor = len(m.cc.ActiveTodos()) - 1
			}
			m.flashMessage = "Undid last action"
			m.flashMessageAt = time.Now()
			prevStatus := entry.prevStatus
			prevDoneAt := entry.prevDoneAt
			dbCmd := dbWriteCmd(m.db, func(database *sql.DB) error {
				return db.DBRestoreTodo(database, entry.todoID, prevStatus, prevDoneAt)
			})
			if focusCmd := m.triggerFocusRefresh(); focusCmd != nil {
				return m, tea.Batch(dbCmd, focusCmd)
			}
			return m, dbCmd
		}
		return m, nil

	case "d":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			todoID := activeTodos[m.ccCursor].ID
			m.cc.DeferTodo(todoID)
			dbCmd := dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBDeferTodo(database, todoID) })
			if focusCmd := m.triggerFocusRefresh(); focusCmd != nil {
				return m, tea.Batch(dbCmd, focusCmd)
			}
			return m, dbCmd
		}
		return m, nil

	case "p":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			todoID := activeTodos[m.ccCursor].ID
			m.cc.PromoteTodo(todoID)
			m.ccCursor = 0
			m.ccScrollOffset = 0
			dbCmd := dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBPromoteTodo(database, todoID) })
			if focusCmd := m.triggerFocusRefresh(); focusCmd != nil {
				return m, tea.Batch(dbCmd, focusCmd)
			}
			return m, dbCmd
		}
		return m, nil

	case " ":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			m.detailView = true
			m.detailTodoIdx = m.ccCursor
			m.textInput.Reset()
			m.textInput.Placeholder = "Tell me what changed..."
			m.textInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case "c":
		ensureCC(&m.cc)
		m.addingTodoRich = true
		m.flashMessage = ""
		m.commandConversation = nil
		m.todoTextArea.Reset()
		cmd := m.todoTextArea.Focus()
		return m, cmd

	case "b":
		m.showBacklog = !m.showBacklog
		return m, nil

	case "s":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			m.bookingMode = true
			m.bookingCursor = 2
		}
		return m, nil

	case "r":
		if !m.ccRefreshing {
			m.ccRefreshing = true
			m.ccLastRefreshTriggered = time.Now()
			return m, refreshCCCmd()
		}
		return m, nil

	case "enter":
		if len(activeTodos) > 0 && m.ccCursor < len(activeTodos) {
			todo := activeTodos[m.ccCursor]
			if todo.SessionID != "" {
				dir := todo.ProjectDir
				if dir == "" {
					home, _ := os.UserHomeDir()
					dir = home
				}
				m.Launch = &LaunchAction{
					Dir:  dir,
					Args: []string{"-r", todo.SessionID},
				}
				return m, tea.Quit
			}
			if todo.ProjectDir != "" {
				m.Launch = &LaunchAction{
					Dir:           todo.ProjectDir,
					InitialPrompt: formatTodoContext(todo),
				}
				return m, tea.Quit
			}
			m.pendingLaunchTodo = &todo
			m.activeTab = tabNew
			return m, nil
		}
		return m, nil
	}

	return m, nil
}

func (m Model) updateDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		instruction := strings.TrimSpace(m.textInput.Value())
		if instruction == "" {
			return m, nil
		}
		activeTodos := m.cc.ActiveTodos()
		if m.detailTodoIdx >= len(activeTodos) {
			m.detailView = false
			m.textInput.Blur()
			return m, nil
		}
		todo := activeTodos[m.detailTodoIdx]
		prompt := buildEditPrompt(todo, instruction)
		m.detailView = false
		m.textInput.Blur()
		m.textInput.Reset()
		m.claudeLoading = true
		m.claudeLoadingMsg = "Updating todo..."
		m.claudeLoadingTodo = todo.ID
		return m, claudeEditCmd(prompt, todo.ID)

	case "esc":
		m.detailView = false
		m.textInput.Blur()
		m.textInput.Reset()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m *Model) triggerFocusRefresh() tea.Cmd {
	if m.cc == nil || len(m.cc.ActiveTodos()) == 0 {
		return nil
	}
	m.claudeLoading = true
	m.claudeLoadingMsg = "Updating focus..."
	return claudeFocusCmd(buildFocusPrompt(m.cc))
}

func (m Model) updateAddingTodoRich(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+d":
		text := strings.TrimSpace(m.todoTextArea.Value())
		if text == "" {
			m.addingTodoRich = false
			m.todoTextArea.Blur()
			m.commandConversation = nil
			return m, nil
		}
		m.commandConversation = append(m.commandConversation, commandTurn{role: "user", text: text})
		prompt := buildCommandPromptWithHistory(m.cc, m.cfg.Name, m.commandConversation)
		m.addingTodoRich = false
		m.todoTextArea.Blur()
		m.claudeLoading = true
		m.claudeLoadingMsg = "Processing..."
		return m, claudeCommandCmd(prompt, "")

	case "esc":
		m.addingTodoRich = false
		m.todoTextArea.Blur()
		m.commandConversation = nil
		return m, nil
	}

	var cmd tea.Cmd
	m.todoTextArea, cmd = m.todoTextArea.Update(msg)
	return m, cmd
}

func (m Model) updateThreadsTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cc == nil {
		return m, nil
	}
	active := m.cc.ActiveThreads()
	paused := m.cc.PausedThreads()
	total := len(active) + len(paused)
	maxCursor := total - 1
	if maxCursor < 0 {
		maxCursor = 0
	}

	switch msg.String() {
	case "up", "k":
		if m.threadCursor > 0 {
			m.threadCursor--
		}
		return m, nil

	case "down", "j":
		if m.threadCursor < maxCursor {
			m.threadCursor++
		}
		return m, nil

	case "p":
		thread := m.threadAtCursor(active, paused)
		if thread != nil && thread.Status == "active" {
			threadID := thread.ID
			m.cc.PauseThread(threadID)
			return m, dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBPauseThread(database, threadID) })
		}
		return m, nil

	case "s":
		thread := m.threadAtCursor(active, paused)
		if thread != nil && thread.Status == "paused" {
			threadID := thread.ID
			m.cc.StartThread(threadID)
			return m, dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBStartThread(database, threadID) })
		}
		return m, nil

	case "x":
		thread := m.threadAtCursor(active, paused)
		if thread != nil {
			threadID := thread.ID
			m.cc.CloseThread(threadID)
			if m.threadCursor > 0 {
				newTotal := len(m.cc.ActiveThreads()) + len(m.cc.PausedThreads())
				if m.threadCursor >= newTotal {
					m.threadCursor = newTotal - 1
				}
				if m.threadCursor < 0 {
					m.threadCursor = 0
				}
			}
			return m, dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBCloseThread(database, threadID) })
		}
		return m, nil

	case "a":
		m.addingThread = true
		m.textInput.Reset()
		m.textInput.Placeholder = "New thread..."
		m.textInput.Focus()
		return m, textinput.Blink

	case "enter":
		thread := m.threadAtCursor(active, paused)
		if thread != nil && thread.ProjectDir != "" {
			m.Launch = &LaunchAction{Dir: thread.ProjectDir}
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m Model) threadAtCursor(active, paused []db.Thread) *db.Thread {
	if m.threadCursor < len(active) {
		return &active[m.threadCursor]
	}
	pausedIdx := m.threadCursor - len(active)
	if pausedIdx < len(paused) {
		return &paused[pausedIdx]
	}
	return nil
}

func (m Model) updateTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.textInput.Value())
		var cmd tea.Cmd
		if title != "" && m.addingThread {
			ensureCC(&m.cc)
			thread := m.cc.AddThread(title, "manual")
			threadCopy := *thread
			cmd = dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBInsertThread(database, threadCopy) })
		}
		m.addingThread = false
		m.textInput.Blur()
		return m, cmd

	case "esc":
		m.addingThread = false
		m.textInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateBooking(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		if m.bookingCursor > 0 {
			m.bookingCursor--
		}
		return m, nil

	case "right", "l":
		if m.bookingCursor < len(bookingDurations)-1 {
			m.bookingCursor++
		}
		return m, nil

	case "enter":
		activeTodos := m.cc.ActiveTodos()
		if m.ccCursor < len(activeTodos) {
			todoID := activeTodos[m.ccCursor].ID
			dur := bookingDurations[m.bookingCursor]
			m.cc.AddPendingBooking(todoID, dur)
			action := db.PendingAction{
				Type:            "booking",
				TodoID:          todoID,
				DurationMinutes: dur,
				RequestedAt:     time.Now(),
			}
			dbCmd := dbWriteCmd(m.db, func(database *sql.DB) error { return db.DBInsertPendingAction(database, action) })
			m.ccRefreshing = true
			m.flashMessage = fmt.Sprintf("Booking %dm for %s...", dur, activeTodos[m.ccCursor].Title)
			m.flashMessageAt = time.Now()
			m.bookingMode = false
			return m, tea.Batch(dbCmd, refreshCCCmd())
		}
		m.bookingMode = false
		return m, nil

	case "esc":
		m.bookingMode = false
		return m, nil
	}

	return m, nil
}

func (m Model) updateConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	doDelete := func() {
		if m.confirmResume != nil {
			if m.db != nil {
				_ = db.DBRemoveBookmark(m.db, m.confirmResume.session.SessionID)
			}
			sessions, _ := db.DBLoadBookmarks(m.db)
			m.resumeList.SetItems(buildSessionItems(sessions))
		} else {
			m.paths = db.RemovePath(m.paths, m.confirmItem.path)
			if m.db != nil {
				_ = db.DBRemovePath(m.db, m.confirmItem.path)
			}
			m.newList.SetItems(buildNewItems(m.cfg.Name, m.paths))
		}
	}

	switch msg.String() {
	case "y":
		doDelete()
		m.confirming = false
		return m, nil
	case "enter":
		if m.confirmYes {
			doDelete()
		}
		m.confirming = false
		return m, nil
	case "n", "esc":
		m.confirming = false
		return m, nil
	case "left", "right", "tab":
		m.confirmYes = !m.confirmYes
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	topPad := "\n\n\n\n\n\n"
	banner := topPad + renderGradientBanner(&m.grad, m.cfg.Name, contentMaxWidth, m.frame)

	tabBar := m.renderTabBar()

	var content string
	switch m.activeTab {
	case tabNew:
		content = m.viewNewTab()
	case tabResume:
		content = m.viewResumeTab()
	case tabCommand:
		content = m.viewCommandTab()
	case tabThreads:
		content = m.viewThreadsTab()
	}

	page := lipgloss.JoinVertical(lipgloss.Left,
		banner,
		"",
		tabBar,
		"",
		content,
	)

	if m.width > 0 && m.height > 0 {
		placed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, page)
		if m.showHelp {
			overlay := renderHelpOverlay(&m.styles, m.activeTab, m.width, m.height)
			return overlay
		}
		return placed
	}
	return page
}

func (m Model) renderTabBar() string {
	labels := []string{"New Session", "Resume", "Command Center", "Threads"}
	sep := m.styles.InactiveTab.Render(" | ")

	var parts []string
	for i, label := range labels {
		if tab(i) == m.activeTab {
			parts = append(parts, m.styles.ActiveTab.Render("> "+label))
		} else {
			parts = append(parts, m.styles.InactiveTab.Render(label))
		}
		if i < len(labels)-1 {
			parts = append(parts, sep)
		}
	}
	tabBar := strings.Join(parts, "")
	return lipgloss.PlaceHorizontal(contentMaxWidth, lipgloss.Center, tabBar)
}

func (m Model) viewNewTab() string {
	var banner string
	if m.pendingLaunchTodo != nil {
		banner = m.styles.SectionHeader.Render("Select project for: ") +
			lipgloss.NewStyle().Foreground(m.styles.ColorWhite).Bold(true).Render(m.pendingLaunchTodo.Title) +
			m.styles.Hint.Render("  (esc to cancel)")
	}
	listView := m.newList.View()
	hints := m.renderHints()
	if banner != "" {
		return lipgloss.JoinVertical(lipgloss.Left, banner, "", listView, "", hints)
	}
	return lipgloss.JoinVertical(lipgloss.Left, listView, "", hints)
}

func (m Model) viewResumeTab() string {
	var listView string
	if m.loading {
		listView = "  " + m.spinner.View() + " Loading sessions..."
	} else {
		listView = m.resumeList.View()
	}
	hints := m.renderHints()
	return lipgloss.JoinVertical(lipgloss.Left, listView, "", hints)
}

func (m Model) viewCommandTab() string {
	viewWidth := contentMaxWidth
	if m.width > 0 && m.width < viewWidth {
		viewWidth = m.width
	}
	viewHeight := m.height - 14
	if viewHeight < 10 {
		viewHeight = 10
	}

	if m.detailView && m.cc != nil {
		activeTodos := m.cc.ActiveTodos()
		if m.detailTodoIdx < len(activeTodos) {
			return renderDetailView(&m.styles, activeTodos[m.detailTodoIdx], m.textInput.View(), viewWidth)
		}
	}

	if m.ccExpanded && m.cc != nil {
		view := renderExpandedTodoView(&m.styles, &m.grad, m.cc.ActiveTodos(), m.ccCursor, m.ccExpandedOffset, m.expandedRowsPerCol(), m.expandedNumCols(), viewWidth, viewHeight, m.frame, m.claudeLoadingTodo, m.ccRefreshing)
		if m.claudeLoading {
			loadingLine := "  " + m.spinner.View() + " " + m.claudeLoadingMsg
			view = lipgloss.JoinVertical(lipgloss.Left, view, "", loadingLine)
		}
		return view
	}

	view := renderCommandCenterView(&m.styles, &m.grad, m.cc, m.cfg.Calendar.Calendars, viewWidth, viewHeight, m.ccCursor, m.ccScrollOffset, m.frame, m.claudeLoadingTodo, m.showBacklog, m.ccRefreshing)

	if m.claudeLoading {
		loadingLine := "  " + m.spinner.View() + " " + m.claudeLoadingMsg
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", loadingLine)
	}
	if m.flashMessage != "" {
		flash := lipgloss.NewStyle().Foreground(m.styles.ColorGreen).Render("  > " + m.flashMessage)
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", flash)
	}
	if m.addingTodoRich {
		inputLine := m.styles.SectionHeader.Render("COMMAND (ctrl+d submit, esc cancel):") + "\n" + m.todoTextArea.View()
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", inputLine)
	}
	if m.bookingMode {
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", m.renderBookingPicker())
	}

	return view
}

func (m Model) viewThreadsTab() string {
	viewWidth := contentMaxWidth
	if m.width > 0 && m.width < viewWidth {
		viewWidth = m.width
	}
	viewHeight := m.height - 14
	if viewHeight < 10 {
		viewHeight = 10
	}

	view := renderThreadsView(&m.styles, &m.grad, m.cc, viewWidth, viewHeight, m.threadCursor, m.frame)

	if m.addingThread {
		inputLine := m.styles.SectionHeader.Render("Add thread: ") + m.textInput.View()
		view = lipgloss.JoinVertical(lipgloss.Left, view, "", inputLine)
	}

	return view
}

func (m Model) renderBookingPicker() string {
	labels := []string{"15m", "30m", "1h", "2h", "4h"}
	var parts []string
	for i, label := range labels {
		if i == m.bookingCursor {
			parts = append(parts, m.styles.ActiveTab.Render("> "+label))
		} else {
			parts = append(parts, m.styles.InactiveTab.Render(label))
		}
	}
	picker := strings.Join(parts, "  ")
	return m.styles.SectionHeader.Render("Book time: ") + picker + m.styles.Hint.Render("  (<-> select, enter confirm, esc cancel)")
}

func (m Model) renderHints() string {
	var hints string
	if m.confirming {
		var label string
		if m.confirmResume != nil {
			label = m.confirmResume.session.Repo + " (" + m.confirmResume.session.Branch + ")"
		} else {
			label = m.confirmItem.label
		}
		yesStr := "yes"
		noStr := "no"
		if m.confirmYes {
			yesStr = m.styles.ActiveTab.Render("> yes")
			noStr = m.styles.InactiveTab.Render("no")
		} else {
			yesStr = m.styles.InactiveTab.Render("yes")
			noStr = m.styles.ActiveTab.Render("> no")
		}
		hints = m.styles.Hint.Render(fmt.Sprintf("Remove %q from saved list?  ", label)) + yesStr + m.styles.Hint.Render("  |  ") + noStr
	} else {
		hints = m.styles.Hint.Render("tab switch   up/down navigate   enter launch   del remove   esc quit")
	}
	return lipgloss.PlaceHorizontal(contentMaxWidth, lipgloss.Center, hints)
}
