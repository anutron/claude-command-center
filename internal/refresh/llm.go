package refresh

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

func generateSuggestions(ctx context.Context, l llm.LLM, cc *db.CommandCenter) (*db.Suggestions, error) {
	state, _ := json.Marshal(struct {
		Calendar db.CalendarData `json:"calendar"`
		Todos    []db.Todo       `json:"todos"`
	}{
		Calendar: cc.Calendar,
		Todos:    activeTodos(cc.Todos),
	})

	prompt := fmt.Sprintf(`Given this current state of my calendar and todos, provide:

1. A 1-2 sentence "focus" recommendation of what I should work on next and why. Consider: deadlines, who's waiting, available time gaps in my calendar, effort required.
2. A ranked list of todo IDs by suggested priority.
3. A one-line reason for each todo's ranking.

Return ONLY JSON with this exact structure, no other text:
{"focus": "...", "ranked_todo_ids": ["id1", "id2"], "reasons": {"id1": "reason", "id2": "reason"}}

Current state:
%s`, string(state))

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("suggestion generation: %w", err)
	}

	text = CleanJSON(text)

	var suggestions db.Suggestions
	if err := json.Unmarshal([]byte(text), &suggestions); err != nil {
		return nil, fmt.Errorf("parsing suggestions: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	return &suggestions, nil
}

// CleanJSON strips markdown code block wrappers from LLM JSON responses.
func CleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	// Extract the first JSON value if the LLM appended explanatory text.
	// Look for array [...] or object {...} boundaries.
	if i := strings.IndexAny(s, "[{"); i >= 0 {
		open, close := s[i], byte(']')
		if open == '{' {
			close = '}'
		}
		depth := 0
		for j := i; j < len(s); j++ {
			if s[j] == open {
				depth++
			} else if s[j] == close {
				depth--
				if depth == 0 {
					return s[i : j+1]
				}
			}
		}
	}
	return strings.TrimSpace(s)
}

// generateProposedPrompts fills in ProposedPrompt (and ProjectDir) for eligible
// todos (active, has a source, but no prompt yet). Uses project context — path
// descriptions, skills, and routing rules — to choose the best project and
// generate a tailored prompt for each todo.
func generateProposedPrompts(ctx context.Context, l llm.LLM, database *sql.DB, todos []db.Todo) []db.Todo {
	var eligible []int
	for i, t := range todos {
		if !db.IsTerminalStatus(t.Status) && t.Source != "" && t.Source != "manual" && t.ProposedPrompt == "" {
			eligible = append(eligible, i)
		}
	}
	if len(eligible) == 0 {
		return todos
	}

	// Load project context for routing decisions.
	pathCtx := loadPathContext(database)

	// If we have no paths, there's nothing to route to — fall back to prompt-only
	// generation without project assignment.
	if len(pathCtx.Paths) == 0 {
		return generateProposedPromptsLegacy(ctx, l, todos, eligible)
	}

	for _, i := range eligible {
		result, err := GenerateTodoPrompt(ctx, l, todos[i], pathCtx)
		if err != nil {
			log.Printf("todo prompt generation for %q: %v", todos[i].ID, err)
			continue
		}
		if result.ProjectDir == "REJECT" {
			log.Printf("todo %q rejected by routing: %s", todos[i].Title, result.Reasoning)
			todos[i].Status = "dismissed"
			todos[i].ProposedPrompt = "REJECTED: " + result.Reasoning
			continue
		}
		todos[i].ProposedPrompt = result.ProposedPrompt
		if result.ProjectDir != "" {
			todos[i].ProjectDir = result.ProjectDir
		}
	}

	return todos
}

// loadPathContext gathers path metadata, skills, and routing rules for the
// routing prompt. Errors are logged but not fatal — the agent can still work
// with partial context.
func loadPathContext(database *sql.DB) PathContext {
	var ctx PathContext

	if database == nil {
		return ctx
	}

	// Load paths with descriptions from DB.
	entries, err := db.DBLoadPathsFull(database)
	if err != nil {
		log.Printf("loading paths for routing: %v", err)
		return ctx
	}

	// Load routing rules (from config file, not DB).
	routingRules, err := db.LoadRoutingRules()
	if err != nil {
		log.Printf("loading routing rules: %v", err)
	}

	// Load global skills.
	globalSkills, err := db.GetGlobalSkills(false)
	if err != nil {
		log.Printf("loading global skills: %v", err)
	}
	ctx.GlobalSkills = globalSkills

	// Build path entries with skills and routing rules.
	for _, entry := range entries {
		pm := PathWithMeta{
			Path:        entry.Path,
			Description: entry.Description,
		}

		// Load project-specific skills (uses disk cache with 1hr TTL).
		skills, err := db.GetProjectSkills(entry.Path, false)
		if err != nil {
			log.Printf("loading skills for %s: %v", entry.Path, err)
		}
		pm.Skills = skills

		// Attach routing rules if present for this path.
		if rule, ok := routingRules[entry.Path]; ok {
			pm.RoutingRules = &rule
		}

		ctx.Paths = append(ctx.Paths, pm)
	}

	return ctx
}

// generateProposedPromptsLegacy is the original batch prompt approach, used as
// a fallback when no learned paths are available (so routing is not possible).
func generateProposedPromptsLegacy(ctx context.Context, l llm.LLM, todos []db.Todo, eligible []int) []db.Todo {
	type todoSummary struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Detail  string `json:"detail"`
		Context string `json:"context"`
		Source  string `json:"source"`
	}
	var summaries []todoSummary
	for _, i := range eligible {
		t := todos[i]
		summaries = append(summaries, todoSummary{
			ID:      t.ID,
			Title:   t.Title,
			Detail:  t.Detail,
			Context: t.Context,
			Source:  t.Source,
		})
	}

	input, _ := json.Marshal(summaries)

	prompt := fmt.Sprintf(`For each todo below, generate a self-contained Claude Code prompt that an agent could execute in a headless session. The prompt should:

1. State the objective clearly in imperative mood ("Add...", "Fix...", "Update...")
2. Include relevant context from the todo detail (key decisions, requirements, constraints)
3. Mention who is waiting or what meeting/thread originated the task (for attribution)
4. Suggest what "done" looks like (testable outcomes)

Return ONLY a JSON object mapping todo ID to the proposed prompt string. No other text.
Example: {"id1": "## Objective\nAdd rate limiting to the API...\n\n## Context\n...\n\n## Done Criteria\n- Tests pass\n- Rate limit returns 429 after threshold"}

Todos:
%s`, string(input))

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return todos
	}

	text = CleanJSON(text)

	var prompts map[string]string
	if err := json.Unmarshal([]byte(text), &prompts); err != nil {
		return todos
	}

	for _, i := range eligible {
		if p, ok := prompts[todos[i].ID]; ok && p != "" {
			todos[i].ProposedPrompt = p
		}
	}

	return todos
}

func activeTodos(todos []db.Todo) []db.Todo {
	var out []db.Todo
	for _, t := range todos {
		if !db.IsTerminalStatus(t.Status) {
			out = append(out, t)
		}
	}
	return out
}

