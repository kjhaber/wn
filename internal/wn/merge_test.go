package wn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunMerge_noCurrentNoWID(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".wn"), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// No meta current, no WorkID
	err = RunMerge(store, MergeOpts{Root: dir, MainBranch: "main", ValidateCmd: "true", Audit: os.Stderr})
	if err == nil {
		t.Fatal("RunMerge want error when no current and no WorkID")
	}
	if !strings.Contains(err.Error(), "no current task") {
		t.Errorf("RunMerge error = %v, want message containing 'no current task'", err)
	}
}

func TestRunMerge_noBranchNote(t *testing.T) {
	dir := t.TempDir()
	setupMergeMeta(t, dir, "abc123")
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID: "abc123", Description: "task", Created: now, Updated: now,
		ReviewReady: true, Log: []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{}, // no branch note
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = RunMerge(store, MergeOpts{Root: dir, WorkID: "abc123", MainBranch: "main", ValidateCmd: "true", Audit: os.Stderr})
	if err == nil {
		t.Fatal("RunMerge want error when item has no branch note")
	}
	if !strings.Contains(err.Error(), "no branch note") {
		t.Errorf("RunMerge error = %v, want message containing 'no branch note'", err)
	}
}

func TestRunMerge_notReviewReady(t *testing.T) {
	dir := t.TempDir()
	setupMergeMeta(t, dir, "def456")
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID: "def456", Description: "task", Created: now, Updated: now,
		ReviewReady: false, Log: []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-def456-feature"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = RunMerge(store, MergeOpts{Root: dir, WorkID: "def456", MainBranch: "main", ValidateCmd: "true", Audit: os.Stderr})
	if err == nil {
		t.Fatal("RunMerge want error when item is not review-ready")
	}
	if !strings.Contains(err.Error(), "review-ready") {
		t.Errorf("RunMerge error = %v, want message containing 'review-ready'", err)
	}
}

func TestRunMerge_itemNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".wn"), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = RunMerge(store, MergeOpts{Root: dir, WorkID: "nonexistent", MainBranch: "main", ValidateCmd: "true", Audit: os.Stderr})
	if err == nil {
		t.Fatal("RunMerge want error when WorkID not found")
	}
	if !strings.Contains(err.Error(), "work item") {
		t.Errorf("RunMerge error = %v, want message containing 'work item'", err)
	}
}

func TestRunMerge_emptyBranchNote(t *testing.T) {
	dir := t.TempDir()
	setupMergeMeta(t, dir, "ghi789")
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID: "ghi789", Description: "task", Created: now, Updated: now,
		ReviewReady: true, Log: []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "   "}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = RunMerge(store, MergeOpts{Root: dir, WorkID: "ghi789", MainBranch: "main", ValidateCmd: "true", Audit: os.Stderr})
	if err == nil {
		t.Fatal("RunMerge want error when branch note is empty")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("RunMerge error = %v, want message containing 'empty'", err)
	}
}

// setupMergeMeta creates .wn and meta with current task set.
func setupMergeMeta(t *testing.T, root, currentID string) {
	t.Helper()
	wnDir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := Meta{CurrentID: currentID}
	if err := WriteMeta(root, meta); err != nil {
		t.Fatal(err)
	}
}
