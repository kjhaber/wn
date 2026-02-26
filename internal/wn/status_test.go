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
