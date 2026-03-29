package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type doFlags struct {
	next         bool
	loop         bool
	maxTasks     int
	claim        string
	delay        string
	poll         string
	worktreeBase string
	branch       string
	branchPrefix string
	tag          string
}

func newDoCmd() *cobra.Command {
	flags := &doFlags{}
	cmd := &cobra.Command{
		Use:   "do [runner] [id]",
		Short: "Run agent on a work item; optionally loop through the queue",
		Long: `Run a headless agent on a work item, then exit.

  wn do [runner] [id]  Run agent on the current item (or a specific id), then exit.
  wn do --next         Claim the next item from the queue, run once, then exit. Fails immediately if the queue is empty.
  wn do --loop         Continuously claim and process items from the queue (polls when empty).
  wn do --loop -n N    Stop after processing N items.

Runner is resolved from settings.runners; defaults to agent.default.`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDo(cmd, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.next, "next", false, "Claim the next undone item from the queue, run once, then exit. Errors if queue is empty.")
	cmd.Flags().BoolVar(&flags.loop, "loop", false, "Loop: continuously claim and process items (polls when queue empty).")
	cmd.Flags().IntVarP(&flags.maxTasks, "max-tasks", "n", 0, "Stop after processing N items (only with --loop; 0 = run indefinitely).")
	cmd.Flags().StringVar(&flags.claim, "claim", "", "Claim duration per item (e.g. 2h). Overrides settings.")
	cmd.Flags().StringVar(&flags.delay, "delay", "", "Delay between runs (e.g. 5m). Overrides settings.")
	cmd.Flags().StringVar(&flags.poll, "poll", "", "Poll interval when queue empty (e.g. 60s). Overrides settings.")
	cmd.Flags().StringVar(&flags.worktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	cmd.Flags().StringVar(&flags.branch, "branch", "", "Default branch override (e.g. main). Overrides settings.")
	cmd.Flags().StringVar(&flags.branchPrefix, "branch-prefix", "", "Prefix for generated branch names (e.g. keith/). Overrides settings.")
	cmd.Flags().StringVar(&flags.tag, "tag", "", "Only consider items with this tag (queue modes). Overrides settings.")
	return cmd
}

func runDo(_ *cobra.Command, args []string, f *doFlags) error {
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

	opts := wn.AgentOrchOpts{
		Root:  cc.Root,
		Audit: os.Stderr,
	}

	if ws.Claim != "" {
		if d, err := time.ParseDuration(ws.Claim); err == nil {
			opts.ClaimFor = d
		}
	}
	if as.Delay != "" {
		if d, err := time.ParseDuration(as.Delay); err == nil {
			opts.Delay = d
		}
	}
	if as.Poll != "" {
		if d, err := time.ParseDuration(as.Poll); err == nil {
			opts.Poll = d
		}
	}
	if ws.Base != "" {
		opts.WorktreesBase = ws.Base
	}
	if ws.DefaultBranch != "" {
		opts.DefaultBranch = ws.DefaultBranch
	}
	if ws.BranchPrefix != "" {
		opts.BranchPrefix = ws.BranchPrefix
	}
	if ws.BranchTemplate != "" {
		opts.BranchTemplate = ws.BranchTemplate
	}
	if as.CommitTemplate != "" {
		opts.CommitTemplate = as.CommitTemplate
	}
	if ns.Tag != "" {
		opts.Tag = ns.Tag
	}

	if f.claim != "" {
		d, err := time.ParseDuration(f.claim)
		if err != nil {
			return fmt.Errorf("--claim: %w", err)
		}
		opts.ClaimFor = d
	}
	if f.delay != "" {
		d, err := time.ParseDuration(f.delay)
		if err != nil {
			return fmt.Errorf("--delay: %w", err)
		}
		opts.Delay = d
	}
	if f.poll != "" {
		d, err := time.ParseDuration(f.poll)
		if err != nil {
			return fmt.Errorf("--poll: %w", err)
		}
		opts.Poll = d
	}
	if f.worktreeBase != "" {
		opts.WorktreesBase = f.worktreeBase
	}
	if f.branch != "" {
		opts.DefaultBranch = f.branch
	}
	if f.branchPrefix != "" {
		opts.BranchPrefix = f.branchPrefix
	}
	if f.tag != "" {
		opts.Tag = f.tag
	}

	if opts.ClaimFor == 0 {
		opts.ClaimFor = 2 * time.Hour
	}
	if opts.Poll == 0 {
		opts.Poll = 60 * time.Second
	}

	switch {
	case f.next:
		// If the current item is still undone (not done or review-ready), re-use it rather
		// than picking a new item from the queue. This supports running "wn do --next"
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
				opts.WorkID = meta.CurrentID
				break
			}
		}
		opts.FailIfEmpty = true
		opts.MaxTasks = 1
	case f.loop:
		opts.MaxTasks = f.maxTasks
	case workID != "":
		opts.WorkID = workID
	default:
		meta, err := wn.ReadMeta(cc.Root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task (use wn pick or wn next first)")
		}
		opts.WorkID = meta.CurrentID
	}

	runner, err := wn.ResolveRunner(cc.Settings, runnerName)
	if err != nil {
		return err
	}
	opts.AgentCmd = runner.Cmd
	opts.PromptTpl = runner.Prompt
	opts.LeaveWorktree = runner.LeaveWorktree

	ctx := context.Background()
	return wn.RunAgentOrch(ctx, opts)
}
