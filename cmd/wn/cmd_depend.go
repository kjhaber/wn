package main

import (
	"fmt"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type tagPersistFlags struct {
	wid string
}

type tagAddFlags struct {
	interactive bool
}

func newTagCmd() *cobra.Command {
	persist := &tagPersistFlags{}
	tagCmd := &cobra.Command{
		Use:   "tag",
		Short: "Add, remove, or list tags on a work item",
		Long:  "Subcommands: add, rm, list. Use --wid to specify work item; when omitted, uses the current task. Use 'wn tag add -i <tag>' to pick items with fzf and toggle the tag on each.",
	}
	tagCmd.PersistentFlags().StringVar(&persist.wid, "wid", "", "Work item id (default: current task)")

	addF := &tagAddFlags{}
	tagAddCmd := &cobra.Command{
		Use:   "add <tag-name>",
		Short: "Add a tag to a work item",
		Long:  "Add a tag. Use --wid <id> to specify the work item; when omitted, uses the current task. Use -i/--interactive to pick items with fzf and toggle the tag on each selected item.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagAdd(cmd, args, persist, addF)
		},
	}
	tagAddCmd.Flags().BoolVarP(&addF.interactive, "interactive", "i", false, "Pick work items with fzf (or numbered list); toggle tag on selected items")

	tagRmCmd := &cobra.Command{
		Use:   "rm <tag-name>",
		Short: "Remove a tag from a work item",
		Long:  "Remove a tag. Use --wid <id> to specify the work item; when omitted, uses the current task.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagRm(cmd, args, persist)
		},
	}

	tagListCmd := &cobra.Command{
		Use:   "list",
		Short: "List tags on a work item (one per line)",
		Long:  "List tags on the work item. Use --wid <id> to specify the work item; when omitted, uses the current task. Output is one tag per line.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagList(cmd, args, persist)
		},
	}

	tagCmd.AddCommand(tagAddCmd, tagRmCmd, tagListCmd)
	return tagCmd
}

func resolveTagWid(p *tagPersistFlags) (string, error) {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return "", err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return "", err
	}
	return wn.ResolveItemID(meta.CurrentID, p.wid)
}

func runTagAdd(cmd *cobra.Command, args []string, persist *tagPersistFlags, addF *tagAddFlags) error {
	if addF.interactive {
		return runTagInteractive(args)
	}
	tag := args[0]
	if err := wn.ValidateTag(tag); err != nil {
		return err
	}
	id, err := resolveTagWid(persist)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		for _, t := range it.Tags {
			if t == tag {
				return it, nil
			}
		}
		it.Tags = append(it.Tags, tag)
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "tag_added", Msg: tag})
		return it, nil
	})
}

func runTagInteractive(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("interactive tag requires exactly one argument: the tag name")
	}
	tag := args[0]
	if err := wn.ValidateTag(tag); err != nil {
		return err
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	items, err := wn.UndoneItems(cc.Store)
	if err != nil {
		return err
	}
	items = wn.ApplySort(items, interactiveSortSpec(cc.Root))
	ids, err := wn.PickMultiInteractiveWithTags(items)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, id := range ids {
		err := cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
			hasTag := false
			for _, t := range it.Tags {
				if t == tag {
					hasTag = true
					break
				}
			}
			if hasTag {
				var newTags []string
				for _, t := range it.Tags {
					if t != tag {
						newTags = append(newTags, t)
					}
				}
				it.Tags = newTags
				it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "tag_removed", Msg: tag})
			} else {
				it.Tags = append(it.Tags, tag)
				it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "tag_added", Msg: tag})
			}
			it.Updated = now
			return it, nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func runTagRm(cmd *cobra.Command, args []string, persist *tagPersistFlags) error {
	tag := args[0]
	id, err := resolveTagWid(persist)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		var newTags []string
		for _, t := range it.Tags {
			if t != tag {
				newTags = append(newTags, t)
			}
		}
		it.Tags = newTags
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "tag_removed", Msg: tag})
		return it, nil
	})
}

func runTagList(cmd *cobra.Command, args []string, persist *tagPersistFlags) error {
	id, err := resolveTagWid(persist)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	item, err := cc.Store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	out := cmd.Root().OutOrStdout()
	for _, t := range item.Tags {
		_, _ = fmt.Fprintln(out, t)
	}
	return nil
}

type dependAddFlags struct {
	on          string
	wid         string
	interactive bool
}

type dependRmFlags struct {
	on          string
	wid         string
	interactive bool
}

type dependListFlags struct {
	wid string
}

