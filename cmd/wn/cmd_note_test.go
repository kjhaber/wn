package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestNoteAddAndList(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Add a note with a name
	rootCmd.SetArgs([]string{"note", "add", "pr-url", itemID, "-m", "I wrote this in file X"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	// Verify add persisted
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	item, err := store.Get(itemID)
	if err != nil {
		t.Fatalf("Get after add: %v", err)
	}
	if len(item.Notes) != 1 || item.Notes[0].Name != "pr-url" || item.Notes[0].Body != "I wrote this in file X" {
		t.Fatalf("after note add: item.Notes = %v, want one note with name pr-url and body", item.Notes)
	}

	// List notes: should show name and body
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "list", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note list: %v", err)
		}
	})
	if !strings.Contains(out, "I wrote this in file X") {
		t.Errorf("note list should contain note body; got %q", out)
	}
	if !strings.Contains(out, "pr-url") {
		t.Errorf("note list should show note name pr-url; got %q", out)
	}
}

func TestNoteListOrderedByCreateTime(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "first", itemID, "-m", "first note"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add 1: %v", err)
	}
	rootCmd.SetArgs([]string{"note", "add", "second", itemID, "-m", "second note"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add 2: %v", err)
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "list", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note list: %v", err)
		}
	})
	idx1 := strings.Index(out, "first note")
	idx2 := strings.Index(out, "second note")
	if idx1 < 0 || idx2 < 0 {
		t.Fatalf("note list should show both notes; got %q", out)
	}
	// First note (older) should appear before second in output
	if idx1 > idx2 {
		t.Errorf("notes should be ordered by create time (first before second); got %q", out)
	}
}

func TestNoteEdit(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "pr-url", itemID, "-m", "original"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	rootCmd.SetArgs([]string{"note", "edit", itemID, "pr-url", "-m", "edited body"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note edit: %v", err)
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "list", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note list: %v", err)
		}
	})
	if !strings.Contains(out, "edited body") {
		t.Errorf("note list after edit should show edited body; got %q", out)
	}
	if strings.Contains(out, "original") {
		t.Errorf("note list after edit should not show original; got %q", out)
	}
}

func TestNoteRm(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "to-remove", itemID, "-m", "to be removed"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	rootCmd.SetArgs([]string{"note", "rm", itemID, "to-remove"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note rm: %v", err)
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "list", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note list: %v", err)
		}
	})
	if strings.Contains(out, "to be removed") {
		t.Errorf("note list after rm should not show removed note; got %q", out)
	}
}

func TestNoteShow(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "branch", itemID, "-m", "my-feature-branch"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	// show with explicit id
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "show", itemID, "branch"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note show: %v", err)
		}
	})
	if strings.TrimSpace(out) != "my-feature-branch" {
		t.Errorf("note show output = %q, want %q", strings.TrimSpace(out), "my-feature-branch")
	}
}

func TestNoteShow_CurrentItem(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// claim so there's a current item
	rootCmd.SetArgs([]string{"claim", itemID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("claim: %v", err)
	}

	rootCmd.SetArgs([]string{"note", "add", "branch", "-m", "current-branch"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	// show with just note name (uses current item)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "show", "branch"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note show (current): %v", err)
		}
	})
	if strings.TrimSpace(out) != "current-branch" {
		t.Errorf("note show current item output = %q, want %q", strings.TrimSpace(out), "current-branch")
	}
}

func TestNoteShow_NotFound(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "show", itemID, "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("note show with nonexistent note should fail")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention note name; got %v", err)
	}
}

func TestNoteAddInvalidName(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "bad name", itemID, "-m", "body"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("note add with invalid name (space) should fail")
	}
	if !strings.Contains(err.Error(), "name") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention name/invalid; got %v", err)
	}
}

