package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// SlackSource fetches Slack messages with commitment language and uses LLM to extract todos.
type SlackSource struct {
	LLM llm.LLM
}

// NewSlackSource creates a SlackSource with the given LLM.
func NewSlackSource(l llm.LLM) *SlackSource {
	return &SlackSource{LLM: l}
}

func (s *SlackSource) Name() string  { return "slack" }
func (s *SlackSource) Enabled() bool { return true } // always enabled; auth check in Fetch

func (s *SlackSource) Fetch(ctx context.Context) (*SourceResult, error) {
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
			return &SourceResult{
				Warnings: []db.Warning{{Source: "slack", Message: fmt.Sprintf("LLM extraction failed: %v", err)}},
			}, nil
		}
	}

	return &SourceResult{Todos: todos}, nil
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

func extractSlackCommitments(ctx context.Context, l llm.LLM, candidates []slackCandidate) ([]db.Todo, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("## Message %d (from #%s)\n", i+1, c.Channel))
		sb.WriteString(fmt.Sprintf("Permalink: %s\n", c.Permalink))
		sb.WriteString(fmt.Sprintf("Message: %s\n", c.Message))
		if c.ThreadContext != "" {
			sb.WriteString(fmt.Sprintf("Thread context:\n%s\n", c.ThreadContext))
		}
		sb.WriteString("\n---\n\n")
	}

	prompt := fmt.Sprintf(`You are filtering Slack messages for real commitments the user made. The bar is VERY high.

A message is ONLY a todo if:
1. The user explicitly committed to a specific deliverable (not just participating in conversation)
2. There is a concrete next action with a clear outcome
3. You can write an actionable title starting with a verb (Send, Review, Schedule, Build, Write, Follow up, etc.)

REJECT messages that are:
- Conversational responses ("done", "good process!", "sounds good")
- Observations, tips, shared links, compliments
- Descriptions of past actions ("I just...", "I found that...")
- Vague intentions without a specific deliverable

Use the thread context to understand WHAT was committed to. Build the todo title from the full context, not just the short message.

For each real commitment, return:
- title: Actionable title starting with a verb (20+ chars)
- source_ref: The permalink
- context: Channel name and what area this relates to
- detail: Full context — who was in the conversation, what was discussed, what's expected
- who_waiting: Person(s) waiting on this
- due: YYYY-MM-DD if mentioned, empty string if not

Return ONLY a JSON array. Return [] if no real commitments found. Expect 0-3 results from these %d candidates.

Messages:
%s`, len(candidates), sb.String())

	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("slack commitment extraction: %w", err)
	}

	text = cleanJSON(text)

	var items []struct {
		Title      string `json:"title"`
		SourceRef  string `json:"source_ref"`
		Context    string `json:"context"`
		Detail     string `json:"detail"`
		WhoWaiting string `json:"who_waiting"`
		Due        string `json:"due"`
	}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, fmt.Errorf("parsing slack commitment response: %w (raw: %s)", err, text[:min(200, len(text))])
	}

	var todos []db.Todo
	for _, item := range items {
		todos = append(todos, db.Todo{
			Title:      item.Title,
			Source:     "slack",
			SourceRef:  item.SourceRef,
			Context:    item.Context,
			Detail:     item.Detail,
			WhoWaiting: item.WhoWaiting,
			Due:        item.Due,
			Status:     "active",
		})
	}

	return todos, nil
}
