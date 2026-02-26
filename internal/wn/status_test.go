package wn

import (
	"testing"
	"time"
)

func TestValidStatus(t *testing.T) {
	for _, s := range ValidStatuses {
		if !ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = false, want true", s)
		}
	}
	if ValidStatus("") {
		t.Error("ValidStatus(\"\") = true, want false")
	}
	if ValidStatus("invalid") {
		t.Error("ValidStatus(\"invalid\") = true, want false")
	}
}

func TestSetStatus_undone(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "item",
		Created:     now,
		Updated:     now,
		Done:        true,
		DoneStatus:  DoneStatusSuspend,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := SetStatus(store, "abc123", StatusUndone, StatusOpts{}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get("abc123")
	if got.Done || got.DoneStatus != "" || got.ReviewReady {
		t.Errorf("after SetStatus undone: Done=%v DoneStatus=%q ReviewReady=%v", got.Done, got.DoneStatus, got.ReviewReady)
	}
}

func TestSetStatus_suspend(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "item",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := SetStatus(store, "abc123", StatusSuspend, StatusOpts{DoneMessage: "deferred"}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get("abc123")
	if !got.Done || got.DoneStatus != DoneStatusSuspend || got.DoneMessage != "deferred" {
		t.Errorf("after SetStatus suspend: Done=%v DoneStatus=%q DoneMessage=%q", got.Done, got.DoneStatus, got.DoneMessage)
	}
}

func TestSetStatus_closed(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "item",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := SetStatus(store, "abc123", StatusClosed, StatusOpts{}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get("abc123")
	if !got.Done || got.DoneStatus != DoneStatusClosed {
		t.Errorf("after SetStatus closed: Done=%v DoneStatus=%q", got.Done, got.DoneStatus)
	}
}

func TestSetStatus_closed_with_duplicate_of(t *testing.T) {
	root := t.TempDir()
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	orig := &Item{ID: "orig12", Description: "original", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	dup := &Item{ID: "dup456", Description: "duplicate", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	for _, it := range []*Item{orig, dup} {
		if err := store.Put(it); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if err := SetStatus(store, "dup456", StatusClosed, StatusOpts{DuplicateOf: "orig12"}); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	item, err := store.Get("dup456")
	if err != nil {
		t.Fatalf("Get dup456: %v", err)
	}
	if !item.Done || item.DoneStatus != DoneStatusClosed {
		t.Errorf("after status closed --duplicate-of: Done=%v DoneStatus=%q", item.Done, item.DoneStatus)
	}
	idx := item.NoteIndexByName(NoteNameDuplicateOf)
	if idx < 0 {
		t.Fatalf("note %q not found", NoteNameDuplicateOf)
	}
	if item.Notes[idx].Body != "orig12" {
		t.Errorf("duplicate-of body = %q, want orig12", item.Notes[idx].Body)
	}
}

func TestSetStatus_closed_duplicate_of_rejects_same_id(t *testing.T) {
	root := t.TempDir()
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{ID: "abc123", Description: "only", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = SetStatus(store, "abc123", StatusClosed, StatusOpts{DuplicateOf: "abc123"})
	if err == nil {
		t.Error("SetStatus(closed, DuplicateOf: self) should error")
	}
}

func TestSetStatus_closed_duplicate_of_rejects_missing_original(t *testing.T) {
	root := t.TempDir()
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{ID: "dup456", Description: "dup", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = SetStatus(store, "dup456", StatusClosed, StatusOpts{DuplicateOf: "nonexistent"})
	if err == nil {
		t.Error("SetStatus(closed, DuplicateOf: missing original) should error")
	}
}

func TestSetStatus_invalid(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := &Item{ID: "abc123", Description: "x", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	err = SetStatus(store, "abc123", "invalid", StatusOpts{})
	if err == nil {
		t.Error("SetStatus(invalid) expected error")
	}
}
