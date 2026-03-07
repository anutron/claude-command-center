package plugin

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// stubPlugin is a minimal Plugin implementation for testing.
type stubPlugin struct {
	slug    string
	tabName string
}

func (s *stubPlugin) Slug() string                                          { return s.slug }
func (s *stubPlugin) TabName() string                                       { return s.tabName }
func (s *stubPlugin) Init(ctx Context) error                                { return nil }
func (s *stubPlugin) Shutdown()                                             {}
func (s *stubPlugin) Migrations() []Migration                               { return nil }
func (s *stubPlugin) View(width, height, frame int) string                  { return "" }
func (s *stubPlugin) KeyBindings() []KeyBinding                             { return nil }
func (s *stubPlugin) HandleKey(msg tea.KeyMsg) Action                       { return NoopAction() }
func (s *stubPlugin) HandleMessage(msg tea.Msg) (bool, Action)             { return false, NoopAction() }
func (s *stubPlugin) Routes() []Route                                       { return nil }
func (s *stubPlugin) NavigateTo(route string, args map[string]string)       {}
func (s *stubPlugin) RefreshInterval() time.Duration                        { return 0 }
func (s *stubPlugin) Refresh() tea.Cmd                                      { return nil }

func TestRegistryRegisterAndAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubPlugin{slug: "a", tabName: "A"})
	r.Register(&stubPlugin{slug: "b", tabName: "B"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(all))
	}
	if all[0].Slug() != "a" {
		t.Errorf("expected first slug 'a', got %q", all[0].Slug())
	}
	if all[1].Slug() != "b" {
		t.Errorf("expected second slug 'b', got %q", all[1].Slug())
	}
}

func TestRegistryBySlug(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubPlugin{slug: "sessions", tabName: "Sessions"})

	p, ok := r.BySlug("sessions")
	if !ok {
		t.Fatal("expected to find 'sessions'")
	}
	if p.TabName() != "Sessions" {
		t.Errorf("expected TabName 'Sessions', got %q", p.TabName())
	}

	_, ok = r.BySlug("nonexistent")
	if ok {
		t.Error("expected false for nonexistent slug")
	}
}

func TestRegistryCount(t *testing.T) {
	r := NewRegistry()
	if r.Count() != 0 {
		t.Errorf("expected 0, got %d", r.Count())
	}

	r.Register(&stubPlugin{slug: "x"})
	if r.Count() != 1 {
		t.Errorf("expected 1, got %d", r.Count())
	}
}

func TestRegistryDuplicateSlug(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubPlugin{slug: "dup", tabName: "Original"})
	r.Register(&stubPlugin{slug: "dup", tabName: "Replacement"})

	if r.Count() != 1 {
		t.Errorf("expected 1 plugin after duplicate, got %d", r.Count())
	}
	p, _ := r.BySlug("dup")
	if p.TabName() != "Replacement" {
		t.Errorf("expected 'Replacement', got %q", p.TabName())
	}
}

func TestRegistryIndexOf(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubPlugin{slug: "a"})
	r.Register(&stubPlugin{slug: "b"})
	r.Register(&stubPlugin{slug: "c"})

	if r.IndexOf("a") != 0 {
		t.Errorf("expected index 0, got %d", r.IndexOf("a"))
	}
	if r.IndexOf("c") != 2 {
		t.Errorf("expected index 2, got %d", r.IndexOf("c"))
	}
	if r.IndexOf("z") != -1 {
		t.Errorf("expected -1 for unknown, got %d", r.IndexOf("z"))
	}
}
