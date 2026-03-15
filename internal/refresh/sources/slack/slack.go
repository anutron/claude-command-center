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
	// Subtract a 2-minute overlap to avoid losing messages sent during a sync
	// cycle (lastSuccess is recorded at sync completion, not fetch start).
	var lastSuccess time.Time
	if s.DB != nil {
		if ss, err := db.DBLoadSourceSync(s.DB, "slack"); err == nil && ss.LastSuccess != nil {
			lastSuccess = ss.LastSuccess.Add(-2 * time.Minute)
			log.Printf("slack: last successful sync: %s (with 2min overlap: %s)",
				ss.LastSuccess.Format(time.RFC3339), lastSuccess.Format(time.RFC3339))
		} else {
			log.Printf("slack: no previous successful sync found — processing all candidates")
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
			log.Printf("slack: LLM extraction failed: %v", err)
			return &refresh.SourceResult{
				Warnings: []db.Warning{{Source: "slack", Message: fmt.Sprintf("LLM extraction failed: %v", err)}},
			}, nil
		}
	} else if len(newCandidates) > 0 && s.LLM == nil {
		log.Printf("slack: WARNING — %d candidates found but LLM is nil, cannot extract commitments", len(newCandidates))
	}

	log.Printf("slack: returning %d todos", len(todos))
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
	"action item", "i committed", "i promise", "follow up",
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
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IsIM    bool    `json:"is_im"`
	IsMpim  bool    `json:"is_mpim"`
	User    string  `json:"user"`    // For IM channels: the other user's ID (or self for self-DMs)
	Updated float64 `json:"updated"` // Unix timestamp of last activity (0 if never)
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

