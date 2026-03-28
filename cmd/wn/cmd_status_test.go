package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestStatusCommand(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn status suspend [id] marks item done with done_status suspend
	rootCmd.SetArgs([]string{"status", "suspend", itemID, "-m", "deferred"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("wn status suspend: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Get(itemID)
	if err != nil {
		t.Fatal(err)
	}
	if !item.Done || item.DoneStatus != wn.DoneStatusSuspend {
		t.Errorf("after status suspend: Done=%v DoneStatus=%q", item.Done, item.DoneStatus)
	}

	// wn status undone [id] clears done/suspend
	rootCmd.SetArgs([]string{"status", "undone", itemID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("wn status undone: %v", err)
	}
	item, _ = store.Get(itemID)
	if item.Done || item.DoneStatus != "" {
		t.Errorf("after status undone: Done=%v DoneStatus=%q", item.Done, item.DoneStatus)
	}

	// invalid status returns error
	rootCmd.SetArgs([]string{"status", "invalid", itemID})
	if err := rootCmd.Execute(); err == nil {
		t.Error("wn status invalid should fail")
	}
}

// TestNextWithTag verifies that "wn next --tag X" sets current to the next undone item that has tag X (dependency order).

func TestStatus_closed_duplicate_of(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, it := range []*wn.Item{
		{ID: "abc123", Description: "original", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "def456", Description: "duplicate", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "abc123"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"status", "closed", "def456", "--duplicate-of", "abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("wn status closed --duplicate-of: %v", err)
		}
	})
	if !strings.Contains(out, "marked def456 as duplicate of abc123") {
		t.Errorf("wn status closed --duplicate-of should print confirmation; got %q", out)
	}
	item, err := store.Get("def456")
	if err != nil {
		t.Fatalf("Get def456: %v", err)
	}
	if !item.Done || item.DoneStatus != wn.DoneStatusClosed {
		t.Errorf("item should be closed after status closed --duplicate-of: Done=%v DoneStatus=%q", item.Done, item.DoneStatus)
	}
	idx := item.NoteIndexByName(wn.NoteNameDuplicateOf)
	if idx < 0 {
		t.Fatalf("note %q not found", wn.NoteNameDuplicateOf)
	}
	if item.Notes[idx].Body != "abc123" {
		t.Errorf("duplicate-of body = %q, want abc123", item.Notes[idx].Body)
	}
}

// TestStatus_duplicate_of_only_with_closed verifies that --duplicate-of is rejected when status is not closed.

func TestStatus_duplicate_of_only_with_closed(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"status", "done", "--duplicate-of", "abc123"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("wn status done --duplicate-of should error")
	}
	if err != nil && !strings.Contains(err.Error(), "only valid when setting status to closed") {
		t.Errorf("expected error about --duplicate-of only with closed; got %v", err)
	}
}

// TestClaimWithoutForUsesDefault verifies that "wn claim" without --for uses the default duration
// so agents can renew (extend) a claim without passing a duration.

func TestClaimWithoutForUsesDefault(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"claim"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("wn claim (no --for): %v", err)
	}

	showOut := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--json", itemID})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(showOut, "in_progress_until") || strings.Contains(showOut, "\"in_progress_until\":\"0001-01-01T00:00:00Z\"") {
		t.Errorf("wn claim without --for should set in_progress_until (default duration); got %s", showOut)
	}
}

func TestReviewReadySetsState(t *testing.T) {
	for _, cmdName := range []string{"review-ready", "rr"} {
		t.Run(cmdName, func(t *testing.T) {
			dir, itemID := setupWnRoot(t)
			cwd, _ := os.Getwd()
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("Chdir: %v", err)
			}
			defer func() { _ = os.Chdir(cwd) }()

			rootCmd.SetArgs([]string{cmdName, itemID})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("%s: %v", cmdName, err)
			}

			resetListFlags()
			out := captureStdout(t, func() {
				rootCmd.SetArgs([]string{"list", "--review-ready", "--json"})
				if err := rootCmd.Execute(); err != nil {
					t.Errorf("list: %v", err)
				}
			})
			list := parseListJSON(t, out)
			if len(list.Items) != 1 {
				t.Fatalf("list want 1 item, got %d", len(list.Items))
			}
			if !list.Items[0].ReviewReady || list.Items[0].Done {
				t.Errorf("after wn %s, want review_ready true and done false; got review_ready=%v done=%v", cmdName, list.Items[0].ReviewReady, list.Items[0].Done)
			}
		})
	}
}

