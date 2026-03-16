package refresh

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

// ContextFetcher is implemented by sources that can retrieve raw context
// for a todo's source_ref.
type ContextFetcher interface {
	FetchContext(sourceRef string) (string, error)
	ContextTTL() time.Duration // 0 means immutable
}

// ContextRegistry maps source names to their ContextFetcher implementations.
type ContextRegistry struct {
	fetchers map[string]ContextFetcher
}

// NewContextRegistry creates an empty registry.
func NewContextRegistry() *ContextRegistry {
	return &ContextRegistry{fetchers: make(map[string]ContextFetcher)}
}

// Register adds a fetcher for the given source name.
func (r *ContextRegistry) Register(source string, f ContextFetcher) {
	r.fetchers[source] = f
}

// Get returns the fetcher for the given source, if registered.
func (r *ContextRegistry) Get(source string) (ContextFetcher, bool) {
	f, ok := r.fetchers[source]
	return f, ok
}

// shouldRefresh determines whether a todo's source context needs refreshing.
func shouldRefresh(todo db.Todo, fetcher ContextFetcher) bool {
	if todo.SourceContext == "" {
		return true
	}
	ttl := fetcher.ContextTTL()
	if ttl == 0 {
		return false
	}
	fetchedAt := db.ParseTime(todo.SourceContextAt)
	if fetchedAt.IsZero() {
		return true
	}
	return time.Since(fetchedAt) > ttl
}

// FetchAndSave fetches context for a todo if needed and persists it.
// Returns the context content (possibly cached) and any error.
func FetchAndSave(ctx context.Context, database *sql.DB, registry *ContextRegistry, todo *db.Todo) (string, error) {
	fetcher, ok := registry.Get(todo.Source)
	if !ok {
		return "", nil
	}
	if !shouldRefresh(*todo, fetcher) {
		return todo.SourceContext, nil
	}
	content, err := fetcher.FetchContext(todo.SourceRef)
	if err != nil {
		return "", fmt.Errorf("fetch context for %s/%s: %w", todo.Source, todo.SourceRef, err)
	}
	now := db.FormatTime(time.Now())
	if err := db.DBUpdateTodoSourceContext(database, todo.ID, content, now); err != nil {
		return content, fmt.Errorf("save source context: %w", err)
	}
	todo.SourceContext = content
	todo.SourceContextAt = now
	return content, nil
}

// FetchContextBestEffort fetches and saves context, logging any errors
// instead of returning them.
func FetchContextBestEffort(ctx context.Context, database *sql.DB, registry *ContextRegistry, todo *db.Todo) {
	_, err := FetchAndSave(ctx, database, registry, todo)
	if err != nil {
		log.Printf("source context for %s #%d: %v", todo.Source, todo.DisplayID, err)
	}
}
