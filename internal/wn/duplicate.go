package wn

import (
	"fmt"
	"time"
)

// MarkDuplicateOf marks the work item id as a duplicate of the work item originalID.
// It adds the standard note NoteNameDuplicateOf with body originalID and marks the item done
// so it leaves the active queue while preserving the item for reference. Returns an error if
// id or originalID do not exist, or if id == originalID.
func MarkDuplicateOf(store Store, id, originalID string) error {
	if id == originalID {
		return fmt.Errorf("cannot mark item as duplicate of itself")
	}
	if _, err := store.Get(originalID); err != nil {
		return fmt.Errorf("original item %s not found", originalID)
	}
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *Item) (*Item, error) {
		if it.Notes == nil {
			it.Notes = []Note{}
		}
		idx := it.NoteIndexByName(NoteNameDuplicateOf)
		if idx >= 0 {
			it.Notes[idx].Body = originalID
		} else {
			it.Notes = append(it.Notes, Note{Name: NoteNameDuplicateOf, Created: now, Body: originalID})
		}
		it.Done = true
		it.DoneMessage = ""
		it.ReviewReady = false
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "duplicate_of", Msg: originalID})
		return it, nil
	})
}
