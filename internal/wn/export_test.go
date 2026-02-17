package wn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExportImport(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "exported task",
		Created:     now,
		Updated:     now,
		Tags:        []string{"x"},
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.json")
	if err := Export(store, path); err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Import into a new store (replace)
	root2 := t.TempDir()
	store2, err := NewFileStore(root2)
	if err != nil {
		t.Fatal(err)
	}
	if err := ImportReplace(store2, path); err != nil {
		t.Fatalf("ImportReplace: %v", err)
	}
	got, err := store2.Get("abc123")
	if err != nil {
		t.Fatalf("Get after import: %v", err)
	}
	if got.Description != item.Description {
		t.Errorf("description = %q, want %q", got.Description, item.Description)
	}
}

func TestStoreHasItems(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	has, err := StoreHasItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("empty store should report no items")
	}
	item := &Item{ID: "x", Created: time.Now().UTC(), Updated: time.Now().UTC()}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	has, err = StoreHasItems(store)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("store with item should report has items")
	}
}

func TestExport_SchemaVersion(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "out.json")
	if err := Export(store, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var exp ExportData
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatal(err)
	}
	if exp.Version != ExportSchemaVersion {
		t.Errorf("version = %d, want %d", exp.Version, ExportSchemaVersion)
	}
}

func TestImportReplace_FileNotFound(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	err = ImportReplace(store, filepath.Join(root, "nonexistent.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}
