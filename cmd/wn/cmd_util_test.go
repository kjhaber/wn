package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestRmWithExplicitId(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"rm", itemID})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm %s: %v", itemID, err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if _, err := store.Get(itemID); err == nil {
		t.Error("item should be removed")
	}
}

func TestRmMultipleIds(t *testing.T) {
	dir, _ := setupWnRoot(t)
	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	for _, it := range []*wn.Item{
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "cc3333", Description: "third", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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

	root := newRootCmd()
	root.SetArgs([]string{"rm", "abc123", "bb2222", "cc3333"})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm multiple: %v", err)
	}
	for _, id := range []string{"abc123", "bb2222", "cc3333"} {
		if _, err := store.Get(id); err == nil {
			t.Errorf("item %s should be removed", id)
		}
	}
}

func TestRmNoArgsRemovesCurrentItem(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	if err := store.Put(&wn.Item{ID: "bb2222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: itemID}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"rm"})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm (no args): %v", err)
	}
	// Current item should be removed
	if _, err := store.Get(itemID); err == nil {
		t.Error("current item should be removed")
	}
	// Other items should remain
	if _, err := store.Get("bb2222"); err != nil {
		t.Errorf("item bb2222 should still exist: %v", err)
	}
	// CurrentID should be cleared
	meta, _ := wn.ReadMeta(dir)
	if meta.CurrentID != "" {
		t.Errorf("CurrentID should be cleared; got %q", meta.CurrentID)
	}
}

func TestRmNoArgsErrorsWhenNoCurrent(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	store, _ := wn.NewFileStore(dir)
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: ""}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"rm"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when no current item, got nil")
	}
	// Item should be untouched
	if _, err := store.Get(itemID); err != nil {
		t.Errorf("item should still exist: %v", err)
	}
}

func TestRmClearsCurrentWhenDeleted(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"rm", itemID})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm: %v", err)
	}
	meta, _ := wn.ReadMeta(dir)
	if meta.CurrentID != "" {
		t.Errorf("CurrentID should be cleared when current task is removed; got %q", meta.CurrentID)
	}
}

func TestArchiveCmd_RemovesItemFromStore(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"archive", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "archived "+itemID) {
		t.Errorf("output = %q, want 'archived %s'", out, itemID)
	}

	// Item should be gone from store
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("store should be empty after archive, got %d items", len(items))
	}
}

func TestArchiveCmd_WritesArchiveFile(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"archive", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	archivePath := filepath.Join(dir, ".wn", "archive", itemID+".json")
	if _, err := os.Stat(archivePath); err != nil {
		t.Errorf("archive file not created at %s: %v", archivePath, err)
	}
}

func TestArchiveCmd_CustomLocation(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	customDir := filepath.Join(t.TempDir(), "custom-archive")
	captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"archive", "--location", customDir, itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	archivePath := filepath.Join(customDir, itemID+".json")
	if _, err := os.Stat(archivePath); err != nil {
		t.Errorf("archive file not created at %s: %v", archivePath, err)
	}
}

func TestArchiveCmd_ClearsCurrentID(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"archive", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "" {
		t.Errorf("CurrentID = %q, want empty after archiving current item", meta.CurrentID)
	}
}

func TestArchiveCmd_NotFound(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"archive", "nonexistent"})
	err := root.Execute()
	if err == nil {
		t.Error("expected error archiving nonexistent item")
	}
}

func TestArchiveCmd_includesPromptDepsInArchive(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID: "pmt-arc01", Description: "Is this right?",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	_ = store.Put(promptItem)
	_ = store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, "pmt-arc01")
		return it, nil
	})

	captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"archive", parentID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute archive: %v", err)
		}
	})

	// Both items removed from store
	items, _ := store.List()
	for _, it := range items {
		if it.ID == parentID || it.ID == "pmt-arc01" {
			t.Errorf("item %s should have been removed from store", it.ID)
		}
	}

	// Archive file contains both items
	archivePath := filepath.Join(dir, ".wn", "archive", parentID+".json")
	root2 := t.TempDir()
	store2, _ := wn.NewFileStore(root2)
	if err := wn.ImportAppend(store2, archivePath); err != nil {
		t.Fatalf("ImportAppend: %v", err)
	}
	if _, err := store2.Get(parentID); err != nil {
		t.Error("parent not found in archive")
	}
	if _, err := store2.Get("pmt-arc01"); err != nil {
		t.Error("prompt dep not found in archive")
	}
}

func writeVerifySettings(t *testing.T, root, verifyCmd string) {
	t.Helper()
	wnDir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := fmt.Sprintf(`{"verify":%q}`, verifyCmd)
	if err := os.WriteFile(filepath.Join(wnDir, "settings.json"), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}
}

