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
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// enterTaskRunner initializes the task runner view for a given todo.
func (p *Plugin) enterTaskRunner(todo db.Todo) {
	p.taskRunnerView = true
	p.taskRunnerStep = 1

	// Reload paths from DB so newly added sessions are available.
	if p.database != nil {
		if paths, err := db.DBLoadPaths(p.database); err == nil {
			p.detailPaths = paths
		}
	}

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
	vp.SetContent(wrapText(promptText, 40))
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

// taskRunnerLaunchInteractive launches an interactive Claude session with the
// todo's prompt as context. The user works on the todo themselves in Claude.
// Sets status to "running" so the todo shows as in-progress.
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

		// Mark todo as running and persist project dir + launch mode so resume uses the right settings
		p.setTodoStatus(todo.ID, db.StatusRunning)
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
		cmds = append(cmds, p.persistTodoStatus(todo.ID, db.StatusRunning))
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
		if p.canLaunchAgent() || p.agentRunner.QueueLen() == 0 {
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
