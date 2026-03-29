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

type showFlags struct {
	jsonOutput bool
	plain      bool
	all        bool
	fields     string
}

func newShowCmd() *cobra.Command {
	flags := &showFlags{}
	cmd := &cobra.Command{
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&flags.plain, "plain", false, "Output description text only (for agents/scripts)")
	cmd.Flags().BoolVar(&flags.all, "all", false, "Show all fields including log")
	cmd.Flags().StringVar(&flags.fields, "fields", "", "Comma-separated fields: title,body,status,deps,notes,log")
	return cmd
}

func runShow(cmd *cobra.Command, args []string, flags *showFlags) error {
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
		return fmt.Errorf("no id provided and no current task; use 'wn pick' or 'wn next'")
	}
	item, err := cc.Store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	if flags.jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(item)
	}
	if flags.plain {
		fmt.Println(wn.PromptContent(item.Description))
		return nil
	}
	fields := resolveShowFields(flags.all, flags.fields, cc.Settings)
	return renderItemHuman(item, fields, cc.Store)
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
		const titleWidth = 56
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

func newLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log [id]",
		Short: "Show history of a work item",
		Long:  "If id is omitted, shows log for the current task.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runLog,
	}
}

func runLog(cmd *cobra.Command, args []string) error {
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
	item, err := cc.Store.Get(id)
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

type nextFlags struct {
	tag     string
	claim   string
	claimBy string
}

func newNextCmd() *cobra.Command {
	flags := &nextFlags{}
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Pick the next task (first undone in dependency order) and set as current",
		Long:  "When --tag is provided, pick the next undone item that has that tag (dependency order). Use --claim <duration> to also claim the task (e.g. wn next --claim 30m).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNext(cmd, args, flags)
		},
	}
	cmd.Flags().StringVar(&flags.tag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	cmd.Flags().StringVar(&flags.claim, "claim", "", "Also claim the task for this duration (e.g. 30m, 1h)")
	cmd.Flags().StringVar(&flags.claimBy, "claim-by", "", "Optional worker ID when using --claim")
	return cmd
}

func runNext(cmd *cobra.Command, args []string, flags *nextFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	next, err := wn.NextUndoneItem(cc.Store, flags.tag)
	if err != nil {
		return err
	}
	if next == nil {
		fmt.Println("No next task.")
		return nil
	}
	if err := wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = next.ID
		return m, nil
	}); err != nil {
		return err
	}
	if flags.claim != "" {
		d, err := time.ParseDuration(flags.claim)
		if err != nil {
			return fmt.Errorf("invalid --claim duration %q: %w", flags.claim, err)
		}
		if d <= 0 {
			return fmt.Errorf("--claim duration must be positive, got %v", d)
		}
		now := time.Now().UTC()
		until := now.Add(d)
		if err := cc.Store.UpdateItem(next.ID, func(it *wn.Item) (*wn.Item, error) {
			it.InProgressUntil = until
			it.InProgressBy = flags.claimBy
			it.Updated = now
			it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "in_progress", Msg: flags.claim})
			return it, nil
		}); err != nil {
			return err
		}
		fmt.Printf("  %s: %s (claimed for %s)\n", next.ID, next.Description, flags.claim)
		return nil
	}
	fmt.Printf("  %s: %s\n", next.ID, next.Description)
	return nil
}

type pickFlags struct {
	undone      bool
	done        bool
	all         bool
	reviewReady bool
	tag         string
}

