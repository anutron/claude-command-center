package ui

import "strings"

// WrapText wraps text at word boundaries to fit within maxWidth columns.
// It preserves existing newlines and handles empty paragraphs.
func WrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			if len(current)+1+len(word) > maxWidth {
				lines = append(lines, current)
				current = word
			} else {
				current += " " + word
			}
		}
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

// TruncateToWidth truncates a string to maxWidth runes, appending "~" if truncated.
// Returns empty string if maxWidth <= 0.
func TruncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "~"
}

// FlattenTitle collapses newlines and multiple spaces in a string to single spaces.
func FlattenTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
