package commandcenter

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (p *Plugin) handleDetailView(msg tea.KeyMsg) plugin.Action {
	switch p.detailMode {
	case "editingField":
		return p.handleDetailEditingField(msg)
	case "selectingStatus":
		return p.handleDetailStatusSelect(msg)
	case "selectingPath":
		return p.handleDetailPathSelect(msg)
	case "commandInput":
		return p.handleDetailCommandInput(msg)
	default:
		return p.handleDetailViewing(msg)
	}
}

// statusOptions are the available status values for inline selection.
var statusOptions = []string{"active", "waiting", "completed"}

// detailFieldCount is the number of cyclable fields in the detail view.
const detailFieldCount = 3 // 0=Status, 1=Due, 2=ProjectDir

func (p *Plugin) handleDetailViewing(msg tea.KeyMsg) plugin.Action {
	// While showing a notice, ignore all keys except esc
	if p.detailNotice != "" {
		if msg.String() == "esc" {
			p.detailNotice = ""
			p.detailView = false
			p.detailMode = "viewing"
			return plugin.NoopAction()
		}
		return plugin.NoopAction()
	}

	// Block edit/mutation operations when an agent is actively updating this todo.
	agentActive := false
	if todo := p.detailTodo(); todo != nil && todo.SessionStatus == "active" {
		agentActive = true
	}

	switch msg.String() {
	case "tab":
		p.detailSelectedField = (p.detailSelectedField + 1) % detailFieldCount
		return plugin.ConsumedAction()
	case "shift+tab":
		p.detailSelectedField = (p.detailSelectedField - 1 + detailFieldCount) % detailFieldCount
		return plugin.ConsumedAction()
	case "enter":
		if agentActive {
			p.flashMessage = "Todo is being updated by agent"
			p.flashMessageAt = time.Now()
			return plugin.ConsumedAction()
		}
		return p.enterDetailFieldEdit()
	case "x":
		if agentActive {
			p.flashMessage = "Todo is being updated by agent"
			p.flashMessageAt = time.Now()
			return plugin.ConsumedAction()
		}
		return p.detailCompleteTodo()
	case "X":
		if agentActive {
			p.flashMessage = "Todo is being updated by agent"
			p.flashMessageAt = time.Now()
			return plugin.ConsumedAction()
		}
		return p.detailDismissTodo()
	case "j":
		// Next todo
		activeTodos := p.cc.ActiveTodos()
		idx := p.detailTodoActiveIndex()
		if idx >= 0 && idx < len(activeTodos)-1 {
			p.detailTodoID = activeTodos[idx+1].ID
			p.detailSelectedField = 0
		}
		return plugin.ConsumedAction()
	case "k":
		// Previous todo
		activeTodos := p.cc.ActiveTodos()
		idx := p.detailTodoActiveIndex()
		if idx > 0 {
			p.detailTodoID = activeTodos[idx-1].ID
			p.detailSelectedField = 0
		}
		return plugin.ConsumedAction()
	case "o":
		// If todo has a session_id, join/resume that session directly
		if todo := p.detailTodo(); todo != nil {
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
						"todo_id":   todo.ID,
					},
				}
			}
			// Always go through task runner for new launches
			p.enterTaskRunner(*todo)
		}
		return plugin.NoopAction()
	case "c":
		if agentActive {
			p.flashMessage = "Todo is being updated by agent"
			p.flashMessageAt = time.Now()
			return plugin.ConsumedAction()
		}
		p.detailMode = "commandInput"
		p.commandTextArea.Reset()
		inputWidth := p.textareaWidth()
		p.commandTextArea.SetWidth(inputWidth)
		cmd := p.commandTextArea.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	case "g":
		p.gPending = true
		return plugin.NoopAction()
	case "esc":
		p.detailView = false
		p.detailMode = "viewing"
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

// detailCompleteTodo marks the current detail todo as done and shows a notice.
func (p *Plugin) detailCompleteTodo() plugin.Action {
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		return plugin.NoopAction()
	}
	todo := *todoPtr
	p.undoStack = append(p.undoStack, undoEntry{
		todoID:     todo.ID,
		prevStatus: todo.Status,
		prevDoneAt: todo.CompletedAt,
		cursorPos:  p.ccCursor,
	})
	todoID := todo.ID
	p.cc.CompleteTodo(todoID)
	p.publishEvent("todo.completed", map[string]interface{}{"id": todoID, "title": todo.Title})

	// Adjust list cursor to stay in bounds (use filteredTodos to match the view)
	newFiltered := len(p.filteredTodos())
	if p.ccCursor >= newFiltered && newFiltered > 0 {
		p.ccCursor = newFiltered - 1
	}
	if p.ccScrollOffset > p.ccCursor {
		p.ccScrollOffset = p.ccCursor
	}

	p.detailNotice = fmt.Sprintf("Done: %s", flattenTitle(todo.Title))
	p.detailNoticeType = "done"
	p.detailNoticeAt = time.Now()

	dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBCompleteTodo(database, todoID) })
	if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
	}
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
}

