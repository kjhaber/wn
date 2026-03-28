package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show a work item",
	Long: `Show a work item. If id is omitted, uses current task.

Output modes:
  (default)  Human-readable; fields controlled by --fields or --all
  --plain    Description text only, suitable for pasting into an agent
  --json     Full item as machine-readable JSON

Field selection (human-readable mode only):
  --fields title,body,status,deps,notes,log
  --all      Show all fields (equivalent to --fields title,body,status,deps,notes,log)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runShow,
}

var showJson, showPlain, showAll bool
var showFields string

func init() {
	showCmd.Flags().BoolVar(&showJson, "json", false, "Output as JSON")
	showCmd.Flags().BoolVar(&showPlain, "plain", false, "Output description text only (for agents/scripts)")
	showCmd.Flags().BoolVar(&showAll, "all", false, "Show all fields including log")
	showCmd.Flags().StringVar(&showFields, "fields", "", "Comma-separated fields: title,body,status,deps,notes,log")
}

func runShow(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("no id provided and no current task; use 'wn pick' or 'wn next'")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	if showJson {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(item)
	}
	if showPlain {
		fmt.Println(wn.PromptContent(item.Description))
		return nil
	}
	settings, _ := wn.ReadSettingsInRoot(root)
	fields := resolveShowFields(showAll, showFields, settings)
	return renderItemHuman(item, fields, store)
}

// resolveShowFields returns the active field set for human-readable output.
// Priority: --all > --fields flag > settings default > built-in default.
func resolveShowFields(all bool, fieldsFlag string, settings wn.Settings) map[string]bool {
	const allFields = "title,body,status,deps,notes,log"
	if all {
		return parseFieldSet(allFields)
	}
	if fieldsFlag != "" {
		return parseFieldSet(fieldsFlag)
	}
	if settings.Show.DefaultFields != "" {
		return parseFieldSet(settings.Show.DefaultFields)
	}
	return parseFieldSet(defaultShowFields)
}

func parseFieldSet(s string) map[string]bool {
	m := make(map[string]bool)
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			m[f] = true
		}
	}
	return m
}

// renderItemHuman prints a work item in human-readable format, showing only the requested fields.
func renderItemHuman(item *wn.Item, fields map[string]bool, store wn.Store) error {
	const timeFmt = "2006-01-02 15:04:05"

	// Compute blocked state once: non-done items with unresolved deps.
	blocked := false
	if !item.Done && !item.ReviewReady && len(item.DependsOn) > 0 {
		if allItems, err := store.List(); err == nil {
			blocked = wn.BlockedSet(allItems)[item.ID]
		}
	}

	if fields["title"] {
		var state string
		if item.Done {
			switch item.DoneStatus {
			case wn.DoneStatusClosed:
				state = " (closed)"
			case wn.DoneStatusSuspend:
				state = " (suspend)"
			default:
				state = " (done)"
			}
		} else if item.ReviewReady {
			state = " (review)"
		} else if blocked {
			state = " (blocked)"
		} else if wn.IsInProgress(item, time.Now().UTC()) {
			state = " (claimed)"
		}
		firstLine := wn.FirstLine(item.Description)
		tagsStr := formatTags(item.Tags)
		const titleWidth = 56 // pad so tags/state align on the right
		content := fmt.Sprintf("[%s] %s", item.ID, firstLine)
		if tagsStr != "" {
			if len(content) > titleWidth {
				content = content[:titleWidth-3] + "..."
			} else {
				content = content + strings.Repeat(" ", titleWidth-len(content))
			}
			fmt.Printf("%s  %s%s\n", content, tagsStr, state)
		} else {
			fmt.Printf("%s%s\n", content, state)
		}
	}

	if fields["body"] {
		if _, rest, ok := strings.Cut(item.Description, "\n"); ok && strings.TrimSpace(rest) != "" {
			fmt.Print(rest)
			if !strings.HasSuffix(rest, "\n") {
				fmt.Println()
			}
		}
	}

	if fields["status"] {
		status := wn.ItemListStatus(item, time.Now().UTC(), blocked)
		if item.Done && item.DoneMessage != "" {
			status += " (" + item.DoneMessage + ")"
		} else if !item.InProgressUntil.IsZero() && item.InProgressUntil.After(time.Now().UTC()) {
			status = "in progress until " + item.InProgressUntil.Format(timeFmt)
			if item.InProgressBy != "" {
				status += " (by " + item.InProgressBy + ")"
			}
		}
		fmt.Printf("status: %s\n", status)
	}

	if fields["deps"] {
		if len(item.DependsOn) > 0 {
			fmt.Printf("depends on: %s\n", strings.Join(item.DependsOn, ", "))
		}
		dependents, err := wn.Dependents(store, item.ID)
		if err == nil && len(dependents) > 0 {
			fmt.Printf("dependent tasks: %s\n", strings.Join(dependents, ", "))
		}
	}

	if fields["notes"] && len(item.Notes) > 0 {
		var metaNotes, userNotes []wn.Note
		for _, n := range item.Notes {
			if strings.HasPrefix(n.Name, "wn:") {
				metaNotes = append(metaNotes, n)
			} else {
				userNotes = append(userNotes, n)
			}
		}
		if len(metaNotes) > 0 {
			fmt.Println("metadata:")
			for _, n := range metaNotes {
				fmt.Printf("  %s\t%s\t%s\n", n.Name, n.Created.Format(timeFmt), n.Body)
			}
		}
		if len(userNotes) > 0 {
			fmt.Println("notes:")
			for _, n := range userNotes {
				fmt.Printf("  %s\t%s\t%s\n", n.Name, n.Created.Format(timeFmt), n.Body)
			}
		}
	}

	if fields["log"] && len(item.Log) > 0 {
		fmt.Println("log:")
		for _, e := range item.Log {
			fmt.Printf("  %s %s", e.At.Format(timeFmt), e.Kind)
			if e.Msg != "" {
				fmt.Printf(" %s", e.Msg)
			}
			fmt.Println()
		}
	}

	return nil
}

var logCmd = &cobra.Command{
	Use:   "log [id]",
	Short: "Show history of a work item",
	Long:  "If id is omitted, shows log for the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLog,
}

func runLog(cmd *cobra.Command, args []string) error {
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
	for _, e := range item.Log {
		fmt.Printf("%s %s", e.At.Format("2006-01-02 15:04:05"), e.Kind)
		if e.Msg != "" {
			fmt.Printf(" %s", e.Msg)
		}
		fmt.Println()
	}
	return nil
}

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Pick the next task (first undone in dependency order) and set as current",
	Long:  "When --tag is provided, pick the next undone item that has that tag (dependency order). Use --claim <duration> to also claim the task (e.g. wn next --claim 30m).",
	RunE:  runNext,
}
var nextClaimFor string
var nextClaimBy string
var nextTag string

func init() {
	nextCmd.Flags().StringVar(&nextTag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	nextCmd.Flags().StringVar(&nextClaimFor, "claim", "", "Also claim the task for this duration (e.g. 30m, 1h)")
	nextCmd.Flags().StringVar(&nextClaimBy, "claim-by", "", "Optional worker ID when using --claim")
}

func runNext(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	next, err := wn.NextUndoneItem(store, nextTag)
	if err != nil {
		return err
	}
	if next == nil {
		fmt.Println("No next task.")
		return nil
	}
	if err := wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = next.ID
		return m, nil
	}); err != nil {
		return err
	}
	if nextClaimFor != "" {
		d, err := time.ParseDuration(nextClaimFor)
		if err != nil {
			return fmt.Errorf("invalid --claim duration %q: %w", nextClaimFor, err)
		}
		if d <= 0 {
			return fmt.Errorf("--claim duration must be positive, got %v", d)
		}
		now := time.Now().UTC()
		until := now.Add(d)
		if err := store.UpdateItem(next.ID, func(it *wn.Item) (*wn.Item, error) {
			it.InProgressUntil = until
			it.InProgressBy = nextClaimBy
			it.Updated = now
			it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "in_progress", Msg: nextClaimFor})
			return it, nil
		}); err != nil {
			return err
		}
		fmt.Printf("  %s: %s (claimed for %s)\n", next.ID, next.Description, nextClaimFor)
		return nil
	}
	fmt.Printf("  %s: %s\n", next.ID, next.Description)
	return nil
}

var pickCmd = &cobra.Command{
	Use:   "pick [id|.|−]",
	Short: "Interactively pick a current task (uses fzf if available)",
	Long:  "With no id, shows an interactive list to choose from. Pass an id to set current task directly. Pass '.' to select the item for the current directory's git branch (useful when switching between worktrees). Pass '-' to switch to the previously selected item (like git checkout -). Use --undone (default), --done, --all, or --rr/--review-ready to filter by state.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPick,
}

var pickUndone bool
var pickDone bool
var pickAll bool
var pickReviewReady bool
var pickTag string

func initPick() {
	pickCmd.Flags().BoolVar(&pickUndone, "undone", false, "Pick from undone items only (default)")
	pickCmd.Flags().BoolVar(&pickDone, "done", false, "Pick from done items only")
	pickCmd.Flags().BoolVar(&pickAll, "all", false, "Pick from all items")
	pickCmd.Flags().BoolVar(&pickReviewReady, "rr", false, "Pick from review-ready items only")
	pickCmd.Flags().BoolVar(&pickReviewReady, "review-ready", false, "Pick from review-ready items only")
	pickCmd.Flags().StringVar(&pickTag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
}

func runPick(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}

	// If id passed, set current to that item (must exist)
	if len(args) == 1 {
		id := args[0]
		// "-" is a special argument: switch to the previously selected item (like git checkout -)
		if id == "-" {
			meta, err := wn.ReadMeta(root)
			if err != nil {
				return err
			}
			if meta.PreviousID == "" {
				return fmt.Errorf("no previous task")
			}
			item, err := store.Get(meta.PreviousID)
			if err != nil {
				return fmt.Errorf("previous task %s not found", meta.PreviousID)
			}
			if err := wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
				m.CurrentID = meta.PreviousID
				return m, nil
			}); err != nil {
				return err
			}
			fmt.Printf("%s %s\n", item.ID, wn.FirstLine(item.Description))
			return nil
		}
		// "." is a special argument: resolve item from current directory's git branch
		if id == "." {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			branch, err := wn.CurrentBranchInDir(cwd)
			if err != nil {
				return fmt.Errorf("could not determine git branch: %w", err)
			}
			item, err := wn.FindItemByBranch(store, branch)
			if err != nil {
				return err
			}
			if item == nil {
				return fmt.Errorf("no work item found for branch %q", branch)
			}
			if err := wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
				m.CurrentID = item.ID
				return m, nil
			}); err != nil {
				return err
			}
			fmt.Printf("%s %s\n", item.ID, wn.FirstLine(item.Description))
			return nil
		}
		if _, err := store.Get(id); err != nil {
			return fmt.Errorf("item %s not found", id)
		}
		return wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
			m.CurrentID = id
			return m, nil
		})
	}

	stateFlags := 0
	if pickUndone {
		stateFlags++
	}
	if pickDone {
		stateFlags++
	}
	if pickAll {
		stateFlags++
	}
	if pickReviewReady {
		stateFlags++
	}
	if stateFlags > 1 {
		return fmt.Errorf("only one of --undone, --done, --all, --review-ready may be set")
	}

	var items []*wn.Item
	if pickAll {
		items, err = store.List()
		if err != nil {
			return err
		}
	} else if pickDone {
		all, err := store.List()
		if err != nil {
			return err
		}
		for _, it := range all {
			if it.Done {
				items = append(items, it)
			}
		}
	} else if pickReviewReady {
		items, err = wn.ReviewReadyItems(store)
		if err != nil {
			return err
		}
	} else {
		// default: undone (available for next/claim)
		items, err = wn.UndoneItems(store)
		if err != nil {
			return err
		}
	}

	items = wn.FilterByTag(items, pickTag)
	if len(items) == 0 {
		msg := "No undone tasks."
		if pickDone {
			msg = "No done tasks."
		} else if pickAll {
			msg = "No tasks."
		} else if pickReviewReady {
			msg = "No review-ready tasks."
		}
		fmt.Println(msg)
		return nil
	}
	items = wn.ApplySort(items, interactiveSortSpec(root))
	id, err := wn.PickInteractive(items)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	return wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = id
		return m, nil
	})
}

var listCmd = &cobra.Command{
	Use:     "list [@view]",
	Aliases: []string{"ls"},
	Short:   "List work items (default: undone, in dependency order)",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runList,
}
var listUndone bool
var listDone bool
var listAll bool
var listReviewReady bool
var listTag string
var listSort string
var listLimit int
var listOffset int

var listJson bool
var listGroup string

func init() {
	listCmd.Flags().BoolVar(&listUndone, "undone", false, "List undone items (default when no filter; includes both available and review-ready; excludes in-progress)")
	listCmd.Flags().BoolVar(&listDone, "done", false, "List done items")
	listCmd.Flags().BoolVar(&listAll, "all", false, "List all items")
	listCmd.Flags().BoolVar(&listReviewReady, "review-ready", false, "List review-ready items only")
	listCmd.Flags().BoolVar(&listReviewReady, "rr", false, "List review-ready items only")
	listCmd.Flags().StringVar(&listTag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	listCmd.Flags().StringVar(&listSort, "sort", "", "Sort order (e.g. updated:desc,priority,tags). Overrides settings. Keys: created, updated, priority, alpha, tags")
	listCmd.Flags().IntVar(&listLimit, "limit", 0, "Return at most N items (0 = no limit)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Skip first N items")
	listCmd.Flags().BoolVar(&listJson, "json", false, "Output as JSON (same format as export: version, exported_at, items with all attributes)")
	listCmd.Flags().StringVar(&listGroup, "group", "", "Group items by key: tags, status")
	initPick()
}

func runList(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	// If the first positional arg starts with '@', expand the named view from settings.
	if len(args) > 0 && strings.HasPrefix(args[0], "@") {
		viewName := args[0][1:]
		settings, _ := wn.ReadSettingsInRoot(root)
		viewArgs, err := wn.ResolveView(settings, viewName)
		if err != nil {
			return err
		}
		// Re-parse the view flags into this command's flag set so the global vars are set.
		if err := cmd.Flags().Parse(viewArgs); err != nil {
			return fmt.Errorf("view %q: invalid flags: %w", viewName, err)
		}
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	stateFlags := 0
	if listAll {
		stateFlags++
	}
	if listDone {
		stateFlags++
	}
	if listUndone {
		stateFlags++
	}
	if listReviewReady {
		stateFlags++
	}
	if stateFlags > 1 {
		return fmt.Errorf("only one of --undone, --done, --all, --review-ready may be set")
	}
	// Default when no filter: undone (available for next/claim)
	useUndone := listUndone || stateFlags == 0
	// Load all items once for blocked state computation.
	allItems, err := store.List()
	if err != nil {
		return err
	}
	blockedSet := wn.BlockedSet(allItems)
	var items []*wn.Item
	if listAll {
		items = allItems
	} else if listDone {
		for _, it := range allItems {
			if it.Done {
				items = append(items, it)
			}
		}
	} else if listReviewReady {
		items, err = wn.ReviewReadyItems(store)
		if err != nil {
			return err
		}
	} else if useUndone {
		// --undone or default: all undone (including review-ready); exclude in-progress only
		items, err = wn.ListableUndoneItems(store)
		if err != nil {
			return err
		}
	} else {
		items = nil
	}
	items = wn.FilterByTag(items, listTag)
	var ordered []*wn.Item
	sortSpec := listSortSpec(root)
	if len(sortSpec) > 0 {
		ordered = wn.ApplySort(items, sortSpec)
	} else {
		var acyclic bool
		ordered, acyclic = wn.TopoOrder(items)
		if !acyclic && len(ordered) > 0 {
			ordered = items
		}
	}
	// Apply offset and limit (bounded window for pagination).
	if listOffset > 0 || listLimit > 0 {
		if listOffset > len(ordered) {
			ordered = nil
		} else {
			ordered = ordered[listOffset:]
			if listLimit > 0 && len(ordered) > listLimit {
				ordered = ordered[:listLimit]
			}
		}
	}
	if listGroup != "" {
		switch listGroup {
		case "tags", "status":
		default:
			return fmt.Errorf("invalid --group key %q (use: tags, status)", listGroup)
		}
		if listJson {
			return fmt.Errorf("--group and --json are incompatible")
		}
		// Group-first sort: when grouping by tags, sort by tags as primary key.
		// When grouping by status, sort by status (computed) as primary.
		now := time.Now().UTC()
		ordered = applyGroupSort(ordered, listGroup, now, blockedSet)
		printGroupedList(ordered, listGroup, now, blockedSet)
		return nil
	}
	if listJson {
		// Same format as wn export: version, exported_at, items (full attributes).
		return wn.ExportItems(ordered, "")
	}
	now := time.Now().UTC()
	const listStatusWidth = 7
	const listDescWidth = 51 // so tags align on the right
	for _, it := range ordered {
		status := itemListStatus(it, now, blockedSet[it.ID])
		desc := wn.FirstLine(it.Description)
		if len(desc) > listDescWidth {
			desc = desc[:listDescWidth-3] + "..."
		}
		tagsStr := formatTags(it.Tags)
		fmt.Printf("  %-6s  %-*s  %-*s  %s\n", it.ID, listStatusWidth, status, listDescWidth, desc, tagsStr)
	}
	return nil
}

// applyGroupSort sorts items so that items with the same group key are adjacent.
// For "tags", uses the canonical tag string. For "status", uses the computed status string.
func applyGroupSort(items []*wn.Item, by string, now time.Time, blockedSet map[string]bool) []*wn.Item {
	switch by {
	case "tags":
		// Prepend tags as primary sort key, preserving existing sort for items within a group.
		spec, _ := wn.ParseSortSpec("tags")
		return wn.ApplySort(items, spec)
	case "status":
		result := make([]*wn.Item, len(items))
		copy(result, items)
		sort.Slice(result, func(i, j int) bool {
			si := itemListStatus(result[i], now, blockedSet[result[i].ID])
			sj := itemListStatus(result[j], now, blockedSet[result[j].ID])
			if si != sj {
				return si < sj
			}
			return result[i].ID < result[j].ID
		})
		return result
	}
	return items
}

// itemGroupKey returns the display group key for an item under the given grouping.
func itemGroupKey(it *wn.Item, by string, now time.Time, blockedSet map[string]bool) string {
	switch by {
	case "tags":
		if len(it.Tags) == 0 {
			return ""
		}
		return wn.TagsKey(it.Tags)
	case "status":
		return itemListStatus(it, now, blockedSet[it.ID])
	}
	return ""
}

// itemGroupHeader returns the formatted section header for a group key.
func itemGroupHeader(key, by string) string {
	switch by {
	case "tags":
		if key == "" {
			return "--- (no tags) ---"
		}
		// Convert comma-separated canonical tag string to "#tag1 #tag2" display form.
		tags := strings.Split(key, ",")
		var parts []string
		for _, t := range tags {
			parts = append(parts, "#"+t)
		}
		return "--- " + strings.Join(parts, " ") + " ---"
	case "status":
		return "--- " + key + " ---"
	}
	return "--- " + key + " ---"
}

// printGroupedList prints items with section headers between groups.
func printGroupedList(items []*wn.Item, by string, now time.Time, blockedSet map[string]bool) {
	const listStatusWidth = 7
	const listDescWidth = 51
	var currentGroup *string
	for _, it := range items {
		key := itemGroupKey(it, by, now, blockedSet)
		if currentGroup == nil || *currentGroup != key {
			currentGroup = &key
			fmt.Println(itemGroupHeader(key, by))
		}
		status := itemListStatus(it, now, blockedSet[it.ID])
		desc := wn.FirstLine(it.Description)
		if len(desc) > listDescWidth {
			desc = desc[:listDescWidth-3] + "..."
		}
		tagsStr := formatTags(it.Tags)
		fmt.Printf("  %-6s  %-*s  %-*s  %s\n", it.ID, listStatusWidth, status, listDescWidth, desc, tagsStr)
	}
}

// listSortSpec returns sort options from --sort flag or effective settings (user + project). Invalid spec returns nil.
func listSortSpec(root string) []wn.SortOption {
	if listSort != "" {
		spec, err := wn.ParseSortSpec(listSort)
		if err != nil {
			return nil
		}
		return spec
	}
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return nil
	}
	return wn.SortSpecFromSettings(settings)
}

// formatTags returns tags joined with ", " and wrapped in square brackets, or "" if none.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "[" + strings.Join(tags, ", ") + "]"
}

// itemListStatus returns the display status for list output.
func itemListStatus(it *wn.Item, now time.Time, blocked bool) string {
	return wn.ItemListStatus(it, now, blocked)
}
