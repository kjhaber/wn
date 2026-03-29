package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestMergeCmd_success(t *testing.T) {
	dir, def := setupGitWnRootNoItem(t)
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Create a feature branch with a commit.
	execIn(t, dir, "git", "checkout", "-b", "wn-abc999-feat")
	writeFile(t, filepath.Join(dir, "feat.txt"), "feature content")
	execIn(t, dir, "git", "add", "feat.txt")
	execIn(t, dir, "git", "commit", "-m", "feat commit")
	execIn(t, dir, "git", "checkout", def)

	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "abc999",
		Description: "Implement feat",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
		Notes:       []wn.Note{{Name: wn.NoteNameBranch, Body: "wn-abc999-feat", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"merge", "wn-abc999-feat", "-m", "Squash: implement feat"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn merge: %v", err)
		}
	})
	if !strings.Contains(out, "abc999") {
		t.Errorf("merge output should mention item ID; got %q", out)
	}

	got, err := store.Get("abc999")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Done {
		t.Error("item should be marked done after merge")
	}
	idx := got.NoteIndexByName(wn.NoteNameCommit)
	if idx < 0 {
		t.Error("item should have wn:commit note after merge")
	}
}

func TestMergeCmd_dryRun(t *testing.T) {
	dir, def := setupGitWnRootNoItem(t)
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	execIn(t, dir, "git", "checkout", "-b", "wn-dry999-feat")
	writeFile(t, filepath.Join(dir, "dry.txt"), "dry content")
	execIn(t, dir, "git", "add", "dry.txt")
	execIn(t, dir, "git", "commit", "-m", "dry commit")
	execIn(t, dir, "git", "checkout", def)

	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "dry999",
		Description: "Dry run item",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
		Notes:       []wn.Note{{Name: wn.NoteNameBranch, Body: "wn-dry999-feat", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"merge", "wn-dry999-feat", "-m", "dry msg", "--dry-run"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn merge --dry-run: %v", err)
		}
	})

	got, err := store.Get("dry999")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Done {
		t.Error("item should not be done after dry-run merge")
	}
}

func TestMergeCmd_missingItem(t *testing.T) {
	dir, def := setupGitWnRootNoItem(t)

	execIn(t, dir, "git", "checkout", "-b", "wn-noitem-feat")
	writeFile(t, filepath.Join(dir, "x.txt"), "x")
	execIn(t, dir, "git", "add", "x.txt")
	execIn(t, dir, "git", "commit", "-m", "x commit")
	execIn(t, dir, "git", "checkout", def)

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	err := func() error {
		root := newRootCmd()
		root.SetArgs([]string{"merge", "wn-noitem-feat", "-m", "some msg"})
		return root.Execute()
	}()
	if err == nil {
		t.Fatal("expected error for branch with no wn item")
	}
}

func TestMergeCmd_missingBranchArg(t *testing.T) {
	dir, _ := setupGitWnRootNoItem(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	err := func() error {
		root := newRootCmd()
		root.SetArgs([]string{"merge"})
		return root.Execute()
	}()
	if err == nil {
		t.Fatal("expected error for missing branch argument")
	}
}

func setupGitRepoMain(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "init")
}

// TestWnRootCmd_NormalRepo verifies that 'wn root' prints the git repo root from a normal checkout.
