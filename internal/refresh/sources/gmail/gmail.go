package gmail

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/anutron/claude-command-center/internal/auth"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
	"golang.org/x/oauth2"
	gmail "google.golang.org/api/gmail/v1"
)

// GmailSource fetches unread actionable emails and label-based todos from Gmail.
type GmailSource struct {
	cfg    config.GmailConfig
	llm    llm.LLM
	client *SafeGmailClient // populated during Fetch, reused in PostMerge

	// freshLabeledIDs tracks message IDs returned by the label query during Fetch.
	// Used by PostMerge to know which emails currently have the todo label.
	freshLabeledIDs map[string]bool
}

// New creates a GmailSource with the given config and optional LLM.
func New(cfg config.GmailConfig, l llm.LLM) *GmailSource {
	return &GmailSource{cfg: cfg, llm: l}
}

func (s *GmailSource) Name() string  { return "gmail" }
func (s *GmailSource) Enabled() bool { return s.cfg.Enabled }

func (s *GmailSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	ts, err := loadGmailAuth(s.cfg.Advanced)
	if err != nil {
		return nil, fmt.Errorf("gmail auth: %w", err)
	}

	client, err := NewSafeClient(ctx, ts, s.cfg.Advanced)
	if err != nil {
		return nil, fmt.Errorf("gmail client: %w", err)
	}
	s.client = client

	result := &refresh.SourceResult{}

	// Fetch label-based todos if configured
	if s.cfg.TodoLabel != "" {
		todos, labeledIDs, err := fetchLabeledTodos(ctx, client, s.cfg.TodoLabel)
		if err != nil {
			log.Printf("gmail: label todos: %v", err)
		} else {
			log.Printf("gmail: %d labeled todos found", len(todos))
			result.Todos = todos
			s.freshLabeledIDs = labeledIDs
		}

		// LLM commitment detection — auto-label emails (requires advanced mode)
		if s.cfg.Advanced && s.llm != nil {
			labelID, err := client.GetLabelID(ctx, s.cfg.TodoLabel)
			if err != nil {
				log.Printf("gmail: resolve label ID: %v", err)
			} else {
				log.Printf("gmail: scanning sent mail for commitments (label ID: %s)", labelID)
				if err := detectAndLabelCommitments(ctx, s.llm, client, labelID); err != nil {
					log.Printf("gmail: commitment detection: %v", err)
				} else {
					log.Printf("gmail: commitment detection complete")
				}
			}
		} else if s.cfg.Advanced && s.llm == nil {
			log.Printf("gmail: advanced mode enabled but LLM is nil — skipping commitment detection")
		}
	} else {
		log.Printf("gmail: no todo_label configured — skipping label sync")
	}

	return result, nil
}

// PostMerge implements refresh.PostMerger.
//
// For completed gmail todos whose email still has the todo label:
//   - Remove the label from the email (cleanup after completion).
//
// If a user re-labels a completed email's NEW reply, that's a different message ID
// and creates a new todo naturally. For the edge case of re-labeling the exact same
// message after completion, the label gets removed again on the next refresh.
func (s *GmailSource) PostMerge(ctx context.Context, database *sql.DB, cc *db.CommandCenter, verbose bool) error {
	if !s.cfg.Advanced || s.cfg.TodoLabel == "" || s.client == nil {
		return nil
	}

	labelID, err := s.client.GetLabelID(ctx, s.cfg.TodoLabel)
	if err != nil {
		return fmt.Errorf("resolve label ID: %w", err)
	}

	for _, todo := range cc.Todos {
		if todo.Source != "gmail" || todo.SourceRef == "" || todo.Status != "completed" {
			continue
		}

		// Only remove label if the email currently has it
		if !s.freshLabeledIDs[todo.SourceRef] {
			continue
		}

		if err := s.client.ModifyLabels(ctx, todo.SourceRef, nil, []string{labelID}); err != nil {
			log.Printf("gmail: remove label from %s: %v", todo.SourceRef, err)
			continue
		}
		if verbose {
			log.Printf("gmail: removed label from completed todo %q (%s)", todo.Title, todo.SourceRef)
		}
	}

	return nil
}

func loadGmailAuth(advanced bool) (oauth2.TokenSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".gmail-mcp", "work.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no gmail token at %s: %w", path, err)
	}

	var tf auth.GoogleTokenFile
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

	var scopes []string
	if advanced {
		scopes = []string{gmail.GmailModifyScope, gmail.GmailComposeScope}
	} else {
		scopes = []string{gmail.GmailReadonlyScope}
	}

	conf := auth.LoadGoogleOAuth2Config(clientID, clientSecret, scopes...)
	tok := tf.ToOAuth2Token()
	return conf.TokenSource(context.Background(), tok), nil
}

// fetchLabeledTodos queries emails with the given label and returns todos + the set of message IDs found.
func fetchLabeledTodos(ctx context.Context, client *SafeGmailClient, labelName string) ([]db.Todo, map[string]bool, error) {
	query := fmt.Sprintf("label:%s", labelName)
	msgs, err := client.ListMessages(ctx, query, 50)
	if err != nil {
		return nil, nil, fmt.Errorf("searching label %q: %w", labelName, err)
	}

	labeledIDs := make(map[string]bool, len(msgs))
	var todos []db.Todo

	for _, msg := range msgs {
		labeledIDs[msg.Id] = true

		detail, err := client.GetMessage(ctx, msg.Id, "metadata", "Subject", "From")
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

		senderName := from
		if idx := strings.Index(from, "<"); idx > 0 {
			senderName = strings.TrimSpace(from[:idx])
		}

		todos = append(todos, db.Todo{
			Title:     subject,
			Source:    "gmail",
			SourceRef: msg.Id,
			Context:   fmt.Sprintf("From: %s", senderName),
			Status:    "active",
		})
	}

	return todos, labeledIDs, nil
}
