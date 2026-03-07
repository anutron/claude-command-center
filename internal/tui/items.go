package tui

import (
	"fmt"
	"io"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// newItem represents a "new session" entry in the launcher list.
type newItem struct {
	path     string
	label    string
	isHome   bool // replaces isAiron — the "home" project entry
	isBrowse bool
}

func (i newItem) Title() string       { return i.label }
func (i newItem) Description() string  { return i.path }
func (i newItem) FilterValue() string  { return i.label + " " + i.path }

// sessionItem represents a paused/resumable session.
type sessionItem struct {
	session db.Session
}

func (i sessionItem) Title() string {
	return i.session.Repo + " (" + i.session.Branch + ")"
}

func (i sessionItem) Description() string {
	date := i.session.Created.Format("Jan 02 15:04")
	if i.session.Summary != "" {
		return date + " -- " + i.session.Summary
	}
	return date
}

func (i sessionItem) FilterValue() string {
	return i.session.Repo + " " + i.session.Branch + " " + i.session.Summary
}

// itemDelegate renders list items with per-item color styling.
type itemDelegate struct {
	frame  int
	styles *Styles
	grad   *GradientColors
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	selected := index == m.Index()
	width := m.Width() - 4

	var title, desc string
	pointer := "  "
	if selected && d.grad != nil {
		pointer = pulsingPointerStyle(d.grad, d.frame).Render("> ")
	}

	switch it := item.(type) {
	case newItem:
		if it.isHome {
			title = d.styles.TitleBoldC.Render("* " + it.Title())
			desc = "  " + d.styles.DescMuted.Render(it.path)
		} else if it.isBrowse {
			title = d.styles.TitleBoldC.Render("+ " + it.Title())
		} else {
			title = d.styles.TitleBoldW.Render(it.Title())
			desc = "  " + d.styles.DescMuted.Render(it.path)
		}

	case sessionItem:
		repo := d.styles.TitleBoldW.Render(it.session.Repo)
		branch := d.styles.BranchYellow.Render(it.session.Branch)
		title = fmt.Sprintf("%s (%s)", repo, branch)
		desc = "  " + d.styles.DescMuted.Render(it.session.Created.Format("Jan 02")+" -- "+it.session.Summary)

	default:
		title = item.FilterValue()
	}

	line := title + desc
	line = truncate(line, width)

	if selected {
		line = d.styles.SelectedItem.Render(line)
	}

	fmt.Fprintf(w, "%s%s", pointer, line)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	if ansi.StringWidth(s) > max {
		return ansi.Truncate(s, max-1, "...")
	}
	return s
}
