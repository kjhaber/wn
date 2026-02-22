package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keith/wn/internal/wn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "wn",
	Short: "What's Next — local task/work item tracker",
	Long:  `wn is a CLI for tracking work items. Use wn init to create a tracker in the current directory.`,
	RunE:  runCurrent,
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("wn version {{.Version}}\n")
	rootCmd.AddCommand(initCmd, addCmd, rmCmd, editCmd, tagCmd, untagCmd, dependCmd, rmdependCmd, orderCmd, doneCmd, undoneCmd, claimCmd, releaseCmd, logCmd, descCmd, showCmd, nextCmd, pickCmd, mcpCmd, settingsCmd, exportCmd, importCmd, listCmd, noteCmd)
	rootCmd.CompletionOptions.DisableDefaultCmd = false
}

func runCurrent(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	if meta.CurrentID == "" {
		fmt.Println("No current task. Use 'wn pick' to choose one or 'wn next' to advance.")
		return nil
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(meta.CurrentID)
	if err != nil {
		fmt.Printf("Current task ID %s not found. Use 'wn pick' to choose one.\n", meta.CurrentID)
		return nil
	}
	var state string
	if item.Done {
		state = " (done)"
	} else if wn.IsInProgress(item, time.Now().UTC()) {
		state = " (claimed)"
	}
	firstLine := wn.FirstLine(item.Description)
	tagsStr := formatTags(item.Tags)
	const currentTaskContentWidth = 56 // pad so tags/state align on the right
	content := fmt.Sprintf("current task: [%s] %s", item.ID, firstLine)
	if tagsStr != "" {
		if len(content) > currentTaskContentWidth {
			content = content[:currentTaskContentWidth-3] + "..."
		} else {
			content = content + strings.Repeat(" ", currentTaskContentWidth-len(content))
		}
		fmt.Printf("%s  %s%s\n", content, tagsStr, state)
	} else {
		fmt.Printf("%s%s\n", content, state)
	}
	// Remaining description lines, if any (preserve blank lines)
	if rest := getRestOfDescription(item.Description); rest != "" {
		fmt.Print(rest)
		if !strings.HasSuffix(rest, "\n") {
			fmt.Println()
		}
	}
	return nil
}

// getRestOfDescription returns all but the first line of s (unchanged, so blank lines are preserved).
func getRestOfDescription(s string) string {
	_, rest, _ := strings.Cut(s, "\n")
	return rest
}

var descCmd = &cobra.Command{
	Use:   "desc [id]",
	Short: "Print a work item description (prompt-ready: title only or body only)",
	Long:  "Output is suitable for pasting into an agent prompt. If id is omitted, uses current task. Single-line descriptions are printed as-is; multi-line descriptions print only the lines after the title. Use --json for machine-readable output.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDesc,
}
var descJson bool

func init() {
	descCmd.Flags().BoolVar(&descJson, "json", false, "Output as JSON object with description field")
}

func runDesc(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	body := wn.PromptBody(item.Description)
	if descJson {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Description string `json:"description"`
		}{Description: body})
	}
	fmt.Println(body)
	return nil
}

var showCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Output one work item as JSON",
	Long:  "Prints the full work item (id, description, done, tags, dependencies, log, etc.) as JSON. If id is omitted, uses current task. For agents and scripts.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runShow,
}

func runShow(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(item)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize wn in the current directory",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := wn.InitRoot(dir); err != nil {
		return err
	}
	fmt.Println(`wn initialized at ".wn"`)
	return nil
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a work item",
	RunE:  runAdd,
}
var addMessage string
var addTags []string

func init() {
	addCmd.Flags().StringVarP(&addMessage, "message", "m", "", "Description of the work item")
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", nil, "Tag (repeatable)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	msg := addMessage
	if msg == "" {
		var err error
		msg, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if msg == "" {
			return fmt.Errorf("empty description")
		}
	}
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	id, err := wn.GenerateID(store)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          id,
		Description: msg,
		Created:     now,
		Updated:     now,
		Tags:        addTags,
		DependsOn:   nil,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		return err
	}
	if err := wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = id
		return m, nil
	}); err != nil {
		return err
	}
	fmt.Printf("added entry %s\n", id)
	return nil
}

var rmCmd = &cobra.Command{
	Use:   "rm [id]",
	Short: "Remove a work item",
	Long:  "If id is omitted, removes the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRm,
}

func runRm(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	if _, err := store.Get(id); err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	if err := store.Delete(id); err != nil {
		return err
	}
	fmt.Printf("removed entry %s\n", id)
	return nil
}

