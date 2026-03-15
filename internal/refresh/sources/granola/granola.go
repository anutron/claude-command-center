package granola

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
)

// RawMeeting is a meeting transcript from Granola.
type RawMeeting struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Transcript string    `json:"transcript"`
	Summary    string    `json:"summary"`
	Attendees  []string  `json:"attendees"`
}

// GranolaSource fetches meeting transcripts from Granola and uses LLM to extract commitments.
type GranolaSource struct {
	LLM     llm.LLM
	DB      *sql.DB
	enabled bool
}

// New creates a GranolaSource with the given config.
func New(enabled bool, l llm.LLM, database *sql.DB) *GranolaSource {
	return &GranolaSource{
		LLM:     l,
		DB:      database,
		enabled: enabled,
	}
}

func (s *GranolaSource) Name() string  { return "granola" }
func (s *GranolaSource) Enabled() bool { return s.enabled }

func (s *GranolaSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	token, err := loadGranolaAuth()
	if err != nil {
		return nil, fmt.Errorf("granola auth: %w", err)
	}

	meetings, err := fetchGranolaMeetings(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	log.Printf("granola: %d meetings found", len(meetings))

	// Only send meetings newer than our last successful sync to the LLM.
	var lastSuccess time.Time
	if s.DB != nil {
		if ss, err := db.DBLoadSourceSync(s.DB, "granola"); err == nil && ss.LastSuccess != nil {
			lastSuccess = *ss.LastSuccess
		}
	}

	var newMeetings []RawMeeting
	for _, m := range meetings {
		if !lastSuccess.IsZero() && !m.StartTime.After(lastSuccess) {
			log.Printf("granola: skipping already-processed meeting %q (%s, started %s)",
				m.Title, m.ID, m.StartTime.Format(time.RFC3339))
			continue
		}
		newMeetings = append(newMeetings, m)
	}

	log.Printf("granola: %d new meetings to process (skipped %d)", len(newMeetings), len(meetings)-len(newMeetings))
	for i, m := range newMeetings {
		log.Printf("granola: [%d] %s (transcript=%d chars, summary=%d chars)", i, m.Title, len(m.Transcript), len(m.Summary))
	}

	// Extract commitments via LLM only for new meetings
	var todos []db.Todo
	if len(newMeetings) > 0 && s.LLM != nil {
		todos, err = extractCommitments(ctx, s.LLM, newMeetings)
		if err != nil {
			return &refresh.SourceResult{
				Warnings: []db.Warning{{Source: "granola", Message: fmt.Sprintf("LLM extraction failed: %v", err)}},
			}, nil
		}
	}

	return &refresh.SourceResult{Todos: todos}, nil
}

type granolaStoredAccounts struct {
	Accounts []granolaAccount `json:"accounts"`
}

type granolaAccount struct {
	Email       string            `json:"email"`
	AccessToken string            `json:"access_token"`
	ObtainedAt  int64             `json:"obtained_at"`
	ExpiresIn   int64             `json:"expires_in"`
	Tokens      map[string]string `json:"tokens"`
}

func loadGranolaAuth() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, "Library", "Application Support", "Granola", "stored-accounts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no granola auth at %s: %w", path, err)
	}

	var wrapper struct {
		Accounts string `json:"accounts"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", fmt.Errorf("parsing granola stored-accounts.json: %w", err)
	}

	var accounts []struct {
		UserID  string `json:"userId"`
		Email   string `json:"email"`
		Tokens  string `json:"tokens"`
		SavedAt int64  `json:"savedAt"`
	}
	if err := json.Unmarshal([]byte(wrapper.Accounts), &accounts); err != nil {
		return "", fmt.Errorf("parsing granola accounts array: %w", err)
	}

	if len(accounts) == 0 {
		return "", fmt.Errorf("no granola accounts found")
	}

	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal([]byte(accounts[0].Tokens), &tokens); err != nil {
		return "", fmt.Errorf("parsing granola tokens: %w", err)
	}

	if tokens.AccessToken == "" {
		return "", fmt.Errorf("granola access token is empty")
	}

	if accounts[0].SavedAt > 0 && tokens.ExpiresIn > 0 {
		savedAt := time.UnixMilli(accounts[0].SavedAt)
		expiresAt := savedAt.Add(time.Duration(tokens.ExpiresIn) * time.Second)
		if time.Now().After(expiresAt) {
			return "", fmt.Errorf("granola token expired at %s — open Granola app to refresh", expiresAt.Format(time.RFC3339))
		}
	}

	return tokens.AccessToken, nil
}

const granolaAPI = "https://api.granola.ai"

func fetchGranolaMeetings(ctx context.Context, token string) ([]RawMeeting, error) {
	meetings, err := granolaListMeetings(ctx, token)
	if err != nil {
		return nil, err
	}

	for i := range meetings {
		transcript, err := granolaGetTranscript(ctx, token, meetings[i].ID)
		if err != nil {
			log.Printf("granola: transcript error for %q: %v", meetings[i].Title, err)
			continue
		}
		meetings[i].Transcript = transcript
	}

	return meetings, nil
}

func granolaPost(ctx context.Context, token, endpoint string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", granolaAPI+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("granola %s: HTTP %d", endpoint, resp.StatusCode)
	}

	const maxResponseSize = 10 * 1024 * 1024 // 10MB

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip decode: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	return io.ReadAll(io.LimitReader(reader, maxResponseSize))
}

func granolaListMeetings(ctx context.Context, token string) ([]RawMeeting, error) {
	data, err := granolaPost(ctx, token, "/v2/get-documents", struct{}{})
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}

	var resp struct {
		Docs []struct {
			ID         string  `json:"id"`
			Title      string  `json:"title"`
			CreatedAt  string  `json:"created_at"`
			UpdatedAt  string  `json:"updated_at"`
			DeletedAt  *string `json:"deleted_at"`
			Summary    *string `json:"summary"`
			NotesPlain *string `json:"notes_plain"`
			People     *struct {
				Creator struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"creator"`
				Attendees []struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"attendees"`
			} `json:"people"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing documents: %w", err)
	}

	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, weekStart.Location())
	cutoff := weekStart.Format(time.RFC3339)

	var meetings []RawMeeting
	for _, doc := range resp.Docs {
		if doc.DeletedAt != nil {
			continue
		}
		if doc.CreatedAt < cutoff {
			continue
		}
		if doc.Title == "" {
			continue
		}

		m := RawMeeting{
			ID:    doc.ID,
			Title: doc.Title,
		}

		if t, err := time.Parse(time.RFC3339Nano, doc.CreatedAt); err == nil {
			m.StartTime = t
		}
		if doc.Summary != nil {
			m.Summary = *doc.Summary
		}
		if doc.NotesPlain != nil && m.Summary == "" {
			m.Summary = *doc.NotesPlain
		}

		if doc.People != nil {
			for _, a := range doc.People.Attendees {
				name := a.Name
				if name == "" {
					name = a.Email
				}
				if name != "" {
					m.Attendees = append(m.Attendees, name)
				}
			}
		}

		meetings = append(meetings, m)
	}

	return meetings, nil
}

func granolaGetTranscript(ctx context.Context, token, documentID string) (string, error) {
	data, err := granolaPost(ctx, token, "/v1/get-document-transcript", map[string]string{
		"document_id": documentID,
	})
	if err != nil {
		return "", err
	}

	var chunks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &chunks); err != nil {
		return "", fmt.Errorf("parsing transcript: %w", err)
	}

	if len(chunks) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for _, c := range chunks {
		if c.Text != "" {
			sb.WriteString(c.Text)
			sb.WriteString(" ")
		}
	}

	return strings.TrimSpace(sb.String()), nil
}
