package refresh

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/auth"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// Options configures a refresh run.
type Options struct {
	Verbose bool
	DryRun  bool
	DB      *sql.DB
	Sources []DataSource
	LLM     llm.LLM // for suggestions (post-merge)
}

// Run performs a full data refresh: iterates DataSources in parallel,
// merges with existing state, runs LLM suggestions, and saves.
func Run(opts Options) error {
	if !opts.Verbose {
		log.SetOutput(os.Stderr)
	}

	auth.LoadEnvFile()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	existing, err := db.LoadCommandCenterFromDB(opts.DB)
	if err != nil {
		log.Printf("warning: loading existing state: %v", err)
	}

	// Fetch from all enabled sources in parallel
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		results  []*SourceResult
		warnings []db.Warning
	)

	for _, src := range opts.Sources {
		if !src.Enabled() {
			continue
		}
		wg.Add(1)
		go func(s DataSource) {
			defer wg.Done()
			result, err := s.Fetch(ctx)
			if err != nil {
				log.Printf("%s: %v", s.Name(), err)
				mu.Lock()
				warnings = append(warnings, db.Warning{
					Source:  s.Name(),
					Message: err.Error(),
					At:      time.Now(),
				})
				mu.Unlock()
				return
			}
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			if opts.Verbose {
				logSourceResult(s.Name(), result)
			}
		}(src)
	}

	wg.Wait()

	fresh := combineResults(results)
	// Collect warnings from successful source results
	for _, r := range results {
		if r != nil && len(r.Warnings) > 0 {
			warnings = append(warnings, r.Warnings...)
		}
	}

	merged := Merge(existing, fresh)

	// Execute post-merge hooks
	for _, src := range opts.Sources {
		if pm, ok := src.(PostMerger); ok {
			if err := pm.PostMerge(ctx, opts.DB, merged, opts.Verbose); err != nil {
				log.Printf("%s post-merge: %v", src.Name(), err)
			}
		}
	}

	// Generate suggestions if LLM is available
	if opts.LLM != nil && (len(merged.Todos) > 0 || len(merged.Threads) > 0) {
		suggestions, err := generateSuggestions(ctx, opts.LLM, merged)
		if err != nil {
			log.Printf("suggestion generation: %v", err)
		} else {
			merged.Suggestions = *suggestions
		}
	}

	merged.Warnings = warnings
	merged.GeneratedAt = time.Now()

	if opts.DryRun {
		data, _ := json.MarshalIndent(merged, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if err := db.DBSaveRefreshResult(opts.DB, merged); err != nil {
		return fmt.Errorf("saving command center: %w", err)
	}

	if opts.Verbose {
		log.Printf("saved to db (todos=%d, threads=%d, warnings=%d)",
			len(merged.Todos), len(merged.Threads), len(warnings))
	}

	return nil
}

func logSourceResult(name string, r *SourceResult) {
	if r.Calendar != nil {
		log.Printf("%s: %d today, %d tomorrow", name, len(r.Calendar.Today), len(r.Calendar.Tomorrow))
	}
	if len(r.Todos) > 0 {
		log.Printf("%s: %d todos", name, len(r.Todos))
	}
	if len(r.Threads) > 0 {
		log.Printf("%s: %d threads", name, len(r.Threads))
	}
}
