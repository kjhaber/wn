package wn

import (
	"encoding/json"
	"os"
	"time"
)

// ExportSchemaVersion is the schema version written to export files.
const ExportSchemaVersion = 1

// ExportData is the top-level structure of an export file (used when reading/importing).
type ExportData struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Items      []*Item   `json:"items"`
}

// ExportItem mirrors Item but with no omitempty so export always includes every attribute.
type ExportItem struct {
	ID              string     `json:"id"`
	Description     string     `json:"description"`
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
	Done            bool       `json:"done"`
	DoneMessage     string     `json:"done_message"`
	InProgressUntil time.Time  `json:"in_progress_until"`
	InProgressBy    string     `json:"in_progress_by"`
	ReviewReady     bool       `json:"review_ready"`
	Tags            []string   `json:"tags"`
	DependsOn       []string   `json:"depends_on"`
	Order           *int       `json:"order"`
	Log             []LogEntry `json:"log"`
	Notes           []Note     `json:"notes"`
}

// ItemToExportItem converts an Item to an ExportItem (all fields present in JSON).
func ItemToExportItem(it *Item) *ExportItem {
	if it == nil {
		return nil
	}
	e := &ExportItem{
		ID:              it.ID,
		Description:     it.Description,
		Created:         it.Created,
		Updated:         it.Updated,
		Done:            it.Done,
		DoneMessage:     it.DoneMessage,
		InProgressUntil: it.InProgressUntil,
		InProgressBy:    it.InProgressBy,
		ReviewReady:     it.ReviewReady,
		Log:             it.Log,
	}
	if len(it.Tags) > 0 {
		e.Tags = make([]string, len(it.Tags))
		copy(e.Tags, it.Tags)
	}
	if len(it.DependsOn) > 0 {
		e.DependsOn = make([]string, len(it.DependsOn))
		copy(e.DependsOn, it.DependsOn)
	}
	if it.Order != nil {
		o := *it.Order
		e.Order = &o
	}
	if len(it.Notes) > 0 {
		e.Notes = make([]Note, len(it.Notes))
		copy(e.Notes, it.Notes)
	}
	return e
}

// exportDataWire is the structure written to export files (full attributes per item).
type exportDataWire struct {
	Version    int           `json:"version"`
	ExportedAt time.Time     `json:"exported_at"`
	Items      []*ExportItem `json:"items"`
}

// Export writes all items from the store to a single JSON file (or stdout if path is "").
func Export(store Store, path string) error {
	items, err := store.List()
	if err != nil {
		return err
	}
	return ExportItems(items, path)
}

// ExportItems writes the given items to a single JSON file (or stdout if path is "").
// Every item is written with all attributes (no omitempty). Callers can pass a filtered
// subset of items from the store (e.g. by tag or status).
func ExportItems(items []*Item, path string) error {
	if items == nil {
		items = []*Item{}
	}
	wire := exportDataWire{
		Version:    ExportSchemaVersion,
		ExportedAt: time.Now().UTC(),
		Items:      make([]*ExportItem, len(items)),
	}
	for i, it := range items {
		wire.Items[i] = ItemToExportItem(it)
	}
	out, err := json.Marshal(wire)
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
