package granola

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ContextFetcherImpl implements refresh.ContextFetcher for Granola meeting transcripts.
type ContextFetcherImpl struct{}

// NewContextFetcher creates a Granola ContextFetcher.
func NewContextFetcher() *ContextFetcherImpl {
	return &ContextFetcherImpl{}
}

// ContextTTL returns 0 because meeting transcripts are immutable.
func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 0 }

// FetchContext retrieves the transcript for a Granola meeting.
// The sourceRef format is "{meeting_id}-{title_hash}"; the meeting ID is
// everything before the last dash.
func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
	lastDash := strings.LastIndex(sourceRef, "-")
	if lastDash < 0 {
		return "", fmt.Errorf("invalid granola source_ref: %s", sourceRef)
	}
	meetingID := sourceRef[:lastDash]

	token, err := loadGranolaAuth()
	if err != nil {
		return "", fmt.Errorf("granola auth: %w", err)
	}

	ctx := context.Background()
	transcript, err := granolaGetTranscript(ctx, token, meetingID)
	if err != nil {
		return "", fmt.Errorf("fetch transcript: %w", err)
	}

	return transcript, nil
}