var editCmd = &cobra.Command{
	Use:   "edit [id]",
	Short: "Edit a work item description in $EDITOR",
	Long:  "If id is omitted, edits the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEdit,
}

func runEdit(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		edited, err := wn.EditWithEditor(it.Description)
		if err != nil {
			return nil, err
		}
		it.Description = edited
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "updated"})
		return it, nil
	})
}

var tagCmd = &cobra.Command{
	Use:   "tag [id] <tag>",
	Short: "Add a tag to a work item",
	Long:  "If id is omitted, tags the current task. Use -i/--interactive to pick items with fzf and toggle the tag on each. Example: wn tag my-tag  or  wn tag -i mytag",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runTag,
}
var tagInteractive bool

func init() {
	tagCmd.Flags().BoolVarP(&tagInteractive, "interactive", "i", false, "Pick work items with fzf (or numbered list); toggle tag on selected items")
}

func runTag(cmd *cobra.Command, args []string) error {
	if tagInteractive {
		return runTagInteractive(args)
	}
	var id, tag string
	if len(args) == 2 {
		id, tag = args[0], args[1]
	} else {
		tag = args[0]
	}
	if err := wn.ValidateTag(tag); err != nil {
		return err
	}
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		var errResolve error
		id, errResolve = wn.ResolveItemID(meta.CurrentID, "")
		if errResolve != nil {
			return fmt.Errorf("no id provided and no current task")
		}
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
	root, err := wn.FindRoot()
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
	items = wn.ApplySort(items, interactiveSortSpec())
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

var untagCmd = &cobra.Command{
	Use:   "untag [id] <tag>",
	Short: "Remove a tag from a work item",
	Long:  "If id is omitted, untags the current task.",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runUntag,
}

func runUntag(cmd *cobra.Command, args []string) error {
	var id, tag string
	if len(args) == 2 {
		id, tag = args[0], args[1]
	} else {
		tag = args[0]
	}
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		var errResolve error
		id, errResolve = wn.ResolveItemID(meta.CurrentID, "")
		if errResolve != nil {
			return fmt.Errorf("no id provided and no current task")
		}
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

var dependCmd = &cobra.Command{
	Use:   "depend [id] --on [id2]",
	Short: "Mark an item as depending on another",
	Long:  "If id is omitted, uses the current task. Use -i/--interactive to pick the depended-on item from undone work items (fzf or numbered list).",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDepend,
}
var dependOn string
var dependInteractive bool

func init() {
	dependCmd.Flags().StringVar(&dependOn, "on", "", "ID of the dependency")
	dependCmd.Flags().BoolVarP(&dependInteractive, "interactive", "i", false, "Pick the depended-on item with fzf (undone items only)")
}

func runDepend(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	var onID string
	if dependInteractive {
		onID, err = runDependInteractive(store, id)
		if err != nil {
			return err
		}
		if onID == "" {
			return nil
		}
	} else {
		if dependOn == "" {
			return fmt.Errorf("required flag \"on\" not set")
		}
		onID = dependOn
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

func runDependInteractive(store wn.Store, excludeID string) (string, error) {
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
	candidates = wn.ApplySort(candidates, interactiveSortSpec())
	return wn.PickInteractive(candidates)
}

var rmdependCmd = &cobra.Command{
	Use:   "rmdepend [id] --on [id2]",
	Short: "Remove a dependency",
	Long:  "If id is omitted, uses the current task. Use -i/--interactive to pick which dependency to remove (fzf or numbered list).",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRmdepend,
}
var rmdependOn string
var rmdependInteractive bool

func init() {
	rmdependCmd.Flags().StringVar(&rmdependOn, "on", "", "ID of the dependency to remove")
	rmdependCmd.Flags().BoolVarP(&rmdependInteractive, "interactive", "i", false, "Pick the dependency to remove with fzf")
}

func runRmdepend(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	var onID string
	if rmdependInteractive {
		onID, err = runRmdependInteractive(store, id)
		if err != nil {
			return err
		}
		if onID == "" {
			return nil
		}
	} else {
		if rmdependOn == "" {
			return fmt.Errorf("required flag \"on\" not set")
		}
		onID = rmdependOn
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

func runRmdependInteractive(store wn.Store, id string) (string, error) {
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
	candidates = wn.ApplySort(candidates, interactiveSortSpec())
	return wn.PickInteractive(candidates)
}

// interactiveSortSpec returns sort options from user settings for fzf/numbered lists. No CLI override.
func interactiveSortSpec() []wn.SortOption {
	settings, err := wn.ReadSettings()
	if err != nil {
		return nil
	}
	return wn.SortSpecFromSettings(settings)
}

var orderCmd = &cobra.Command{
	Use:   "order [id] --set <n> | --unset",
	Short: "Set or clear optional backlog order (lower = earlier when deps don't define order)",
	Long:  "If id is omitted, uses the current task. Use --set to assign a number, --unset to clear.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOrder,
}
var orderSet int
var orderUnset bool

func init() {
	orderCmd.Flags().IntVar(&orderSet, "set", 0, "Set order to this number (lower = earlier in backlog)")
	orderCmd.Flags().BoolVar(&orderUnset, "unset", false, "Clear the order field")
}

func runOrder(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	if orderUnset {
		return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
			it.Order = nil
			it.Updated = time.Now().UTC()
			it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "order_cleared"})
			return it, nil
		})
	}
	if !cmd.Flags().Changed("set") {
		return fmt.Errorf("use --set <n> or --unset")
	}
	n := orderSet
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Order = &n
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "order_set", Msg: fmt.Sprintf("%d", n)})
		return it, nil
	})
}

