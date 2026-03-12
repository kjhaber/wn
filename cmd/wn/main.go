package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

var pickerFlag string

var rootCmd = &cobra.Command{
	Use:   "wn",
	Short: "What's Next — local task/work item tracker",
	Long:  `wn is a CLI for tracking work items. Use wn init to create a tracker in the current directory.`,
	Args:  cobra.MaximumNArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Determine effective picker mode: settings, overridden by --picker flag.
		mode := ""
		root, err := wn.FindRootForCLI()
		if err == nil {
			settings, _ := wn.ReadSettingsInRoot(root)
			mode = settings.Picker
		}
		if cmd.Root().PersistentFlags().Changed("picker") {
			mode = pickerFlag
		}
		return wn.SetPickerMode(mode)
	},
	RunE: runCurrent,
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("wn version {{.Version}}\n")
	rootCmd.PersistentFlags().StringVar(&pickerFlag, "picker", "", "Picker mode: fzf, numbered, or empty (auto-detect)")
	rootCmd.AddCommand(initCmd, addCmd, rmCmd, archiveCmd, editCmd, tagCmd, dependCmd, doneCmd, undoneCmd, statusCmd, claimCmd, releaseCmd, reviewReadyCmd, cleanupCmd, mergeCmd, logCmd, showCmd, nextCmd, pickCmd, mcpCmd, doCmd, launchCmd, worktreeSetupCmd, settingsCmd, exportCmd, importCmd, listCmd, noteCmd, tuiCmd, promptCmd, respondCmd)
	rootCmd.CompletionOptions.DisableDefaultCmd = false
}

// defaultShowFields is the built-in default for bare 'wn [id]' and 'wn show [id]'
// when no --fields flag is given and settings.Show.DefaultFields is empty.
const defaultShowFields = "title,body,deps,notes"

func runCurrent(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	if len(args) > 0 {
		id = args[0]
	} else {
		if meta.CurrentID == "" {
			fmt.Println("No current task. Use 'wn pick' to choose one or 'wn next' to advance.")
			return nil
		}
		id = meta.CurrentID
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
	if err != nil {
		if len(args) == 0 {
			fmt.Printf("Current task ID %s not found. Use 'wn pick' to choose one.\n", id)
			return nil
		}
		return fmt.Errorf("item %s not found", id)
	}
	settings, _ := wn.ReadSettingsInRoot(root)
	fields := resolveShowFields(false, "", settings)
	return renderItemHuman(item, fields, store)
}

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
		fmt.Println("notes:")
		for _, n := range item.Notes {
			fmt.Printf("  %s\t%s\t%s\n", n.Name, n.Created.Format(timeFmt), n.Body)
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
	root, err := wn.FindRootForCLI()
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
	Use:   "rm [id ...]",
	Short: "Remove a work item",
	Long:  "If no id is given, shows an interactive list (fzf or numbered) with multi-select to remove several items at once. Pass one or more ids to remove those directly.",
	Args:  cobra.ArbitraryArgs,
	RunE:  runRm,
}

func runRm(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}

	var idsToRemove []string
	if len(args) == 0 {
		items, err := store.List()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No tasks.")
			return nil
		}
		items = wn.ApplySort(items, interactiveSortSpec(root))
		idsToRemove, err = wn.PickMultiInteractive(items)
		if err != nil {
			return err
		}
		if len(idsToRemove) == 0 {
			return nil
		}
	} else {
		idsToRemove = args
	}

	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	clearCurrent := false
	for _, id := range idsToRemove {
		if _, err := store.Get(id); err != nil {
			return fmt.Errorf("item %s not found", id)
		}
		if id == meta.CurrentID {
			clearCurrent = true
		}
		if err := store.Delete(id); err != nil {
			return err
		}
		fmt.Printf("removed entry %s\n", id)
	}
	if clearCurrent {
		return wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
			m.CurrentID = ""
			return m, nil
		})
	}
	return nil
}

var archiveLocation string

