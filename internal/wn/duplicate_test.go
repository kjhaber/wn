package wn

import (
	"testing"
	"time"
)

func TestMarkDuplicateOf_adds_note_and_marks_done(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	orig := &Item{ID: "orig12", Description: "original", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	dup := &Item{ID: "dup456", Description: "duplicate", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	for _, it := range []*Item{orig, dup} {
		if err := store.Put(it); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	err = MarkDuplicateOf(store, "dup456", "orig12")
	if err != nil {
		t.Fatalf("MarkDuplicateOf: %v", err)
	}

	item, err := store.Get("dup456")
	if err != nil {
		t.Fatalf("Get dup456: %v", err)
	}
	if !item.Done || item.DoneStatus != DoneStatusClosed {
		t.Errorf("item should be closed after MarkDuplicateOf: Done=%v DoneStatus=%q", item.Done, item.DoneStatus)
	}
	idx := item.NoteIndexByName(NoteNameDuplicateOf)
	if idx < 0 {
		t.Fatalf("note %q not found", NoteNameDuplicateOf)
	}
	if item.Notes[idx].Body != "orig12" {
		t.Errorf("duplicate-of body = %q, want orig12", item.Notes[idx].Body)
	}
}

func TestMarkDuplicateOf_rejects_same_id(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{ID: "abc123", Description: "only", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	err = MarkDuplicateOf(store, "abc123", "abc123")
	if err == nil {
		t.Error("MarkDuplicateOf(id, id) should error")
	}
}

func TestMarkDuplicateOf_rejects_missing_original(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{ID: "dup456", Description: "dup", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	err = MarkDuplicateOf(store, "dup456", "nonexistent")
	if err == nil {
		t.Error("MarkDuplicateOf with missing original should error")
	}
}

func TestMarkDuplicateOf_rejects_missing_item(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{ID: "orig12", Description: "original", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	err = MarkDuplicateOf(store, "missing", "orig12")
	if err == nil {
		t.Error("MarkDuplicateOf with missing item should error")
	}
}
