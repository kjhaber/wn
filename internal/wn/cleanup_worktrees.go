package wn

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// WorktreeInfo holds information about a single git worktree.
type WorktreeInfo struct {
	Path   string // absolute path to the worktree
	Head   string // HEAD commit hash
	Branch string // short branch name (e.g. "main"); empty if detached
}

// ListWorktrees returns all git worktrees for the repo at mainRoot using
// git worktree list --porcelain. The first entry is always the main worktree.
func ListWorktrees(mainRoot string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = mainRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list --porcelain: %w", err)
	}
	return parseWorktreePorcelain(string(out)), nil
}

// parseWorktreePorcelain parses the output of git worktree list --porcelain.
// Each worktree block is separated by a blank line.
func parseWorktreePorcelain(output string) []WorktreeInfo {
	var result []WorktreeInfo
	for block := range strings.SplitSeq(strings.TrimSpace(output), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var wt WorktreeInfo
		for line := range strings.SplitSeq(block, "\n") {
			line = strings.TrimSpace(line)
			if path, ok := strings.CutPrefix(line, "worktree "); ok {
				wt.Path = path
			} else if head, ok := strings.CutPrefix(line, "HEAD "); ok {
				wt.Head = head
			} else if ref, ok := strings.CutPrefix(line, "branch "); ok {
				// Convert "refs/heads/name" -> "name"
				if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
					wt.Branch = name
				} else {
					wt.Branch = ref
				}
			}
		}
		if wt.Path != "" {
			result = append(result, wt)
		}
	}
	return result
}

// CleanIgnoredFiles runs git clean -fdX in dir, removing gitignored files and
// directories (build artifacts, temp config files, etc.). audit is written to
// with a timestamped entry (can be nil).
func CleanIgnoredFiles(dir string, audit io.Writer) error {
	auditLog(audit, "git clean -fdX (Dir=%s)", dir)
	cmd := exec.Command("git", "clean", "-fdX")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean -fdX: %w\n%s", err, out)
	}
	return nil
}

// CleanupWorktreeResult reports one worktree's outcome from CleanupWorktrees.
type CleanupWorktreeResult struct {
	Path          string
	Branch        string
	ItemID        string
	Status        string // "removed", "branch_deleted", "skipped_not_done", "skipped_not_merged", "skipped_no_item", "skipped_detached", "error"
	Reason        string
	BranchDeleted bool // true if the branch was also deleted
}

