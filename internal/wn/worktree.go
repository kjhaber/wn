package wn

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// auditLog writes a timestamped line to w. If w is nil, no-op.
func auditLog(w io.Writer, format string, args ...interface{}) {
	if w == nil {
		return
	}
	ts := time.Now().UTC().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s "+format+"\n", append([]interface{}{ts}, args...)...)
	_, _ = io.WriteString(w, line)
}

// DefaultBranch returns the default branch name for the repo at mainRoot (e.g. "main" or "master").
func DefaultBranch(mainRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = mainRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "main", nil
	}
	return branch, nil
}

// EnsureWorktree creates a worktree at worktreeBase/branchName (or a path derived from it).
// If createBranch is true, creates a new branch from the default branch and adds the worktree;
// otherwise the branch must already exist. audit is written to with timestamped git commands (can be nil).
// Returns the absolute path to the new worktree.
func EnsureWorktree(mainRoot, worktreeBase, branchName string, createBranch bool, audit io.Writer) (string, error) {
	absBase, err := filepath.Abs(worktreeBase)
	if err != nil {
		return "", err
	}
	worktreePath := filepath.Join(absBase, branchName)
	if err := os.MkdirAll(absBase, 0755); err != nil {
		return "", err
	}

	if createBranch {
		def, err := DefaultBranch(mainRoot)
		if err != nil {
			return "", err
		}
		auditLog(audit, "git branch %s %s", branchName, def)
		cmd := exec.Command("git", "branch", branchName, def)
		cmd.Dir = mainRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git branch %s %s: %w\n%s", branchName, def, err, out)
		}
	}

	auditLog(audit, "git worktree add %s %s", worktreePath, branchName)
	cmd := exec.Command("git", "worktree", "add", worktreePath, branchName)
	cmd.Dir = mainRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return worktreePath, nil
	}
	return absPath, nil
}

// RemoveWorktree removes the worktree at worktreePath. mainRoot is the repo root (where .git is).
// audit is written to with the git command (can be nil).
func RemoveWorktree(mainRoot, worktreePath string, audit io.Writer) error {
	auditLog(audit, "git worktree remove %s", worktreePath)
	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	cmd.Dir = mainRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, out)
	}
	return nil
}
