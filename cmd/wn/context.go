package main

import (
	"github.com/kjhaber/wn/internal/wn"
)

// cmdCtx holds the resolved root, store, and settings for a command handler.
// Use newCmdCtx to construct it; pass a non-empty rootFlag to pin the root
// explicitly (e.g. for future --root flag support), or pass "" to auto-detect
// via FindRootForCLI.
type cmdCtx struct {
	Root     string
	Settings wn.Settings
	Store    wn.Store
}

// newCmdCtx resolves the project root, opens the file store, and reads
// the effective settings. When rootFlag is non-empty it is used as the root
// directory; otherwise the root is discovered from the current working
// directory (or WN_ROOT env var) via FindRootForCLI.
func newCmdCtx(rootFlag string) (*cmdCtx, error) {
	var root string
	var err error
	if rootFlag != "" {
		root, err = wn.FindRootFromDir(rootFlag)
	} else {
		root, err = wn.FindRootForCLI()
	}
	if err != nil {
		return nil, err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return nil, err
	}
	settings, _ := wn.ReadSettingsInRoot(root)
	return &cmdCtx{Root: root, Settings: settings, Store: store}, nil
}
