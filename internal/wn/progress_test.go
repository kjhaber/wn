package wn

import (
	"testing"
	"time"
)

func TestItemListStatus(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		item *Item
		now  time.Time
		want string
	}{
		{"undone", &Item{Done: false}, now, "undone"},
		{"claimed", &Item{Done: false, InProgressUntil: future}, now, "claimed"},
		{"review", &Item{Done: false, ReviewReady: true}, now, "review"},
		{"done (default)", &Item{Done: true}, now, "done"},
		{"done (explicit)", &Item{Done: true, DoneStatus: DoneStatusDone}, now, "done"},
		{"closed", &Item{Done: true, DoneStatus: DoneStatusClosed}, now, "closed"},
		{"suspend", &Item{Done: true, DoneStatus: DoneStatusSuspend}, now, "suspend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ItemListStatus(tt.item, tt.now); got != tt.want {
				t.Errorf("ItemListStatus() = %q, want %q", got, tt.want)
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
