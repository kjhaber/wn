package wn

import (
	"fmt"
	"os"
	"path/filepath"
)

const archiveDirName = "archive"

// DefaultArchiveDir returns the default archive directory path for the given root.
func DefaultArchiveDir(root string) string {
	return filepath.Join(root, ".wn", archiveDirName)
}

// ArchiveItem saves an item to the archive directory as a single-item export file,
// then deletes it from the store. If archiveDir is empty, uses DefaultArchiveDir(store.Root()).
// Returns the path where the item was archived.
func ArchiveItem(store Store, id string, archiveDir string) (string, error) {
	item, err := store.Get(id)
	if err != nil {
		return "", fmt.Errorf("item %s not found", id)
	}

	if archiveDir == "" {
		archiveDir = DefaultArchiveDir(store.Root())
	}

	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("create archive directory: %w", err)
	}

	archivePath := filepath.Join(archiveDir, id+".json")
	if err := ExportItems([]*Item{item}, archivePath); err != nil {
		return "", fmt.Errorf("write archive file: %w", err)
	}

	if err := store.Delete(id); err != nil {
		// Roll back: remove the archive file we just wrote
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("delete item from store: %w", err)
	}

	return archivePath, nil
}
