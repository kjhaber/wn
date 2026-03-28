package main

import (
	"fmt"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Add, remove, or list tags on a work item",
	Long:  "Subcommands: add, rm, list. Use --wid to specify work item; when omitted, uses the current task. Use 'wn tag add -i <tag>' to pick items with fzf and toggle the tag on each.",
}

var tagWid string
var tagAddInteractive bool

var tagAddCmd = &cobra.Command{
	Use:   "add <tag-name>",
	Short: "Add a tag to a work item",
	Long:  "Add a tag. Use --wid <id> to specify the work item; when omitted, uses the current task. Use -i/--interactive to pick items with fzf and toggle the tag on each selected item.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTagAdd,
}

var tagRmCmd = &cobra.Command{
	Use:   "rm <tag-name>",
	Short: "Remove a tag from a work item",
	Long:  "Remove a tag. Use --wid <id> to specify the work item; when omitted, uses the current task.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTagRm,
}

var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tags on a work item (one per line)",
	Long:  "List tags on the work item. Use --wid <id> to specify the work item; when omitted, uses the current task. Output is one tag per line.",
	Args:  cobra.NoArgs,
	RunE:  runTagList,
}

func init() {
	tagCmd.PersistentFlags().StringVar(&tagWid, "wid", "", "Work item id (default: current task)")
	tagAddCmd.Flags().BoolVarP(&tagAddInteractive, "interactive", "i", false, "Pick work items with fzf (or numbered list); toggle tag on selected items")
	tagCmd.AddCommand(tagAddCmd, tagRmCmd, tagListCmd)
}

func resolveTagWid() (string, error) {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return "", err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return "", err
	}
	return wn.ResolveItemID(meta.CurrentID, tagWid)
}

func runTagAdd(cmd *cobra.Command, args []string) error {
	if tagAddInteractive {
		return runTagInteractive(args)
	}
	tag := args[0]
	if err := wn.ValidateTag(tag); err != nil {
		return err
	}
	id, err := resolveTagWid()
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
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
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	items, err := wn.UndoneItems(store)
	if err != nil {
		return err
	}
	items = wn.ApplySort(items, interactiveSortSpec(root))
	ids, err := wn.PickMultiInteractiveWithTags(items)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, id := range ids {
		err := store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
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

func runTagRm(cmd *cobra.Command, args []string) error {
	tag := args[0]
	id, err := resolveTagWid()
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
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

func runTagList(cmd *cobra.Command, args []string) error {
	id, err := resolveTagWid()
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	out := cmd.Root().OutOrStdout()
	for _, t := range item.Tags {
		_, _ = fmt.Fprintln(out, t)
	}
	return nil
}

// depend command and subcommands add, rm, list. Work item id is --wid (current task when omitted).
var dependCmd = &cobra.Command{
	Use:   "depend",
	Short: "Add, remove, or list dependencies on a work item",
	Long:  "Use 'wn depend add --on <id> [--wid <id>]', 'wn depend rm --on <id> [--wid <id>]', and 'wn depend list [--wid <id>]'. Omit --wid to use the current task.",
}

var dependAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Mark an item as depending on another",
	Long:  "Add a dependency. Use --on for the dependency id; omit --wid to use the current task. Use -i to pick the depended-on item interactively (fzf or numbered list).",
	Args:  cobra.NoArgs,
	RunE:  runDependAdd,
}
var dependAddOn string
var dependAddWid string
var dependAddInteractive bool

func init() {
	dependAddCmd.Flags().StringVar(&dependAddOn, "on", "", "ID of the item this one will depend on")
	dependAddCmd.Flags().StringVar(&dependAddWid, "wid", "", "Work item id (current task when omitted)")
	dependAddCmd.Flags().BoolVarP(&dependAddInteractive, "interactive", "i", false, "Pick the depended-on item with fzf (undone items only)")
	dependCmd.AddCommand(dependAddCmd)
}

func runDependAdd(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	id, err := wn.ResolveItemID(meta.CurrentID, dependAddWid)
	if err != nil {
		return fmt.Errorf("no work item (use --wid or set current task)")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	var onID string
	if dependAddInteractive {
		onID, err = runDependInteractive(store, root, id)
		if err != nil {
			return err
		}
		if onID == "" {
			return nil
		}
	} else {
		if dependAddOn == "" {
			return fmt.Errorf("required flag \"on\" not set")
		}
		onID = dependAddOn
	}
	items, err := store.List()
	if err != nil {
		return err
	}
	if wn.WouldCreateCycle(items, id, onID) {
		return fmt.Errorf("circular dependency detected, could not mark entry %s dependent on %s", id, onID)
	}
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
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

var dependRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a dependency",
	Long:  "Remove a dependency. Use --on for the dependency id to remove; omit --wid to use the current task. Use -i to pick which dependency to remove (fzf or numbered list).",
	Args:  cobra.NoArgs,
	RunE:  runDependRm,
}
var dependRmOn string
var dependRmWid string
var dependRmInteractive bool

func init() {
	dependRmCmd.Flags().StringVar(&dependRmOn, "on", "", "ID of the dependency to remove")
	dependRmCmd.Flags().StringVar(&dependRmWid, "wid", "", "Work item id (current task when omitted)")
	dependRmCmd.Flags().BoolVarP(&dependRmInteractive, "interactive", "i", false, "Pick the dependency to remove with fzf")
	dependCmd.AddCommand(dependRmCmd)
}

func runDependRm(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	id, err := wn.ResolveItemID(meta.CurrentID, dependRmWid)
	if err != nil {
		return fmt.Errorf("no work item (use --wid or set current task)")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	var onID string
	if dependRmInteractive {
		onID, err = runRmdependInteractive(store, root, id)
		if err != nil {
			return err
		}
		if onID == "" {
			return nil
		}
	} else {
		if dependRmOn == "" {
			return fmt.Errorf("required flag \"on\" not set")
		}
		onID = dependRmOn
	}
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
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

var dependListCmd = &cobra.Command{
	Use:   "list",
	Short: "List dependencies of a work item (one id per line)",
	Long:  "Output the dependency ids of the work item, one per line. Omit --wid to use the current task.",
	Args:  cobra.NoArgs,
	RunE:  runDependList,
}
var dependListWid string

func init() {
	dependListCmd.Flags().StringVar(&dependListWid, "wid", "", "Work item id (current task when omitted)")
	dependCmd.AddCommand(dependListCmd)
}

func runDependList(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	id, err := wn.ResolveItemID(meta.CurrentID, dependListWid)
	if err != nil {
		return fmt.Errorf("no work item (use --wid or set current task)")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
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
