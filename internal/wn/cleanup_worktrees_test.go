package wn

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListWorktrees_mainOnly(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("ListWorktrees: want 1 worktree, got %d: %v", len(wts), wts)
	}
	if wts[0].Branch == "" {
		t.Error("main worktree should have a branch name")
	}
}

func TestListWorktrees_withExtraWorktree(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	var audit bytes.Buffer
	wtPath, err := EnsureWorktree(dir, filepath.Join(dir, "wt-list-test"), "wn-list-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	defer func() { _ = RemoveWorktree(dir, wtPath, nil) }()

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("ListWorktrees: want 2 worktrees, got %d", len(wts))
	}
	found := false
	for _, wt := range wts {
		if wt.Branch == "wn-list-branch" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListWorktrees should find wn-list-branch; got %v", wts)
	}
}

func TestCleanupWorktrees_removesEligible(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "aabbcc", Description: "test task", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-aabbcc-cleanup"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Create the worktree on that branch
	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-aabbcc")
	_, err = EnsureWorktree(dir, wtPath, "wn-aabbcc-cleanup", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}

	// Add a commit on the feature branch (from the worktree) so there's something to merge
	writeFile(t, filepath.Join(wtPath, "feature.txt"), "feature work")
	execIn(t, wtPath, "git", "add", "feature.txt")
	execIn(t, wtPath, "git", "commit", "-m", "add feature")

	// Mark item done
	if err := store.UpdateItem("aabbcc", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		it.Updated = time.Now().UTC()
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}

	// Merge branch into main
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-aabbcc-cleanup", "--no-ff", "-m", "merge feature")

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	var removed []CleanupWorktreeResult
	for _, r := range results {
		if r.Status == "removed" {
			removed = append(removed, r)
		}
	}
	if len(removed) != 1 {
		t.Errorf("want 1 removed, got %d; all results: %v", len(removed), results)
	}
	if len(removed) == 1 && removed[0].ItemID != "aabbcc" {
		t.Errorf("removed ItemID = %q, want aabbcc", removed[0].ItemID)
	}

	// Worktree directory should be gone
	if _, err := os.Stat(wtPath); err == nil {
		t.Error("worktree path should have been removed")
	}
}

func TestCleanupWorktrees_skipsNotDone(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "notdone", Description: "undone task", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-notdone-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-notdone")
	_, err = EnsureWorktree(dir, wtPath, "wn-notdone-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	defer func() { _ = RemoveWorktree(dir, wtPath, nil) }()

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	for _, r := range results {
		if r.Branch == "wn-notdone-branch" && r.Status != "skipped_not_done" {
			t.Errorf("want skipped_not_done for undone item, got status %q", r.Status)
		}
	}

	// Worktree should still exist
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree should still exist: %v", err)
	}
}

func TestCleanupWorktrees_skipsNotMerged(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "notmrgd", Description: "unmerged task", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-notmrgd-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-notmrgd")
	_, err = EnsureWorktree(dir, wtPath, "wn-notmrgd-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	defer func() { _ = RemoveWorktree(dir, wtPath, nil) }()

	// Add a commit on the feature branch so it's ahead of main (not merged)
	writeFile(t, filepath.Join(wtPath, "unmerged.txt"), "unmerged work")
	execIn(t, wtPath, "git", "add", "unmerged.txt")
	execIn(t, wtPath, "git", "commit", "-m", "unmerged commit")

	// Mark item done (but branch not merged)
	if err := store.UpdateItem("notmrgd", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Branch == "wn-notmrgd-branch" {
			found = true
			if r.Status != "skipped_not_merged" {
				t.Errorf("want skipped_not_merged for unmerged branch, got status %q reason %q", r.Status, r.Reason)
			}
		}
	}
	if !found {
		t.Errorf("no result for wn-notmrgd-branch; got %v", results)
	}

	// Worktree should still exist
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree should still exist: %v", err)
	}
}