var archiveCmd = &cobra.Command{
	Use:   "archive [id]",
	Short: "Archive a work item",
	Long: `Archive a work item: saves its content to an archive file then removes it from the project.

The archived item can be recovered with 'wn import'.

By default, archives are saved under .wn/archive/<id>.json. Use --location to override.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runArchive,
}

func init() {
	archiveCmd.Flags().StringVar(&archiveLocation, "location", "", "Directory to write archive file (default: .wn/archive)")
}

func runArchive(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}

	var id string
	if len(args) == 0 {
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		items, err := store.List()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No tasks.")
			return nil
		}
		items = wn.ApplySort(items, interactiveSortSpec(root))
		ids, err := wn.PickMultiInteractive(items)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		clearCurrent := false
		for _, aid := range ids {
			archivePath, err := wn.ArchiveItem(store, aid, archiveLocation)
			if err != nil {
				return err
			}
			fmt.Printf("archived %s -> %s\n", aid, archivePath)
			if aid == meta.CurrentID {
				clearCurrent = true
			}
		}
		if clearCurrent {
			return wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
				m.CurrentID = ""
				return m, nil
			})
		}
		return nil
	}

	id = args[0]
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	archivePath, err := wn.ArchiveItem(store, id, archiveLocation)
	if err != nil {
		return err
	}
	fmt.Printf("archived %s -> %s\n", id, archivePath)
	if id == meta.CurrentID {
		return wn.WithMetaLock(root, func(m wn.Meta) (wn.Meta, error) {
			m.CurrentID = ""
			return m, nil
		})
	}
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
		fmt.Fprintln(out, t)
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
		fmt.Fprintln(out, depID)
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

var doneCmd = &cobra.Command{
	Use:   "done [id]",
	Short: "Mark a work item complete",
	Long:  "If id is omitted, marks the current task complete. Use --next to then set the next undone item as current (convenience for done + next).",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDone,
}
var doneMessage string
var doneForce bool
var doneNext bool

func init() {
	doneCmd.Flags().StringVarP(&doneMessage, "message", "m", "", "Completion message (e.g. git commit)")
	doneCmd.Flags().BoolVar(&doneForce, "force", false, "Mark complete even if dependencies are not done")
	doneCmd.Flags().BoolVar(&doneNext, "next", false, "After marking done, set the next undone item as current (like running wn next)")
}

func runDone(cmd *cobra.Command, args []string) error {
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
	if !doneForce {
		for _, depID := range item.DependsOn {
			dep, err := store.Get(depID)
			if err != nil {
				return err
			}
			if dep.Done || dep.PromptReady {
				continue
			}
			return fmt.Errorf("dependency %s not complete, use --force to mark complete anyway", depID)
		}
	}
	now := time.Now().UTC()
	// Auto-mark prompt deps as done.
	for _, depID := range item.DependsOn {
		dep, err := store.Get(depID)
		if err != nil {
			return err
		}
		if dep.PromptReady {
			if err := store.UpdateItem(depID, func(it *wn.Item) (*wn.Item, error) {
				it.Done = true
				it.PromptReady = false
				it.DoneStatus = wn.DoneStatusDone
				it.Updated = now
				it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: "auto-closed with parent"})
				return it, nil
			}); err != nil {
				return err
			}
		}
	}
	if err := store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = true
		it.DoneMessage = doneMessage
		it.DoneStatus = wn.DoneStatusDone
		it.ReviewReady = false
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: doneMessage})
		return it, nil
	}); err != nil {
		return err
	}
	if !doneNext {
		return nil
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

var undoneCmd = &cobra.Command{
	Use:   "undone [id]",
	Short: "Mark a work item not complete",
	Long:  "If id is omitted, marks the current task undone.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUndone,
}

func runUndone(cmd *cobra.Command, args []string) error {
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
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = false
		it.DoneMessage = ""
		it.DoneStatus = ""
		it.ReviewReady = false
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "undone"})
		return it, nil
	})
}

var statusCmd = &cobra.Command{
	Use:   "status <undone|claimed|review|done|closed|suspend> [id]",
	Short: "Set work item status",
	Long:  "Set the work item to the given status. If id is omitted, uses the current task. Use --for when setting to claimed (duration, e.g. 30m); -m for a message when setting to done/closed/suspend. Use --duplicate-of <id> when setting to closed to mark the item as a duplicate of another (adds duplicate-of note).",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runStatus,
}
var statusFor string
var statusMessage string
var statusClaimBy string
var statusDuplicateOf string

func init() {
	statusCmd.Flags().StringVar(&statusFor, "for", "", "Claim duration when setting to claimed (e.g. 30m, 1h); default 1h")
	statusCmd.Flags().StringVarP(&statusMessage, "message", "m", "", "Optional message when setting to done, closed, or suspend")
	statusCmd.Flags().StringVar(&statusClaimBy, "by", "", "Optional worker ID when setting to claimed")
	statusCmd.Flags().StringVar(&statusDuplicateOf, "duplicate-of", "", "When setting to closed: mark item as duplicate of this work item id (adds duplicate-of note)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	state := args[0]
	if !wn.ValidStatus(state) {
		return fmt.Errorf("invalid status %q; must be one of: undone, claimed, review, done, closed, suspend", state)
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
	if state != wn.StatusClosed && statusDuplicateOf != "" {
		return fmt.Errorf("--duplicate-of is only valid when setting status to closed")
	}
	opts := wn.StatusOpts{DoneMessage: statusMessage, ClaimBy: statusClaimBy, DuplicateOf: statusDuplicateOf}
	if state == wn.StatusClaimed && statusFor != "" {
		d, err := time.ParseDuration(statusFor)
		if err != nil {
			return fmt.Errorf("invalid --for duration %q: %w", statusFor, err)
		}
		if d <= 0 {
			return fmt.Errorf("--for duration must be positive, got %v", d)
		}
		opts.ClaimFor = d
	}
	if err := wn.SetStatus(store, id, state, opts); err != nil {
		return err
	}
	if state == wn.StatusClosed && statusDuplicateOf != "" {
		fmt.Printf("marked %s as duplicate of %s\n", id, statusDuplicateOf)
	} else {
		fmt.Printf("marked %s %s\n", id, state)
	}
	return nil
}

var claimCmd = &cobra.Command{
	Use:   "claim [id]",
	Short: "Mark a work item in progress (exclusive until expiration)",
	Long:  "Claims the item so it leaves the undone list until --for duration expires or you run wn done/release. If id is omitted, uses current task. Omit --for to use default (1h) and renew/extend a claim without losing context.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runClaim,
}
var claimFor string
var claimBy string

func init() {
	claimCmd.Flags().StringVar(&claimFor, "for", "", "Duration the claim is held (e.g. 30m, 1h); default 1h so you can renew with just wn claim")
	claimCmd.Flags().StringVar(&claimBy, "by", "", "Optional worker ID for logging")
}

func runClaim(cmd *cobra.Command, args []string) error {
	d := wn.DefaultClaimDuration
	if claimFor != "" {
		var err error
		d, err = time.ParseDuration(claimFor)
		if err != nil {
			return fmt.Errorf("invalid --for duration %q: %w", claimFor, err)
		}
		if d <= 0 {
			return fmt.Errorf("--for duration must be positive, got %v", d)
		}
	}
	claimForMsg := claimFor
	if claimForMsg == "" {
		claimForMsg = d.String()
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
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "in_progress", Msg: claimForMsg})
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
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.ReviewReady = true
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "released"})
		return it, nil
	})
}

var reviewReadyCmd = &cobra.Command{
	Use:     "review-ready [id]",
	Aliases: []string{"rr"},
	Short:   "Set work item to review-ready (excluded from wn next until marked done)",
	Long:    "If id is omitted, uses the current task. Clears in-progress and marks the item review-ready.",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runReviewReady,
}

func runReviewReady(cmd *cobra.Command, args []string) error {
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
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = false
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.ReviewReady = true
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "review_ready"})
		return it, nil
	})
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Bulk maintenance utilities for work items",
}

var cleanupSetMergedReviewItemsDoneCmd = &cobra.Command{
	Use:   "set-merged-review-items-done",
	Short: "Mark review items done when their work has been merged",
	Long:  "Checks all review-ready work items, finds their 'branch' note, and marks them done if that branch (or recorded commit) has been merged into the current branch (or --branch). Use --dry-run to see what would be marked without making changes.",
	Args:  cobra.NoArgs,
	RunE:  runCleanupSetMergedReviewItemsDone,
}

var cleanupMergedDryRun bool
var cleanupMergedBranch string

var cleanupCloseDoneItemsCmd = &cobra.Command{
	Use:   "close-done-items",
	Short: "Close done items older than a configurable age",
	Long:  "Finds items in done state whose done time is older than the configured age and sets them to closed. Age comes from --age or settings cleanup.close_done_items_age.",
	Args:  cobra.NoArgs,
	RunE:  runCleanupCloseDoneItems,
}

var cleanupCloseDoneItemsAge string
var cleanupCloseDoneItemsDryRun bool

func init() {
	cleanupSetMergedReviewItemsDoneCmd.Flags().BoolVar(&cleanupMergedDryRun, "dry-run", false, "Report what would be marked without making changes")
	cleanupSetMergedReviewItemsDoneCmd.Flags().StringVarP(&cleanupMergedBranch, "branch", "b", "", "Check merged into this ref (default: current HEAD)")
	cleanupCloseDoneItemsCmd.Flags().StringVar(&cleanupCloseDoneItemsAge, "age", "", "Age threshold (e.g. 30d, 7d, 48h); items done longer ago are closed")
	cleanupCloseDoneItemsCmd.Flags().BoolVar(&cleanupCloseDoneItemsDryRun, "dry-run", false, "Report what would be closed without making changes")
	cleanupCmd.AddCommand(cleanupSetMergedReviewItemsDoneCmd, cleanupCloseDoneItemsCmd)
}

func runCleanupSetMergedReviewItemsDone(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	intoRef := cleanupMergedBranch
	results, err := wn.MarkMergedItems(store, root, intoRef, cleanupMergedDryRun)
	if err != nil {
		return err
	}
	for _, r := range results {
		switch r.Status {
		case "marked":
			prefix := "marked"
			if cleanupMergedDryRun {
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

func runCleanupCloseDoneItems(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return err
	}
	ageStr := cleanupCloseDoneItemsAge
	if ageStr == "" {
		ageStr = settings.Cleanup.CloseDoneItemsAge
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
	results, err := wn.CloseDoneItems(store, cutoff, cleanupCloseDoneItemsDryRun)
	if err != nil {
		return err
	}
	for _, r := range results {
		switch r.Status {
		case "closed":
			prefix := "closed"
			if cleanupCloseDoneItemsDryRun {
				prefix = "would close"
			}
			fmt.Printf("%s %s: %s\n", prefix, r.ID, r.Reason)
		case "skipped_not_done", "skipped_not_old_enough":
			fmt.Printf("skip %s: %s\n", r.ID, r.Reason)
		default:
			// Unknown status: still print for visibility.
			fmt.Printf("%s %s: %s\n", r.Status, r.ID, r.Reason)
		}
	}
	return nil
}

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge a review-ready work item's branch into main",
	Long:  "From the main worktree: checkout the work item's branch (removing its worktree if present), run validate (e.g. make), rebase main, checkout main, merge the branch, run validate again, mark the item done, and delete the branch. Use current task or --wid <id>. Logs activity with timestamps to stderr (same as wn agent-orch). On validate or rebase failure, exits with instructions for the agent to fix and re-run.",
	RunE:  runMerge,
}

var (
	mergeWID         string
	mergeMainBranch  string
	mergeValidateCmd string
)

func init() {
	mergeCmd.Flags().StringVar(&mergeWID, "wid", "", "Work item id to merge; omit to use current task (must be review-ready)")
	mergeCmd.Flags().StringVar(&mergeMainBranch, "main-branch", "main", "Branch to rebase onto and merge into")
	mergeCmd.Flags().StringVar(&mergeValidateCmd, "validate", "make", "Build/validation command to run before and after merge (e.g. make)")
}

func runMerge(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	opts := wn.MergeOpts{
		Root:        root,
		WorkID:      mergeWID,
		MainBranch:  mergeMainBranch,
		ValidateCmd: mergeValidateCmd,
		Audit:       os.Stderr,
	}
	if err := wn.RunMerge(store, opts); err != nil {
		return err
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
	nextCmd.Flags().StringVar(&nextTag, "tag", "", "Only consider items with this tag (next undone in dependency order)")
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

func initPick() {
	pickCmd.Flags().BoolVar(&pickUndone, "undone", false, "Pick from undone items only (default)")
	pickCmd.Flags().BoolVar(&pickDone, "done", false, "Pick from done items only")
	pickCmd.Flags().BoolVar(&pickAll, "all", false, "Pick from all items")
	pickCmd.Flags().BoolVar(&pickReviewReady, "rr", false, "Pick from review-ready items only")
	pickCmd.Flags().BoolVar(&pickReviewReady, "review-ready", false, "Pick from review-ready items only")
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

Runner is resolved from settings.runners; defaults to agent.default_launch.`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runLaunch,
}

