package wn

import (
	"fmt"
	"io"
)

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
			if n.Name == NoteNameBranch || n.Name == "branch" {
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
			// Branch exists but is not an ancestor — may be a squash merge.
			if commitRef := commitRefFromNotes(item); commitRef != "" {
				if commitMerged, commitErr := CommitMergedInto(mainRoot, commitRef, ref); commitErr == nil && commitMerged {
					merged = true
				}
			}
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