// detailDismissTodo removes the current detail todo and shows a notice.
func (p *Plugin) detailDismissTodo() plugin.Action {
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		return plugin.NoopAction()
	}
	todo := *todoPtr
	p.undoStack = append(p.undoStack, undoEntry{
		todoID:     todo.ID,
		prevStatus: todo.Status,
		prevDoneAt: todo.CompletedAt,
		cursorPos:  p.ccCursor,
	})
	todoID := todo.ID
	p.cc.RemoveTodo(todoID)
	p.publishEvent("todo.dismissed", map[string]interface{}{"id": todoID, "title": todo.Title})

	// Adjust list cursor to stay in bounds (use filteredTodos to match the view)
	newFiltered := len(p.filteredTodos())
	if p.ccCursor >= newFiltered && newFiltered > 0 {
		p.ccCursor = newFiltered - 1
	}
	if p.ccScrollOffset > p.ccCursor {
		p.ccScrollOffset = p.ccCursor
	}

	p.detailNotice = fmt.Sprintf("Removed: %s", flattenTitle(todo.Title))
	p.detailNoticeType = "removed"
	p.detailNoticeAt = time.Now()

	dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBDismissTodo(database, todoID) })
	if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
	}
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
}

func (p *Plugin) enterDetailFieldEdit() plugin.Action {
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		return plugin.NoopAction()
	}
	todo := *todoPtr

	switch p.detailSelectedField {
	case 0: // Status — show inline selector
		p.detailMode = "selectingStatus"
		p.detailStatusCursor = 0
		for i, opt := range statusOptions {
			if opt == todo.Status {
				p.detailStatusCursor = i
				break
			}
		}
		return plugin.NoopAction()
	case 1: // Due — open text input
		p.detailMode = "editingField"
		p.detailFieldInput.Reset()
		p.detailFieldInput.Placeholder = "mm dd, or natural language"
		p.detailFieldInput.SetValue(todo.Due)
		p.detailFieldInput.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: textinput.Blink}
	case 2: // ProjectDir — open scrollable path picker
		// Reload paths from DB so newly added sessions are available.
		if p.database != nil {
			if paths, err := db.DBLoadPaths(p.database); err == nil {
				p.detailPaths = paths
			}
		}
		if len(p.detailPaths) == 0 {
			// No paths available; open text input instead
			p.detailMode = "editingField"
			p.detailFieldInput.Reset()
			p.detailFieldInput.Placeholder = "/path/to/project"
			p.detailFieldInput.SetValue(todo.ProjectDir)
			p.detailFieldInput.Focus()
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: textinput.Blink}
		}
		// Enter path selection mode
		p.detailMode = "selectingPath"
		p.detailPathFilter = ""
		p.detailPathCursor = 0
		for i, path := range p.detailPaths {
			if path == todo.ProjectDir {
				p.detailPathCursor = i
				break
			}
		}
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

func (p *Plugin) commitDetailFieldEdit(todo db.Todo, field, value string) plugin.Action {
	// Apply the change in-memory
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todo.ID {
			switch field {
			case "status":
				p.cc.Todos[i].Status = value
			case "due":
				p.cc.Todos[i].Due = value
			case "project_dir":
				p.cc.Todos[i].ProjectDir = value
			case "proposed_prompt":
				p.cc.Todos[i].ProposedPrompt = value
			}
			// Persist full todo update
			updated := p.cc.Todos[i]
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
				return db.DBUpdateTodo(database, updated.ID, updated)
			})
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
		}
	}
	return plugin.NoopAction()
}

