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
// Any prompt-ready deps of the item are included in the same archive file and also deleted.
// Returns the path where the item was archived.
func ArchiveItem(store Store, id string, archiveDir string) (string, error) {
	item, err := store.Get(id)
	if err != nil {
		return "", fmt.Errorf("item %s not found", id)
	}

	// Collect prompt deps to include in the archive.
	items := []*Item{item}
	var promptDepIDs []string
	for _, depID := range item.DependsOn {
		dep, err := store.Get(depID)
		if err != nil {
			continue
		}
		if dep.PromptReady {
			items = append(items, dep)
			promptDepIDs = append(promptDepIDs, depID)
		}
	}

	if archiveDir == "" {
		archiveDir = DefaultArchiveDir(store.Root())
	}

	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("create archive directory: %w", err)
	}

	archivePath := filepath.Join(archiveDir, id+".json")
	if err := ExportItems(items, archivePath); err != nil {
		return "", fmt.Errorf("write archive file: %w", err)
	}

	if err := store.Delete(id); err != nil {
		// Roll back: remove the archive file we just wrote
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("delete item from store: %w", err)
	}

	for _, depID := range promptDepIDs {
		_ = store.Delete(depID)
	}

	return archivePath, nil
}
