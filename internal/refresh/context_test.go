package refresh

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

type mockFetcher struct {
	content string
	err     error
	ttl     time.Duration
}

func (m *mockFetcher) FetchContext(sourceRef string) (string, error) {
	return m.content, m.err
}
func (m *mockFetcher) ContextTTL() time.Duration { return m.ttl }

func TestShouldRefresh_EmptyContext(t *testing.T) {
	todo := db.Todo{SourceContext: ""}
	f := &mockFetcher{ttl: 24 * time.Hour}
	if !shouldRefresh(todo, f) {
		t.Error("expected refresh for empty source_context")
	}
}

func TestShouldRefresh_ImmutableSource(t *testing.T) {
	todo := db.Todo{SourceContext: "content", SourceContextAt: db.FormatTime(time.Now())}
	f := &mockFetcher{ttl: 0}
	if shouldRefresh(todo, f) {
		t.Error("expected no refresh for immutable source with content")
	}
}

func TestShouldRefresh_FreshTTL(t *testing.T) {
	todo := db.Todo{SourceContext: "content", SourceContextAt: db.FormatTime(time.Now())}
	f := &mockFetcher{ttl: 24 * time.Hour}
	if shouldRefresh(todo, f) {
		t.Error("expected no refresh for fresh TTL source")
	}
}

func TestShouldRefresh_StaleTTL(t *testing.T) {
	staleTime := time.Now().Add(-25 * time.Hour)
	todo := db.Todo{SourceContext: "content", SourceContextAt: db.FormatTime(staleTime)}
	f := &mockFetcher{ttl: 24 * time.Hour}
	if !shouldRefresh(todo, f) {
		t.Error("expected refresh for stale TTL source")
	}
}

func TestRegistryLookup(t *testing.T) {
	reg := NewContextRegistry()
	f := &mockFetcher{content: "test"}
	reg.Register("granola", f)

	got, ok := reg.Get("granola")
	if !ok || got != f {
		t.Error("expected to find registered fetcher")
	}

	_, ok = reg.Get("unknown")
	if ok {
		t.Error("expected not found for unregistered source")
	}
}
