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

func TestEnsureWorktree_alreadyExists_sameBranch(t *testing.T) {
	// Simulate restart after Ctrl-C: worktree and branch already exist; should reuse path and continue.
	dir := t.TempDir()
	setupGitRepo(t, dir)
	base := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	var audit bytes.Buffer
	worktreePath := filepath.Join(base, "wn-reuse-test")
	path1, err := EnsureWorktree(dir, worktreePath, "wn-reuse-test", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree first time: %v", err)
	}
	// Second call with same path and branch (e.g. restart after Ctrl-C): should succeed and return same path.
	path2, err := EnsureWorktree(dir, worktreePath, "wn-reuse-test", false, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree when worktree already exists: %v", err)
	}
	if path1 != path2 {
		t.Errorf("reuse returned path %q, want %q", path2, path1)
	}
	// Third call with createBranch true (e.g. restart before branch note was saved): branch already exists, worktree exists; should still reuse.
	path3, err := EnsureWorktree(dir, worktreePath, "wn-reuse-test", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree when branch and worktree already exist: %v", err)
	}
	if path1 != path3 {
		t.Errorf("reuse with createBranch true returned path %q, want %q", path3, path1)
	}
	_ = RemoveWorktree(dir, path1, &audit)
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

func TestBranchMergedInto(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	def, err := DefaultBranch(dir)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}

	// Create feature branch with a commit
	execIn(t, dir, "git", "checkout", "-b", "wn-abc-feature")
	writeFile(t, filepath.Join(dir, "feature.txt"), "feature work")
	execIn(t, dir, "git", "add", "feature.txt")
	execIn(t, dir, "git", "commit", "-m", "add feature")

	// Merge into main
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-abc-feature", "-m", "merge feature")

	// Branch should be merged into HEAD
	merged, err := BranchMergedInto(dir, "wn-abc-feature", "")
	if err != nil {
		t.Fatalf("BranchMergedInto: %v", err)
	}
	if !merged {
		t.Error("BranchMergedInto(wn-abc-feature, HEAD) = false, want true (branch was merged)")
	}

	// Create unmerged branch
	execIn(t, dir, "git", "checkout", "-b", "wn-xyz-unmerged")
	writeFile(t, filepath.Join(dir, "unmerged.txt"), "unmerged")
	execIn(t, dir, "git", "add", "unmerged.txt")
	execIn(t, dir, "git", "commit", "-m", "unmerged change")
	execIn(t, dir, "git", "checkout", def)

	// Unmerged branch should not be merged into HEAD
	merged, err = BranchMergedInto(dir, "wn-xyz-unmerged", "")
	if err != nil {
		t.Fatalf("BranchMergedInto: %v", err)
	}
	if merged {
		t.Error("BranchMergedInto(wn-xyz-unmerged, HEAD) = true, want false (branch not merged)")
	}

	// Non-existent branch
	merged, err = BranchMergedInto(dir, "nonexistent-branch", "")
	if err == nil {
		t.Error("BranchMergedInto(nonexistent) want error, got nil")
	}
	if merged {
		t.Error("BranchMergedInto(nonexistent) should not return true")
	}
}

func TestWorktreePathForBranch(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	base := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	var audit bytes.Buffer
	worktreePath := filepath.Join(base, "wn-path-test")
	path, err := EnsureWorktree(dir, worktreePath, "wn-path-test", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	got, err := WorktreePathForBranch(dir, "wn-path-test")
	if err != nil {
		t.Fatalf("WorktreePathForBranch: %v", err)
	}
	absPath, _ := filepath.Abs(path)
	absGot, _ := filepath.Abs(got)
	normPath, _ := filepath.EvalSymlinks(absPath)
	normGot, _ := filepath.EvalSymlinks(absGot)
	if normGot != normPath {
		t.Errorf("WorktreePathForBranch(wn-path-test) = %q, want %q", got, path)
	}
	// Branch not in any worktree returns ""
	gotNone, err := WorktreePathForBranch(dir, "nonexistent-branch")
	if err != nil {
		t.Fatalf("WorktreePathForBranch(nonexistent): %v", err)
	}
	if gotNone != "" {
		t.Errorf("WorktreePathForBranch(nonexistent) = %q, want \"\"", gotNone)
	}
	_ = RemoveWorktree(dir, path, &audit)
}
