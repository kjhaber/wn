package wn

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStore_PutGetListDelete(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	item := &Item{
		ID:          "abc123",
		Description: "test task",
		Created:     time.Now().UTC(),
		Updated:     time.Now().UTC(),
		Done:        false,
		Tags:        []string{"a"},
		DependsOn:   nil,
		Log:         []LogEntry{{At: time.Now().UTC(), Kind: "created"}},
	}

	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != item.Description {
		t.Errorf("Get description = %q, want %q", got.Description, item.Description)
	}

	items, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List len = %d, want 1", len(items))
	}
	if items[0].ID != "abc123" {
		t.Errorf("List[0].ID = %q, want abc123", items[0].ID)
	}

	if err := store.Delete("abc123"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = store.Get("abc123")
	if err == nil {
		t.Error("Get after Delete should error")
	}
	items, _ = store.List()
	if len(items) != 0 {
		t.Errorf("List after Delete len = %d, want 0", len(items))
	}
}

func TestFileStore_ListEmpty(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("List empty store: len = %d, want 0", len(items))
	}
}

func TestNewFileStore_RequiresItemsDir(t *testing.T) {
	root := t.TempDir()
	wnDir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatal(err)
	}
	// .wn exists but no .wn/items
	_, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore with .wn but no items: expected to create items dir, got %v", err)
	}
	// After NewFileStore, items dir should exist
	itemsPath := filepath.Join(root, ".wn", "items")
	if info, err := os.Stat(itemsPath); err != nil || !info.IsDir() {
		t.Errorf("expected .wn/items to exist: err=%v isDir=%v", err, info != nil && info.IsDir())
	}
}

func TestGenerateID_Unique(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateID(store)
		if err != nil {
			t.Fatalf("GenerateID: %v", err)
		}
		if len(id) != 6 {
			t.Errorf("GenerateID() = %q len %d, want 6", id, len(id))
		}
		for _, c := range id {
			if (c >= 'a' && c <= 'f') || (c >= '0' && c <= '9') {
				continue
			}
			t.Errorf("GenerateID() = %q has non-hex char %c", id, c)
		}
		if seen[id] {
			t.Errorf("duplicate ID %q", id)
		}
		seen[id] = true
	}
}
