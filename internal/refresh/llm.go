package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

func generateSuggestions(ctx context.Context, l llm.LLM, cc *db.CommandCenter) (*db.Suggestions, error) {
	state, _ := json.Marshal(struct {
		Calendar db.CalendarData `json:"calendar"`
		Todos    []db.Todo       `json:"todos"`
		Threads  []db.Thread     `json:"threads"`
	}{
		Calendar: cc.Calendar,
		Todos:    activeTodos(cc.Todos),
		Threads:  activeThreads(cc.Threads),
	})

	prompt := fmt.Sprintf(`Given this current state of my calendar, todos, and active threads, provide:

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
	return strings.TrimSpace(s)
}

func activeTodos(todos []db.Todo) []db.Todo {
	var out []db.Todo
	for _, t := range todos {
		if t.Status == "active" {
			out = append(out, t)
		}
	}
	return out
}

func activeThreads(threads []db.Thread) []db.Thread {
	var out []db.Thread
	for _, t := range threads {
		if t.Status == "active" {
			out = append(out, t)
		}
	}
	return out
}
