package commandcenter

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"database/sql"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// parseUserError extracts a short, user-facing message from an LLM error.
func parseUserError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Check if it looks like claude stderr output
	if strings.Contains(msg, "claude exited") {
		// Extract the stderr portion after the exit code prefix
		if idx := strings.Index(msg, ": "); idx >= 0 {
			return llm.ParseClaudeError(msg[idx+2:])
		}
	}
	if len(msg) > 80 {
		return msg[:77] + "..."
	}
	return msg
}

// HandleMessage handles non-key messages and returns whether it was handled.
func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Forward mouse events to the active viewport for scroll support.
		if p.detailView && p.detailVPReady && !p.sessionViewerActive && !p.taskRunnerView {
			var cmd tea.Cmd
			p.detailVP, cmd = p.detailVP.Update(msg)
			if cmd != nil {
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
			}
			return true, plugin.NoopAction()
		}
		if p.sessionViewerActive {
			var cmd tea.Cmd
			p.sessionViewerVP, cmd = p.sessionViewerVP.Update(msg)
			if cmd != nil {
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
			}
			return true, plugin.NoopAction()
		}
		return false, plugin.NoopAction()

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

	case claudeSynthesizeFinishedMsg:
		return p.handleSynthesizeFinished(msg)

	case claudeCommandFinishedMsg:
		return p.handleClaudeCommandFinished(msg)

	case claudeFocusFinishedMsg:
		return p.handleClaudeFocusFinished(msg)

	case claudeRefinePromptMsg:
		return p.handleClaudeRefinePromptFinished(msg)

	case claudeDateParseFinishedMsg:
		return p.handleClaudeDateParseFinished(msg)

	case claudeTrainFinishedMsg:
		return p.handleClaudeTrainFinished(msg)

	case plannotatorFinishedMsg:
		return p.handlePlannotatorFinished(msg)

	case plannotatorReviewMsg:
		return p.handlePlannotatorReviewFinished(msg)

	case claudeReviewAddressedMsg:
		return p.handleClaudeReviewAddressed(msg)

	case agentEventMsg:
		return p.handleAgentEvent(msg)

	case agentEventsDoneMsg:
		return p.handleAgentEventsDone(msg)

	case agentStartedInternalMsg:
		return p.handleAgentStartedInternal(msg)

	case agentStartedMsg:
		return p.handleAgentStarted(msg)

	case agentStatusMsg:
		return p.handleAgentStatus(msg)

	case agentSessionIDMsg:
		return p.handleAgentSessionID(msg)

	case agentFinishedMsg:
		return p.handleAgentFinished(msg)

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		// Update commandTextArea width so text wraps correctly after resize
		if p.detailMode == "commandInput" {
			p.commandTextArea.SetWidth(p.textareaWidth())
		}
		return false, plugin.NoopAction() // Let host also handle this

	case plugin.TabViewMsg:
		// Reload from DB if data is stale (>2s since last read).
		if p.database != nil && time.Since(p.ccLastRead) > ccStaleThreshold {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.loadCCFromDBCmd()}
		}
		return true, plugin.NoopAction()

	case plugin.LaunchMsg:
		return p.handleLaunchMsg(msg)

	case plugin.ReturnMsg:
		// Always reload from DB when returning from a Claude session.
		var cmds []tea.Cmd
		if p.database != nil {
			cmds = append(cmds, p.loadCCFromDBCmd())
		}

		// If we were viewing a specific todo (joined session), restore the detail view.
		if msg.TodoID != "" {
			p.detailView = true
			p.detailTodoID = msg.TodoID
			p.detailMode = "viewing"
			p.detailSelectedField = 0

			// Update session status when returning from a Claude session.
			// - Resume/join sessions → "review" (user reviewed existing work)
			// - Interactive sessions launched via "Run Claude" → "completed"
			//   (only if not tracked as a headless agent, which has its own completion path)
			if msg.WasResumeJoin {
				p.setTodoStatus(msg.TodoID, db.StatusReview)
				cmds = append(cmds, p.persistTodoStatus(msg.TodoID, db.StatusReview))
			} else if _, isHeadless := p.activeSessions[msg.TodoID]; !isHeadless {
				// Interactive session returned — detect completion.
				p.setTodoStatus(msg.TodoID, db.StatusReview)
				cmds = append(cmds, p.persistTodoStatus(msg.TodoID, db.StatusReview))
			}
		}

		if len(cmds) > 0 {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(cmds...)}
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
	if p.detailView && p.detailMode == "commandInput" {
		var cmd tea.Cmd
		p.commandTextArea, cmd = p.commandTextArea.Update(msg)
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

	var cmds []tea.Cmd

	// When an agent submits a session summary (via ccc update-todo), the DB
	// reload picks it up here. Terminate the agent process since it has
	// declared its work done, and transition the todo to "review" status.
	// We don't use killAgent here because that resets status to "backlog",
	// which is wrong — the agent completed successfully.
	if p.cc != nil {
		for _, todo := range p.cc.Todos {
			if todo.SessionSummary != "" {
				if sess, ok := p.activeSessions[todo.ID]; ok {
					if sess.Stdin != nil {
						sess.Stdin.Close()
					}
					if sess.Cmd != nil && sess.Cmd.Process != nil {
						sess.Cmd.Process.Kill()
					}
					delete(p.activeSessions, todo.ID)

					p.setTodoStatus(todo.ID, db.StatusReview)
					p.publishEvent("agent.completed", map[string]interface{}{
						"todo_id": todo.ID,
					})

					// If the session viewer is watching this session, mark it done.
					if p.sessionViewerActive && p.sessionViewerTodoID == todo.ID {
						p.sessionViewerDone = true
						p.sessionViewerListening = false
						p.updateSessionViewerContent()
					}

					cmds = append(cmds, p.persistTodoStatus(todo.ID, db.StatusReview))
				}
			}
		}
	}

	// Generate focus suggestion if empty (first load, or DB was cleared).
	if p.cc != nil && p.cc.Suggestions.Focus == "" && !p.claudeLoading {
		if cmd := p.triggerFocusRefresh(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) > 0 {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(cmds...)}
	}
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
	if msg.err != nil {
		p.flashMessage = "Edit failed: " + parseUserError(msg.err)
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	if msg.output != "" && p.cc != nil {
		jsonStr := extractJSON(msg.output)
		var updated db.Todo
		if err := json.Unmarshal([]byte(jsonStr), &updated); err != nil {
			p.flashMessage = "Edit returned invalid JSON"
			p.flashMessageAt = time.Now()
			return true, plugin.NoopAction()
		}
		todoID := msg.todoID
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == todoID {
				// Preserve system-managed fields that the LLM shouldn't overwrite.
				existing := p.cc.Todos[i]
				updated.ID = existing.ID
				if updated.CreatedAt.IsZero() {
					updated.CreatedAt = existing.CreatedAt
				}
				updated.CompletedAt = existing.CompletedAt
				updated.SessionID = existing.SessionID
				updated.SessionSummary = existing.SessionSummary
				updated.DisplayID = existing.DisplayID
				p.cc.Todos[i] = updated
				break
			}
		}
		p.publishEvent("todo.edited", map[string]interface{}{"id": todoID, "title": updated.Title})
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
			return db.DBUpdateTodo(database, todoID, updated)
		})}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleClaudeEnrichFinished(msg claudeEnrichFinishedMsg) (bool, plugin.Action) {
	p.claudeLoading = false
	if msg.err != nil {
		p.flashMessage = "Enrich failed: " + parseUserError(msg.err)
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	if msg.output != "" {
		jsonStr := extractJSON(msg.output)
		var enriched struct {
			Title          string `json:"title"`
			Due            string `json:"due"`
			WhoWaiting     string `json:"who_waiting"`
			Effort         string `json:"effort"`
			Context        string `json:"context"`
			Detail         string `json:"detail"`
			ProjectDir     string `json:"project_dir"`
			ProposedPrompt string `json:"proposed_prompt"`
			MergeInto      string `json:"merge_into"`
			MergeNote      string `json:"merge_note"`
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
			todo.ProposedPrompt = enriched.ProposedPrompt
			todoCopy := *todo
			p.publishEvent("todo.created", map[string]interface{}{"id": todoCopy.ID, "title": todoCopy.Title, "source": "enrich"})

			// Merge handling: if the LLM detected a duplicate, trigger synthesis
			if enriched.MergeInto != "" {
				target := p.cc.FindTodo(enriched.MergeInto)
				if target != nil && enriched.MergeInto != todoCopy.ID {
					// Gather originals
					var originals []db.Todo
					if target.Source == "merge" {
						origIDs := db.DBGetOriginalIDs(p.cc.Merges, target.ID)
						for _, oid := range origIDs {
							if orig := p.cc.FindTodo(oid); orig != nil {
								originals = append(originals, *orig)
							}
						}
					} else {
						originals = []db.Todo{*target}
					}
					originals = append(originals, todoCopy) // newest last

					// Insert the original todo first, then trigger synthesis
					return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(
						p.dbWriteCmd(func(database *sql.DB) error {
							return db.DBInsertTodo(database, todoCopy)
						}),
						claudeSynthesizeCmd(p.llm, originals, target),
					)}
				}
			}

			p.flashMessage = fmt.Sprintf("Added todo #%d", todoCopy.DisplayID)
			p.flashMessageAt = time.Now()
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
				return db.DBInsertTodo(database, todoCopy)
			})}
		}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleSynthesizeFinished(msg claudeSynthesizeFinishedMsg) (bool, plugin.Action) {
	if msg.err != nil {
		p.flashMessage = "Merge failed: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	synth := msg.synthesis
	synthCopy := synth
	p.cc.Todos = append(p.cc.Todos, synth)
	// Add merge records to in-memory state
	for _, orig := range msg.originals {
		p.cc.Merges = append(p.cc.Merges, db.TodoMerge{
			SynthesisID: synthCopy.ID,
			OriginalID:  orig.ID,
		})
	}
	p.flashMessage = fmt.Sprintf("Merged into #%d: %s", synthCopy.DisplayID, synthCopy.Title)
	p.flashMessageAt = time.Now()

	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
		if msg.oldSynthID != "" {
			_ = db.DBDeleteSynthesisMerges(database, msg.oldSynthID)
			_ = db.DBDeleteTodo(database, msg.oldSynthID)
			// Also remove old synthesis from in-memory list
			for i, t := range p.cc.Todos {
				if t.ID == msg.oldSynthID {
					p.cc.Todos = append(p.cc.Todos[:i], p.cc.Todos[i+1:]...)
					break
				}
			}
		}
		if err := db.DBInsertTodo(database, synthCopy); err != nil {
			return err
		}
		for _, orig := range msg.originals {
			if err := db.DBInsertMerge(database, synthCopy.ID, orig.ID, ""); err != nil {
				return err
			}
		}
		return nil
	})}
}

