package commandcenter

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"database/sql"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// HandleMessage handles non-key messages and returns whether it was handled.
func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case ccLoadedMsg:
		return p.handleCCLoaded(msg)

	case ccRefreshFinishedMsg:
		return p.handleRefreshFinished(msg)

	case dbWriteResult:
		return p.handleDBWriteResult(msg)

	case claudeEditFinishedMsg:
		return p.handleClaudeEditFinished(msg)

	case claudeEnrichFinishedMsg:
		return p.handleClaudeEnrichFinished(msg)

	case claudeCommandFinishedMsg:
		return p.handleClaudeCommandFinished(msg)

	case claudeFocusFinishedMsg:
		return p.handleClaudeFocusFinished(msg)

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return false, plugin.NoopAction() // Let host also handle this

	case plugin.TabViewMsg:
		// Reload from DB if data is stale (>2s since last read).
		if p.database != nil && time.Since(p.ccLastRead) > ccStaleThreshold {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.loadCCFromDBCmd()}
		}
		return true, plugin.NoopAction()

	case plugin.ReturnMsg:
		// Always reload from DB when returning from a Claude session.
		if p.database != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.loadCCFromDBCmd()}
		}
		return true, plugin.NoopAction()

	// Handle external notifications by reloading from DB
	default:
		if _, ok := msg.(plugin.NotifyMsg); ok && p.database != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.loadCCFromDBCmd()}
		}

	case ui.TickMsg:
		return p.handleTickMsg()

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		if cmd != nil {
			return false, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return false, plugin.NoopAction()
	}

	// Pass through to textarea / textinput if active
	if p.addingTodoQuick {
		var cmd tea.Cmd
		p.quickTodoTextArea, cmd = p.quickTodoTextArea.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}
	if p.addingTodoRich {
		var cmd tea.Cmd
		p.todoTextArea, cmd = p.todoTextArea.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}
	if p.detailView {
		var cmd tea.Cmd
		p.textInput, cmd = p.textInput.Update(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}

	return false, plugin.NoopAction()
}

func (p *Plugin) handleCCLoaded(msg ccLoadedMsg) (bool, plugin.Action) {
	if msg.cc != nil {
		p.cc = msg.cc
	}
	p.ccLastRead = time.Now()
	return true, plugin.NoopAction()
}

func (p *Plugin) handleRefreshFinished(msg ccRefreshFinishedMsg) (bool, plugin.Action) {
	p.ccRefreshing = false
	p.lastRefreshAt = time.Now()
	if msg.err != nil {
		p.lastRefreshError = msg.err.Error()
		p.flashMessage = "Refresh error: " + msg.err.Error()
		p.flashMessageAt = time.Now()
	} else {
		p.lastRefreshError = ""
		p.publishEvent("data.refreshed", map[string]interface{}{"source": "ccc-refresh"})
	}
	if p.database != nil {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.loadCCFromDBCmd()}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleDBWriteResult(msg dbWriteResult) (bool, plugin.Action) {
	if msg.err != nil {
		fmt.Fprintf(os.Stderr, "DB write error: %v\n", msg.err)
		if p.database != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.loadCCFromDBCmd()}
		}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleClaudeEditFinished(msg claudeEditFinishedMsg) (bool, plugin.Action) {
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
			p.publishEvent("todo.edited", map[string]interface{}{"id": todoID, "title": updated.Title})
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
				return db.DBUpdateTodo(database, todoID, updated)
			})}
		}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleClaudeEnrichFinished(msg claudeEnrichFinishedMsg) (bool, plugin.Action) {
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
			p.publishEvent("todo.created", map[string]interface{}{"id": todoCopy.ID, "title": todoCopy.Title, "source": "enrich"})
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
				return db.DBInsertTodo(database, todoCopy)
			})}
		}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleClaudeCommandFinished(msg claudeCommandFinishedMsg) (bool, plugin.Action) {
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
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.todoTextArea.Focus()}
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
					p.publishEvent("todo.created", map[string]interface{}{"id": t.ID, "title": t.Title, "source": "command"})
					dbCmds = append(dbCmds, p.dbWriteCmd(func(database *sql.DB) error { return db.DBInsertTodo(database, t) }))
				}
				for _, id := range resp.CompleteTodoIDs {
					p.cc.CompleteTodo(id)
					p.publishEvent("todo.completed", map[string]interface{}{"id": id, "title": ""})
					cid := id
					dbCmds = append(dbCmds, p.dbWriteCmd(func(database *sql.DB) error { return db.DBCompleteTodo(database, cid) }))
				}
				if focusCmd := p.triggerFocusRefresh(); focusCmd != nil {
					dbCmds = append(dbCmds, focusCmd)
				}
				p.commandConversation = nil
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmds...)}
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
}

func (p *Plugin) handleClaudeFocusFinished(msg claudeFocusFinishedMsg) (bool, plugin.Action) {
	p.claudeLoading = false
	if msg.err == nil && msg.output != "" {
		focus := strings.TrimSpace(msg.output)
		focus = strings.Trim(focus, "\"")
		if focus != "" && p.cc != nil {
			p.cc.Suggestions.Focus = focus
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error { return db.DBSaveFocus(database, focus) })}
		}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleTickMsg() (bool, plugin.Action) {
	p.frame++
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 15*time.Second {
		p.flashMessage = ""
	}
	var cmds []tea.Cmd
	// Trigger ccc-refresh when data is older than the refresh interval (default 5m).
	if p.cc != nil && !p.ccRefreshing && time.Since(p.cc.GeneratedAt) > ccRefreshInterval {
		if time.Since(p.ccLastRefreshTriggered) > ccRefreshInterval {
			p.ccRefreshing = true
			p.ccLastRefreshTriggered = time.Now()
			cmds = append(cmds, refreshCCCmd())
		}
	}
	if len(cmds) > 0 {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(cmds...)}
	}
	return false, plugin.NoopAction() // Tick is shared, don't claim exclusive ownership
}
