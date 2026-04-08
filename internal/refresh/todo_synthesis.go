package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// SynthesisResult is the LLM output for a combined todo.
type SynthesisResult struct {
	Title      string `json:"title"`
	Due        string `json:"due"`
	WhoWaiting string `json:"who_waiting"`
	Detail     string `json:"detail"`
	Context    string `json:"context"`
	Effort     string `json:"effort"`
}

// Synthesize calls the LLM to combine multiple originals into one todo.
// The newest original (last in slice) is source of truth on overlap.
func Synthesize(ctx context.Context, l llm.LLM, originals []db.Todo) (*SynthesisResult, error) {
	prompt := buildSynthesisPrompt(originals)
	text, err := l.Complete(llm.WithOperation(ctx, "todo-synthesis"), prompt)
	if err != nil {
		return nil, fmt.Errorf("synthesis LLM call: %w", err)
	}
	return parseSynthesisResult(CleanJSON(text))
}

func buildSynthesisPrompt(originals []db.Todo) string {
	var b strings.Builder
	b.WriteString(`Combine these related todos into one. The newest entry is the source of truth where information overlaps. Fill gaps from older entries.

Originals (oldest first):
`)
	for i, t := range originals {
		fmt.Fprintf(&b, "%d. [#%d] %q (source: %s", i+1, t.DisplayID, t.Title, t.Source)
		if t.Due != "" {
			fmt.Fprintf(&b, ", due: %s", t.Due)
		}
		if t.WhoWaiting != "" {
			fmt.Fprintf(&b, ", who_waiting: %s", t.WhoWaiting)
		}
		if t.Effort != "" {
			fmt.Fprintf(&b, ", effort: %s", t.Effort)
		}
		if t.Detail != "" {
			fmt.Fprintf(&b, ", detail: %s", t.Detail)
		}
		b.WriteString(")\n")
	}
	b.WriteString(`
Return a single combined todo as JSON with these fields:
- title: concise action item
- due: YYYY-MM-DD or empty string
- who_waiting: person name(s) or empty string
- detail: comprehensive background combining all sources
- context: short categorization
- effort: estimated effort or empty string

Output ONLY the JSON object, no markdown code fences, no explanation.`)
	return b.String()
}

func parseSynthesisResult(raw string) (*SynthesisResult, error) {
	var result SynthesisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		n := len(raw)
		if n > 200 {
			n = 200
		}
		return nil, fmt.Errorf("parsing synthesis result: %w (raw: %s)", err, raw[:n])
	}
	return &result, nil
}

// BuildSynthesisTodo creates a new Todo from a synthesis result.
// Status comes from mergeTarget (preserves triage decisions).
// Non-LLM fields come from the newest original.
func BuildSynthesisTodo(result *SynthesisResult, originals []db.Todo, mergeTarget *db.Todo) db.Todo {
	newest := originals[len(originals)-1]

	status := mergeTarget.Status

	return db.Todo{
		ID:              db.GenID(),
		DisplayID:       0, // DB auto-assigns via MAX(display_id)+1 on insert
		Title:           result.Title,
		Status:          status,
		Source:          "merge",
		Detail:          result.Detail,
		Context:         result.Context,
		WhoWaiting:      result.WhoWaiting,
		Due:             result.Due,
		Effort:          result.Effort,
		ProjectDir:      newest.ProjectDir,
		ProposedPrompt:  newest.ProposedPrompt,
		SourceContext:   newest.SourceContext,
		SourceContextAt: newest.SourceContextAt,
	}
}
