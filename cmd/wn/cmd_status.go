package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type statusFlags struct {
	forDuration string
	message     string
	claimBy     string
	duplicateOf string
}

func newStatusCmd() *cobra.Command {
	flags := &statusFlags{}
	cmd := &cobra.Command{
		Use:   "status <undone|claimed|review|done|closed|suspend> [id]",
		Short: "Set work item status",
		Long:  "Set the work item to the given status. If id is omitted, uses the current task. Use --for when setting to claimed (duration, e.g. 30m); -m for a message when setting to done/closed/suspend. Use --duplicate-of <id> when setting to closed to mark the item as a duplicate of another (adds duplicate-of note).",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, flags)
		},
	}
	cmd.Flags().StringVar(&flags.forDuration, "for", "", "Claim duration when setting to claimed (e.g. 30m, 1h); default 1h")
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Optional message when setting to done, closed, or suspend")
	cmd.Flags().StringVar(&flags.claimBy, "by", "", "Optional worker ID when setting to claimed")
	cmd.Flags().StringVar(&flags.duplicateOf, "duplicate-of", "", "When setting to closed: mark item as duplicate of this work item id (adds duplicate-of note)")
	return cmd
}

func runStatus(cmd *cobra.Command, args []string, flags *statusFlags) error {
	state := args[0]
	if !wn.ValidStatus(state) {
		return fmt.Errorf("invalid status %q; must be one of: undone, claimed, review, done, closed, suspend", state)
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 1 {
		explicitID = args[1]
	}
	id, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	if state != wn.StatusClosed && flags.duplicateOf != "" {
		return fmt.Errorf("--duplicate-of is only valid when setting status to closed")
	}
	opts := wn.StatusOpts{DoneMessage: flags.message, ClaimBy: flags.claimBy, DuplicateOf: flags.duplicateOf}
	if state == wn.StatusClaimed && flags.forDuration != "" {
		d, err := time.ParseDuration(flags.forDuration)
		if err != nil {
			return fmt.Errorf("invalid --for duration %q: %w", flags.forDuration, err)
		}
		if d <= 0 {
			return fmt.Errorf("--for duration must be positive, got %v", d)
		}
		opts.ClaimFor = d
	}
	if err := wn.SetStatus(cc.Store, id, state, opts); err != nil {
		return err
	}
	if state == wn.StatusClosed && flags.duplicateOf != "" {
		fmt.Printf("marked %s as duplicate of %s\n", id, flags.duplicateOf)
	} else {
		fmt.Printf("marked %s %s\n", id, state)
	}
	return nil
}

type claimFlags struct {
	forDuration string
	by          string
}

func newClaimCmd() *cobra.Command {
	flags := &claimFlags{}
	cmd := &cobra.Command{
		Use:   "claim [id]",
		Short: "Mark a work item in progress (exclusive until expiration)",
		Long:  "Claims the item so it leaves the undone list until --for duration expires or you run wn done/release. If id is omitted, uses current task. Omit --for to use default (1h) and renew/extend a claim without losing context.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClaim(cmd, args, flags)
		},
	}
	cmd.Flags().StringVar(&flags.forDuration, "for", "", "Duration the claim is held (e.g. 30m, 1h); default 1h so you can renew with just wn claim")
	cmd.Flags().StringVar(&flags.by, "by", "", "Optional worker ID for logging")
	return cmd
}

func runClaim(cmd *cobra.Command, args []string, flags *claimFlags) error {
	d := wn.DefaultClaimDuration
	if flags.forDuration != "" {
		var err error
		d, err = time.ParseDuration(flags.forDuration)
		if err != nil {
			return fmt.Errorf("invalid --for duration %q: %w", flags.forDuration, err)
		}
		if d <= 0 {
			return fmt.Errorf("--for duration must be positive, got %v", d)
		}
	}
	claimForMsg := flags.forDuration
	if claimForMsg == "" {
		claimForMsg = d.String()
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 0 {
		explicitID = args[0]
	}
	id, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task; use wn pick or wn next")
	}
	now := time.Now().UTC()
	until := now.Add(d)
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.InProgressUntil = until
		it.InProgressBy = flags.by
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "in_progress", Msg: claimForMsg})
		return it, nil
	})
}

func newReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release [id]",
		Short: "Clear in-progress on a work item (return to undone list)",
		Long:  "If id is omitted, releases the current task.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runRelease,
	}
}

func runRelease(cmd *cobra.Command, args []string) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
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
	now := time.Now().UTC()
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.ReviewReady = true
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "released"})
		return it, nil
	})
}

func newReviewReadyCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "review-ready [id]",
		Aliases: []string{"rr"},
		Short:   "Set work item to review-ready (excluded from wn next until marked done)",
		Long:    "If id is omitted, uses the current task. Clears in-progress and marks the item review-ready.",
		Args:    cobra.MaximumNArgs(1),
		RunE:    runReviewReady,
	}
}

