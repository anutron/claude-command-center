package gmail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	gm "google.golang.org/api/gmail/v1"
)

// ContextFetcherImpl implements refresh.ContextFetcher for Gmail threads.
type ContextFetcherImpl struct {
	cfg config.GmailConfig
}

// NewContextFetcher creates a new Gmail ContextFetcher.
func NewContextFetcher(cfg config.GmailConfig) *ContextFetcherImpl {
	return &ContextFetcherImpl{cfg: cfg}
}

func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 24 * time.Hour }

func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
	ctx := context.Background()

	ts, err := loadGmailAuth(f.cfg.Advanced)
	if err != nil {
		return "", fmt.Errorf("gmail auth: %w", err)
	}

	client, err := NewSafeClient(ctx, ts, false)
	if err != nil {
		return "", fmt.Errorf("gmail client: %w", err)
	}

	// sourceRef is a message ID; fetch the message to get the thread ID.
	msg, err := client.GetMessage(ctx, sourceRef, "metadata")
	if err != nil {
		return "", fmt.Errorf("get message %s: %w", sourceRef, err)
	}

	thread, err := client.GetThread(ctx, msg.ThreadId)
	if err != nil {
		return "", fmt.Errorf("get thread %s: %w", msg.ThreadId, err)
	}

	var parts []string
	for _, m := range thread.Messages {
		parts = append(parts, formatThreadMessage(m))
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}

// formatThreadMessage formats a single message from a thread for display.
func formatThreadMessage(m *gm.Message) string {
	var from, to, subject, date string
	if m.Payload != nil {
		for _, h := range m.Payload.Headers {
			switch h.Name {
			case "From":
				from = h.Value
			case "To":
				to = h.Value
			case "Subject":
				subject = h.Value
			case "Date":
				date = h.Value
			}
		}
	}

	var sb strings.Builder
	if subject != "" {
		fmt.Fprintf(&sb, "**Subject:** %s\n", subject)
	}
	if from != "" {
		fmt.Fprintf(&sb, "**From:** %s\n", from)
	}
	if to != "" {
		fmt.Fprintf(&sb, "**To:** %s\n", to)
	}
	if date != "" {
		fmt.Fprintf(&sb, "**Date:** %s\n", date)
	}
	sb.WriteString("\n")

	body := extractTextBody(m.Payload)
	if body != "" {
		sb.WriteString(body)
	} else {
		sb.WriteString("(no text body)")
	}

	return sb.String()
}
