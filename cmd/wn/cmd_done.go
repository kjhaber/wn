package main

import (
	"fmt"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type doneFlags struct {
	message string
	force   bool
	next    bool
}

func newDoneCmd() *cobra.Command {
	flags := &doneFlags{}
	cmd := &cobra.Command{
		Use:   "done [id]",
		Short: "Mark a work item complete",
		Long:  "If id is omitted, marks the current task complete. Use --next to then set the next undone item as current (convenience for done + next).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDone(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Completion message (e.g. git commit)")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Mark complete even if dependencies are not done")
	cmd.Flags().BoolVar(&flags.next, "next", false, "After marking done, set the next undone item as current (like running wn next)")
	return cmd
}

func runDone(cmd *cobra.Command, args []string, flags *doneFlags) error {
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
	if !flags.force {
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
		it.DoneMessage = flags.message
		it.DoneStatus = wn.DoneStatusDone
		it.ReviewReady = false
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: flags.message})
		return it, nil
	}); err != nil {
		return err
	}
	if !flags.next {
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

func newUndoneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undone [id]",
		Short: "Mark a work item not complete",
		Long:  "If id is omitted, marks the current task undone.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runUndone,
	}
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
