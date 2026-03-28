package main

import (
	"fmt"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

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