func TestVerifyCommand_runsConfiguredCommand(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Use a simple command that always succeeds and produces output
	writeVerifySettings(t, dir, "echo verify-ok")

	// Use a temp user config dir so only project settings apply
	configDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", configDir)

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"verify"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "verify-ok") {
		t.Errorf("verify output = %q, want to contain 'verify-ok'", out)
	}
}

func TestVerifyCommand_noVerifyConfigured_errors(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Use a temp user config dir with no settings
	configDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", configDir)

	root := newRootCmd()
	root.SetArgs([]string{"verify"})
	err := root.Execute()
	if err == nil {
		t.Error("verify with no configured command should return error")
	}
}

func TestVerifyCommand_failingCommand_errors(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	writeVerifySettings(t, dir, "false") // shell 'false' always exits 1

	configDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", configDir)

	root := newRootCmd()
	root.SetArgs([]string{"verify"})
	err := root.Execute()
	if err == nil {
		t.Error("verify with failing command should return error")
	}
}

func TestVerifyCommand_failingCommand_suppressesUsage(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	writeVerifySettings(t, dir, "false")

	configDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", configDir)

	// Cobra prints usage to root's out writer (via Println); capture it to assert it's suppressed.
	var outBuf bytes.Buffer
	root := newRootCmd()
	root.SetOut(&outBuf)
	defer root.SetOut(nil)
	root.SetArgs([]string{"verify"})
	_ = root.Execute()

	if strings.Contains(outBuf.String(), "Usage:") {
		t.Errorf("wn verify should not print usage on command failure; got: %q", outBuf.String())
	}
}

