package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestDoWithoutArgNoCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	// No current task set
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"do"})
	err := root.Execute()
	if err == nil {
		t.Error("wn do without arg and no current task should fail")
	}
	if !strings.Contains(err.Error(), "no current task") {
		t.Errorf("want 'no current task' error; got: %v", err)
	}
}

func TestDoWithArgInvokesAgentOrch(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn do <id> should invoke agent-orch logic. It fails before running (agent_cmd, default branch, or similar).
	root := newRootCmd()
	root.SetArgs([]string{"do", itemID})
	err := root.Execute()
	if err == nil {
		t.Error("wn do without full setup should fail")
	}
	if strings.Contains(err.Error(), "unknown command") {
		t.Errorf("wn do should reach agent-orch; got: %v", err)
	}
}

func TestDoWithoutArgUsesCurrent(t *testing.T) {
	dir, _ := setupWnRoot(t) // has current task set
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn do with no arg should use current task and reach agent-orch. Fails before running.
	root := newRootCmd()
	root.SetArgs([]string{"do"})
	err := root.Execute()
	if err == nil {
		t.Error("wn do without full setup should fail")
	}
	if strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "no current task") {
		t.Errorf("wn do (no arg) should use current and reach agent-orch; got: %v", err)
	}
}

func TestWorktreeSetup_noCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"worktree"})
	err := root.Execute()
	if err == nil {
		t.Error("wn worktree with no current task should fail")
	}
	if !strings.Contains(err.Error(), "no current task") {
		t.Errorf("want 'no current task'; got: %v", err)
	}
}

func TestWorktreeSetup_nextAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"worktree", "--next", itemID})
	err := root.Execute()
	if err == nil {
		t.Error("wn worktree --next <id> should fail")
	}
	if !strings.Contains(err.Error(), "either") {
		t.Errorf("want mutual exclusion error; got: %v", err)
	}
}

func TestWorktreeSetup_withID(t *testing.T) {
	dir, itemID := setupGitWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	worktreesBase := filepath.Join(dir, "worktrees")
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"worktree", itemID, "--worktree-base", worktreesBase})
		if err := root.Execute(); err != nil {
			t.Errorf("wn worktree %s: %v", itemID, err)
		}
	})
	worktreePath := strings.TrimSpace(out)
	if worktreePath == "" {
		t.Fatal("wn worktree printed empty path")
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree path %q should exist: %v", worktreePath, err)
	}
	// Item should be claimed
	store, _ := wn.NewFileStore(dir)
	item, _ := store.Get(itemID)
	if item.InProgressUntil.IsZero() {
		t.Error("item should be claimed after wn worktree")
	}
	// Branch note should be set using the wn:branch special note name
	if item.NoteIndexByName(wn.NoteNameBranch) < 0 {
		t.Error("item should have wn:branch note after wn worktree")
	}
}

func TestWorktreeSetup_usesCurrent(t *testing.T) {
	dir, itemID := setupGitWnRoot(t)
	// Set the item as current
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: itemID}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	worktreesBase := filepath.Join(dir, "worktrees")
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"worktree", "--worktree-base", worktreesBase})
		if err := root.Execute(); err != nil {
			t.Errorf("wn worktree (no args, current set): %v", err)
		}
	})
	worktreePath := strings.TrimSpace(out)
	if worktreePath == "" {
		t.Fatal("wn worktree printed empty path")
	}
	store, _ := wn.NewFileStore(dir)
	item, _ := store.Get(itemID)
	if item.InProgressUntil.IsZero() {
		t.Error("current item should be claimed after wn worktree")
	}
}

