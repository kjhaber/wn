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

	"github.com/keith/wn/internal/wn"
)

// resetPickFlags clears pick filter flags to avoid Cobra's flag persistence across
// Execute() calls (see https://github.com/spf13/cobra/issues/2079). Call before
// each test that invokes "pick" with different flags.
func resetPickFlags() {
	pickUndone = false
	pickDone = false
	pickAll = false
	pickReviewReady = false
}

// resetListFlags clears list flags to avoid Cobra's flag persistence across
// Execute() calls (see https://github.com/spf13/cobra/issues/2079). Call before
// each test that invokes "list" with different flags.
func resetListFlags() {
	listUndone = false
	listDone = false
	listAll = false
	listReviewReady = false
	listTag = ""
	listSort = ""
	listLimit = 0
	listOffset = 0
	listJson = false
}

// listStatusWidth and listIDWidth must match runList formatting for alignment tests.
const listStatusWidth = 7
const listIDWidth = 6
const listDescriptionStart = 2 + listIDWidth + 2 + listStatusWidth + 2 // "  "+id+"  "+status+"  "

// listExportShape is the JSON shape of "wn list --json" (same as "wn export").
type listExportShape struct {
	Version    int        `json:"version"`
	ExportedAt time.Time  `json:"exported_at"`
	Items      []*wn.Item `json:"items"`
}

func parseListJSON(t *testing.T, out string) listExportShape {
	t.Helper()
	var list listExportShape
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("Unmarshal list: %v\noutput: %s", err, out)
	}
	return list
}

// setupWnRoot creates a temp dir with .wn and one undone item; returns the dir and item id.
// Caller must chdir to dir before running commands and restore cwd in defer.
func setupWnRoot(t *testing.T) (dir string, itemID string) {
	t.Helper()
	dir = t.TempDir()
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
		Description: "first line\nsecond line",
		Created:     now,
		Updated:     now,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "abc123"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	return dir, "abc123"
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestListJSON(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"list", "--json"})
		if err := rootCmd.Execute(); err != nil {
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

func TestDescJSON(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"desc", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	var doc struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("Unmarshal desc: %v\noutput: %s", err, out)
	}
	// PromptBody of "first line\nsecond line" is "second line"
	if doc.Description != "second line" {
		t.Errorf("description = %q, want second line", doc.Description)
	}
}

func TestPromptMultiLineUsesTemplateBody(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompt", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// Default template-body is "Please implement the following:\n\n{}"; item has "first line\nsecond line"
	want := "Please implement the following:\n\nfirst line\nsecond line"
	if out != want+"\n" {
		t.Errorf("prompt (multi-line) = %q, want %q", out, want+"\n")
	}
}

