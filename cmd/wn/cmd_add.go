package main

import (
	"fmt"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

type addFlags struct {
	message string
	tags    []string
}

func newAddCmd() *cobra.Command {
	flags := &addFlags{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a work item",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Description of the work item")
	cmd.Flags().StringSliceVarP(&flags.tags, "tag", "t", nil, "Tag (repeatable)")
	return cmd
}

func runAdd(cmd *cobra.Command, args []string, flags *addFlags) error {
	msg := flags.message
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
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	id, err := wn.GenerateID(cc.Store)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          id,
		Description: msg,
		Created:     now,
		Updated:     now,
		Tags:        flags.tags,
		DependsOn:   nil,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := cc.Store.Put(item); err != nil {
		return err
	}
	if err := wn.WithMetaLock(cc.Root, func(m wn.Meta) (wn.Meta, error) {
		m.CurrentID = id
		return m, nil
	}); err != nil {
		return err
	}
	fmt.Printf("added entry %s\n", id)
	return nil
}
