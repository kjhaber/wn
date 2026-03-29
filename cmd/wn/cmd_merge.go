package main

import (
	"fmt"
	"strings"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type mergeFlags struct {
	message string
	dryRun  bool
}

func newMergeCmd() *cobra.Command {
	flags := &mergeFlags{}
	cmd := &cobra.Command{
		Use:   "merge <branch>",
		Short: "Squash-merge a feature branch, record commit hash, and mark item done",
		Long: `Squash-merge a feature branch into the current HEAD of the main worktree.

Steps performed:
  1. Find the wn item associated with <branch> via its wn:branch note
  2. Run git merge --squash <branch> in the main repo root
  3. Commit with the provided message (-m flag, or open $EDITOR if omitted)
  4. Record the squash commit hash as a wn:commit note (needed for cleanup worktrees)
  5. Mark the item done

This command is worktree-aware: git operations run in the main repo root
regardless of the current working directory.

Use --dry-run to preview what would happen without making any changes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMerge(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Commit message (opens $EDITOR if omitted)")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Show what would happen without making changes")
	return cmd
}

func runMerge(cmd *cobra.Command, args []string, flags *mergeFlags) error {
	branch := args[0]

	message := flags.message
	if message == "" {
		var err error
		message, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("empty commit message")
		}
	}

	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}

	result, err := wn.SquashMerge(store, root, branch, message, flags.dryRun)
	if err != nil {
		return err
	}

	if flags.dryRun {
		fmt.Printf("would squash-merge %s (item %s) into current HEAD\n", result.Branch, result.ItemID)
		return nil
	}

	fmt.Printf("merged %s → %s (item %s marked done)\n", result.Branch, result.CommitHash[:7], result.ItemID)
	return nil
}
