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

// KnowledgeExtractFn is the function signature for knowledge extraction.
// It is called for each todo with source_context populated after the
// source-context fetch step. The function receives the database, LLM,
// source_ref, source type, content, and existing topic names.
type KnowledgeExtractFn func(ctx context.Context, database *sql.DB, model llm.LLM, sourceRef, sourceType, content string, existingTopics []string) error

// SilenceAnalysisFn is the function signature for silence analysis.
// It is called after knowledge extraction completes (no LLM required).
type SilenceAnalysisFn func(database *sql.DB) error

// Options configures a refresh run.
type Options struct {
	Verbose          bool
	DryRun           bool
	DB               *sql.DB
	Sources          []DataSource
	LLM              llm.LLM // for extraction and suggestions (haiku)
	RoutingLLM       llm.LLM // for routing and validation (sonnet) — falls back to LLM if nil
	ContextRegistry  *ContextRegistry
	KnowledgeExtract KnowledgeExtractFn // knowledge extraction callback (nil = disabled)
	KnowledgeLLM     llm.LLM            // Sonnet for knowledge extraction — falls back to RoutingLLM/LLM if nil
	SilenceAnalysis  SilenceAnalysisFn   // silence alert analysis callback (nil = disabled)
}

// Run performs a full data refresh: iterates DataSources in parallel,
// merges with existing state, runs LLM suggestions, and saves.
func Run(opts Options) error {
	if !opts.Verbose {
		log.SetOutput(os.Stderr)
	}

	auth.LoadEnvFile()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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

	// Track per-source sync results for the cc_source_sync table
	type syncOutcome struct {
		name string
		err  error
	}
	var syncOutcomes []syncOutcome

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
				syncOutcomes = append(syncOutcomes, syncOutcome{name: s.Name(), err: err})
				mu.Unlock()
				return
			}
			mu.Lock()
			results = append(results, result)
			syncOutcomes = append(syncOutcomes, syncOutcome{name: s.Name(), err: nil})
			mu.Unlock()
			if opts.Verbose {
				logSourceResult(s.Name(), result)
			}
		}(src)
	}

	wg.Wait()

	// Record per-source sync status in the database
	if opts.DB != nil {
		for _, outcome := range syncOutcomes {
			if err := db.DBUpsertSourceSync(opts.DB, outcome.name, outcome.err); err != nil {
				log.Printf("warning: recording sync status for %s: %v", outcome.name, err)
			}
		}
	}

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
	if opts.LLM != nil && len(merged.Todos) > 0 {
		suggestions, err := generateSuggestions(ctx, opts.LLM, merged)
		if err != nil {
			log.Printf("suggestion generation: %v", err)
		} else {
			merged.Suggestions = *suggestions
		}
	}

	// Generate proposed prompts for todos that have a source but no prompt yet.
	// Use RoutingLLM (sonnet) if available, otherwise fall back to LLM (haiku).
	routingLLM := opts.RoutingLLM
	if routingLLM == nil {
		routingLLM = opts.LLM
	}
	if routingLLM != nil && len(merged.Todos) > 0 {
		merged.Todos = generateProposedPrompts(ctx, routingLLM, opts.DB, merged.Todos)
	}

	// Dedup pass: process merge suggestions from routing
	if routingLLM != nil && opts.DB != nil && len(merged.Todos) > 0 {
		existingMerges, _ := db.DBLoadMerges(opts.DB)
		merged.Todos = dedupTodos(ctx, routingLLM, opts.DB, merged.Todos, existingMerges)
	}

	// Fetch source context for todos that have a source_ref
	if opts.ContextRegistry != nil && opts.DB != nil {
		for i := range merged.Todos {
			FetchContextBestEffort(ctx, opts.DB, opts.ContextRegistry, &merged.Todos[i])
		}
	}

	// Knowledge extraction pass: process todos with source_context populated.
	// Only processes narrative sources (granola, slack, gmail) -- not calendar or github.
	if opts.KnowledgeExtract != nil && opts.DB != nil {
		knowledgeLLM := opts.KnowledgeLLM
		if knowledgeLLM == nil {
			knowledgeLLM = opts.RoutingLLM
		}
		if knowledgeLLM == nil {
			knowledgeLLM = opts.LLM
		}
		if knowledgeLLM != nil {
			runKnowledgeExtraction(ctx, opts.DB, knowledgeLLM, merged.Todos, opts.KnowledgeExtract, opts.Verbose)
		}
	}

	// Silence analysis pass: runs after extraction, no LLM required.
	// Scans knowledge tables for topics/threads that have gone quiet.
	if opts.SilenceAnalysis != nil && opts.DB != nil {
		if err := opts.SilenceAnalysis(opts.DB); err != nil {
			log.Printf("silence analysis: %v", err)
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
		log.Printf("saved to db (todos=%d, warnings=%d)",
			len(merged.Todos), len(warnings))
	}

	return nil
}

