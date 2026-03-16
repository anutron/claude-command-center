package slack

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ContextFetcherImpl implements refresh.ContextFetcher for Slack conversations.
type ContextFetcherImpl struct {
	botToken string
}

// NewContextFetcher creates a Slack ContextFetcher with the given bot token.
func NewContextFetcher(botToken string) *ContextFetcherImpl {
	return &ContextFetcherImpl{botToken: botToken}
}

// ContextTTL returns 24 hours because Slack conversations are live and evolving.
func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 24 * time.Hour }

// FetchContext retrieves conversation context around a Slack message.
// The sourceRef is a Slack permalink like:
//
//	https://app.slack.com/archives/{channelID}/p{ts_without_dot}
func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
	token := strings.TrimSpace(f.botToken)
	if token == "" {
		return "", fmt.Errorf("slack auth: bot token not configured")
	}

	channelID, ts, err := parseSlackPermalink(sourceRef)
	if err != nil {
		return "", fmt.Errorf("parse permalink: %w", err)
	}

	ctx := context.Background()

	// Fetch messages in a +/-24h window around the target message.
	msgTime, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return "", fmt.Errorf("parse timestamp %q: %w", ts, err)
	}
	oldest := strconv.FormatFloat(msgTime-86400, 'f', 6, 64)
	latest := strconv.FormatFloat(msgTime+86400, 'f', 6, 64)

	params := url.Values{
		"channel": {channelID},
		"oldest":  {oldest},
		"latest":  {latest},
		"limit":   {"100"},
	}

	var histResult slackHistoryResponse
	if err := slackAPIGet(ctx, token, "conversations.history", params, &histResult); err != nil {
		return "", fmt.Errorf("conversations.history: %w", err)
	}
	if !histResult.OK {
		return "", fmt.Errorf("conversations.history error: %s", histResult.Error)
	}

	// Build formatted output: each message plus its thread replies.
	var sb strings.Builder
	for _, msg := range histResult.Messages {
		if msg.Type != "message" || msg.Text == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n", msg.TS, msg.Text))

		// Fetch thread replies for each message.
		replies, err := fetchThreadContext(ctx, token, channelID, msg.TS)
		if err == nil && len(replies) > 1 {
			// First reply is the parent message itself; skip it.
			for _, reply := range replies[1:] {
				sb.WriteString(fmt.Sprintf("  [%s] %s\n", reply.TS, reply.Text))
			}
		}
	}

	return sb.String(), nil
}

// parseSlackPermalink extracts the channel ID and message timestamp from a
// Slack permalink. The expected format is:
//
//	https://app.slack.com/archives/{channelID}/p{ts_without_dot}
//
// The timestamp in the permalink has no dot; this function re-inserts it:
//
//	p1710000000000100 -> 1710000000.000100
func parseSlackPermalink(permalink string) (channelID, ts string, err error) {
	// Strip query parameters and fragments.
	clean := permalink
	if idx := strings.IndexAny(clean, "?#"); idx >= 0 {
		clean = clean[:idx]
	}

	// Also handle https://slack.com/archives/... (without "app." prefix).
	clean = strings.TrimSuffix(clean, "/")

	parts := strings.Split(clean, "/")
	// Expect: https: / / {host} / archives / {channelID} / p{ts}
	// Find "archives" in the path and take the next two segments.
	archivesIdx := -1
	for i, p := range parts {
		if p == "archives" {
			archivesIdx = i
			break
		}
	}
	if archivesIdx < 0 || archivesIdx+2 >= len(parts) {
		return "", "", fmt.Errorf("invalid slack permalink (no archives segment): %s", permalink)
	}

	channelID = parts[archivesIdx+1]
	tsRaw := parts[archivesIdx+2]

	if !strings.HasPrefix(tsRaw, "p") {
		return "", "", fmt.Errorf("invalid slack permalink (timestamp missing p prefix): %s", permalink)
	}
	tsRaw = tsRaw[1:] // strip the "p"

	// Re-insert the dot: "1710000000000100" -> "1710000000.000100"
	// The integer part is 10 digits (Unix seconds), remainder is the fractional part.
	if len(tsRaw) <= 10 {
		return "", "", fmt.Errorf("invalid slack permalink (timestamp too short): %s", permalink)
	}
	ts = tsRaw[:10] + "." + tsRaw[10:]

	return channelID, ts, nil
}
