package wn

import (
	"bytes"
	"os/exec"
	"testing"
	"time"
)

func TestBranchSlug(t *testing.T) {
	tests := []struct {
		desc string
		in   string
		want string
	}{
		{"single word", "Add feature", "add-feature"},
		{"lowercase", "add feature", "add-feature"},
		{"special chars", "Fix bug #123 (urgent!)", "fix-bug-123-urgent"},
		{"multiple spaces/dashes", "  one   two---three  ", "one-two-three"},
		{"empty", "", ""},
		{"only symbols", "!!!", ""},
		{"truncate long", "This is a very long description that should be truncated to about thirty characters", "this-is-a-very-long-descriptio"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := BranchSlug(tt.in)
			if got != tt.want {
				t.Errorf("BranchSlug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestClaimNextItem_emptyReturnsNil(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	got, err := ClaimNextItem(store, root, 30*time.Minute, "", "")
	if err != nil {
		t.Fatalf("ClaimNextItem: %v", err)
	}
	if got != nil {
		t.Errorf("ClaimNextItem(empty store) = %v, want nil", got)
	}
}

func TestClaimNextItem_claimsFirstUndone(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "first task",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}

	got, err := ClaimNextItem(store, root, 30*time.Minute, "runner1", "")
	if err != nil {
		t.Fatalf("ClaimNextItem: %v", err)
	}
	if got == nil {
		t.Fatal("ClaimNextItem returned nil, want item")
		return
	}
	if got.ID != "abc123" {
		t.Errorf("ClaimNextItem returned id %q, want abc123", got.ID)
	}

	// Item should now be claimed in store
	updated, err := store.Get("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if updated.InProgressUntil.IsZero() {
		t.Error("item should be claimed (InProgressUntil set)")
	}
	if updated.InProgressBy != "runner1" {
		t.Errorf("InProgressBy = %q, want runner1", updated.InProgressBy)
	}
}

func TestClaimNextItem_skipsReviewReady(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "review-ready",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}

	got, err := ClaimNextItem(store, root, 30*time.Minute, "", "")
	if err != nil {
		t.Fatalf("ClaimNextItem: %v", err)
	}
	if got != nil {
		t.Errorf("ClaimNextItem(review-ready only) = %v, want nil", got)
	}
}

func TestExpandPromptTemplate(t *testing.T) {
	item := &Item{ID: "abc123", Description: "Add feature\nWith details"}
	got, err := ExpandPromptTemplate("{{.Description}}", item, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != item.Description {
		t.Errorf("got %q", got)
	}
	got, err = ExpandPromptTemplate("Item {{.ItemID}}: {{.FirstLine}}", item, "/wt", "wn-abc-add-feature")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Item abc123: Add feature" {
		t.Errorf("got %q", got)
	}
}

func TestExpandCommandTemplate(t *testing.T) {
	got, err := ExpandCommandTemplate("echo {{.Prompt}}", "hello world", "abc", "/wt", "br")
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo hello world" {
		t.Errorf("got %q", got)
	}
}

// TestExpandCommandTemplate_escapesQuotes verifies that prompts containing double
// quotes, single quotes, and backslashes are shell-escaped so the command can
// be safely passed to sh -c (fixes agent-orch failure when item description
// contains e.g. "resolved", "won't fix").
func TestExpandCommandTemplate_escapesQuotes(t *testing.T) {
	prompt := `Add a "resolved" state similar to "done". Can be used for "won't fix", "duplicate".`
	tpl := `printf '%s' "{{.Prompt}}"`
	got, err := ExpandCommandTemplate(tpl, prompt, "abc", "/wt", "br")
	if err != nil {
		t.Fatal(err)
	}
	// Must not contain unescaped " in the middle (would break sh -c)
	if got == `printf '%s' "Add a "resolved" state similar to "done". Can be used for "won't fix", "duplicate"."` {
		t.Fatal("prompt was not escaped; unescaped quotes would break sh -c")
	}
	// Run through sh -c to verify it executes without syntax error and produces correct output
	cmd := exec.Command("sh", "-c", got)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("sh -c failed (shell syntax error): %v.\nExpanded command: %s", err, got)
	}
	if stdout.String() != prompt {
		t.Errorf("sh -c output = %q, want %q (escaped prompt did not round-trip)", stdout.String(), prompt)
	}
}

// TestExpandCommandTemplate_escapesBackticksAndDollar verifies that prompts
// containing backticks and $ are shell-escaped so they are not interpreted as
// command substitution or variable expansion when passed to sh -c (fixes
// agent-orch failure when item description contains e.g. `--wid <id>`).
func TestExpandCommandTemplate_escapesBackticksAndDollar(t *testing.T) {
	prompt := "wn tag add <tag-name> [--wid <id>]\n`--wid <id>` should be used. Cost $5 or $(id) risky."
	tpl := `printf '%s' "{{.Prompt}}"`
	got, err := ExpandCommandTemplate(tpl, prompt, "abc", "/wt", "br")
	if err != nil {
		t.Fatal(err)
	}
	// Run through sh -c to verify it executes without command-substitution or expansion
	cmd := exec.Command("sh", "-c", got)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("sh -c failed (shell syntax/command substitution): %v.\nExpanded command: %s", err, got)
	}
	if stdout.String() != prompt {
		t.Errorf("sh -c output = %q, want %q (backticks/$ did not round-trip safely)", stdout.String(), prompt)
	}
}

