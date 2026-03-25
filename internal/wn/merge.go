package wn

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SquashMergeResult reports the outcome of a SquashMerge call.
type SquashMergeResult struct {
	ItemID     string // the associated work item ID
	Branch     string // the feature branch that was merged
	CommitHash string // the squash commit hash recorded in wn:commit (empty on dry-run)
}

// SquashMerge squash-merges branchName into the current HEAD of mainRoot,
// records the resulting commit hash as a wn:commit note on the work item
// associated with the branch (found via its wn:branch note), and marks the
// item done.
//
// message must be non-empty. If dryRun is true, no git or store changes are made.
func SquashMerge(store Store, mainRoot, branchName, message string, dryRun bool) (*SquashMergeResult, error) {
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("commit message must not be empty")
	}

	item, err := FindItemByBranch(store, branchName)
	if err != nil {
		return nil, fmt.Errorf("store lookup: %w", err)
	}
	if item == nil {
		return nil, fmt.Errorf("no wn item found for branch %q", branchName)
	}

	exists, err := BranchExists(mainRoot, branchName)
	if err != nil {
		return nil, fmt.Errorf("branch check: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("branch %q does not exist", branchName)
	}

	if dryRun {
		return &SquashMergeResult{ItemID: item.ID, Branch: branchName}, nil
	}

	mergeCmd := exec.Command("git", "merge", "--squash", branchName)
	mergeCmd.Dir = mainRoot
	if out, mergeErr := mergeCmd.CombinedOutput(); mergeErr != nil {
		return nil, fmt.Errorf("git merge --squash %s: %w\n%s", branchName, mergeErr, strings.TrimSpace(string(out)))
	}

	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = mainRoot
	if out, commitErr := commitCmd.CombinedOutput(); commitErr != nil {
		return nil, fmt.Errorf("git commit: %w\n%s", commitErr, strings.TrimSpace(string(out)))
	}

	revCmd := exec.Command("git", "rev-parse", "HEAD")
	revCmd.Dir = mainRoot
	hashOut, err := revCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	commitHash := strings.TrimSpace(string(hashOut))

	now := time.Now().UTC()
	if err := store.UpdateItem(item.ID, func(it *Item) (*Item, error) {
		idx := it.NoteIndexByName(NoteNameCommit)
		if idx >= 0 {
			it.Notes[idx].Body = commitHash
		} else {
			it.Notes = append(it.Notes, Note{Name: NoteNameCommit, Created: now, Body: commitHash})
		}
		it.Done = true
		it.DoneMessage = message
		it.DoneStatus = DoneStatusDone
		it.ReviewReady = false
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "done", Msg: "squash merged to main"})
		return it, nil
	}); err != nil {
		return nil, fmt.Errorf("store update: %w", err)
	}

	return &SquashMergeResult{ItemID: item.ID, Branch: branchName, CommitHash: commitHash}, nil
}