func TestNoteAddUpsert(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "issue-number", itemID, "-m", "first"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add 1: %v", err)
	}
	rootCmd.SetArgs([]string{"note", "add", "issue-number", itemID, "-m", "second"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add 2 (same name): %v", err)
	}

	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	item, err := store.Get(itemID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(item.Notes) != 1 {
		t.Fatalf("upsert should keep one note; got %d notes", len(item.Notes))
	}
	if item.Notes[0].Body != "second" {
		t.Errorf("upsert should update body; got %q", item.Notes[0].Body)
	}
}

func TestNoteListEmpty(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "list", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note list: %v", err)
		}
	})
	// Empty list: no notes line or just empty
	if len(strings.TrimSpace(out)) != 0 && !strings.Contains(out, "no note") && !strings.Contains(out, "0 note") {
		// Accept empty output or a message like "no notes"
		t.Logf("note list (empty) output: %q", out)
	}
}

func resetNoteSearchFlags() {
	noteSearchFirst = false
	noteSearchLatest = false
	noteSearchIDOnly = false
}

func TestNoteSearch_ByName(t *testing.T) {
	dir, item1ID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Create a second item
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item2 := &wn.Item{
		ID:          "def456",
		Description: "second item",
		Created:     now,
		Updated:     now,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}

	// Add note "pr-url" to item1 only
	rootCmd.SetArgs([]string{"note", "add", "pr-url", item1ID, "-m", "https://example.com/1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	// Search by name: should find item1
	resetNoteSearchFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "search", "pr-url"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note search: %v", err)
		}
	})
	if !strings.Contains(out, item1ID) {
		t.Errorf("note search pr-url should include item1 %q; got %q", item1ID, out)
	}
	if strings.Contains(out, "def456") {
		t.Errorf("note search pr-url should not include item2 def456; got %q", out)
	}
}

func TestNoteSearch_ByNameAndValue(t *testing.T) {
	dir, item1ID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Create a second item
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item2 := &wn.Item{
		ID:          "def456",
		Description: "second item",
		Created:     now,
		Updated:     now,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}

	// Add same note name with different values to both items
	rootCmd.SetArgs([]string{"note", "add", "branch", item1ID, "-m", "feature-a"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add item1: %v", err)
	}
	rootCmd.SetArgs([]string{"note", "add", "branch", "def456", "-m", "feature-b"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add item2: %v", err)
	}

	// Search by name+value: should find only item1
	resetNoteSearchFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "search", "branch", "feature-a"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note search: %v", err)
		}
	})
	if !strings.Contains(out, item1ID) {
		t.Errorf("note search branch feature-a should include item1 %q; got %q", item1ID, out)
	}
	if strings.Contains(out, "def456") {
		t.Errorf("note search branch feature-a should not include item2; got %q", out)
	}
}

func TestNoteSearch_NoMatches(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	resetNoteSearchFlags()
	err := func() error {
		rootCmd.SetArgs([]string{"note", "search", "nonexistent-note"})
		return rootCmd.Execute()
	}()
	if err == nil {
		t.Fatal("note search with no matches should return error")
	}
}

func TestNoteSearch_FirstFlag(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Create two items with same note name
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC()
	item1 := &wn.Item{
		ID:          "older1",
		Description: "older item",
		Created:     t1,
		Updated:     t1,
		Log:         []wn.LogEntry{{At: t1, Kind: "created"}},
		Notes:       []wn.Note{{Name: "shared-note", Created: t1, Body: "val"}},
	}
	item2 := &wn.Item{
		ID:          "newer2",
		Description: "newer item",
		Created:     t2,
		Updated:     t2,
		Log:         []wn.LogEntry{{At: t2, Kind: "created"}},
		Notes:       []wn.Note{{Name: "shared-note", Created: t2, Body: "val"}},
	}
	if err := store.Put(item1); err != nil {
		t.Fatalf("Put item1: %v", err)
	}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}

	// --first should return only the oldest item
	resetNoteSearchFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "search", "shared-note", "--first"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note search --first: %v", err)
		}
	})
	if !strings.Contains(out, "older1") {
		t.Errorf("--first should include older item; got %q", out)
	}
	if strings.Contains(out, "newer2") {
		t.Errorf("--first should not include newer item; got %q", out)
	}
}

