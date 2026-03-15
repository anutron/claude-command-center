package commandcenter

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// triageFilterOrder defines the tab order for triage filters in expanded view.
var triageFilterOrder = []string{"accepted", "new", "review", "blocked", "active", "all"}

// HandleKey handles key input and returns an action.
func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	// Help overlay
	if p.showHelp {
		p.showHelp = false
		return plugin.NoopAction()
	}

	// Task runner view (sub-view of detail)
	if p.taskRunnerView && p.detailView {
		return p.handleTaskRunnerView(msg)
	}

	// Detail view
	if p.detailView {
		return p.handleDetailView(msg)
	}

	// Search input
	if p.searchActive {
		return p.handleSearchInput(msg)
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
		// Clear search filter if active (before collapsing expanded view)
		if strings.TrimSpace(p.searchInput.Value()) != "" {
			p.searchInput.SetValue("")
			p.ccCursor = 0
			p.ccScrollOffset = 0
			p.ccExpandedOffset = 0
			return plugin.NoopAction()
		}
		if p.ccExpanded {
			p.ccExpanded = false
			p.ccExpandedCols = 0
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
	if p.cc == nil {
		return plugin.NoopAction()
	}
	activeTodos := p.filteredTodos()
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
					p.ccScrollOffset++
				}
			}
		}
		return plugin.NoopAction()

	case "left", "h":
		if p.ccExpanded {
			rowsPerCol := p.expandedRowsPerCol()
			numCols := p.expandedNumCols()
			pageSize := rowsPerCol * numCols
			relIdx := p.ccCursor - p.ccExpandedOffset
			col := relIdx / rowsPerCol
			row := relIdx % rowsPerCol
			if col > 0 {
				// Move to previous column on same page
				p.ccCursor = p.ccExpandedOffset + (col-1)*rowsPerCol + row
				if p.ccCursor > maxCursor {
					p.ccCursor = maxCursor
				}
			} else if p.ccExpandedOffset > 0 {
				// Paginate left: go to previous page, land in last column same row
				p.ccExpandedOffset -= pageSize
				if p.ccExpandedOffset < 0 {
					p.ccExpandedOffset = 0
				}
				p.ccCursor = p.ccExpandedOffset + (numCols-1)*rowsPerCol + row
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
			pageSize := rowsPerCol * numCols
			relIdx := p.ccCursor - p.ccExpandedOffset
			col := relIdx / rowsPerCol
			row := relIdx % rowsPerCol
			if col < numCols-1 {
				// Move to next column on same page
				target := p.ccExpandedOffset + (col+1)*rowsPerCol + row
				if target > maxCursor {
					target = maxCursor
				}
				p.ccCursor = target
			} else if p.ccExpandedOffset+pageSize <= maxCursor {
				// Paginate right: go to next page, land in first column same row
				p.ccExpandedOffset += pageSize
				p.ccCursor = p.ccExpandedOffset + row
				if p.ccCursor > maxCursor {
					p.ccCursor = maxCursor
				}
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
		// Cycle expanded view: collapsed → 2-col → 1-col → collapsed
		if !p.ccExpanded {
			p.ccExpanded = true
			p.ccExpandedCols = 2
			p.ccExpandedOffset = 0
		} else if p.ccExpandedCols == 2 {
			p.ccExpandedCols = 1
			p.ccExpandedOffset = 0
		} else {
			p.ccExpanded = false
			p.ccExpandedCols = 0
			p.ccExpandedOffset = 0
			p.ccScrollOffset = 0
			if p.ccCursor >= todoViewHeight {
				p.ccCursor = todoViewHeight - 1
			}
		}
		return plugin.NoopAction()

	case "c":
		ensureCC(&p.cc)
		p.addingTodoRich = true
		p.flashMessage = ""
		p.commandConversation = nil
		p.todoTextArea.Reset()
		taWidth := p.textareaWidth()
		p.todoTextArea.SetWidth(taWidth)
		cmd := p.todoTextArea.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

	case "t":
		ensureCC(&p.cc)
		p.addingTodoQuick = true
		p.flashMessage = ""
		p.quickTodoTextArea.Reset()
		taWidth := p.textareaWidth()
		p.quickTodoTextArea.SetWidth(taWidth)
		cmd := p.quickTodoTextArea.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

	case "/":
		p.searchActive = true
		p.searchInput.Focus()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: textinput.Blink}

	case "tab":
		if p.ccExpanded {
			// Cycle triage filter forward
			idx := 0
			for i, f := range triageFilterOrder {
				if f == p.triageFilter {
					idx = i
					break
				}
			}
			p.triageFilter = triageFilterOrder[(idx+1)%len(triageFilterOrder)]
			p.ccCursor = 0
			p.ccExpandedOffset = 0
			return plugin.ConsumedAction()
		}
		return plugin.Action{Type: plugin.ActionUnhandled}

	case "shift+tab":
		if p.ccExpanded {
			// Cycle triage filter backward
			idx := 0
			for i, f := range triageFilterOrder {
				if f == p.triageFilter {
					idx = i
					break
				}
			}
			p.triageFilter = triageFilterOrder[(idx-1+len(triageFilterOrder))%len(triageFilterOrder)]
			p.ccCursor = 0
			p.ccExpandedOffset = 0
			return plugin.ConsumedAction()
		}
		return plugin.Action{Type: plugin.ActionUnhandled}

	case "y":
		if p.ccExpanded {
			filtered := p.filteredTodos()
			if len(filtered) > 0 && p.ccCursor < len(filtered) {
				todo := filtered[p.ccCursor]
				p.cc.AcceptTodo(todo.ID)
				todoID := todo.ID
				// Adjust cursor if the filtered list will shrink
				newFiltered := p.filteredTodos()
				if p.ccCursor >= len(newFiltered) && len(newFiltered) > 0 {
					p.ccCursor = len(newFiltered) - 1
				}
				dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBAcceptTodo(database, todoID)
				})
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
			}
		}
		return plugin.NoopAction()

	case "Y":
		if p.ccExpanded {
			filtered := p.filteredTodos()
			if len(filtered) > 0 && p.ccCursor < len(filtered) {
				todo := filtered[p.ccCursor]
				p.cc.AcceptTodo(todo.ID)
				todoID := todo.ID

				// Find the todo in the full active list for detail view
				allActive := p.cc.ActiveTodos()
				detailIdx := 0
				for i, t := range allActive {
					if t.ID == todoID {
						detailIdx = i
						break
					}
				}

				// Enter detail/task runner view
				p.detailView = true
				p.detailTodoID = activeTodos[detailIdx].ID
				p.detailMode = "viewing"
				p.detailSelectedField = 0
				p.textInput.Reset()
				p.textInput.Placeholder = "Tell me what changed..."
				p.detailFieldInput.Reset()
				p.enterTaskRunner(allActive[detailIdx])

				dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBAcceptTodo(database, todoID)
				})
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
			}
		}
		return plugin.NoopAction()

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
			p.detailView = true
			p.detailTodoID = activeTodos[p.ccCursor].ID
			p.detailMode = "viewing"
			p.detailSelectedField = 0
			p.textInput.Reset()
			p.textInput.Placeholder = "Tell me what changed..."
			p.detailFieldInput.Reset()
			return plugin.NoopAction()
		}
		return plugin.NoopAction()

	case "o":
		if len(activeTodos) > 0 && p.ccCursor < len(activeTodos) {
			todo := activeTodos[p.ccCursor]
			// If todo has an existing session, resume it directly
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
			// Otherwise, enter detail view + task runner (don't launch directly)
			p.detailView = true
			p.detailTodoID = todo.ID
			p.detailMode = "viewing"
			p.detailSelectedField = 0
			p.textInput.Reset()
			p.detailFieldInput.Reset()
			p.enterTaskRunner(todo)
			return plugin.NoopAction()
		}
		return plugin.NoopAction()
	}

	return plugin.NoopAction()
}

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

	// Adjust list cursor to stay in bounds
	newLen := len(p.cc.ActiveTodos())
	if p.ccCursor >= newLen && newLen > 0 {
		p.ccCursor = newLen - 1
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

	// Adjust list cursor to stay in bounds
	newLen := len(p.cc.ActiveTodos())
	if p.ccCursor >= newLen && newLen > 0 {
		p.ccCursor = newLen - 1
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

func (p *Plugin) handleSearchInput(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		p.searchActive = false
		p.searchInput.Blur()
		// Ensure cursor is valid for the (possibly shorter) filtered list
		filtered := p.filteredTodos()
		if p.ccCursor >= len(filtered) {
			if len(filtered) > 0 {
				p.ccCursor = len(filtered) - 1
			} else {
				p.ccCursor = 0
			}
		}
		p.ccScrollOffset = 0
		p.ccExpandedOffset = 0
		return plugin.NoopAction()
	case "esc":
		p.searchActive = false
		p.searchInput.Blur()
		p.searchInput.SetValue("")
		p.ccCursor = 0
		p.ccScrollOffset = 0
		p.ccExpandedOffset = 0
		return plugin.NoopAction()
	}

	prevQuery := p.searchInput.Value()
	var cmd tea.Cmd
	p.searchInput, cmd = p.searchInput.Update(msg)
	// Reset cursor when filter changes
	if p.searchInput.Value() != prevQuery {
		p.ccCursor = 0
		p.ccScrollOffset = 0
		p.ccExpandedOffset = 0
	}
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
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

// enterTaskRunner initializes the task runner view for a given todo.
func (p *Plugin) enterTaskRunner(todo db.Todo) {
	p.taskRunnerView = true
	p.taskRunnerStep = 1

	// Initialize defaults from config
	agentCfg := p.cfg.Agent
	p.taskRunnerMode = agentCfg.DefaultMode
	if p.taskRunnerMode == "" {
		p.taskRunnerMode = "normal"
	}
	// Headless agents should always execute — default to "auto" permission.
	p.taskRunnerPerm = "auto"
	p.taskRunnerBudget = agentCfg.DefaultBudget
	if p.taskRunnerBudget <= 0 {
		p.taskRunnerBudget = 5.00
	}
	p.taskRunnerRefining = false
	p.taskRunnerReviewing = false
	p.taskRunnerInputting = false
	p.taskRunnerReviewClean = ""
	p.taskRunnerPickingPath = false
	p.taskRunnerPathFilter = ""
	p.taskRunnerLaunchCursor = 0
	// Initialize path cursor to match the todo's project dir
	p.taskRunnerPathCursor = -1 // -1 means "use todo's original project dir"
	for i, path := range p.detailPaths {
		if path == todo.ProjectDir {
			p.taskRunnerPathCursor = i
			break
		}
	}

	// Restore saved wizard selections for this todo (project, mode).
	// First check in-memory cache, then fall back to persisted launch_mode.
	if saved, ok := p.wizardSelections[todo.ID]; ok {
		p.taskRunnerPathCursor = saved.pathCursor
		p.taskRunnerMode = saved.mode
	} else if todo.LaunchMode != "" {
		p.taskRunnerMode = todo.LaunchMode
	}

	// Auto-open path picker if todo has no project dir and no saved selection
	_, hasSaved := p.wizardSelections[todo.ID]
	if todo.ProjectDir == "" && len(p.detailPaths) > 0 && !hasSaved {
		p.taskRunnerPickingPath = true
		p.taskRunnerPathFilter = ""
	}

	// Build prompt text from todo context
	promptText := todo.ProposedPrompt
	if promptText == "" {
		promptText = formatTodoContext(todo)
	}
	p.taskRunnerPromptText = promptText

	// Set up viewport for prompt. Use minimal initial dimensions;
	// viewCommandTab will resize to the correct size on the first render.
	vp := viewport.New(40, 5)
	vp.SetContent(promptText)
	p.taskRunnerPrompt = vp
}

// saveWizardSelections persists the current wizard project/mode selections for the active todo.
func (p *Plugin) saveWizardSelections() {
	if p.detailTodoID != "" {
		p.wizardSelections[p.detailTodoID] = wizardSelection{
			pathCursor: p.taskRunnerPathCursor,
			mode:       p.taskRunnerMode,
		}
	}
}

// taskRunnerModes and taskRunnerPerms are the available options for cycling.
var taskRunnerModes = []string{"normal", "worktree", "sandbox"}
var taskRunnerPerms = []string{"default", "plan", "auto"}

func (p *Plugin) handleTaskRunnerView(msg tea.KeyMsg) plugin.Action {
	// Consume tab/shift-tab so they don't propagate to the host's tab navigation.
	if msg.Type == tea.KeyTab || msg.Type == tea.KeyShiftTab {
		return plugin.ConsumedAction()
	}

	// Path picker sub-mode (available from step 1)
	if p.taskRunnerPickingPath {
		return p.handleTaskRunnerPathSelect(msg)
	}

	switch p.taskRunnerStep {
	case 1:
		return p.handleWizardStep1(msg)
	case 2:
		return p.handleWizardStep2(msg)
	case 3:
		return p.handleWizardStep3(msg)
	}
	return plugin.NoopAction()
}

// handleWizardStep1 handles Step 1: Project selection.
func (p *Plugin) handleWizardStep1(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		p.taskRunnerStep = 2
		return plugin.NoopAction()
	case "/":
		if len(p.detailPaths) > 0 {
			p.taskRunnerPickingPath = true
			p.taskRunnerPathFilter = ""
		}
		return plugin.NoopAction()
	case "esc":
		p.saveWizardSelections()
		p.taskRunnerView = false
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

// handleWizardStep2 handles Step 2: Mode selection.
func (p *Plugin) handleWizardStep2(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "left", "h":
		idx := indexOf(taskRunnerModes, p.taskRunnerMode)
		idx = (idx - 1 + len(taskRunnerModes)) % len(taskRunnerModes)
		p.taskRunnerMode = taskRunnerModes[idx]
		return plugin.NoopAction()
	case "right", "l":
		idx := indexOf(taskRunnerModes, p.taskRunnerMode)
		idx = (idx + 1) % len(taskRunnerModes)
		p.taskRunnerMode = taskRunnerModes[idx]
		return plugin.NoopAction()
	case "enter":
		p.taskRunnerStep = 3
		return plugin.NoopAction()
	case "esc":
		p.saveWizardSelections()
		p.taskRunnerStep = 1
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

// handleWizardStep3 handles Step 3: Prompt review & launch.
func (p *Plugin) handleWizardStep3(msg tea.KeyMsg) plugin.Action {
	// Blocking modal while Plannotator is open in browser
	if p.taskRunnerReviewing {
		if msg.String() == "esc" {
			p.taskRunnerReviewing = false
			p.flashMessage = "Review cancelled"
			p.flashMessageAt = time.Now()
			// Note: the background plannotator process will still be running
			// but its result will be ignored since reviewing is false.
		}
		return plugin.NoopAction()
	}

	// If user is typing instructions for AI refine (c key)
	if p.taskRunnerInputting {
		switch msg.Type {
		case tea.KeyEnter:
			instruction := p.taskRunnerInstructInput.Value()
			p.taskRunnerInputting = false
			if instruction != "" {
				return p.taskRunnerRefineWithInstruction(instruction)
			}
			return plugin.NoopAction()
		case tea.KeyEscape:
			p.taskRunnerInputting = false
			return plugin.NoopAction()
		default:
			var cmd tea.Cmd
			p.taskRunnerInstructInput, cmd = p.taskRunnerInstructInput.Update(msg)
			if cmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
			}
			return plugin.NoopAction()
		}
	}

	switch msg.String() {
	case "j":
		p.taskRunnerPrompt.LineDown(1)
		return plugin.NoopAction()
	case "k":
		p.taskRunnerPrompt.LineUp(1)
		return plugin.NoopAction()
	case "left", "h":
		if p.taskRunnerLaunchCursor > 0 {
			p.taskRunnerLaunchCursor--
		}
		return plugin.NoopAction()
	case "right", "l":
		if p.taskRunnerLaunchCursor < 2 {
			p.taskRunnerLaunchCursor++
		}
		return plugin.NoopAction()
	case "enter":
		switch p.taskRunnerLaunchCursor {
		case 0:
			return p.taskRunnerLaunchInteractive()
		case 1:
			return p.taskRunnerLaunch(false) // queue
		case 2:
			return p.taskRunnerLaunch(true) // run now
		}
	case "e":
		// Launch external editor to refine the prompt.
		if todoPtr := p.detailTodo(); todoPtr != nil {
			todo := *todoPtr
			prompt := todo.ProposedPrompt
			if prompt == "" {
				prompt = formatTodoContext(todo)
			}
			cmd := launchPlannotator(todo.ID, prompt)
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return plugin.NoopAction()
	case "c":
		// Open instruction input for AI-guided prompt refinement.
		p.taskRunnerInputting = true
		ta := textarea.New()
		ta.Placeholder = "Instructions for AI to rewrite prompt..."
		ta.CharLimit = 0
		ta.ShowLineNumbers = false
		ta.SetWidth(p.textareaWidth())
		ta.SetHeight(3)
		ta.FocusedStyle.Base = ta.FocusedStyle.Base.Foreground(p.styles.ColorWhite)
		ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
		ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
		ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(p.styles.ColorMuted)
		ta.Focus()
		p.taskRunnerInstructInput = ta
		return plugin.NoopAction()
	case "r", "p":
		return p.taskRunnerReviewLoop()
	case "esc":
		p.saveWizardSelections()
		p.taskRunnerStep = 2
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

// handleTaskRunnerPathSelect handles key input in the task runner's scrollable path picker.
func (p *Plugin) handleTaskRunnerPathSelect(msg tea.KeyMsg) plugin.Action {
	filtered := p.taskRunnerFilteredPaths()

	// Clamp cursor to valid range
	if len(filtered) == 0 {
		p.taskRunnerPathCursor = 0
	} else if p.taskRunnerPathCursor >= len(filtered) {
		p.taskRunnerPathCursor = len(filtered) - 1
	}

	switch msg.String() {
	case "up", "k":
		if p.taskRunnerPathCursor > 0 {
			p.taskRunnerPathCursor--
		}
		return plugin.NoopAction()
	case "down", "j":
		if p.taskRunnerPathCursor < len(filtered)-1 {
			p.taskRunnerPathCursor++
		}
		return plugin.NoopAction()
	case "enter":
		if len(filtered) > 0 && p.taskRunnerPathCursor >= 0 && p.taskRunnerPathCursor < len(filtered) {
			// Find the index of the selected path in the full detailPaths list
			selectedPath := filtered[p.taskRunnerPathCursor]
			for i, path := range p.detailPaths {
				if path == selectedPath {
					p.taskRunnerPathCursor = i
					break
				}
			}
		}
		p.taskRunnerPickingPath = false
		p.taskRunnerPathFilter = ""
		return plugin.NoopAction()
	case "esc":
		p.taskRunnerPickingPath = false
		p.taskRunnerPathFilter = ""
		return plugin.NoopAction()
	case "backspace":
		if len(p.taskRunnerPathFilter) > 0 {
			p.taskRunnerPathFilter = p.taskRunnerPathFilter[:len(p.taskRunnerPathFilter)-1]
			p.taskRunnerPathCursor = 0
		}
		return plugin.NoopAction()
	default:
		// Typing characters filters the list
		key := msg.String()
		if len(key) == 1 {
			p.taskRunnerPathFilter += key
			p.taskRunnerPathCursor = 0
		}
		return plugin.NoopAction()
	}
}

// taskRunnerRefineWithInstruction sends user instructions + prompt to LLM for rewriting.
func (p *Plugin) taskRunnerRefineWithInstruction(instruction string) plugin.Action {
	if p.taskRunnerRefining {
		return plugin.NoopAction()
	}
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		return plugin.NoopAction()
	}
	prompt := todoPtr.ProposedPrompt
	if prompt == "" {
		prompt = formatTodoContext(*todoPtr)
	}
	p.taskRunnerRefining = true
	cmd := claudeRefinePromptWithInstructionCmd(p.llm, todoPtr.ID, prompt, instruction)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) taskRunnerReviewLoop() plugin.Action {
	if p.taskRunnerRefining || p.taskRunnerReviewing {
		return plugin.NoopAction()
	}
	todoPtr := p.detailTodo()
	if todoPtr == nil {
		return plugin.NoopAction()
	}
	prompt := todoPtr.ProposedPrompt
	if prompt == "" {
		prompt = formatTodoContext(*todoPtr)
	}
	p.taskRunnerReviewClean = prompt
	p.taskRunnerReviewing = true
	cmd := launchPlannotatorReview(todoPtr.ID, prompt, 1)
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

// taskRunnerFilteredPaths returns the path list filtered by the task runner's filter string.
func (p *Plugin) taskRunnerFilteredPaths() []string {
	if p.taskRunnerPathFilter == "" {
		return p.detailPaths
	}
	lower := strings.ToLower(p.taskRunnerPathFilter)
	var out []string
	for _, path := range p.detailPaths {
		if strings.Contains(strings.ToLower(path), lower) {
			out = append(out, path)
		}
	}
	return out
}

// taskRunnerLaunch launches the agent, optionally forcing immediate start.
// taskRunnerLaunchInteractive launches an interactive Claude session with the
// todo's prompt as context. The user works on the todo themselves in Claude.
// Sets session_status to "active" so the todo shows as in-progress.
func (p *Plugin) taskRunnerLaunchInteractive() plugin.Action {
	if todoPtr := p.detailTodo(); todoPtr != nil {
		todo := *todoPtr
		prompt := todo.ProposedPrompt
		if prompt == "" {
			prompt = formatTodoContext(todo)
		}
		projectDir := todo.ProjectDir
		if p.taskRunnerPathCursor >= 0 && p.taskRunnerPathCursor < len(p.detailPaths) {
			projectDir = p.detailPaths[p.taskRunnerPathCursor]
		}
		if projectDir == "" {
			home, _ := os.UserHomeDir()
			projectDir = home
		}

		// Mark todo as active and persist project dir + launch mode so resume uses the right settings
		p.setTodoSessionStatus(todo.ID, "active")
		p.setTodoProjectDir(todo.ID, projectDir)
		p.setTodoLaunchMode(todo.ID, p.taskRunnerMode)
		p.cc.AcceptTodo(todo.ID)

		p.taskRunnerView = false
		p.detailView = false

		args := map[string]string{
			"dir":            projectDir,
			"initial_prompt": prompt,
			"todo_id":        todo.ID,
		}
		if p.taskRunnerMode == "worktree" {
			args["worktree"] = "true"
		}

		var cmds []tea.Cmd
		cmds = append(cmds, p.persistSessionStatus(todo.ID, "active"))
		cmds = append(cmds, p.persistProjectDir(todo.ID, projectDir))
		cmds = append(cmds, p.persistLaunchMode(todo.ID, p.taskRunnerMode))
		cmds = append(cmds, p.dbWriteCmd(func(database *sql.DB) error {
			return db.DBAcceptTodo(database, todo.ID)
		}))

		return plugin.Action{
			Type:   "launch",
			Args:   args,
			TeaCmd: tea.Batch(cmds...),
		}
	}
	p.taskRunnerView = false
	p.detailView = false
	return plugin.NoopAction()
}

func (p *Plugin) taskRunnerLaunch(immediate bool) plugin.Action {
	if todoPtr := p.detailTodo(); todoPtr != nil {
		todo := *todoPtr
		prompt := todo.ProposedPrompt
		if prompt == "" {
			prompt = formatTodoContext(todo)
		}
		// Use task runner's selected path if available, otherwise fall back to todo's
		projectDir := todo.ProjectDir
		if p.taskRunnerPathCursor >= 0 && p.taskRunnerPathCursor < len(p.detailPaths) {
			projectDir = p.detailPaths[p.taskRunnerPathCursor]
		}
		if projectDir == "" {
			home, _ := os.UserHomeDir()
			projectDir = home
		}
		// Persist project dir and launch mode so resume uses the right settings
		p.setTodoProjectDir(todo.ID, projectDir)
		p.setTodoLaunchMode(todo.ID, p.taskRunnerMode)
		qs := queuedSession{
			TodoID:     todo.ID,
			Prompt:     prompt,
			ProjectDir: projectDir,
			Mode:       p.taskRunnerMode,
			Perm:       p.taskRunnerPerm,
			Budget:     p.taskRunnerBudget,
			AutoStart:  immediate,
		}
		cmd := tea.Batch(p.persistProjectDir(todo.ID, projectDir), p.persistLaunchMode(todo.ID, p.taskRunnerMode), p.launchOrQueueAgent(qs))
		p.taskRunnerView = false
		p.detailView = false
		if p.canLaunchAgent() || len(p.sessionQueue) == 0 {
			p.flashMessage = fmt.Sprintf("Agent launched for: %s", truncateToWidth(flattenTitle(todo.Title), 40))
		} else {
			p.flashMessage = fmt.Sprintf("Agent queued for: %s", truncateToWidth(flattenTitle(todo.Title), 40))
		}
		p.flashMessageAt = time.Now()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	p.taskRunnerView = false
	p.detailView = false
	return plugin.NoopAction()
}


// indexOf returns the index of s in the slice, or 0 if not found.
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if strings.EqualFold(v, s) {
			return i
		}
	}
	return 0
}

// parseDueDate attempts to parse common date input formats.
// Returns (YYYY-MM-DD, true) if recognized, or ("", false) if LLM fallback is needed.
func parseDueDate(input string, now time.Time) (string, bool) {
	input = strings.TrimSpace(input)

	// Already YYYY-MM-DD
	if _, err := time.Parse("2006-01-02", input); err == nil {
		return input, true
	}

	// Try "mm dd" or "m dd" or "mm d" or "m d" (space-separated)
	parts := strings.Fields(input)
	if len(parts) == 2 {
		var month, day int
		if _, err := fmt.Sscanf(parts[0], "%d", &month); err == nil {
			if _, err := fmt.Sscanf(parts[1], "%d", &day); err == nil {
				if month >= 1 && month <= 12 && day >= 1 && day <= 31 {
					year := now.Year()
					candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
					// Use next year if the date has already passed
					if candidate.Before(now.Truncate(24 * time.Hour)) {
						year++
					}
					return fmt.Sprintf("%04d-%02d-%02d", year, month, day), true
				}
			}
		}
	}

	return "", false
}
