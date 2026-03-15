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

	case claudeRefinePromptMsg:
		return p.handleClaudeRefinePromptFinished(msg)

	case claudeDateParseFinishedMsg:
		return p.handleClaudeDateParseFinished(msg)

	case plannotatorFinishedMsg:
		return p.handlePlannotatorFinished(msg)

	case plannotatorReviewMsg:
		return p.handlePlannotatorReviewFinished(msg)

	case claudeReviewAddressedMsg:
		return p.handleClaudeReviewAddressed(msg)

	case agentStartedInternalMsg:
		return p.handleAgentStartedInternal(msg)

	case agentStartedMsg:
		return p.handleAgentStarted(msg)

	case agentStatusMsg:
		return p.handleAgentStatus(msg)

	case agentFinishedMsg:
		return p.handleAgentFinished(msg)

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

func (p *Plugin) handleClaudeRefinePromptFinished(msg claudeRefinePromptMsg) (bool, plugin.Action) {
	p.taskRunnerRefining = false
	if msg.err != nil {
		p.flashMessage = "Refine failed: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	refined := strings.TrimSpace(msg.output)
	if refined == "" {
		p.flashMessage = "Refine returned empty result"
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	// Update viewport
	p.taskRunnerPrompt.SetContent(refined)
	p.taskRunnerPrompt.GotoTop()
	// Update in-memory todo
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == msg.todoID {
				p.cc.Todos[i].ProposedPrompt = refined
				updated := p.cc.Todos[i]
				p.flashMessage = "Prompt refined"
				p.flashMessageAt = time.Now()
				dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBUpdateTodo(database, updated.ID, updated)
				})
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
			}
		}
	}
	p.flashMessage = "Prompt refined"
	p.flashMessageAt = time.Now()
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

func (p *Plugin) handleClaudeDateParseFinished(msg claudeDateParseFinishedMsg) (bool, plugin.Action) {
	p.claudeLoading = false
	if msg.err != nil {
		p.flashMessage = "Date parse failed: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	parsed := strings.TrimSpace(msg.output)
	// Validate the LLM returned a proper YYYY-MM-DD date
	if _, err := time.Parse("2006-01-02", parsed); err != nil {
		p.flashMessage = fmt.Sprintf("Could not parse date: %q", parsed)
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	// Find the todo and update its due date
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == msg.todoID {
				p.cc.Todos[i].Due = parsed
				updated := p.cc.Todos[i]
				p.flashMessage = fmt.Sprintf("Due date set to %s", parsed)
				p.flashMessageAt = time.Now()
				dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBUpdateTodo(database, updated.ID, updated)
				})
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
			}
		}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handlePlannotatorFinished(msg plannotatorFinishedMsg) (bool, plugin.Action) {
	if msg.err != nil {
		p.flashMessage = "Editor exited with error: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// Read the prompt back from the temp file.
	newPrompt := readTempPrompt(msg.tempFile)

	// Clean up the temp file.
	if msg.tempFile != "" {
		os.Remove(msg.tempFile)
	}

	// If the user left the file empty, don't overwrite.
	if newPrompt == "" {
		p.flashMessage = "Prompt unchanged (empty file)"
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// Update the todo's ProposedPrompt in-memory and persist.
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == msg.todoID {
				p.cc.Todos[i].ProposedPrompt = newPrompt
				updated := p.cc.Todos[i]

				// Refresh the task runner prompt viewport.
				p.taskRunnerPrompt.SetContent(newPrompt)
				p.taskRunnerPrompt.GotoTop()

				p.flashMessage = "Prompt updated"
				p.flashMessageAt = time.Now()

				dbCmd := p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBUpdateTodo(database, updated.ID, updated)
				})
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: dbCmd}
			}
		}
	}

	return true, plugin.NoopAction()
}

