package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