func TestWorktreeSetup_claimsNext(t *testing.T) {
	dir, itemID := setupGitWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	worktreesBase := filepath.Join(dir, "worktrees")
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"worktree", "--next", "--worktree-base", worktreesBase})
		if err := root.Execute(); err != nil {
			t.Errorf("wn worktree --next: %v", err)
		}
	})
	worktreePath := strings.TrimSpace(out)
	if worktreePath == "" {
		t.Fatal("wn worktree printed empty path")
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree path %q should exist: %v", worktreePath, err)
	}
	// Item should be claimed
	store, _ := wn.NewFileStore(dir)
	item, _ := store.Get(itemID)
	if item.InProgressUntil.IsZero() {
		t.Error("next item should be claimed after wn worktree")
	}
}

func TestWorktreeSetup_branchFlag(t *testing.T) {
	dir, itemID := setupGitWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	worktreesBase := filepath.Join(dir, "worktrees")
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"worktree", itemID, "--worktree-base", worktreesBase, "--branch", "my-feature"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn worktree --branch: %v", err)
		}
	})
	worktreePath := strings.TrimSpace(out)
	if worktreePath == "" {
		t.Fatal("wn worktree printed empty path")
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree path %q should exist: %v", worktreePath, err)
	}
	store, _ := wn.NewFileStore(dir)
	item, _ := store.Get(itemID)
	idx := item.NoteIndexByName("branch")
	if idx < 0 {
		t.Fatal("item should have branch note after wn worktree --branch")
	}
	got := item.Notes[idx].Body
	wantSuffix := "wn-" + itemID + "-my-feature"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("branch note = %q, want suffix %q", got, wantSuffix)
	}
	// Worktree path should reflect the branch name
	if !strings.Contains(worktreePath, "my-feature") {
		t.Errorf("worktree path %q should contain branch slug %q", worktreePath, "my-feature")
	}
}

func TestWorktreeSetup_branchFlagWithPrefix(t *testing.T) {
	dir, itemID := setupGitWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	worktreesBase := filepath.Join(dir, "worktrees")
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"worktree", itemID, "--worktree-base", worktreesBase, "--branch", "my-feature", "--branch-prefix", "keith/"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn worktree --branch --branch-prefix: %v", err)
		}
	})
	worktreePath := strings.TrimSpace(out)
	if worktreePath == "" {
		t.Fatal("wn worktree printed empty path")
	}
	store, _ := wn.NewFileStore(dir)
	item, _ := store.Get(itemID)
	idx := item.NoteIndexByName("branch")
	if idx < 0 {
		t.Fatal("item should have branch note")
	}
	got := item.Notes[idx].Body
	want := "keith/wn-" + itemID + "-my-feature"
	if got != want {
		t.Errorf("branch note = %q, want %q", got, want)
	}
}

// TestDoUnified_nextAndIdArgError verifies that "wn do --next <id>" is rejected.

func TestDoUnified_nextAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"do", "--next", itemID})
	err := root.Execute()
	if err == nil {
		t.Error("wn do --next <id> should fail")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("want mutual exclusion error; got: %v", err)
	}
}

// TestDoUnified_loopAndIdArgError verifies that "wn do --loop <id>" is rejected.

func TestDoUnified_loopAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"do", "--loop", itemID})
	err := root.Execute()
	if err == nil {
		t.Error("wn do --loop <id> should fail")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("want mutual exclusion error; got: %v", err)
	}
}

// TestDoUnified_nextEmptyQueue verifies that "wn do --next" errors immediately when no items are queued.

func TestDoUnified_nextEmptyQueue(t *testing.T) {
	// Needs a git repo so default branch detection doesn't fail before we reach the queue check.
	dir := t.TempDir()
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	// No items added — queue is empty.
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"do", "--next"})
	err := root.Execute()
	if err == nil {
		t.Error("wn do --next on empty queue should fail")
	}
	if !strings.Contains(err.Error(), "no items") {
		t.Errorf("want 'no items' error; got: %v", err)
	}
}

// TestDoUnified_nCurrentError verifies that "wn do -n N" without --loop is rejected.

