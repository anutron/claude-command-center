package calendar

import (
	"testing"
)

func TestMatchesDomain(t *testing.T) {
	tests := []struct {
		email   string
		domains []string
		want    bool
	}{
		{"user@example.com", []string{"example.com"}, true},
		{"user@example.com", []string{"other.com"}, false},
		{"user@example.com", []string{"other.com", "example.com"}, true},
		{"user@sub.example.com", []string{"example.com"}, false},
		{"", []string{"example.com"}, false},
		{"user@example.com", nil, false},
		{"user@example.com", []string{}, false},
	}
	for _, tt := range tests {
		got := matchesDomain(tt.email, tt.domains)
		if got != tt.want {
			t.Errorf("matchesDomain(%q, %v) = %v, want %v", tt.email, tt.domains, got, tt.want)
		}
	}
}
