package logger

import (
	"fmt"
	"strings"
)

// SanitizeForLog escapes control characters in a string to prevent log injection attacks.
// It preserves Unicode characters (accented chars, emoji, CJK, etc.) while escaping:
// - Newlines (\n, \r) that could create fake log entries
// - Tabs (\t) that could misalign log output
// - Null bytes (\x00) that could truncate log entries
// - ANSI escape codes (\x1b) that could manipulate terminal output
// - Other control characters (< 32, 127) as hex escapes
func SanitizeForLog(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\x00':
			result.WriteString("\\x00")
		default:
			if r < 32 || r == 127 || r == '\x1b' {
				result.WriteString(fmt.Sprintf("\\x%02x", r))
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}