func newPickCmd() *cobra.Command {
	flags := &pickFlags{}
	cmd := &cobra.Command{
		Use:   "pick [id|.|−]",
		Short: "Interactively pick a current task (uses fzf if available)",
		Long:  "With no id, shows an interactive list to choose from. Pass an id to set current task directly. Pass '.' to select the item for the current directory's git branch (useful when switching between worktrees). Pass '-' to switch to the previously selected item (like git checkout -). Use --undone (default), --done, --all, or --rr/--review-ready to filter by state.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runPick(c, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.undone, "undone", false, "Pick from undone items only (default)")
	cmd.Flags().BoolVar(&flags.done, "done", false, "Pick from done items only")
	cmd.Flags().BoolVar(&flags.all, "all", false, "Pick from all items")
	cmd.Flags().BoolVar(&flags.reviewReady, "rr", false, "Pick from review-ready items only")
	cmd.Flags().BoolVar(&flags.reviewReady, "review-ready", false, "Pick from review-ready items only")
	cmd.Flags().StringVar(&flags.tag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	return cmd
}

func runPick(cmd *cobra.Command, args []string, flags *pickFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}

	if len(args) == 1 {
		id := args[0]
		if id == "-" {
			meta, err := wn.ReadMeta(cc.Root)
			if err != nil {
				return err
			}
			if meta.PreviousID == "" {
				return fmt.Errorf("no previous task")
			}
			item, err := cc.Store.Get(meta.PreviousID)
			if err != nil {
				return fmt.Errorf("previous task %s not found", meta.PreviousID)
			}
			if err := wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
				m.CurrentID = meta.PreviousID
				return m, nil
			}); err != nil {
				return err
			}
			fmt.Printf("%s %s\n", item.ID, wn.FirstLine(item.Description))
			return nil
		}
		if id == "." {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			branch, err := wn.CurrentBranchInDir(cwd)
			if err != nil {
				return fmt.Errorf("could not determine git branch: %w", err)
			}
			item, err := wn.FindItemByBranch(cc.Store, branch)
			if err != nil {
				return err
			}
			if item == nil {
				return fmt.Errorf("no work item found for branch %q", branch)
			}
			if err := wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
				m.CurrentID = item.ID
				return m, nil
			}); err != nil {
				return err
			}
			fmt.Printf("%s %s\n", item.ID, wn.FirstLine(item.Description))
			return nil
		}
		if _, err := cc.Store.Get(id); err != nil {
			return fmt.Errorf("item %s not found", id)
		}
		return wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
			m.CurrentID = id
			return m, nil
		})
	}

	stateFlags := 0
	if flags.undone {
		stateFlags++
	}
	if flags.done {
		stateFlags++
	}
	if flags.all {
		stateFlags++
	}
	if flags.reviewReady {
		stateFlags++
	}
	if stateFlags > 1 {
		return fmt.Errorf("only one of --undone, --done, --all, --review-ready may be set")
	}

	var items []*wn.Item
	if flags.all {
		items, err = cc.Store.List()
		if err != nil {
			return err
		}
	} else if flags.done {
		all, err := cc.Store.List()
		if err != nil {
			return err
		}
		for _, it := range all {
			if it.Done {
				items = append(items, it)
			}
		}
	} else if flags.reviewReady {
		items, err = wn.ReviewReadyItems(cc.Store)
		if err != nil {
			return err
		}
	} else {
		items, err = wn.UndoneItems(cc.Store)
		if err != nil {
			return err
		}
	}

	items = wn.FilterByTag(items, flags.tag)
	if len(items) == 0 {
		msg := "No undone tasks."
		if flags.done {
			msg = "No done tasks."
		} else if flags.all {
			msg = "No tasks."
		} else if flags.reviewReady {
			msg = "No review-ready tasks."
		}
		fmt.Println(msg)
		return nil
	}
	items = wn.ApplySort(items, interactiveSortSpec(cc.Root))
	id, err := wn.PickInteractive(items)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	return wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = id
		return m, nil
	})
}

type listFlags struct {
	undone      bool
	done        bool
	all         bool
	reviewReady bool
	tag         string
	sort        string
	limit       int
	offset      int
	jsonOutput  bool
	group       string
}

