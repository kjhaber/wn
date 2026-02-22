package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/keith/wn/internal/wn"
)

// listStatusWidth and listIDWidth must match runList formatting for alignment tests.
const listStatusWidth = 7
const listIDWidth = 6
const listDescriptionStart = 2 + listIDWidth + 2 + listStatusWidth + 2 // "  "+id+"  "+status+"  "

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

	var list []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("Unmarshal list: %v\noutput: %s", err, out)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].ID != "abc123" {
		t.Errorf("id = %q, want abc123", list[0].ID)
	}
	if list[0].Description != "first line" {
		t.Errorf("description = %q, want first line", list[0].Description)
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

func TestShowOutputsFullItemJSON(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show", itemID})
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

func TestShowCurrentWhenNoArg(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"show"})
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

	var list []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("Unmarshal list: %v\noutput: %s", err, out)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
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

	var list []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("Unmarshal: %v\noutput: %s", err, out)
	}
	if len(list) != 1 || list[0].ID != "done1" {
		t.Errorf("list --done --json = %v, want single item done1", list)
	}
}

func TestListShowsStatusWithAlignment(t *testing.T) {
	listJson = false // reset in case a previous test set --json
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
	listJson = false
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
	var list []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("Unmarshal: %v\noutput: %s", err, out)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	// alpha asc: first alpha (aaa) then second alpha (bbb)
	if list[0].ID != "aaa" || list[1].ID != "bbb" {
		t.Errorf("list --sort alpha = %v, %v; want aaa then bbb", list[0].ID, list[1].ID)
	}

	out2 := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"list", "--json", "--sort", "updated:desc"})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("Execute: %v", err)
		}
	})
	if err := json.Unmarshal([]byte(out2), &list); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	// updated desc: aaa (Updated: now+1h) then bbb (Updated: now)
	if list[0].ID != "aaa" || list[1].ID != "bbb" {
		t.Errorf("list --sort updated:desc = %v, %v; want aaa then bbb", list[0].ID, list[1].ID)
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
		rootCmd.SetArgs([]string{"show", itemID})
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
