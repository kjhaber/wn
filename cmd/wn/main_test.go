package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/keith/wn/internal/wn"
)

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
