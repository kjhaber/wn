package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

func TestListJSON(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"list", "--json"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 1 {
		t.Fatalf("len(list.Items) = %d, want 1", len(list.Items))
	}
	if list.Items[0].ID != "abc123" {
		t.Errorf("id = %q, want abc123", list.Items[0].ID)
	}
	if list.Items[0].Description != "first line\nsecond line" {
		t.Errorf("description = %q, want full description", list.Items[0].Description)
	}
}

func TestShowPlain(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--plain", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// PromptContent of "first line\nsecond line" (multi-line) returns the full description
	want := "first line\nsecond line\n"
	if out != want {
		t.Errorf("show --plain = %q, want %q", out, want)
	}
}

func TestShowPlainOneLine(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &wn.Item{ID: "aaa111", Description: "one liner task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aaa111"}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--plain"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if out != "one liner task\n" {
		t.Errorf("show --plain (one-line) = %q, want %q", out, "one liner task\n")
	}
}

func TestShowFields(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--fields", "title", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// Only title line; should contain ID and first line but not body
	if !strings.Contains(out, itemID) || !strings.Contains(out, "first line") {
		t.Errorf("show --fields=title should contain id and first line; got %q", out)
	}
	if strings.Contains(out, "second line") {
		t.Errorf("show --fields=title should not contain body; got %q", out)
	}
}

func TestShowAll(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--all", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// --all should include log entries
	if !strings.Contains(out, "log:") || !strings.Contains(out, "created") {
		t.Errorf("show --all should include log section; got %q", out)
	}
}

func TestBareWnAcceptsID(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// A second item (not current) to verify we can view it by ID
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	other := &wn.Item{ID: "zzz999", Description: "other item", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(other); err != nil {
		t.Fatal(err)
	}
	_ = itemID // current task stays as abc123

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"zzz999"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "zzz999") || !strings.Contains(out, "other item") {
		t.Errorf("bare wn <id> should show item zzz999; got %q", out)
	}
	if strings.Contains(out, "abc123") {
		t.Errorf("bare wn <id> should show zzz999, not abc123; got %q", out)
	}
}

