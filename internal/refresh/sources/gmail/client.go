package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// SAFETY: Do NOT add Send, Delete, or Trash methods to SafeGmailClient.
// Do NOT expose the raw *gmail.Service outside this wrapper.
// These restrictions require explicit override from the user. See CLAUDE.md.

// SafeGmailClient wraps the Gmail API with a restricted surface area.
// Only read, label modification, and draft creation are permitted.
type SafeGmailClient struct {
	svc      *gmail.Service // unexported — never expose
	advanced bool
}

// NewSafeClient creates a SafeGmailClient.
// If advanced is false, only read operations are available; ModifyLabels and CreateDraft return errors.
func NewSafeClient(ctx context.Context, ts oauth2.TokenSource, advanced bool) (*SafeGmailClient, error) {
	svc, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &SafeGmailClient{svc: svc, advanced: advanced}, nil
}

// ListMessages searches Gmail messages matching the query.
func (c *SafeGmailClient) ListMessages(ctx context.Context, query string, maxResults int64) ([]*gmail.Message, error) {
	resp, err := c.svc.Users.Messages.List("me").
		Q(query).
		MaxResults(maxResults).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

// GetMessage fetches a single message with the specified format and metadata headers.
func (c *SafeGmailClient) GetMessage(ctx context.Context, id string, format string, headers ...string) (*gmail.Message, error) {
	call := c.svc.Users.Messages.Get("me", id).
		Format(format).
		Context(ctx)
	if len(headers) > 0 && format == "metadata" {
		call = call.MetadataHeaders(headers...)
	}
	return call.Do()
}

// GetMessageBody fetches the plain-text body of a message for LLM analysis.
func (c *SafeGmailClient) GetMessageBody(ctx context.Context, id string) (string, error) {
	msg, err := c.svc.Users.Messages.Get("me", id).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return "", err
	}
	return extractTextBody(msg.Payload), nil
}

// ModifyLabels adds or removes labels from a message.
// Returns an error if the client is not in advanced mode.
func (c *SafeGmailClient) ModifyLabels(ctx context.Context, messageID string, addLabelIDs, removeLabelIDs []string) error {
	if !c.advanced {
		return fmt.Errorf("gmail: label modification requires advanced mode (gmail.advanced: true in config)")
	}
	req := &gmail.ModifyMessageRequest{
		AddLabelIds:    addLabelIDs,
		RemoveLabelIds: removeLabelIDs,
	}
	_, err := c.svc.Users.Messages.Modify("me", messageID, req).Context(ctx).Do()
	return err
}

// GetLabelID resolves a label name to its Gmail label ID.
// Returns an error if the label is not found.
func (c *SafeGmailClient) GetLabelID(ctx context.Context, name string) (string, error) {
	resp, err := c.svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("listing labels: %w", err)
	}
	lower := strings.ToLower(name)
	for _, l := range resp.Labels {
		if strings.ToLower(l.Name) == lower {
			return l.Id, nil
		}
	}
	return "", fmt.Errorf("gmail label %q not found", name)
}

// extractTextBody walks a MIME payload tree looking for text/plain parts.
func extractTextBody(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}

	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(part.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}

	for _, child := range part.Parts {
		if text := extractTextBody(child); text != "" {
			return text
		}
	}
	return ""
}