func TestPromptOneLineUsesTemplate(t *testing.T) {
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	// Add a one-line item and set as current
	rootCmd.SetArgs([]string{"add", "-m", "add a prompt template feature"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompt"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	want := "Please implement the following work item: add a prompt template feature\n"
	if out != want {
		t.Errorf("prompt (one-line) = %q, want %q", out, want)
	}
}

func TestPromptCustomTemplate(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompt", "--template-body", "Task: {}", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	want := "Task: first line\nsecond line"
	if out != want+"\n" {
		t.Errorf("prompt (custom template-body) = %q, want %q", out, want+"\n")
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
		rootCmd.SetArgs([]string{"show", "--json", itemID})
		if err := rootCmd.Execute(); err != nil {
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

	// Cobra does not reset flags between Execute() calls; reset so default is human-readable.
	_ = showCmd.Flags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// Default show should be human-readable, not JSON.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("show (default) should be human-readable, not JSON; got: %s", out)
	}
	if !strings.Contains(out, "id:") || !strings.Contains(out, "description:") {
		t.Errorf("show (default) should contain id: and description:; got: %s", out)
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
		rootCmd.SetArgs([]string{"show", "--json"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs(nil)
		if err := rootCmd.Execute(); err != nil {
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

	// Cobra does not reset flags between Execute() calls; reset so default is human-readable.
	_ = showCmd.Flags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "aa1111"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"list", "--json"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"list", "--json", "--done"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list := parseListJSON(t, out)
	if len(list.Items) != 1 || list.Items[0].ID != "done1" {
		t.Errorf("list --done --json = %d items (ids %v), want single item done1", len(list.Items), itemIDs(list.Items))
	}
}

func TestListUndoneExcludesReviewReady(t *testing.T) {
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

	resetListFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"list", "--json", "--undone"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})

	list := parseListJSON(t, out)
	if len(list.Items) != 1 || list.Items[0].ID != "undone1" {
		t.Errorf("list --undone --json = %v, want single item undone1 (review-ready must not appear)", list.Items)
	}
	if list.Items[0].Done || list.Items[0].ReviewReady {
		t.Errorf("list --undone item should be undone and not review-ready; got done=%v review_ready=%v", list.Items[0].Done, list.Items[0].ReviewReady)
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

	rootCmd.SetArgs([]string{"pick", itemID})
	if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"pick", "badid"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("pick badid: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("pick badid: error = %v, want containing \"not found\"", err)
	}
}

func TestPickWithDoneFlag(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
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
	w.Close()

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

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "--done"})
	if err := rootCmd.Execute(); err != nil {
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
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
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
	w.Close()

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

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "--all"})
	if err := rootCmd.Execute(); err != nil {
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
			os.Setenv("PATH", "")
			t.Cleanup(func() { os.Setenv("PATH", origPath) })
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
			w.Close()

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

			resetPickFlags()
			rootCmd.SetArgs([]string{"pick", flag})
			if err := rootCmd.Execute(); err != nil {
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
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
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
	w.Close()

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

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick"})
	if err := rootCmd.Execute(); err != nil {
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
		resetPickFlags()
		rootCmd.SetArgs(args)
		err := rootCmd.Execute()
		if err == nil {
			t.Errorf("pick %v: expected error (only one state flag allowed), got nil", args)
		}
		if err != nil && !strings.Contains(err.Error(), "one of") {
			t.Errorf("pick %v: error = %v, want message containing \"one of\"", args, err)
		}
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

	rootCmd.SetArgs([]string{"export", "--tag", "prio", "-o", outPath})
	if err := rootCmd.Execute(); err != nil {
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
func TestListShowsStatusWithAlignment(t *testing.T) {
	resetListFlags()
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
		rootCmd.SetArgs([]string{"list", "--all"})
		if err := rootCmd.Execute(); err != nil {
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
			rootCmd.SetArgs(nil)
			if err := rootCmd.Execute(); err != nil {
				t.Errorf("Execute: %v", err)
			}
		})
		// Undone is default: no state suffix
		if bytes.Contains([]byte(out), []byte("(undone)")) {
			t.Errorf("current task output should not show (undone); got %q", out)
		}
		if !bytes.Contains([]byte(out), []byte("current task:")) || !bytes.Contains([]byte(out), []byte("abc123")) {
			t.Errorf("current task output should show id and description; got %q", out)
		}
	})

	t.Run("done", func(t *testing.T) {
		dir, _ := setupWnRoot(t)
		cwd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		defer func() { _ = os.Chdir(cwd) }()

		rootCmd.SetArgs([]string{"done"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("wn done: %v", err)
		}

		out := captureStdout(t, func() {
			rootCmd.SetArgs(nil)
			if err := rootCmd.Execute(); err != nil {
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

		rootCmd.SetArgs([]string{"claim", "--for", "1h"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("wn claim: %v", err)
		}

		out := captureStdout(t, func() {
			rootCmd.SetArgs(nil)
			if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"next", "--claim", "30m"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("wn next --claim: %v", err)
		}
	})
	if !strings.Contains(out, itemID) || !strings.Contains(out, "claimed") {
		t.Errorf("wn next --claim output should contain id and claimed; got %q", out)
	}
	// Verify item is actually claimed: show --json should have in_progress_until set
	showOut := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--json", itemID})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(showOut, "in_progress_until") || strings.Contains(showOut, "\"in_progress_until\":\"0001-01-01T00:00:00Z\"") {
		t.Errorf("wn show after next --claim should show in_progress_until; got %s", showOut)
	}
}

// TestNextWithTag verifies that "wn next --tag X" sets current to the next undone item that has tag X (dependency order).
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
		rootCmd.SetArgs([]string{"next", "--tag", "agent"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"next", "--tag", "nonexistent"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"done", "--next"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"done", "--next"})
		if err := rootCmd.Execute(); err != nil {
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

// TestDuplicate_adds_note_and_marks_done verifies that "wn duplicate <id> --of <orig>" adds the standard duplicate-of note and marks the item done.
func TestDuplicate_adds_note_and_marks_done(t *testing.T) {
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
		rootCmd.SetArgs([]string{"duplicate", "def456", "--of", "abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("wn duplicate: %v", err)
		}
	})
	if !strings.Contains(out, "marked def456 as duplicate of abc123") {
		t.Errorf("wn duplicate should print confirmation; got %q", out)
	}
	item, err := store.Get("def456")
	if err != nil {
		t.Fatalf("Get def456: %v", err)
	}
	if !item.Done {
		t.Error("item should be done after duplicate")
	}
	idx := item.NoteIndexByName(wn.NoteNameDuplicateOf)
	if idx < 0 {
		t.Fatalf("note %q not found", wn.NoteNameDuplicateOf)
	}
	if item.Notes[idx].Body != "abc123" {
		t.Errorf("duplicate-of body = %q, want abc123", item.Notes[idx].Body)
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
		rootCmd.SetArgs(nil)
		if err := rootCmd.Execute(); err != nil {
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
	resetListFlags()
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
		rootCmd.SetArgs([]string{"list", "--all"})
		if err := rootCmd.Execute(); err != nil {
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
	resetListFlags()
	listJson = true
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
		rootCmd.SetArgs([]string{"list", "--json", "--sort", "alpha"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"list", "--json", "--sort", "updated:desc"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	list2 := parseListJSON(t, out2)
	// updated desc: aaa (Updated: now+1h) then bbb (Updated: now)
	if list2.Items[0].ID != "aaa" || list2.Items[1].ID != "bbb" {
		t.Errorf("list --sort updated:desc = %v, %v; want aaa then bbb", list2.Items[0].ID, list2.Items[1].ID)
	}
	listJson = false
}

func TestListLimit(t *testing.T) {
	resetListFlags()
	listJson = true
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
		rootCmd.SetArgs([]string{"list", "--json", "--sort", "alpha", "--limit", "2"})
		if err := rootCmd.Execute(); err != nil {
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
	listJson = false
}

func TestListLimitOffset(t *testing.T) {
	resetListFlags()
	listJson = true
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
		rootCmd.SetArgs([]string{"list", "--json", "--sort", "alpha", "--limit", "1", "--offset", "1"})
		if err := rootCmd.Execute(); err != nil {
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
	listJson = false
}

func TestTagInteractive_Toggle(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if _, err := w.WriteString("1 2\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()

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
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Tags: []string{"mytag"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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

	rootCmd.SetArgs([]string{"tag", "-i", "mytag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag -i mytag: %v", err)
	}

	it1, _ := store.Get("aa1111")
	it2, _ := store.Get("bb2222")
	has1 := false
	for _, tag := range it1.Tags {
		if tag == "mytag" {
			has1 = true
			break
		}
	}
	has2 := false
	for _, tag := range it2.Tags {
		if tag == "mytag" {
			has2 = true
			break
		}
	}
	if !has1 {
		t.Error("item aa1111 should have mytag after toggle (was added)")
	}
	if has2 {
		t.Error("item bb2222 should not have mytag after toggle (was removed)")
	}
}

func TestTagInteractive_OnlyUndoneItems(t *testing.T) {
	// wn tag -i should list only undone items; done items must not appear in fzf/numbered list
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

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
	w.Close()

	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	doneItem := &wn.Item{ID: "aa1111", Description: "done task", Done: true, Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	undoneItem := &wn.Item{ID: "bb2222", Description: "undone task", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}
	for _, it := range []*wn.Item{doneItem, undoneItem} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"tag", "-i", "mytag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag -i mytag: %v", err)
	}

	// Only undone item (bb2222) should be in the list; selecting "1" must tag bb2222, not aa1111
	gotDone, _ := store.Get("aa1111")
	gotUndone, _ := store.Get("bb2222")
	doneHasTag := false
	for _, tag := range gotDone.Tags {
		if tag == "mytag" {
			doneHasTag = true
			break
		}
	}
	undoneHasTag := false
	for _, tag := range gotUndone.Tags {
		if tag == "mytag" {
			undoneHasTag = true
			break
		}
	}
	if doneHasTag {
		t.Error("tag -i must not list done items; aa1111 (done) should not have been selectable and must not have mytag")
	}
	if !undoneHasTag {
		t.Error("selecting the only listed item (bb2222, undone) should have added mytag")
	}
}

func TestDependInteractive(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

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
	w.Close()

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
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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

	rootCmd.SetArgs([]string{"depend", "-i"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend -i: %v", err)
	}

	it, _ := store.Get("aa1111")
	found := false
	for _, dep := range it.DependsOn {
		if dep == "bb2222" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("aa1111 should depend on bb2222 after depend -i; DependsOn = %v", it.DependsOn)
	}
}

func TestRmdependInteractive(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

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
	w.Close()

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
		{ID: "aa1111", Description: "first", Created: now, Updated: now, DependsOn: []string{"bb2222"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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

	rootCmd.SetArgs([]string{"rmdepend", "-i"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rmdepend -i: %v", err)
	}

	it, _ := store.Get("aa1111")
	if len(it.DependsOn) != 0 {
		t.Errorf("aa1111 should have no dependencies after rmdepend -i; DependsOn = %v", it.DependsOn)
	}
}

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

func TestShowIncludesNotes(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"note", "add", "see-file", itemID, "-m", "see file X"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--json", itemID})
		if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"do"})
	err := rootCmd.Execute()
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
	rootCmd.SetArgs([]string{"do", itemID})
	err := rootCmd.Execute()
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
	rootCmd.SetArgs([]string{"do"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("wn do without full setup should fail")
	}
	if strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "no current task") {
		t.Errorf("wn do (no arg) should use current and reach agent-orch; got: %v", err)
	}
}

func TestMarkMerged_MarksDoneWhenBranchMerged(t *testing.T) {
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
		rootCmd.SetArgs([]string{"mark-merged"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("mark-merged: %v", err)
		}
	})
	if !strings.Contains(out, "marked abc123") {
		t.Errorf("mark-merged output should contain 'marked abc123'; got %q", out)
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
