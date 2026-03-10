package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
)

// SlackSource fetches Slack messages with commitment language and uses LLM to extract todos.
type SlackSource struct {
	LLM llm.LLM
}

// New creates a SlackSource with the given LLM.
func New(l llm.LLM) *SlackSource {
	return &SlackSource{LLM: l}
}

func (s *SlackSource) Name() string  { return "slack" }
func (s *SlackSource) Enabled() bool { return true } // always enabled; auth check in Fetch

func (s *SlackSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	token, err := loadSlackToken()
	if err != nil {
		return nil, fmt.Errorf("slack auth: %w", err)
	}

	candidates, err := fetchSlackCandidates(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	// Extract commitments via LLM if we have candidates and a real LLM
	var todos []db.Todo
	if len(candidates) > 0 && s.LLM != nil {
		todos, err = extractSlackCommitments(ctx, s.LLM, candidates)
		if err != nil {
			// LLM extraction failure is non-fatal; return raw data without todos
			return &refresh.SourceResult{
				Warnings: []db.Warning{{Source: "slack", Message: fmt.Sprintf("LLM extraction failed: %v", err)}},
			}, nil
		}
	}

	return &refresh.SourceResult{Todos: todos}, nil
}

func loadSlackToken() (string, error) {
	tok := os.Getenv("SLACK_BOT_TOKEN")
	if tok == "" {
		return "", fmt.Errorf("SLACK_BOT_TOKEN not set")
	}
	return tok, nil
}

// slackCandidate is a Slack message that may contain a commitment.
type slackCandidate struct {
	Message       string
	Permalink     string
	Channel       string
	ChannelID     string
	Timestamp     string
	ThreadContext string
}

var commitmentPhrases = []string{
	"i'll", "i will", "i need to", "let me", "i'm going to",
	"action item", "i committed", "i promised", "follow up",
	"send you", "set up", "schedule", "i can do", "i'll take",
	"i'll handle", "i'll get", "i'll send", "i'll look",
	"i'll check", "i'll follow", "i'll set", "i'll make",
	"i'll write", "i'll review", "i'll update", "i'll fix",
	"i'll create", "i'll put", "i'll share", "i'll reach out",
}

type slackSearchResponse struct {
	OK       bool `json:"ok"`
	Messages struct {
		Matches []slackMessage `json:"matches"`
	} `json:"messages"`
	Error string `json:"error,omitempty"`
}

type slackMessage struct {
	Text      string `json:"text"`
	Permalink string `json:"permalink"`
	Timestamp string `json:"ts"`
	Channel   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	Username string `json:"username"`
}

type slackReply struct {
	User string `json:"user"`
	Text string `json:"text"`
	TS   string `json:"ts"`
}

type slackRepliesResponse struct {
	OK       bool         `json:"ok"`
	Messages []slackReply `json:"messages"`
	Error    string       `json:"error,omitempty"`
}

func fetchSlackCandidates(ctx context.Context, token string) ([]slackCandidate, error) {
	params := url.Values{
		"query": {"from:me after:3days"},
		"sort":  {"timestamp"},
		"count": {"50"},
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://slack.com/api/search.messages?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result slackSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	var candidates []slackCandidate
	for _, msg := range result.Messages.Matches {
		if !hasCommitmentLanguage(msg.Text) {
			continue
		}

		c := slackCandidate{
			Message:   msg.Text,
			Permalink: msg.Permalink,
			Channel:   msg.Channel.Name,
			ChannelID: msg.Channel.ID,
			Timestamp: msg.Timestamp,
		}

		thread, err := fetchThreadContext(ctx, token, msg.Channel.ID, msg.Timestamp)
		if err == nil && len(thread) > 1 {
			var sb strings.Builder
			for _, reply := range thread {
				sb.WriteString(reply.Text)
				sb.WriteString("\n")
			}
			c.ThreadContext = sb.String()
		}

		candidates = append(candidates, c)
	}

	return candidates, nil
}

func fetchThreadContext(ctx context.Context, token, channelID, ts string) ([]slackReply, error) {
	params := url.Values{
		"channel": {channelID},
		"ts":      {ts},
		"limit":   {"20"},
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://slack.com/api/conversations.replies?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result slackRepliesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("slack replies error: %s", result.Error)
	}

	return result.Messages, nil
}

func hasCommitmentLanguage(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range commitmentPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