var doneCmd = &cobra.Command{
	Use:   "done [id]",
	Short: "Mark a work item complete",
	Long:  "If id is omitted, marks the current task complete.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDone,
}
var doneMessage string
var doneForce bool

func init() {
	doneCmd.Flags().StringVarP(&doneMessage, "message", "m", "", "Completion message (e.g. git commit)")
	doneCmd.Flags().BoolVar(&doneForce, "force", false, "Mark complete even if dependencies are not done")
}

func runDone(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	if !doneForce {
		item, err := store.Get(id)
		if err != nil {
			return err
		}
		for _, depID := range item.DependsOn {
			dep, err := store.Get(depID)
			if err != nil {
				return err
			}
			if !dep.Done {
				return fmt.Errorf("dependency %s not complete, use --force to mark complete anyway", depID)
			}
		}
	}
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = true
		it.DoneMessage = doneMessage
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: doneMessage})
		return it, nil
	})
}

var undoneCmd = &cobra.Command{
	Use:   "undone [id]",
	Short: "Mark a work item not complete",
	Long:  "If id is omitted, marks the current task undone.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUndone,
}

func runUndone(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = false
		it.DoneMessage = ""
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "undone"})
		return it, nil
	})
}

var claimCmd = &cobra.Command{
	Use:   "claim [id]",
	Short: "Mark a work item in progress (exclusive until expiration)",
	Long:  "Claims the item so it leaves the undone list until --for duration expires or you run wn done/release. If id is omitted, uses current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runClaim,
}
var claimFor string
var claimBy string

func init() {
	claimCmd.Flags().StringVar(&claimFor, "for", "", "Duration the claim is held (e.g. 30m, 1h); required")
	_ = claimCmd.MarkFlagRequired("for")
	claimCmd.Flags().StringVar(&claimBy, "by", "", "Optional worker ID for logging")
}

func runClaim(cmd *cobra.Command, args []string) error {
	d, err := time.ParseDuration(claimFor)
	if err != nil {
		return fmt.Errorf("invalid --for duration %q: %w", claimFor, err)
	}
	if d <= 0 {
		return fmt.Errorf("--for duration must be positive, got %v", d)
	}
	root, err := wn.FindRoot()
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
		return fmt.Errorf("no id provided and no current task; use wn pick or wn next")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	until := now.Add(d)
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.InProgressUntil = until
		it.InProgressBy = claimBy
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "in_progress", Msg: claimFor})
		return it, nil
	})
}

var releaseCmd = &cobra.Command{
	Use:   "release [id]",
	Short: "Clear in-progress on a work item (return to undone list)",
	Long:  "If id is omitted, releases the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRelease,
}

func runRelease(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "released"})
		return it, nil
	})
}

var logCmd = &cobra.Command{
	Use:   "log [id]",
	Short: "Show history of a work item",
	Long:  "If id is omitted, shows log for the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLog,
}

func runLog(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
	RunE:  runNext,
}

func runNext(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	undone, err := wn.UndoneItems(store)
	if err != nil {
		return err
	}
	ordered, acyclic := wn.TopoOrder(undone)
	if !acyclic || len(ordered) == 0 {
		fmt.Println("No next task.")
		return nil
	}
	next := ordered[0]
	if err := wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = next.ID
		return m, nil
	}); err != nil {
		return err
	}
	fmt.Printf("  %s: %s\n", next.ID, next.Description)
	return nil
}

var pickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Interactively pick a current task (uses fzf if available)",
	RunE:  runPick,
}

