package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionType distinguishes how a session should be resumed.
type SessionType int

const (
	SessionWinddown SessionType = iota
	SessionBookmark
)

// Session represents a resumable Claude Code session (bookmark or winddown).
type Session struct {
	Filename  string
	Project   string
	Repo      string
	Branch    string
	Created   time.Time
	Summary   string
	Type      SessionType
	SessionID string // Claude Code session UUID (bookmarks only)
}

// Bookmark is the JSON structure stored in bookmarks.json.
type Bookmark struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Label     string `json:"label"`
	Summary   string `json:"summary"`
	Created   string `json:"created"`
}

// ---------------------------------------------------------------------------
// Session parsing
// ---------------------------------------------------------------------------

func ParseSessionFile(path string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}

	lines := strings.Split(string(data), "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Session{}, fmt.Errorf("no frontmatter in %s", path)
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return Session{}, fmt.Errorf("unclosed frontmatter in %s", path)
	}

	s := Session{Filename: filepath.Base(path)}
	for _, line := range lines[1:endIdx] {
		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "project":
			s.Project = val
		case "repo":
			s.Repo = val
		case "branch":
			s.Branch = val
		case "summary":
			s.Summary = val
		case "created":
			t, err := time.Parse("2006-01-02T15:04:05", val)
			if err != nil {
				t, err = time.Parse(time.RFC3339, val)
				if err != nil {
					return Session{}, fmt.Errorf("bad created time %q: %w", val, err)
				}
			}
			s.Created = t
		}
	}
	return s, nil
}

func LoadWinddownSessions(sessionsDir string) ([]Session, error) {
	entries, err := os.ReadDir(sessionsDir)
	if os.IsNotExist(err) {
		return []Session{}, nil
	}
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		s, err := ParseSessionFile(filepath.Join(sessionsDir, e.Name()))
		if err != nil {
			continue
		}
		s.Type = SessionWinddown
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}

func LoadBookmarks(bookmarksFile string) ([]Session, error) {
	data, err := os.ReadFile(bookmarksFile)
	if os.IsNotExist(err) {
		return []Session{}, nil
	}
	if err != nil {
		return nil, err
	}

	var bookmarks []Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return nil, fmt.Errorf("parse bookmarks: %w", err)
	}

	sessions := make([]Session, 0, len(bookmarks))
	for _, b := range bookmarks {
		t, _ := time.Parse(time.RFC3339, b.Created)
		sessions = append(sessions, Session{
			SessionID: b.SessionID,
			Project:   b.Project,
			Repo:      b.Repo,
			Branch:    b.Branch,
			Created:   t,
			Summary:   b.Summary,
			Type:      SessionBookmark,
		})
	}
	return sessions, nil
}

func LoadAllSessions(sessionsDir, bookmarksFile string) ([]Session, error) {
	winddowns, err := LoadWinddownSessions(sessionsDir)
	if err != nil {
		return nil, err
	}
	bookmarks, err := LoadBookmarks(bookmarksFile)
	if err != nil {
		bookmarks = []Session{}
	}

	all := append(winddowns, bookmarks...)
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].Created.After(all[i].Created) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	return all, nil
}

func RemoveBookmark(bookmarksFile, sessionID string) error {
	data, err := os.ReadFile(bookmarksFile)
	if err != nil {
		return err
	}

	var bookmarks []Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return err
	}

	filtered := make([]Bookmark, 0, len(bookmarks))
	for _, b := range bookmarks {
		if b.SessionID != sessionID {
			filtered = append(filtered, b)
		}
	}

	out, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bookmarksFile, out, 0o644)
}
