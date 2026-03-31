package wn

import (
	"bytes"
	"strings"
	"text/template"
)

// PromptData is passed to the prompt template.
type PromptData struct {
	ItemID      string
	Description string
	FirstLine   string
	Worktree    string
	Branch      string
}

// ExpandPromptTemplate executes the prompt template with item and optional worktree/branch.
func ExpandPromptTemplate(tpl string, item *Item, worktree, branch string) (string, error) {
	if tpl == "" {
		return item.Description, nil
	}
	data := PromptData{
		ItemID:      item.ID,
		Description: item.Description,
		FirstLine:   FirstLine(item.Description),
		Worktree:    worktree,
		Branch:      branch,
	}
	tm, err := template.New("prompt").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tm.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// shellEscapeForDoubleQuoted escapes a string for safe embedding inside a
// double-quoted string in sh. Escapes \, ", `, and $ so the result can be used
// in templates like `cursor agent "{{.Prompt}}"` without breaking sh -c.
// Backticks and $ must be escaped to prevent command substitution and variable
// expansion (e.g. work item descriptions like `--wid <id>` or "cost $5").
func shellEscapeForDoubleQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "$", "\\$")
	return s
}

// shellEscapeForShWord wraps a string in single quotes and escapes internal
// single quotes as '\", producing a single sh word that evaluates to the
// original string. Safe for ItemID, Worktree, Branch when used in templates
// that pass the result to sh -c.
func shellEscapeForShWord(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// resumeFlag returns "--resume <sessionID>" if sessionID is non-empty, else "".
func resumeFlag(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return "--resume " + sessionID
}

// ExpandCommandTemplate executes the command template; prompt is the result of the prompt template.
// Prompt is escaped for double-quoted context; ItemID, Worktree, and Branch are escaped as
// single-quoted shell words so descriptions, imported IDs, and branch notes cannot inject
// commands when the result is passed to sh -c.
// sessionID is the Claude Code session ID for resume support (from the "claude-session" note);
// ResumeFlag is "--resume <sessionID>" if sessionID is non-empty, else "".
func ExpandCommandTemplate(tpl string, prompt, itemID, worktree, branch, sessionID string) (string, error) {
	escapedPrompt := shellEscapeForDoubleQuoted(prompt)
	data := struct {
		Prompt     string
		ItemID     string
		Worktree   string
		Branch     string
		ResumeFlag string
		SessionID  string
	}{escapedPrompt, shellEscapeForShWord(itemID), shellEscapeForShWord(worktree), shellEscapeForShWord(branch), resumeFlag(sessionID), sessionID}
	tm, err := template.New("cmd").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tm.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