// dedupTodos processes merge suggestions from routing. For each group of todos
// that the LLM flagged as duplicates of an existing todo, it synthesizes a
// combined todo and records the merge relationships.
func dedupTodos(ctx context.Context, l llm.LLM, database *sql.DB, todos []db.Todo, merges []db.TodoMerge) []db.Todo {
	// Build lookup
	todoByID := make(map[string]*db.Todo)
	for i := range todos {
		todoByID[todos[i].ID] = &todos[i]
	}

	// Group merge suggestions: target_id -> []new_originals
	type mergeGroup struct {
		target   *db.Todo
		newTodos []db.Todo
	}
	groups := make(map[string]*mergeGroup)

	for i, t := range todos {
		if t.MergeInto == "" || t.MergeInto == t.ID {
			continue
		}
		target, ok := todoByID[t.MergeInto]
		if !ok {
			continue
		}
		// Veto check — refresh respects vetoes
		if db.WerePreviouslyMergedAndVetoed(merges, t.ID, target.ID) {
			todos[i].MergeInto = ""
			continue
		}
		if _, exists := groups[target.ID]; !exists {
			groups[target.ID] = &mergeGroup{target: target}
		}
		groups[target.ID].newTodos = append(groups[target.ID].newTodos, t)
	}

	for _, g := range groups {
		var originals []db.Todo
		if g.target.Source == "merge" {
			origIDs := db.DBGetOriginalIDs(merges, g.target.ID)
			for _, oid := range origIDs {
				if orig := todoByID[oid]; orig != nil {
					originals = append(originals, *orig)
				}
			}
		} else {
			originals = []db.Todo{*g.target}
		}
		originals = append(originals, g.newTodos...)

		result, err := Synthesize(ctx, l, originals)
		if err != nil {
			log.Printf("dedup synthesis for %q: %v", g.target.ID, err)
			continue
		}
		synth := BuildSynthesisTodo(result, originals, g.target)
		synth.CreatedAt = time.Now()

		if err := db.DBInsertTodo(database, synth); err != nil {
			log.Printf("dedup insert synthesis: %v", err)
			continue
		}
		for _, orig := range originals {
			_ = db.DBInsertMerge(database, synth.ID, orig.ID)
		}
		if g.target.Source == "merge" {
			_ = db.DBDeleteSynthesisMerges(database, g.target.ID)
			_ = db.DBDeleteTodo(database, g.target.ID)
		}

		todos = append(todos, synth)
	}
	return todos
}

// narrativeSources are the source types that contain narrative content
// suitable for knowledge extraction (transcripts, threads, emails).
var narrativeSources = map[string]bool{
	"granola": true,
	"slack":   true,
	"gmail":   true,
}

// runKnowledgeExtraction iterates over todos with source_context populated
// and calls the extraction function for each. Errors are logged but do not
// block other todos or the rest of the pipeline.
func runKnowledgeExtraction(ctx context.Context, database *sql.DB, model llm.LLM, todos []db.Todo, extractFn KnowledgeExtractFn, verbose bool) {
	// Collect existing topic names for dedup guidance.
	existingTopics := loadExistingTopicNames(database)

	processed := 0
	for _, t := range todos {
		if t.SourceContext == "" {
			continue
		}
		if !narrativeSources[t.Source] {
			continue
		}
		if t.SourceRef == "" {
			continue
		}

		if err := extractFn(ctx, database, model, t.SourceRef, t.Source, t.SourceContext, existingTopics); err != nil {
			log.Printf("knowledge extraction for %s/%s: %v", t.Source, t.SourceRef, err)
			continue
		}
		processed++

		// Refresh topic names after each extraction so subsequent extractions
		// can see newly created topics for dedup.
		existingTopics = loadExistingTopicNames(database)
	}

	if verbose && processed > 0 {
		log.Printf("knowledge: extracted artifacts from %d source items", processed)
	}
}

// loadExistingTopicNames queries all topic names from the knowledge_topics table.
// Returns an empty slice if the table doesn't exist or the query fails.
func loadExistingTopicNames(database *sql.DB) []string {
	rows, err := database.Query("SELECT name FROM knowledge_topics")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	return names
}

func logSourceResult(name string, r *SourceResult) {
	if r.Calendar != nil {
		log.Printf("%s: %d today, %d tomorrow", name, len(r.Calendar.Today), len(r.Calendar.Tomorrow))
	}
	if len(r.Todos) > 0 {
		log.Printf("%s: %d todos", name, len(r.Todos))
	}
}
