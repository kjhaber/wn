package wn

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestExport_Stdout(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "stdout1",
		Description: "exported to stdout",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	err = Export(store, "")
	os.Stdout = old
	if err != nil {
		w.Close()
		t.Fatalf("Export(store, \"\"): %v", err)
	}
	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		r.Close()
		t.Fatalf("io.Copy: %v", err)
	}
	r.Close()
	var exp ExportData
	if err := json.Unmarshal(buf.Bytes(), &exp); err != nil {
		t.Fatalf("stdout output is not valid JSON: %v", err)
	}
	if exp.Version != ExportSchemaVersion {
		t.Errorf("version = %d, want %d", exp.Version, ExportSchemaVersion)
	}
	if len(exp.Items) != 1 || exp.Items[0].ID != "stdout1" {
		t.Errorf("Items = %v, want single item stdout1", exp.Items)
	}
}

func TestImportReplace_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	err = ImportReplace(store, path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExportItems_ExportsSubset(t *testing.T) {
	now := time.Now().UTC()
	items := []*Item{
		{ID: "aaa111", Description: "first", Created: now, Updated: now, Tags: []string{"x"}, Log: []LogEntry{{At: now, Kind: "created"}}},
		{ID: "bbb222", Description: "second", Created: now, Updated: now, Done: true, Log: []LogEntry{{At: now, Kind: "created"}}},
	}
	path := filepath.Join(t.TempDir(), "subset.json")
	if err := ExportItems(items, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var exp ExportData
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if exp.Version != ExportSchemaVersion {
		t.Errorf("version = %d, want %d", exp.Version, ExportSchemaVersion)
	}
	if len(exp.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(exp.Items))
	}
	if exp.Items[0].ID != "aaa111" || exp.Items[1].ID != "bbb222" {
		t.Errorf("Items = %v, %v", exp.Items[0].ID, exp.Items[1].ID)
	}
}

func TestExportItems_EmptyList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := ExportItems(nil, path); err != nil {
		t.Fatalf("ExportItems(nil): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var exp ExportData
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(exp.Items) != 0 {
		t.Errorf("len(Items) = %d, want 0", len(exp.Items))
	}
}
