package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type launchFlags struct {
	next         bool
	loop         bool
	maxTasks     int
	claim        string
	worktreeBase string
	branch       string
	branchPrefix string
	tag          string
}

func newLaunchCmd() *cobra.Command {
	flags := &launchFlags{}
	cmd := &cobra.Command{
		Use:   "launch [runner] [id]",
		Short: "Dispatch agent on a work item asynchronously (fire-and-forget)",
		Long: `Set up the worktree for a work item and dispatch the configured launch command without waiting.
Intended for async workflows such as opening a new tmux window or launching an IDE.

  wn launch [runner] [id]  Dispatch for a specific item (or current if id omitted).
  wn launch --next         Dispatch for the next item in the queue.
  wn launch --loop         Continuously dispatch items from the queue (polls when empty).
  wn launch --loop -n N    Stop after dispatching N items.

Runner is resolved from settings.runners; defaults to agent.default_launch.`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLaunch(cmd, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.next, "next", false, "Dispatch for the next undone item from the queue.")
	cmd.Flags().BoolVar(&flags.loop, "loop", false, "Loop: continuously dispatch items (polls when queue empty).")
	cmd.Flags().IntVarP(&flags.maxTasks, "max-tasks", "n", 0, "Stop after dispatching N items (only with --loop; 0 = run indefinitely).")
	cmd.Flags().StringVar(&flags.claim, "claim", "", "Claim duration per item (e.g. 2h). Overrides settings.")
	cmd.Flags().StringVar(&flags.worktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	cmd.Flags().StringVar(&flags.branch, "branch", "", "Default branch override (e.g. main). Overrides settings.")
	cmd.Flags().StringVar(&flags.branchPrefix, "branch-prefix", "", "Prefix for generated branch names. Overrides settings.")
	cmd.Flags().StringVar(&flags.tag, "tag", "", "Only consider items with this tag (with --next or --loop). Overrides settings.")
	return cmd
}

func runLaunch(_ *cobra.Command, args []string, f *launchFlags) error {
	if f.maxTasks != 0 && !f.loop {
		return fmt.Errorf("-n / --max-tasks requires --loop")
	}

	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	ws := cc.Settings.Worktree
	as := cc.Settings.Agent
	ns := cc.Settings.Next

	var runnerName, workID string
	switch len(args) {
	case 2:
		runnerName = args[0]
		workID = args[1]
	case 1:
		if _, ok := cc.Settings.Runners[args[0]]; ok {
			runnerName = args[0]
		} else {
			workID = args[0]
		}
	}

	if f.next && workID != "" {
		return fmt.Errorf("use either an id argument or --next, not both")
	}
	if f.loop && workID != "" {
		return fmt.Errorf("use either an id argument or --loop, not both")
	}

	tag := ns.Tag
	if f.tag != "" {
		tag = f.tag
	}
	var orchWorkID string
	var orchFailIfEmpty bool
	var orchMaxTasks int
	switch {
	case f.next:
		// If the current item is still undone (not done or review-ready), re-use it rather
		// than picking a new item from the queue. This supports running "wn launch --next"
		// repeatedly while using "wn pick" to advance to the next item.
		meta, err := wn.ReadMeta(cc.Root)
		if err != nil {
			return err
		}
		if meta.CurrentID != "" {
			curStore, err := wn.NewFileStore(cc.Root)
			if err != nil {
				return err
			}
			if it, getErr := curStore.Get(meta.CurrentID); getErr == nil && !it.Done && !it.ReviewReady {
				orchWorkID = meta.CurrentID
				break
			}
		}
		orchFailIfEmpty = true
		orchMaxTasks = 1
	case f.loop:
		orchMaxTasks = f.maxTasks
	case workID != "":
		orchWorkID = workID
	default:
		meta, err := wn.ReadMeta(cc.Root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task (use wn pick or wn next first)")
		}
		orchWorkID = meta.CurrentID
	}

	runner, err := wn.ResolveLaunchRunner(cc.Settings, runnerName)
	if err != nil {
		return err
	}

	opts := wn.AgentOrchOpts{
		Root:        cc.Root,
		Audit:       os.Stderr,
		Async:       true,
		AgentCmd:    runner.Cmd,
		PromptTpl:   runner.Prompt,
		WorkID:      orchWorkID,
		FailIfEmpty: orchFailIfEmpty,
		MaxTasks:    orchMaxTasks,
		Tag:         tag,
	}

	if ws.Claim != "" {
		if d, err := time.ParseDuration(ws.Claim); err == nil {
			opts.ClaimFor = d
		}
	}
	if f.claim != "" {
		d, err := time.ParseDuration(f.claim)
		if err != nil {
			return fmt.Errorf("--claim: %w", err)
		}
		opts.ClaimFor = d
	}
	if opts.ClaimFor == 0 {
		opts.ClaimFor = 2 * time.Hour
	}

	if as.Poll != "" {
		if d, err := time.ParseDuration(as.Poll); err == nil {
			opts.Poll = d
		}
	}
	if opts.Poll == 0 {
		opts.Poll = 60 * time.Second
	}

	if ws.Base != "" {
		opts.WorktreesBase = ws.Base
	}
	if f.worktreeBase != "" {
		opts.WorktreesBase = f.worktreeBase
	}
	if ws.DefaultBranch != "" {
		opts.DefaultBranch = ws.DefaultBranch
	}
	if f.branch != "" {
		opts.DefaultBranch = f.branch
	}
	if ws.BranchPrefix != "" {
		opts.BranchPrefix = ws.BranchPrefix
	}
	if f.branchPrefix != "" {
		opts.BranchPrefix = f.branchPrefix
	}
	if ws.BranchTemplate != "" {
		opts.BranchTemplate = ws.BranchTemplate
	}

	ctx := context.Background()
	if err := wn.RunAgentOrch(ctx, opts); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "launched %s\n", opts.WorkID)
	return nil
}
