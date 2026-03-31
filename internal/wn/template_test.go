package wn

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestExpandPromptTemplate(t *testing.T) {
	item := &Item{ID: "abc123", Description: "Add feature\nWith details"}
	got, err := ExpandPromptTemplate("{{.Description}}", item, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != item.Description {
		t.Errorf("got %q", got)
	}
	got, err = ExpandPromptTemplate("Item {{.ItemID}}: {{.FirstLine}}", item, "/wt", "wn-abc-add-feature")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Item abc123: Add feature" {
		t.Errorf("got %q", got)
	}
}

func TestExpandCommandTemplate(t *testing.T) {
	got, err := ExpandCommandTemplate("echo {{.Prompt}}", "hello world", "abc", "/wt", "br", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo hello world" {
		t.Errorf("got %q", got)
	}
}

// TestExpandCommandTemplate_escapesQuotes verifies that prompts containing double
// quotes, single quotes, and backslashes are shell-escaped so the command can
// be safely passed to sh -c (fixes agent-orch failure when item description
// contains e.g. "resolved", "won't fix").
func TestExpandCommandTemplate_escapesQuotes(t *testing.T) {
	prompt := `Add a "resolved" state similar to "done". Can be used for "won't fix", "duplicate".`
	tpl := `printf '%s' "{{.Prompt}}"`
	got, err := ExpandCommandTemplate(tpl, prompt, "abc", "/wt", "br", "")
	if err != nil {
		t.Fatal(err)
	}
	// Must not contain unescaped " in the middle (would break sh -c)
	if got == `printf '%s' "Add a "resolved" state similar to "done". Can be used for "won't fix", "duplicate"."` {
		t.Fatal("prompt was not escaped; unescaped quotes would break sh -c")
	}
	// Run through sh -c to verify it executes without syntax error and produces correct output
	cmd := exec.Command("sh", "-c", got)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("sh -c failed (shell syntax error): %v.\nExpanded command: %s", err, got)
	}
	if stdout.String() != prompt {
		t.Errorf("sh -c output = %q, want %q (escaped prompt did not round-trip)", stdout.String(), prompt)
	}
}

// TestExpandCommandTemplate_escapesBackticksAndDollar verifies that prompts
// containing backticks and $ are shell-escaped so they are not interpreted as
// command substitution or variable expansion when passed to sh -c (fixes
// agent-orch failure when item description contains e.g. `--wid <id>`).
func TestExpandCommandTemplate_escapesBackticksAndDollar(t *testing.T) {
	prompt := "wn tag add <tag-name> [--wid <id>]\n`--wid <id>` should be used. Cost $5 or $(id) risky."
	tpl := `printf '%s' "{{.Prompt}}"`
	got, err := ExpandCommandTemplate(tpl, prompt, "abc", "/wt", "br", "")
	if err != nil {
		t.Fatal(err)
	}
	// Run through sh -c to verify it executes without command-substitution or expansion
	cmd := exec.Command("sh", "-c", got)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("sh -c failed (shell syntax/command substitution): %v.\nExpanded command: %s", err, got)
	}
	if stdout.String() != prompt {
		t.Errorf("sh -c output = %q, want %q (backticks/$ did not round-trip safely)", stdout.String(), prompt)
	}
}

// TestExpandCommandTemplate_escapesItemIDWorktreeBranch verifies that ItemID,
// Worktree, and Branch are shell-escaped so values with metacharacters (from
// import or branch notes) cannot inject commands when passed to sh -c.
func TestExpandCommandTemplate_escapesItemIDWorktreeBranch(t *testing.T) {
	itemID := `x; rm -rf /`
	worktree := `/tmp/worktree with spaces`
	branch := `main'$(id)'`
	tpl := `printf 'id=%s wd=%s br=%s' {{.ItemID}} {{.Worktree}} {{.Branch}}`
	got, err := ExpandCommandTemplate(tpl, "prompt", itemID, worktree, branch, "")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", got)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("sh -c failed (shell syntax/injection): %v.\nExpanded command: %s", err, got)
	}
	want := "id=" + itemID + " wd=" + worktree + " br=" + branch
	if stdout.String() != want {
		t.Errorf("sh -c output = %q, want %q (ItemID/Worktree/Branch did not round-trip safely)", stdout.String(), want)
	}
}

func TestExpandCommandTemplate_withSessionID(t *testing.T) {
	tpl := `claude {{.ResumeFlag}} --print "{{.Prompt}}"`
	got, err := ExpandCommandTemplate(tpl, "do the thing", "abc", "/wt", "br", "ses-xyz789")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "--resume") {
		t.Errorf("expected --resume in expanded cmd, got %q", got)
	}
	if !strings.Contains(got, "ses-xyz789") {
		t.Errorf("expected session ID in expanded cmd, got %q", got)
	}
}

func TestExpandCommandTemplate_noSessionID(t *testing.T) {
	tpl := `claude {{.ResumeFlag}} --print "{{.Prompt}}"`
	got, err := ExpandCommandTemplate(tpl, "do the thing", "abc", "/wt", "br", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "--resume") {
		t.Errorf("expected no --resume when sessionID is empty, got %q", got)
	}
}
