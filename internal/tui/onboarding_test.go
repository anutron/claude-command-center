package tui

import (
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
)

func TestSetOnboarding_EmptyDB(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Empty DB — onboarding should be activatable
	if !db.DBIsEmpty(database) {
		t.Fatal("expected empty DB")
	}

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})
	m.SetOnboarding()

	if !m.onboarding {
		t.Error("expected onboarding to be true after SetOnboarding")
	}
	if m.onboardingState == nil {
		t.Fatal("expected onboardingState to be initialized")
	}
	if m.onboardingState.step != stepWelcome {
		t.Errorf("expected initial step to be stepWelcome, got %d", m.onboardingState.step)
	}
}

func TestSetOnboarding_NotTriggeredWithData(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert data — simulates a non-first-run scenario
	db.DBInsertTodo(database, db.Todo{
		ID: "existing", Title: "Existing task", Status: db.StatusBacklog, Source: "manual",
	})

	if db.DBIsEmpty(database) {
		t.Fatal("expected non-empty DB after insert")
	}

	// Model created without SetOnboarding — verifies the decision logic
	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})
	if m.onboarding {
		t.Error("expected onboarding to be false when not explicitly set")
	}
	if m.onboardingState != nil {
		t.Error("expected onboardingState to be nil when onboarding not set")
	}
}

func TestOnboardingState_StepConstants(t *testing.T) {
	// Verify step ordering is well-defined
	if stepWelcome >= stepPalette {
		t.Error("stepWelcome should be less than stepPalette")
	}
	if stepPalette >= stepSources {
		t.Error("stepPalette should be less than stepSources")
	}
	if stepSources >= stepSourceDetail {
		t.Error("stepSources should be less than stepSourceDetail")
	}
	if stepSourceDetail >= stepDone {
		t.Error("stepSourceDetail should be less than stepDone")
	}
}

func TestNewOnboardingState_InitializesSources(t *testing.T) {
	cfg := &config.Config{
		Name:     "Test",
		Palette:  "aurora",
		Calendar: config.CalendarConfig{Enabled: true},
		GitHub:   config.GitHubConfig{Enabled: false},
		Granola:  config.GranolaConfig{Enabled: true},
		Slack:    config.SlackConfig{Enabled: false},
	}

	o := newOnboardingState(cfg)

	if len(o.sources) != 4 {
		t.Fatalf("expected 4 sources, got %d", len(o.sources))
	}

	// Calendar should be enabled (from config)
	if o.sources[0].slug != "calendar" || !o.sources[0].enabled {
		t.Errorf("expected calendar enabled, got slug=%q enabled=%v", o.sources[0].slug, o.sources[0].enabled)
	}
	// GitHub should be disabled (from config)
	if o.sources[1].slug != "github" || o.sources[1].enabled {
		t.Errorf("expected github disabled, got slug=%q enabled=%v", o.sources[1].slug, o.sources[1].enabled)
	}
	// Granola should be enabled (from config)
	if o.sources[2].slug != "granola" || !o.sources[2].enabled {
		t.Errorf("expected granola enabled, got slug=%q enabled=%v", o.sources[2].slug, o.sources[2].enabled)
	}
	// Slack should be disabled (from config)
	if o.sources[3].slug != "slack" || o.sources[3].enabled {
		t.Errorf("expected slack disabled, got slug=%q enabled=%v", o.sources[3].slug, o.sources[3].enabled)
	}
}

func TestNewOnboardingState_NameFromConfig(t *testing.T) {
	cfg := &config.Config{
		Name:     "My Dashboard",
		Subtitle: "v2",
		Palette:  "aurora",
	}

	o := newOnboardingState(cfg)

	if o.nameInput.Value() != "My Dashboard" {
		t.Errorf("expected name input 'My Dashboard', got %q", o.nameInput.Value())
	}
	if o.subtitleInput.Value() != "v2" {
		t.Errorf("expected subtitle input 'v2', got %q", o.subtitleInput.Value())
	}
}

func TestOnboardingState_TotalSourceItems(t *testing.T) {
	cfg := &config.Config{
		Name:    "Test",
		Palette: "aurora",
	}

	o := newOnboardingState(cfg)

	// 4 sources + 1 "Continue" button
	if o.totalSourceItems() != 5 {
		t.Errorf("expected 5 total source items, got %d", o.totalSourceItems())
	}
}

func TestOnboardingState_FindSource(t *testing.T) {
	cfg := &config.Config{
		Name:    "Test",
		Palette: "aurora",
	}

	o := newOnboardingState(cfg)

	cal := o.findSource("calendar")
	if cal == nil {
		t.Fatal("expected to find calendar source")
	}
	if cal.name != "Google Calendar" {
		t.Errorf("expected name 'Google Calendar', got %q", cal.name)
	}

	missing := o.findSource("nonexistent")
	if missing != nil {
		t.Error("expected nil for nonexistent source")
	}
}
