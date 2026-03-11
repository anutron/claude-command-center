package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/refresh"
	"golang.org/x/oauth2"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailSource fetches unread actionable emails from Gmail.
type GmailSource struct {
	enabled bool
}

// New creates a GmailSource with the given enabled flag.
func New(enabled bool) *GmailSource { return &GmailSource{enabled: enabled} }

func (s *GmailSource) Name() string  { return "gmail" }
func (s *GmailSource) Enabled() bool { return s.enabled }

func (s *GmailSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	ts, err := loadGmailAuth()
	if err != nil {
		return nil, fmt.Errorf("gmail auth: %w", err)
	}

	threads, err := fetchActionableEmails(ctx, ts)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return &refresh.SourceResult{Threads: threads}, nil
}

func loadGmailAuth() (oauth2.TokenSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".gmail-mcp", "work.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no gmail token at %s: %w", path, err)
	}

	var tf refresh.GoogleTokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing gmail token: %w", err)
	}

	clientID := tf.ClientID
	clientSecret := tf.ClientSecret
	if clientID == "" {
		clientID = os.Getenv("GMAIL_CLIENT_ID")
		clientSecret = os.Getenv("GMAIL_CLIENT_SECRET")
	}
	if clientID == "" {
		return nil, fmt.Errorf("gmail token missing clientId and GMAIL_CLIENT_ID not set")
	}

	conf := refresh.LoadGoogleOAuth2Config(clientID, clientSecret, gmail.GmailReadonlyScope)
	tok := tf.ToOAuth2Token()
	return conf.TokenSource(context.Background(), tok), nil
}

func fetchActionableEmails(ctx context.Context, ts oauth2.TokenSource) ([]db.Thread, error) {
	srv, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}

	resp, err := srv.Users.Messages.List("me").
		Q("is:unread newer_than:3d").
		MaxResults(20).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}

	var threads []db.Thread
	for _, msg := range resp.Messages {
		detail, err := srv.Users.Messages.Get("me", msg.Id).
			Format("metadata").
			MetadataHeaders("Subject", "From", "Date").
			Context(ctx).
			Do()
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

		url := fmt.Sprintf("https://mail.google.com/mail/u/0/#inbox/%s", msg.Id)

		senderName := from
		if idx := strings.Index(from, "<"); idx > 0 {
			senderName = strings.TrimSpace(from[:idx])
		}

		threads = append(threads, db.Thread{
			Type:    "email",
			Title:   subject,
			URL:     url,
			Summary: fmt.Sprintf("From: %s", senderName),
		})
	}

	return threads, nil
}
