package refresh

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// GranolaSource fetches meeting transcripts from Granola and uses LLM to extract commitments.
type GranolaSource struct {
	LLM     llm.LLM
	enabled bool
}

// NewGranolaSource creates a GranolaSource with the given config.
func NewGranolaSource(enabled bool, l llm.LLM) *GranolaSource {
	return &GranolaSource{
		LLM:     l,
		enabled: enabled,
	}
}

func (s *GranolaSource) Name() string  { return "granola" }
func (s *GranolaSource) Enabled() bool { return s.enabled }

func (s *GranolaSource) Fetch(ctx context.Context) (*SourceResult, error) {
	token, err := loadGranolaAuth()
	if err != nil {
		return nil, fmt.Errorf("granola auth: %w", err)
	}

	meetings, err := fetchGranolaMeetings(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	// Extract commitments via LLM if we have meetings and a real LLM
	var todos []db.Todo
	if len(meetings) > 0 && s.LLM != nil {
		todos, err = extractCommitments(ctx, s.LLM, meetings)
		if err != nil {
			// LLM extraction failure is non-fatal; return without todos
			return &SourceResult{
				Warnings: []db.Warning{{Source: "granola", Message: fmt.Sprintf("LLM extraction failed: %v", err)}},
			}, nil
		}
	}

	return &SourceResult{Todos: todos}, nil
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

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip decode: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	return io.ReadAll(reader)
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
		Text           string  `json:"text"`
		StartTimestamp float64 `json:"start_timestamp"`
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
