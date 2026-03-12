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

	"github.com/keith/wn/internal/wn"
)

// resetPickFlags clears pick filter flags to avoid Cobra's flag persistence across
// Execute() calls (see https://github.com/spf13/cobra/issues/2079). Call before
// each test that invokes "pick" with different flags.
// resetShowFlags clears show flags to avoid Cobra's flag persistence across Execute() calls.
func resetShowFlags() {
	showJson = false
	showPlain = false
	showAll = false
	showFields = ""
}

func resetPickFlags() {
	pickUndone = false
	pickDone = false
	pickAll = false
	pickReviewReady = false
}

// resetTagFlags clears tag flags to avoid Cobra's flag persistence across
// Execute() calls. Call before each test that invokes "tag" with different flags.
func resetTagFlags() {
	tagWid = ""
	tagAddInteractive = false
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

// resetDependFlags clears depend subcommand flags to avoid Cobra's flag persistence
// across Execute() calls. Call before each test that invokes "depend" with different flags.
func resetDependFlags() {
	dependAddOn = ""
	dependAddWid = ""
	dependAddInteractive = false
	dependRmOn = ""
	dependRmWid = ""
	dependRmInteractive = false
	dependListWid = ""
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

// writeRunnerSettings writes a project-level settings file configuring a single runner
// named runnerName with the given cmd, set as agent.default.
func writeRunnerSettings(t *testing.T, root, runnerName, cmd string) {
	t.Helper()
	wnDir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := fmt.Sprintf(`{"runners":{%q:{"cmd":%q}},"agent":{"default":%q}}`, runnerName, cmd, runnerName)
	if err := os.WriteFile(filepath.Join(wnDir, "settings.json"), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}
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

func TestShowPlain(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	resetShowFlags()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--plain", itemID})
		if err := rootCmd.Execute(); err != nil {
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
	resetShowFlags()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--plain"})
		if err := rootCmd.Execute(); err != nil {
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
	resetShowFlags()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--fields", "title", itemID})
		if err := rootCmd.Execute(); err != nil {
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
	resetShowFlags()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--all", itemID})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"zzz999"})
		if err := rootCmd.Execute(); err != nil {
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
	resetShowFlags()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", itemID})
		if err := rootCmd.Execute(); err != nil {
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
	resetShowFlags()

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
	resetShowFlags()

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

	resetListFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"list", "--json", "--undone"})
		if err := rootCmd.Execute(); err != nil {
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
		pickerFlag = ""
		_ = wn.SetPickerMode("")
	})
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
	if err := store.Put(&wn.Item{ID: "task1", Description: "task one", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "--picker", "numbered"})
	if err := rootCmd.Execute(); err != nil {
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
		pickerFlag = ""
		_ = wn.SetPickerMode("")
	}()

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "--picker", "invalid"})
	err := rootCmd.Execute()
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

// resetImportFlags clears import flags so tests get consistent behavior.
func resetImportFlags() {
	importReplace = false
	importAppend = false
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
	resetImportFlags()
	rootCmd.SetArgs([]string{"import", path})
	err = rootCmd.Execute()
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
	resetImportFlags()
	rootCmd.SetArgs([]string{"import", "--replace", path})
	if err := rootCmd.Execute(); err != nil {
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
	resetImportFlags()
	rootCmd.SetArgs([]string{"import", "--append", path})
	if err := rootCmd.Execute(); err != nil {
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
	resetImportFlags()
	rootCmd.SetArgs([]string{"import", "--append", "--replace", path})
	err := rootCmd.Execute()
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
	resetImportFlags()
	rootCmd.SetArgs([]string{"import", path})
	if err := rootCmd.Execute(); err != nil {
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

// TestStatus_closed_duplicate_of verifies that "wn status closed [id] --duplicate-of <id2>" adds the standard duplicate-of note and marks the item closed.
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
	resetTagFlags()
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

	rootCmd.SetArgs([]string{"tag", "add", "-i", "mytag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag add -i mytag: %v", err)
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
	resetTagFlags()
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

	rootCmd.SetArgs([]string{"tag", "add", "-i", "mytag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag add -i mytag: %v", err)
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

func TestTagAdd(t *testing.T) {
	resetTagFlags()
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
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn tag add mytag (current item)
	rootCmd.SetArgs([]string{"tag", "add", "mytag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag add mytag: %v", err)
	}
	it, _ := store.Get("aa1111")
	hasTag := false
	for _, tag := range it.Tags {
		if tag == "mytag" {
			hasTag = true
			break
		}
	}
	if !hasTag {
		t.Error("tag add mytag should add tag to current item aa1111")
	}

	// wn tag add other --wid bb2222
	rootCmd.SetArgs([]string{"tag", "add", "other", "--wid", "bb2222"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag add other --wid bb2222: %v", err)
	}
	it2, _ := store.Get("bb2222")
	hasOther := false
	for _, tag := range it2.Tags {
		if tag == "other" {
			hasOther = true
			break
		}
	}
	if !hasOther {
		t.Error("tag add other --wid bb2222 should add tag to bb2222")
	}
}

func TestTagRm(t *testing.T) {
	resetTagFlags()
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
		{ID: "aa1111", Description: "first", Created: now, Updated: now, Tags: []string{"mytag", "other"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Tags: []string{"mytag"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aa1111"}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn tag rm mytag (current item)
	rootCmd.SetArgs([]string{"tag", "rm", "mytag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag rm mytag: %v", err)
	}
	it, _ := store.Get("aa1111")
	for _, tag := range it.Tags {
		if tag == "mytag" {
			t.Error("tag rm mytag should remove tag from current item aa1111")
			break
		}
	}

	// wn tag rm mytag --wid bb2222
	rootCmd.SetArgs([]string{"tag", "rm", "mytag", "--wid", "bb2222"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag rm mytag --wid bb2222: %v", err)
	}
	it2, _ := store.Get("bb2222")
	if len(it2.Tags) != 0 {
		t.Errorf("tag rm mytag --wid bb2222 should remove tag from bb2222; remaining tags: %v", it2.Tags)
	}
}

func TestTagList(t *testing.T) {
	resetTagFlags()
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
		{ID: "aa1111", Description: "first", Created: now, Updated: now, Tags: []string{"foo", "bar"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Tags: []string{"baz"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aa1111"}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// wn tag list (current item) — one per line
	rootCmd.SetArgs([]string{"tag", "list"})
	var out strings.Builder
	rootCmd.SetOut(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag list: %v", err)
	}
	rootCmd.SetOut(nil)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("tag list: want 2 lines (foo, bar), got %d: %q", len(lines), lines)
	}
	if lines[0] != "foo" || lines[1] != "bar" {
		t.Errorf("tag list: want lines foo, bar; got %q", lines)
	}

	// wn tag list --wid bb2222
	rootCmd.SetArgs([]string{"tag", "list", "--wid", "bb2222"})
	out.Reset()
	rootCmd.SetOut(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tag list --wid bb2222: %v", err)
	}
	rootCmd.SetOut(nil)
	lines2 := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines2) != 1 || lines2[0] != "baz" {
		t.Errorf("tag list --wid bb2222: want one line 'baz'; got %q", lines2)
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

	rootCmd.SetArgs([]string{"depend", "add", "-i"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend add -i: %v", err)
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

	rootCmd.SetArgs([]string{"depend", "rm", "-i"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend rm -i: %v", err)
	}

	it, _ := store.Get("aa1111")
	if len(it.DependsOn) != 0 {
		t.Errorf("aa1111 should have no dependencies after rmdepend -i; DependsOn = %v", it.DependsOn)
	}
}

// TestDependAddWithOnAndWid tests "wn depend add --on <id> [--wid <id>]"
func TestDependAddWithOnAndWid(t *testing.T) {
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

	resetDependFlags()
	// depend add --on bb2222 --wid aa1111
	rootCmd.SetArgs([]string{"depend", "add", "--on", "bb2222", "--wid", "aa1111"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend add --on bb2222 --wid aa1111: %v", err)
	}
	it, _ := store.Get("aa1111")
	if len(it.DependsOn) != 1 || it.DependsOn[0] != "bb2222" {
		t.Errorf("after depend add: DependsOn = %v, want [bb2222]", it.DependsOn)
	}
}

// TestDependAddWithOnCurrent tests "wn depend add --on <id>" without --wid uses current task
func TestDependAddWithOnCurrent(t *testing.T) {
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

	resetDependFlags()
	rootCmd.SetArgs([]string{"depend", "add", "--on", "bb2222"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend add --on bb2222: %v", err)
	}
	it, _ := store.Get("aa1111")
	if len(it.DependsOn) != 1 || it.DependsOn[0] != "bb2222" {
		t.Errorf("after depend add (current): DependsOn = %v, want [bb2222]", it.DependsOn)
	}
}

// TestDependRmWithOnAndWid tests "wn depend rm --on <id> [--wid <id>]"
func TestDependRmWithOnAndWid(t *testing.T) {
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

	resetDependFlags()
	rootCmd.SetArgs([]string{"depend", "rm", "--on", "bb2222", "--wid", "aa1111"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend rm --on bb2222 --wid aa1111: %v", err)
	}
	it, _ := store.Get("aa1111")
	if len(it.DependsOn) != 0 {
		t.Errorf("after depend rm: DependsOn = %v, want []", it.DependsOn)
	}
}

// TestDependList tests "wn depend list [--wid <id>]" outputs dependency ids one per line
func TestDependList(t *testing.T) {
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
		{ID: "aa1111", Description: "first", Created: now, Updated: now, DependsOn: []string{"bb2222", "cc3333"}, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "bb2222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
		{ID: "cc3333", Description: "third", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}},
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

	resetDependFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	defer func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) }()

	rootCmd.SetArgs([]string{"depend", "list", "--wid", "aa1111"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend list --wid aa1111: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Errorf("depend list: got %d lines, want 2; output %q", len(lines), out)
	}
	if out != "bb2222\ncc3333" && out != "cc3333\nbb2222" {
		t.Errorf("depend list: output should be two lines (bb2222 and cc3333); got %q", out)
	}
}

// TestDependListEmpty tests "wn depend list" when item has no dependencies
func TestDependListEmpty(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	resetDependFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	defer func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) }()

	rootCmd.SetArgs([]string{"depend", "list", "--wid", itemID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("depend list: %v", err)
	}
	if buf.String() != "" {
		t.Errorf("depend list (no deps): got %q, want empty", buf.String())
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

func TestRmWithExplicitId(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"rm", itemID})
	if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"rm", "abc123", "bb2222", "cc3333"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rm multiple: %v", err)
	}
	for _, id := range []string{"abc123", "bb2222", "cc3333"} {
		if _, err := store.Get(id); err == nil {
			t.Errorf("item %s should be removed", id)
		}
	}
}

func TestRmInteractiveMultiSelect(t *testing.T) {
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

	dir, _ := setupWnRoot(t)
	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	if err := store.Put(&wn.Item{ID: "bb2222", Description: "second", Created: now, Updated: now, Log: []wn.LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"rm"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rm (interactive): %v", err)
	}
	// Items 1 and 2 in the list (abc123, bb2222) should be removed
	if _, err := store.Get("abc123"); err == nil {
		t.Error("item abc123 should be removed")
	}
	if _, err := store.Get("bb2222"); err == nil {
		t.Error("item bb2222 should be removed")
	}
}

func TestRmInteractiveCancel(t *testing.T) {
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
	if _, err := w.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()

	dir, itemID := setupWnRoot(t)
	store, _ := wn.NewFileStore(dir)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"rm"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rm (cancel): %v", err)
	}
	if _, err := store.Get(itemID); err != nil {
		t.Error("item should still exist after cancel")
	}
}

func TestRmClearsCurrentWhenDeleted(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"rm", itemID})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rm: %v", err)
	}
	meta, _ := wn.ReadMeta(dir)
	if meta.CurrentID != "" {
		t.Errorf("CurrentID should be cleared when current task is removed; got %q", meta.CurrentID)
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

func TestMerge_noBranchNote(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatal(err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID: "abc123", Description: "task", Created: now, Updated: now,
		ReviewReady: true, Log: []wn.LogEntry{{At: now, Kind: "created"}},
		Notes: []wn.Note{},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "abc123"}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	rootCmd.SetArgs([]string{"merge"})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("wn merge with no branch note should fail")
	}
	if !strings.Contains(err.Error(), "branch note") {
		t.Errorf("wn merge error = %v, want message containing 'branch note'", err)
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

	rootCmd.SetArgs([]string{"worktree"})
	err := rootCmd.Execute()
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

	rootCmd.SetArgs([]string{"worktree", "--next", itemID})
	err := rootCmd.Execute()
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
		rootCmd.SetArgs([]string{"worktree", itemID, "--worktree-base", worktreesBase})
		if err := rootCmd.Execute(); err != nil {
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
	// Branch note should be set
	if item.NoteIndexByName("branch") < 0 {
		t.Error("item should have branch note after wn worktree")
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
		rootCmd.SetArgs([]string{"worktree", "--worktree-base", worktreesBase})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"worktree", "--next", "--worktree-base", worktreesBase})
		if err := rootCmd.Execute(); err != nil {
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

// resetDoFlags resets wn do flags between test invocations.
func resetDoFlags() {
	doNext = false
	doLoop = false
	doMaxTasks = 0
}

// TestDoUnified_nextAndIdArgError verifies that "wn do --next <id>" is rejected.
func TestDoUnified_nextAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()

	rootCmd.SetArgs([]string{"do", "--next", itemID})
	err := rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()

	rootCmd.SetArgs([]string{"do", "--loop", itemID})
	err := rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()

	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"do", "--next"})
	err := rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()

	rootCmd.SetArgs([]string{"do", "-n", "3"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("wn do -n N without --loop should fail")
	}
	if !strings.Contains(err.Error(), "--loop") {
		t.Errorf("want error mentioning --loop; got: %v", err)
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

	rootCmd.SetArgs([]string{"launch"})
	err := rootCmd.Execute()
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
	rootCmd.SetArgs([]string{"launch"})
	err := rootCmd.Execute()
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

	rootCmd.SetArgs([]string{"launch", "--next", itemID})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("wn launch --next <id> should fail")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("want mutual exclusion error; got: %v", err)
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

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "def456"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("pick def456: %v", err)
	}

	// Now pick - should return to abc123
	resetPickFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pick", "-"})
		if err := rootCmd.Execute(); err != nil {
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

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "-"})
	err := rootCmd.Execute()
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

	resetPickFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pick", "."})
		if err := rootCmd.Execute(); err != nil {
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

	resetPickFlags()
	rootCmd.SetArgs([]string{"pick", "."})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("wn pick . with no matching item should error")
	}
	if !strings.Contains(err.Error(), "no work item") {
		t.Errorf("want 'no work item' error; got: %v", err)
	}
}

func resetArchiveFlags() {
	archiveLocation = ""
}

func TestArchiveCmd_RemovesItemFromStore(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	resetArchiveFlags()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"archive", itemID})
		if err := rootCmd.Execute(); err != nil {
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
	resetArchiveFlags()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"archive", itemID})
		if err := rootCmd.Execute(); err != nil {
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
	resetArchiveFlags()

	customDir := filepath.Join(t.TempDir(), "custom-archive")
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"archive", "--location", customDir, itemID})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	resetArchiveFlags()

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
	resetArchiveFlags()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"archive", itemID})
		if err := rootCmd.Execute(); err != nil {
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
	resetArchiveFlags()

	rootCmd.SetArgs([]string{"archive", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error archiving nonexistent item")
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
		rootCmd.SetArgs([]string{"prompt", parentID, "-m", "What should the behavior be?"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"prompt", "-m", "Is this right?"})
		if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"respond", "pmt111", "-m", "Yes, proceed."})
	if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"respond", itemID, "-m", "This should fail."})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when responding to non-prompt item")
	}
}

func TestDoneCmd_autoMarksPromptDepsAsDone(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID: "pmt-done01", Description: "Need answer",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	_ = store.Put(promptItem)
	_ = store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, "pmt-done01")
		return it, nil
	})

	rootCmd.SetArgs([]string{"done", parentID})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("Execute done: %v", err)
	}

	parent, _ := store.Get(parentID)
	if !parent.Done {
		t.Error("parent should be done")
	}
	prompt, _ := store.Get("pmt-done01")
	if !prompt.Done {
		t.Error("prompt dep should be auto-marked done")
	}
}

func TestDoneCmd_promptDepDoesNotBlockDone(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	store, _ := wn.NewFileStore(dir)
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID: "pmt-done02", Description: "Need answer",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	_ = store.Put(promptItem)
	_ = store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, "pmt-done02")
		return it, nil
	})

	// Should succeed without --force even though prompt dep is undone
	rootCmd.SetArgs([]string{"done", parentID})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("done should not be blocked by prompt dep: %v", err)
	}
}

func TestArchiveCmd_includesPromptDepsInArchive(t *testing.T) {
	dir, parentID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	resetArchiveFlags()

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
		rootCmd.SetArgs([]string{"archive", parentID})
		if err := rootCmd.Execute(); err != nil {
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