func runPick(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	undone, err := wn.UndoneItems(store)
	if err != nil {
		return err
	}
	if len(undone) == 0 {
		fmt.Println("No undone tasks.")
		return nil
	}
	undone = wn.ApplySort(undone, interactiveSortSpec())
	id, err := wn.PickInteractive(undone)
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

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Open wn settings file in $EDITOR",
	RunE:  runSettings,
}

func runSettings(cmd *cobra.Command, args []string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	wnDir := filepath.Join(configDir, "wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		return err
	}
	settingsPath := filepath.Join(wnDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if err := os.WriteFile(settingsPath, []byte("{}\n"), 0644); err != nil {
			return err
		}
	}
	return wn.RunEditorOnFile(settingsPath)
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export all work items to JSON",
	RunE:  runExport,
}
var exportOutput string

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Write to file (default: stdout)")
}

func runExport(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	return wn.Export(store, exportOutput)
}

var importCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import work items from an export file",
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}
var importReplace bool

func init() {
	importCmd.Flags().BoolVar(&importReplace, "replace", false, "Replace existing items (required if store has items)")
}

func runImport(cmd *cobra.Command, args []string) error {
	path := args[0]
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	hasItems, err := wn.StoreHasItems(store)
	if err != nil {
		return err
	}
	if hasItems && !importReplace {
		return fmt.Errorf("store already has items; use --replace to overwrite")
	}
	return wn.ImportReplace(store, path)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List work items (default: undone, in dependency order)",
	RunE:    runList,
}
var listUndone bool
var listDone bool
var listAll bool
var listTag string
var listSort string

var listJson bool

func init() {
	listCmd.Flags().BoolVar(&listUndone, "undone", true, "List undone items (default)")
	listCmd.Flags().BoolVar(&listDone, "done", false, "List done items")
	listCmd.Flags().BoolVar(&listAll, "all", false, "List all items")
	listCmd.Flags().StringVar(&listTag, "tag", "", "Filter by tag")
	listCmd.Flags().StringVar(&listSort, "sort", "", "Sort order (e.g. updated:desc,priority,tags). Overrides settings. Keys: created, updated, priority, alpha, tags")
	listCmd.Flags().BoolVar(&listJson, "json", false, "Output as JSON (array of id and description)")
}

func runList(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	var items []*wn.Item
	if listAll {
		items, err = store.List()
		if err != nil {
			return err
		}
	} else if listDone {
		all, err := store.List()
		if err != nil {
			return err
		}
		for _, it := range all {
			if it.Done {
				items = append(items, it)
			}
		}
	} else {
		items, err = wn.UndoneItems(store)
		if err != nil {
			return err
		}
	}
	if listTag != "" {
		var filtered []*wn.Item
		for _, it := range items {
			for _, t := range it.Tags {
				if t == listTag {
					filtered = append(filtered, it)
					break
				}
			}
		}
		items = filtered
	}
	var ordered []*wn.Item
	sortSpec := listSortSpec()
	if len(sortSpec) > 0 {
		ordered = wn.ApplySort(items, sortSpec)
	} else {
		var acyclic bool
		ordered, acyclic = wn.TopoOrder(items)
		if !acyclic && len(ordered) > 0 {
			ordered = items
		}
	}
	if listJson {
		out := make([]struct {
			ID          string   `json:"id"`
			Description string   `json:"description"`
			Status      string   `json:"status,omitempty"`
			Tags        []string `json:"tags,omitempty"`
		}, len(ordered))
		now := time.Now().UTC()
		for i, it := range ordered {
			out[i].ID = it.ID
			out[i].Description = wn.FirstLine(it.Description)
			out[i].Status = itemListStatus(it, now)
			out[i].Tags = it.Tags
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(out)
	}
	now := time.Now().UTC()
	const listStatusWidth = 7
	const listDescWidth = 51 // so tags align on the right
	for _, it := range ordered {
		status := itemListStatus(it, now)
		desc := wn.FirstLine(it.Description)
		if len(desc) > listDescWidth {
			desc = desc[:listDescWidth-3] + "..."
		}
		tagsStr := formatTags(it.Tags)
		fmt.Printf("  %-6s  %-*s  %-*s  %s\n", it.ID, listStatusWidth, status, listDescWidth, desc, tagsStr)
	}
	return nil
}

// listSortSpec returns sort options from --sort flag or user settings. Invalid spec returns nil.
func listSortSpec() []wn.SortOption {
	if listSort != "" {
		spec, err := wn.ParseSortSpec(listSort)
		if err != nil {
			return nil
		}
		return spec
	}
	settings, err := wn.ReadSettings()
	if err != nil {
		return nil
	}
	return wn.SortSpecFromSettings(settings)
}

// --- note command and subcommands add, list, edit, rm ---

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Add, list, edit, or remove notes (attachments) on a work item",
	Long:  "Notes attach text by logical name (e.g. pr-url, issue-number). Use 'wn note add <name> [id] -m \"...\"', 'wn note list [id]', 'wn note edit [id] <name> -m \"...\"', and 'wn note rm [id] <name>'. Names are alphanumeric, slash, underscore, or hyphen, up to 32 chars.",
}

