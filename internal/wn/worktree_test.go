package wn

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBranch(t *testing.T) {
	// Need a real git repo with origin/HEAD or at least one branch
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Skipf("git init failed (no git?): %v", err)
	}
	// Create initial commit so we have a branch
	writeFile(t, filepath.Join(dir, "f"), "x")
	execIn(t, dir, "git", "add", "f")
	execIn(t, dir, "git", "commit", "-m", "init")

	got, err := DefaultBranch(dir)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	// Fresh init creates "main" on modern git, or "master" on older
	if got != "main" && got != "master" {
		t.Errorf("DefaultBranch = %q, want main or master", got)
	}
}

func TestEnsureWorktree_newBranch(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	base := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	var audit bytes.Buffer

	worktreePath := filepath.Join(base, "wn-abc-add-feature")
	path, err := EnsureWorktree(dir, worktreePath, "wn-abc-add-feature", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	if path == "" {
		t.Fatal("EnsureWorktree returned empty path")
	}
	if !strings.HasPrefix(path, base) {
		t.Errorf("worktree path %q not under base %q", path, base)
	}
	// Audit log should contain timestamp and git command
	auditStr := audit.String()
	if !strings.Contains(auditStr, "git") {
		t.Errorf("audit log should mention git: %q", auditStr)
	}
	if !strings.Contains(auditStr, "worktree") {
		t.Errorf("audit log should mention worktree: %q", auditStr)
	}
	// Cleanup
	_ = RemoveWorktree(dir, path, &audit)
}

func TestCommitWorktreeChanges(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	base := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	var audit bytes.Buffer
	worktreePath := filepath.Join(base, "wn-commit-test")
	path, err := EnsureWorktree(dir, worktreePath, "wn-commit-test", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	writeFile(t, filepath.Join(path, "newfile"), "content")
	err = CommitWorktreeChanges(path, "wn abc123: Add feature", &audit)
	if err != nil {
		t.Fatalf("CommitWorktreeChanges: %v", err)
	}
	cmd := exec.Command("git", "log", "-1", "--oneline")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "wn abc123") {
		t.Errorf("git log -1 = %q, want commit message containing 'wn abc123'", out)
	}
	_ = RemoveWorktree(dir, path, &audit)
}

func TestCommitWorktreeChanges_noChanges(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	var audit bytes.Buffer
	err := CommitWorktreeChanges(dir, "nothing", &audit)
	if err != nil {
		t.Fatalf("CommitWorktreeChanges(clean): %v", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	base := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	var audit bytes.Buffer
	worktreePath := filepath.Join(base, "wn-rm-test")
	path, err := EnsureWorktree(dir, worktreePath, "wn-rm-test", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	err = RemoveWorktree(dir, path, &audit)
	if err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("worktree path %q should be removed", path)
	}
}

func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
}

func execIn(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
