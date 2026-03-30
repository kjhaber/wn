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

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize wn in the current directory",
		RunE:  runInit,
	}
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

func newRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm [id ...]",
		Short: "Remove a work item",
		Long:  "If no id is given, removes the current task. Pass one or more ids to remove those directly.",
		Args:  cobra.ArbitraryArgs,
		RunE:  runRm,
	}
}

func runRm(cmd *cobra.Command, args []string) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}

	var idsToRemove []string
	if len(args) == 0 {
		meta, err := wn.ReadMeta(cc.Root)
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

	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	clearCurrent := false
	for _, id := range idsToRemove {
		if _, err := cc.Store.Get(id); err != nil {
			return fmt.Errorf("item %s not found", id)
		}
		if id == meta.CurrentID {
			clearCurrent = true
		}
		if err := cc.Store.Delete(id); err != nil {
			return err
		}
		fmt.Printf("removed entry %s\n", id)
	}
	if clearCurrent {
		return wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
			m.CurrentID = ""
			return m, nil
		})
	}
	return nil
}

type archiveFlags struct {
	location string
}

func newArchiveCmd() *cobra.Command {
	flags := &archiveFlags{}
	cmd := &cobra.Command{
		Use:   "archive [id]",
		Short: "Archive a work item",
		Long: `Archive a work item: saves its content to an archive file then removes it from the project.

The archived item can be recovered with 'wn import'.

By default, archives are saved under .wn/archive/<id>.json. Use --location to override.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runArchive(cmd, args, flags)
		},
	}
	cmd.Flags().StringVar(&flags.location, "location", "", "Directory to write archive file (default: .wn/archive)")
	return cmd
}

func runArchive(cmd *cobra.Command, args []string, flags *archiveFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}

	var id string
	if len(args) == 0 {
		meta, err := wn.ReadMeta(cc.Root)
		if err != nil {
			return err
		}
		items, err := cc.Store.List()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No tasks.")
			return nil
		}
		items = wn.ApplySort(items, interactiveSortSpec(cc.Root))
		ids, err := wn.PickMultiInteractive(items)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		clearCurrent := false
		for _, aid := range ids {
			archivePath, err := wn.ArchiveItem(cc.Store, aid, flags.location)
			if err != nil {
				return err
			}
			fmt.Printf("archived %s -> %s\n", aid, archivePath)
			if aid == meta.CurrentID {
				clearCurrent = true
			}
		}
		if clearCurrent {
			return wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
				m.CurrentID = ""
				return m, nil
			})
		}
		return nil
	}

	id = args[0]
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	archivePath, err := wn.ArchiveItem(cc.Store, id, flags.location)
	if err != nil {
		return err
	}
	fmt.Printf("archived %s -> %s\n", id, archivePath)
	if id == meta.CurrentID {
		return wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
			m.CurrentID = ""
			return m, nil
		})
	}
	return nil
}

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [id]",
		Short: "Edit a work item description in $EDITOR",
		Long:  "If id is omitted, edits the current task.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runEdit,
	}
}

func runEdit(cmd *cobra.Command, args []string) error {
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
	return cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
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

type settingsEditFlags struct {
	user         bool
	userLocal    bool
	project      bool
	projectLocal bool
}

func newSettingsCmd() *cobra.Command {
	settingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage wn settings",
		Long:  "Subcommands: show (print effective merged settings as JSON), edit (open a settings file in $EDITOR).",
	}

	settingsShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Print effective merged settings as JSON",
		Long:  "Prints the fully merged effective settings (user + user-local + project + project-local) as JSON.",
		RunE:  runSettingsShow,
	}

	editF := &settingsEditFlags{}
	settingsEditCmd := &cobra.Command{
		Use:   "edit",
		Short: "Open a settings file in $EDITOR",
		Long: `Interactively pick a settings file to open in $EDITOR. Use a flag to skip the picker:
  --user          edit user settings (WN_SETTINGS_USER or default)
  --user-local    edit user-local settings (WN_SETTINGS_USER_LOCAL)
  --project       edit project settings (.wn/settings.json)
  --project-local edit project-local settings (.wn/settings.local.json)

Missing files are created as {} before opening.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettingsEdit(cmd, args, editF)
		},
	}
	settingsEditCmd.Flags().BoolVar(&editF.user, "user", false, "Edit user settings")
	settingsEditCmd.Flags().BoolVar(&editF.userLocal, "user-local", false, "Edit user-local settings (requires WN_SETTINGS_USER_LOCAL)")
	settingsEditCmd.Flags().BoolVar(&editF.project, "project", false, "Edit project settings (.wn/settings.json)")
	settingsEditCmd.Flags().BoolVar(&editF.projectLocal, "project-local", false, "Edit project-local settings (.wn/settings.local.json)")

	settingsCmd.AddCommand(settingsShowCmd, settingsEditCmd)
	return settingsCmd
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

