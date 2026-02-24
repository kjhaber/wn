package wn

import (
	"testing"
	"time"
)

func TestDependents_Empty(t *testing.T) {
	root := t.TempDir()
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	_ = store.Put(&Item{ID: "aa1111", Description: "a", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}})

	ids, err := Dependents(store, "aa1111")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("Dependents(aa1111) = %v, want []", ids)
	}
}

func TestDependents_OneDependent(t *testing.T) {
	root := t.TempDir()
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	_ = store.Put(&Item{ID: "aa1111", Description: "a", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}})
	_ = store.Put(&Item{ID: "bb2222", Description: "b", Created: now, Updated: now, DependsOn: []string{"aa1111"}, Log: []LogEntry{{At: now, Kind: "created"}}})

	ids, err := Dependents(store, "aa1111")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(ids) != 1 || ids[0] != "bb2222" {
		t.Errorf("Dependents(aa1111) = %v, want [bb2222]", ids)
	}
}

func TestDependents_NonexistentID(t *testing.T) {
	root := t.TempDir()
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ids, err := Dependents(store, "nonexistent")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("Dependents(nonexistent) = %v, want []", ids)
	}
}
