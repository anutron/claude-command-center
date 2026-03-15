package slack

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/refresh"
)

// SlackSource fetches Slack messages with commitment language and uses LLM to extract todos.
type SlackSource struct {
	enabled  bool
	botToken string
	LLM      llm.LLM
	DB       *sql.DB
}

// New creates a SlackSource with the given token and LLM.
func New(enabled bool, botToken string, l llm.LLM, database *sql.DB) *SlackSource {
	return &SlackSource{enabled: enabled, botToken: botToken, LLM: l, DB: database}
}

func (s *SlackSource) Name() string  { return "slack" }
func (s *SlackSource) Enabled() bool { return s.enabled }

func (s *SlackSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	token := strings.TrimSpace(s.botToken)
	if token == "" {
		return nil, fmt.Errorf("slack auth: bot token not configured")
	}

	candidates, err := fetchSlackCandidates(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	log.Printf("slack: %d candidates found", len(candidates))

	// Only send messages newer than our last successful sync to the LLM.
	var lastSuccess time.Time
	if s.DB != nil {
		if ss, err := db.DBLoadSourceSync(s.DB, "slack"); err == nil && ss.LastSuccess != nil {
			lastSuccess = *ss.LastSuccess
		}
	}

	var newCandidates []slackCandidate
	for _, c := range candidates {
		if !lastSuccess.IsZero() && c.Timestamp != "" {
			// Slack timestamps are Unix epoch with decimal (e.g. "1710000000.000100")
			if ts, err := strconv.ParseFloat(c.Timestamp, 64); err == nil {
				msgTime := time.Unix(int64(ts), 0)
				if !msgTime.After(lastSuccess) {
					continue
				}
			}
		}
		newCandidates = append(newCandidates, c)
	}

	log.Printf("slack: %d new candidates to process (skipped %d)", len(newCandidates), len(candidates)-len(newCandidates))

	// Extract commitments via LLM only for new candidates
	var todos []db.Todo
	if len(newCandidates) > 0 && s.LLM != nil {
		todos, err = extractSlackCommitments(ctx, s.LLM, newCandidates)
		if err != nil {
			return &refresh.SourceResult{
				Warnings: []db.Warning{{Source: "slack", Message: fmt.Sprintf("LLM extraction failed: %v", err)}},
			}, nil
		}
	}

	return &refresh.SourceResult{Todos: todos}, nil
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

// API response types for bot-compatible endpoints.

type slackConversationsListResponse struct {
	OK       bool           `json:"ok"`
	Channels []slackChannel `json:"channels"`
	Error    string         `json:"error,omitempty"`
	Meta     struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

type slackChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type slackHistoryResponse struct {
	OK       bool                `json:"ok"`
	Messages []slackHistoryEntry `json:"messages"`
	Error    string              `json:"error,omitempty"`
}

type slackHistoryEntry struct {
	Type string `json:"type"`
	User string `json:"user"`
	Text string `json:"text"`
	TS   string `json:"ts"`
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

// slackAPIGet performs a GET request to the Slack API and decodes the response.
func slackAPIGet(ctx context.Context, token, endpoint string, params url.Values, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://slack.com/api/"+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return err
	}

	return json.Unmarshal(body, dest)
}

// fetchChannels retrieves the list of channels the bot has access to.
func fetchChannels(ctx context.Context, token string) ([]slackChannel, error) {
	params := url.Values{
		"types":            {"public_channel,private_channel"},
		"exclude_archived": {"true"},
		"limit":            {"200"},
	}

	var result slackConversationsListResponse
	if err := slackAPIGet(ctx, token, "conversations.list", params, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("conversations.list error: %s", result.Error)
	}

	return result.Channels, nil
}

// fetchChannelHistory retrieves recent messages from a channel since the given timestamp.
func fetchChannelHistory(ctx context.Context, token, channelID string, oldest string) ([]slackHistoryEntry, error) {
	params := url.Values{
		"channel": {channelID},
		"oldest":  {oldest},
		"limit":   {"100"},
	}

	var result slackHistoryResponse
	if err := slackAPIGet(ctx, token, "conversations.history", params, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("conversations.history error: %s", result.Error)
	}

	return result.Messages, nil
}

// buildPermalink constructs a Slack message permalink from channel ID and timestamp.
// Format: https://slack.com/archives/{channelID}/p{ts_without_dot}
func buildPermalink(channelID, ts string) string {
	// Slack permalinks use the timestamp without the dot
	tsNoDot := strings.Replace(ts, ".", "", 1)
	return fmt.Sprintf("https://app.slack.com/archives/%s/p%s", channelID, tsNoDot)
}

// isMissingScopeError checks if a Slack API error indicates a missing OAuth scope.
func isMissingScopeError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "missing_scope") || strings.Contains(err.Error(), "not_allowed_token_type")
}

func fetchSlackCandidates(ctx context.Context, token string) ([]slackCandidate, error) {
	// Try conversations-based approach first (requires channels:read scope).
	// If it fails due to missing scope, fall back to search.messages (requires search:read only).
	channels, err := fetchChannels(ctx, token)
	if err != nil {
		if isMissingScopeError(err) {
			// Fall back to search.messages which only requires search:read scope
			return fetchSlackCandidatesViaSearch(ctx, token)
		}
		return nil, fmt.Errorf("listing channels: %w", err)
	}

	// Look back 3 days (matching the old search.messages "after:3days" filter)
	oldest := strconv.FormatInt(time.Now().Add(-3*24*time.Hour).Unix(), 10)

	var candidates []slackCandidate
	for _, ch := range channels {
		messages, err := fetchChannelHistory(ctx, token, ch.ID, oldest)
		if err != nil {
			if isMissingScopeError(err) {
				// Token lacks channels:history — fall back to search API
				return fetchSlackCandidatesViaSearch(ctx, token)
			}
			// Skip individual channel errors (permissions, etc.) — non-fatal
			continue
		}

		for _, msg := range messages {
			if msg.Type != "message" || msg.Text == "" {
				continue
			}
			if !hasCommitmentLanguage(msg.Text) {
				continue
			}

			c := slackCandidate{
				Message:   msg.Text,
				Permalink: buildPermalink(ch.ID, msg.TS),
				Channel:   ch.Name,
				ChannelID: ch.ID,
				Timestamp: msg.TS,
			}

			// Fetch thread context if this message is part of a thread — non-fatal if scope missing
			thread, threadErr := fetchThreadContext(ctx, token, ch.ID, msg.TS)
			if threadErr == nil && len(thread) > 1 {
				var sb strings.Builder
				for _, reply := range thread {
					sb.WriteString(reply.Text)
					sb.WriteString("\n")
				}
				c.ThreadContext = sb.String()
			}

			candidates = append(candidates, c)
		}
	}

	return candidates, nil
}

// slackSearchResponse is the response from search.messages API.
type slackSearchResponse struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Messages struct {
		Matches []slackSearchMatch `json:"matches"`
	} `json:"messages"`
}