func runSettingsEdit(_ *cobra.Command, _ []string, f *settingsEditFlags) error {
	root, _ := wn.FindRootForCLI()
	candidates, err := settingsEditCandidates(root)
	if err != nil {
		return err
	}

	var targetLabel string
	switch {
	case f.user:
		targetLabel = "user"
	case f.userLocal:
		targetLabel = "user-local"
	case f.project:
		targetLabel = "project"
	case f.projectLocal:
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

type verifyFlags struct {
	atRoot bool
}

func newVerifyCmd() *cobra.Command {
	flags := &verifyFlags{}
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run the configured verify command (e.g. make all, npm test)",
		Long:  "Runs the shell command configured in settings.verify. Set it in project settings (.wn/settings.json) or user settings (~/.config/wn/settings.json). Useful for agents and humans alike to confirm the build passes.",
		RunE: func(c *cobra.Command, args []string) error {
			return runVerify(c, args, flags)
		},
		SilenceUsage: true,
	}
	cmd.Flags().BoolVar(&flags.atRoot, "root", false, "Run verify in the main git worktree root instead of cwd; useful when invoked from a linked git worktree or subdirectory")
	return cmd
}

func runVerify(cobraCmd *cobra.Command, _ []string, f *verifyFlags) error {
	cc, _ := newCmdCtx("")
	var settings wn.Settings
	if cc != nil {
		settings = cc.Settings
	}
	if f.atRoot {
		gitRoot, err := wn.GitRepoRoot()
		if err != nil {
			return fmt.Errorf("--root: %w", err)
		}
		if err := os.Chdir(gitRoot); err != nil {
			return err
		}
		if wnRoot, err := wn.FindRootFromDir(gitRoot); err == nil {
			settings, _ = wn.ReadSettingsInRoot(wnRoot)
		}
	}
	if settings.Verify == "" {
		return fmt.Errorf("no verify command configured; set 'verify' in .wn/settings.json or ~/.config/wn/settings.json")
	}
	effectiveDir, _ := os.Getwd()
	_, _ = fmt.Fprintf(cobraCmd.ErrOrStderr(), "running: %s\nin: %s\n", settings.Verify, effectiveDir)
	cmd := exec.Command("sh", "-c", settings.Verify)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = cobraCmd.ErrOrStderr()
	return cmd.Run()
}

func newRepoRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "root",
		Short:        "Print the main git repository root (worktree-aware)",
		Long:         "Prints the absolute path to the main git repository root. In a linked worktree, this resolves to the main repo root rather than the worktree directory. Useful in scripts and hooks that need to operate on the main repo from a feature worktree.",
		Args:         cobra.NoArgs,
		RunE:         runRepoRoot,
		SilenceUsage: true,
	}
}

func runRepoRoot(_ *cobra.Command, _ []string) error {
	root, err := wn.GitRepoRoot()
	if err != nil {
		return err
	}
	fmt.Println(root)
	return nil
}