func TestCleanupWorktrees_skipsNoItem(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	// No items in store at all

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-noitem")
	_, err = EnsureWorktree(dir, wtPath, "wn-noitem-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	defer func() { _ = RemoveWorktree(dir, wtPath, nil) }()

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Branch == "wn-noitem-branch" {
			found = true
			if r.Status != "skipped_no_item" {
				t.Errorf("want skipped_no_item, got status %q", r.Status)
			}
		}
	}
	if !found {
		t.Errorf("no result for wn-noitem-branch; got %v", results)
	}
}

func TestCleanupWorktrees_dryRun(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "dryrun", Description: "dry run task", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-dryrun-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-dryrun")
	_, err = EnsureWorktree(dir, wtPath, "wn-dryrun-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	defer func() { _ = RemoveWorktree(dir, wtPath, nil) }()

	// Mark done (branch at same commit as main = "merged")
	if err := store.UpdateItem("dryrun", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}

	results, err := CleanupWorktrees(store, dir, "", true /* dryRun */, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees dryRun: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Branch == "wn-dryrun-branch" {
			found = true
			if r.Status != "removed" {
				t.Errorf("dry-run: want status removed (would remove), got %q", r.Status)
			}
		}
	}
	if !found {
		t.Errorf("no result for wn-dryrun-branch; got %v", results)
	}

	// Worktree should still exist (it's a dry run)
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("dry-run: worktree should still exist: %v", err)
	}
}