// CleanupWorktrees finds non-main worktrees whose associated wn item is done and
// whose branch has been merged into intoRef (empty = HEAD), then removes them.
// When cleanIgnored is true, gitignored files are removed from each eligible
// worktree before removal (to free disk space from build artifacts, etc.).
// When dryRun is true, no changes are made but results are still returned.
// When worktreesOnly is false (the default), the associated branch is also deleted
// after the worktree is removed. Branches for which the worktree was already
// removed (orphaned branches) are also cleaned up unless worktreesOnly is true.
// The main worktree (first in git worktree list) is always skipped.
func CleanupWorktrees(store Store, mainRoot, intoRef string, dryRun, cleanIgnored, worktreesOnly bool, audit io.Writer) ([]CleanupWorktreeResult, error) {
	worktrees, err := ListWorktrees(mainRoot)
	if err != nil {
		return nil, err
	}

	ref := intoRef
	if ref == "" {
		ref = "HEAD"
	}

	// Track which branches are covered by an active worktree so we can detect orphans.
	activeBranches := make(map[string]bool)
	for _, wt := range worktrees[1:] {
		if wt.Branch != "" {
			activeBranches[wt.Branch] = true
		}
	}

	var results []CleanupWorktreeResult
	// Skip index 0 — that's always the main worktree
	for _, wt := range worktrees[1:] {
		if wt.Branch == "" {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Status: "skipped_detached",
				Reason: "worktree is in detached HEAD state",
			})
			continue
		}

		item, err := FindItemByBranch(store, wt.Branch)
		if err != nil {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Branch: wt.Branch,
				Status: "error",
				Reason: fmt.Sprintf("store lookup failed: %v", err),
			})
			continue
		}
		if item == nil {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Branch: wt.Branch,
				Status: "skipped_no_item",
				Reason: "no wn item found for this branch",
			})
			continue
		}

		if !item.Done {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Branch: wt.Branch,
				ItemID: item.ID,
				Status: "skipped_not_done",
				Reason: "work item is not done",
			})
			continue
		}

		// Check if the branch is merged. If the branch no longer exists we treat
		// it as merged (it was already cleaned up post-merge).
		branchAlreadyGone := false
		merged, err := BranchMergedInto(mainRoot, wt.Branch, ref)
		if err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				// Branch was already deleted — safe to remove orphaned worktree.
				merged = true
				branchAlreadyGone = true
			} else {
				results = append(results, CleanupWorktreeResult{
					Path:   wt.Path,
					Branch: wt.Branch,
					ItemID: item.ID,
					Status: "error",
					Reason: fmt.Sprintf("merge check failed: %v", err),
				})
				continue
			}
		}
		if !merged {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Branch: wt.Branch,
				ItemID: item.ID,
				Status: "skipped_not_merged",
				Reason: fmt.Sprintf("branch %s has not been merged into %s", wt.Branch, ref),
			})
			continue
		}

		if dryRun {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Branch: wt.Branch,
				ItemID: item.ID,
				Status: "removed",
				Reason: "would remove (dry run)",
			})
			continue
		}

		if cleanIgnored {
			if err := CleanIgnoredFiles(wt.Path, audit); err != nil {
				// Non-fatal: log and proceed with worktree removal
				auditLog(audit, "warning: clean ignored files in %s: %v", wt.Path, err)
			}
		}

		if err := RemoveWorktree(mainRoot, wt.Path, audit); err != nil {
			results = append(results, CleanupWorktreeResult{
				Path:   wt.Path,
				Branch: wt.Branch,
				ItemID: item.ID,
				Status: "error",
				Reason: fmt.Sprintf("git worktree remove failed: %v", err),
			})
			continue
		}

		branchDeleted := false
		if !worktreesOnly && !branchAlreadyGone {
			if delErr := DeleteBranch(mainRoot, wt.Branch, audit); delErr != nil {
				auditLog(audit, "warning: delete branch %s: %v", wt.Branch, delErr)
			} else {
				branchDeleted = true
			}
		}

		results = append(results, CleanupWorktreeResult{
			Path:          wt.Path,
			Branch:        wt.Branch,
			ItemID:        item.ID,
			Status:        "removed",
			Reason:        fmt.Sprintf("removed (branch %s merged into %s)", wt.Branch, ref),
			BranchDeleted: branchDeleted,
		})
	}

	// Scan for orphaned branches: branches with done+merged items but no active worktree.
	if !worktreesOnly {
		orphanResults, err := cleanupOrphanedBranches(store, mainRoot, ref, activeBranches, dryRun, audit)
		if err != nil {
			auditLog(audit, "warning: orphaned branch scan: %v", err)
		} else {
			results = append(results, orphanResults...)
		}
	}

	return results, nil
}

// cleanupOrphanedBranches finds branches associated with done+merged wn items
// that have no active worktree, and deletes them.
func cleanupOrphanedBranches(store Store, mainRoot, ref string, activeBranches map[string]bool, dryRun bool, audit io.Writer) ([]CleanupWorktreeResult, error) {
	items, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("store.List: %w", err)
	}

	var results []CleanupWorktreeResult
	for _, item := range items {
		if !item.Done {
			continue
		}
		branch := ""
		for _, n := range item.Notes {
			if n.Name == "branch" {
				branch = n.Body
				break
			}
		}
		if branch == "" {
			continue
		}
		// Skip branches that are currently checked out in an active worktree
		// (those are handled by the main worktree loop above).
		if activeBranches[branch] {
			continue
		}

		exists, err := BranchExists(mainRoot, branch)
		if err != nil {
			results = append(results, CleanupWorktreeResult{
				Branch: branch,
				ItemID: item.ID,
				Status: "error",
				Reason: fmt.Sprintf("branch existence check failed: %v", err),
			})
			continue
		}
		if !exists {
			continue
		}

		merged, err := BranchMergedInto(mainRoot, branch, ref)
		if err != nil {
			results = append(results, CleanupWorktreeResult{
				Branch: branch,
				ItemID: item.ID,
				Status: "error",
				Reason: fmt.Sprintf("merge check failed: %v", err),
			})
			continue
		}
		if !merged {
			continue
		}

		if dryRun {
			results = append(results, CleanupWorktreeResult{
				Branch:        branch,
				ItemID:        item.ID,
				Status:        "branch_deleted",
				Reason:        "would delete orphaned branch (dry run)",
				BranchDeleted: true,
			})
			continue
		}

		if err := DeleteBranch(mainRoot, branch, audit); err != nil {
			results = append(results, CleanupWorktreeResult{
				Branch: branch,
				ItemID: item.ID,
				Status: "error",
				Reason: fmt.Sprintf("git branch -D failed: %v", err),
			})
			continue
		}

		results = append(results, CleanupWorktreeResult{
			Branch:        branch,
			ItemID:        item.ID,
			Status:        "branch_deleted",
			Reason:        fmt.Sprintf("deleted orphaned branch %s (merged into %s)", branch, ref),
			BranchDeleted: true,
		})
	}
	return results, nil
}