func TestNoteSearch_LatestFlag(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC()
	item1 := &wn.Item{
		ID:          "older1",
		Description: "older item",
		Created:     t1,
		Updated:     t1,
		Log:         []wn.LogEntry{{At: t1, Kind: "created"}},
		Notes:       []wn.Note{{Name: "shared-note", Created: t1, Body: "val"}},
	}
	item2 := &wn.Item{
		ID:          "newer2",
		Description: "newer item",
		Created:     t2,
		Updated:     t2,
		Log:         []wn.LogEntry{{At: t2, Kind: "created"}},
		Notes:       []wn.Note{{Name: "shared-note", Created: t2, Body: "val"}},
	}
	if err := store.Put(item1); err != nil {
		t.Fatalf("Put item1: %v", err)
	}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}

	// --latest should return only the most recently updated item
	resetNoteSearchFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "search", "shared-note", "--latest"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note search --latest: %v", err)
		}
	})
	if !strings.Contains(out, "newer2") {
		t.Errorf("--latest should include newer item; got %q", out)
	}
	if strings.Contains(out, "older1") {
		t.Errorf("--latest should not include older item; got %q", out)
	}
}

func TestNoteSearch_FirstAndLatestMutuallyExclusive(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	resetNoteSearchFlags()
	err := func() error {
		rootCmd.SetArgs([]string{"note", "search", "some-note", "--first", "--latest"})
		return rootCmd.Execute()
	}()
	if err == nil {
		t.Fatal("note search with both --first and --latest should return error")
	}
}

func TestNoteSearch_IDOnly(t *testing.T) {
	dir, item1ID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "pr-url", item1ID, "-m", "https://example.com/pr/1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	// --id-only should print just the id, no description
	resetNoteSearchFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "search", "pr-url", "--id-only"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note search --id-only: %v", err)
		}
	})
	if strings.TrimSpace(out) != item1ID {
		t.Errorf("--id-only should print only the item id %q; got %q", item1ID, out)
	}
}

func TestNoteSearch_IDOnly_MultipleMatches(t *testing.T) {
	dir, item1ID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item2 := &wn.Item{
		ID:          "def456",
		Description: "second item",
		Created:     now,
		Updated:     now,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}

	rootCmd.SetArgs([]string{"note", "add", "shared", item1ID, "-m", "val"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add item1: %v", err)
	}
	rootCmd.SetArgs([]string{"note", "add", "shared", "def456", "-m", "val"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add item2: %v", err)
	}

	// --id-only with multiple matches should print one id per line
	resetNoteSearchFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"note", "search", "shared", "--id-only"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("note search --id-only: %v", err)
		}
	})
	lines := strings.Fields(out)
	if len(lines) != 2 {
		t.Errorf("--id-only with two matches should print 2 lines; got %q", out)
	}
	ids := map[string]bool{item1ID: true, "def456": true}
	for _, l := range lines {
		if !ids[l] {
			t.Errorf("unexpected id in output: %q", l)
		}
	}
}

func TestNoteAdd_WnBranchUnknownName(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "wn:unknown", itemID, "-m", "value"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("note add with unknown wn: name should fail")
	}
	if !strings.Contains(err.Error(), "wn:unknown") {
		t.Errorf("error should mention the unknown name; got %v", err)
	}
}

func TestNoteAdd_WnBranchExplicit(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "wn:branch", itemID, "-m", "my-feature-branch"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add wn:branch: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	item, err := store.Get(itemID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	idx := item.NoteIndexByName(wn.NoteNameBranch)
	if idx < 0 {
		t.Fatalf("wn:branch note not found on item")
	}
	if item.Notes[idx].Body != "my-feature-branch" {
		t.Errorf("wn:branch note body = %q, want my-feature-branch", item.Notes[idx].Body)
	}
}

func TestNoteAdd_WnBranchAutoDetect(t *testing.T) {
	dir, itemID := setupWnRootWithGit(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Detect what git considers the current branch
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	wantBranch := strings.TrimSpace(string(out))

	noteAddMessage = "" // reset in case a prior test set it via -m
	rootCmd.SetArgs([]string{"note", "add", "wn:branch", itemID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add wn:branch (auto-detect): %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	item, err := store.Get(itemID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	idx := item.NoteIndexByName(wn.NoteNameBranch)
	if idx < 0 {
		t.Fatalf("wn:branch note not found on item after auto-detect")
	}
	if item.Notes[idx].Body != wantBranch {
		t.Errorf("wn:branch auto-detect = %q, want %q", item.Notes[idx].Body, wantBranch)
	}
}