func TestShowRespectsSettingsDefaultFields(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Write project settings that include "log" in default fields
	wnDir := filepath.Join(dir, ".wn")
	settingsPath := filepath.Join(wnDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"show":{"default_fields":"title,body,log"}}`), 0644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// settings includes log, so log section should appear
	if !strings.Contains(out, "log:") || !strings.Contains(out, "created") {
		t.Errorf("show should include log when settings.show.default_fields contains log; got %q", out)
	}
}

func TestShowOutputsFullItemJSON(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--json", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	var item wn.Item
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("Unmarshal show: %v\noutput: %s", err, out)
	}
	if item.ID != itemID {
		t.Errorf("id = %q, want %s", item.ID, itemID)
	}
	if item.Description != "first line\nsecond line" {
		t.Errorf("description = %q", item.Description)
	}
	if item.Done {
		t.Error("done = true, want false")
	}
	if len(item.Log) != 1 || item.Log[0].Kind != "created" {
		t.Errorf("log = %v", item.Log)
	}
}

func TestShowDefaultIsHumanReadable(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// Default show should be human-readable, not JSON.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("show (default) should be human-readable, not JSON; got: %s", out)
	}
	if !strings.Contains(out, itemID) || !strings.Contains(out, "first line") {
		t.Errorf("show (default) should contain item id and description text; got: %s", out)
	}
}

func TestShowCurrentWhenNoArg(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--json"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	var item wn.Item
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("Unmarshal show: %v\noutput: %s", err, out)
	}
	if item.ID != itemID {
		t.Errorf("show (no arg) id = %q, want %s (current)", item.ID, itemID)
	}
}

func TestCurrentTaskShowsDependsOnAndDependentTasks(t *testing.T) {
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
		{ID: "aa1111", Description: "current task", Created: now, Updated: now, DependsOn: []string{"bb2222"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "prerequisite", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "cc3333", Description: "follow-up", Created: now, Updated: now, DependsOn: []string{"aa1111"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aa1111"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs(nil)
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "depends on: bb2222") {
		t.Errorf("wn (current task) should show depends on; got %q", out)
	}
	if !strings.Contains(out, "dependent tasks: cc3333") {
		t.Errorf("wn (current task) should show dependent tasks; got %q", out)
	}
}

func TestShowShowsDependentTasks(t *testing.T) {
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
		{ID: "aa1111", Description: "first", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "second", Created: now, Updated: now, DependsOn: []string{"aa1111"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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
		root.SetArgs([]string{"show", "aa1111"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "dependent tasks: bb2222") {
		t.Errorf("wn show should show dependent tasks when item has dependents; got %q", out)
	}
}

func TestListJSONEmpty(t *testing.T) {
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
		root.SetArgs([]string{"list", "--json"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 0 {
		t.Errorf("len(list.Items) = %d, want 0", len(list.Items))
	}
}

func TestListJSONRespectsDoneFilter(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "done1", Description: "done", Created: now, Updated: now, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--json", "--done"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 1 || list.Items[0].ID != "done1" {
		t.Errorf("list --done --json = %d items (ids %v), want single item done1", len(list.Items), itemIDs(list.Items))
	}
}

func TestListUndoneIncludesReviewReady(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "rr1", Description: "review-ready", Created: now, Updated: now, ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--json", "--undone"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	list := parseListJSON(t, out)
	if len(list.Items) != 2 {
		t.Errorf("list --undone --json = %d items (want 2: undone and review-ready); ids %v", len(list.Items), itemIDs(list.Items))
	}
	byID := make(map[string]*wn.Item)
	for _, it := range list.Items {
		byID[it.ID] = it
	}
	if byID["undone1"] == nil || byID["rr1"] == nil {
		t.Errorf("list --undone --json want both undone1 and rr1; got %v", itemIDs(list.Items))
	}
	if byID["undone1"] != nil && (byID["undone1"].Done || byID["undone1"].ReviewReady) {
		t.Errorf("undone1 should be undone and not review-ready; got done=%v review_ready=%v", byID["undone1"].Done, byID["undone1"].ReviewReady)
	}
	if byID["rr1"] != nil && (byID["rr1"].Done || !byID["rr1"].ReviewReady) {
		t.Errorf("rr1 should be review-ready and not done; got done=%v review_ready=%v", byID["rr1"].Done, byID["rr1"].ReviewReady)
	}
}

func TestPickWithID_SetsCurrent(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: ""}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", itemID})
	if err := root.Execute(); err != nil {
		t.Fatalf("pick %s: %v", itemID, err)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != itemID {
		t.Errorf("after pick %s: CurrentID = %q, want %q", itemID, meta.CurrentID, itemID)
	}
}

func TestPickWithID_NotFound(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "badid"})
	err := root.Execute()
	if err == nil {
		t.Fatal("pick badid: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("pick badid: error = %v, want containing \"not found\"", err)
	}
}

func TestPickWithDoneFlag(t *testing.T) {
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if _, err := w.WriteString("1\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	doneItem := &wn.Item{ID: "done1", Description: "done task", Done: true, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(doneItem); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "--done"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pick --done: %v", err)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "done1" {
		t.Errorf("after pick --done and choose 1: CurrentID = %q, want done1", meta.CurrentID)
	}
}

func TestPickWithAllFlag(t *testing.T) {
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if _, err := w.WriteString("2\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

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
		{ID: "done1", Description: "done", Created: now, Updated: now, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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
	root.SetArgs([]string{"pick", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pick --all: %v", err)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	// Order with --all is from store.List() then ApplySort; we chose "2" so we get second item (id depends on sort)
	if meta.CurrentID != "done1" && meta.CurrentID != "undone1" {
		t.Errorf("after pick --all and choose 2: CurrentID = %q, want done1 or undone1", meta.CurrentID)
	}
}

func TestPickWithReviewReadyFlag(t *testing.T) {
	for _, flag := range []string{"--rr", "--review-ready"} {
		t.Run(flag, func(t *testing.T) {
			origPath := os.Getenv("PATH")
			_ = os.Setenv("PATH", "")
			t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			origStdin := os.Stdin
			os.Stdin = r
			t.Cleanup(func() { os.Stdin = origStdin })
			if _, err := w.WriteString("1\n"); err != nil {
				t.Fatal(err)
			}
			_ = w.Close()

			dir := t.TempDir()
			if err := wn.InitRoot(dir); err != nil {
				t.Fatalf("InitRoot: %v", err)
			}
			store, err := wn.NewFileStore(dir)
			if err != nil {
				t.Fatalf("NewFileStore: %v", err)
			}
			now := time.Now().UTC()
			rrItem := &wn.Item{ID: "rr1111", Description: "review-ready task", Created: now, Updated: now, ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
			if err := store.Put(rrItem); err != nil {
				t.Fatal(err)
			}
			cwd, _ := os.Getwd()
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("Chdir: %v", err)
			}
			defer func() { _ = os.Chdir(cwd) }()

			root := newRootCmd()
			root.SetArgs([]string{"pick", flag})
			if err := root.Execute(); err != nil {
				t.Fatalf("pick %s: %v", flag, err)
			}
			meta, err := wn.ReadMeta(dir)
			if err != nil {
				t.Fatalf("ReadMeta: %v", err)
			}
			if meta.CurrentID != "rr1111" {
				t.Errorf("after pick %s and choose 1: CurrentID = %q, want rr1111", flag, meta.CurrentID)
			}
		})
	}
}

func TestPickDefaultIsUndone(t *testing.T) {
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if _, err := w.WriteString("1\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

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
		{ID: "done1", Description: "done", Created: now, Updated: now, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "rr1", Description: "review-ready", Created: now, Updated: now, ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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
	root.SetArgs([]string{"pick"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pick (default): %v", err)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	// Default is undone only (excludes done and review-ready), so only one choice
	if meta.CurrentID != "undone1" {
		t.Errorf("after pick with no flag: CurrentID = %q, want undone1 (default filter is undone)", meta.CurrentID)
	}
}

func TestPickStateFlagsMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	for _, args := range [][]string{
		{"pick", "--done", "--all"},
		{"pick", "--undone", "--all"},
		{"pick", "--done", "--rr"},
	} {
		root := newRootCmd()
		root.SetArgs(args)
		err := root.Execute()
		if err == nil {
			t.Errorf("pick %v: expected error (only one state flag allowed), got nil", args)
		}
		if err != nil && !strings.Contains(err.Error(), "one of") {
			t.Errorf("pick %v: error = %v, want message containing \"one of\"", args, err)
		}
	}
}

func TestPickWithPickerNumberedFlag(t *testing.T) {
	// --picker numbered forces numbered list even when fzf is in PATH
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = wn.SetPickerMode("")
	})
	if _, err := w.WriteString("1\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Put(&wn.Item{ID: "task1", Description: "task one", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "--picker", "numbered"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pick --picker numbered: %v", err)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "task1" {
		t.Errorf("after pick --picker numbered: CurrentID = %q, want task1", meta.CurrentID)
	}
}

func TestPickWithPickerInvalidFlag(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
		_ = wn.SetPickerMode("")
	}()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "--picker", "invalid"})
	err := root.Execute()
	if err == nil {
		t.Fatal("pick --picker invalid: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid picker mode") {
		t.Errorf("pick --picker invalid: error = %v, want containing \"invalid picker mode\"", err)
	}
}

func TestExportWithCriteria(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "aaa111", Description: "tagged", Created: now, Updated: now, Tags: []string{"prio"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "untagged", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := dir + "/out.json"
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"export", "--tag", "prio", "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --tag prio: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var exp struct {
		Version int        `json:"version"`
		Items   []*wn.Item `json:"items"`
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(exp.Items) != 1 || exp.Items[0].ID != "aaa111" {
		t.Errorf("export --tag prio: got %d items (ids %v), want 1 [aaa111]", len(exp.Items), itemIDs(exp.Items))
	}
}

func itemIDs(items []*wn.Item) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

func TestExport_ReviewReady(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	items := []*wn.Item{
		{ID: "aaa111", Description: "available", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "review-ready", Created: now, Updated: now, ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	}
	for _, item := range items {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := dir + "/out.json"
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"export", "--review-ready", "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --review-ready: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var exp struct {
		Items []*wn.Item `json:"items"`
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(exp.Items) != 1 || exp.Items[0].ID != "bbb222" {
		t.Errorf("export --review-ready: got %v, want [bbb222]", itemIDs(exp.Items))
	}
}

func TestExport_CompoundTag(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	items := []*wn.Item{
		{ID: "aaa111", Description: "both tags", Created: now, Updated: now, Tags: []string{"a", "b"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "one tag", Created: now, Updated: now, Tags: []string{"a"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc333", Description: "no tags", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	}
	for _, item := range items {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := dir + "/out.json"
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// AND filter: only items with both "a" and "b"
	root := newRootCmd()
	root.SetArgs([]string{"export", "--tag", "a,b", "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --tag a,b: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var exp struct {
		Items []*wn.Item `json:"items"`
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(exp.Items) != 1 || exp.Items[0].ID != "aaa111" {
		t.Errorf("export --tag a,b: got %v, want [aaa111]", itemIDs(exp.Items))
	}
}

func TestExport_Sort(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	items := []*wn.Item{
		{ID: "aaa111", Description: "alpha first", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "alpha second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	}
	for _, item := range items {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := dir + "/out.json"
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Sort alphabetically descending — should put "alpha second" before "alpha first"
	root := newRootCmd()
	root.SetArgs([]string{"export", "--all", "--sort", "alpha:desc", "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --sort alpha:desc: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var exp struct {
		Items []*wn.Item `json:"items"`
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(exp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(exp.Items))
	}
	if exp.Items[0].ID != "bbb222" || exp.Items[1].ID != "aaa111" {
		t.Errorf("export --sort alpha:desc: got order %v, want [bbb222, aaa111]", itemIDs(exp.Items))
	}
}

func TestExport_LimitOffset(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	ord0, ord1, ord2 := 0, 1, 2
	items := []*wn.Item{
		{ID: "aaa111", Description: "first", Created: now, Updated: now, Order: &ord0, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "second", Created: now, Updated: now, Order: &ord1, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc333", Description: "third", Created: now, Updated: now, Order: &ord2, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	}
	for _, item := range items {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := dir + "/out.json"
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// --limit 2: should return first 2 items
	root := newRootCmd()
	root.SetArgs([]string{"export", "--all", "--sort", "priority", "--limit", "2", "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --limit 2: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var exp struct {
		Items []*wn.Item `json:"items"`
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(exp.Items) != 2 {
		t.Fatalf("export --limit 2: got %d items, want 2", len(exp.Items))
	}
	if exp.Items[0].ID != "aaa111" || exp.Items[1].ID != "bbb222" {
		t.Errorf("export --limit 2: got %v, want [aaa111, bbb222]", itemIDs(exp.Items))
	}

	// --offset 1: should skip first item
	root = newRootCmd()
	root.SetArgs([]string{"export", "--all", "--sort", "priority", "--offset", "1", "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --offset 1: %v", err)
	}
	data, err = os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(exp.Items) != 2 {
		t.Fatalf("export --offset 1: got %d items, want 2", len(exp.Items))
	}
	if exp.Items[0].ID != "bbb222" || exp.Items[1].ID != "ccc333" {
		t.Errorf("export --offset 1: got %v, want [bbb222, ccc333]", itemIDs(exp.Items))
	}
}

func TestExport_MultipleStateFlags_Errors(t *testing.T) {
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
	root.SetArgs([]string{"export", "--undone", "--done"})
	if err := root.Execute(); err == nil {
		t.Error("export --undone --done: expected error, got nil")
	}
}

func TestImport_StoreHasItemsNoFlagErrors(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Put(&wn.Item{ID: "abc123", Description: "existing", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	path := dir + "/export.json"
	if err := wn.Export(store, path); err != nil {
		t.Fatalf("Export: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	root := newRootCmd()
	root.SetArgs([]string{"import", path})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error when store has items and no --append/--replace")
	}
	if !strings.Contains(err.Error(), "--append") || !strings.Contains(err.Error(), "--replace") {
		t.Errorf("error should mention --append and --replace: %v", err)
	}
}

func TestImport_Replace(t *testing.T) {
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
		{ID: "aaa111", Description: "first", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	path := dir + "/export.json"
	if err := wn.Export(store, path); err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Add another item so store has 3
	if err := store.Put(&wn.Item{ID: "ccc333", Description: "third", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	root := newRootCmd()
	root.SetArgs([]string{"import", "--replace", path})
	if err := root.Execute(); err != nil {
		t.Fatalf("import --replace: %v", err)
	}
	store2, _ := wn.NewFileStore(dir)
	all, err := store2.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("after --replace: len(List) = %d, want 2 (file had 2 items)", len(all))
	}
}

func TestImport_Append(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Put(&wn.Item{ID: "old111", Description: "existing", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	path := dir + "/new.json"
	if err := wn.ExportItems([]*wn.Item{
		{ID: "new222", Description: "from file", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	root := newRootCmd()
	root.SetArgs([]string{"import", "--append", path})
	if err := root.Execute(); err != nil {
		t.Fatalf("import --append: %v", err)
	}
	store2, _ := wn.NewFileStore(dir)
	all, err := store2.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("after --append: len(List) = %d, want 2", len(all))
	}
	got, _ := store2.Get("new222")
	if got.Description != "from file" {
		t.Errorf("new222 description = %q, want from file", got.Description)
	}
}

func TestImport_BothAppendAndReplaceErrors(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	path := dir + "/export.json"
	if err := wn.ExportItems(nil, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	root := newRootCmd()
	root.SetArgs([]string{"import", "--append", "--replace", path})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when both --append and --replace")
	}
	if !strings.Contains(err.Error(), "append") || !strings.Contains(err.Error(), "replace") {
		t.Errorf("error should mention append and replace: %v", err)
	}
}

func TestImport_EmptyStoreNoFlagSucceeds(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	path := dir + "/export.json"
	if err := wn.ExportItems([]*wn.Item{
		{ID: "only1", Description: "only item", Created: time.Now().UTC(), Updated: time.Now().UTC(), Log: []wn.LogEntry{{At: time.Now().UTC(), Kind: "created"}}},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	root := newRootCmd()
	root.SetArgs([]string{"import", path})
	if err := root.Execute(); err != nil {
		t.Fatalf("import into empty store: %v", err)
	}
	store, _ := wn.NewFileStore(dir)
	got, err := store.Get("only1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "only item" {
		t.Errorf("description = %q, want only item", got.Description)
	}
}

func TestListShowsStatusWithAlignment(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "done11", Description: "done task", Created: now, Updated: now, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "undone1", Description: "undone task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--all"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("list --all should show at least 2 lines; got %q", out)
	}
	// Status column shows done/undone (and claimed when applicable); include default undone.
	if !strings.Contains(out, "done") {
		t.Errorf("list output should contain status 'done'; got %q", out)
	}
	if !strings.Contains(out, "undone") {
		t.Errorf("list output should contain status 'undone'; got %q", out)
	}
	// Descriptions aligned: each line has status then "  " then description at fixed column.
	for i, line := range lines {
		if len(line) < listDescriptionStart {
			t.Errorf("line %d too short for aligned format: %q", i, line)
			continue
		}
		if line[listDescriptionStart-2:listDescriptionStart] != "  " {
			t.Errorf("line %d: expected two spaces before description at col %d; got %q", i, listDescriptionStart, line[listDescriptionStart-2:listDescriptionStart])
		}
	}
}

// TestCurrentTaskShowsState verifies that running "wn" (no args) prints the current task's state: done, undone, or claimed.

func TestCurrentTaskShowsState(t *testing.T) {
	t.Run("undone", func(t *testing.T) {
		dir, _ := setupWnRoot(t)
		cwd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		defer func() { _ = os.Chdir(cwd) }()

		out := captureStdout(t, func() {
			root := newRootCmd()
			root.SetArgs(nil)
			if err := root.Execute(); err != nil {
				t.Errorf("Execute: %v", err)
			}
		})
		// Undone is default: no state suffix
		if bytes.Contains([]byte(out), []byte("(undone)")) {
			t.Errorf("current task output should not show (undone); got %q", out)
		}
		if !bytes.Contains([]byte(out), []byte("abc123")) {
			t.Errorf("current task output should show item id; got %q", out)
		}
	})

	t.Run("done", func(t *testing.T) {
		dir, _ := setupWnRoot(t)
		cwd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		defer func() { _ = os.Chdir(cwd) }()

		root := newRootCmd()
		root.SetArgs([]string{"done"})
		if err := root.Execute(); err != nil {
			t.Fatalf("wn done: %v", err)
		}

		out := captureStdout(t, func() {
			root := newRootCmd()
			root.SetArgs(nil)
			if err := root.Execute(); err != nil {
				t.Errorf("Execute: %v", err)
			}
		})
		if !bytes.Contains([]byte(out), []byte("done")) {
			t.Errorf("current task output should contain state 'done'; got %q", out)
		}
	})

	t.Run("claimed", func(t *testing.T) {
		dir, _ := setupWnRoot(t)
		cwd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		defer func() { _ = os.Chdir(cwd) }()

		root := newRootCmd()
		root.SetArgs([]string{"claim", "--for", "1h"})
		if err := root.Execute(); err != nil {
			t.Fatalf("wn claim: %v", err)
		}

		out := captureStdout(t, func() {
			root := newRootCmd()
			root.SetArgs(nil)
			if err := root.Execute(); err != nil {
				t.Errorf("Execute: %v", err)
			}
		})
		if !bytes.Contains([]byte(out), []byte("claimed")) {
			t.Errorf("current task output should contain state 'claimed'; got %q", out)
		}
	})
}

func TestNextWithClaim(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"next", "--claim", "30m"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn next --claim: %v", err)
		}
	})
	if !strings.Contains(out, itemID) || !strings.Contains(out, "claimed") {
		t.Errorf("wn next --claim output should contain id and claimed; got %q", out)
	}
	// Verify item is actually claimed: show --json should have in_progress_until set
	showOut := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--json", itemID})
		_ = root.Execute()
	})
	if !strings.Contains(showOut, "in_progress_until") || strings.Contains(showOut, "\"in_progress_until\":\"0001-01-01T00:00:00Z\"") {
		t.Errorf("wn show after next --claim should show in_progress_until; got %s", showOut)
	}
}

func TestNextWithTag(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	ord0, ord1 := 0, 1
	for _, it := range []*wn.Item{
		{ID: "aa1111", Description: "no tag", Created: now, Updated: now, Order: &ord0, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "has agent tag", Created: now, Updated: now, Order: &ord1, Tags: []string{"agent"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aa1111"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn next --tag agent should set current to bb2222
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"next", "--tag", "agent"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn next --tag agent: %v", err)
		}
	})
	if !strings.Contains(out, "bb2222") || !strings.Contains(out, "has agent tag") {
		t.Errorf("wn next --tag agent should output bb2222 and description; got %q", out)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "bb2222" {
		t.Errorf("after wn next --tag agent: CurrentID = %q, want bb2222", meta.CurrentID)
	}

	// wn next --tag nonexistent should print "No next task."
	out2 := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"next", "--tag", "nonexistent"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn next --tag nonexistent: %v", err)
		}
	})
	if !strings.Contains(out2, "No next task.") {
		t.Errorf("wn next --tag nonexistent should print No next task.; got %q", out2)
	}
}

// TestDoneNext_oneItem verifies that "wn done --next" with only one (current) item prints "No next task."

func TestDoneNext_oneItem(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"done", "--next"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn done --next: %v", err)
		}
	})
	if !strings.Contains(out, "No next task.") {
		t.Errorf("wn done --next with one item should print 'No next task.'; got %q", out)
	}
}

// TestDoneNext_twoItems verifies that "wn done --next" marks current done and sets next undone as current.

func TestDoneNext_twoItems(t *testing.T) {
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
		{ID: "abc123", Description: "first task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "def456", Description: "second task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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
		root := newRootCmd()
		root.SetArgs([]string{"done", "--next"})
		if err := root.Execute(); err != nil {
			t.Errorf("wn done --next: %v", err)
		}
	})
	if !strings.Contains(out, "def456") || !strings.Contains(out, "second task") {
		t.Errorf("wn done --next with two items should print next item id and description; got %q", out)
	}
	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "def456" {
		t.Errorf("after done --next: CurrentID = %q, want def456", meta.CurrentID)
	}
}

// TestStatus_closed_duplicate_of verifies that "wn status closed [id] --duplicate-of <id2>" adds the standard duplicate-of note and marks the item closed.

func TestCurrentTaskShowsTags(t *testing.T) {
	dir := t.TempDir()
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
		Description: "first line",
		Created:     now,
		Updated:     now,
		Tags:        []string{"urgent", "backend"},
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
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
		root := newRootCmd()
		root.SetArgs(nil)
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "urgent") || !strings.Contains(out, "backend") {
		t.Errorf("current task output should show tags on the right; got %q", out)
	}
	// First line should contain both description and tags
	firstLine := strings.Split(out, "\n")[0]
	if !strings.Contains(firstLine, "first line") || !strings.Contains(firstLine, "urgent") {
		t.Errorf("first line should contain description and tags; got %q", firstLine)
	}
}

func TestListShowsTags(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "xyz789",
		Description: "task with tags",
		Created:     now,
		Updated:     now,
		Tags:        []string{"foo", "bar"},
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"list", "--all"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
		t.Errorf("list output should show tags on the right; got %q", out)
	}
	// Tags should appear on the same line as the item (right of description)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line; got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "task with tags") || !strings.Contains(lines[0], "foo") {
		t.Errorf("line should contain description and tags; got %q", lines[0])
	}
}

func TestListSortFlag(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "bbb", Description: "second alpha", Created: now.Add(1 * time.Hour), Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "aaa", Description: "first alpha", Created: now, Updated: now.Add(1 * time.Hour), Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--json", "--sort", "alpha"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 2 {
		t.Fatalf("len(list.Items) = %d, want 2", len(list.Items))
	}
	// alpha asc: first alpha (aaa) then second alpha (bbb)
	if list.Items[0].ID != "aaa" || list.Items[1].ID != "bbb" {
		t.Errorf("list --sort alpha = %v, %v; want aaa then bbb", list.Items[0].ID, list.Items[1].ID)
	}

	out2 := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"list", "--json", "--sort", "updated:desc"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list2 := parseListJSON(t, out2)
	// updated desc: aaa (Updated: now+1h) then bbb (Updated: now)
	if list2.Items[0].ID != "aaa" || list2.Items[1].ID != "bbb" {
		t.Errorf("list --sort updated:desc = %v, %v; want aaa then bbb", list2.Items[0].ID, list2.Items[1].ID)
	}
}

func TestListLimit(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "aaa", Description: "first", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc", Description: "third", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--json", "--sort", "alpha", "--limit", "2"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 2 {
		t.Fatalf("list --limit 2: len = %d, want 2", len(list.Items))
	}
	if list.Items[0].ID != "aaa" || list.Items[1].ID != "bbb" {
		t.Errorf("list --limit 2 = %v, %v; want aaa, bbb", list.Items[0].ID, list.Items[1].ID)
	}
}

func TestListLimitOffset(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "aaa", Description: "first", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc", Description: "third", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--json", "--sort", "alpha", "--limit", "1", "--offset", "1"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 1 {
		t.Fatalf("list --limit 1 --offset 1: len = %d, want 1", len(list.Items))
	}
	if list.Items[0].ID != "bbb" {
		t.Errorf("list --limit 1 --offset 1 = %v; want bbb", list.Items[0].ID)
	}
}

func TestListGroupByTags(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "aaa111", Description: "agent task", Created: now, Updated: now, Tags: []string{"agent"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "backend task", Created: now, Updated: now, Tags: []string{"backend"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc333", Description: "another agent task", Created: now, Updated: now, Tags: []string{"agent"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--group", "tags"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	// Should have group headers for each tag set
	if !strings.Contains(out, "#agent") {
		t.Errorf("output should contain #agent group header; got:\n%s", out)
	}
	if !strings.Contains(out, "#backend") {
		t.Errorf("output should contain #backend group header; got:\n%s", out)
	}
	// Items in the same group (agent) should be adjacent — header appears once before both agent items
	agentHeaderIdx := strings.Index(out, "--- #agent ---")
	if agentHeaderIdx < 0 {
		t.Fatalf("expected '--- #agent ---' header; got:\n%s", out)
	}
	backendHeaderIdx := strings.Index(out, "--- #backend ---")
	if backendHeaderIdx < 0 {
		t.Fatalf("expected '--- #backend ---' header; got:\n%s", out)
	}
	// Both agent items should appear before the backend header
	aaa111Idx := strings.Index(out, "aaa111")
	ccc333Idx := strings.Index(out, "ccc333")
	bbb222Idx := strings.Index(out, "bbb222")
	if aaa111Idx < 0 || ccc333Idx < 0 || bbb222Idx < 0 {
		t.Fatalf("all item ids should appear in output; got:\n%s", out)
	}
	if aaa111Idx > backendHeaderIdx || ccc333Idx > backendHeaderIdx {
		t.Errorf("agent items should appear before backend header; got:\n%s", out)
	}
	if bbb222Idx < backendHeaderIdx {
		t.Errorf("backend item should appear after backend header; got:\n%s", out)
	}
}

func TestListGroupByTagsNoTagsGroup(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "aaa111", Description: "tagged task", Created: now, Updated: now, Tags: []string{"agent"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "untagged task", Created: now, Updated: now, Tags: nil, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--group", "tags"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "(no tags)") {
		t.Errorf("output should contain '(no tags)' group header for untagged items; got:\n%s", out)
	}
	if !strings.Contains(out, "#agent") {
		t.Errorf("output should contain '#agent' group header; got:\n%s", out)
	}
	if !strings.Contains(out, "bbb222") {
		t.Errorf("output should contain untagged item id; got:\n%s", out)
	}
}

func TestListGroupByStatus(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*wn.Item{
		{ID: "aaa111", Description: "done task", Created: now, Updated: now, Done: true, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "undone task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ccc333", Description: "another undone task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
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
		root.SetArgs([]string{"list", "--all", "--group", "status"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	// Group headers for done and undone
	if !strings.Contains(out, "--- done ---") {
		t.Errorf("output should contain '--- done ---' header; got:\n%s", out)
	}
	if !strings.Contains(out, "--- undone ---") {
		t.Errorf("output should contain '--- undone ---' header; got:\n%s", out)
	}
	// The two undone items should both appear after the undone header
	undoneHeaderIdx := strings.Index(out, "--- undone ---")
	bbb222Idx := strings.Index(out, "bbb222")
	ccc333Idx := strings.Index(out, "ccc333")
	if bbb222Idx < undoneHeaderIdx || ccc333Idx < undoneHeaderIdx {
		t.Errorf("undone items should appear after '--- undone ---' header; got:\n%s", out)
	}
}

func TestListGroupInvalidKey(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"list", "--group", "bogus"})
	if err := root.Execute(); err == nil {
		t.Error("list --group bogus should return an error")
	}
}

func TestListGroupWithJSON(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"list", "--group", "tags", "--json"})
	if err := root.Execute(); err == nil {
		t.Error("list --group with --json should return an error")
	}
}

// writeViewSettings writes a project-level settings file with named views.
func writeViewSettings(t *testing.T, root string, views map[string]string) {
	t.Helper()
	wnDir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body, err := json.Marshal(map[string]interface{}{"views": views})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wnDir, "settings.json"), body, 0644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}
}

func TestListViewAtSyntax_filtersByTag(t *testing.T) {
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
		{ID: "aaa111", Description: "agent item", Tags: []string{"agent"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "other item", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	writeViewSettings(t, dir, map[string]string{"agent": "--tag agent --json"})
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"list", "@agent"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1 (only tagged 'agent')", len(list.Items))
	}
	if list.Items[0].ID != "aaa111" {
		t.Errorf("Items[0].ID = %q, want aaa111", list.Items[0].ID)
	}
}

func TestListViewAtSyntax_unknownView(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	writeViewSettings(t, dir, map[string]string{"agent": "--tag agent"})
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"list", "@nosuchview"})
	if err := root.Execute(); err == nil {
		t.Error("list @nosuchview should return an error")
	}
}

func TestListViewAtSyntax_withSortAndGroup(t *testing.T) {
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
		{ID: "ccc333", Description: "cc item", Tags: []string{"backend"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "ddd444", Description: "dd item", Tags: []string{"frontend"}, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "eee555", Description: "ee item", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	writeViewSettings(t, dir, map[string]string{"bygroup": "--all --group tags"})
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"list", "@bygroup"})
		if err := root.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// With --group tags, output should contain section headers
	if !strings.Contains(out, "---") {
		t.Errorf("output should contain group headers, got: %s", out)
	}
}

func TestListViewAtSyntax_noViews(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	// Isolate from user settings so no views are configured.
	t.Setenv("WN_CONFIG_DIR", t.TempDir())
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"list", "@agent"})
	if err := root.Execute(); err == nil {
		t.Error("list @agent with no views configured should return an error")
	}
}

func TestShowIncludesNotes(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"note", "add", "see-file", itemID, "-m", "see file X"})
	if err := root.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--json", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("show: %v", err)
		}
	})
	var item wn.Item
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("Unmarshal show: %v", err)
	}
	if len(item.Notes) != 1 || item.Notes[0].Name != "see-file" || item.Notes[0].Body != "see file X" {
		t.Errorf("show should include notes with name; got Notes = %v", item.Notes)
	}
}

func TestPickDash_selectsPreviousItem(t *testing.T) {
	dir, _ := setupWnRoot(t)
	// Add a second item
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Put(&wn.Item{ID: "def456", Description: "second item", Created: now, Updated: now}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Start with abc123 as current (set by setupWnRoot), pick def456 to establish previous
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "def456"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pick def456: %v", err)
	}

	// Now pick - should return to abc123
	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"pick", "-"})
		if err := root.Execute(); err != nil {
			t.Fatalf("pick -: %v", err)
		}
	})

	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "abc123" {
		t.Errorf("CurrentID = %q, want abc123", meta.CurrentID)
	}
	if meta.PreviousID != "def456" {
		t.Errorf("PreviousID = %q, want def456 (swapped)", meta.PreviousID)
	}
	if !strings.Contains(out, "abc123") {
		t.Errorf("output %q does not contain abc123", out)
	}
}

func TestPickDash_noPreviousItem(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "-"})
	err := root.Execute()
	if err == nil {
		t.Error("wn pick - with no previous item should error")
	}
	if !strings.Contains(err.Error(), "no previous") {
		t.Errorf("want 'no previous' error; got: %v", err)
	}
}

func gitExecIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPickDot_selectsItemForCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	gitExecIn(t, dir, "init")
	if out, err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").CombinedOutput(); err != nil {
		t.Skipf("git commit failed (no git config?): %s", out)
	}

	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	branchName := "keith/wn-abc123-fix-the-thing"
	item := &wn.Item{
		ID:          "abc123",
		Description: "Fix the thing",
		Created:     now,
		Updated:     now,
		Notes:       []wn.Note{{Name: "branch", Body: branchName, Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}
	gitExecIn(t, dir, "checkout", "-b", branchName)

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"pick", "."})
		if err := root.Execute(); err != nil {
			t.Fatalf("pick .: %v", err)
		}
	})

	meta, err := wn.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "abc123" {
		t.Errorf("CurrentID = %q, want abc123", meta.CurrentID)
	}
	if !strings.Contains(out, "abc123") {
		t.Errorf("output %q does not contain abc123", out)
	}
}

func TestPickDot_noMatchingItem(t *testing.T) {
	dir := t.TempDir()
	gitExecIn(t, dir, "init")
	if out, err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").CombinedOutput(); err != nil {
		t.Skipf("git commit failed (no git config?): %s", out)
	}

	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	root := newRootCmd()
	root.SetArgs([]string{"pick", "."})
	err := root.Execute()
	if err == nil {
		t.Error("wn pick . with no matching item should error")
	}
	if !strings.Contains(err.Error(), "no work item") {
		t.Errorf("want 'no work item' error; got: %v", err)
	}
}

func TestShowMetadataSection(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Add a special wn: note and a regular note
	root := newRootCmd()
	root.SetArgs([]string{"note", "add", "wn:branch", itemID, "-m", "feat/my-branch"})
	if err := root.Execute(); err != nil {
		t.Fatalf("note add wn:branch: %v", err)
	}
	root = newRootCmd()
	root.SetArgs([]string{"note", "add", "pr-url", itemID, "-m", "https://example.com/pr/1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("note add pr-url: %v", err)
	}

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"show", "--fields", "notes", itemID})
		if err := root.Execute(); err != nil {
			t.Errorf("show: %v", err)
		}
	})

	if !strings.Contains(out, "metadata:") {
		t.Errorf("show output should have 'metadata:' section for wn: notes; got:\n%s", out)
	}
	if !strings.Contains(out, "wn:branch") {
		t.Errorf("show output should include wn:branch in metadata section; got:\n%s", out)
	}
	if !strings.Contains(out, "notes:") {
		t.Errorf("show output should have 'notes:' section for regular notes; got:\n%s", out)
	}
	if !strings.Contains(out, "pr-url") {
		t.Errorf("show output should include pr-url in notes section; got:\n%s", out)
	}
	// wn: notes should NOT appear in the regular notes section
	metadataIdx := strings.Index(out, "metadata:")
	notesIdx := strings.Index(out, "notes:")
	if metadataIdx >= 0 && notesIdx >= 0 && notesIdx < metadataIdx {
		t.Errorf("metadata: section should come before notes: section")
	}
}

// TestCompletionZsh verifies zsh shell completion output is non-empty and well-formed.
// Uses cobra's direct generation API to avoid output-writer state issues from sequential Execute() calls.