func (p *Plugin) handleClaudeTrainFinished(msg claudeTrainFinishedMsg) (bool, plugin.Action) {
	p.claudeLoading = false
	if msg.err != nil {
		p.flashMessage = "Training failed: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	jsonStr := extractJSON(msg.output)
	var result struct {
		ProjectDir        string `json:"project_dir"`
		UseForRules       []struct {
			Path string `json:"path"`
			Rule string `json:"rule"`
		} `json:"use_for_rules"`
		NotForRules []struct {
			Path string `json:"path"`
			Rule string `json:"rule"`
		} `json:"not_for_rules"`
		PromptHint        string `json:"prompt_hint"`
		PromptHintProject string `json:"prompt_hint_project"`
		RegeneratedPrompt string `json:"regenerated_prompt"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		p.flashMessage = "Training returned invalid result"
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// Apply routing rule changes
	var changes []string
	for _, r := range result.UseForRules {
		if r.Path != "" && r.Rule != "" {
			if err := db.AddRoutingRule(r.Path, "use_for", r.Rule); err == nil {
				changes = append(changes, fmt.Sprintf("+use_for on %s", lastPathSegment(r.Path)))
			}
		}
	}
	for _, r := range result.NotForRules {
		if r.Path != "" && r.Rule != "" {
			if err := db.AddRoutingRule(r.Path, "not_for", r.Rule); err == nil {
				changes = append(changes, fmt.Sprintf("+not_for on %s", lastPathSegment(r.Path)))
			}
		}
	}

	// Apply prompt hint
	hintProject := result.PromptHintProject
	if hintProject == "" {
		hintProject = result.ProjectDir
	}
	if result.PromptHint != "" && hintProject != "" {
		if err := db.SetPromptHint(hintProject, result.PromptHint); err == nil {
			changes = append(changes, fmt.Sprintf("hint on %s", lastPathSegment(hintProject)))
		}
	}

	// Update todo's project and prompt
	var dbCmds []tea.Cmd
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == msg.todoID {
				if result.ProjectDir != "" {
					p.cc.Todos[i].ProjectDir = result.ProjectDir
				}
				if result.RegeneratedPrompt != "" {
					p.cc.Todos[i].ProposedPrompt = result.RegeneratedPrompt
				}
				updated := p.cc.Todos[i]
				dbCmds = append(dbCmds, p.dbWriteCmd(func(database *sql.DB) error {
					return db.DBUpdateTodo(database, updated.ID, updated)
				}))
				break
			}
		}
	}

	if len(changes) > 0 {
		p.flashMessage = fmt.Sprintf("Trained: %s", strings.Join(changes, ", "))
	} else {
		p.flashMessage = "Trained: prompt updated"
	}
	p.flashMessageAt = time.Now()

	if len(dbCmds) > 0 {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(dbCmds...)}
	}
	return true, plugin.NoopAction()
}

// lastPathSegment returns the last non-empty segment of a file path for display.
func lastPathSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
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
	p.taskRunnerPromptText = refined
	p.taskRunnerPrompt.SetContent(wrapText(refined, p.taskRunnerPrompt.Width))
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
	if msg.err != nil {
		p.flashMessage = "Focus failed: " + parseUserError(msg.err)
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}
	if msg.output != "" {
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
				p.taskRunnerPromptText = newPrompt
				p.taskRunnerPrompt.SetContent(wrapText(newPrompt, p.taskRunnerPrompt.Width))
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
	// If user cancelled via esc, ignore the result.
	if !p.taskRunnerReviewing {
		return true, plugin.NoopAction()
	}
	p.taskRunnerReviewing = false

	if msg.err != nil {
		p.flashMessage = "Plannotator exited with error: " + msg.err.Error()
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	// User approved the prompt.
	if msg.approved {
		p.flashMessage = "Prompt approved"
		p.flashMessageAt = time.Now()
		p.taskRunnerReviewClean = ""
		return true, plugin.NoopAction()
	}

	// User denied with feedback — send to LLM to address annotations.
	feedback := strings.TrimSpace(msg.feedback)
	if feedback == "" {
		p.flashMessage = "Review cancelled"
		p.flashMessageAt = time.Now()
		return true, plugin.NoopAction()
	}

	p.taskRunnerRefining = true
	cmd := claudeReviewAddressCmd(p.llm, msg.todoID, p.taskRunnerReviewClean, feedback, msg.round)
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
	p.taskRunnerPromptText = refined
	p.taskRunnerPrompt.SetContent(wrapText(refined, p.taskRunnerPrompt.Width))
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
	p.taskRunnerReviewing = true
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
	p.setTodoStatus(msg.todoID, db.StatusRunning)

	var cmds []tea.Cmd
	cmds = append(cmds, p.persistTodoStatus(msg.todoID, db.StatusRunning))

	// Persist the session log path so it can be replayed later.
	if msg.session.LogPath != "" {
		p.setTodoSessionLogPath(msg.todoID, msg.session.LogPath)
		cmds = append(cmds, p.persistSessionLogPath(msg.todoID, msg.session.LogPath))
	}

	// If session viewer is open for this todo, start listening for events.
	if p.sessionViewerActive && p.sessionViewerTodoID == msg.todoID && !p.sessionViewerListening {
		p.sessionViewerListening = true
		cmds = append(cmds, listenForAgentEvent(msg.todoID, msg.session.EventsCh))
	}

	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(cmds...)}
}

func (p *Plugin) handleAgentStarted(msg agentStartedMsg) (bool, plugin.Action) {
	p.setTodoStatus(msg.todoID, db.StatusRunning)
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.persistTodoStatus(msg.todoID, db.StatusRunning)}
}

func (p *Plugin) handleAgentStatus(msg agentStatusMsg) (bool, plugin.Action) {
	if sess, ok := p.activeSessions[msg.todoID]; ok {
		sess.Status = msg.status
		sess.Question = msg.question
	}
	p.setTodoStatus(msg.todoID, msg.status)
	if msg.status == "blocked" {
		p.publishEvent("agent.blocked", map[string]interface{}{
			"todo_id":  msg.todoID,
			"question": msg.question,
		})
	}
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.persistTodoStatus(msg.todoID, msg.status)}
}

func (p *Plugin) handleLaunchMsg(msg plugin.LaunchMsg) (bool, plugin.Action) {
	// When joining a session (--resume), gracefully stop the headless agent first
	// so Claude CLI can save state and the interactive resume finds the session.
	if msg.ResumeID != "" {
		for todoID, sess := range p.activeSessions {
			sess.mu.Lock()
			sid := sess.SessionID
			sess.mu.Unlock()
			if sid == msg.ResumeID {
				// Send SIGINT for graceful shutdown, then wait for exit.
				if sess.Stdin != nil {
					sess.Stdin.Close()
				}
				if sess.Cmd != nil && sess.Cmd.Process != nil {
					sess.Cmd.Process.Signal(syscall.SIGINT)
				}
				// Wait for the agent to exit (up to 5s).
				if sess.done != nil {
					select {
					case <-sess.done:
					case <-time.After(5 * time.Second):
					}
				}
				delete(p.activeSessions, todoID)
				break
			}
		}
	}
	return false, plugin.NoopAction() // Let host continue with the launch
}

func (p *Plugin) handleAgentSessionID(msg agentSessionIDMsg) (bool, plugin.Action) {
	// Update in-memory todo with the captured session ID.
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == msg.todoID {
				p.cc.Todos[i].SessionID = msg.sessionID
				break
			}
		}
	}
	// Persist to DB.
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBUpdateTodoSessionID(database, msg.todoID, msg.sessionID)
	})}
}

func (p *Plugin) handleAgentFinished(msg agentFinishedMsg) (bool, plugin.Action) {
	cmd := p.onAgentFinished(msg.todoID, msg.exitCode)
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
}

func (p *Plugin) handleAgentEvent(msg agentEventMsg) (bool, plugin.Action) {
	// Update session viewer if it's active and watching this todo
	if p.sessionViewerActive && p.sessionViewerTodoID == msg.todoID {
		p.updateSessionViewerContent()
	}

	// Continue listening for more events
	if sess, ok := p.activeSessions[msg.todoID]; ok {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: listenForAgentEvent(msg.todoID, sess.EventsCh)}
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleAgentEventsDone(msg agentEventsDoneMsg) (bool, plugin.Action) {
	p.sessionViewerListening = false
	// Mark the viewer as done if it's active for this todo
	if p.sessionViewerActive && p.sessionViewerTodoID == msg.todoID {
		p.sessionViewerDone = true
		p.updateSessionViewerContent()
	}
	return true, plugin.NoopAction()
}

func (p *Plugin) handleTickMsg() (bool, plugin.Action) {
	p.frame++
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 15*time.Second {
		p.flashMessage = ""
	}

	// Auto-advance detail view after notice expires (1 second)
	if p.detailNotice != "" && time.Since(p.detailNoticeAt) > 1*time.Second {
		p.detailNotice = ""
		filtered := p.filteredTodos()
		if len(filtered) == 0 {
			// No more todos — exit detail view
			p.detailView = false
			p.detailMode = "viewing"
		} else {
			// After completing/dismissing, advance to next filtered todo
			idx := p.detailTodoActiveIndex()
			if idx < 0 {
				// Current todo no longer active (was completed/dismissed); pick next one
				// Use ccCursor as fallback position (clamped to filtered list)
				if p.ccCursor >= len(filtered) {
					p.ccCursor = len(filtered) - 1
				}
				p.detailTodoID = filtered[p.ccCursor].ID
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
