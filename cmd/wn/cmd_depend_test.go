package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kjhaber/wn/internal/wn"
)

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