func runReviewReady(cmd *cobra.Command, args []string) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
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
	now := time.Now().UTC()
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = false
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.ReviewReady = true
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "review_ready"})
		return it, nil
	})
}

type cleanupMergedFlags struct {
	dryRun bool
	branch string
}

type cleanupCloseDoneFlags struct {
	age    string
	dryRun bool
}

type cleanupWorktreesFlags struct {
	dryRun        bool
	branch        string
	cleanIgnored  bool
	force         bool
	worktreesOnly bool
}

func newCleanupCmd() *cobra.Command {
	cleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Bulk maintenance utilities for work items",
	}

	mergedF := &cleanupMergedFlags{}
	cleanupSetMergedReviewItemsDoneCmd := &cobra.Command{
		Use:   "set-merged-review-items-done",
		Short: "Mark review items done when their work has been merged",
		Long:  "Checks all review-ready work items, finds their 'branch' note, and marks them done if that branch (or recorded commit) has been merged into the current branch (or --branch). Use --dry-run to see what would be marked without making changes.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupSetMergedReviewItemsDone(cmd, args, mergedF)
		},
	}
	cleanupSetMergedReviewItemsDoneCmd.Flags().BoolVar(&mergedF.dryRun, "dry-run", false, "Report what would be marked without making changes")
	cleanupSetMergedReviewItemsDoneCmd.Flags().StringVarP(&mergedF.branch, "branch", "b", "", "Check merged into this ref (default: current HEAD)")

	closeF := &cleanupCloseDoneFlags{}
	cleanupCloseDoneItemsCmd := &cobra.Command{
		Use:   "close-done-items",
		Short: "Close done items older than a configurable age",
		Long:  "Finds items in done state whose done time is older than the configured age and sets them to closed. Age comes from --age or settings cleanup.close_done_items_age.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupCloseDoneItems(cmd, args, closeF)
		},
	}
	cleanupCloseDoneItemsCmd.Flags().StringVar(&closeF.age, "age", "", "Age threshold (e.g. 30d, 7d, 48h); items done longer ago are closed")
	cleanupCloseDoneItemsCmd.Flags().BoolVar(&closeF.dryRun, "dry-run", false, "Report what would be closed without making changes")

	wtF := &cleanupWorktreesFlags{}
	cleanupWorktreesCmd := &cobra.Command{
		Use:   "worktrees",
		Short: "Remove completed worktrees and branches whose work is merged",
		Long:  "Finds all non-main git worktrees whose associated wn item is done and whose branch has been merged into the current HEAD (or --branch). Removes those worktrees and deletes their branches. Also finds and deletes orphaned branches (branches with no worktree whose item is done and merged). Use --worktrees-only to skip branch deletion. Use --clean-ignored to also remove gitignored files (build artifacts, temp config) before removal. Use --dry-run to preview without making changes.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupWorktrees(cmd, args, wtF)
		},
	}
	cleanupWorktreesCmd.Flags().BoolVar(&wtF.dryRun, "dry-run", false, "Report what would be removed without making changes")
	cleanupWorktreesCmd.Flags().StringVarP(&wtF.branch, "branch", "b", "", "Check merged into this ref (default: current HEAD)")
	cleanupWorktreesCmd.Flags().BoolVar(&wtF.cleanIgnored, "clean-ignored", false, "Remove gitignored files (e.g. build artifacts) from each worktree before removal")
	cleanupWorktreesCmd.Flags().BoolVar(&wtF.force, "force", false, "Skip confirmation prompt and remove immediately")
	cleanupWorktreesCmd.Flags().BoolVar(&wtF.worktreesOnly, "worktrees-only", false, "Remove worktrees but do not delete their branches")

	cleanupCmd.AddCommand(cleanupSetMergedReviewItemsDoneCmd, cleanupCloseDoneItemsCmd, cleanupWorktreesCmd)
	return cleanupCmd
}

// promptYN writes prompt to w and reads one line from r.
// Returns true only if the line is "y" or "Y". Defaults to false (no).
func promptYN(r io.Reader, w io.Writer, prompt string) bool {
	_, _ = fmt.Fprintf(w, "%s [y/N]: ", prompt)
	var line string
	_, _ = fmt.Fscanln(r, &line)
	return strings.EqualFold(strings.TrimSpace(line), "y")
}

