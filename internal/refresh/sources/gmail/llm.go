package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
)

// detectAndLabelCommitments analyzes recent sent emails for commitments
// and applies the todo label to any that contain explicit commitments.
func detectAndLabelCommitments(ctx context.Context, l llm.LLM, client *SafeGmailClient, todoLabelID string) error {
	// Search for recent sent emails (user's own commitments)
	msgs, err := client.ListMessages(ctx, "in:sent newer_than:3d", 20)
	if err != nil {
		return fmt.Errorf("listing sent emails: %w", err)
	}

	if len(msgs) == 0 {
		return nil
	}

	// Build context for LLM analysis
	type emailCandidate struct {
		ID      string
		Subject string
		From    string
		Body    string
	}

	var candidates []emailCandidate
	for _, msg := range msgs {
		detail, err := client.GetMessage(ctx, msg.Id, "metadata", "Subject", "From", "To")
		if err != nil {
			continue
		}

		var subject, from string
		for _, h := range detail.Payload.Headers {
			switch h.Name {
			case "Subject":
				subject = h.Value
			case "From":
				from = h.Value
			}
		}

		if subject == "" {
			continue
		}

		body, err := client.GetMessageBody(ctx, msg.Id)
		if err != nil {
			continue
		}

		// Truncate long bodies
		if len(body) > 2000 {
			body = body[:2000] + "..."
		}

		candidates = append(candidates, emailCandidate{
			ID:      msg.Id,
			Subject: subject,
			From:    from,
			Body:    body,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Build prompt for LLM
	var sb strings.Builder
	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("## Email %d\n", i+1))
		sb.WriteString(fmt.Sprintf("ID: %s\n", c.ID))
		sb.WriteString(fmt.Sprintf("Subject: %s\n", c.Subject))
		sb.WriteString(fmt.Sprintf("From: %s\n", c.From))
		sb.WriteString(fmt.Sprintf("Body:\n%s\n", c.Body))
		sb.WriteString("\n---\n\n")
	}

	prompt := fmt.Sprintf(`You are filtering sent emails for real commitments the user made. The bar is VERY high.

An email contains a commitment ONLY if:
1. The user explicitly committed to a specific deliverable or action
2. There is a concrete next action with a clear outcome
3. Someone is waiting on this deliverable

REJECT emails that are:
- Informational updates without action items
- Questions without commitments
- Acknowledgements ("thanks", "got it", "sounds good")
- Past-tense descriptions of completed work

For each real commitment, return the email ID.

Return ONLY a JSON array of email ID strings. Return [] if no real commitments found.
Expect 0-2 results from these %d candidates.

Emails:
%s`, len(candidates), sb.String())

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("commitment extraction: %w", err)
	}

	text = refresh.CleanJSON(text)

	var messageIDs []string
	if err := json.Unmarshal([]byte(text), &messageIDs); err != nil {
		return fmt.Errorf("parsing commitment response: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	// Apply the todo label to identified emails
	for _, msgID := range messageIDs {
		if err := client.ModifyLabels(ctx, msgID, []string{todoLabelID}, nil); err != nil {
			return fmt.Errorf("labeling message %s: %w", msgID, err)
		}
	}

	return nil
}

// emailForTitleGen holds the data needed to generate an actionable title for a labeled email.
type emailForTitleGen struct {
	ID      string
	Subject string
	From    string
	To      string
	Body    string
}

// generateTodoTitles uses the LLM to generate actionable, verb-first titles for labeled emails.
// Returns a map of message ID → generated title. Missing entries should fall back to the subject.
func generateTodoTitles(ctx context.Context, l llm.LLM, emails []emailForTitleGen) (map[string]string, error) {
	if len(emails) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, e := range emails {
		sb.WriteString(fmt.Sprintf("## Email %d\n", i+1))
		sb.WriteString(fmt.Sprintf("ID: %s\n", e.ID))
		sb.WriteString(fmt.Sprintf("Subject: %s\n", e.Subject))
		sb.WriteString(fmt.Sprintf("From: %s\n", e.From))
		sb.WriteString(fmt.Sprintf("To: %s\n", e.To))
		sb.WriteString(fmt.Sprintf("Body:\n%s\n", e.Body))
		sb.WriteString("\n---\n\n")
	}

	prompt := fmt.Sprintf(`You are generating actionable todo titles from emails. Each email has been labeled as a todo — your job is to write a title that captures the job to be done.

Rules:
- Imperative mood, verb-first (Send, Review, Follow up, Schedule, Prepare, etc.)
- 20+ characters — specific enough to be actionable
- For received emails (From is someone else): "What is this email asking me to do?"
- For sent emails (From is the user): "What did I commit to doing?"
- Resolve pronouns and vague references using the email body
- Do NOT use the email subject as the title — subjects like "Re: Q2 Planning" or "Quick question" are useless
- Focus on the concrete action, not the topic of the thread

For each email, return its ID and your generated title.

Return ONLY a JSON array of objects: [{"id": "message_id", "title": "generated title"}, ...]

Emails:
%s`, sb.String())

	log.Printf("gmail: generating titles for %d labeled emails", len(emails))

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("gmail title generation: %w", err)
	}

	text = refresh.CleanJSON(text)

	var items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, fmt.Errorf("parsing title generation response: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	titles := make(map[string]string, len(items))
	for _, item := range items {
		if item.Title != "" {
			titles[item.ID] = item.Title
		}
	}

	log.Printf("gmail: generated %d titles from %d emails", len(titles), len(emails))
	return titles, nil
}
