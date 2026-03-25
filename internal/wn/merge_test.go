package wn

import (
	"strings"
	"testing"
	"time"
)

func TestSquashMerge_success(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	// Create a feature branch with a commit.
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "branch", "wn-abc123-feat")
	execIn(t, dir, "git", "checkout", "wn-abc123-feat")
	writeFile(t, dir+"/feat.txt", "feature work")
	execIn(t, dir, "git", "add", "feat.txt")
	execIn(t, dir, "git", "commit", "-m", "feature commit")
	execIn(t, dir, "git", "checkout", def)

	item := &Item{
		ID:          "abc123",
		Description: "Implement the feature",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: NoteNameBranch, Body: "wn-abc123-feat", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result, err := SquashMerge(store, dir, "wn-abc123-feat", "Squash: implement the feature", false)
	if err != nil {
		t.Fatalf("SquashMerge: %v", err)
	}

	if result.ItemID != "abc123" {
		t.Errorf("ItemID = %q, want abc123", result.ItemID)
	}
	if result.Branch != "wn-abc123-feat" {
		t.Errorf("Branch = %q, want wn-abc123-feat", result.Branch)
	}
	if result.CommitHash == "" {
		t.Error("CommitHash should not be empty")
	}

	// Verify item in store is done and has wn:commit note.
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !got.Done {
		t.Error("item should be marked done")
	}
	if got.ReviewReady {
		t.Error("item ReviewReady should be cleared")
	}

	idx := got.NoteIndexByName(NoteNameCommit)
	if idx < 0 {
		t.Fatal("wn:commit note not found on item")
	}
	if got.Notes[idx].Body != result.CommitHash {
		t.Errorf("wn:commit note body = %q, want %q", got.Notes[idx].Body, result.CommitHash)
	}

	// Verify the squash commit is in the git history.
	merged, err := CommitMergedInto(dir, result.CommitHash, "HEAD")
	if err != nil {
		t.Fatalf("CommitMergedInto: %v", err)
	}
	if !merged {
		t.Error("squash commit should be in HEAD")
	}
}

func TestSquashMerge_dryRun(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "branch", "wn-dry123-feat")
	execIn(t, dir, "git", "checkout", "wn-dry123-feat")
	writeFile(t, dir+"/dry.txt", "dry run content")
	execIn(t, dir, "git", "add", "dry.txt")
	execIn(t, dir, "git", "commit", "-m", "dry run commit")
	execIn(t, dir, "git", "checkout", def)

	item := &Item{
		ID:          "dry123",
		Description: "Dry run item",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: NoteNameBranch, Body: "wn-dry123-feat", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result, err := SquashMerge(store, dir, "wn-dry123-feat", "would squash", true)
	if err != nil {
		t.Fatalf("SquashMerge dry-run: %v", err)
	}

	if result.ItemID != "dry123" {
		t.Errorf("ItemID = %q, want dry123", result.ItemID)
	}
	if result.CommitHash != "" {
		t.Errorf("CommitHash should be empty on dry-run, got %q", result.CommitHash)
	}

	// Verify item is NOT done in store (dry-run should not modify).
	got, err := store.Get("dry123")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if got.Done {
		t.Error("item should not be marked done on dry-run")
	}
	if got.NoteIndexByName(NoteNameCommit) >= 0 {
		t.Error("wn:commit note should not be added on dry-run")
	}
}

func TestSquashMerge_noItem(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	_, err = SquashMerge(store, dir, "nonexistent-branch", "some message", false)
	if err == nil {
		t.Fatal("expected error for branch with no item, got nil")
	}
	if !strings.Contains(err.Error(), "no wn item") {
		t.Errorf("expected 'no wn item' error, got: %v", err)
	}
}

func TestSquashMerge_emptyMessage(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	_, err = SquashMerge(store, dir, "some-branch", "", false)
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
	if !strings.Contains(err.Error(), "message") {
		t.Errorf("expected message-related error, got: %v", err)
	}
}

func TestSquashMerge_branchDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID:          "nogit1",
		Description: "Item with no git branch",
		Created:     now,
		Updated:     now,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: NoteNameBranch, Body: "wn-nogit1-missing", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, err = SquashMerge(store, dir, "wn-nogit1-missing", "some message", false)
	if err == nil {
		t.Fatal("expected error for non-existent git branch, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}