func TestCleanupSetMergedReviewItemsDone_MarksDoneWhenBranchMerged(t *testing.T) {
	dir := t.TempDir()
	// Create git repo
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
	def, _ := wn.DefaultBranch(dir)

	// Init wn in same dir
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "abc123",
		Description: "feature task",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Notes:       []wn.Note{{Name: "branch", Created: now, Body: "wn-abc-feature"}},
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}

	// Create feature branch, commit, merge to main
	execIn(t, dir, "git", "checkout", "-b", "wn-abc-feature")
	writeFile(t, filepath.Join(dir, "feature.txt"), "feature")
	execIn(t, dir, "git", "add", "feature.txt")
	execIn(t, dir, "git", "commit", "-m", "add feature")
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-abc-feature", "-m", "merge")

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"cleanup", "set-merged-review-items-done"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("cleanup set-merged-review-items-done: %v", err)
		}
	})
	if !strings.Contains(out, "marked abc123") {
		t.Errorf("cleanup set-merged-review-items-done output should contain 'marked abc123'; got %q", out)
	}
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Done {
		t.Error("item should be done after mark-merged")
	}
	if got.ReviewReady {
		t.Error("item should not be review-ready after marked done")
	}
}

func TestCleanupSetMergedReviewItemsDone_BranchDeletedUsesCommitNote(t *testing.T) {
	dir := t.TempDir()
	// Create git repo
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
	def, _ := wn.DefaultBranch(dir)

	// Init wn in same dir
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "abc123",
		Description: "feature task",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}

	// Create feature branch, commit, capture commit hash, merge to main, then delete branch
	execIn(t, dir, "git", "checkout", "-b", "wn-abc-feature")
	writeFile(t, filepath.Join(dir, "feature.txt"), "feature")
	execIn(t, dir, "git", "add", "feature.txt")
	execIn(t, dir, "git", "commit", "-m", "add feature")

	// Capture commit hash for commit note
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	outHash, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	commitHash := strings.TrimSpace(string(outHash))

	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-abc-feature", "-m", "merge")
	execIn(t, dir, "git", "branch", "-d", "wn-abc-feature")

	// Add branch and commit notes after merge; branch ref is deleted but note remains
	if err := store.UpdateItem("abc123", func(it *wn.Item) (*wn.Item, error) {
		it.Notes = []wn.Note{
			{Name: "branch", Created: now, Body: "wn-abc-feature"},
			{Name: "commit", Created: now, Body: commitHash + " add feature"},
		}
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"cleanup", "set-merged-review-items-done"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("cleanup set-merged-review-items-done: %v", err)
		}
	})
	if !strings.Contains(out, "marked abc123") {
		t.Fatalf("cleanup set-merged-review-items-done output should contain 'marked abc123'; got %q", out)
	}
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Done {
		t.Error("item should be done after cleanup set-merged-review-items-done with deleted branch and commit note")
	}
	if got.ReviewReady {
		t.Error("item should not be review-ready after marked done")
	}
}

