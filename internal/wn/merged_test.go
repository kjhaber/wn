package wn

import (
	"strings"
	"testing"
	"time"
)

func TestMarkMergedItems_wnColonBranchNote(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	// Create a feature branch with a commit and merge it into main.
	execIn(t, dir, "git", "branch", "wn-abc123-fix-thing")
	execIn(t, dir, "git", "checkout", "wn-abc123-fix-thing")
	writeFile(t, dir+"/fix.txt", "fix")
	execIn(t, dir, "git", "add", "fix.txt")
	execIn(t, dir, "git", "commit", "-m", "fix thing")

	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-abc123-fix-thing", "--no-ff", "-m", "merge fix")

	// Item uses new "wn:branch" note name.
	item := &Item{
		ID:          "abc123",
		Description: "Fix the thing",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: NoteNameBranch, Body: "wn-abc123-fix-thing", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := MarkMergedItems(store, dir, "", false)
	if err != nil {
		t.Fatalf("MarkMergedItems: %v", err)
	}

	var marked []MarkMergedResult
	for _, r := range results {
		if r.Status == "marked" {
			marked = append(marked, r)
		}
	}
	if len(marked) != 1 {
		t.Errorf("want 1 marked, got %d; all results: %v", len(marked), results)
	}
	if len(marked) == 1 && marked[0].ID != "abc123" {
		t.Errorf("marked ID = %q, want abc123", marked[0].ID)
	}

	// Verify the item is actually done in the store.
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !got.Done {
		t.Error("item should be marked done")
	}
}

func TestMarkMergedItems_squashMergeWithCommitNote(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	// Create a feature branch with a commit.
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "branch", "wn-ghi789-squash-me")
	execIn(t, dir, "git", "checkout", "wn-ghi789-squash-me")
	writeFile(t, dir+"/squash.txt", "squash work")
	execIn(t, dir, "git", "add", "squash.txt")
	execIn(t, dir, "git", "commit", "-m", "squash commit")

	execIn(t, dir, "git", "checkout", def)
	// Squash merge: branch tip is NOT an ancestor of the resulting commit.
	execIn(t, dir, "git", "merge", "--squash", "wn-ghi789-squash-me")
	execIn(t, dir, "git", "commit", "-m", "squash merge wn-ghi789")
	squashCommit := strings.TrimSpace(execOut(t, dir, "git", "rev-parse", "HEAD"))

	// Item has branch note and squash commit note.
	item := &Item{
		ID:          "ghi789",
		Description: "Squash me",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{
			{Name: NoteNameBranch, Body: "wn-ghi789-squash-me", Created: now},
			{Name: "commit", Body: squashCommit, Created: now},
		},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := MarkMergedItems(store, dir, "", false)
	if err != nil {
		t.Fatalf("MarkMergedItems: %v", err)
	}

	var marked []MarkMergedResult
	for _, r := range results {
		if r.Status == "marked" {
			marked = append(marked, r)
		}
	}
	if len(marked) != 1 {
		t.Errorf("want 1 marked for squash merge, got %d; all results: %v", len(marked), results)
	}
}

func TestMarkMergedItems_legacyBranchNoteStillWorks(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	execIn(t, dir, "git", "branch", "wn-def456-old-style")
	execIn(t, dir, "git", "checkout", "wn-def456-old-style")
	writeFile(t, dir+"/old.txt", "old")
	execIn(t, dir, "git", "add", "old.txt")
	execIn(t, dir, "git", "commit", "-m", "old style")

	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-def456-old-style", "--no-ff", "-m", "merge old")

	// Item uses old "branch" note name.
	item := &Item{
		ID:          "def456",
		Description: "Old style item",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: "branch", Body: "wn-def456-old-style", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := MarkMergedItems(store, dir, "", false)
	if err != nil {
		t.Fatalf("MarkMergedItems: %v", err)
	}

	var marked []MarkMergedResult
	for _, r := range results {
		if r.Status == "marked" {
			marked = append(marked, r)
		}
	}
	if len(marked) != 1 {
		t.Errorf("want 1 marked, got %d; all results: %v", len(marked), results)
	}
}
