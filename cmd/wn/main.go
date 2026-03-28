package main

import (
	"fmt"
	"os"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

var version = "dev"

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
	rootCmd.AddCommand(initCmd, addCmd, rmCmd, archiveCmd, editCmd, tagCmd, dependCmd, doneCmd, undoneCmd, statusCmd, claimCmd, releaseCmd, reviewReadyCmd, cleanupCmd, logCmd, showCmd, nextCmd, pickCmd, mcpCmd, doCmd, launchCmd, worktreeSetupCmd, settingsCmd, verifyCmd, exportCmd, importCmd, listCmd, noteCmd, tuiCmd, promptCmd, respondCmd, summaryCmd, mergeCmd, repoRootCmd)
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