var (
	launchNext         bool
	launchClaim        string
	launchWorktreeBase string
	launchBranch       string
	launchBranchPrefix string
	launchTag          string
)

func init() {
	launchCmd.Flags().BoolVar(&launchNext, "next", false, "Dispatch for the next undone item from the queue.")
	launchCmd.Flags().StringVar(&launchClaim, "claim", "", "Claim duration per item (e.g. 2h). Overrides settings.")
	launchCmd.Flags().StringVar(&launchWorktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	launchCmd.Flags().StringVar(&launchBranch, "branch", "", "Default branch override (e.g. main). Overrides settings.")
	launchCmd.Flags().StringVar(&launchBranchPrefix, "branch-prefix", "", "Prefix for generated branch names. Overrides settings.")
	launchCmd.Flags().StringVar(&launchTag, "tag", "", "Only consider items with this tag (with --next). Overrides settings.")
}

func runLaunch(cmd *cobra.Command, args []string) error {
	isNext, _ := cmd.Flags().GetBool("next")
	flagClaim, _ := cmd.Flags().GetString("claim")
	flagWorktreeBase, _ := cmd.Flags().GetString("worktree-base")
	flagBranch, _ := cmd.Flags().GetString("branch")
	flagBranchPrefix, _ := cmd.Flags().GetString("branch-prefix")
	flagTag, _ := cmd.Flags().GetString("tag")

	_ = cmd.Flags().Set("next", "false")
	_ = cmd.Flags().Set("claim", "")
	_ = cmd.Flags().Set("worktree-base", "")
	_ = cmd.Flags().Set("branch", "")
	_ = cmd.Flags().Set("branch-prefix", "")
	_ = cmd.Flags().Set("tag", "")

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

	// Determine the work item (or validate current task) before resolving runner.
	tag := ns.Tag
	if flagTag != "" {
		tag = flagTag
	}
	_ = as // suppress unused warning; orchestrator fields (delay/poll) not used for launch

	var orchWorkID string
	var orchFailIfEmpty bool
	var orchMaxTasks int
	switch {
	case isNext:
		orchFailIfEmpty = true
		orchMaxTasks = 1
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
)

func init() {
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupClaim, "claim", "", "Claim duration (e.g. 2h). Overrides settings.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupBranchPrefix, "branch-prefix", "", "Branch name prefix (e.g. keith/). Overrides settings.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupWorktreeBase, "worktree-base", "", "Base directory for worktrees. Overrides settings.")
	worktreeSetupCmd.Flags().StringVar(&worktreeSetupTag, "tag", "", "Only consider items with this tag (with --next).")
	worktreeSetupCmd.Flags().BoolVar(&worktreeSetupNext, "next", false, "Claim the next undone item from the queue.")
}

func runWorktreeSetup(cmd *cobra.Command, args []string) error {
	// Read all flags from cmd directly; package-level flag vars may retain stale values
	// across successive Execute() calls (e.g. in tests).
	isNext, _ := cmd.Flags().GetBool("next")
	flagClaim, _ := cmd.Flags().GetString("claim")
	flagBranchPrefix, _ := cmd.Flags().GetString("branch-prefix")
	flagWorktreeBase, _ := cmd.Flags().GetString("worktree-base")
	flagTag, _ := cmd.Flags().GetString("tag")

	// Reset flags so they don't persist across test invocations.
	_ = cmd.Flags().Set("next", "false")
	_ = cmd.Flags().Set("claim", "")
	_ = cmd.Flags().Set("branch-prefix", "")
	_ = cmd.Flags().Set("worktree-base", "")
	_ = cmd.Flags().Set("tag", "")

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

	worktreePath, branchName, err := wn.SetupItemWorktree(store, root, item, worktreesBase, mainDirname, branchPrefix, os.Stderr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "claimed %s: %s\nbranch: %s\n", item.ID, wn.FirstLine(item.Description), branchName)
	fmt.Println(worktreePath)
	return nil
}

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Open wn settings file in $EDITOR",
	Long:  "Opens user-level settings (~/.config/wn/settings.json) in $EDITOR. Use --project to open project-level settings (.wn/settings.json) which override user settings when present.",
	RunE:  runSettings,
}
var settingsProject bool