func TestCleanupWorktrees_cleanIgnored(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "clnigg", Description: "clean ignored test", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-clnigg-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-clnigg")
	_, err = EnsureWorktree(dir, wtPath, "wn-clnigg-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}

	// Add a .gitignore in the worktree and create an ignored file
	writeFile(t, filepath.Join(wtPath, ".gitignore"), "build/\n")
	execIn(t, wtPath, "git", "add", ".gitignore")
	execIn(t, wtPath, "git", "commit", "-m", "add gitignore")
	if err := os.MkdirAll(filepath.Join(wtPath, "build"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(wtPath, "build", "artifact"), "binary data")

	// Mark done
	if err := store.UpdateItem("clnigg", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}

	// Merge into main
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-clnigg-branch", "--no-ff", "-m", "merge")

	results, err := CleanupWorktrees(store, dir, "", false, true /* cleanIgnored */, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees(cleanIgnored): %v", err)
	}

	removed := 0
	for _, r := range results {
		if r.Status == "removed" {
			removed++
		}
	}
	if removed != 1 {
		t.Errorf("want 1 removed, got %d; results: %v", removed, results)
	}

	// Worktree should be gone
	if _, err := os.Stat(wtPath); err == nil {
		t.Error("worktree should have been removed")
	}
}

func TestCleanupWorktrees_deletesBranchByDefault(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "delbr1", Description: "branch delete test", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-delbr1-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-delbr1")
	_, err = EnsureWorktree(dir, wtPath, "wn-delbr1-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}

	// Add a commit so there's something to merge
	writeFile(t, filepath.Join(wtPath, "delbr1.txt"), "work")
	execIn(t, wtPath, "git", "add", "delbr1.txt")
	execIn(t, wtPath, "git", "commit", "-m", "add delbr1")

	// Mark done and merge
	if err := store.UpdateItem("delbr1", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-delbr1-branch", "--no-ff", "-m", "merge delbr1")

	// Run cleanup without worktrees-only (default: also delete branches)
	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	var removed []CleanupWorktreeResult
	for _, r := range results {
		if r.Status == "removed" && r.Branch == "wn-delbr1-branch" {
			removed = append(removed, r)
		}
	}
	if len(removed) != 1 {
		t.Errorf("want 1 removed result, got %d; results: %v", len(removed), results)
	}
	if len(removed) == 1 && !removed[0].BranchDeleted {
		t.Errorf("BranchDeleted should be true when worktreesOnly=false")
	}

	// Worktree should be gone
	if _, err := os.Stat(wtPath); err == nil {
		t.Error("worktree path should have been removed")
	}
	// Branch should also be deleted
	exists, err := BranchExists(dir, "wn-delbr1-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if exists {
		t.Error("branch should have been deleted")
	}
}

func TestCleanupWorktrees_worktreesOnlySkipsBranchDelete(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "wtonly", Description: "worktrees only test", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-wtonly-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-wtonly")
	_, err = EnsureWorktree(dir, wtPath, "wn-wtonly-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}

	writeFile(t, filepath.Join(wtPath, "wtonly.txt"), "work")
	execIn(t, wtPath, "git", "add", "wtonly.txt")
	execIn(t, wtPath, "git", "commit", "-m", "add wtonly")

	if err := store.UpdateItem("wtonly", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-wtonly-branch", "--no-ff", "-m", "merge wtonly")

	// Run cleanup with worktrees-only=true
	results, err := CleanupWorktrees(store, dir, "", false, false, true, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	var removed []CleanupWorktreeResult
	for _, r := range results {
		if r.Status == "removed" && r.Branch == "wn-wtonly-branch" {
			removed = append(removed, r)
		}
	}
	if len(removed) != 1 {
		t.Errorf("want 1 removed result, got %d; results: %v", len(removed), results)
	}
	if len(removed) == 1 && removed[0].BranchDeleted {
		t.Errorf("BranchDeleted should be false when worktreesOnly=true")
	}

	// Worktree should be gone
	if _, err := os.Stat(wtPath); err == nil {
		t.Error("worktree path should have been removed")
	}
	// Branch should still exist
	exists, err := BranchExists(dir, "wn-wtonly-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !exists {
		t.Error("branch should still exist when worktreesOnly=true")
	}
}

func TestCleanupWorktrees_orphanedBranchCleaned(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "orphan", Description: "orphaned branch test", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-orphan-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Create branch with a commit but NO worktree
	def, _ := DefaultBranch(dir) // capture before switching branches
	execIn(t, dir, "git", "branch", "wn-orphan-branch")
	// Temporarily check it out to add a commit, then go back
	execIn(t, dir, "git", "checkout", "wn-orphan-branch")
	writeFile(t, filepath.Join(dir, "orphan.txt"), "orphaned work")
	execIn(t, dir, "git", "add", "orphan.txt")
	execIn(t, dir, "git", "commit", "-m", "orphan commit")

	// Mark done and merge back to main
	if err := store.UpdateItem("orphan", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-orphan-branch", "--no-ff", "-m", "merge orphan")
	// Branch exists but no worktree for it

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	var branchDeleted []CleanupWorktreeResult
	for _, r := range results {
		if r.Status == "branch_deleted" && r.Branch == "wn-orphan-branch" {
			branchDeleted = append(branchDeleted, r)
		}
	}
	if len(branchDeleted) != 1 {
		t.Errorf("want 1 branch_deleted result, got %d; results: %v", len(branchDeleted), results)
	}
	if len(branchDeleted) == 1 && !branchDeleted[0].BranchDeleted {
		t.Errorf("BranchDeleted should be true for orphaned branch result")
	}

	// Branch should be gone
	exists, err := BranchExists(dir, "wn-orphan-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if exists {
		t.Error("orphaned branch should have been deleted")
	}
}

func TestCleanupWorktrees_orphanedBranchWorktreesOnly(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &Item{
		ID: "orphnw", Description: "orphaned branch worktrees-only test", Created: now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{{Name: "branch", Created: now, Body: "wn-orphnw-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Create branch with a commit but NO worktree
	def, _ := DefaultBranch(dir) // capture before switching branches
	execIn(t, dir, "git", "branch", "wn-orphnw-branch")
	execIn(t, dir, "git", "checkout", "wn-orphnw-branch")
	writeFile(t, filepath.Join(dir, "orphnw.txt"), "orphaned work")
	execIn(t, dir, "git", "add", "orphnw.txt")
	execIn(t, dir, "git", "commit", "-m", "orphnw commit")

	if err := store.UpdateItem("orphnw", func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneStatus = DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "wn-orphnw-branch", "--no-ff", "-m", "merge orphnw")

	// Run with worktrees-only=true: orphaned branches should be skipped
	results, err := CleanupWorktrees(store, dir, "", false, false, true, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	for _, r := range results {
		if r.Branch == "wn-orphnw-branch" && r.Status == "branch_deleted" {
			t.Errorf("should not delete orphaned branch when worktreesOnly=true")
		}
	}

	// Branch should still exist
	exists, err := BranchExists(dir, "wn-orphnw-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !exists {
		t.Error("orphaned branch should still exist when worktreesOnly=true")
	}
}

func TestCleanupWorktrees_squashMergeWithCommitNote(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	// Create a worktree on a feature branch with a commit.
	var audit bytes.Buffer
	wtPath := filepath.Join(dir, "wt-squash")
	_, err = EnsureWorktree(dir, wtPath, "wn-squash-branch", true, &audit)
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	writeFile(t, filepath.Join(wtPath, "squash.txt"), "squash work")
	execIn(t, wtPath, "git", "add", "squash.txt")
	execIn(t, wtPath, "git", "commit", "-m", "squash work")

	// Squash-merge the branch into main.
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "--squash", "wn-squash-branch")
	execIn(t, dir, "git", "commit", "-m", "squash merge wn-squash-branch")
	squashCommit := strings.TrimSpace(execOut(t, dir, "git", "rev-parse", "HEAD"))

	// Item is done and has the squash commit stored as a note.
	item := &Item{
		ID: "squash", Description: "squash test", Created: now, Updated: now,
		Done:       true,
		DoneStatus: DoneStatusDone,
		Tags:       []string{}, DependsOn: []string{},
		Log: []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{
			{Name: NoteNameBranch, Body: "wn-squash-branch", Created: now},
			{Name: NoteNameCommit, Body: squashCommit, Created: now},
		},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	var removed []CleanupWorktreeResult
	for _, r := range results {
		if r.Status == "removed" {
			removed = append(removed, r)
		}
	}
	if len(removed) != 1 {
		t.Errorf("want 1 removed for squash merge, got %d; all results: %v", len(removed), results)
	}
	if _, statErr := os.Stat(wtPath); statErr == nil {
		t.Error("worktree path should have been removed")
	}
}

func TestCleanupWorktrees_orphanedBranchSquashMerge(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()

	// Create branch with a commit but NO worktree.
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "branch", "wn-sqorphan-branch")
	execIn(t, dir, "git", "checkout", "wn-sqorphan-branch")
	writeFile(t, filepath.Join(dir, "sqorphan.txt"), "work")
	execIn(t, dir, "git", "add", "sqorphan.txt")
	execIn(t, dir, "git", "commit", "-m", "sqorphan work")

	// Squash-merge into main (no worktree involved).
	execIn(t, dir, "git", "checkout", def)
	execIn(t, dir, "git", "merge", "--squash", "wn-sqorphan-branch")
	execIn(t, dir, "git", "commit", "-m", "squash merge sqorphan")
	squashCommit := strings.TrimSpace(execOut(t, dir, "git", "rev-parse", "HEAD"))

	item := &Item{
		ID: "sqorph", Description: "squash orphan test", Created: now, Updated: now,
		Done:       true,
		DoneStatus: DoneStatusDone,
		Tags:       []string{}, DependsOn: []string{},
		Log: []LogEntry{{At: now, Kind: "created"}},
		Notes: []Note{
			{Name: NoteNameBranch, Body: "wn-sqorphan-branch", Created: now},
			{Name: NoteNameCommit, Body: squashCommit, Created: now},
		},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := CleanupWorktrees(store, dir, "", false, false, false, nil)
	if err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	var branchDeleted []CleanupWorktreeResult
	for _, r := range results {
		if r.Status == "branch_deleted" && r.Branch == "wn-sqorphan-branch" {
			branchDeleted = append(branchDeleted, r)
		}
	}
	if len(branchDeleted) != 1 {
		t.Errorf("want 1 branch_deleted for squash orphan, got %d; all results: %v", len(branchDeleted), results)
	}
}
