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

// MergeOpts configures a single-item merge (wn merge).
type MergeOpts struct {
	Root          string // project root (contains .wn)
	WorkID        string // work item id; if empty, use current task (must be review-ready)
	MainBranch    string // branch to rebase onto and merge into (e.g. "main")
	ValidateCmd   string // build/validation command (e.g. "make"); run in Root
	Audit         io.Writer
	WorktreesBase string // worktree base path for finding/removing worktree; empty = filepath.Dir(Root)
}

// RunMerge merges the given work item's branch into main: checkout work item branch (removing
// its worktree if present), run validate, rebase main, checkout main, merge branch, validate again,
// mark item done, delete branch. All git and validate commands are logged with timestamps to Audit.
// On validate or rebase failure returns an error instructing the agent to fix and re-run.
func RunMerge(store Store, opts MergeOpts) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	mainBranch := opts.MainBranch
	if mainBranch == "" {
		mainBranch = "main"
	}
	validateCmd := opts.ValidateCmd
	if validateCmd == "" {
		validateCmd = "make"
	}

	// Resolve work item
	var item *Item
	if opts.WorkID != "" {
		var err error
		item, err = store.Get(opts.WorkID)
		if err != nil {
			return fmt.Errorf("work item %s: %w", opts.WorkID, err)
		}
	} else {
		meta, err := ReadMeta(opts.Root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task; use wn pick or pass --wid <id>")
		}
		item, err = store.Get(meta.CurrentID)
		if err != nil {
			return fmt.Errorf("current task %s: %w", meta.CurrentID, err)
		}
	}
	if item.Done {
		return fmt.Errorf("work item %s is already done", item.ID)
	}
	if !item.ReviewReady {
		return fmt.Errorf("work item %s is not review-ready; merge only applies to review-ready items", item.ID)
	}
	idx := item.NoteIndexByName("branch")
	if idx < 0 {
		return fmt.Errorf("work item %s has no branch note (required for merge)", item.ID)
	}
	branchName := strings.TrimSpace(item.Notes[idx].Body)
	if branchName == "" {
		return fmt.Errorf("work item %s branch note is empty", item.ID)
	}

	// If work item branch is checked out in another worktree, remove it so we can checkout in main worktree
	wtPath, err := WorktreePathForBranch(root, branchName)
	if err != nil {
		return fmt.Errorf("find worktree for branch %s: %w", branchName, err)
	}
	if wtPath != "" {
		auditLog(opts.Audit, "git worktree remove %s (so we can checkout %s here)", wtPath, branchName)
		if err := RemoveWorktree(root, wtPath, opts.Audit); err != nil {
			return fmt.Errorf("remove worktree for %s: %w", branchName, err)
		}
	}

	// Checkout work item branch if not already on it
	currentBranch, err := worktreeBranch(root)
	if err != nil {
		return fmt.Errorf("detect current branch: %w", err)
	}
	if currentBranch != branchName {
		auditLog(opts.Audit, "git checkout %s", branchName)
		cmd := exec.Command("git", "checkout", branchName)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout %s: %w\n%s", branchName, err, out)
		}
	}

	// Run build/validation
	if err := runValidate(root, validateCmd, opts.Audit); err != nil {
		return fmt.Errorf("%w\nFix the failure, then run: git add -A && git commit --amend --no-edit (or new commit), then re-run wn merge", err)
	}

	// Rebase onto main
	auditLog(opts.Audit, "git rebase %s", mainBranch)
	cmd := exec.Command("git", "rebase", mainBranch)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rebase %s failed (merge conflicts?): %w\n%s\nFix conflicts, then run: git add -A && git rebase --continue (or git rebase --abort), then re-run wn merge", mainBranch, err, out)
	}

	// Checkout main
	auditLog(opts.Audit, "git checkout %s", mainBranch)
	cmd = exec.Command("git", "checkout", mainBranch)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", mainBranch, err, out)
	}

	// Merge work item branch (should be clean)
	auditLog(opts.Audit, "git merge %s", branchName)
	cmd = exec.Command("git", "merge", branchName, "-m", "wn merge "+item.ID+": "+FirstLine(item.Description))
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge %s: %w\n%s", branchName, err, out)
	}

	// Run build/validation again on main
	if err := runValidate(root, validateCmd, opts.Audit); err != nil {
		return fmt.Errorf("validate after merge failed: %w\nFix, then git add -A && git commit --amend --no-edit, then re-run wn merge (you may need to merge again if main moved)", err)
	}

	// Mark work item done
	now := time.Now().UTC()
	msg := "merged to " + mainBranch
	if err := store.UpdateItem(item.ID, func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneMessage = msg
		it.DoneStatus = DoneStatusDone
		it.ReviewReady = false
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "done", Msg: msg})
		return it, nil
	}); err != nil {
		return fmt.Errorf("mark item done: %w", err)
	}

	// Delete the branch
	auditLog(opts.Audit, "git branch -d %s", branchName)
	cmd = exec.Command("git", "branch", "-d", branchName)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		// Log but don't fail merge; item is already marked done
		if opts.Audit != nil {
			fmt.Fprintf(opts.Audit, "%s git branch -d %s failed: %v\n%s", now.Format("2006-01-02 15:04:05"), branchName, err, out)
		}
	}
	return nil
}

func runValidate(repoRoot, command string, audit io.Writer) error {
	auditLog(audit, "exec (validate): %s", command)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = repoRoot
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if audit != nil {
			fmt.Fprintf(audit, "%s validate failed: %v\n", time.Now().UTC().Format("2006-01-02 15:04:05"), err)
		}
		return fmt.Errorf("validate command %q failed: %w", command, err)
	}
	return nil
}
