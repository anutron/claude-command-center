package calendar

import (
	"testing"
	"time"
)

func TestParseDateTime(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		// Standard RFC3339
		{"2026-04-11T10:00:00Z", false},
		{"2026-04-11T10:00:00-07:00", false},
		{"2026-04-11T10:00:00+05:30", false},
		// Fractional seconds (some Google Calendar sources)
		{"2026-04-11T10:00:00.000Z", false},
		{"2026-04-11T10:00:00.123456789-07:00", false},
		// No timezone (Exchange-synced / iCal subscriptions)
		{"2026-04-11T10:00:00", false},
		// Invalid
		{"", true},
		{"2026-04-11", true},
		{"not-a-time", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDateTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDateTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got.IsZero() {
				t.Errorf("parseDateTime(%q) returned zero time", tt.input)
			}
			if !tt.wantErr {
				// All inputs represent 10:00 on April 11
				if got.Hour() != 10 || got.Minute() != 0 {
					// Only check for inputs that don't have a timezone offset
					// (UTC-parsed ones will show 10:00, offset ones will differ)
					if tt.input == "2026-04-11T10:00:00" || tt.input == "2026-04-11T10:00:00Z" || tt.input == "2026-04-11T10:00:00.000Z" {
						utc := got.UTC()
						if utc.Hour() != 10 || utc.Minute() != 0 {
							t.Errorf("parseDateTime(%q) = %v, expected 10:00 UTC", tt.input, got)
						}
					}
				}
			}
		})
	}
}

func TestParseDateTimePreservesTimezone(t *testing.T) {
	// An event at 10am Pacific (-07:00) should be 17:00 UTC
	got, err := parseDateTime("2026-04-11T10:00:00-07:00")
	if err != nil {
		t.Fatal(err)
	}
	if got.UTC().Hour() != 17 {
		t.Errorf("expected 17:00 UTC, got %v", got.UTC())
	}
}

func TestParseDateTimeNoTimezoneDefaultsUTC(t *testing.T) {
	// Bare datetime without timezone — should parse (not return zero)
	got, err := parseDateTime("2026-04-11T10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsZero() {
		t.Fatal("expected non-zero time")
	}
	// time.Parse with no timezone returns time in UTC
	if got.Location() != time.UTC {
		t.Errorf("expected UTC location, got %v", got.Location())
	}
}

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
