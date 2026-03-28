package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp [project_root]",
	Short: "Run MCP server on stdio (for Cursor and other MCP clients)",
	Long:  "Starts the Model Context Protocol server over stdin/stdout. Optional project_root is the directory containing .wn; when provided (or when WN_ROOT is set), the server is locked to that project and the per-request \"root\" parameter is ignored. No continuous process—exits when the client disconnects.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMCP,
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Fixed root: spawn-time arg wins, then WN_ROOT env, else no lock (tools use cwd or request "root").
	if len(args) > 0 {
		wn.SetMCPFixedRoot(args[0])
	} else if r := os.Getenv("WN_ROOT"); r != "" {
		wn.SetMCPFixedRoot(r)
	}
	server := wn.NewMCPServer()
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		return err
	}
	return nil
}

var doCmd = &cobra.Command{
	Use:   "do [runner] [id]",
	Short: "Run agent on a work item; optionally loop through the queue",
	Long: `Run a headless agent on a work item, then exit.

  wn do [runner] [id]  Run agent on the current item (or a specific id), then exit.
  wn do --next         Claim the next item from the queue, run once, then exit. Fails immediately if the queue is empty.
  wn do --loop         Continuously claim and process items from the queue (polls when empty).
  wn do --loop -n N    Stop after processing N items.

Runner is resolved from settings.runners; defaults to agent.default.`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runDo,
}

var (
	doNext         bool
	doLoop         bool
	doMaxTasks     int
	doClaim        string
	doDelay        string
	doPoll         string
	doWorktreeBase string
	doBranch       string
	doBranchPrefix string
	doTag          string
)

