package gmail

import (
	"context"
	"encoding/json"
	"fmt"
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
