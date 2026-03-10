package slack

import (
	"testing"
)

func TestHasCommitmentLanguage(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"I'll send you the report", true},
		{"I will handle this", true},
		{"Let me check on that", true},
		{"No commitments here", false},
		{"Just a regular message", false},
		{"I'LL SEND IT", true}, // case insensitive
		{"action item: review PR", true},
		{"", false},
	}
	for _, tt := range tests {
		got := hasCommitmentLanguage(tt.text)
		if got != tt.want {
			t.Errorf("hasCommitmentLanguage(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
