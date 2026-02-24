package wn

import "strings"

// HasDescriptionBody reports whether the description has content after the first line
// (i.e. more than a title-only one-liner).
func HasDescriptionBody(description string) bool {
	_, rest, ok := strings.Cut(description, "\n")
	return ok && strings.TrimSpace(rest) != ""
}

// PromptContent returns the work item content to substitute into a prompt template.
// For a title-only one-liner it returns the single line; for multi-line descriptions
// it returns the full description (title and body).
func PromptContent(description string) string {
	if !HasDescriptionBody(description) {
		return strings.TrimSpace(description)
	}
	return description
}

// FormatPrompt replaces the first "{}" in template with content and returns the result.
// Used by the prompt subcommand to wrap work item content in a user-provided template.
func FormatPrompt(template, content string) string {
	return strings.Replace(template, "{}", content, 1)
}
