package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

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

var rmCmd = &cobra.Command{
	Use:   "rm [id ...]",
	Short: "Remove a work item",
	Long:  "If no id is given, removes the current task. Pass one or more ids to remove those directly.",
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
		meta, err := wn.ReadMeta(root)
		if err != nil {
			return err
		}
		if meta.CurrentID == "" {
			return fmt.Errorf("no current task; specify an id to remove")
		}
		idsToRemove = []string{meta.CurrentID}
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

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage wn settings",
	Long:  "Subcommands: show (print effective merged settings as JSON), edit (open a settings file in $EDITOR).",
}

var settingsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print effective merged settings as JSON",
	Long:  "Prints the fully merged effective settings (user + user-local + project + project-local) as JSON.",
	RunE:  runSettingsShow,
}

var settingsEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open a settings file in $EDITOR",
	Long: `Interactively pick a settings file to open in $EDITOR. Use a flag to skip the picker:
  --user          edit user settings (WN_SETTINGS_USER or default)
  --user-local    edit user-local settings (WN_SETTINGS_USER_LOCAL)
  --project       edit project settings (.wn/settings.json)
  --project-local edit project-local settings (.wn/settings.local.json)

Missing files are created as {} before opening.`,
	RunE: runSettingsEdit,
}

var settingsEditUser bool
var settingsEditUserLocal bool
var settingsEditProject bool
var settingsEditProjectLocal bool

func init() {
	settingsCmd.AddCommand(settingsShowCmd, settingsEditCmd)
	settingsEditCmd.Flags().BoolVar(&settingsEditUser, "user", false, "Edit user settings")
	settingsEditCmd.Flags().BoolVar(&settingsEditUserLocal, "user-local", false, "Edit user-local settings (requires WN_SETTINGS_USER_LOCAL)")
	settingsEditCmd.Flags().BoolVar(&settingsEditProject, "project", false, "Edit project settings (.wn/settings.json)")
	settingsEditCmd.Flags().BoolVar(&settingsEditProjectLocal, "project-local", false, "Edit project-local settings (.wn/settings.local.json)")
}

func runSettingsShow(cmd *cobra.Command, _ []string) error {
	root, _ := wn.FindRootForCLI()
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

type settingsFileEntry struct {
	label string
	path  string
}

func settingsEditCandidates(root string) ([]settingsFileEntry, error) {
	named, err := wn.UserSettingsNamedPaths()
	if err != nil {
		return nil, err
	}
	var candidates []settingsFileEntry
	for _, n := range named {
		candidates = append(candidates, settingsFileEntry{label: n.Name, path: n.Path})
	}
	if root != "" {
		candidates = append(candidates, settingsFileEntry{label: "project", path: wn.ProjectSettingsPath(root)})
		candidates = append(candidates, settingsFileEntry{label: "project-local", path: wn.ProjectLocalSettingsPath(root)})
	}
	return candidates, nil
}

func openSettingsFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
			return err
		}
	}
	return wn.RunEditorOnFile(path)
}

func runSettingsEdit(_ *cobra.Command, _ []string) error {
	root, _ := wn.FindRootForCLI()
	candidates, err := settingsEditCandidates(root)
	if err != nil {
		return err
	}

	var targetLabel string
	switch {
	case settingsEditUser:
		targetLabel = "user"
	case settingsEditUserLocal:
		targetLabel = "user-local"
	case settingsEditProject:
		targetLabel = "project"
	case settingsEditProjectLocal:
		targetLabel = "project-local"
	}
	if targetLabel != "" {
		for _, c := range candidates {
			if c.label == targetLabel {
				return openSettingsFile(c.path)
			}
		}
		return fmt.Errorf("settings file %q not available (for user-local, set WN_SETTINGS_USER_LOCAL; for project files, run from a wn project)", targetLabel)
	}

	lines := make([]string, len(candidates))
	for i, c := range candidates {
		_, err := os.Stat(c.path)
		exists := err == nil
		label := "[" + c.label + "]"
		if exists {
			lines[i] = fmt.Sprintf("%-16s %s", label, c.path)
		} else {
			lines[i] = fmt.Sprintf("%-16s %s  [new]", label, c.path)
		}
	}

	idx, err := wn.PickStringInteractive(lines)
	if err != nil {
		return err
	}
	if idx < 0 {
		return nil
	}
	return openSettingsFile(candidates[idx].path)
}

var verifyAtRoot bool

var verifyCmd = &cobra.Command{
	Use:          "verify",
	Short:        "Run the configured verify command (e.g. make all, npm test)",
	Long:         "Runs the shell command configured in settings.verify. Set it in project settings (.wn/settings.json) or user settings (~/.config/wn/settings.json). Useful for agents and humans alike to confirm the build passes.",
	RunE:         runVerify,
	SilenceUsage: true,
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyAtRoot, "root", false, "Change to the project root directory before running verify")
}