func (p *Plugin) handleDetailStatusSelect(msg tea.KeyMsg) plugin.Action {
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		p.detailMode = "viewing"
		return plugin.NoopAction()
	}
	todo := *todoPtr

	switch msg.String() {
	case "left", "h":
		if p.detailStatusCursor > 0 {
			p.detailStatusCursor--
		}
		return plugin.NoopAction()
	case "right", "l":
		if p.detailStatusCursor < len(statusOptions)-1 {
			p.detailStatusCursor++
		}
		return plugin.NoopAction()
	case "enter":
		newStatus := statusOptions[p.detailStatusCursor]
		p.detailMode = "viewing"
		return p.commitDetailFieldEdit(todo, "status", newStatus)
	case "esc":
		p.detailMode = "viewing"
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

// filteredPaths returns the subset of detailPaths matching the current filter.
func (p *Plugin) filteredPaths() []string {
	if p.detailPathFilter == "" {
		return p.detailPaths
	}
	lower := strings.ToLower(p.detailPathFilter)
	var out []string
	for _, path := range p.detailPaths {
		if strings.Contains(strings.ToLower(path), lower) {
			out = append(out, path)
		}
	}
	return out
}

func (p *Plugin) handleDetailPathSelect(msg tea.KeyMsg) plugin.Action {
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		p.detailMode = "viewing"
		return plugin.NoopAction()
	}
	todo := *todoPtr

	filtered := p.filteredPaths()

	switch msg.String() {
	case "up", "k":
		if p.detailPathCursor > 0 {
			p.detailPathCursor--
		}
		return plugin.NoopAction()
	case "down", "j":
		if p.detailPathCursor < len(filtered)-1 {
			p.detailPathCursor++
		}
		return plugin.NoopAction()
	case "enter":
		if len(filtered) > 0 && p.detailPathCursor < len(filtered) {
			newPath := filtered[p.detailPathCursor]
			p.detailMode = "viewing"
			p.detailPathFilter = ""
			return p.commitDetailFieldEdit(todo, "project_dir", newPath)
		}
		p.detailMode = "viewing"
		p.detailPathFilter = ""
		return plugin.NoopAction()
	case "esc":
		p.detailMode = "viewing"
		p.detailPathFilter = ""
		return plugin.NoopAction()
	case "backspace":
		if len(p.detailPathFilter) > 0 {
			p.detailPathFilter = p.detailPathFilter[:len(p.detailPathFilter)-1]
			p.detailPathCursor = 0
		}
		return plugin.NoopAction()
	default:
		// Typing characters filters the list
		key := msg.String()
		if len(key) == 1 {
			p.detailPathFilter += key
			p.detailPathCursor = 0
		}
		return plugin.NoopAction()
	}
}

func (p *Plugin) handleDetailEditingField(msg tea.KeyMsg) plugin.Action {
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		p.detailMode = "viewing"
		p.detailFieldInput.Blur()
		return plugin.NoopAction()
	}
	todo := *todoPtr

	switch msg.String() {
	case "enter":
		value := strings.TrimSpace(p.detailFieldInput.Value())
		p.detailMode = "viewing"
		p.detailFieldInput.Blur()
		switch p.detailSelectedField {
		case 1: // Due
			if value == "" {
				return p.commitDetailFieldEdit(todo, "due", "")
			}
			parsed, ok := parseDueDate(value, time.Now())
			if ok {
				return p.commitDetailFieldEdit(todo, "due", parsed)
			}
			// Not a recognized format — use LLM to parse natural language
			p.claudeLoading = true
			p.claudeLoadingMsg = "Parsing date..."
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: claudeDateParseCmd(p.llm, value, todo.ID)}
		case 2: // ProjectDir
			return p.commitDetailFieldEdit(todo, "project_dir", value)
		}
		return plugin.NoopAction()
	case "esc":
		p.detailMode = "viewing"
		p.detailFieldInput.Blur()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.detailFieldInput, cmd = p.detailFieldInput.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleDetailCommandInput(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		// Enter submits the command (not a newline)
		instruction := strings.TrimSpace(p.commandTextArea.Value())
		if instruction == "" {
			return plugin.NoopAction()
		}
		todoPtr := p.detailTodo()
		if todoPtr == nil {
			p.detailMode = "viewing"
			p.commandTextArea.Blur()
			return plugin.NoopAction()
		}
		todo := *todoPtr
		prompt := buildEditPrompt(todo, instruction)
		p.detailView = false
		p.detailMode = "viewing"
		p.commandTextArea.Blur()
		p.commandTextArea.Reset()
		p.claudeLoading = true
		p.claudeLoadingMsg = "Updating todo..."
		p.claudeLoadingTodo = todo.ID
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: claudeEditCmd(p.llm, prompt, todo.ID)}
	case "esc":
		p.detailMode = "viewing"
		p.commandTextArea.Blur()
		p.commandTextArea.Reset()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.commandTextArea, cmd = p.commandTextArea.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}
