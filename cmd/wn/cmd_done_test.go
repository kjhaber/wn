package main

import (
	"os"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestDoneCmd_autoMarksPromptDepsAsDone(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID: "pmt-done01", Description: "Need answer",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	_ = store.Put(promptItem)
	_ = store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, "pmt-done01")
		return it, nil
	})

	rootCmd.SetArgs([]string{"done", parentID})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("Execute done: %v", err)
	}

	parent, _ := store.Get(parentID)
	if !parent.Done {
		t.Error("parent should be done")
	}
	prompt, _ := store.Get("pmt-done01")
	if !prompt.Done {
		t.Error("prompt dep should be auto-marked done")
	}
}

func TestDoneCmd_promptDepDoesNotBlockDone(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID: "pmt-done02", Description: "Need answer",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	_ = store.Put(promptItem)
	_ = store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, "pmt-done02")
		return it, nil
	})

	// Should succeed without --force even though prompt dep is undone
	rootCmd.SetArgs([]string{"done", parentID})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("done should not be blocked by prompt dep: %v", err)
	}
}