func init() {
	settingsCmd.Flags().BoolVar(&settingsProject, "project", false, "Edit project-level settings (.wn/settings.json) instead of user settings")
}

func runSettings(cmd *cobra.Command, args []string) error {
	if settingsProject {
		root, err := wn.FindRootForCLI()
		if err != nil {
			return err
		}
		wnDir := filepath.Join(root, ".wn")
		if err := os.MkdirAll(wnDir, 0755); err != nil {
			return err
		}
		settingsPath := wn.ProjectSettingsPath(root)
		if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
			if err := os.WriteFile(settingsPath, []byte("{}\n"), 0644); err != nil {
				return err
			}
		}
		return wn.RunEditorOnFile(settingsPath)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	wnDir := filepath.Join(configDir, "wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		return err
	}
	settingsPath, err := wn.SettingsPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if err := os.WriteFile(settingsPath, []byte("{}\n"), 0644); err != nil {
			return err
		}
	}
	return wn.RunEditorOnFile(settingsPath)
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export work items to JSON (optionally filtered by criteria)",
	RunE:  runExport,
}
var exportOutput string
var exportAll bool
var exportUndone bool
var exportDone bool
var exportTag string

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Write to file (default: stdout)")
	exportCmd.Flags().BoolVar(&exportAll, "all", false, "Export all items (default when no status filter)")
	exportCmd.Flags().BoolVar(&exportUndone, "undone", false, "Export only undone items")
	exportCmd.Flags().BoolVar(&exportDone, "done", false, "Export only done items")
	exportCmd.Flags().StringVar(&exportTag, "tag", "", "Export only items with this tag")
}

