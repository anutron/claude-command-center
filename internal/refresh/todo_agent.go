package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// PathContext holds the project paths and global skills needed for routing.
type PathContext struct {
	Paths        []PathWithMeta `json:"paths"`
	GlobalSkills []db.SkillInfo `json:"global_skills"`
}

// PathWithMeta combines a learned path with its discovered skills and routing rules.
type PathWithMeta struct {
	Path         string         `json:"path"`
	Description  string         `json:"description"`
	Skills       []db.SkillInfo `json:"skills,omitempty"`
	RoutingRules *db.RoutingRule `json:"routing_rules,omitempty"`
}

// TodoPromptResult is the structured output from the LLM routing decision.
type TodoPromptResult struct {
	ProjectDir     string `json:"project_dir"`
	ProposedPrompt string `json:"proposed_prompt"`
	Reasoning      string `json:"reasoning"`
	MergeInto      string `json:"merge_into"`
	MergeNote      string `json:"merge_note"`
}

// GenerateTodoPrompt takes a todo and project context, returns a project_dir
// assignment and a proposed prompt. The LLM chooses which project best matches
// the task based on descriptions, skills, and routing rules.
func GenerateTodoPrompt(ctx context.Context, l llm.LLM, todo db.Todo, paths PathContext, activeTodos []db.Todo) (*TodoPromptResult, error) {
	prompt := buildRoutingPrompt(todo, paths, activeTodos)

	text, err := l.Complete(llm.WithOperation(ctx, "todo-routing"), prompt)
	if err != nil {
		return nil, fmt.Errorf("todo routing LLM call: %w", err)
	}

	text = CleanJSON(text)

	var result TodoPromptResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing todo routing result: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	return &result, nil
}

// buildRoutingPrompt constructs the full prompt for the LLM routing decision.
func buildRoutingPrompt(todo db.Todo, paths PathContext, activeTodos []db.Todo) string {
	var b strings.Builder

	b.WriteString(`You are choosing which project directory is most appropriate for a task,
and generating a prompt to execute it there.

## Task
`)
	fmt.Fprintf(&b, "Title: %s\n", todo.Title)
	if todo.Detail != "" {
		fmt.Fprintf(&b, "Detail: %s\n", todo.Detail)
	}
	if todo.Context != "" {
		fmt.Fprintf(&b, "Context: %s\n", todo.Context)
	}
	if todo.Source != "" {
		fmt.Fprintf(&b, "Source: %s\n", todo.Source)
	}
	if todo.WhoWaiting != "" {
		fmt.Fprintf(&b, "Who waiting: %s\n", todo.WhoWaiting)
	}
	if todo.Due != "" {
		fmt.Fprintf(&b, "Due: %s\n", todo.Due)
	}

	if todo.SourceContext != "" {
		b.WriteString("\n## Source Context\n")
		fmt.Fprintf(&b, "Source: %s (ref: %s)\n", todo.Source, todo.SourceRef)
		if todo.SourceContextAt != "" {
			fmt.Fprintf(&b, "Fetched: %s\n", todo.SourceContextAt)
		}
		b.WriteString("\n<source_context>\n")
		b.WriteString(todo.SourceContext)
		b.WriteString("\n</source_context>\n")
	}

	b.WriteString("\n## Available Projects\n")
	for _, p := range paths.Paths {
		fmt.Fprintf(&b, "\n### %s\n", p.Path)
		if p.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", p.Description)
		}
		if len(p.Skills) > 0 {
			b.WriteString("Project skills: ")
			var skillParts []string
			for _, s := range p.Skills {
				if s.Description != "" {
					skillParts = append(skillParts, fmt.Sprintf("%s (%s)", s.Name, s.Description))
				} else {
					skillParts = append(skillParts, s.Name)
				}
			}
			b.WriteString(strings.Join(skillParts, ", "))
			b.WriteString("\n")
		}
		if p.RoutingRules != nil {
			if len(p.RoutingRules.UseFor) > 0 {
				fmt.Fprintf(&b, "Routing preferences — use for: %s\n", strings.Join(p.RoutingRules.UseFor, "; "))
			}
			if len(p.RoutingRules.NotFor) > 0 {
				fmt.Fprintf(&b, "Routing preferences — not for: %s\n", strings.Join(p.RoutingRules.NotFor, "; "))
			}
			if p.RoutingRules.PromptHint != "" {
				fmt.Fprintf(&b, "Prompt generation hint: %s\n", p.RoutingRules.PromptHint)
			}
		}
	}

	if len(paths.GlobalSkills) > 0 {
		b.WriteString("\n## Global Skills (available in ALL projects)\n")
		for _, s := range paths.GlobalSkills {
			if s.Description != "" {
				fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
			} else {
				fmt.Fprintf(&b, "- %s\n", s.Name)
			}
		}
		b.WriteString("Note: Do not prefer a project just because it has skills that are also\navailable globally. Focus on whether the project's PURPOSE matches the task.\n")
	}

	if len(activeTodos) > 0 {
		b.WriteString("\n## Existing Todos\n")
		b.WriteString("If this task is semantically the same as an existing todo, return its ID in merge_into.\n\n")
		limit := len(activeTodos)
		if limit > 50 {
			limit = 50
		}
		for _, t := range activeTodos[:limit] {
			fmt.Fprintf(&b, "- [#%d] (id: %s) %s", t.DisplayID, t.ID, t.Title)
			if t.Due != "" {
				fmt.Fprintf(&b, " (due: %s)", t.Due)
			}
			b.WriteString("\n")
		}
	}

	if instructions := loadTodoInstructions(); instructions != "" {
		fmt.Fprintf(&b, "\n## User Instructions\n%s\n", instructions)
	}

	b.WriteString(`
## Instructions
1. First, verify this is actually Aaron's task. A task is Aaron's if ANY of these are true:
   a) Aaron stated he would do it or explicitly agreed to do it
   b) Someone else assigned the work to Aaron by name (e.g., "Aaron will...", "Bob and Aaron will follow-up on...",
      "Aaron is going to...", "Aaron to handle..."). These are commitments made ON BEHALF of Aaron.
   REJECT only if:
   - The commitment was made by someone else about THEMSELVES (not Aaron) — e.g., "I will" in [Other] blocks
   - Aaron is not mentioned or involved in the commitment
2. If this is Aaron's task, choose the best project directory and generate an actionable prompt
   for a Claude Code agent working in that directory. The prompt should:
   - State the objective clearly in imperative mood
   - Include relevant context from the todo detail
   - Mention who is waiting (for attribution)
   - Suggest what "done" looks like

Return ONLY JSON. If rejecting:
{"project_dir": "REJECT", "proposed_prompt": "", "reasoning": "This is [name]'s commitment, not Aaron's — [evidence]"}

If accepting:
{"project_dir": "/path/to/project", "proposed_prompt": "## Objective\n...", "reasoning": "One sentence explaining why this project"}

If merging with an existing todo, also include:
"merge_into": "existing-todo-id", "merge_note": "reason for merge"
`)

	return b.String()
}

// loadTodoInstructions reads todo_instructions.md from the project root.
// Returns empty string if the file doesn't exist.
func loadTodoInstructions() string {
	// Walk up from the executable or use a known path.
	// The refresh binary runs from the project root.
	candidates := []string{
		"todo_instructions.md",
		filepath.Join(os.Getenv("HOME"), ".config", "ccc", "todo_instructions.md"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}