type exportFlags struct {
	output      string
	all         bool
	undone      bool
	done        bool
	reviewReady bool
	tag         string
	sort        string
	limit       int
	offset      int
}

func newExportCmd() *cobra.Command {
	flags := &exportFlags{}
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export work items to JSON (optionally filtered by criteria)",
		RunE: func(c *cobra.Command, args []string) error {
			return runExport(c, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Write to file (default: stdout)")
	cmd.Flags().BoolVar(&flags.all, "all", false, "Export all items (default when no status filter)")
	cmd.Flags().BoolVar(&flags.undone, "undone", false, "Export only undone items")
	cmd.Flags().BoolVar(&flags.done, "done", false, "Export only done items")
	cmd.Flags().BoolVar(&flags.reviewReady, "review-ready", false, "Export only review-ready items")
	cmd.Flags().BoolVar(&flags.reviewReady, "rr", false, "Export only review-ready items")
	cmd.Flags().StringVar(&flags.tag, "tag", "", `Filter by tag; use "a,b" for AND (must have both), "a|b" for OR (has either)`)
	cmd.Flags().StringVar(&flags.sort, "sort", "", "Sort order (e.g. updated:desc,priority,tags). Overrides settings. Keys: created, updated, priority, alpha, tags")
	cmd.Flags().IntVar(&flags.limit, "limit", 0, "Return at most N items (0 = no limit)")
	cmd.Flags().IntVar(&flags.offset, "offset", 0, "Skip first N items")
	return cmd
}

func runExport(cmd *cobra.Command, args []string, flags *exportFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	stateFlags := 0
	if flags.all {
		stateFlags++
	}
	if flags.undone {
		stateFlags++
	}
	if flags.done {
		stateFlags++
	}
	if flags.reviewReady {
		stateFlags++
	}
	if stateFlags > 1 {
		return fmt.Errorf("only one of --undone, --done, --all, --review-ready may be set")
	}
	useCriteria := stateFlags > 0 || flags.tag != "" || flags.sort != "" || flags.limit > 0 || flags.offset > 0
	if !useCriteria {
		return wn.Export(cc.Store, flags.output)
	}
	var items []*wn.Item
	if flags.undone {
		items, err = wn.ListableUndoneItems(cc.Store)
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
		items, err = cc.Store.List()
		if err != nil {
			return err
		}
	}
	items = wn.FilterByTag(items, flags.tag)
	var sortSpec []wn.SortOption
	if flags.sort != "" {
		sortSpec, err = wn.ParseSortSpec(flags.sort)
		if err != nil {
			return err
		}
	} else {
		sortSpec = wn.SortSpecFromSettings(cc.Settings)
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
	return wn.ExportItems(ordered, flags.output)
}

type importFlags struct {
	replace bool
	append  bool
}

func newImportCmd() *cobra.Command {
	flags := &importFlags{}
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import work items from an export file",
		Long:  "Import work items from a JSON export file. When the store already has items, you must choose --append (add/merge from file) or --replace (delete all existing, then load file). When the store is empty, either flag is optional.",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runImport(c, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.append, "append", false, "Add items from file to the store (merge by ID; same ID overwrites)")
	cmd.Flags().BoolVar(&flags.replace, "replace", false, "Replace all existing items with the contents of the file")
	return cmd
}

func runImport(cmd *cobra.Command, args []string, flags *importFlags) error {
	if flags.append && flags.replace {
		return fmt.Errorf("cannot use both --append and --replace; choose one")
	}
	path := args[0]
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	hasItems, err := wn.StoreHasItems(cc.Store)
	if err != nil {
		return err
	}
	if hasItems && !flags.append && !flags.replace {
		return fmt.Errorf("store already has items; use --append to add to existing items or --replace to replace all")
	}
	if flags.replace {
		return wn.ImportReplace(cc.Store, path)
	}
	return wn.ImportAppend(cc.Store, path)
}