func init() {
	doCmd.Flags().BoolVar(&doNext, "next", false, "Claim the next undone item from the queue, run once, then exit. Errors if queue is empty.")
	doCmd.Flags().BoolVar(&doLoop, "loop", false, "Loop: continuously claim and process items (polls when queue empty).")
	doCmd.Flags().IntVarP(&doMaxTasks, "max-tasks", "n", 0, "Stop after processing N items (only with --loop; 0 = run indefinitely).")
	doCmd.Flags().StringVar(&doClaim, "claim", "", "Claim duration per item (e.g. 2h). Overrides settings.")
	doCmd.Flags().StringVar(&doDelay, "delay", "", "Delay between runs (e.g. 5m). Overrides settings.")
	doCmd.Flags().StringVar(&doPoll, "poll", "", "Poll interval when queue empty (e.g. 60s). Overrides settings.")
	doCmd.Flags().StringVar(&doWorktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	doCmd.Flags().StringVar(&doBranch, "branch", "", "Default branch override (e.g. main). Overrides settings.")
	doCmd.Flags().StringVar(&doBranchPrefix, "branch-prefix", "", "Prefix for generated branch names (e.g. keith/). Overrides settings.")
	doCmd.Flags().StringVar(&doTag, "tag", "", "Only consider items with this tag (queue modes). Overrides settings.")
}

func runDo(cmd *cobra.Command, args []string) error {
	// Read flags fresh; reset immediately to avoid persistence across test Execute() calls.
	isNext, _ := cmd.Flags().GetBool("next")
	isLoop, _ := cmd.Flags().GetBool("loop")
	maxTasks, _ := cmd.Flags().GetInt("max-tasks")
	flagClaim, _ := cmd.Flags().GetString("claim")
	flagDelay, _ := cmd.Flags().GetString("delay")
	flagPoll, _ := cmd.Flags().GetString("poll")
	flagWorktreeBase, _ := cmd.Flags().GetString("worktree-base")
	flagBranch, _ := cmd.Flags().GetString("branch")
	flagBranchPrefix, _ := cmd.Flags().GetString("branch-prefix")
	flagTag, _ := cmd.Flags().GetString("tag")

	_ = cmd.Flags().Set("next", "false")
	_ = cmd.Flags().Set("loop", "false")
	_ = cmd.Flags().Set("max-tasks", "0")
	_ = cmd.Flags().Set("claim", "")
	_ = cmd.Flags().Set("delay", "")
	_ = cmd.Flags().Set("poll", "")
	_ = cmd.Flags().Set("worktree-base", "")
	_ = cmd.Flags().Set("branch", "")
	_ = cmd.Flags().Set("branch-prefix", "")
	_ = cmd.Flags().Set("tag", "")

	if maxTasks != 0 && !isLoop {
		return fmt.Errorf("-n / --max-tasks requires --loop")
	}

	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return err
	}
	ws := settings.Worktree
	as := settings.Agent
	ns := settings.Next

	// Parse positional args: optional runner name and/or item ID.
	// With 2 args: first = runner, second = item ID.
	// With 1 arg: if arg matches a runner name, treat as runner; else treat as item ID.
	var runnerName, workID string
	switch len(args) {
	case 2:
		runnerName = args[0]
		workID = args[1]
	case 1:
		if _, ok := settings.Runners[args[0]]; ok {
			runnerName = args[0]
		} else {
			workID = args[0]
		}
	}

	if isNext && workID != "" {
		return fmt.Errorf("use either an id argument or --next, not both")
	}
	if isLoop && workID != "" {
		return fmt.Errorf("use either an id argument or --loop, not both")
	}

	opts := wn.AgentOrchOpts{
		Root:  root,
		Audit: os.Stderr,
	}

	// Apply settings defaults
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

	// Flag overrides
	if flagClaim != "" {
		d, err := time.ParseDuration(flagClaim)
		if err != nil {
			return fmt.Errorf("--claim: %w", err)
		}
		opts.ClaimFor = d
	}
	if flagDelay != "" {
		d, err := time.ParseDuration(flagDelay)
		if err != nil {
			return fmt.Errorf("--delay: %w", err)
		}
		opts.Delay = d
	}
	if flagPoll != "" {
		d, err := time.ParseDuration(flagPoll)
		if err != nil {
			return fmt.Errorf("--poll: %w", err)
		}
		opts.Poll = d
	}
	if flagWorktreeBase != "" {
		opts.WorktreesBase = flagWorktreeBase
	}
	if flagBranch != "" {
		opts.DefaultBranch = flagBranch
	}
	if flagBranchPrefix != "" {
		opts.BranchPrefix = flagBranchPrefix
	}
	if flagTag != "" {
		opts.Tag = flagTag
	}

	// Defaults when still zero
	if opts.ClaimFor == 0 {
		opts.ClaimFor = 2 * time.Hour
	}
	if opts.Poll == 0 {
		opts.Poll = 60 * time.Second
	}

	// Determine mode and work item before resolving runner.
	switch {
	case isNext:
		// If the current item is still undone (not done or review-ready), re-use it rather
		// than picking a new item from the queue. This supports running "wn do --next"
		// repeatedly while using "wn pick" to advance to the next item.
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		if meta.CurrentID != "" {
			curStore, err := wn.NewFileStore(root)
			if err != nil {
				return err
			}
			if it, getErr := curStore.Get(meta.CurrentID); getErr == nil && !it.Done && !it.ReviewReady {
				opts.WorkID = meta.CurrentID
				break
			}
		}
		// --next: claim next from queue, run once, fail if empty
		opts.FailIfEmpty = true
		opts.MaxTasks = 1
	case isLoop:
		// --loop: queue mode, poll when empty
		opts.MaxTasks = maxTasks // 0 = indefinite
	case workID != "":
		opts.WorkID = workID
	default:
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task (use wn pick or wn next first)")
		}
		opts.WorkID = meta.CurrentID
	}

	// Resolve runner and apply to opts
	runner, err := wn.ResolveRunner(settings, runnerName)
	if err != nil {
		return err
	}
	opts.AgentCmd = runner.Cmd
	opts.PromptTpl = runner.Prompt
	opts.LeaveWorktree = runner.LeaveWorktree

	ctx := context.Background()
	return wn.RunAgentOrch(ctx, opts)
}

var launchCmd = &cobra.Command{
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
	RunE: runLaunch,
}

var (
	launchNext         bool
	launchLoop         bool
	launchMaxTasks     int
	launchClaim        string
	launchWorktreeBase string
	launchBranch       string
	launchBranchPrefix string
	launchTag          string
)

