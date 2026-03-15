// Package sanitize provides functions to strip potentially dangerous content
// from external data before it enters the system.
package sanitize

import "github.com/charmbracelet/x/ansi"

// StripANSI removes all ANSI escape sequences (CSI, OSC, etc.) from a string.
// This prevents terminal injection attacks via malicious titles, messages, etc.
func StripANSI(s string) string {
	if s == "" {
		return s
	}
	return ansi.Strip(s)
}
