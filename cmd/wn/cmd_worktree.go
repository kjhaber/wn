package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type worktreeSetupFlags struct {
	claim        string
	branchPrefix string
	worktreeBase string
	tag          string
	next         bool
	branch       string
}

func newWorktreeSetupCmd() *cobra.Command {
	flags := &worktreeSetupFlags{}
	cmd := &cobra.Command{
		Use:   "worktree [id]",
		Short: "Claim a work item and create its git worktree, printing the path to stdout",
		Long: `Claim a work item, create a branch and git worktree for it, and print the worktree path to stdout.

Without args: uses the currently selected item (set via wn pick or wn next).
With id: claims that specific item.
With --next: claims the next undone item from the queue.

Human-readable info (item id, title, branch) is written to stderr.
The worktree path is written to stdout, making it easy to script:

  WORKTREE=$(wn worktree abc123)
  tmux new-window -c "$WORKTREE" "cursor $WORKTREE"

Settings from agent_orch (worktree_base, branch_prefix, claim) are reused.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeSetup(cmd, args, flags)
		},
	}
	cmd.Flags().StringVar(&flags.claim, "claim", "", "Claim duration (e.g. 2h). Overrides settings.")
	cmd.Flags().StringVar(&flags.branchPrefix, "branch-prefix", "", "Branch name prefix (e.g. keith/). Overrides settings.")
	cmd.Flags().StringVar(&flags.worktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	cmd.Flags().StringVar(&flags.tag, "tag", "", "Only consider items with this tag (with --next).")
	cmd.Flags().BoolVar(&flags.next, "next", false, "Claim the next undone item from the queue.")
	cmd.Flags().StringVar(&flags.branch, "branch", "", "Branch slug override (e.g. saved-views). Full name becomes [prefix]wn-<id>-<slug>. Overrides auto-generated slug.")
	return cmd
}

func runWorktreeSetup(_ *cobra.Command, args []string, f *worktreeSetupFlags) error {
	if f.next && len(args) > 0 {
		return fmt.Errorf("use either an id argument or --next, not both")
	}

	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	ws := cc.Settings.Worktree
	ns := cc.Settings.Next

	claimFor := 2 * time.Hour
	if ws.Claim != "" {
		if d, err := time.ParseDuration(ws.Claim); err == nil {
			claimFor = d
		}
	}
	if f.claim != "" {
		d, err := time.ParseDuration(f.claim)
		if err != nil {
			return fmt.Errorf("--claim: %w", err)
		}
		claimFor = d
	}

	branchPrefix := ws.BranchPrefix
	if f.branchPrefix != "" {
		branchPrefix = f.branchPrefix
	}
	branchTemplate := ws.BranchTemplate
	worktreesBase := ws.Base
	if f.worktreeBase != "" {
		worktreesBase = f.worktreeBase
	}
	tag := ns.Tag
	if f.tag != "" {
		tag = f.tag
	}

	absRoot, err := filepath.Abs(cc.Root)
	if err != nil {
		return err
	}
	if worktreesBase == "" {
		worktreesBase = filepath.Dir(absRoot)
	}
	mainDirname := filepath.Base(absRoot)

	var item *wn.Item
	switch {
	case len(args) > 0:
		item, err = cc.Store.Get(args[0])
		if err != nil {
			return fmt.Errorf("item %s not found", args[0])
		}
		if item.Done {
			return fmt.Errorf("item %s is already done", args[0])
		}
		if err := wn.ClaimItem(cc.Store, cc.Root, item.ID, claimFor, ""); err != nil {
			return err
		}
		item, err = cc.Store.Get(item.ID)
		if err != nil {
			return err
		}
	case f.next:
		item, err = wn.ClaimNextItem(cc.Store, cc.Root, claimFor, "", tag)
		if err != nil {
			return err
		}
		if item == nil {
			return fmt.Errorf("no items in queue")
		}
	default:
		meta, err := wn.ReadMeta(cc.Root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task (use wn pick, wn next, or wn worktree --next)")
		}
		item, err = cc.Store.Get(meta.CurrentID)
		if err != nil {
			return err
		}
		if item.Done {
			return fmt.Errorf("current item %s is already done", item.ID)
		}
		if err := wn.ClaimItem(cc.Store, cc.Root, item.ID, claimFor, ""); err != nil {
			return err
		}
		item, err = cc.Store.Get(item.ID)
		if err != nil {
			return err
		}
	}

	if f.branch != "" {
		slug := wn.BranchSlug(f.branch)
		fullBranch := branchPrefix + wn.ExpandBranchTemplate(branchTemplate, item.ID, slug)
		now := time.Now().UTC()
		if err = cc.Store.UpdateItem(item.ID, func(it *wn.Item) (*wn.Item, error) {
			idx := it.NoteIndexByName("branch")
			if idx >= 0 {
				it.Notes[idx].Body = fullBranch
			} else {
				it.Notes = append(it.Notes, wn.Note{Name: "branch", Created: now, Body: fullBranch})
			}
			it.Updated = now
			return it, nil
		}); err != nil {
			return fmt.Errorf("set branch note: %w", err)
		}
		item, err = cc.Store.Get(item.ID)
		if err != nil {
			return err
		}
	}

	worktreePath, branchName, err := wn.SetupItemWorktree(cc.Store, cc.Root, item, worktreesBase, mainDirname, branchPrefix, branchTemplate, os.Stderr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "claimed %s: %s\nbranch: %s\n", item.ID, wn.FirstLine(item.Description), branchName)
	fmt.Println(worktreePath)
	return nil
}