// slackAuthTestResponse is the response from auth.test API.
type slackAuthTestResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	UserID string `json:"user_id"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
}

// slackUserInfoResponse is the response from users.info API.
type slackUserInfoResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	User  struct {
		RealName    string `json:"real_name"`
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
	} `json:"user"`
}

// fetchAuthIdentity calls auth.test to get the authenticated user's ID.
func fetchAuthIdentity(ctx context.Context, token string) (userID string, userName string, err error) {
	var result slackAuthTestResponse
	if err := slackAPIGet(ctx, token, "auth.test", url.Values{}, &result); err != nil {
		return "", "", err
	}
	if !result.OK {
		return "", "", fmt.Errorf("auth.test error: %s", result.Error)
	}
	return result.UserID, result.User, nil
}

// fetchUserName looks up a user's display name by ID. Returns "" on error.
func fetchUserName(ctx context.Context, token, userID string) string {
	var result slackUserInfoResponse
	if err := slackAPIGet(ctx, token, "users.info", url.Values{"user": {userID}}, &result); err != nil {
		return ""
	}
	if !result.OK {
		return ""
	}
	if result.User.RealName != "" {
		return result.User.RealName
	}
	if result.User.DisplayName != "" {
		return result.User.DisplayName
	}
	return result.User.Name
}

// channelDisplayName returns a human-readable channel name, handling IM channels
// which have no name field in the Slack API response.
func channelDisplayName(ctx context.Context, token string, ch slackChannel, selfUserID string) string {
	if ch.Name != "" {
		return ch.Name
	}
	if ch.IsIM {
		if ch.User == selfUserID {
			return "self-DM"
		}
		name := fetchUserName(ctx, token, ch.User)
		if name != "" {
			return "DM:" + name
		}
		return "DM:" + ch.User
	}
	if ch.IsMpim {
		return "group-DM:" + ch.ID
	}
	return ch.ID
}

// slackAPIGet performs a GET request to the Slack API and decodes the response.
// Retries once on rate limit (429) using the Retry-After header.
func slackAPIGet(ctx context.Context, token, endpoint string, params url.Values, dest interface{}) error {
	for attempt := 0; attempt < 3; attempt++ {
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

		if resp.StatusCode == 429 {
			resp.Body.Close()
			retryAfter := 5 // default 5s
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if v, err := strconv.Atoi(ra); err == nil {
					retryAfter = v
				}
			}
			log.Printf("slack: rate limited on %s, retrying in %ds", endpoint, retryAfter)
			select {
			case <-time.After(time.Duration(retryAfter) * time.Second):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		const maxResponseSize = 10 * 1024 * 1024 // 10MB
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
		if err != nil {
			return err
		}

		return json.Unmarshal(body, dest)
	}
	return fmt.Errorf("%s: rate limited after retries", endpoint)
}

// fetchChannels retrieves channels the user is a member of (public + private only).
// DMs and group DMs are handled by the search.messages fallback to avoid fetching
// history from thousands of conversations.
func fetchChannels(ctx context.Context, token string) ([]slackChannel, error) {
	var allChannels []slackChannel
	cursor := ""

	for {
		params := url.Values{
			"types":            {"public_channel,private_channel"},
			"exclude_archived": {"true"},
			"limit":            {"999"},
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}

		var result slackConversationsListResponse
		if err := slackAPIGet(ctx, token, "users.conversations", params, &result); err != nil {
			return nil, fmt.Errorf("listing channels: %w", err)
		}
		if !result.OK {
			if result.Error == "ratelimited" {
				log.Printf("slack: users.conversations rate limited, waiting 5s")
				select {
				case <-time.After(5 * time.Second):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, fmt.Errorf("users.conversations error: %s", result.Error)
		}

		allChannels = append(allChannels, result.Channels...)

		// Paginate if there are more results
		if result.Meta.NextCursor == "" {
			break
		}
		cursor = result.Meta.NextCursor
		log.Printf("slack: users.conversations paginating (have %d channels so far)", len(allChannels))
	}

	return allChannels, nil
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
	// Get the authenticated user's identity for labeling self-DMs
	selfUserID, selfUserName, authErr := fetchAuthIdentity(ctx, token)
	if authErr != nil {
		log.Printf("slack: auth.test failed (non-fatal): %v", authErr)
	} else {
		log.Printf("slack: authenticated as %s (ID: %s)", selfUserName, selfUserID)
	}

	// Try conversations-based approach first (requires channels:read scope).
	// If it fails due to missing scope, fall back to search.messages (requires search:read only).
	channels, err := fetchChannels(ctx, token)
	if err != nil {
		if isMissingScopeError(err) {
			log.Printf("slack: conversations.list missing scope, falling back to search.messages")
			return fetchSlackCandidatesViaSearch(ctx, token)
		}
		return nil, fmt.Errorf("listing channels: %w", err)
	}

	// Log channel type breakdown for diagnostics
	var nPublic, nPrivate, nIM, nMpim int
	for _, ch := range channels {
		switch {
		case ch.IsIM:
			nIM++
		case ch.IsMpim:
			nMpim++
		case ch.Name != "":
			// Heuristic: named channels are public or private
			nPublic++
		default:
			nPrivate++
		}
	}
	log.Printf("slack: %d channels found (public/private=%d, im=%d, mpim=%d, other=%d)",
		len(channels), nPublic, nIM, nMpim, nPrivate)

	// Look back 2 days for recent activity
	cutoff := time.Now().Add(-2 * 24 * time.Hour)
	oldest := strconv.FormatInt(cutoff.Unix(), 10)

	// Filter to channels with recent activity to avoid hammering the API.
	// Slack's updated field is in milliseconds, not seconds.
	cutoffMs := float64(cutoff.UnixMilli())
	var recentChannels []slackChannel
	for _, ch := range channels {
		if ch.Updated >= cutoffMs {
			recentChannels = append(recentChannels, ch)
		}
	}
	log.Printf("slack: %d/%d channels have recent activity (last 2 days)", len(recentChannels), len(channels))

	var candidates []slackCandidate
	for _, ch := range recentChannels {
		displayName := channelDisplayName(ctx, token, ch, selfUserID)

		messages, err := fetchChannelHistory(ctx, token, ch.ID, oldest)
		if err != nil {
			if isMissingScopeError(err) {
				log.Printf("slack: conversations.history missing scope for %s (%s), falling back to search.messages",
					displayName, ch.ID)
				return fetchSlackCandidatesViaSearch(ctx, token)
			}
			// Log individual channel errors instead of silently skipping
			log.Printf("slack: error fetching history for %s (%s): %v", displayName, ch.ID, err)
			continue
		}

		var commitmentCount int
		for _, msg := range messages {
			if msg.Type != "message" || msg.Text == "" {
				continue
			}
			if !hasCommitmentLanguage(msg.Text) {
				continue
			}
			commitmentCount++

			c := slackCandidate{
				Message:   msg.Text,
				Permalink: buildPermalink(ch.ID, msg.TS),
				Channel:   displayName,
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

		if len(messages) > 0 || ch.IsIM || ch.IsMpim {
			log.Printf("slack: %s (%s): %d messages, %d with commitment language",
				displayName, ch.ID, len(messages), commitmentCount)
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
	log.Printf("slack: using search.messages fallback path")

	var allCandidates []slackCandidate

	// Search for commitment phrases in batches — Slack search supports basic query strings
	// We group related phrases to minimize API calls
	searchQueries := []string{
		"i'll",
		"i will",
		"i promise",
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

		var matchCount int
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
			matchCount++

			permalink := match.Permalink
			if permalink == "" {
				permalink = buildPermalink(match.Channel.ID, match.Timestamp)
			}

			channelName := match.Channel.Name
			if channelName == "" {
				channelName = "DM:" + match.Channel.ID
			}

			allCandidates = append(allCandidates, slackCandidate{
				Message:   match.Text,
				Permalink: permalink,
				Channel:   channelName,
				ChannelID: match.Channel.ID,
				Timestamp: match.Timestamp,
			})
		}
		log.Printf("slack: search query %q: %d matches, %d with commitment language",
			query, len(result.Messages.Matches), matchCount)
	}

	log.Printf("slack: search fallback found %d total candidates", len(allCandidates))
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