func init() {
	launchCmd.Flags().BoolVar(&launchNext, "next", false, "Dispatch for the next undone item from the queue.")
	launchCmd.Flags().BoolVar(&launchLoop, "loop", false, "Loop: continuously dispatch items (polls when queue empty).")
	launchCmd.Flags().IntVarP(&launchMaxTasks, "max-tasks", "n", 0, "Stop after dispatching N items (only with --loop; 0 = run indefinitely).")
	launchCmd.Flags().StringVar(&launchClaim, "claim", "", "Claim duration per item (e.g. 2h). Overrides settings.")
	launchCmd.Flags().StringVar(&launchWorktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	launchCmd.Flags().StringVar(&launchBranch, "branch", "", "Default branch override (e.g. main). Overrides settings.")
	launchCmd.Flags().StringVar(&launchBranchPrefix, "branch-prefix", "", "Prefix for generated branch names. Overrides settings.")
	launchCmd.Flags().StringVar(&launchTag, "tag", "", "Only consider items with this tag (with --next or --loop). Overrides settings.")
}

func runLaunch(cmd *cobra.Command, args []string) error {
	isNext, _ := cmd.Flags().GetBool("next")
	isLoop, _ := cmd.Flags().GetBool("loop")
	maxTasks, _ := cmd.Flags().GetInt("max-tasks")
	flagClaim, _ := cmd.Flags().GetString("claim")
	flagWorktreeBase, _ := cmd.Flags().GetString("worktree-base")
	flagBranch, _ := cmd.Flags().GetString("branch")
	flagBranchPrefix, _ := cmd.Flags().GetString("branch-prefix")
	flagTag, _ := cmd.Flags().GetString("tag")

	_ = cmd.Flags().Set("next", "false")
	_ = cmd.Flags().Set("loop", "false")
	_ = cmd.Flags().Set("max-tasks", "0")
	_ = cmd.Flags().Set("claim", "")
	_ = cmd.Flags().Set("worktree-base", "")
	_ = cmd.Flags().Set("branch", "")
	_ = cmd.Flags().Set("branch-prefix", "")
	_ = cmd.Flags().Set("tag", "")

	if maxTasks != 0 && !isLoop {
		return fmt.Errorf("-n / --max-tasks requires --loop")
	}

	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return err
	}
	ws := settings.Worktree
	as := settings.Agent
	ns := settings.Next

	// Parse positional args: optional runner name and/or item ID.
	var runnerName, workID string
	switch len(args) {
	case 2:
		runnerName = args[0]
		workID = args[1]
	case 1:
		if _, ok := settings.Runners[args[0]]; ok {
			runnerName = args[0]
		} else {
			workID = args[0]
		}
	}

	if isNext && workID != "" {
		return fmt.Errorf("use either an id argument or --next, not both")
	}
	if isLoop && workID != "" {
		return fmt.Errorf("use either an id argument or --loop, not both")
	}

	// Determine the work item (or validate current task) before resolving runner.
	tag := ns.Tag
	if flagTag != "" {
		tag = flagTag
	}
	var orchWorkID string
	var orchFailIfEmpty bool
	var orchMaxTasks int
	switch {
	case isNext:
		// If the current item is still undone (not done or review-ready), re-use it rather
		// than picking a new item from the queue. This supports running "wn launch --next"
		// repeatedly while using "wn pick" to advance to the next item.
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		if meta.CurrentID != "" {
			curStore, err := wn.NewFileStore(root)
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
	case isLoop:
		orchMaxTasks = maxTasks // 0 = indefinite
	case workID != "":
		orchWorkID = workID
	default:
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task (use wn pick or wn next first)")
		}
		orchWorkID = meta.CurrentID
	}

	runner, err := wn.ResolveLaunchRunner(settings, runnerName)
	if err != nil {
		return err
	}

	opts := wn.AgentOrchOpts{
		Root:          root,
		Audit:         os.Stderr,
		Async:         true,
		AgentCmd:      runner.Cmd,
		PromptTpl:     runner.Prompt,
		LeaveWorktree: true, // always leave worktree for async dispatch
		WorkID:        orchWorkID,
		FailIfEmpty:   orchFailIfEmpty,
		MaxTasks:      orchMaxTasks,
		Tag:           tag,
	}

	if ws.Claim != "" {
		if d, err := time.ParseDuration(ws.Claim); err == nil {
			opts.ClaimFor = d
		}
	}
	if flagClaim != "" {
		d, err := time.ParseDuration(flagClaim)
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
	if flagWorktreeBase != "" {
		opts.WorktreesBase = flagWorktreeBase
	}
	if ws.DefaultBranch != "" {
		opts.DefaultBranch = ws.DefaultBranch
	}
	if flagBranch != "" {
		opts.DefaultBranch = flagBranch
	}
	if ws.BranchPrefix != "" {
		opts.BranchPrefix = ws.BranchPrefix
	}
	if flagBranchPrefix != "" {
		opts.BranchPrefix = flagBranchPrefix
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

var worktreeSetupCmd = &cobra.Command{
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
	RunE: runWorktreeSetup,
}

var (
	worktreeSetupClaim        string
	worktreeSetupBranchPrefix string
	worktreeSetupWorktreeBase string
	worktreeSetupTag          string
	worktreeSetupNext         bool
	worktreeSetupBranch       string
)

func init() {
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupClaim, "claim", "", "Claim duration (e.g. 2h). Overrides settings.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupBranchPrefix, "branch-prefix", "", "Branch name prefix (e.g. keith/). Overrides settings.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupWorktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupTag, "tag", "", "Only consider items with this tag (with --next).")
	worktreeSetupCmd.Flags().BoolVar(&worktreeSetupNext, "next", false, "Claim the next undone item from the queue.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupBranch, "branch", "", "Branch slug override (e.g. saved-views). Full name becomes [prefix]wn-<id>-<slug>. Overrides auto-generated slug.")
}

func runWorktreeSetup(cmd *cobra.Command, args []string) error {
	// Read all flags from cmd directly; package-level flag vars may retain stale values
	// across successive Execute() calls (e.g. in tests).
	isNext, _ := cmd.Flags().GetBool("next")
	flagClaim, _ := cmd.Flags().GetString("claim")
	flagBranchPrefix, _ := cmd.Flags().GetString("branch-prefix")
	flagWorktreeBase, _ := cmd.Flags().GetString("worktree-base")
	flagTag, _ := cmd.Flags().GetString("tag")
	flagBranch, _ := cmd.Flags().GetString("branch")

	// Reset flags so they don't persist across test invocations.
	_ = cmd.Flags().Set("next", "false")
	_ = cmd.Flags().Set("claim", "")
	_ = cmd.Flags().Set("branch-prefix", "")
	_ = cmd.Flags().Set("worktree-base", "")
	_ = cmd.Flags().Set("tag", "")
	_ = cmd.Flags().Set("branch", "")

	if isNext && len(args) > 0 {
		return fmt.Errorf("use either an id argument or --next, not both")
	}

	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return err
	}
	ws := settings.Worktree
	ns := settings.Next

	claimFor := 2 * time.Hour
	if ws.Claim != "" {
		if d, err := time.ParseDuration(ws.Claim); err == nil {
			claimFor = d
		}
	}
	if flagClaim != "" {
		d, err := time.ParseDuration(flagClaim)
		if err != nil {
			return fmt.Errorf("--claim: %w", err)
		}
		claimFor = d
	}

	branchPrefix := ws.BranchPrefix
	if flagBranchPrefix != "" {
		branchPrefix = flagBranchPrefix
	}
	branchTemplate := ws.BranchTemplate
	worktreesBase := ws.Base
	if flagWorktreeBase != "" {
		worktreesBase = flagWorktreeBase
	}
	tag := ns.Tag
	if flagTag != "" {
		tag = flagTag
	}

	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
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
		item, err = store.Get(args[0])
		if err != nil {
			return fmt.Errorf("item %s not found", args[0])
		}
		if item.Done {
			return fmt.Errorf("item %s is already done", args[0])
		}
		if err := wn.ClaimItem(store, root, item.ID, claimFor, ""); err != nil {
			return err
		}
		item, err = store.Get(item.ID)
		if err != nil {
			return err
		}
	case isNext:
		item, err = wn.ClaimNextItem(store, root, claimFor, "", tag)
		if err != nil {
			return err
		}
		if item == nil {
			return fmt.Errorf("no items in queue")
		}
	default:
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task (use wn pick, wn next, or wn worktree --next)")
		}
		item, err = store.Get(meta.CurrentID)
		if err != nil {
			return err
		}
		if item.Done {
			return fmt.Errorf("current item %s is already done", item.ID)
		}
		if err := wn.ClaimItem(store, root, item.ID, claimFor, ""); err != nil {
			return err
		}
		item, err = store.Get(item.ID)
		if err != nil {
			return err
		}
	}

	// If --branch is provided, pre-set the branch note so SetupItemWorktree uses it.
	// The slug comes from the flag value; the full name is generated via the branch template.
	if flagBranch != "" {
		slug := wn.BranchSlug(flagBranch)
		fullBranch := branchPrefix + wn.ExpandBranchTemplate(branchTemplate, item.ID, slug)
		now := time.Now().UTC()
		if err = store.UpdateItem(item.ID, func(it *wn.Item) (*wn.Item, error) {
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
		item, err = store.Get(item.ID)
		if err != nil {
			return err
		}
	}

	worktreePath, branchName, err := wn.SetupItemWorktree(store, root, item, worktreesBase, mainDirname, branchPrefix, branchTemplate, os.Stderr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "claimed %s: %s\nbranch: %s\n", item.ID, wn.FirstLine(item.Description), branchName)
	fmt.Println(worktreePath)
	return nil
}

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show a summary dashboard: counts by status and tag",
	Long: `Show aggregate counts of work items by status and by tag.

Example output:
  status      count
  undone          12
  blocked          3
  review           2
  done            47

  tag              undone  blocked   review
  agent                 5        1        1
  backend               4        2        0
  (no tags)             3        0        1

Useful for a quick project health check without scrolling through all items.`,
	Args: cobra.NoArgs,
	RunE: runSummary,
}

func runSummary(_ *cobra.Command, _ []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	allItems, err := store.List()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	blockedSet := wn.BlockedSet(allItems)

	// Count items by status.
	statusCounts := make(map[string]int)
	for _, it := range allItems {
		status := wn.ItemListStatus(it, now, blockedSet[it.ID])
		statusCounts[status]++
	}
	statusOrder := []string{"undone", "blocked", "claimed", "prompt", "review", "done", "closed", "suspend"}
	fmt.Printf("  %-10s  %s\n", "status", "count")
	for _, s := range statusOrder {
		if n := statusCounts[s]; n > 0 {
			fmt.Printf("  %-10s  %d\n", s, n)
		}
	}

	// Tag breakdown for active statuses (undone, blocked, review).
	type tagRow struct{ undone, blocked, review int }
	tagMap := make(map[string]*tagRow)
	var noTagRow tagRow
	for _, it := range allItems {
		status := wn.ItemListStatus(it, now, blockedSet[it.ID])
		if status != "undone" && status != "blocked" && status != "review" {
			continue
		}
		if len(it.Tags) == 0 {
			switch status {
			case "undone":
				noTagRow.undone++
			case "blocked":
				noTagRow.blocked++
			case "review":
				noTagRow.review++
			}
			continue
		}
		for _, tag := range it.Tags {
			if tagMap[tag] == nil {
				tagMap[tag] = &tagRow{}
			}
			switch status {
			case "undone":
				tagMap[tag].undone++
			case "blocked":
				tagMap[tag].blocked++
			case "review":
				tagMap[tag].review++
			}
		}
	}

	hasActive := len(tagMap) > 0 || noTagRow.undone+noTagRow.blocked+noTagRow.review > 0
	if !hasActive {
		return nil
	}

	tags := make([]string, 0, len(tagMap))
	tagColWidth := len("(no tags)")
	for t := range tagMap {
		tags = append(tags, t)
		if len(t) > tagColWidth {
			tagColWidth = len(t)
		}
	}
	sort.Strings(tags)

	fmt.Println()
	fmt.Printf("  %-*s  %7s  %7s  %7s\n", tagColWidth, "tag", "undone", "blocked", "review")
	for _, tag := range tags {
		tr := tagMap[tag]
		fmt.Printf("  %-*s  %7d  %7d  %7d\n", tagColWidth, tag, tr.undone, tr.blocked, tr.review)
	}
	if noTagRow.undone+noTagRow.blocked+noTagRow.review > 0 {
		fmt.Printf("  %-*s  %7d  %7d  %7d\n", tagColWidth, "(no tags)", noTagRow.undone, noTagRow.blocked, noTagRow.review)
	}
	return nil
}

var promptMessage string

var promptCmd = &cobra.Command{
	Use:   "prompt [parent-id]",
	Short: "Create a prompt item (question for user) and add as dependency of parent",
	Long: `Creates a new prompt-state work item (a question for the user) and adds it as a
dependency of the parent item. The parent item becomes blocked until the user responds.

If parent-id is omitted, the current work item is used.
Use -m to provide the question inline, or $EDITOR will be opened.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPrompt,
}

func init() {
	promptCmd.Flags().StringVarP(&promptMessage, "message", "m", "", "Question text (or open $EDITOR if omitted)")
}

func runPrompt(cmd *cobra.Command, args []string) error {
	msg := promptMessage
	if msg == "" {
		var err error
		msg, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("empty question")
		}
	}
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 0 {
		explicitID = args[0]
	}
	parentID, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	// Verify parent exists
	if _, err := store.Get(parentID); err != nil {
		return err
	}
	// Create the prompt item
	promptID, err := wn.GenerateID(store)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID:          promptID,
		Description: strings.TrimSpace(msg),
		Created:     now,
		Updated:     now,
		PromptReady: true,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}, {At: now, Kind: "prompt_ready"}},
	}
	if err := store.Put(promptItem); err != nil {
		return err
	}
	// Add prompt item as dependency of parent
	items, err := store.List()
	if err != nil {
		return err
	}
	if wn.WouldCreateCycle(items, parentID, promptID) {
		_ = store.Delete(promptID)
		return fmt.Errorf("circular dependency would result")
	}
	if err := store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, promptID)
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "depend_added", Msg: promptID})
		return it, nil
	}); err != nil {
		return err
	}
	fmt.Printf("created prompt %s; %s is now blocked\n", promptID, parentID)
	return nil
}

var respondMessage string

var respondCmd = &cobra.Command{
	Use:   "respond [prompt-id]",
	Short: "Respond to a prompt item (marks it done and stores the response)",
	Long: `Marks a prompt-state work item as done and stores the response as a note.
This unblocks the parent item that was waiting for the response.

If prompt-id is omitted, the current work item is used.
Use -m to provide the answer inline, or $EDITOR will be opened.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRespond,
}

func init() {
	respondCmd.Flags().StringVarP(&respondMessage, "message", "m", "", "Response text (or open $EDITOR if omitted)")
}

func runRespond(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 0 {
		explicitID = args[0]
	}
	id, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
	if err != nil {
		return err
	}
	if !item.PromptReady {
		return fmt.Errorf("item %s is not in prompt state", id)
	}
	msg := respondMessage
	if msg == "" {
		var err error
		msg, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("empty response")
		}
	}
	now := time.Now().UTC()
	if err := store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = true
		it.DoneStatus = wn.DoneStatusDone
		it.PromptReady = false
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: msg})
		// Store response as a note
		if it.Notes == nil {
			it.Notes = []wn.Note{}
		}
		idx := it.NoteIndexByName(wn.NoteNameResponse)
		trimmed := strings.TrimSpace(msg)
		if idx >= 0 {
			it.Notes[idx].Body = trimmed
		} else {
			it.Notes = append(it.Notes, wn.Note{Name: wn.NoteNameResponse, Created: now, Body: trimmed})
		}
		return it, nil
	}); err != nil {
		return err
	}
	fmt.Printf("responded to %s; prompt marked done\n", id)
	return nil
}
