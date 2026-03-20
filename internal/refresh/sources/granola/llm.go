package granola

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	prompt := fmt.Sprintf(`Analyze these meeting notes and transcripts. The transcripts use speaker labels:
- [Aaron] = the user (this is whose todo list we are building)
- [Other] = other meeting participants

Extract action items and commitments where Aaron will do the work. A commitment is Aaron's if:
- Aaron states he will do something in an [Aaron] block (e.g., "I'll handle that")
- Aaron agrees/affirms in an [Aaron] block when asked by [Other] (e.g., [Other]: "Can you do X?" [Aaron]: "Yes")
- Someone in an [Other] block assigns work to Aaron by name (e.g., "Aaron will follow up on...",
  "Bob and Aaron will handle...", "Aaron is going to..."). These are commitments made ON BEHALF
  of Aaron that he needs to be aware of, even if Aaron didn't explicitly agree in the transcript.

Do NOT extract:
- Commitments made by others about THEMSELVES in [Other] blocks (e.g., [Other]: "I will handle that")
- General discussion points or ideas without a clear commitment involving Aaron
- Action items assigned to other people that don't mention Aaron by name

For each of Aaron's commitments, provide:
- title: Brief actionable title (imperative mood)
- meeting_id: The meeting ID (from the ID: field) this commitment came from
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
		MeetingID  string `json:"meeting_id"`
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
		// Build a deterministic source_ref from meeting ID + title hash.
		// This survives LLM non-determinism in ordering/count as long as
		// the title is semantically stable (which it is for the same commitment).
		h := sha256.Sum256([]byte(strings.ToLower(item.Title)))
		ref := fmt.Sprintf("%s-%s", item.MeetingID, hex.EncodeToString(h[:4]))
		todos = append(todos, db.Todo{
			Title:      item.Title,
			Source:     "granola",
			SourceRef:  ref,
			Context:    item.Context,
			Detail:     item.Detail,
			WhoWaiting: item.WhoWaiting,
			Due:        item.Due,
			Status:     "",
		})
	}

	return todos, nil
}
