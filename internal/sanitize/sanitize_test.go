package sanitize

import "testing"

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain text", "hello world", "hello world"},
		{"CSI color", "\x1b[31mred text\x1b[0m", "red text"},
		{"OSC title set", "\x1b]0;malicious title\x07normal text", "normal text"},
		{"OSC with ST", "\x1b]0;malicious\x1b\\normal", "normal"},
		{"mixed escapes", "\x1b[1m\x1b[31mBold Red\x1b[0m plain", "Bold Red plain"},
		{"cursor movement", "\x1b[2Ahello", "hello"},
		{"no false positive", "normal [text] here", "normal [text] here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
