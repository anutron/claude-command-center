package refresh

import (
	"strings"
	"testing"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestBuildSynthesisPrompt(t *testing.T) {
	originals := []db.Todo{
		{ID: "a", DisplayID: 12, Title: "Do X", Source: "manual"},
		{ID: "b", DisplayID: 14, Title: "Do X for Carol by Monday", Source: "slack",
			Due: "2026-03-24", WhoWaiting: "Carol"},
	}

	prompt := buildSynthesisPrompt(originals)

	if !strings.Contains(prompt, "#12") {
		t.Error("prompt should reference display IDs")
	}
	if !strings.Contains(prompt, "Do X for Carol") {
		t.Error("prompt should contain original titles")
	}
	if !strings.Contains(prompt, "newest entry is the source of truth") {
		t.Error("prompt should instruct newest-wins on overlap")
	}
}

func TestParseSynthesisResult(t *testing.T) {
	raw := `{"title":"Do X for Carol","due":"2026-03-24","who_waiting":"Carol","detail":"Combined from manual + slack","context":"","effort":""}`
	result, err := parseSynthesisResult(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Do X for Carol" {
		t.Errorf("expected title 'Do X for Carol', got %q", result.Title)
	}
	if result.Due != "2026-03-24" {
		t.Errorf("expected due '2026-03-24', got %q", result.Due)
	}
}