func TestVerifyCommand_printsCmdBeforeRunning(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	verifyCmd := "echo verify-output"
	writeVerifySettings(t, dir, verifyCmd)

	configDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", configDir)

	var stderrBuf bytes.Buffer
	root := newRootCmd()
	root.SetErr(&stderrBuf)
	defer root.SetErr(os.Stderr)

	captureStdout(t, func() {
		root.SetArgs([]string{"verify"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	stderrOut := stderrBuf.String()
	if !strings.Contains(stderrOut, verifyCmd) {
		t.Errorf("stderr = %q, want to contain the configured command %q", stderrOut, verifyCmd)
	}
}

func TestVerifyCommand_rootFlag_runsFromWnRoot(t *testing.T) {
	mainRoot, _ := setupWnRoot(t)
	writeVerifySettings(t, mainRoot, "pwd")

	// cd into a subdirectory so cwd != mainRoot
	subdir := filepath.Join(mainRoot, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("MkdirAll subdir: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(subdir); err != nil {
		t.Fatalf("Chdir subdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	configDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", configDir)

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"verify", "--root"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	// pwd should print mainRoot, not subdir
	if !strings.Contains(out, mainRoot) {
		t.Errorf("verify --root output = %q, want to contain main root %q", out, mainRoot)
	}
	if strings.Contains(out, subdir) {
		t.Errorf("verify --root output = %q, should not contain subdir %q", out, subdir)
	}
}

func TestSettingsShow(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// User settings file
	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"updated:desc","picker":"numbered"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	// Project settings override sort
	wnDir := filepath.Join(dir, ".wn")
	if err := os.WriteFile(filepath.Join(wnDir, "settings.json"), []byte(`{"sort":"created:asc"}`), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	root := newRootCmd()
	root.SetOut(&buf)
	defer root.SetOut(nil)
	root.SetArgs([]string{"settings", "show"})
	if err := root.Execute(); err != nil {
		t.Fatalf("settings show: %v", err)
	}

	var s wn.Settings
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &s); err != nil {
		t.Fatalf("unmarshal settings show output: %v\noutput: %s", err, buf.String())
	}
	if s.Sort != "created:asc" {
		t.Errorf("Sort = %q, want created:asc (project overrides user)", s.Sort)
	}
	if s.Picker != "numbered" {
		t.Errorf("Picker = %q, want numbered (from user, project doesn't override)", s.Picker)
	}
}

func TestSettingsEdit_projectFlag(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	t.Setenv("EDITOR", "true")
	t.Setenv("WN_SETTINGS_USER", filepath.Join(t.TempDir(), "user.json"))
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	projectSettingsPath := filepath.Join(dir, ".wn", "settings.json")
	root := newRootCmd()
	root.SetArgs([]string{"settings", "edit", "--project"})
	if err := root.Execute(); err != nil {
		t.Fatalf("settings edit --project: %v", err)
	}

	if _, err := os.Stat(projectSettingsPath); os.IsNotExist(err) {
		t.Error("project settings file was not created by settings edit --project")
	}
	data, err := os.ReadFile(projectSettingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "{}" {
		t.Errorf("project settings content = %q, want {}", strings.TrimSpace(string(data)))
	}
}

func TestSettingsEdit_userFlag(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	t.Setenv("EDITOR", "true")
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	root := newRootCmd()
	root.SetArgs([]string{"settings", "edit", "--user"})
	if err := root.Execute(); err != nil {
		t.Fatalf("settings edit --user: %v", err)
	}

	if _, err := os.Stat(userPath); os.IsNotExist(err) {
		t.Error("user settings file was not created by settings edit --user")
	}
	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "{}" {
		t.Errorf("user settings content = %q, want {}", strings.TrimSpace(string(data)))
	}
}

// setupGitWnRepo creates a temp dir with a git repo and wn store, creates an item
// with a branch note (already "merged" since the branch is at HEAD), and marks
// the item done. Returns the dir and worktree path.
func setupGitWnRepo(t *testing.T) (dir string, wtPath string) {
	t.Helper()
	dir = t.TempDir()
	gitExecIn(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, "readme"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	gitExecIn(t, dir, "add", "readme")
	gitExecIn(t, dir, "commit", "-m", "init")

	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "wttest",
		Description: "worktree test item",
		Created:     now, Updated: now,
		Tags: []string{}, DependsOn: []string{},
		Log:   []wn.LogEntry{{At: now, Kind: "created"}},
		Notes: []wn.Note{{Name: "branch", Created: now, Body: "wn-wttest-branch"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Create worktree on branch (branch is at HEAD, so already "merged")
	wtPath = filepath.Join(dir, "wt-test")
	gitExecIn(t, dir, "branch", "wn-wttest-branch", "HEAD")
	gitExecIn(t, dir, "worktree", "add", wtPath, "wn-wttest-branch")

	// Mark item done
	if err := store.UpdateItem("wttest", func(it *wn.Item) (*wn.Item, error) {
		it.Done = true
		it.DoneStatus = wn.DoneStatusDone
		return it, nil
	}); err != nil {
		t.Fatalf("UpdateItem done: %v", err)
	}

	return dir, wtPath
}

func TestCompletionZsh(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd()
	if err := root.GenZshCompletion(&buf); err != nil {
		t.Fatalf("GenZshCompletion: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "#compdef") {
		t.Errorf("zsh completion output should contain '#compdef', got:\n%.200s", out)
	}
	if !strings.Contains(out, "wn") {
		t.Errorf("zsh completion output should mention 'wn', got:\n%.200s", out)
	}
}

// TestCompletionBash verifies bash shell completion output is non-empty and well-formed.

func TestCompletionBash(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd()
	if err := root.GenBashCompletionV2(&buf, true); err != nil {
		t.Fatalf("GenBashCompletionV2: %v", err)
	}
	out := buf.String()
	if len(out) == 0 {
		t.Error("bash completion output should not be empty")
	}
	if !strings.Contains(out, "wn") {
		t.Errorf("bash completion output should mention 'wn', got:\n%.200s", out)
	}
}

// TestCompletionFish verifies fish shell completion output is non-empty and well-formed.

func TestCompletionFish(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd()
	if err := root.GenFishCompletion(&buf, true); err != nil {
		t.Fatalf("GenFishCompletion: %v", err)
	}
	out := buf.String()
	if len(out) == 0 {
		t.Error("fish completion output should not be empty")
	}
	if !strings.Contains(out, "wn") {
		t.Errorf("fish completion output should mention 'wn', got:\n%.200s", out)
	}
}

// setupGitWnRootNoItem creates a temp dir with a git repo and wn initialized.
// Returns the dir and the default branch name.
func setupGitWnRootNoItem(t *testing.T) (dir string, def string) {
	t.Helper()
	dir = t.TempDir()
	execIn(t, dir, "git", "init")
	writeFile(t, filepath.Join(dir, "readme"), "x")
	execIn(t, dir, "git", "add", "readme")
	execIn(t, dir, "git", "commit", "-m", "init")
	def, _ = wn.DefaultBranch(dir)
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	return dir, def
}

func TestWnRootCmd_NormalRepo(t *testing.T) {
	repoDir := t.TempDir()
	setupGitRepoMain(t, repoDir)

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"root"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	got := strings.TrimSpace(out)
	normGot, _ := filepath.EvalSymlinks(got)
	normWant, _ := filepath.EvalSymlinks(repoDir)
	if normGot != normWant {
		t.Errorf("wn root = %q (norm %q), want %q (norm %q)", got, normGot, repoDir, normWant)
	}
}

// TestWnRootCmd_Worktree verifies that 'wn root' prints the main repo root from a linked worktree.

func TestWnRootCmd_Worktree(t *testing.T) {
	mainRepo := t.TempDir()
	setupGitRepoMain(t, mainRepo)

	worktreeDir := t.TempDir()
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "wn-root-cmd-test")
	cmd.Dir = mainRepo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"root"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	got := strings.TrimSpace(out)
	normGot, _ := filepath.EvalSymlinks(got)
	normWant, _ := filepath.EvalSymlinks(mainRepo)
	if normGot != normWant {
		t.Errorf("wn root from worktree = %q (norm %q), want main %q (norm %q)", got, normGot, mainRepo, normWant)
	}
}