var noteAddCmd = &cobra.Command{
	Use:   "add <name> [id]",
	Short: "Add or update a note by name on a work item",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteAdd,
}
var noteAddMessage string

func init() {
	noteAddCmd.Flags().StringVarP(&noteAddMessage, "message", "m", "", "Note text (or open $EDITOR if omitted)")
	noteCmd.AddCommand(noteAddCmd, noteListCmd, noteEditCmd, noteRmCmd)
}

func runNoteAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !wn.ValidNoteName(name) {
		return fmt.Errorf("invalid note name %q (alphanumeric, slash, underscore, hyphen, 1-32 chars)", name)
	}
	body := noteAddMessage
	if body == "" {
		var err error
		body, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("empty note")
		}
	}
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
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
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		if it.Notes == nil {
			it.Notes = []wn.Note{}
		}
		idx := it.NoteIndexByName(name)
		trimmed := strings.TrimSpace(body)
		if idx >= 0 {
			it.Notes[idx].Body = trimmed
		} else {
			it.Notes = append(it.Notes, wn.Note{Name: name, Created: now, Body: trimmed})
		}
		it.Updated = now
		return it, nil
	})
}

var noteListCmd = &cobra.Command{
	Use:   "list [id]",
	Short: "List notes on a work item (ordered by create time)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runNoteList,
}

func runNoteList(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
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
		return fmt.Errorf("item %s not found", id)
	}
	for _, n := range item.Notes {
		fmt.Printf("%s\t%s\t%s\n", n.Name, n.Created.Format("2006-01-02 15:04:05"), n.Body)
	}
	return nil
}

var noteEditCmd = &cobra.Command{
	Use:   "edit [id] <name>",
	Short: "Edit a note by name",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteEdit,
}
var noteEditMessage string

func init() {
	noteEditCmd.Flags().StringVarP(&noteEditMessage, "message", "m", "", "New note text (or open $EDITOR with current body if omitted)")
}

func runNoteEdit(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	nameArg := ""
	if len(args) == 2 {
		id, nameArg = args[0], args[1]
	} else {
		id, err = wn.ResolveItemID(meta.CurrentID, "")
		if err != nil {
			return fmt.Errorf("no id provided and no current task")
		}
		nameArg = args[0]
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	body := noteEditMessage
	if body == "" {
		item, err := store.Get(id)
		if err != nil {
			return fmt.Errorf("item %s not found", id)
		}
		idx := item.NoteIndexByName(nameArg)
		if idx < 0 {
			return fmt.Errorf("no note named %q", nameArg)
		}
		var errEdit error
		body, errEdit = wn.EditWithEditor(item.Notes[idx].Body)
		if errEdit != nil {
			return errEdit
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("empty note")
		}
		body = strings.TrimSpace(body)
	} else {
		body = strings.TrimSpace(body)
	}
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		idx := it.NoteIndexByName(nameArg)
		if idx < 0 {
			return nil, fmt.Errorf("no note named %q", nameArg)
		}
		it.Notes[idx].Body = body
		it.Updated = now
		return it, nil
	})
}

var noteRmCmd = &cobra.Command{
	Use:   "rm [id] <name>",
	Short: "Remove a note by name",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteRm,
}

func runNoteRm(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRoot()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	nameArg := ""
	if len(args) == 2 {
		id, nameArg = args[0], args[1]
	} else {
		id, err = wn.ResolveItemID(meta.CurrentID, "")
		if err != nil {
			return fmt.Errorf("no id provided and no current task")
		}
		nameArg = args[0]
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		idx := it.NoteIndexByName(nameArg)
		if idx < 0 {
			return nil, fmt.Errorf("no note named %q", nameArg)
		}
		it.Notes = append(it.Notes[:idx], it.Notes[idx+1:]...)
		it.Updated = time.Now().UTC()
		return it, nil
	})
}

// formatTags returns tags joined with ", " and wrapped in square brackets, or "" if none.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "[" + strings.Join(tags, ", ") + "]"
}

// itemListStatus returns "done", "undone", or "claimed" for list and JSON output.
func itemListStatus(it *wn.Item, now time.Time) string {
	if it.Done {
		return "done"
	}
	if wn.IsInProgress(it, now) {
		return "claimed"
	}
	return "undone"
}