func execIn(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestCleanupCloseDoneItems_closesOldDoneKeepsRecent verifies that
// "wn cleanup close-done-items --age 1d" closes items that have been done
// longer than 1d while leaving more recent done items unchanged.

func TestCleanupCloseDoneItems_closesOldDoneKeepsRecent(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	oldDoneAt := now.Add(-48 * time.Hour)
	recentDoneAt := now.Add(-30 * time.Minute)

	oldItem := &wn.Item{
		ID:          "old111",
		Description: "old done",
		Created:     oldDoneAt,
		Updated:     oldDoneAt,
		Done:        true,
		DoneStatus:  wn.DoneStatusDone,
		Log: []wn.LogEntry{
			{At: oldDoneAt, Kind: "created"},
			{At: oldDoneAt, Kind: "done"},
		},
	}
	recentItem := &wn.Item{
		ID:          "new222",
		Description: "recent done",
		Created:     recentDoneAt,
		Updated:     recentDoneAt,
		Done:        true,
		DoneStatus:  wn.DoneStatusDone,
		Log: []wn.LogEntry{
			{At: recentDoneAt, Kind: "created"},
			{At: recentDoneAt, Kind: "done"},
		},
	}
	if err := store.Put(oldItem); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(recentItem); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"cleanup", "close-done-items", "--age", "1d"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("cleanup close-done-items: %v", err)
		}
	})
	if !strings.Contains(out, "old111") {
		t.Errorf("output should mention old111; got %q", out)
	}

	gotOld, err := store.Get("old111")
	if err != nil {
		t.Fatalf("Get old111: %v", err)
	}
	if !gotOld.Done || gotOld.DoneStatus != wn.DoneStatusClosed {
		t.Errorf("old111 should be closed; Done=%v DoneStatus=%q", gotOld.Done, gotOld.DoneStatus)
	}

	gotRecent, err := store.Get("new222")
	if err != nil {
		t.Fatalf("Get new222: %v", err)
	}
	if !gotRecent.Done || (gotRecent.DoneStatus != wn.DoneStatusDone && gotRecent.DoneStatus != "") {
		t.Errorf("new222 should remain done (not closed); Done=%v DoneStatus=%q", gotRecent.Done, gotRecent.DoneStatus)
	}
}

func setupGitWnRoot(t *testing.T) (dir string, itemID string) {
	t.Helper()
	dir = t.TempDir()
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "abc123",
		Description: "Add feature\nDetails here",
		Created:     now,
		Updated:     now,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return dir, "abc123"
}

func TestPromptYN_yes(t *testing.T) {
	r := strings.NewReader("y\n")
	var w bytes.Buffer
	got := promptYN(r, &w, "Remove 1 worktree?")
	if !got {
		t.Error("promptYN('y') = false, want true")
	}
	if !strings.Contains(w.String(), "Remove 1 worktree?") {
		t.Errorf("promptYN output %q, want prompt text", w.String())
	}
}

func TestPromptYN_uppercase(t *testing.T) {
	r := strings.NewReader("Y\n")
	var w bytes.Buffer
	if !promptYN(r, &w, "prompt") {
		t.Error("promptYN('Y') = false, want true")
	}
}

func TestPromptYN_no(t *testing.T) {
	r := strings.NewReader("n\n")
	var w bytes.Buffer
	if promptYN(r, &w, "prompt") {
		t.Error("promptYN('n') = true, want false")
	}
}

func TestPromptYN_empty(t *testing.T) {
	r := strings.NewReader("\n")
	var w bytes.Buffer
	if promptYN(r, &w, "prompt") {
		t.Error("promptYN('') = true, want false (default no)")
	}
}

func TestPromptYN_eof(t *testing.T) {
	r := strings.NewReader("")
	var w bytes.Buffer
	if promptYN(r, &w, "prompt") {
		t.Error("promptYN(EOF) = true, want false")
	}
}

func TestCleanupWorktreesCmd_force(t *testing.T) {
	dir, wtPath := setupGitWnRepo(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	resetCleanupWorktreesFlags()
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"cleanup", "worktrees", "--force"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	if _, err := os.Stat(wtPath); err == nil {
		t.Error("--force: worktree should have been removed")
	}
}

func TestCleanupWorktreesCmd_confirmY(t *testing.T) {
	dir, wtPath := setupGitWnRepo(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	resetCleanupWorktreesFlags()
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"cleanup", "worktrees"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	if _, err := os.Stat(wtPath); err == nil {
		t.Error("confirm 'y': worktree should have been removed")
	}
}

func TestCleanupWorktreesCmd_confirmN(t *testing.T) {
	dir, wtPath := setupGitWnRepo(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = os.RemoveAll(wtPath) // cleanup: test skipped removal
	})
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	resetCleanupWorktreesFlags()
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"cleanup", "worktrees"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("confirm 'n': worktree should still exist: %v", err)
	}
}

// setupWnRootWithGit creates a temp dir with a git repo and a .wn root, adds a test item,
// and returns the dir and item ID. Useful for tests that need git branch detection.
func setupWnRootWithGit(t *testing.T) (dir string, itemID string) {
	t.Helper()
	dir, itemID = setupWnRoot(t)
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	return dir, itemID
}
