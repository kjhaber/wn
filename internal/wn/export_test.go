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

// TestExportItems_IncludesAllAttributes ensures export always writes every item attribute
// (no omitempty), so export files are complete and list --json matches.
func TestExportItems_IncludesAllAttributes(t *testing.T) {
	now := time.Now().UTC()
	orderVal := 3
	items := []*Item{
		{
			ID: "full1", Description: "desc", Created: now, Updated: now,
			Done: false, DoneMessage: "", InProgressUntil: time.Time{}, InProgressBy: "", ReviewReady: false,
			Tags: nil, DependsOn: nil, Order: &orderVal, Log: []LogEntry{{At: now, Kind: "created"}}, Notes: nil,
		},
	}
	path := filepath.Join(t.TempDir(), "full.json")
	if err := ExportItems(items, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	itemsArr, ok := raw["items"].([]any)
	if !ok || len(itemsArr) != 1 {
		t.Fatalf("expected items array with one element, got %T %v", raw["items"], raw["items"])
	}
	itemObj, ok := itemsArr[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first item to be object, got %T", itemsArr[0])
	}
	wantKeys := []string{"id", "description", "created", "updated", "done", "done_message", "done_status", "in_progress_until", "in_progress_by", "review_ready", "tags", "depends_on", "order", "log", "notes"}
	for _, k := range wantKeys {
		if _, has := itemObj[k]; !has {
			t.Errorf("export item missing key %q (export must include all attributes)", k)
		}
	}
}

func TestImportAppend_IntoEmptyStore(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "export.json")
	if err := ExportItems([]*Item{
		{ID: "aaa111", Description: "only item", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	if err := ImportAppend(store, path); err != nil {
		t.Fatalf("ImportAppend: %v", err)
	}
	got, err := store.Get("aaa111")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "only item" {
		t.Errorf("description = %q, want %q", got.Description, "only item")
	}
}

func TestImportAppend_AddsToExisting(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	existing := &Item{ID: "old111", Description: "existing", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := store.Put(existing); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.json")
	if err := ExportItems([]*Item{
		{ID: "new222", Description: "from file", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	if err := ImportAppend(store, path); err != nil {
		t.Fatalf("ImportAppend: %v", err)
	}
	all, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(List) = %d, want 2", len(all))
	}
	gotOld, _ := store.Get("old111")
	if gotOld.Description != "existing" {
		t.Errorf("old111 description = %q, want existing", gotOld.Description)
	}
	gotNew, _ := store.Get("new222")
	if gotNew.Description != "from file" {
		t.Errorf("new222 description = %q, want from file", gotNew.Description)
	}
}

func TestImportAppend_SameIDOverwrites(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Put(&Item{ID: "abc123", Description: "old text", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.json")
	if err := ExportItems([]*Item{
		{ID: "abc123", Description: "new text", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}},
	}, path); err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	if err := ImportAppend(store, path); err != nil {
		t.Fatalf("ImportAppend: %v", err)
	}
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "new text" {
		t.Errorf("description = %q, want new text", got.Description)
	}
}
