package automation

import (
	"testing"
	"time"
)

func TestIsDue_EveryRefresh(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.Local)
	lastRun := now.Add(-1 * time.Minute)

	if !isDue("every_refresh", lastRun, now) {
		t.Error("every_refresh should always be due")
	}
	if !isDue("every_refresh", time.Time{}, now) {
		t.Error("every_refresh should be due even with zero lastRun")
	}
}

func TestIsDue_Daily(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 3, 19, 14, 0, 0, 0, loc) // Thursday 2pm

	tests := []struct {
		name    string
		lastRun time.Time
		want    bool
	}{
		{
			name:    "never run (zero time)",
			lastRun: time.Time{},
			want:    true,
		},
		{
			name:    "ran yesterday",
			lastRun: time.Date(2026, 3, 18, 20, 0, 0, 0, loc),
			want:    true,
		},
		{
			name:    "ran today at midnight",
			lastRun: time.Date(2026, 3, 19, 0, 0, 0, 0, loc),
			want:    false,
		},
		{
			name:    "ran today earlier",
			lastRun: time.Date(2026, 3, 19, 8, 0, 0, 0, loc),
			want:    false,
		},
		{
			name:    "ran just before midnight",
			lastRun: time.Date(2026, 3, 18, 23, 59, 59, 0, loc),
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDue("daily", tt.lastRun, now)
			if got != tt.want {
				t.Errorf("isDue(daily, %v, %v) = %v, want %v", tt.lastRun, now, got, tt.want)
			}
		})
	}
}

func TestIsDue_Daily9am(t *testing.T) {
	loc := time.Local

	tests := []struct {
		name    string
		lastRun time.Time
		now     time.Time
		want    bool
	}{
		{
			name:    "before 9am today, never run",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 19, 8, 30, 0, 0, loc),
			want:    false,
		},
		{
			name:    "at 9am today, never run",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 19, 9, 0, 0, 0, loc),
			want:    true,
		},
		{
			name:    "after 9am today, ran yesterday",
			lastRun: time.Date(2026, 3, 18, 9, 0, 0, 0, loc),
			now:     time.Date(2026, 3, 19, 10, 0, 0, 0, loc),
			want:    true,
		},
		{
			name:    "after 9am today, already ran today at 9am",
			lastRun: time.Date(2026, 3, 19, 9, 0, 0, 0, loc),
			now:     time.Date(2026, 3, 19, 14, 0, 0, 0, loc),
			want:    false,
		},
		{
			name:    "after 9am today, ran today at 9:01",
			lastRun: time.Date(2026, 3, 19, 9, 1, 0, 0, loc),
			now:     time.Date(2026, 3, 19, 14, 0, 0, 0, loc),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDue("daily_9am", tt.lastRun, tt.now)
			if got != tt.want {
				t.Errorf("isDue(daily_9am, %v, %v) = %v, want %v", tt.lastRun, tt.now, got, tt.want)
			}
		})
	}
}

func TestIsDue_WeeklyFriday(t *testing.T) {
	loc := time.Local

	tests := []struct {
		name    string
		lastRun time.Time
		now     time.Time
		want    bool
	}{
		{
			name:    "it is Friday, never run",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 20, 10, 0, 0, 0, loc), // Friday
			want:    true,
		},
		{
			name:    "it is Friday, ran last Friday",
			lastRun: time.Date(2026, 3, 13, 10, 0, 0, 0, loc),
			now:     time.Date(2026, 3, 20, 10, 0, 0, 0, loc),
			want:    true,
		},
		{
			name:    "it is Friday, already ran today",
			lastRun: time.Date(2026, 3, 20, 8, 0, 0, 0, loc),
			now:     time.Date(2026, 3, 20, 10, 0, 0, 0, loc),
			want:    false,
		},
		{
			name:    "it is Thursday, not Friday",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 19, 10, 0, 0, 0, loc), // Thursday
			want:    false,
		},
		{
			name:    "it is Saturday, not Friday",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 21, 10, 0, 0, 0, loc), // Saturday
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDue("weekly_friday", tt.lastRun, tt.now)
			if got != tt.want {
				t.Errorf("isDue(weekly_friday, %v, %v) = %v, want %v", tt.lastRun, tt.now, got, tt.want)
			}
		})
	}
}

func TestIsDue_WeeklyMonday(t *testing.T) {
	loc := time.Local

	tests := []struct {
		name    string
		lastRun time.Time
		now     time.Time
		want    bool
	}{
		{
			name:    "it is Monday, never run",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 23, 10, 0, 0, 0, loc), // Monday
			want:    true,
		},
		{
			name:    "it is Monday, ran last Monday",
			lastRun: time.Date(2026, 3, 16, 10, 0, 0, 0, loc),
			now:     time.Date(2026, 3, 23, 10, 0, 0, 0, loc),
			want:    true,
		},
		{
			name:    "it is Monday, already ran today",
			lastRun: time.Date(2026, 3, 23, 8, 0, 0, 0, loc),
			now:     time.Date(2026, 3, 23, 10, 0, 0, 0, loc),
			want:    false,
		},
		{
			name:    "it is Tuesday, not Monday",
			lastRun: time.Time{},
			now:     time.Date(2026, 3, 24, 10, 0, 0, 0, loc), // Tuesday
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDue("weekly_monday", tt.lastRun, tt.now)
			if got != tt.want {
				t.Errorf("isDue(weekly_monday, %v, %v) = %v, want %v", tt.lastRun, tt.now, got, tt.want)
			}
		})
	}
}

func TestIsDue_UnknownSchedule(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.Local)
	if isDue("banana", time.Time{}, now) {
		t.Error("unknown schedule should return false")
	}
	if isDue("", time.Time{}, now) {
		t.Error("empty schedule should return false")
	}
}

func TestIsDue_MidnightBoundary(t *testing.T) {
	loc := time.Local
	// Run at 11:59:59 PM, check at 12:00:00 AM (next day).
	lastRun := time.Date(2026, 3, 18, 23, 59, 59, 0, loc)
	now := time.Date(2026, 3, 19, 0, 0, 0, 0, loc)

	if !isDue("daily", lastRun, now) {
		t.Error("daily should be due when crossing midnight boundary")
	}
}
