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

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp [project_root]",
		Short: "Run MCP server on stdio (for Cursor and other MCP clients)",
		Long:  "Starts the Model Context Protocol server over stdin/stdout. Optional project_root is the directory containing .wn; when provided (or when WN_ROOT is set), the server is locked to that project and the per-request \"root\" parameter is ignored. No continuous process—exits when the client disconnects.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runMCP,
	}
}

func runMCP(cmd *cobra.Command, args []string) error {
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

func runDo(cmd *cobra.Command, args []string, f *doFlags) error {
	if f.maxTasks != 0 && !f.loop {
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

	if f.next && workID != "" {
		return fmt.Errorf("use either an id argument or --next, not both")
	}
	if f.loop && workID != "" {
		return fmt.Errorf("use either an id argument or --loop, not both")
	}

	opts := wn.AgentOrchOpts{
		Root:  root,
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
		opts.FailIfEmpty = true
		opts.MaxTasks = 1
	case f.loop:
		opts.MaxTasks = f.maxTasks
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

func runLaunch(cmd *cobra.Command, args []string, f *launchFlags) error {
	if f.maxTasks != 0 && !f.loop {
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
	case f.loop:
		orchMaxTasks = f.maxTasks
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
		LeaveWorktree: true,
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

func runWorktreeSetup(cmd *cobra.Command, args []string, f *worktreeSetupFlags) error {
	if f.next && len(args) > 0 {
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
	case f.next:
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

	if f.branch != "" {
		slug := wn.BranchSlug(f.branch)
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

func newSummaryCmd() *cobra.Command {
	return &cobra.Command{
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

type promptFlags struct {
	message string
}

func newPromptCmd() *cobra.Command {
	flags := &promptFlags{}
	cmd := &cobra.Command{
		Use:   "prompt [parent-id]",
		Short: "Create a prompt item (question for user) and add as dependency of parent",
		Long: `Creates a new prompt-state work item (a question for the user) and adds it as a
dependency of the parent item. The parent item becomes blocked until the user responds.

If parent-id is omitted, the current work item is used.
Use -m to provide the question inline, or $EDITOR will be opened.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrompt(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Question text (or open $EDITOR if omitted)")
	return cmd
}

func runPrompt(cmd *cobra.Command, args []string, f *promptFlags) error {
	msg := f.message
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
	if _, err := store.Get(parentID); err != nil {
		return err
	}
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

type respondFlags struct {
	message string
}

func newRespondCmd() *cobra.Command {
	flags := &respondFlags{}
	cmd := &cobra.Command{
		Use:   "respond [prompt-id]",
		Short: "Respond to a prompt item (marks it done and stores the response)",
		Long: `Marks a prompt-state work item as done and stores the response as a note.
This unblocks the parent item that was waiting for the response.

If prompt-id is omitted, the current work item is used.
Use -m to provide the answer inline, or $EDITOR will be opened.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRespond(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Response text (or open $EDITOR if omitted)")
	return cmd
}

func runRespond(cmd *cobra.Command, args []string, f *respondFlags) error {
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
	msg := f.message
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
