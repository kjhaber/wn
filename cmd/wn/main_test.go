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
	listGroup = ""
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

func writeLaunchRunnerSettings(t *testing.T, root, runnerName, cmd string) {
	t.Helper()
	wnDir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := fmt.Sprintf(`{"runners":{%q:{"cmd":%q}},"agent":{"default_launch":%q}}`, runnerName, cmd, runnerName)
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
	defer resetExportFlags()

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

// resetExportFlags clears export flags to avoid Cobra's flag persistence across Execute() calls.
func resetExportFlags() {
	exportOutput = ""
	exportAll = false
	exportUndone = false
	exportDone = false
	exportReviewReady = false
	exportTag = ""
	exportSort = ""
	exportLimit = 0
	exportOffset = 0
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
	defer resetExportFlags()

	rootCmd.SetArgs([]string{"export", "--review-ready", "-o", outPath})
	if err := rootCmd.Execute(); err != nil {
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
	defer resetExportFlags()

	// AND filter: only items with both "a" and "b"
	rootCmd.SetArgs([]string{"export", "--tag", "a,b", "-o", outPath})
	if err := rootCmd.Execute(); err != nil {
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
	defer resetExportFlags()

	// Sort alphabetically descending — should put "alpha second" before "alpha first"
	rootCmd.SetArgs([]string{"export", "--all", "--sort", "alpha:desc", "-o", outPath})
	if err := rootCmd.Execute(); err != nil {
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
	defer resetExportFlags()

	// --limit 2: should return first 2 items
	rootCmd.SetArgs([]string{"export", "--all", "--sort", "priority", "--limit", "2", "-o", outPath})
	if err := rootCmd.Execute(); err != nil {
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
	resetExportFlags()
	rootCmd.SetArgs([]string{"export", "--all", "--sort", "priority", "--offset", "1", "-o", outPath})
	if err := rootCmd.Execute(); err != nil {
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
	defer resetExportFlags()

	rootCmd.SetArgs([]string{"export", "--undone", "--done"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("export --undone --done: expected error, got nil")
	}
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

func TestListGroupByTags(t *testing.T) {
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
		rootCmd.SetArgs([]string{"list", "--group", "tags"})
		if err := rootCmd.Execute(); err != nil {
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
		resetListFlags()
		rootCmd.SetArgs([]string{"list", "--group", "tags"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"list", "--all", "--group", "status"})
		if err := rootCmd.Execute(); err != nil {
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
	resetListFlags()
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"list", "--group", "bogus"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("list --group bogus should return an error")
	}
}

func TestListGroupWithJSON(t *testing.T) {
	resetListFlags()
	dir, _ := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	rootCmd.SetArgs([]string{"list", "--group", "tags", "--json"})
	if err := rootCmd.Execute(); err == nil {
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
		resetListFlags()
		rootCmd.SetArgs([]string{"list", "@agent"})
		if err := rootCmd.Execute(); err != nil {
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
	resetListFlags()
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

	resetListFlags()
	rootCmd.SetArgs([]string{"list", "@nosuchview"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("list @nosuchview should return an error")
	}
}

func TestListViewAtSyntax_withSortAndGroup(t *testing.T) {
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
		resetListFlags()
		rootCmd.SetArgs([]string{"list", "@bygroup"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	// With --group tags, output should contain section headers
	if !strings.Contains(out, "---") {
		t.Errorf("output should contain group headers, got: %s", out)
	}
}

func TestListViewAtSyntax_noViews(t *testing.T) {
	resetListFlags()
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

	resetListFlags()
	rootCmd.SetArgs([]string{"list", "@agent"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("list @agent with no views configured should return an error")
	}
}

func TestTagInteractive_Toggle(t *testing.T) {
	resetTagFlags()
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
	if _, err := w.WriteString("1 2\n"); err != nil {
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

	rootCmd.SetArgs([]string{"rm"})
	if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"rm"})
	if err := rootCmd.Execute(); err == nil {
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

func TestWorktreeSetup_branchFlag(t *testing.T) {
	dir, itemID := setupGitWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	worktreesBase := filepath.Join(dir, "worktrees")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"worktree", itemID, "--worktree-base", worktreesBase, "--branch", "my-feature"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"worktree", itemID, "--worktree-base", worktreesBase, "--branch", "my-feature", "--branch-prefix", "keith/"})
		if err := rootCmd.Execute(); err != nil {
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
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()
	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"do", "task1"})
	err = rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()
	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"do"})
	err = rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()
	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"launch", "task1"})
	err = rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()
	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"launch"})
	err = rootCmd.Execute()
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

// resetLaunchFlags resets wn launch flags between test invocations.
func resetLaunchFlags() {
	launchNext = false
	launchLoop = false
	launchMaxTasks = 0
}

// TestLaunchLoopAndIdArgError verifies that "wn launch --loop <id>" is rejected.
func TestLaunchLoopAndIdArgError(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()

	rootCmd.SetArgs([]string{"launch", "--loop", itemID})
	err := rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()

	rootCmd.SetArgs([]string{"launch", "-n", "3"})
	err := rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()

	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"launch", "--next"})
	err := rootCmd.Execute()
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
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()

	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"launch", "--next", "--worktree-base", worktreesBase})
	if err := rootCmd.Execute(); err != nil {
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
	defer func() {
		_ = os.Chdir(cwd)
		resetLaunchFlags()
	}()

	writeLaunchRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"launch", "--next", "--worktree-base", worktreesBase})
	if err := rootCmd.Execute(); err != nil {
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
	defer func() {
		_ = os.Chdir(cwd)
		resetDoFlags()
	}()

	writeRunnerSettings(t, dir, "echo-runner", "echo hello")
	rootCmd.SetArgs([]string{"do", "--next", "--worktree-base", worktreesBase})
	if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"verify"})
		if err := rootCmd.Execute(); err != nil {
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

	rootCmd.SetArgs([]string{"verify"})
	err := rootCmd.Execute()
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

	rootCmd.SetArgs([]string{"verify"})
	err := rootCmd.Execute()
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

	// Cobra prints usage to rootCmd's out writer (via Println); capture it to assert it's suppressed.
	var outBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	defer rootCmd.SetOut(nil)

	rootCmd.SetArgs([]string{"verify"})
	_ = rootCmd.Execute()

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
	rootCmd.SetErr(&stderrBuf)
	defer rootCmd.SetErr(os.Stderr)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"verify"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	stderrOut := stderrBuf.String()
	if !strings.Contains(stderrOut, verifyCmd) {
		t.Errorf("stderr = %q, want to contain the configured command %q", stderrOut, verifyCmd)
	}
}

func resetSettingsEditFlags() {
	settingsEditUser = false
	settingsEditUserLocal = false
	settingsEditProject = false
	settingsEditProjectLocal = false
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
	rootCmd.SetOut(&buf)
	defer rootCmd.SetOut(nil)
	rootCmd.SetArgs([]string{"settings", "show"})
	if err := rootCmd.Execute(); err != nil {
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
	defer resetSettingsEditFlags()
	rootCmd.SetArgs([]string{"settings", "edit", "--project"})
	if err := rootCmd.Execute(); err != nil {
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

	defer resetSettingsEditFlags()
	rootCmd.SetArgs([]string{"settings", "edit", "--user"})
	if err := rootCmd.Execute(); err != nil {
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

// resetCleanupWorktreesFlags resets cleanup worktrees flags to defaults between tests.
func resetCleanupWorktreesFlags() {
	cleanupWorktreesDryRun = false
	cleanupWorktreesBranch = ""
	cleanupWorktreesCleanIgnored = false
	cleanupWorktreesForce = false
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
		rootCmd.SetArgs([]string{"summary"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"summary"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"summary"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"summary"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"summary"})
		if err := rootCmd.Execute(); err != nil {
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

func TestShowMetadataSection(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Add a special wn: note and a regular note
	rootCmd.SetArgs([]string{"note", "add", "wn:branch", itemID, "-m", "feat/my-branch"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add wn:branch: %v", err)
	}
	rootCmd.SetArgs([]string{"note", "add", "pr-url", itemID, "-m", "https://example.com/pr/1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("note add pr-url: %v", err)
	}

	resetShowFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", "--fields", "notes", itemID})
		if err := rootCmd.Execute(); err != nil {
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
func TestCompletionZsh(t *testing.T) {
	var buf bytes.Buffer
	if err := rootCmd.GenZshCompletion(&buf); err != nil {
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
	if err := rootCmd.GenBashCompletionV2(&buf, true); err != nil {
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
	if err := rootCmd.GenFishCompletion(&buf, true); err != nil {
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

func resetMergeFlags() {
	mergeMessage = ""
	mergeDryRun = false
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

	resetMergeFlags()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"merge", "wn-abc999-feat", "-m", "Squash: implement feat"})
		if err := rootCmd.Execute(); err != nil {
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

	resetMergeFlags()
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"merge", "wn-dry999-feat", "-m", "dry msg", "--dry-run"})
		if err := rootCmd.Execute(); err != nil {
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

	resetMergeFlags()
	err := func() error {
		rootCmd.SetArgs([]string{"merge", "wn-noitem-feat", "-m", "some msg"})
		return rootCmd.Execute()
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

	resetMergeFlags()
	err := func() error {
		rootCmd.SetArgs([]string{"merge"})
		return rootCmd.Execute()
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
func TestWnRootCmd_NormalRepo(t *testing.T) {
	repoDir := t.TempDir()
	setupGitRepoMain(t, repoDir)

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"root"})
		if err := rootCmd.Execute(); err != nil {
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
		rootCmd.SetArgs([]string{"root"})
		if err := rootCmd.Execute(); err != nil {
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
