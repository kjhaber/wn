package wn

import (
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
	got, err := ClaimNextItem(store, root, 30*time.Minute, "")
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

	got, err := ClaimNextItem(store, root, 30*time.Minute, "runner1")
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

	got, err := ClaimNextItem(store, root, 30*time.Minute, "")
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

func TestResolveBranchName(t *testing.T) {
	item := &Item{ID: "abc123", Description: "Add feature"}
	if got := resolveBranchName(item); got != "wn-abc123-add-feature" {
		t.Errorf("resolveBranchName = %q, want wn-abc123-add-feature", got)
	}
	item.Notes = []Note{{Name: "branch", Body: "reuse-me"}}
	if got := resolveBranchName(item); got != "reuse-me" {
		t.Errorf("resolveBranchName with note = %q, want reuse-me", got)
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

	got, err := ClaimNextItem(store, root, 30*time.Minute, "")
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
