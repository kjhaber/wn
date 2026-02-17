package wn

import (
	"strings"
)

// FirstLine returns the first line of s, trimmed. Use for compact display (e.g. wn list).
// Descriptions can use a git-commit style: first line is a short summary, rest is detail.
func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
