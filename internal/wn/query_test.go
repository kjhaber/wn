package wn

import (
	"testing"
	"time"
)

// boolPtr is a test helper to get a pointer to a bool literal.
func boolPtr(b bool) *bool { return &b }

func newQueryStore(t *testing.T) Store {
	t.Helper()
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return store
}

func putItem(t *testing.T, store Store, it *Item) {
	t.Helper()
	if err := store.Put(it); err != nil {
		t.Fatalf("store.Put(%s): %v", it.ID, err)
	}
}

func newItem(id string, now time.Time) *Item {
	return &Item{ID: id, Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
}

func TestQueryItems_Done(t *testing.T) {
	now := time.Now().UTC()
	store := newQueryStore(t)
	putItem(t, store, newItem("undone1", now))
	putItem(t, store, newItem("undone2", now))
	it := newItem("done1", now)
	it.Done = true
	putItem(t, store, it)

	t.Run("Done=false returns only undone", func(t *testing.T) {
		items, err := QueryItems(store, ItemQuery{Done: boolPtr(false)})
		if err != nil {
			t.Fatalf("QueryItems: %v", err)
		}
		for _, it := range items {
			if it.Done {
				t.Errorf("QueryItems(Done=false) returned done item %s", it.ID)
			}
		}
		if len(items) != 2 {
			t.Errorf("QueryItems(Done=false): got %d items, want 2", len(items))
		}
	})

	t.Run("Done=true returns only done", func(t *testing.T) {
		items, err := QueryItems(store, ItemQuery{Done: boolPtr(true)})
		if err != nil {
			t.Fatalf("QueryItems: %v", err)
		}
		if len(items) != 1 || items[0].ID != "done1" {
			t.Errorf("QueryItems(Done=true): got %v, want [done1]", itemIDs(items))
		}
	})

	t.Run("Done=nil returns all", func(t *testing.T) {
		items, err := QueryItems(store, ItemQuery{})
		if err != nil {
			t.Fatalf("QueryItems: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("QueryItems(Done=nil): got %d items, want 3", len(items))
		}
	})
}

func TestQueryItems_ReviewReady(t *testing.T) {
	now := time.Now().UTC()
	store := newQueryStore(t)
	putItem(t, store, newItem("undone1", now))
	rr1 := newItem("rr1", now)
	rr1.ReviewReady = true
	putItem(t, store, rr1)
	rr2 := newItem("rr2", now)
	rr2.ReviewReady = true
	putItem(t, store, rr2)

	t.Run("ReviewReady=true returns only review-ready", func(t *testing.T) {
		items, err := QueryItems(store, ItemQuery{Done: boolPtr(false), ReviewReady: boolPtr(true)})
		if err != nil {
			t.Fatalf("QueryItems: %v", err)
		}
		for _, it := range items {
			if !it.ReviewReady {
				t.Errorf("QueryItems(ReviewReady=true) returned non-review-ready item %s", it.ID)
			}
		}
		if len(items) != 2 {
			t.Errorf("QueryItems(ReviewReady=true): got %d items, want 2", len(items))
		}
	})

	t.Run("ReviewReady=false excludes review-ready", func(t *testing.T) {
		items, err := QueryItems(store, ItemQuery{Done: boolPtr(false), ReviewReady: boolPtr(false)})
		if err != nil {
			t.Fatalf("QueryItems: %v", err)
		}
		for _, it := range items {
			if it.ReviewReady {
				t.Errorf("QueryItems(ReviewReady=false) returned review-ready item %s", it.ID)
			}
		}
		if len(items) != 1 || items[0].ID != "undone1" {
			t.Errorf("QueryItems(ReviewReady=false): got %v, want [undone1]", itemIDs(items))
		}
	})

	t.Run("ReviewReady=nil includes both", func(t *testing.T) {
		items, err := QueryItems(store, ItemQuery{Done: boolPtr(false)})
		if err != nil {
			t.Fatalf("QueryItems: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("QueryItems(ReviewReady=nil): got %d items, want 3", len(items))
		}
	})
}

func TestQueryItems_ExcludesPromptReady(t *testing.T) {
	now := time.Now().UTC()
	store := newQueryStore(t)
	putItem(t, store, newItem("undone1", now))
	pr := newItem("prompt1", now)
	pr.PromptReady = true
	putItem(t, store, pr)

	items, err := QueryItems(store, ItemQuery{Done: boolPtr(false), ReviewReady: boolPtr(false)})
	if err != nil {
		t.Fatalf("QueryItems: %v", err)
	}
	for _, it := range items {
		if it.PromptReady {
			t.Errorf("QueryItems(ReviewReady=false) returned prompt-ready item %s", it.ID)
		}
	}
	if len(items) != 1 || items[0].ID != "undone1" {
		t.Errorf("QueryItems(ReviewReady=false): got %v, want [undone1]", itemIDs(items))
	}
}

func TestQueryItems_ExcludesInProgress(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	store := newQueryStore(t)
	putItem(t, store, newItem("undone1", now))
	ip := newItem("inprog1", now)
	ip.InProgressUntil = future
	ip.InProgressBy = "agent"
	putItem(t, store, ip)

	items, err := QueryItems(store, ItemQuery{Done: boolPtr(false)})
	if err != nil {
		t.Fatalf("QueryItems: %v", err)
	}
	for _, it := range items {
		if it.ID == "inprog1" {
			t.Errorf("QueryItems(Done=false) included in-progress item inprog1")
		}
	}
	if len(items) != 1 || items[0].ID != "undone1" {
		t.Errorf("QueryItems(Done=false): got %v, want [undone1]", itemIDs(items))
	}
}

func TestQueryItems_ClearsExpiredInProgress(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	store := newQueryStore(t)
	it := newItem("expired1", now)
	it.InProgressUntil = past
	it.InProgressBy = "agent"
	putItem(t, store, it)

	items, err := QueryItems(store, ItemQuery{Done: boolPtr(false)})
	if err != nil {
		t.Fatalf("QueryItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "expired1" {
		t.Fatalf("QueryItems: expected expired item to be returned after clearing, got %v", itemIDs(items))
	}
	if !items[0].InProgressUntil.IsZero() {
		t.Errorf("QueryItems: expired in-progress item was not cleared")
	}
	stored, err := store.Get("expired1")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !stored.InProgressUntil.IsZero() {
		t.Errorf("QueryItems: expired in-progress not cleared in store")
	}
	var hasExpiredLog bool
	for _, e := range stored.Log {
		if e.Kind == "in_progress_expired" {
			hasExpiredLog = true
			break
		}
	}
	if !hasExpiredLog {
		t.Error("expected in_progress_expired log entry")
	}
}

func TestQueryItems_Tags(t *testing.T) {
	now := time.Now().UTC()
	store := newQueryStore(t)
	ab := newItem("ab", now)
	ab.Tags = []string{"a", "b"}
	putItem(t, store, ab)
	ac := newItem("ac", now)
	ac.Tags = []string{"a", "c"}
	putItem(t, store, ac)
	bc := newItem("bc", now)
	bc.Tags = []string{"b", "c"}
	putItem(t, store, bc)
	putItem(t, store, newItem("none", now))

	tests := []struct {
		name     string
		tags     []string
		matchAll bool
		wantLen  int
		wantIDs  []string
	}{
		{"single tag a", []string{"a"}, false, 2, []string{"ab", "ac"}},
		{"single tag b", []string{"b"}, false, 2, []string{"ab", "bc"}},
		{"OR a|b", []string{"a", "b"}, false, 3, []string{"ab", "ac", "bc"}},
		{"AND a,b", []string{"a", "b"}, true, 1, []string{"ab"}},
		{"AND a,c", []string{"a", "c"}, true, 1, []string{"ac"}},
		{"AND a,b,c none match", []string{"a", "b", "c"}, true, 0, nil},
		{"no tags returns all", nil, false, 4, []string{"ab", "ac", "bc", "none"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := QueryItems(store, ItemQuery{Tags: tt.tags, TagsMatchAll: tt.matchAll})
			if err != nil {
				t.Fatalf("QueryItems: %v", err)
			}
			if len(items) != tt.wantLen {
				t.Fatalf("QueryItems tags=%v matchAll=%v: got %v (len=%d), want %v", tt.tags, tt.matchAll, itemIDs(items), len(items), tt.wantIDs)
			}
			wantSet := make(map[string]bool, len(tt.wantIDs))
			for _, id := range tt.wantIDs {
				wantSet[id] = true
			}
			for _, it := range items {
				if !wantSet[it.ID] {
					t.Errorf("unexpected item %s in result", it.ID)
				}
			}
		})
	}
}

func TestQueryItems_Limit(t *testing.T) {
	now := time.Now().UTC()
	store := newQueryStore(t)
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		putItem(t, store, newItem(id, now))
	}

	items, err := QueryItems(store, ItemQuery{Limit: 3})
	if err != nil {
		t.Fatalf("QueryItems: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("QueryItems(Limit=3): got %d items, want 3", len(items))
	}
}

func TestQueryItems_LimitZeroMeansNoLimit(t *testing.T) {
	now := time.Now().UTC()
	store := newQueryStore(t)
	for _, id := range []string{"a1", "a2", "a3"} {
		putItem(t, store, newItem(id, now))
	}

	items, err := QueryItems(store, ItemQuery{Limit: 0})
	if err != nil {
		t.Fatalf("QueryItems: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("QueryItems(Limit=0): got %d items, want 3 (no limit)", len(items))
	}
}
