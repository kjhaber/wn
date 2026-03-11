package wn

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveItem_DefaultDir(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "arch01",
		Description: "item to archive",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	archivePath, err := ArchiveItem(store, "arch01", "")
	if err != nil {
		t.Fatalf("ArchiveItem: %v", err)
	}

	// Verify archive file was written to the default location
	wantPath := filepath.Join(DefaultArchiveDir(root), "arch01.json")
	if archivePath != wantPath {
		t.Errorf("archivePath = %q, want %q", archivePath, wantPath)
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Errorf("archive file not found: %v", err)
	}

	// Verify item was removed from store
	if _, err := store.Get("arch01"); err == nil {
		t.Error("item should have been removed from store after archiving")
	}
}

func TestArchiveItem_CustomDir(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "arch02",
		Description: "custom dir archive",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	customDir := filepath.Join(t.TempDir(), "my-archive")
	archivePath, err := ArchiveItem(store, "arch02", customDir)
	if err != nil {
		t.Fatalf("ArchiveItem: %v", err)
	}

	wantPath := filepath.Join(customDir, "arch02.json")
	if archivePath != wantPath {
		t.Errorf("archivePath = %q, want %q", archivePath, wantPath)
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Errorf("archive file not found: %v", err)
	}

	// Verify item was removed from store
	if _, err := store.Get("arch02"); err == nil {
		t.Error("item should have been removed from store after archiving")
	}
}

func TestArchiveItem_ArchivedFileIsValidExport(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "arch03",
		Description: "recoverable item",
		Tags:        []string{"important"},
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	archivePath, err := ArchiveItem(store, "arch03", "")
	if err != nil {
		t.Fatalf("ArchiveItem: %v", err)
	}

	// Should be importable back via ImportAppend
	root2 := t.TempDir()
	store2, err := NewFileStore(root2)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if err := ImportAppend(store2, archivePath); err != nil {
		t.Fatalf("ImportAppend from archive: %v", err)
	}
	got, err := store2.Get("arch03")
	if err != nil {
		t.Fatalf("Get after import: %v", err)
	}
	if got.Description != "recoverable item" {
		t.Errorf("description = %q, want %q", got.Description, "recoverable item")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "important" {
		t.Errorf("tags = %v, want [important]", got.Tags)
	}
}

func TestArchiveItem_NotFound(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	_, err = ArchiveItem(store, "nonexistent", "")
	if err == nil {
		t.Error("expected error for nonexistent item")
	}
}

func TestArchiveItem_CreatesArchiveDirIfMissing(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:      "arch04",
		Created: now,
		Updated: now,
		Log:     []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	archiveDir := DefaultArchiveDir(root)
	// Confirm it doesn't exist yet
	if _, err := os.Stat(archiveDir); !os.IsNotExist(err) {
		t.Skip("archive dir already exists")
	}

	if _, err := ArchiveItem(store, "arch04", ""); err != nil {
		t.Fatalf("ArchiveItem: %v", err)
	}

	if _, err := os.Stat(archiveDir); err != nil {
		t.Errorf("archive dir should have been created: %v", err)
	}
}