func (p *Plugin) handlePlannotatorReviewFinished(msg plannotatorReviewMsg) (bool, plugin.Action) {
	if msg.err != nil {
		p.flashMessage = "Editor exited with error: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// Read the annotated prompt from the temp file.
	annotated := readTempPrompt(msg.tempFile)

	// Clean up the temp file.
	if msg.tempFile != "" {
		os.Remove(msg.tempFile)
	}

	if annotated == "" {
		p.flashMessage = "Review cancelled (empty file)"
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// If unchanged from the clean version, the user approves.
	if annotated == p.taskRunnerReviewClean {
		p.flashMessage = "Prompt approved"
		p.flashMessageAt = time.Now()
		p.taskRunnerReviewClean = ""
		return true, plugin.NoopAction()
	}

	// Changed — send to LLM to address annotations.
	p.taskRunnerRefining = true
	cmd := claudeReviewAddressCmd(p.llm, msg.todoID, p.taskRunnerReviewClean, annotated, msg.round)
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleClaudeReviewAddressed(msg claudeReviewAddressedMsg) (bool, plugin.Action) {
	p.taskRunnerRefining = false
	if msg.err != nil {
		p.flashMessage = "Review refine failed: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	refined := strings.TrimSpace(msg.output)
	if refined == "" {
		p.flashMessage = "Review refine returned empty result"
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// Update viewport and in-memory ProposedPrompt, persist to DB.
	p.taskRunnerPrompt.SetContent(refined)
	p.taskRunnerPrompt.GotoTop()

	var dbCmd tea.Cmd
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == msg.todoID {
				p.cc.Todos[i].ProposedPrompt = refined
				updated := p.cc.Todos[i]
				dbCmd = p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBUpdateTodo(database, updated.ID, updated)
				})
				break
			}
		}
	}

	// Store as new clean baseline and reopen Plannotator for next round.
	p.taskRunnerReviewClean = refined
	reviewCmd := launchPlannotatorReview(msg.todoID, refined, msg.round+1)

	if dbCmd != nil {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmd, reviewCmd)}
	}
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: reviewCmd}
}

func (p *Plugin) handleAgentStartedInternal(msg agentStartedInternalMsg) (bool, plugin.Action) {
	// Store the session (already initialized with process handles and done channel).
	p.activeSessions[msg.todoID] = msg.session

	// Update the todo session status in-memory and persist.
	p.setTodoSessionStatus(msg.todoID, "active")
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.persistSessionStatus(msg.todoID, "active")}
}

func (p *Plugin) handleAgentStarted(msg agentStartedMsg) (bool, plugin.Action) {
	p.setTodoSessionStatus(msg.todoID, "active")
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.persistSessionStatus(msg.todoID, "active")}
}

func (p *Plugin) handleAgentStatus(msg agentStatusMsg) (bool, plugin.Action) {
	if sess, ok := p.activeSessions[msg.todoID]; ok {
		sess.Status = msg.status
		sess.Question = msg.question
	}
	p.setTodoSessionStatus(msg.todoID, msg.status)
	if msg.status == "blocked" {
		p.publishEvent("agent.blocked", map[string]interface{}{
			"todo_id":  msg.todoID,
			"question": msg.question,
		})
	}
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.persistSessionStatus(msg.todoID, msg.status)}
}

func (p *Plugin) handleAgentFinished(msg agentFinishedMsg) (bool, plugin.Action) {
	cmd := p.onAgentFinished(msg.todoID, msg.exitCode)
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleTickMsg() (bool, plugin.Action) {
	p.frame++
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 15*time.Second {
		p.flashMessage = ""
	}

	// Auto-advance detail view after notice expires (1 second)
	if p.detailNotice != "" && time.Since(p.detailNoticeAt) > 1*time.Second {
		p.detailNotice = ""
		activeTodos := p.cc.ActiveTodos()
		if len(activeTodos) == 0 {
			// No more todos — exit detail view
			p.detailView = false
			p.detailMode = "viewing"
		} else {
			// After completing/dismissing, advance to next active todo
			idx := p.detailTodoActiveIndex()
			if idx < 0 {
				// Current todo no longer active (was completed/dismissed); pick next one
				// Use ccCursor as fallback position
				if p.ccCursor >= len(activeTodos) {
					p.ccCursor = len(activeTodos) - 1
				}
				p.detailTodoID = activeTodos[p.ccCursor].ID
			}
			p.detailSelectedField = 0
		}
	}

	var cmds []tea.Cmd

	// Check for finished agent processes.
	if agentCmd := p.checkAgentProcesses(); agentCmd != nil {
		cmds = append(cmds, agentCmd)
	}

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
