package refresh

import (
	"context"
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

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