func TestDoUnified_nWithoutLoopError(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"do", "-n", "3"})
	err := root.Execute()
	if err == nil {
		t.Error("wn do -n N without --loop should fail")
	}
	if !strings.Contains(err.Error(), "--loop") {
		t.Errorf("want error mentioning --loop; got: %v", err)
	}
}

// TestDoWithBlockedItem verifies that "wn do <id>" fails when the item is blocked.

func TestDoWithBlockedItem(t *testing.T) {
	dir := t.TempDir()
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
	dep := &wn.Item{ID: "dep1", Description: "dep", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	blocked := &wn.Item{ID: "task1", Description: "add widget feature", DependsOn: []string{"dep1"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(dep); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(blocked); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"do", "task1"})
	err = root.Execute()
	if err == nil {
		t.Fatal("wn do <blocked-id> should fail")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("want error containing 'blocked'; got: %v", err)
	}
}

// TestDoCurrentItemBlocked verifies that "wn do" (no arg) fails when the current item is blocked.

func TestDoCurrentItemBlocked(t *testing.T) {
	dir := t.TempDir()
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
	dep := &wn.Item{ID: "dep1", Description: "dep", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	blocked := &wn.Item{ID: "task1", Description: "add widget feature", DependsOn: []string{"dep1"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(dep); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(blocked); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "task1"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"do"})
	err = root.Execute()
	if err == nil {
		t.Fatal("wn do with blocked current item should fail")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("want error containing 'blocked'; got: %v", err)
	}
}

// TestLaunchWithBlockedItem verifies that "wn launch <id>" fails when the item is blocked.

func TestLaunchWithBlockedItem(t *testing.T) {
	dir := t.TempDir()
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
	dep := &wn.Item{ID: "dep1", Description: "dep", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	blocked := &wn.Item{ID: "task1", Description: "add widget feature", DependsOn: []string{"dep1"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(dep); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(blocked); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"launch", "task1"})
	err = root.Execute()
	if err == nil {
		t.Fatal("wn launch <blocked-id> should fail")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("want error containing 'blocked'; got: %v", err)
	}
}

// TestLaunchCurrentItemBlocked verifies that "wn launch" (no arg) fails when the current item is blocked.

func TestLaunchCurrentItemBlocked(t *testing.T) {
	dir := t.TempDir()
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
	dep := &wn.Item{ID: "dep1", Description: "dep", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	blocked := &wn.Item{ID: "task1", Description: "add widget feature", DependsOn: []string{"dep1"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(dep); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(blocked); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "task1"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"launch"})
	err = root.Execute()
	if err == nil {
		t.Fatal("wn launch with blocked current item should fail")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("want error containing 'blocked'; got: %v", err)
	}
}

func TestLaunchWithoutArgNoCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"launch"})
	err := root.Execute()
	if err == nil {
		t.Error("wn launch without current task should fail")
	}
	if !strings.Contains(err.Error(), "no current task") {
		t.Errorf("want 'no current task' error; got: %v", err)
	}
}

func TestLaunchNoLaunchRunnerConfigured(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	// Isolate from global settings so no default_launch runner is inherited.
	t.Setenv("WN_CONFIG_DIR", t.TempDir())

	// No settings file, so no default_launch runner configured.
	root := newRootCmd()
	root.SetArgs([]string{"launch"})
	err := root.Execute()
	if err == nil {
		t.Error("wn launch without configured launch runner should fail")
	}
	if !strings.Contains(err.Error(), "no runner") {
		t.Errorf("want 'no runner' error; got: %v", err)
	}
}

func TestLaunchNextAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"launch", "--next", itemID})
	err := root.Execute()
	if err == nil {
		t.Error("wn launch --next <id> should fail")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("want mutual exclusion error; got: %v", err)
	}
}

// TestLaunchLoopAndIdArgError verifies that "wn launch --loop <id>" is rejected.

func TestLaunchLoopAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"launch", "--loop", itemID})
	err := root.Execute()
	if err == nil {
		t.Error("wn launch --loop <id> should fail")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("want mutual exclusion error; got: %v", err)
	}
}

// TestLaunchNWithoutLoopError verifies that "wn launch -n N" without --loop is rejected.

func TestLaunchNWithoutLoopError(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"launch", "-n", "3"})
	err := root.Execute()
	if err == nil {
		t.Error("wn launch -n N without --loop should fail")
	}
	if !strings.Contains(err.Error(), "--loop") {
		t.Errorf("want error mentioning --loop; got: %v", err)
	}
}

// TestLaunchNextEmptyQueue verifies that "wn launch --next" errors immediately when queue is empty.

func TestLaunchNextEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	// No items added — queue is empty.
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"launch", "--next"})
	err := root.Execute()
	if err == nil {
		t.Error("wn launch --next on empty queue should fail")
	}
	if !strings.Contains(err.Error(), "no items") {
		t.Errorf("want 'no items' error; got: %v", err)
	}
}

// TestLaunchNext_currentUndoneItemUsed verifies that "wn launch --next" uses the current item
// when it is undone, rather than picking the next item from the queue.
// item2 is given higher queue priority (Order=1) than item1 (Order=nil/default=99),
// so ClaimNextItem would pick item2 without the fix — but the fix should re-use item1.

func TestLaunchNext_currentUndoneItemUsed(t *testing.T) {
	dir, item1ID := setupGitWnRoot(t) // item1 = abc123
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	order1 := 1 // higher queue priority than item1 (DefaultOrder=99)
	item2 := &wn.Item{ID: "def456", Description: "other task", Created: now, Updated: now, Order: &order1, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}
	// Set item1 as current
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: item1ID}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	worktreesBase := filepath.Join(dir, "worktrees")
	defer func() { _ = os.Chdir(cwd) }()

	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"launch", "--next", "--worktree-base", worktreesBase})
	if err := root.Execute(); err != nil {
		t.Fatalf("wn launch --next: %v", err)
	}

	// item1 (current, undone) should be claimed — not item2 (which has higher queue priority)
	item1, _ := store.Get(item1ID)
	if item1.InProgressUntil.IsZero() {
		t.Error("current item1 should be claimed when wn launch --next is run with undone current item")
	}
	// item2 should NOT be claimed despite its higher queue priority
	item2updated, _ := store.Get("def456")
	if !item2updated.InProgressUntil.IsZero() {
		t.Error("item2 should not be claimed when current item1 is undone")
	}
}

// TestLaunchNext_currentReviewReadyPicksFromQueue verifies that "wn launch --next" picks
// from the queue when the current item is already review-ready (agent work is done).

func TestLaunchNext_currentReviewReadyPicksFromQueue(t *testing.T) {
	dir, item1ID := setupGitWnRoot(t) // item1 = abc123
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	// Mark item1 as review-ready
	now := time.Now().UTC()
	if err := store.UpdateItem(item1ID, func(it *wn.Item) (*wn.Item, error) {
		it.ReviewReady = true
		it.Updated = now
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	// Add item2 as undone
	item2 := &wn.Item{ID: "def456", Description: "other task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}
	// Set item1 as current
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: item1ID}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	worktreesBase := filepath.Join(dir, "worktrees")
	defer func() { _ = os.Chdir(cwd) }()

	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"launch", "--next", "--worktree-base", worktreesBase})
	if err := root.Execute(); err != nil {
		t.Fatalf("wn launch --next: %v", err)
	}

	// item2 should be claimed (item1 is review-ready so skip it)
	item2updated, _ := store.Get("def456")
	if item2updated.InProgressUntil.IsZero() {
		t.Error("item2 should be claimed when current item1 is review-ready")
	}
}

// TestDoNext_currentUndoneItemUsed verifies that "wn do --next" uses the current item
// when it is undone, rather than picking the next item from the queue.
// item2 is given higher queue priority (Order=1) than item1 (Order=nil/default=99),
// so ClaimNextItem would pick item2 without the fix — but the fix should re-use item1.

