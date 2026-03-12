package wn

import (
	"testing"
	"time"
)

func TestItemListStatus(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)

	tests := []struct {
		name    string
		item    *Item
		now     time.Time
		blocked bool
		want    string
	}{
		{"undone", &Item{Done: false}, now, false, "undone"},
		{"claimed", &Item{Done: false, InProgressUntil: future}, now, false, "claimed"},
		{"review", &Item{Done: false, ReviewReady: true}, now, false, "review"},
		{"done (default)", &Item{Done: true}, now, false, "done"},
		{"done (explicit)", &Item{Done: true, DoneStatus: DoneStatusDone}, now, false, "done"},
		{"closed", &Item{Done: true, DoneStatus: DoneStatusClosed}, now, false, "closed"},
		{"suspend", &Item{Done: true, DoneStatus: DoneStatusSuspend}, now, false, "suspend"},
		// blocked state
		{"blocked undone", &Item{Done: false}, now, true, "blocked"},
		{"blocked claimed", &Item{Done: false, InProgressUntil: future}, now, true, "blocked"},
		{"blocked review-ready (review takes priority)", &Item{Done: false, ReviewReady: true}, now, true, "review"},
		{"blocked done (done takes priority)", &Item{Done: true}, now, true, "done"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ItemListStatus(tt.item, tt.now, tt.blocked); got != tt.want {
				t.Errorf("ItemListStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlockedSet(t *testing.T) {
	now := time.Now().UTC()

	mk := func(id string, done, reviewReady bool, deps []string) *Item {
		return &Item{ID: id, Done: done, ReviewReady: reviewReady, DependsOn: deps,
			Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	}

	tests := []struct {
		name        string
		items       []*Item
		wantBlocked []string // IDs expected in blocked set (nil = empty set)
	}{
		{"empty", nil, nil},
		{"no deps", []*Item{mk("aaa", false, false, nil)}, nil},
		{"dep is done", []*Item{
			mk("aaa", false, false, []string{"bbb"}),
			mk("bbb", true, false, nil),
		}, nil},
		{"dep is undone", []*Item{
			mk("aaa", false, false, []string{"bbb"}),
			mk("bbb", false, false, nil),
		}, []string{"aaa"}},
		{"dep is review-ready (not done)", []*Item{
			mk("aaa", false, false, []string{"bbb"}),
			mk("bbb", false, true, nil),
		}, []string{"aaa"}},
		{"done item with undone dep is not blocked", []*Item{
			mk("aaa", true, false, []string{"bbb"}),
			mk("bbb", false, false, nil),
		}, nil},
		{"review-ready item with undone dep is not blocked", []*Item{
			mk("aaa", false, true, []string{"bbb"}),
			mk("bbb", false, false, nil),
		}, nil},
		{"non-existent dep does not block", []*Item{
			mk("aaa", false, false, []string{"zzz"}),
		}, nil},
		{"multiple deps all done", []*Item{
			mk("aaa", false, false, []string{"bbb", "ccc"}),
			mk("bbb", true, false, nil),
			mk("ccc", true, false, nil),
		}, nil},
		{"multiple deps one undone", []*Item{
			mk("aaa", false, false, []string{"bbb", "ccc"}),
			mk("bbb", true, false, nil),
			mk("ccc", false, false, nil),
		}, []string{"aaa"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BlockedSet(tt.items)
			wantSet := make(map[string]bool, len(tt.wantBlocked))
			for _, id := range tt.wantBlocked {
				wantSet[id] = true
			}
			for _, id := range tt.wantBlocked {
				if !got[id] {
					t.Errorf("BlockedSet: expected %s to be blocked, got set=%v", id, got)
				}
			}
			for id := range got {
				if !wantSet[id] {
					t.Errorf("BlockedSet: unexpected blocked item %s", id)
				}
			}
		})
	}
}

func TestIsInProgress(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		item *Item
		now  time.Time
		want bool
	}{
		{"zero is not in progress", &Item{InProgressUntil: time.Time{}}, now, false},
		{"past is not in progress", &Item{InProgressUntil: past}, now, false},
		{"future is in progress", &Item{InProgressUntil: future}, now, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInProgress(tt.item, tt.now); got != tt.want {
				t.Errorf("IsInProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAvailableUndone(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		item *Item
		now  time.Time
		want bool
	}{
		{"done is not available", &Item{Done: true}, now, false},
		{"undone no in-progress is available", &Item{Done: false}, now, true},
		{"undone with future in-progress is not available", &Item{Done: false, InProgressUntil: future}, now, false},
		{"undone with past in-progress is available", &Item{Done: false, InProgressUntil: past}, now, true},
		{"review-ready is not available for agent", &Item{Done: false, ReviewReady: true}, now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAvailableUndone(tt.item, tt.now); got != tt.want {
				t.Errorf("IsAvailableUndone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUndoneItems_ExcludesInProgress(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	future := now.Add(30 * time.Minute)
	item := &Item{
		ID:              "abc123",
		Description:     "claimed",
		Created:         now,
		Updated:         now,
		InProgressUntil: future,
		InProgressBy:    "worker1",
		Log:             []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}

	undone, err := UndoneItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(undone) != 0 {
		t.Errorf("UndoneItems with in-progress item: got %d items, want 0", len(undone))
	}
}

func TestUndoneItems_ClearsExpiredInProgress(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	item := &Item{
		ID:              "abc123",
		Description:     "expired claim",
		Created:         now,
		Updated:         now,
		InProgressUntil: past,
		InProgressBy:    "worker1",
		Log:             []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}

	undone, err := UndoneItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(undone) != 1 {
		t.Fatalf("UndoneItems after expiry: got %d items, want 1", len(undone))
	}
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !got.InProgressUntil.IsZero() || got.InProgressBy != "" {
		t.Errorf("after expiry clear: InProgressUntil=%v InProgressBy=%q", got.InProgressUntil, got.InProgressBy)
	}
	var hasExpiredLog bool
	for _, e := range got.Log {
		if e.Kind == "in_progress_expired" {
			hasExpiredLog = true
			break
		}
	}
	if !hasExpiredLog {
		t.Error("expected in_progress_expired log entry")
	}
}

func TestUndoneItems_ExcludesReviewReady(t *testing.T) {
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

	undone, err := UndoneItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(undone) != 0 {
		t.Errorf("UndoneItems with review-ready item: got %d items, want 0", len(undone))
	}
}

func TestListableUndoneItems_IncludesReviewReady(t *testing.T) {
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

	listable, err := ListableUndoneItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(listable) != 1 || listable[0].ID != "abc123" {
		t.Errorf("ListableUndoneItems with review-ready: got %d items (ids %v), want 1 [abc123]", len(listable), itemIDs(listable))
	}
}

func itemIDs(items []*Item) []string {
	var s []string
	for _, it := range items {
		s = append(s, it.ID)
	}
	return s
}

// TestNextUndoneItem_withTag verifies that when tag is provided, the next item is the first undone that has that tag (dependency order).
func TestNextUndoneItem_withTag(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	// aaa no tag, bbb has tag "agent", ccc has tag "agent"; topo order aaa, bbb, ccc
	for i, id := range []string{"aaa", "bbb", "ccc"} {
		ord := i
		tags := []string{}
		if id != "aaa" {
			tags = []string{"agent"}
		}
		item := &Item{
			ID:          id,
			Description: "item " + id,
			Created:     now,
			Updated:     now,
			Order:       &ord,
			Tags:        tags,
			Log:         []LogEntry{{At: now, Kind: "created"}},
		}
		if err := store.Put(item); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}

	// No tag: first in dependency order is aaa
	next, err := NextUndoneItem(store, "")
	if err != nil {
		t.Fatalf("NextUndoneItem(no tag): %v", err)
	}
	if next == nil || next.ID != "aaa" {
		t.Errorf("NextUndoneItem(no tag) = %v, want aaa", next)
	}

	// Tag "agent": first with that tag in dependency order is bbb
	next, err = NextUndoneItem(store, "agent")
	if err != nil {
		t.Fatalf("NextUndoneItem(tag=agent): %v", err)
	}
	if next == nil || next.ID != "bbb" {
		t.Errorf("NextUndoneItem(tag=agent) = %v, want bbb", next)
	}

	// Tag "nonexistent": no item
	next, err = NextUndoneItem(store, "nonexistent")
	if err != nil {
		t.Fatalf("NextUndoneItem(tag=nonexistent): %v", err)
	}
	if next != nil {
		t.Errorf("NextUndoneItem(tag=nonexistent) = %v, want nil", next)
	}
}

func TestItemListStatus_promptReady(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	tests := []struct {
		name    string
		item    *Item
		blocked bool
		want    string
	}{
		{"prompt not blocked", &Item{Done: false, PromptReady: true}, false, "prompt"},
		{"prompt with blocked=true (prompt takes priority)", &Item{Done: false, PromptReady: true}, true, "prompt"},
		{"prompt and claimed", &Item{Done: false, PromptReady: true, InProgressUntil: future}, false, "prompt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ItemListStatus(tt.item, now, tt.blocked); got != tt.want {
				t.Errorf("ItemListStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsAvailableUndone_promptReadyExcluded(t *testing.T) {
	now := time.Now().UTC()
	if IsAvailableUndone(&Item{Done: false, PromptReady: true}, now) {
		t.Error("IsAvailableUndone(PromptReady=true) should return false")
	}
}

func TestUndoneItems_ExcludesPromptReady(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID: "abc123", Description: "prompt-ready",
		Created: now, Updated: now, PromptReady: true,
		Log: []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	undone, err := UndoneItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(undone) != 0 {
		t.Errorf("UndoneItems with PromptReady item: got %d items, want 0", len(undone))
	}
}

func TestListableUndoneItems_IncludesPromptReady(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID: "abc123", Description: "prompt-ready",
		Created: now, Updated: now, PromptReady: true,
		Log: []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	listable, err := ListableUndoneItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(listable) != 1 || listable[0].ID != "abc123" {
		t.Errorf("ListableUndoneItems with PromptReady: got %v, want [abc123]", itemIDs(listable))
	}
}

func TestBlockedSet_promptReadyItemNotBlocked(t *testing.T) {
	now := time.Now().UTC()
	items := []*Item{
		{ID: "aaa", Done: false, PromptReady: true, DependsOn: []string{"bbb"},
			Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb", Done: false,
			Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}},
	}
	got := BlockedSet(items)
	if got["aaa"] {
		t.Error("BlockedSet: prompt-ready item 'aaa' should not be in blocked set")
	}
}
