package wn

import (
	"bytes"
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

// BranchMergedInto returns true if the given branch's commits are reachable from
// intoRef (i.e. the branch has been merged into that ref). intoRef may be empty
// for HEAD. Returns an error if the branch does not exist.
func BranchMergedInto(mainRoot, branchName, intoRef string) (bool, error) {
	if intoRef == "" {
		intoRef = "HEAD"
	}
	exists, err := BranchExists(mainRoot, branchName)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, fmt.Errorf("branch %s does not exist", branchName)
	}
	cmd := exec.Command("git", "merge-base", "--is-ancestor", "refs/heads/"+branchName, intoRef)
	cmd.Dir = mainRoot
	err = cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil // not an ancestor
		}
		return false, fmt.Errorf("git merge-base: %w", err)
	}
	return true, nil
}

// BranchExists returns true if the branch exists in the repo at mainRoot.
func BranchExists(mainRoot, branchName string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branchName)
	cmd.Dir = mainRoot
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil // branch does not exist
		}
		return false, fmt.Errorf("git rev-parse: %w", err)
	}
	return true, nil
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

// EnsureWorktree creates a worktree at worktreePath (full path) for the given branchName.
// If createBranch is true, creates a new branch from the default branch and adds the worktree;
// otherwise the branch must already exist. audit is written to with timestamped git commands (can be nil).
// Returns the absolute path to the new worktree.
func EnsureWorktree(mainRoot, worktreePath, branchName string, createBranch bool, audit io.Writer) (string, error) {
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
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
			// Branch may already exist (e.g. restart after Ctrl-C); continue so we can reuse worktree.
			if !strings.Contains(string(out), "already exists") {
				return "", fmt.Errorf("git branch %s %s: %w\n%s", branchName, def, err, out)
			}
		}
	}

	// If worktree path already exists and is checked out to this branch, reuse it (e.g. restart after Ctrl-C).
	if _, err := os.Stat(absPath); err == nil {
		if current, err := worktreeBranch(absPath); err == nil && current == branchName {
			auditLog(audit, "reuse existing worktree %s (branch %s)", absPath, branchName)
			return absPath, nil
		}
	}

	auditLog(audit, "git worktree add %s %s", absPath, branchName)
	cmd := exec.Command("git", "worktree", "add", absPath, branchName)
	cmd.Dir = mainRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return absPath, nil
}

// worktreeBranch returns the current branch name in the given worktree path, or error if not a valid worktree.
func worktreeBranch(worktreePath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitWorktreeChanges stages all changes in the worktree and commits with the given message.
// If there are no changes (git status --porcelain is empty), no-op and return nil.
// audit is written to with timestamped git commands (can be nil).
func CommitWorktreeChanges(worktreePath, message string, audit io.Writer) error {
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = worktreePath
	out, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil
	}
	auditLog(audit, "git add -A (Dir=%s)", worktreePath)
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = worktreePath
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A: %w\n%s", err, out)
	}
	auditLog(audit, "git commit -m %q (Dir=%s)", message, worktreePath)
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = worktreePath
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
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

// WorktreePathForBranch returns the path of the worktree that has the given branch checked out,
// or "" if no worktree has that branch. mainRoot is the repo root. Used to remove a worktree
// so the branch can be checked out in the main worktree (e.g. for wn merge).
func WorktreePathForBranch(mainRoot, branchName string) (string, error) {
	cmd := exec.Command("git", "worktree", "list")
	cmd.Dir = mainRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}
	// Format: "<path> <commit> [<branch>]" (path may contain spaces; commit is hex)
	suffix := " [" + branchName + "]"
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasSuffix(line, suffix) {
			continue
		}
		beforeBranch := line[:len(line)-len(suffix)]
		// beforeBranch is "<path> <commit>"; path may have spaces, commit is last token
		fields := strings.Fields(beforeBranch)
		if len(fields) < 2 {
			continue
		}
		path := strings.Join(fields[:len(fields)-1], " ")
		return path, nil
	}
	return "", nil
}
