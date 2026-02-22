package wn

import (
	"encoding/json"
	"os"
	"time"
)

// ExportSchemaVersion is the schema version written to export files.
const ExportSchemaVersion = 1

// ExportData is the top-level structure of an export file.
type ExportData struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Items      []*Item   `json:"items"`
}

// Export writes all items from the store to a single JSON file (or stdout if path is "").
func Export(store Store, path string) error {
	items, err := store.List()
	if err != nil {
		return err
	}
	data := ExportData{
		Version:    ExportSchemaVersion,
		ExportedAt: time.Now().UTC(),
		Items:      items,
	}
	out, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if path == "" {
		_, err = os.Stdout.Write(out)
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// ImportReplace reads an export file and replaces all items in the store.
// The store's root must already be initialized (.wn/items exists).
func ImportReplace(store Store, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var exp ExportData
	if err := json.Unmarshal(data, &exp); err != nil {
		return err
	}
	// Delete existing items
	existing, err := store.List()
	if err != nil {
		return err
	}
	for _, it := range existing {
		if err := store.Delete(it.ID); err != nil {
			return err
		}
	}
	// Write new items
	for _, it := range exp.Items {
		if err := store.Put(it); err != nil {
			return err
		}
	}
	return nil
}

// StoreHasItems returns whether the store has at least one item.
func StoreHasItems(store Store) (bool, error) {
	items, err := store.List()
	if err != nil {
		return false, err
	}
	return len(items) > 0, nil
}
