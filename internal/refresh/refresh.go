package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Options configures a refresh run.
type Options struct {
	Verbose            bool
	NoLLM              bool
	DryRun             bool
	GitHubRepos        []string
	GitHubUsername     string
	CalendarIDs        []string
	AutoAcceptDomains  []string
	StateDir           string
}

// Run performs a full data refresh: loads auth, fetches data in parallel,
// runs LLM extraction, merges with existing state, and saves.
func Run(opts Options) error {
	if !opts.Verbose {
		log.SetOutput(os.Stderr)
	}

	loadEnvFile()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stateDir := opts.StateDir
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".config", "ccc", "data")
	}
	ccPath := filepath.Join(stateDir, "command-center.json")
	os.MkdirAll(stateDir, 0o755)

	existing, err := LoadCommandCenter(ccPath)
	if err != nil {
		log.Printf("warning: loading existing state: %v", err)
	}

	if err := migrateCalendarCredentials(); err != nil {
		log.Printf("calendar credential migration: %v", err)
	}

	var (
		calTS        oauth2.TokenSource
		gmailTS      oauth2.TokenSource
		slackToken   string
		granolaToken string
		warnings     []Warning
	)

	calTS, err = loadCalendarAuth()
	if err != nil {
		log.Printf("calendar auth: %v", err)
		warnings = append(warnings, Warning{Source: "calendar", Message: err.Error(), At: time.Now()})
	}

	gmailTS, err = loadGmailAuth()
	if err != nil {
		log.Printf("gmail auth: %v", err)
		warnings = append(warnings, Warning{Source: "gmail", Message: err.Error(), At: time.Now()})
	}

	slackToken, err = loadSlackToken()
	if err != nil {
		log.Printf("slack auth: %v", err)
		warnings = append(warnings, Warning{Source: "slack", Message: err.Error(), At: time.Now()})
	}

	granolaToken, err = loadGranolaAuth()
	if err != nil {
		log.Printf("granola auth: %v", err)
		warnings = append(warnings, Warning{Source: "granola", Message: err.Error(), At: time.Now()})
	}

	if !opts.NoLLM {
		if _, err := exec.LookPath("claude"); err != nil {
			log.Printf("claude CLI not found — LLM features disabled")
			opts.NoLLM = true
		}
	}

	var (
		mu              sync.Mutex
		calData         *CalendarData
		gmailThreads    []Thread
		ghThreads       []Thread
		slackCandidates []slackCandidate
		meetings        []RawMeeting
	)

	var wg sync.WaitGroup

	if calTS != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			calendarIDs := opts.CalendarIDs
			if len(calendarIDs) == 0 {
				calendarIDs = []string{"primary"}
			}
			data, err := fetchCalendarEvents(ctx, calTS, calendarIDs)
			if err != nil {
				log.Printf("calendar fetch: %v", err)
				mu.Lock()
				warnings = append(warnings, Warning{Source: "calendar", Message: fmt.Sprintf("fetch failed: %v", err), At: time.Now()})
				mu.Unlock()
				return
			}
			if len(opts.AutoAcceptDomains) > 0 {
				autoAccept(ctx, calTS, opts.AutoAcceptDomains)
			}
			mu.Lock()
			calData = data
			mu.Unlock()
			if opts.Verbose {
				log.Printf("calendar: %d today, %d tomorrow", len(data.Today), len(data.Tomorrow))
			}
		}()
	}

	if gmailTS != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			threads, err := fetchActionableEmails(ctx, gmailTS)
			if err != nil {
				log.Printf("gmail fetch: %v", err)
				mu.Lock()
				warnings = append(warnings, Warning{Source: "gmail", Message: fmt.Sprintf("fetch failed: %v", err), At: time.Now()})
				mu.Unlock()
				return
			}
			mu.Lock()
			gmailThreads = threads
			mu.Unlock()
			if opts.Verbose {
				log.Printf("gmail: %d actionable emails", len(threads))
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		threads, err := fetchGitHubThreads(ctx, opts.GitHubRepos)
		if err != nil {
			log.Printf("github fetch: %v", err)
			mu.Lock()
			warnings = append(warnings, Warning{Source: "github", Message: fmt.Sprintf("fetch failed: %v", err), At: time.Now()})
			mu.Unlock()
			return
		}
		mu.Lock()
		ghThreads = threads
		mu.Unlock()
		if opts.Verbose {
			log.Printf("github: %d PR threads", len(threads))
		}
	}()

	if slackToken != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidates, err := fetchSlackCandidates(ctx, slackToken)
			if err != nil {
				log.Printf("slack fetch: %v", err)
				mu.Lock()
				warnings = append(warnings, Warning{Source: "slack", Message: fmt.Sprintf("fetch failed: %v", err), At: time.Now()})
				mu.Unlock()
				return
			}
			mu.Lock()
			slackCandidates = candidates
			mu.Unlock()
			if opts.Verbose {
				log.Printf("slack: %d candidates with commitment language", len(candidates))
			}
		}()
	}

	if granolaToken != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := fetchGranolaMeetings(ctx, granolaToken)
			if err != nil {
				log.Printf("granola fetch: %v", err)
				mu.Lock()
				warnings = append(warnings, Warning{Source: "granola", Message: fmt.Sprintf("fetch failed: %v", err), At: time.Now()})
				mu.Unlock()
				return
			}
			mu.Lock()
			meetings = m
			mu.Unlock()
			if opts.Verbose {
				log.Printf("granola: %d meetings", len(m))
			}
		}()
	}

	wg.Wait()

	var granolaTodos, slackTodos []Todo
	if !opts.NoLLM {
		var llmWg sync.WaitGroup

		if len(meetings) > 0 {
			llmWg.Add(1)
			go func() {
				defer llmWg.Done()
				todos, err := extractCommitments(ctx, meetings)
				if err != nil {
					log.Printf("commitment extraction: %v", err)
					return
				}
				mu.Lock()
				granolaTodos = todos
				mu.Unlock()
				if opts.Verbose {
					log.Printf("llm: extracted %d commitments from meetings", len(todos))
				}
			}()
		}
		if len(slackCandidates) > 0 {
			llmWg.Add(1)
			go func() {
				defer llmWg.Done()
				todos, err := extractSlackCommitments(ctx, slackCandidates)
				if err != nil {
					log.Printf("slack commitment extraction: %v", err)
					return
				}
				mu.Lock()
				slackTodos = todos
				mu.Unlock()
				if opts.Verbose {
					log.Printf("llm: extracted %d commitments from %d slack candidates", len(todos), len(slackCandidates))
				}
			}()
		}

		llmWg.Wait()
	}

	if calData == nil {
		calData = &CalendarData{}
	}
	var allTodos []Todo
	allTodos = append(allTodos, granolaTodos...)
	allTodos = append(allTodos, slackTodos...)
	allThreads := append(gmailThreads, ghThreads...)

	fresh := &FreshData{
		Calendar: *calData,
		Todos:    allTodos,
		Threads:  allThreads,
	}

	merged := Merge(existing, fresh)

	if calTS != nil {
		executePendingActions(ctx, calTS, merged)
	}

	if !opts.NoLLM && (len(merged.Todos) > 0 || len(merged.Threads) > 0) {
		suggestions, err := generateSuggestions(ctx, merged)
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

	if err := SaveCommandCenter(ccPath, merged); err != nil {
		return fmt.Errorf("saving command center: %w", err)
	}

	if opts.Verbose {
		log.Printf("wrote %s (todos=%d, threads=%d, warnings=%d)",
			ccPath, len(merged.Todos), len(merged.Threads), len(warnings))
	}

	return nil
}
