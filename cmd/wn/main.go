package main

import (
	"fmt"
	"os"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

var version = "dev"

type rootFlags struct {
	picker string
}

func newRootCmd() *cobra.Command {
	rf := &rootFlags{}
	root := &cobra.Command{
		Use:   "wn",
		Short: "What's Next — local task/work item tracker",
		Long:  `wn is a CLI for tracking work items. Use wn init to create a tracker in the current directory.`,
		Args:  cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			mode := ""
			rootPath, err := wn.FindRootForCLI()
			if err == nil {
				settings, _ := wn.ReadSettingsInRoot(rootPath)
				mode = settings.Picker
			}
			if cmd.Root().PersistentFlags().Changed("picker") {
				mode = rf.picker
			}
			return wn.SetPickerMode(mode)
		},
		RunE: runCurrent,
	}
	root.Version = version
	root.SetVersionTemplate("wn version {{.Version}}\n")
	root.PersistentFlags().StringVar(&rf.picker, "picker", "", "Picker mode: fzf, numbered, or empty (auto-detect)")
	root.AddCommand(
		newInitCmd(), newAddCmd(), newRmCmd(), newArchiveCmd(), newEditCmd(), newTagCmd(), newDependCmd(),
		newDoneCmd(), newUndoneCmd(), newStatusCmd(), newClaimCmd(), newReleaseCmd(), newReviewReadyCmd(), newCleanupCmd(),
		newLogCmd(), newShowCmd(), newNextCmd(), newPickCmd(), newMCPCmd(), newDoCmd(), newLaunchCmd(), newWorktreeSetupCmd(),
		newSettingsCmd(), newVerifyCmd(), newExportCmd(), newImportCmd(), newListCmd(), newNoteCmd(), newTUICmd(),
		newPromptCmd(), newRespondCmd(), newSummaryCmd(), newMergeCmd(), newRepoRootCmd(),
	)
	root.CompletionOptions.DisableDefaultCmd = false
	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