// TestExpandCommandTemplate_escapesItemIDWorktreeBranch verifies that ItemID,
// Worktree, and Branch are shell-escaped so values with metacharacters (from
// import or branch notes) cannot inject commands when passed to sh -c.
func TestExpandCommandTemplate_escapesItemIDWorktreeBranch(t *testing.T) {
	itemID := `x; rm -rf /`
	worktree := `/tmp/worktree with spaces`
	branch := `main'$(id)'`
	tpl := `printf 'id=%s wd=%s br=%s' {{.ItemID}} {{.Worktree}} {{.Branch}}`
	got, err := ExpandCommandTemplate(tpl, "prompt", itemID, worktree, branch)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", got)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("sh -c failed (shell syntax/injection): %v.\nExpanded command: %s", err, got)
	}
	want := "id=" + itemID + " wd=" + worktree + " br=" + branch
	if stdout.String() != want {
		t.Errorf("sh -c output = %q, want %q (ItemID/Worktree/Branch did not round-trip safely)", stdout.String(), want)
	}
}

func TestResolveBranchName(t *testing.T) {
	item := &Item{ID: "abc123", Description: "Add feature"}
	if got := resolveBranchName(item, ""); got != "wn-abc123-add-feature" {
		t.Errorf("resolveBranchName(no prefix) = %q, want wn-abc123-add-feature", got)
	}
	if got := resolveBranchName(item, "keith/"); got != "keith/wn-abc123-add-feature" {
		t.Errorf("resolveBranchName(keith/) = %q, want keith/wn-abc123-add-feature", got)
	}
	item.Notes = []Note{{Name: "branch", Body: "reuse-me"}}
	if got := resolveBranchName(item, "keith/"); got != "reuse-me" {
		t.Errorf("resolveBranchName with note (prefix ignored) = %q, want reuse-me", got)
	}
}

func TestClaimItem(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "single task",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = ClaimItem(store, root, "abc123", 30*time.Minute, "runner1")
	if err != nil {
		t.Fatalf("ClaimItem: %v", err)
	}
	updated, err := store.Get("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if updated.InProgressUntil.IsZero() {
		t.Error("item should be claimed (InProgressUntil set)")
	}
	if updated.InProgressBy != "runner1" {
		t.Errorf("InProgressBy = %q, want runner1", updated.InProgressBy)
	}
}

func TestClaimNextItem_topoOrder(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	put := func(id, desc string, deps []string) {
		it := &Item{
			ID:          id,
			Description: desc,
			Created:     now,
			Updated:     now,
			DependsOn:   deps,
			Log:         []LogEntry{{At: now, Kind: "created"}},
		}
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	put("first", "no deps", nil)
	put("second", "depends on first", []string{"first"})

	got, err := ClaimNextItem(store, root, 30*time.Minute, "", "")
	if err != nil {
		t.Fatalf("ClaimNextItem: %v", err)
	}
	if got == nil {
		t.Fatal("ClaimNextItem returned nil")
		return
	}
	if got.ID != "first" {
		t.Errorf("ClaimNextItem returned %q (first in topo order), want first", got.ID)
	}
}

func TestClaimNextItem_tagFilter(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	put := func(id, desc string, tags []string) {
		it := &Item{
			ID:          id,
			Description: desc,
			Created:     now,
			Updated:     now,
			Tags:        tags,
			Log:         []LogEntry{{At: now, Kind: "created"}},
		}
		if err := store.Put(it); err != nil {
			t.Fatal(err)
		}
	}
	put("a", "item a", []string{"agent"})
	put("b", "item b", nil)
	put("c", "item c", []string{"agent"})

	// No tag: should get first in topo order (a, b, c by id/order)
	got, err := ClaimNextItem(store, root, 30*time.Minute, "", "")
	if err != nil {
		t.Fatalf("ClaimNextItem(no tag): %v", err)
	}
	if got == nil || got.ID != "a" {
		t.Errorf("ClaimNextItem(no tag) = %v, want item a", got)
	}
	// Release so we can claim again
	_ = store.UpdateItem("a", func(it *Item) (*Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		return it, nil
	})

	// With tag "agent": only a and c are candidates; first in topo order is a
	got, err = ClaimNextItem(store, root, 30*time.Minute, "", "agent")
	if err != nil {
		t.Fatalf("ClaimNextItem(tag=agent): %v", err)
	}
	if got == nil || got.ID != "a" {
		t.Errorf("ClaimNextItem(tag=agent) = %v, want item a", got)
	}
	// Mark a as review-ready so it's no longer in UndoneItems; then only c has tag "agent"
	_ = store.UpdateItem("a", func(it *Item) (*Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.ReviewReady = true
		return it, nil
	})

	// With tag "agent" again: only c left in undone with that tag
	got, err = ClaimNextItem(store, root, 30*time.Minute, "", "agent")
	if err != nil {
		t.Fatalf("ClaimNextItem(tag=agent) 2nd: %v", err)
	}
	if got == nil || got.ID != "c" {
		t.Errorf("ClaimNextItem(tag=agent) 2nd = %v, want item c", got)
	}

	// Tag that no item has: no candidate (c is in progress; undone = b only, b has no tag)
	got, err = ClaimNextItem(store, root, 30*time.Minute, "", "nonexistent")
	if err != nil {
		t.Fatalf("ClaimNextItem(tag=nonexistent): %v", err)
	}
	if got != nil {
		t.Errorf("ClaimNextItem(tag=nonexistent) = %v, want nil", got)
	}
}
