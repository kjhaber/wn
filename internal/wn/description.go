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

// PromptBody returns the part of a work item description suitable for pasting into an agent prompt.
// If the description is a single line, returns that line. If there are lines after the title,
// returns those lines (the body) so the agent gets the detailed prompt without the short title.
func PromptBody(s string) string {
	_, rest, ok := strings.Cut(s, "\n")
	if !ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(rest)
}
