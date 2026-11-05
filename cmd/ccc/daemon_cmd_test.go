package main

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
)

func TestDaemonRefreshInterval(t *testing.T) {
	off := false
	on := true

	cases := []struct {
		name     string
		enabled  *bool
		interval string
		want     time.Duration
	}{
		{"default when omitted", nil, "", 5 * time.Minute},
		{"configured interval", nil, "10m", 10 * time.Minute},
		{"explicitly enabled", &on, "2m", 2 * time.Minute},
		{"below minimum falls back to default", nil, "30s", 5 * time.Minute},
		{"disabled means no periodic refresh", &off, "5m", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Refresh.Enabled = tc.enabled
			cfg.Daemon.RefreshInterval = tc.interval
			if got := daemonRefreshInterval(cfg); got != tc.want {
				t.Fatalf("daemonRefreshInterval() = %v, want %v", got, tc.want)
			}
		})
	}
}
