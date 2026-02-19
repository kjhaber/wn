package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/keith/wn/internal/wn"
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
	rootCmd.AddCommand(initCmd, addCmd, rmCmd, editCmd, tagCmd, untagCmd, dependCmd, rmdependCmd, doneCmd, undoneCmd, claimCmd, releaseCmd, logCmd, descCmd, nextCmd, pickCmd, settingsCmd, exportCmd, importCmd, listCmd)
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
	fmt.Printf("current task: [%s] %s\n", item.ID, item.Description)
	return nil
}

var descCmd = &cobra.Command{
	Use:   "desc [id]",
	Short: "Print a work item description (prompt-ready: title only or body only)",
	Long:  "Output is suitable for pasting into an agent prompt. If id is omitted, uses current task. Single-line descriptions are printed as-is; multi-line descriptions print only the lines after the title.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDesc,
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
	fmt.Println(wn.PromptBody(item.Description))
	return nil
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
	Long:  "If id is omitted, tags the current task. Example: wn tag my-tag  or  wn tag abc123 my-tag",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runTag,
}

func runTag(cmd *cobra.Command, args []string) error {
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
	Long:  "If id is omitted, uses the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDepend,
}
var dependOn string

func init() {
	dependCmd.Flags().StringVar(&dependOn, "on", "", "ID of the dependency")
	_ = dependCmd.MarkFlagRequired("on")
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
	onID := dependOn
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
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

var rmdependCmd = &cobra.Command{
	Use:   "rmdepend [id] --on [id2]",
	Short: "Remove a dependency",
	Long:  "If id is omitted, uses the current task.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRmdepend,
}
var rmdependOn string

func init() {
	rmdependCmd.Flags().StringVar(&rmdependOn, "on", "", "ID of the dependency to remove")
	_ = rmdependCmd.MarkFlagRequired("on")
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
	onID := rmdependOn
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
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

func init() {
	listCmd.Flags().BoolVar(&listUndone, "undone", true, "List undone items (default)")
	listCmd.Flags().BoolVar(&listDone, "done", false, "List done items")
	listCmd.Flags().BoolVar(&listAll, "all", false, "List all items")
	listCmd.Flags().StringVar(&listTag, "tag", "", "Filter by tag")
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
	ordered, acyclic := wn.TopoOrder(items)
	if !acyclic && len(ordered) > 0 {
		// Still print something; cycle only matters for depend command
		ordered = items
	}
	for _, it := range ordered {
		fmt.Printf("  %s: %s\n", it.ID, wn.FirstLine(it.Description))
	}
	return nil
}