func runCleanupSetMergedReviewItemsDone(cmd *cobra.Command, args []string, f *cleanupMergedFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	intoRef := f.branch
	results, err := wn.MarkMergedItems(cc.Store, cc.Root, intoRef, f.dryRun)
	if err != nil {
		return err
	}
	for _, r := range results {
		switch r.Status {
		case "marked":
			prefix := "marked"
			if f.dryRun {
				prefix = "would mark"
			}
			fmt.Printf("%s %s: %s\n", prefix, r.ID, r.Reason)
		case "skipped_no_branch":
			fmt.Printf("skip %s: %s\n", r.ID, r.Reason)
		case "skipped_not_merged":
			fmt.Printf("skip %s: %s\n", r.ID, r.Reason)
		case "skipped_error":
			fmt.Fprintf(os.Stderr, "skip %s: %s\n", r.ID, r.Reason)
		}
	}
	return nil
}

func runCleanupCloseDoneItems(cmd *cobra.Command, args []string, f *cleanupCloseDoneFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	ageStr := f.age
	if ageStr == "" {
		ageStr = cc.Settings.Cleanup.CloseDoneItemsAge
	}
	if ageStr == "" {
		return fmt.Errorf("--age is required when cleanup.close_done_items_age is not set in settings")
	}
	age, err := wn.ParseDurationWithDays(ageStr)
	if err != nil {
		return fmt.Errorf("invalid age %q: %w", ageStr, err)
	}
	if age <= 0 {
		return fmt.Errorf("age must be positive, got %v", age)
	}
	cutoff := time.Now().UTC().Add(-age)
	results, err := wn.CloseDoneItems(cc.Store, cutoff, f.dryRun)
	if err != nil {
		return err
	}
	for _, r := range results {
		switch r.Status {
		case "closed":
			prefix := "closed"
			if f.dryRun {
				prefix = "would close"
			}
			fmt.Printf("%s %s: %s\n", prefix, r.ID, r.Reason)
		case "skipped_not_done", "skipped_not_old_enough":
			fmt.Printf("skip %s: %s\n", r.ID, r.Reason)
		default:
			fmt.Printf("%s %s: %s\n", r.Status, r.ID, r.Reason)
		}
	}
	return nil
}

func runCleanupWorktrees(cmd *cobra.Command, args []string, f *cleanupWorktreesFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}

	preview, err := wn.CleanupWorktrees(cc.Store, cc.Root, f.branch, true, f.cleanIgnored, f.worktreesOnly, nil)
	if err != nil {
		return err
	}
	var eligible []wn.CleanupWorktreeResult
	for _, r := range preview {
		if r.Status == "removed" || r.Status == "branch_deleted" {
			eligible = append(eligible, r)
		}
	}

	if len(eligible) == 0 {
		fmt.Println("no worktrees or branches to clean up")
		return nil
	}

	for _, r := range eligible {
		label := r.Branch
		if r.ItemID != "" {
			label = r.ItemID + " (" + r.Branch + ")"
		}
		if r.Status == "branch_deleted" {
			fmt.Printf("  branch %s (orphaned)\n", label)
		} else {
			fmt.Printf("  %s: %s\n", label, r.Path)
		}
	}

	if f.dryRun {
		worktreeCount := 0
		branchCount := 0
		for _, r := range eligible {
			if r.Status == "removed" {
				worktreeCount++
			} else {
				branchCount++
			}
		}
		parts := []string{}
		if worktreeCount > 0 {
			parts = append(parts, fmt.Sprintf("%d worktree(s)", worktreeCount))
		}
		if branchCount > 0 {
			parts = append(parts, fmt.Sprintf("%d orphaned branch(es)", branchCount))
		}
		fmt.Printf("would remove %s (dry run)\n", strings.Join(parts, " and "))
		return nil
	}

	if !f.force {
		msg := fmt.Sprintf("Remove %d item(s)?", len(eligible))
		if !promptYN(os.Stdin, os.Stdout, msg) {
			fmt.Println("aborted")
			return nil
		}
	}

	results, err := wn.CleanupWorktrees(cc.Store, cc.Root, f.branch, false, f.cleanIgnored, f.worktreesOnly, os.Stderr)
	if err != nil {
		return err
	}
	for _, r := range results {
		switch r.Status {
		case "removed":
			label := r.Branch
			if r.ItemID != "" {
				label = r.ItemID + " (" + r.Branch + ")"
			}
			suffix := ""
			if r.BranchDeleted {
				suffix = " (branch deleted)"
			}
			fmt.Printf("removed %s: %s%s\n", label, r.Path, suffix)
		case "branch_deleted":
			label := r.Branch
			if r.ItemID != "" {
				label = r.ItemID + " (" + r.Branch + ")"
			}
			fmt.Printf("deleted orphaned branch %s\n", label)
		case "skipped_not_done", "skipped_not_merged", "skipped_no_item", "skipped_detached":
		case "error":
			fmt.Fprintf(os.Stderr, "error %s: %s\n", r.Branch, r.Reason)
		}
	}
	return nil
}