func TestDoNext_currentUndoneItemUsed(t *testing.T) {
	dir, item1ID := setupGitWnRoot(t) // item1 = abc123
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	order1 := 1 // higher queue priority than item1 (DefaultOrder=99)
	item2 := &wn.Item{ID: "def456", Description: "other task", Created: now, Updated: now, Order: &order1, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item2); err != nil {
		t.Fatalf("Put item2: %v", err)
	}
	// Set item1 as current
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: item1ID}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	worktreesBase := filepath.Join(dir, "worktrees")
	defer func() { _ = os.Chdir(cwd) }()

	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	root := newRootCmd()
	root.SetArgs([]string{"do", "--next", "--worktree-base", worktreesBase})
	if err := root.Execute(); err != nil {
		t.Fatalf("wn do --next: %v", err)
	}

	// item1 (current, undone) should be review-ready after the run — not item2 (higher queue priority)
	item1, _ := store.Get(item1ID)
	if !item1.ReviewReady {
		t.Error("current item1 should be review-ready after wn do --next with undone current item")
	}
	// item2 should NOT be review-ready despite its higher queue priority
	item2updated, _ := store.Get("def456")
	if item2updated.ReviewReady {
		t.Error("item2 should not be processed when current item1 is undone")
	}
}

func TestPromptCmd_createsItemAndDep(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"prompt", parentID, "-m", "What should the behavior be?"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "created") {
		t.Errorf("expected 'created' in output, got %q", out)
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("expected 'blocked' in output, got %q", out)
	}
	store, _ := wn.NewFileStore(dir)
	allItems, _ := store.List()
	var promptItem *wn.Item
	for _, it := range allItems {
		if it.ID != parentID && it.PromptReady {
			promptItem = it
			break
		}
	}
	if promptItem == nil {
		t.Fatal("no PromptReady item found after wn prompt")
	}
	if promptItem.Description != "What should the behavior be?" {
		t.Errorf("prompt item description = %q, want expected question", promptItem.Description)
	}
	parent, _ := store.Get(parentID)
	found := false
	for _, dep := range parent.DependsOn {
		if dep == promptItem.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("parent %s does not depend on prompt item %s; DependsOn=%v", parentID, promptItem.ID, parent.DependsOn)
	}
}

func TestPromptCmd_fallsBackToCurrentItem(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"prompt", "-m", "Is this right?"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "created") {
		t.Errorf("expected 'created' in output, got %q", out)
	}
}

func TestRespondCmd_marksPromptDone(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID: "pmt111", Description: "Is this right?",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	_ = store.Put(promptItem)

	root := newRootCmd()
	root.SetArgs([]string{"respond", "pmt111", "-m", "Yes, proceed."})
	if err := root.Execute(); err != nil {
		t.Errorf("Execute respond: %v", err)
	}
	got, _ := store.Get("pmt111")
	if !got.Done {
		t.Error("after respond: prompt item should be done")
	}
	idx := got.NoteIndexByName(wn.NoteNameResponse)
	if idx < 0 {
		t.Error("after respond: 'response' note not found on prompt item")
	} else if got.Notes[idx].Body != "Yes, proceed." {
		t.Errorf("response note body = %q, want 'Yes, proceed.'", got.Notes[idx].Body)
	}
}

func TestRespondCmd_rejectsNonPromptItem(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"respond", itemID, "-m", "This should fail."})
	err := root.Execute()
	if err == nil {
		t.Error("expected error when responding to non-prompt item")
	}
}

func TestSummaryEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"summary"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// Header line should appear even with no items.
	if !strings.Contains(out, "status") {
		t.Errorf("output should contain 'status' header; got:\n%s", out)
	}
	if !strings.Contains(out, "count") {
		t.Errorf("output should contain 'count' header; got:\n%s", out)
	}
	// No tag section when there are no active items.
	if strings.Contains(out, "tag") {
		t.Errorf("output should not contain 'tag' section for empty store; got:\n%s", out)
	}
}