type slackSearchMatch struct {
	Text      string `json:"text"`
	Timestamp string `json:"ts"`
	Permalink string `json:"permalink"`
	Channel   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
}

// fetchSlackCandidatesViaSearch uses the search.messages API (requires only search:read scope)
// to find commitment-language messages. This is the fallback when conversations.list is unavailable.
func fetchSlackCandidatesViaSearch(ctx context.Context, token string) ([]slackCandidate, error) {
	var allCandidates []slackCandidate

	// Search for commitment phrases in batches — Slack search supports basic query strings
	// We group related phrases to minimize API calls
	searchQueries := []string{
		"i'll",
		"i will",
		"action item",
		"follow up",
		"let me",
	}

	seen := make(map[string]bool)
	for _, query := range searchQueries {
		params := url.Values{
			"query": {fmt.Sprintf("%s after:3d", query)},
			"count": {"50"},
			"sort":  {"timestamp"},
		}

		var result slackSearchResponse
		if err := slackAPIGet(ctx, token, "search.messages", params, &result); err != nil {
			return nil, fmt.Errorf("search.messages: %w", err)
		}
		if !result.OK {
			return nil, fmt.Errorf("search.messages error: %s", result.Error)
		}

		for _, match := range result.Messages.Matches {
			if match.Text == "" {
				continue
			}
			// Deduplicate by timestamp+channel
			key := match.Channel.ID + ":" + match.Timestamp
			if seen[key] {
				continue
			}
			seen[key] = true

			if !hasCommitmentLanguage(match.Text) {
				continue
			}

			permalink := match.Permalink
			if permalink == "" {
				permalink = buildPermalink(match.Channel.ID, match.Timestamp)
			}

			allCandidates = append(allCandidates, slackCandidate{
				Message:   match.Text,
				Permalink: permalink,
				Channel:   match.Channel.Name,
				ChannelID: match.Channel.ID,
				Timestamp: match.Timestamp,
			})
		}
	}

	return allCandidates, nil
}

func fetchThreadContext(ctx context.Context, token, channelID, ts string) ([]slackReply, error) {
	params := url.Values{
		"channel": {channelID},
		"ts":      {ts},
		"limit":   {"20"},
	}

	var result slackRepliesResponse
	if err := slackAPIGet(ctx, token, "conversations.replies", params, &result); err != nil {
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
