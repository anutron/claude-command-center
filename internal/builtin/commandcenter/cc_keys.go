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

	// Quick todo entry
	if p.addingTodoQuick {
		return p.handleAddingTodoQuick(msg)
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
		return plugin.Action{Type: plugin.ActionUnhandled}
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

	case "shift+up":
		if len(activeTodos) > 1 && p.ccCursor > 0 && p.ccCursor < len(activeTodos) {
			// Find the absolute indices in cc.Todos for the two active todos
			activeA := p.ccCursor - 1
			activeB := p.ccCursor
			idA := activeTodos[activeA].ID
			idB := activeTodos[activeB].ID
			// Find absolute indices
			absA, absB := -1, -1
			for i := range p.cc.Todos {
				if p.cc.Todos[i].ID == idA {
					absA = i
				}
				if p.cc.Todos[i].ID == idB {
					absB = i
				}
			}
			if absA >= 0 && absB >= 0 {
				p.cc.SwapTodos(absA, absB)
				p.ccCursor--
				if p.ccCursor < p.ccScrollOffset {
					p.ccScrollOffset = p.ccCursor
				}
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBSwapTodoOrder(database, idA, idB)
				})}
			}
		}
		return plugin.NoopAction()

	case "shift+down":
		if len(activeTodos) > 1 && p.ccCursor < len(activeTodos)-1 {
			activeA := p.ccCursor
			activeB := p.ccCursor + 1
			idA := activeTodos[activeA].ID
			idB := activeTodos[activeB].ID
			absA, absB := -1, -1
			for i := range p.cc.Todos {
				if p.cc.Todos[i].ID == idA {
					absA = i
				}
				if p.cc.Todos[i].ID == idB {
					absB = i
				}
			}
			if absA >= 0 && absB >= 0 {
				p.cc.SwapTodos(absA, absB)
				p.ccCursor++
				todoViewHeight := p.normalMaxVisibleTodos()
				if p.ccCursor >= p.ccScrollOffset+todoViewHeight {
					p.ccScrollOffset++
				}
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBSwapTodoOrder(database, idA, idB)
				})}
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
			p.publishEvent("todo.completed", map[string]interface{}{"id": todoID, "title": todo.Title})
			newLen := len(p.cc.ActiveTodos())
			if p.ccCursor >= newLen && newLen > 0 {
				p.ccCursor = newLen - 1
			}
			if p.ccScrollOffset > p.ccCursor {
				p.ccScrollOffset = p.ccCursor
			}
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBCompleteTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
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
			p.publishEvent("todo.dismissed", map[string]interface{}{"id": todoID, "title": todo.Title})
			newLen := len(p.cc.ActiveTodos())
			if p.ccCursor >= newLen && newLen > 0 {
				p.ccCursor = newLen - 1
			}
			if p.ccScrollOffset > p.ccCursor {
				p.ccScrollOffset = p.ccCursor
			}
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBDismissTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
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
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case "d":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todo := activeTodos[p.ccCursor]
			todoID := todo.ID
			p.cc.DeferTodo(todoID)
			p.publishEvent("todo.deferred", map[string]interface{}{"id": todoID, "title": todo.Title})
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBDeferTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case "p":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todo := activeTodos[p.ccCursor]
			todoID := todo.ID
			p.cc.PromoteTodo(todoID)
			p.publishEvent("todo.promoted", map[string]interface{}{"id": todoID, "title": todo.Title})
			p.ccCursor = 0
			p.ccScrollOffset = 0
			dbCmd := p.dbWriteCmd(func(database *sql.DB) error { return db.DBPromoteTodo(database, todoID) })
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, focusCmd)}
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
		}
		return plugin.NoopAction()

	case " ":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			p.detailView = true
			p.detailTodoIdx = p.ccCursor
			p.textInput.Reset()
			p.textInput.Placeholder = "Tell me what changed..."
			p.textInput.Focus()
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: textinput.Blink}
		}
		return plugin.NoopAction()

	case "c":
		ensureCC(&p.cc)
		p.addingTodoRich = true
		p.flashMessage = ""
		p.commandConversation = nil
		p.todoTextArea.Reset()
		cmd := p.todoTextArea.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

	case "t":
		ensureCC(&p.cc)
		p.addingTodoQuick = true
		p.flashMessage = ""
		p.quickTodoTextArea.Reset()
		cmd := p.quickTodoTextArea.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

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
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: refreshCCCmd()}
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
			p.publishEvent("pending.todo", map[string]interface{}{
				"todo_id":     todo.ID,
				"title":       todo.Title,
				"context":     todo.Context,
				"detail":      todo.Detail,
				"who_waiting": todo.WhoWaiting,
				"due":         todo.Due,
				"effort":      todo.Effort,
			})
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
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: claudeEditCmd(p.llm, prompt, todo.ID)}

	case "esc":
		p.detailView = false
		p.textInput.Blur()
		p.textInput.Reset()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.textInput, cmd = p.textInput.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
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
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: claudeCommandCmd(p.llm, prompt, "")}

	case "esc":
		p.addingTodoRich = false
		p.todoTextArea.Blur()
		p.commandConversation = nil
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.todoTextArea, cmd = p.todoTextArea.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleAddingTodoQuick(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "ctrl+d":
		text := strings.TrimSpace(p.quickTodoTextArea.Value())
		if text == "" {
			p.addingTodoQuick = false
			p.quickTodoTextArea.Blur()
			return plugin.NoopAction()
		}
		ensureCC(&p.cc)
		var dbCmds []tea.Cmd
		var count int
		for _, line := range strings.Split(text, "\n") {
			title := strings.TrimSpace(line)
			if title == "" {
				continue
			}
			todo := p.cc.AddTodo(title)
			t := *todo
			count++
			p.publishEvent("todo.created", map[string]interface{}{"id": t.ID, "title": t.Title, "source": "quick"})
			dbCmds = append(dbCmds, p.dbWriteCmd(func(database *sql.DB) error { return db.DBInsertTodo(database, t) }))
		}
		p.addingTodoQuick = false
		p.quickTodoTextArea.Blur()
		if count > 0 {
			p.flashMessage = fmt.Sprintf("Added %d todo(s)", count)
			p.flashMessageAt = time.Now()
			if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
				dbCmds = append(dbCmds, focusCmd)
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmds...)}
		}
		return plugin.NoopAction()

	case "esc":
		p.addingTodoQuick = false
		p.quickTodoTextArea.Blur()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.quickTodoTextArea, cmd = p.quickTodoTextArea.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
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
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

	case "esc":
		p.addingThread = false
		p.textInput.Blur()
		return plugin.NoopAction()
	}

	var cmd tea.Cmd
	p.textInput, cmd = p.textInput.Update(msg)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
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
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, refreshCCCmd())}
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
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBPauseThread(database, threadID) })}
		}
		return plugin.NoopAction()

	case "s":
		thread := p.threadAtCursor(active, paused)
		if thread != nil && thread.Status == "paused" {
			threadID := thread.ID
			p.cc.StartThread(threadID)
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBStartThread(database, threadID) })}
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
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBCloseThread(database, threadID) })}
		}
		return plugin.NoopAction()

	case "a":
		p.addingThread = true
		p.textInput.Reset()
		p.textInput.Placeholder = "New thread..."
		p.textInput.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: textinput.Blink}

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