func runExport(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	useCriteria := exportAll || exportUndone || exportDone || exportTag != ""
	if !useCriteria {
		return wn.Export(store, exportOutput)
	}
	var items []*wn.Item
	if exportUndone {
		items, err = wn.ListableUndoneItems(store)
		if err != nil {
			return err
		}
	} else if exportDone {
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
		// --all or only --tag
		items, err = store.List()
		if err != nil {
			return err
		}
	}
	if exportTag != "" {
		var filtered []*wn.Item
		for _, it := range items {
			for _, t := range it.Tags {
				if t == exportTag {
					filtered = append(filtered, it)
					break
				}
			}
		}
		items = filtered
	}
	return wn.ExportItems(items, exportOutput)
}

var importCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import work items from an export file",
	Long:  "Import work items from a JSON export file. When the store already has items, you must choose --append (add/merge from file) or --replace (delete all existing, then load file). When the store is empty, either flag is optional.",
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}
var importReplace bool
var importAppend bool

func init() {
	importCmd.Flags().BoolVar(&importAppend, "append", false, "Add items from file to the store (merge by ID; same ID overwrites)")
	importCmd.Flags().BoolVar(&importReplace, "replace", false, "Replace all existing items with the contents of the file")
}

func runImport(cmd *cobra.Command, args []string) error {
	if importAppend && importReplace {
		return fmt.Errorf("cannot use both --append and --replace; choose one")
	}
	path := args[0]
	root, err := wn.FindRootForCLI()
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
	if hasItems && !importAppend && !importReplace {
		return fmt.Errorf("store already has items; use --append to add to existing items or --replace to replace all")
	}
	if importReplace {
		return wn.ImportReplace(store, path)
	}
	return wn.ImportAppend(store, path)
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
	listCmd.Flags().StringVar(&listTag, "tag", "", "Filter by tag")
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

// --- note command and subcommands add, list, edit, rm ---

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Add, list, edit, remove, or show notes (attachments) on a work item",
	Long:  "Notes attach text by logical name (e.g. pr-url, issue-number). Use 'wn note add <name> [id] -m \"...\"', 'wn note list [id]', 'wn note show [id] <name>', 'wn note edit [id] <name> -m \"...\"', and 'wn note rm [id] <name>'. Names are alphanumeric, slash, underscore, or hyphen, up to 32 chars.",
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
	noteCmd.AddCommand(noteAddCmd, noteListCmd, noteShowCmd, noteEditCmd, noteRmCmd)
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
	root, err := wn.FindRootForCLI()
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

var noteShowCmd = &cobra.Command{
	Use:   "show [id] <name>",
	Short: "Print the body of a named note",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteShow,
}

func runNoteShow(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	var nameArg string
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
	item, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	idx := item.NoteIndexByName(nameArg)
	if idx < 0 {
		return fmt.Errorf("no note named %q", nameArg)
	}
	fmt.Println(item.Notes[idx].Body)
	return nil
}

var noteListCmd = &cobra.Command{
	Use:   "list [id]",
	Short: "List notes on a work item (ordered by create time)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runNoteList,
}

func runNoteList(cmd *cobra.Command, args []string) error {
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
	root, err := wn.FindRootForCLI()
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
	root, err := wn.FindRootForCLI()
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

// itemListStatus returns the display status for list output.
func itemListStatus(it *wn.Item, now time.Time, blocked bool) string {
	return wn.ItemListStatus(it, now, blocked)
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
