package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
)

func extractSlackCommitments(ctx context.Context, l llm.LLM, candidates []slackCandidate) ([]db.Todo, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("## Message %d (from #%s)\n", i+1, c.Channel))
		sb.WriteString(fmt.Sprintf("Permalink: %s\n", c.Permalink))
		sb.WriteString(fmt.Sprintf("Message: %s\n", c.Message))
		if c.ThreadContext != "" {
			sb.WriteString(fmt.Sprintf("Thread context:\n%s\n", c.ThreadContext))
		}
		sb.WriteString("\n---\n\n")
	}

	prompt := fmt.Sprintf(`You are filtering Slack messages for real commitments the user made. The bar is VERY high.

A message is ONLY a todo if:
1. The user explicitly committed to a specific deliverable (not just participating in conversation)
2. There is a concrete next action with a clear outcome
3. You can write an actionable title starting with a verb (Send, Review, Schedule, Build, Write, Follow up, etc.)

REJECT messages that are:
- Conversational responses ("done", "good process!", "sounds good")
- Observations, tips, shared links, compliments
- Descriptions of past actions ("I just...", "I found that...")
- Vague intentions without a specific deliverable

Use the thread context to understand WHAT was committed to. Build the todo title from the full context, not just the short message.

For each real commitment, return:
- title: Actionable title starting with a verb (20+ chars)
- source_ref: The permalink
- context: Channel name and what area this relates to
- detail: Full context — who was in the conversation, what was discussed, what's expected
- who_waiting: Person(s) waiting on this
- due: YYYY-MM-DD if mentioned, empty string if not

Return ONLY a JSON array. Return [] if no real commitments found. Expect 0-3 results from these %d candidates.

Messages:
%s`, len(candidates), sb.String())

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("slack commitment extraction: %w", err)
	}

	text = refresh.CleanJSON(text)

	var items []struct {
		Title      string `json:"title"`
		SourceRef  string `json:"source_ref"`
		Context    string `json:"context"`
		Detail     string `json:"detail"`
		WhoWaiting string `json:"who_waiting"`
		Due        string `json:"due"`
	}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, fmt.Errorf("parsing slack commitment response: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	var todos []db.Todo
	for _, item := range items {
		todos = append(todos, db.Todo{
			Title:      item.Title,
			Source:     "slack",
			SourceRef:  item.SourceRef,
			Context:    item.Context,
			Detail:     item.Detail,
			WhoWaiting: item.WhoWaiting,
			Due:        item.Due,
			Status:     "active",
		})
	}

	return todos, nil
}
