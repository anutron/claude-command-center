package refresh

import (
	"context"
	"encoding/json"
	"fmt"
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
}

// GenerateTodoPrompt takes a todo and project context, returns a project_dir
// assignment and a proposed prompt. The LLM chooses which project best matches
// the task based on descriptions, skills, and routing rules.
func GenerateTodoPrompt(ctx context.Context, l llm.LLM, todo db.Todo, paths PathContext) (*TodoPromptResult, error) {
	prompt := buildRoutingPrompt(todo, paths)

	text, err := l.Complete(ctx, prompt)
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
func buildRoutingPrompt(todo db.Todo, paths PathContext) string {
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

	b.WriteString(`
## Instructions
1. Choose the best project directory for this task. Explain your reasoning briefly.
2. Generate an actionable prompt for a Claude Code agent working in that directory.
   The prompt should:
   - State the objective clearly in imperative mood
   - Include relevant context from the todo detail
   - Mention who is waiting (for attribution)
   - Suggest what "done" looks like

Return ONLY JSON:
{"project_dir": "/path/to/project", "proposed_prompt": "## Objective\n...", "reasoning": "One sentence explaining why this project"}
`)

	return b.String()
}
