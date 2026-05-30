package config

import "testing"

// These tests pin the single Slack-token precedence used by BOTH validation
// and the data-fetch paths: config Token > config BotToken > $SLACK_TOKEN >
// $SLACK_BOT_TOKEN. The env-fallback cases are the gap this change closes —
// previously the fetch paths used EffectiveToken(), which ignored the env.

func TestResolveSlackToken_ConfigTokenWins(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "env-token")
	t.Setenv("SLACK_BOT_TOKEN", "env-bot")
	cfg := &Config{Slack: SlackConfig{Token: "cfg-token", BotToken: "cfg-bot"}}
	if got := ResolveSlackToken(cfg); got != "cfg-token" {
		t.Errorf("expected config Token to win, got %q", got)
	}
}

func TestResolveSlackToken_ConfigBotTokenFallback(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "env-token")
	cfg := &Config{Slack: SlackConfig{BotToken: "cfg-bot"}}
	if got := ResolveSlackToken(cfg); got != "cfg-bot" {
		t.Errorf("expected config BotToken when Token empty, got %q", got)
	}
}

func TestResolveSlackToken_EnvSlackToken(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "env-token")
	t.Setenv("SLACK_BOT_TOKEN", "env-bot")
	cfg := &Config{Slack: SlackConfig{}} // no token in config
	if got := ResolveSlackToken(cfg); got != "env-token" {
		t.Errorf("expected $SLACK_TOKEN when config empty, got %q", got)
	}
}

func TestResolveSlackToken_EnvBotTokenFallback(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "")
	t.Setenv("SLACK_BOT_TOKEN", "env-bot")
	cfg := &Config{Slack: SlackConfig{}}
	if got := ResolveSlackToken(cfg); got != "env-bot" {
		t.Errorf("expected $SLACK_BOT_TOKEN fallback, got %q", got)
	}
}

func TestResolveSlackToken_NilConfig(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "env-token")
	if got := ResolveSlackToken(nil); got != "env-token" {
		t.Errorf("expected nil config to fall through to env, got %q", got)
	}
}

func TestResolveSlackToken_AllEmpty(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "")
	t.Setenv("SLACK_BOT_TOKEN", "")
	cfg := &Config{Slack: SlackConfig{}}
	if got := ResolveSlackToken(cfg); got != "" {
		t.Errorf("expected empty string when nothing supplies a token, got %q", got)
	}
}
