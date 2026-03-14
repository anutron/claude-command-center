package granola

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
)

func extractCommitments(ctx context.Context, l llm.LLM, meetings []RawMeeting) ([]db.Todo, error) {
	if len(meetings) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for _, m := range meetings {
		sb.WriteString(fmt.Sprintf("## Meeting: %s (ID: %s)\n", m.Title, m.ID))
		sb.WriteString(fmt.Sprintf("Date: %s\n", m.StartTime.Format("2006-01-02 15:04")))
		if len(m.Attendees) > 0 {
			sb.WriteString(fmt.Sprintf("Attendees: %s\n", strings.Join(m.Attendees, ", ")))
		}
		if m.Summary != "" {
			sb.WriteString(fmt.Sprintf("Summary: %s\n", m.Summary))
		}
		if m.Transcript != "" {
			transcript := m.Transcript
			if len(transcript) > 8000 {
				transcript = transcript[:8000] + "\n...[truncated]"
			}
			sb.WriteString(fmt.Sprintf("Transcript:\n%s\n", transcript))
		}
		sb.WriteString("\n---\n\n")
	}

	prompt := fmt.Sprintf(`Analyze these meeting notes and transcripts. Extract action items, commitments, and things the user said they would do or that others are expecting from them.

For each commitment, provide:
- title: Brief actionable title (imperative mood)
- source_ref: The meeting ID (from the ID: field) for deduplication. If multiple items come from the same meeting, append a short suffix like "-1", "-2" etc.
- context: Which project or area this relates to
- detail: Comprehensive context including who was in the meeting, what was discussed, what's expected, and by when
- who_waiting: Person(s) waiting on this (if identifiable)
- due: Due date in YYYY-MM-DD format if mentioned, empty string if not

Return ONLY a JSON array of objects with these fields. No other text. Return [] if no commitments found.

Meetings:
%s`, sb.String())

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("commitment extraction: %w", err)
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
		return nil, fmt.Errorf("parsing commitment response: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	var todos []db.Todo
	seen := make(map[string]int)
	for _, item := range items {
		ref := item.SourceRef
		// Ensure unique source_ref — append counter if LLM didn't suffix
		if n, ok := seen[ref]; ok {
			ref = fmt.Sprintf("%s-%d", ref, n+1)
		}
		seen[item.SourceRef]++
		todos = append(todos, db.Todo{
			Title:      item.Title,
			Source:     "granola",
			SourceRef:  ref,
			Context:    item.Context,
			Detail:     item.Detail,
			WhoWaiting: item.WhoWaiting,
			Due:        item.Due,
			Status:     "active",
		})
	}

	return todos, nil
}
