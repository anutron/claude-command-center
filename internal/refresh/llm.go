package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
)

func extractCommitments(ctx context.Context, meetings []RawMeeting) ([]db.Todo, error) {
	if len(meetings) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for _, m := range meetings {
		sb.WriteString(fmt.Sprintf("## Meeting: %s\n", m.Title))
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
- source_ref: The meeting ID for deduplication
- context: Which project or area this relates to
- detail: Comprehensive context including who was in the meeting, what was discussed, what's expected, and by when
- who_waiting: Person(s) waiting on this (if identifiable)
- due: Due date in YYYY-MM-DD format if mentioned, empty string if not

Return ONLY a JSON array of objects with these fields. No other text. Return [] if no commitments found.

Meetings:
%s`, sb.String())

	text, err := callClaude(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("commitment extraction: %w", err)
	}

	text = cleanJSON(text)

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
	for _, item := range items {
		todos = append(todos, db.Todo{
			Title:      item.Title,
			Source:     "granola",
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

func generateSuggestions(ctx context.Context, cc *db.CommandCenter) (*db.Suggestions, error) {
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

	text, err := callClaude(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("suggestion generation: %w", err)
	}

	text = cleanJSON(text)

	var suggestions db.Suggestions
	if err := json.Unmarshal([]byte(text), &suggestions); err != nil {
		return nil, fmt.Errorf("parsing suggestions: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	return &suggestions, nil
}

func callClaude(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--output-format", "text",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func cleanJSON(s string) string {
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