func runVerify(cobraCmd *cobra.Command, _ []string) error {
	root, _ := wn.FindRootForCLI()
	if verifyAtRoot {
		if root == "" {
			return wn.ErrNoRoot
		}
		if err := os.Chdir(root); err != nil {
			return err
		}
	}
	settings, err := wn.ReadSettingsInRoot(root)
	if err != nil {
		return err
	}
	if settings.Verify == "" {
		return fmt.Errorf("no verify command configured; set 'verify' in .wn/settings.json or ~/.config/wn/settings.json")
	}
	_, _ = fmt.Fprintf(cobraCmd.ErrOrStderr(), "running: %s\n", settings.Verify)
	cmd := exec.Command("sh", "-c", settings.Verify)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = cobraCmd.ErrOrStderr()
	return cmd.Run()
}

var repoRootCmd = &cobra.Command{
	Use:          "root",
	Short:        "Print the main git repository root (worktree-aware)",
	Long:         "Prints the absolute path to the main git repository root. In a linked worktree, this resolves to the main repo root rather than the worktree directory. Useful in scripts and hooks that need to operate on the main repo from a feature worktree.",
	Args:         cobra.NoArgs,
	RunE:         runRepoRoot,
	SilenceUsage: true,
}

func runRepoRoot(_ *cobra.Command, _ []string) error {
	root, err := wn.GitRepoRoot()
	if err != nil {
		return err
	}
	fmt.Println(root)
	return nil
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
var exportReviewReady bool
var exportTag string
var exportSort string
var exportLimit int
var exportOffset int

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Write to file (default: stdout)")
	exportCmd.Flags().BoolVar(&exportAll, "all", false, "Export all items (default when no status filter)")
	exportCmd.Flags().BoolVar(&exportUndone, "undone", false, "Export only undone items")
	exportCmd.Flags().BoolVar(&exportDone, "done", false, "Export only done items")
	exportCmd.Flags().BoolVar(&exportReviewReady, "review-ready", false, "Export only review-ready items")
	exportCmd.Flags().BoolVar(&exportReviewReady, "rr", false, "Export only review-ready items")
	exportCmd.Flags().StringVar(&exportTag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	exportCmd.Flags().StringVar(&exportSort, "sort", "", "Sort order (e.g. updated:desc,priority,tags). Overrides settings. Keys: created, updated, priority, alpha, tags")
	exportCmd.Flags().IntVar(&exportLimit, "limit", 0, "Return at most N items (0 = no limit)")
	exportCmd.Flags().IntVar(&exportOffset, "offset", 0, "Skip first N items")
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
	stateFlags := 0
	if exportAll {
		stateFlags++
	}
	if exportUndone {
		stateFlags++
	}
	if exportDone {
		stateFlags++
	}
	if exportReviewReady {
		stateFlags++
	}
	if stateFlags > 1 {
		return fmt.Errorf("only one of --undone, --done, --all, --review-ready may be set")
	}
	useCriteria := stateFlags > 0 || exportTag != "" || exportSort != "" || exportLimit > 0 || exportOffset > 0
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
	} else if exportReviewReady {
		items, err = wn.ReviewReadyItems(store)
		if err != nil {
			return err
		}
	} else {
		// --all, or only --tag/--sort/--limit/--offset with no state filter: load everything
		items, err = store.List()
		if err != nil {
			return err
		}
	}
	items = wn.FilterByTag(items, exportTag)
	// Apply sort
	var sortSpec []wn.SortOption
	if exportSort != "" {
		sortSpec, err = wn.ParseSortSpec(exportSort)
		if err != nil {
			return err
		}
	} else {
		settings, _ := wn.ReadSettingsInRoot(root)
		sortSpec = wn.SortSpecFromSettings(settings)
	}
	var ordered []*wn.Item
	if len(sortSpec) > 0 {
		ordered = wn.ApplySort(items, sortSpec)
	} else {
		var acyclic bool
		ordered, acyclic = wn.TopoOrder(items)
		if !acyclic && len(ordered) > 0 {
			ordered = items
		}
	}
	// Apply offset and limit
	if exportOffset > 0 || exportLimit > 0 {
		if exportOffset > len(ordered) {
			ordered = nil
		} else {
			ordered = ordered[exportOffset:]
			if exportLimit > 0 && len(ordered) > exportLimit {
				ordered = ordered[:exportLimit]
			}
		}
	}
	return wn.ExportItems(ordered, exportOutput)
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
