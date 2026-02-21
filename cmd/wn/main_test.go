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