func newListCmd() *cobra.Command {
	flags := &listFlags{}
	cmd := &cobra.Command{
		Use:     "list [@view]",
		Aliases: []string{"ls"},
		Short:   "List work items (default: undone, in dependency order)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runList(c, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.undone, "undone", false, "List undone items (default when no filter; includes both available and review-ready; excludes in-progress)")
	cmd.Flags().BoolVar(&flags.done, "done", false, "List done items")
	cmd.Flags().BoolVar(&flags.all, "all", false, "List all items")
	cmd.Flags().BoolVar(&flags.reviewReady, "review-ready", false, "List review-ready items only")
	cmd.Flags().BoolVar(&flags.reviewReady, "rr", false, "List review-ready items only")
	cmd.Flags().StringVar(&flags.tag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	cmd.Flags().StringVar(&flags.sort, "sort", "", "Sort order (e.g. updated:desc,priority,tags). Overrides settings. Keys: created, updated, priority, alpha, tags")
	cmd.Flags().IntVar(&flags.limit, "limit", 0, "Return at most N items (0 = no limit)")
	cmd.Flags().IntVar(&flags.offset, "offset", 0, "Skip first N items")
	cmd.Flags().BoolVar(&flags.jsonOutput, "json", false, "Output as JSON (same format as export: version, exported_at, items with all attributes)")
	cmd.Flags().StringVar(&flags.group, "group", "", "Group items by key: tags, status")
	return cmd
}

func runList(cmd *cobra.Command, args []string, flags *listFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "@") {
		viewName := args[0][1:]
		viewArgs, err := wn.ResolveView(cc.Settings, viewName)
		if err != nil {
			return err
		}
		if err := cmd.Flags().Parse(viewArgs); err != nil {
			return fmt.Errorf("view %q: invalid flags: %w", viewName, err)
		}
	}
	stateFlags := 0
	if flags.all {
		stateFlags++
	}
	if flags.done {
		stateFlags++
	}
	if flags.undone {
		stateFlags++
	}
	if flags.reviewReady {
		stateFlags++
	}
	if stateFlags > 1 {
		return fmt.Errorf("only one of --undone, --done, --all, --review-ready may be set")
	}
	useUndone := flags.undone || stateFlags == 0
	allItems, err := cc.Store.List()
	if err != nil {
		return err
	}
	blockedSet := wn.BlockedSet(allItems)
	var items []*wn.Item
	if flags.all {
		items = allItems
	} else if flags.done {
		for _, it := range allItems {
			if it.Done {
				items = append(items, it)
			}
		}
	} else if flags.reviewReady {
		items, err = wn.ReviewReadyItems(cc.Store)
		if err != nil {
			return err
		}
	} else if useUndone {
		items, err = wn.ListableUndoneItems(cc.Store)
		if err != nil {
			return err
		}
	} else {
		items = nil
	}
	items = wn.FilterByTag(items, flags.tag)
	var ordered []*wn.Item
	sortSpec := listSortSpec(cc.Root, flags.sort)
	if len(sortSpec) > 0 {
		ordered = wn.ApplySort(items, sortSpec)
	} else {
		var acyclic bool
		ordered, acyclic = wn.TopoOrder(items)
		if !acyclic && len(ordered) > 0 {
			ordered = items
		}
	}
	if flags.offset > 0 || flags.limit > 0 {
		if flags.offset > len(ordered) {
			ordered = nil
		} else {
			ordered = ordered[flags.offset:]
			if flags.limit > 0 && len(ordered) > flags.limit {
				ordered = ordered[:flags.limit]
			}
		}
	}
	if flags.group != "" {
		switch flags.group {
		case "tags", "status":
		default:
			return fmt.Errorf("invalid --group key %q (use: tags, status)", flags.group)
		}
		if flags.jsonOutput {
			return fmt.Errorf("--group and --json are incompatible")
		}
		now := time.Now().UTC()
		ordered = applyGroupSort(ordered, flags.group, now, blockedSet)
		printGroupedList(ordered, flags.group, now, blockedSet)
		return nil
	}
	if flags.jsonOutput {
		return wn.ExportItems(ordered, "")
	}
	now := time.Now().UTC()
	const listStatusWidth = 7
	const listDescWidth = 51
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

func applyGroupSort(items []*wn.Item, by string, now time.Time, blockedSet map[string]bool) []*wn.Item {
	switch by {
	case "tags":
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

func itemGroupHeader(key, by string) string {
	switch by {
	case "tags":
		if key == "" {
			return "--- (no tags) ---"
		}
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

func listSortSpec(root string, sortFlag string) []wn.SortOption {
	if sortFlag != "" {
		spec, err := wn.ParseSortSpec(sortFlag)
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

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "[" + strings.Join(tags, ", ") + "]"
}

func itemListStatus(it *wn.Item, now time.Time, blocked bool) string {
	return wn.ItemListStatus(it, now, blocked)
}
