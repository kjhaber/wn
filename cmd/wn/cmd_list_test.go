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
	t.Chdir(dir)
	list := parseListJSON(t, runCmd(t, []string{"list", "--json"}))
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

// TestShow consolidates basic show command variants that all use setupWnRoot.
func TestShow(t *testing.T) {
	tests := []struct {
		name  string
		args  func(id string) []string
		check func(t *testing.T, out, id string)
	}{
		{
			name: "plain",
			args: func(id string) []string { return []string{"show", "--plain", id} },
			check: func(t *testing.T, out, _ string) {
				want := "first line\nsecond line\n"
				if out != want {
					t.Errorf("show --plain = %q, want %q", out, want)
				}
			},
		},
		{
			name: "fields-title",
			args: func(id string) []string { return []string{"show", "--fields", "title", id} },
			check: func(t *testing.T, out, id string) {
				if !strings.Contains(out, id) || !strings.Contains(out, "first line") {
					t.Errorf("show --fields=title should contain id and first line; got %q", out)
				}
				if strings.Contains(out, "second line") {
					t.Errorf("show --fields=title should not contain body; got %q", out)
				}
			},
		},
		{
			name: "all",
			args: func(id string) []string { return []string{"show", "--all", id} },
			check: func(t *testing.T, out, _ string) {
				if !strings.Contains(out, "log:") || !strings.Contains(out, "created") {
					t.Errorf("show --all should include log section; got %q", out)
				}
			},
		},
		{
			name: "default-human-readable",
			args: func(id string) []string { return []string{"show", id} },
			check: func(t *testing.T, out, id string) {
				if strings.HasPrefix(strings.TrimSpace(out), "{") {
					t.Errorf("show (default) should be human-readable, not JSON; got: %s", out)
				}
				if !strings.Contains(out, id) || !strings.Contains(out, "first line") {
					t.Errorf("show (default) should contain item id and description text; got: %s", out)
				}
			},
		},
		{
			name: "current-no-arg",
			args: func(_ string) []string { return []string{"show", "--json"} },
			check: func(t *testing.T, out, id string) {
				var item wn.Item
				if err := json.Unmarshal([]byte(out), &item); err != nil {
					t.Fatalf("Unmarshal show: %v\noutput: %s", err, out)
				}
				if item.ID != id {
					t.Errorf("show (no arg) id = %q, want %s (current)", item.ID, id)
				}
			},
		},
		{
			name: "json-with-id",
			args: func(id string) []string { return []string{"show", "--json", id} },
			check: func(t *testing.T, out, id string) {
				var item wn.Item
				if err := json.Unmarshal([]byte(out), &item); err != nil {
					t.Fatalf("Unmarshal show: %v\noutput: %s", err, out)
				}
				if item.ID != id {
					t.Errorf("id = %q, want %s", item.ID, id)
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, itemID := setupWnRoot(t)
			t.Chdir(dir)
			out := runCmd(t, tt.args(itemID))
			tt.check(t, out, itemID)
		})
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
	item := &wn.Item{ID: "aaa111", Description: "one liner task", Created: now, Updated: now, Log: createdLog(now)}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aaa111"}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	out := runCmd(t, []string{"show", "--plain"})
	if out != "one liner task\n" {
		t.Errorf("show --plain (one-line) = %q, want %q", out, "one liner task\n")
	}
}

func TestBareWnAcceptsID(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	other := &wn.Item{ID: "zzz999", Description: "other item", Created: now, Updated: now, Log: createdLog(now)}
	if err := store.Put(other); err != nil {
		t.Fatal(err)
	}
	_ = itemID // current task stays as abc123
	t.Chdir(dir)
	out := runCmd(t, []string{"zzz999"})
	if !strings.Contains(out, "zzz999") || !strings.Contains(out, "other item") {
		t.Errorf("bare wn <id> should show item zzz999; got %q", out)
	}
	if strings.Contains(out, "abc123") {
		t.Errorf("bare wn <id> should show zzz999, not abc123; got %q", out)
	}
}

func TestShowRespectsSettingsDefaultFields(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	wnDir := filepath.Join(dir, ".wn")
	settingsPath := filepath.Join(wnDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"show":{"default_fields":"title,body,log"}}`), 0644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}
	t.Chdir(dir)
	out := runCmd(t, []string{"show", itemID})
	if !strings.Contains(out, "log:") || !strings.Contains(out, "created") {
		t.Errorf("show should include log when settings.show.default_fields contains log; got %q", out)
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
		{ID: "aa1111", Description: "current task", Created: now, Updated: now, DependsOn: []string{"bb2222"}, Log: createdLog(now)},
		{ID: "bb2222", Description: "prerequisite", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "cc3333", Description: "follow-up", Created: now, Updated: now, DependsOn: []string{"aa1111"}, Log: createdLog(now)},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aa1111"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	t.Chdir(dir)
	out := runCmd(t, nil)
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
		{ID: "aa1111", Description: "first", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "bb2222", Description: "second", Created: now, Updated: now, DependsOn: []string{"aa1111"}, Log: createdLog(now)},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	out := runCmd(t, []string{"show", "aa1111"})
	if !strings.Contains(out, "dependent tasks: bb2222") {
		t.Errorf("wn show should show dependent tasks when item has dependents; got %q", out)
	}
}

func TestListJSONEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	t.Chdir(dir)
	list := parseListJSON(t, runCmd(t, []string{"list", "--json"}))
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
		{ID: "done1", Description: "done", Created: now, Updated: now, Done: true, Log: createdLog(now)},
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	list := parseListJSON(t, runCmd(t, []string{"list", "--json", "--done"}))
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
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "rr1", Description: "review-ready", Created: now, Updated: now, ReviewReady: true, Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	list := parseListJSON(t, runCmd(t, []string{"list", "--json", "--undone"}))
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
	t.Chdir(dir)
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
	t.Chdir(dir)
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

// TestPickWithStateFlag consolidates pick tests that use a state flag (--done, --rr,
// --review-ready) or the default filter (undone only), piping stdin to select an item.
func TestPickWithStateFlag(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name   string
		items  []*wn.Item
		stdin  string
		args   []string
		wantID string
	}{
		{
			name:   "done",
			items:  []*wn.Item{{ID: "done1", Description: "done task", Done: true, Created: now, Updated: now, Log: createdLog(now)}},
			stdin:  "1",
			args:   []string{"pick", "--done"},
			wantID: "done1",
		},
		{
			name:   "rr-flag",
			items:  []*wn.Item{{ID: "rr1111", Description: "review-ready task", ReviewReady: true, Created: now, Updated: now, Log: createdLog(now)}},
			stdin:  "1",
			args:   []string{"pick", "--rr"},
			wantID: "rr1111",
		},
		{
			name:   "review-ready-flag",
			items:  []*wn.Item{{ID: "rr1111", Description: "review-ready task", ReviewReady: true, Created: now, Updated: now, Log: createdLog(now)}},
			stdin:  "1",
			args:   []string{"pick", "--review-ready"},
			wantID: "rr1111",
		},
		{
			name: "default-is-undone",
			items: []*wn.Item{
				{ID: "done1", Description: "done", Done: true, Created: now, Updated: now, Log: createdLog(now)},
				{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: createdLog(now)},
				{ID: "rr1", Description: "review-ready", ReviewReady: true, Created: now, Updated: now, Log: createdLog(now)},
			},
			stdin:  "1",
			args:   []string{"pick"},
			wantID: "undone1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if _, err := w.WriteString(tt.stdin + "\n"); err != nil {
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
			for _, it := range tt.items {
				if err := store.Put(it); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(dir)

			root := newRootCmd()
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("pick %v: %v", tt.args, err)
			}
			meta, err := wn.ReadMeta(dir)
			if err != nil {
				t.Fatalf("ReadMeta: %v", err)
			}
			if meta.CurrentID != tt.wantID {
				t.Errorf("after pick %v: CurrentID = %q, want %q", tt.args, meta.CurrentID, tt.wantID)
			}
		})
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
		{ID: "done1", Description: "done", Created: now, Updated: now, Done: true, Log: createdLog(now)},
		{ID: "undone1", Description: "undone", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

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

func TestPickStateFlagsMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	t.Chdir(dir)
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
	if err := store.Put(&wn.Item{ID: "task1", Description: "task one", Created: now, Updated: now, Log: createdLog(now)}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

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
	t.Cleanup(func() { _ = wn.SetPickerMode("") })
	t.Chdir(dir)

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

func itemIDs(items []*wn.Item) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

// TestExportFilter consolidates export tests that filter by tag or state.
func TestExportFilter(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name    string
		items   []*wn.Item
		args    func(outPath string) []string
		wantIDs []string
	}{
		{
			name: "tag",
			items: []*wn.Item{
				{ID: "aaa111", Description: "tagged", Created: now, Updated: now, Tags: []string{"prio"}, Log: createdLog(now)},
				{ID: "bbb222", Description: "untagged", Created: now, Updated: now, Log: createdLog(now)},
			},
			args:    func(out string) []string { return []string{"export", "--tag", "prio", "-o", out} },
			wantIDs: []string{"aaa111"},
		},
		{
			name: "review-ready",
			items: []*wn.Item{
				{ID: "aaa111", Description: "available", Created: now, Updated: now, Log: createdLog(now)},
				{ID: "bbb222", Description: "review-ready", Created: now, Updated: now, ReviewReady: true, Log: createdLog(now)},
			},
			args:    func(out string) []string { return []string{"export", "--review-ready", "-o", out} },
			wantIDs: []string{"bbb222"},
		},
		{
			name: "compound-tag",
			items: []*wn.Item{
				{ID: "aaa111", Description: "both tags", Created: now, Updated: now, Tags: []string{"a", "b"}, Log: createdLog(now)},
				{ID: "bbb222", Description: "one tag", Created: now, Updated: now, Tags: []string{"a"}, Log: createdLog(now)},
				{ID: "ccc333", Description: "no tags", Created: now, Updated: now, Log: createdLog(now)},
			},
			args:    func(out string) []string { return []string{"export", "--tag", "a,b", "-o", out} },
			wantIDs: []string{"aaa111"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := wn.InitRoot(dir); err != nil {
				t.Fatalf("InitRoot: %v", err)
			}
			store, err := wn.NewFileStore(dir)
			if err != nil {
				t.Fatalf("NewFileStore: %v", err)
			}
			for _, item := range tt.items {
				if err := store.Put(item); err != nil {
					t.Fatal(err)
				}
			}
			outPath := filepath.Join(dir, "out.json")
			t.Chdir(dir)

			root := newRootCmd()
			root.SetArgs(tt.args(outPath))
			if err := root.Execute(); err != nil {
				t.Fatalf("export %v: %v", tt.args(outPath), err)
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
			if len(exp.Items) != len(tt.wantIDs) {
				t.Fatalf("got %v, want %v", itemIDs(exp.Items), tt.wantIDs)
			}
			wantSet := make(map[string]bool)
			for _, id := range tt.wantIDs {
				wantSet[id] = true
			}
			for _, it := range exp.Items {
				if !wantSet[it.ID] {
					t.Errorf("unexpected item %s in export output", it.ID)
				}
			}
		})
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
	for _, item := range []*wn.Item{
		{ID: "aaa111", Description: "alpha first", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "bbb222", Description: "alpha second", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := filepath.Join(dir, "out.json")
	t.Chdir(dir)

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
	for _, item := range []*wn.Item{
		{ID: "aaa111", Description: "first", Created: now, Updated: now, Order: &ord0, Log: createdLog(now)},
		{ID: "bbb222", Description: "second", Created: now, Updated: now, Order: &ord1, Log: createdLog(now)},
		{ID: "ccc333", Description: "third", Created: now, Updated: now, Order: &ord2, Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	outPath := filepath.Join(dir, "out.json")
	t.Chdir(dir)

	tests := []struct {
		name    string
		args    []string
		wantLen int
		wantIDs []string
	}{
		{
			"limit-2",
			[]string{"export", "--all", "--sort", "priority", "--limit", "2", "-o", outPath},
			2, []string{"aaa111", "bbb222"},
		},
		{
			"offset-1",
			[]string{"export", "--all", "--sort", "priority", "--offset", "1", "-o", outPath},
			2, []string{"bbb222", "ccc333"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newRootCmd()
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("export %s: %v", tt.name, err)
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
			if len(exp.Items) != tt.wantLen {
				t.Fatalf("export %s: got %d items, want %d", tt.name, len(exp.Items), tt.wantLen)
			}
			for i, id := range tt.wantIDs {
				if exp.Items[i].ID != id {
					t.Errorf("export %s: item[%d] = %v, want %v", tt.name, i, exp.Items[i].ID, id)
				}
			}
		})
	}
}

func TestExport_MultipleStateFlags_Errors(t *testing.T) {
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	t.Chdir(dir)
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
	if err := store.Put(&wn.Item{ID: "abc123", Description: "existing", Created: now, Updated: now, Log: createdLog(now)}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "export.json")
	if err := wn.Export(store, path); err != nil {
		t.Fatalf("Export: %v", err)
	}
	t.Chdir(dir)
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
		{ID: "aaa111", Description: "first", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "bbb222", Description: "second", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "export.json")
	if err := wn.Export(store, path); err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Add another item so store has 3
	if err := store.Put(&wn.Item{ID: "ccc333", Description: "third", Created: now, Updated: now, Log: createdLog(now)}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
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
	if err := store.Put(&wn.Item{ID: "old111", Description: "existing", Created: now, Updated: now, Log: createdLog(now)}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "new.json")
	if err := wn.ExportItems([]*wn.Item{
		{ID: "new222", Description: "from file", Created: now, Updated: now, Log: createdLog(now)},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	t.Chdir(dir)
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
	path := filepath.Join(dir, "export.json")
	if err := wn.ExportItems(nil, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	t.Chdir(dir)
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
	now := time.Now().UTC()
	path := filepath.Join(dir, "export.json")
	if err := wn.ExportItems([]*wn.Item{
		{ID: "only1", Description: "only item", Created: now, Updated: now, Log: createdLog(now)},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	t.Chdir(dir)
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
		{ID: "done11", Description: "done task", Created: now, Updated: now, Done: true, Log: createdLog(now)},
		{ID: "undone1", Description: "undone task", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	out := runCmd(t, []string{"list", "--all"})
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("list --all should show at least 2 lines; got %q", out)
	}
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
		t.Chdir(dir)
		out := runCmd(t, nil)
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
		t.Chdir(dir)
		root := newRootCmd()
		root.SetArgs([]string{"done"})
		if err := root.Execute(); err != nil {
			t.Fatalf("wn done: %v", err)
		}
		out := runCmd(t, nil)
		if !bytes.Contains([]byte(out), []byte("done")) {
			t.Errorf("current task output should contain state 'done'; got %q", out)
		}
	})

	t.Run("claimed", func(t *testing.T) {
		dir, _ := setupWnRoot(t)
		t.Chdir(dir)
		root := newRootCmd()
		root.SetArgs([]string{"claim", "--for", "1h"})
		if err := root.Execute(); err != nil {
			t.Fatalf("wn claim: %v", err)
		}
		out := runCmd(t, nil)
		if !bytes.Contains([]byte(out), []byte("claimed")) {
			t.Errorf("current task output should contain state 'claimed'; got %q", out)
		}
	})
}

func TestNextWithClaim(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	t.Chdir(dir)
	out := runCmd(t, []string{"next", "--claim", "30m"})
	if !strings.Contains(out, itemID) || !strings.Contains(out, "claimed") {
		t.Errorf("wn next --claim output should contain id and claimed; got %q", out)
	}
	// Verify item is actually claimed: show --json should have in_progress_until set
	showOut := runCmd(t, []string{"show", "--json", itemID})
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
		{ID: "aa1111", Description: "no tag", Created: now, Updated: now, Order: &ord0, Log: createdLog(now)},
		{ID: "bb2222", Description: "has agent tag", Created: now, Updated: now, Order: &ord1, Tags: []string{"agent"}, Log: createdLog(now)},
	} {
		if err := store.Put(it); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "aa1111"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	t.Chdir(dir)

	// wn next --tag agent should set current to bb2222
	out := runCmd(t, []string{"next", "--tag", "agent"})
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
	out2 := runCmd(t, []string{"next", "--tag", "nonexistent"})
	if !strings.Contains(out2, "No next task.") {
		t.Errorf("wn next --tag nonexistent should print No next task.; got %q", out2)
	}
}

// TestDoneNext_oneItem verifies that "wn done --next" with only one (current) item prints "No next task."

func TestDoneNext_oneItem(t *testing.T) {
	dir, _ := setupWnRoot(t)
	t.Chdir(dir)
	out := runCmd(t, []string{"done", "--next"})
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
		{ID: "abc123", Description: "first task", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "def456", Description: "second task", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(it); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "abc123"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	t.Chdir(dir)
	out := runCmd(t, []string{"done", "--next"})
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
		Log:         createdLog(now),
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := wn.WriteMeta(dir, wn.Meta{CurrentID: "abc123"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	t.Chdir(dir)
	out := runCmd(t, nil)
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
		Log:         createdLog(now),
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	out := runCmd(t, []string{"list", "--all"})
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

// TestListSort consolidates list --sort flag tests that share the same two-item fixture.
func TestListSort(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	for _, item := range []*wn.Item{
		{ID: "bbb", Description: "second alpha", Created: now.Add(time.Hour), Updated: now, Log: createdLog(now)},
		{ID: "aaa", Description: "first alpha", Created: now, Updated: now.Add(time.Hour), Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	tests := []struct {
		sort      string
		wantFirst string
		wantSec   string
	}{
		{"alpha", "aaa", "bbb"},        // alpha asc: first alpha (aaa) then second alpha (bbb)
		{"updated:desc", "aaa", "bbb"}, // aaa has later Updated time
	}
	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			list := parseListJSON(t, runCmd(t, []string{"list", "--json", "--sort", tt.sort}))
			if len(list.Items) != 2 {
				t.Fatalf("len(list.Items) = %d, want 2", len(list.Items))
			}
			if list.Items[0].ID != tt.wantFirst || list.Items[1].ID != tt.wantSec {
				t.Errorf("list --sort %s = [%v, %v]; want [%v, %v]",
					tt.sort, list.Items[0].ID, list.Items[1].ID, tt.wantFirst, tt.wantSec)
			}
		})
	}
}

// TestListLimitOffset consolidates --limit and --limit/--offset tests that share a three-item fixture.
func TestListLimitOffset(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	if err := wn.InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	for _, item := range []*wn.Item{
		{ID: "aaa", Description: "first", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "bbb", Description: "second", Created: now, Updated: now, Log: createdLog(now)},
		{ID: "ccc", Description: "third", Created: now, Updated: now, Log: createdLog(now)},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	tests := []struct {
		name    string
		args    []string
		wantLen int
		wantIDs []string
	}{
		{
			"limit-2",
			[]string{"list", "--json", "--sort", "alpha", "--limit", "2"},
			2, []string{"aaa", "bbb"},
		},
		{
			"limit-1-offset-1",
			[]string{"list", "--json", "--sort", "alpha", "--limit", "1", "--offset", "1"},
			1, []string{"bbb"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := parseListJSON(t, runCmd(t, tt.args))
			if len(list.Items) != tt.wantLen {
				t.Fatalf("list %s: len = %d, want %d", tt.name, len(list.Items), tt.wantLen)
			}
			for i, id := range tt.wantIDs {
				if list.Items[i].ID != id {
					t.Errorf("list %s: item[%d] = %v, want %v", tt.name, i, list.Items[i].ID, id)
				}
			}
		})
	}
}

// TestListGroup consolidates list --group tests for tags and status grouping.
func TestListGroup(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name  string
		items []*wn.Item
		args  []string
		check func(*testing.T, string)
	}{
		{
			name: "by-tags",
			items: []*wn.Item{
				{ID: "aaa111", Description: "agent task", Created: now, Updated: now, Tags: []string{"agent"}, Log: createdLog(now)},
				{ID: "bbb222", Description: "backend task", Created: now, Updated: now, Tags: []string{"backend"}, Log: createdLog(now)},
				{ID: "ccc333", Description: "another agent task", Created: now, Updated: now, Tags: []string{"agent"}, Log: createdLog(now)},
			},
			args: []string{"list", "--group", "tags"},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "#agent") {
					t.Errorf("output should contain #agent group header; got:\n%s", out)
				}
				if !strings.Contains(out, "#backend") {
					t.Errorf("output should contain #backend group header; got:\n%s", out)
				}
				agentHeaderIdx := strings.Index(out, "--- #agent ---")
				backendHeaderIdx := strings.Index(out, "--- #backend ---")
				if agentHeaderIdx < 0 {
					t.Fatalf("expected '--- #agent ---' header; got:\n%s", out)
				}
				if backendHeaderIdx < 0 {
					t.Fatalf("expected '--- #backend ---' header; got:\n%s", out)
				}
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
			},
		},
		{
			name: "by-tags-untagged",
			items: []*wn.Item{
				{ID: "aaa111", Description: "tagged task", Created: now, Updated: now, Tags: []string{"agent"}, Log: createdLog(now)},
				{ID: "bbb222", Description: "untagged task", Created: now, Updated: now, Tags: nil, Log: createdLog(now)},
			},
			args: []string{"list", "--group", "tags"},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "(no tags)") {
					t.Errorf("output should contain '(no tags)' group header for untagged items; got:\n%s", out)
				}
				if !strings.Contains(out, "#agent") {
					t.Errorf("output should contain '#agent' group header; got:\n%s", out)
				}
				if !strings.Contains(out, "bbb222") {
					t.Errorf("output should contain untagged item id; got:\n%s", out)
				}
			},
		},
		{
			name: "by-status",
			items: []*wn.Item{
				{ID: "aaa111", Description: "done task", Created: now, Updated: now, Done: true, Log: createdLog(now)},
				{ID: "bbb222", Description: "undone task", Created: now, Updated: now, Log: createdLog(now)},
				{ID: "ccc333", Description: "another undone task", Created: now, Updated: now, Log: createdLog(now)},
			},
			args: []string{"list", "--all", "--group", "status"},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "--- done ---") {
					t.Errorf("output should contain '--- done ---' header; got:\n%s", out)
				}
				if !strings.Contains(out, "--- undone ---") {
					t.Errorf("output should contain '--- undone ---' header; got:\n%s", out)
				}
				undoneHeaderIdx := strings.Index(out, "--- undone ---")
				bbb222Idx := strings.Index(out, "bbb222")
				ccc333Idx := strings.Index(out, "ccc333")
				if bbb222Idx < undoneHeaderIdx || ccc333Idx < undoneHeaderIdx {
					t.Errorf("undone items should appear after '--- undone ---' header; got:\n%s", out)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := wn.InitRoot(dir); err != nil {
				t.Fatalf("InitRoot: %v", err)
			}
			store, err := wn.NewFileStore(dir)
			if err != nil {
				t.Fatalf("NewFileStore: %v", err)
			}
			for _, item := range tt.items {
				if err := store.Put(item); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(dir)
			tt.check(t, runCmd(t, tt.args))
		})
	}
}

func TestListGroupInvalidKey(t *testing.T) {
	dir, _ := setupWnRoot(t)
	t.Chdir(dir)
	root := newRootCmd()
	root.SetArgs([]string{"list", "--group", "bogus"})
	if err := root.Execute(); err == nil {
		t.Error("list --group bogus should return an error")
	}
}

func TestListGroupWithJSON(t *testing.T) {
	dir, _ := setupWnRoot(t)
	t.Chdir(dir)
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

// TestListViewAtSyntax consolidates list @view tests for filtering, error cases, and
// view options like --group and --sort.
func TestListViewAtSyntax(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		args    []string
		wantErr bool
		check   func(*testing.T, string)
	}{
		{
			name: "filters-by-tag",
			setup: func(t *testing.T, dir string) {
				now := time.Now().UTC()
				store, err := wn.NewFileStore(dir)
				if err != nil {
					t.Fatalf("NewFileStore: %v", err)
				}
				for _, it := range []*wn.Item{
					{ID: "aaa111", Description: "agent item", Tags: []string{"agent"}, Created: now, Updated: now, Log: createdLog(now)},
					{ID: "bbb222", Description: "other item", Created: now, Updated: now, Log: createdLog(now)},
				} {
					if err := store.Put(it); err != nil {
						t.Fatal(err)
					}
				}
				writeViewSettings(t, dir, map[string]string{"agent": "--tag agent --json"})
			},
			args: []string{"list", "@agent"},
			check: func(t *testing.T, out string) {
				list := parseListJSON(t, out)
				if len(list.Items) != 1 {
					t.Fatalf("len(Items) = %d, want 1 (only tagged 'agent')", len(list.Items))
				}
				if list.Items[0].ID != "aaa111" {
					t.Errorf("Items[0].ID = %q, want aaa111", list.Items[0].ID)
				}
			},
		},
		{
			name: "unknown-view",
			setup: func(t *testing.T, dir string) {
				writeViewSettings(t, dir, map[string]string{"agent": "--tag agent"})
			},
			args:    []string{"list", "@nosuchview"},
			wantErr: true,
		},
		{
			name: "with-sort-and-group",
			setup: func(t *testing.T, dir string) {
				now := time.Now().UTC()
				store, err := wn.NewFileStore(dir)
				if err != nil {
					t.Fatalf("NewFileStore: %v", err)
				}
				for _, it := range []*wn.Item{
					{ID: "ccc333", Description: "cc item", Tags: []string{"backend"}, Created: now, Updated: now, Log: createdLog(now)},
					{ID: "ddd444", Description: "dd item", Tags: []string{"frontend"}, Created: now, Updated: now, Log: createdLog(now)},
					{ID: "eee555", Description: "ee item", Created: now, Updated: now, Log: createdLog(now)},
				} {
					if err := store.Put(it); err != nil {
						t.Fatal(err)
					}
				}
				writeViewSettings(t, dir, map[string]string{"bygroup": "--all --group tags"})
			},
			args: []string{"list", "@bygroup"},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "---") {
					t.Errorf("output should contain group headers, got: %s", out)
				}
			},
		},
		{
			name: "no-views-configured",
			setup: func(t *testing.T, dir string) {
				// Isolate from user settings so no views are configured.
				t.Setenv("WN_CONFIG_DIR", t.TempDir())
			},
			args:    []string{"list", "@agent"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := wn.InitRoot(dir); err != nil {
				t.Fatalf("InitRoot: %v", err)
			}
			tt.setup(t, dir)
			t.Chdir(dir)

			if tt.wantErr {
				root := newRootCmd()
				root.SetArgs(tt.args)
				if err := root.Execute(); err == nil {
					t.Errorf("list %v: expected error, got nil", tt.args)
				}
				return
			}
			out := runCmd(t, tt.args)
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestShowIncludesNotes(t *testing.T) {
	dir, itemID := setupWnRoot(t)
	t.Chdir(dir)
	root := newRootCmd()
	root.SetArgs([]string{"note", "add", "see-file", itemID, "-m", "see file X"})
	if err := root.Execute(); err != nil {
		t.Fatalf("note add: %v", err)
	}
	out := runCmd(t, []string{"show", "--json", itemID})
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
	t.Chdir(dir)

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
	t.Chdir(dir)
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
	t.Chdir(dir)

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
	t.Chdir(dir)

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
	t.Chdir(dir)

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

	out := runCmd(t, []string{"show", "--fields", "notes", itemID})
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