func newDependCmd() *cobra.Command {
	dependCmd := &cobra.Command{
		Use:   "depend",
		Short: "Add, remove, or list dependencies on a work item",
		Long:  "Use 'wn depend add --on <id> [--wid <id>]', 'wn depend rm --on <id> [--wid <id>]', and 'wn depend list [--wid <id>]'. Omit --wid to use the current task.",
	}

	addF := &dependAddFlags{}
	dependAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Mark an item as depending on another",
		Long:  "Add a dependency. Use --on for the dependency id; omit --wid to use the current task. Use -i to pick the depended-on item interactively (fzf or numbered list).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDependAdd(cmd, args, addF)
		},
	}
	dependAddCmd.Flags().StringVar(&addF.on, "on", "", "ID of the item this one will depend on")
	dependAddCmd.Flags().StringVar(&addF.wid, "wid", "", "Work item id (current task when omitted)")
	dependAddCmd.Flags().BoolVarP(&addF.interactive, "interactive", "i", false, "Pick the depended-on item with fzf (undone items only)")

	rmF := &dependRmFlags{}
	dependRmCmd := &cobra.Command{
		Use:   "rm",
		Short: "Remove a dependency",
		Long:  "Remove a dependency. Use --on for the dependency id to remove; omit --wid to use the current task. Use -i to pick which dependency to remove (fzf or numbered list).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDependRm(cmd, args, rmF)
		},
	}
	dependRmCmd.Flags().StringVar(&rmF.on, "on", "", "ID of the dependency to remove")
	dependRmCmd.Flags().StringVar(&rmF.wid, "wid", "", "Work item id (current task when omitted)")
	dependRmCmd.Flags().BoolVarP(&rmF.interactive, "interactive", "i", false, "Pick the dependency to remove with fzf")

	listF := &dependListFlags{}
	dependListCmd := &cobra.Command{
		Use:   "list",
		Short: "List dependencies of a work item (one id per line)",
		Long:  "Output the dependency ids of the work item, one per line. Omit --wid to use the current task.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDependList(cmd, args, listF)
		},
	}
	dependListCmd.Flags().StringVar(&listF.wid, "wid", "", "Work item id (current task when omitted)")

	dependCmd.AddCommand(dependAddCmd, dependRmCmd, dependListCmd)
	return dependCmd
}

func runDependAdd(cmd *cobra.Command, args []string, f *dependAddFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	id, err := wn.ResolveItemID(meta.CurrentID, f.wid)
	if err != nil {
		return fmt.Errorf("no work item (use --wid or set current task)")
	}
	var onID string
	if f.interactive {
		onID, err = runDependInteractive(cc.Store, cc.Root, id)
		if err != nil {
			return err
		}
		if onID == "" {
			return nil
		}
	} else {
		if f.on == "" {
			return fmt.Errorf("required flag \"on\" not set")
		}
		onID = f.on
	}
	items, err := cc.Store.List()
	if err != nil {
		return err
	}
	if wn.WouldCreateCycle(items, id, onID) {
		return fmt.Errorf("circular dependency detected, could not mark entry %s dependent on %s", id, onID)
	}
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		for _, d := range it.DependsOn {
			if d == onID {
				return it, nil
			}
		}
		it.DependsOn = append(it.DependsOn, onID)
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "depend_added", Msg: onID})
		return it, nil
	})
}

func runDependInteractive(store wn.Store, root string, excludeID string) (string, error) {
	undone, err := wn.UndoneItems(store)
	if err != nil {
		return "", err
	}
	var candidates []*wn.Item
	for _, it := range undone {
		if it.ID != excludeID {
			candidates = append(candidates, it)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no other undone items to depend on")
	}
	candidates = wn.ApplySort(candidates, interactiveSortSpec(root))
	return wn.PickInteractive(candidates)
}

func runDependRm(cmd *cobra.Command, args []string, f *dependRmFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	id, err := wn.ResolveItemID(meta.CurrentID, f.wid)
	if err != nil {
		return fmt.Errorf("no work item (use --wid or set current task)")
	}
	var onID string
	if f.interactive {
		onID, err = runRmdependInteractive(cc.Store, cc.Root, id)
		if err != nil {
			return err
		}
		if onID == "" {
			return nil
		}
	} else {
		if f.on == "" {
			return fmt.Errorf("required flag \"on\" not set")
		}
		onID = f.on
	}
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		var newDeps []string
		for _, d := range it.DependsOn {
			if d != onID {
				newDeps = append(newDeps, d)
			}
		}
		it.DependsOn = newDeps
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "depend_removed", Msg: onID})
		return it, nil
	})
}

func runRmdependInteractive(store wn.Store, root string, id string) (string, error) {
	it, err := store.Get(id)
	if err != nil {
		return "", err
	}
	if len(it.DependsOn) == 0 {
		return "", fmt.Errorf("item %s has no dependencies to remove", id)
	}
	var candidates []*wn.Item
	for _, depID := range it.DependsOn {
		dep, err := store.Get(depID)
		if err != nil {
			dep = &wn.Item{ID: depID, Description: depID}
		}
		candidates = append(candidates, dep)
	}
	candidates = wn.ApplySort(candidates, interactiveSortSpec(root))
	return wn.PickInteractive(candidates)
}

func runDependList(cmd *cobra.Command, args []string, f *dependListFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	id, err := wn.ResolveItemID(meta.CurrentID, f.wid)
	if err != nil {
		return fmt.Errorf("no work item (use --wid or set current task)")
	}
	item, err := cc.Store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	out := cmd.OutOrStdout()
	for _, depID := range item.DependsOn {
		_, _ = fmt.Fprintln(out, depID)
	}
	return nil
}

// interactiveSortSpec returns sort options from effective settings (user + project) for fzf/numbered lists. No CLI override.
func interactiveSortSpec(root string) []wn.SortOption {
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return nil
	}
	return wn.SortSpecFromSettings(settings)
}