func TestSummaryStatusCounts(t *testing.T) {
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
		{ID: "aaa111", Description: "undone task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "aaa222", Description: "another undone task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb111", Description: "done task", Created: now, Updated: now, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc111", Description: "review task", Created: now, Updated: now, ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"summary"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	// Parse counts from the status section (before the blank line).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	counts := map[string]int{}
	for _, line := range lines {
		if line == "" {
			break
		}
		fields := strings.Fields(line)
		if len(fields) == 2 {
			var n int
			if _, err := fmt.Sscanf(fields[1], "%d", &n); err == nil {
				counts[fields[0]] = n
			}
		}
	}
	if counts["undone"] != 2 {
		t.Errorf("undone count = %d, want 2; output:\n%s", counts["undone"], out)
	}
	if counts["done"] != 1 {
		t.Errorf("done count = %d, want 1; output:\n%s", counts["done"], out)
	}
	if counts["review"] != 1 {
		t.Errorf("review count = %d, want 1; output:\n%s", counts["review"], out)
	}
}

func TestSummaryTagCounts(t *testing.T) {
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
		{ID: "aaa111", Description: "task A1", Created: now, Updated: now, Tags: []string{"agent"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "aaa222", Description: "task A2", Created: now, Updated: now, Tags: []string{"agent"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb111", Description: "task B review", Created: now, Updated: now, Tags: []string{"backend"}, ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ddd111", Description: "done task", Created: now, Updated: now, Tags: []string{"agent"}, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"summary"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	// Tag section header should appear.
	if !strings.Contains(out, "agent") {
		t.Errorf("output should contain 'agent' tag; got:\n%s", out)
	}
	if !strings.Contains(out, "backend") {
		t.Errorf("output should contain 'backend' tag; got:\n%s", out)
	}

	// Parse tag rows from after the blank line. Columns: tag undone blocked review.
	tagSectionIdx := strings.Index(out, "\n\n")
	if tagSectionIdx < 0 {
		t.Fatalf("no blank-line separator between status and tag sections; output:\n%s", out)
	}
	tagSection := out[tagSectionIdx:]
	lines := strings.Split(tagSection, "\n")
	agentUndone := -1
	backendReview := -1
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			switch fields[0] {
			case "agent":
				_, _ = fmt.Sscanf(fields[1], "%d", &agentUndone)
			case "backend":
				_, _ = fmt.Sscanf(fields[3], "%d", &backendReview)
			}
		}
	}
	if agentUndone != 2 {
		t.Errorf("agent undone count = %d, want 2; output:\n%s", agentUndone, out)
	}
	if backendReview != 1 {
		t.Errorf("backend review count = %d, want 1; output:\n%s", backendReview, out)
	}
}

func TestSummaryNoTagsRow(t *testing.T) {
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
		{ID: "aaa111", Description: "tagged task", Created: now, Updated: now, Tags: []string{"mytag"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb111", Description: "untagged task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"summary"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "(no tags)") {
		t.Errorf("output should contain '(no tags)' row for untagged active items; got:\n%s", out)
	}
}

func TestSummaryBlockedCount(t *testing.T) {
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
		{ID: "aaa111", Description: "blocker", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb111", Description: "blocked task", Created: now, Updated: now, DependsOn: []string{"aaa111"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"summary"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	// Parse blocked count from status section (before the blank line).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var blockedCount int
	for _, line := range lines {
		if line == "" {
			break
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "blocked" {
			_, _ = fmt.Sscanf(fields[1], "%d", &blockedCount)
		}
	}
	if blockedCount != 1 {
		t.Errorf("blocked count = %d, want 1; output:\n%s", blockedCount, out)
	}
}
