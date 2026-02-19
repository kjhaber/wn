package wn

import "time"

// IsInProgress returns true if the item is currently in progress (has a future InProgressUntil).
func IsInProgress(it *Item, now time.Time) bool {
	if it.InProgressUntil.IsZero() {
		return false
	}
	return now.Before(it.InProgressUntil)
}

// IsAvailableUndone returns true if the item is undone and available to be claimed
// (not done, and either not in progress or in-progress has expired).
func IsAvailableUndone(it *Item, now time.Time) bool {
	if it.Done {
		return false
	}
	if it.InProgressUntil.IsZero() {
		return true
	}
	return !now.Before(it.InProgressUntil)
}

// UndoneItems returns all items that are undone and available (in-progress expired items
// are cleared lazily). Used by list (default), next, and pick.
func UndoneItems(store Store) ([]*Item, error) {
	now := time.Now().UTC()
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	var result []*Item
	for _, it := range items {
		if it.Done {
			continue
		}
		if !it.InProgressUntil.IsZero() && now.After(it.InProgressUntil) {
			// Expired: clear in-progress and include in result
			if err := store.UpdateItem(it.ID, func(item *Item) (*Item, error) {
				item.InProgressUntil = time.Time{}
				item.InProgressBy = ""
				item.Updated = now
				item.Log = append(item.Log, LogEntry{At: now, Kind: "in_progress_expired"})
				return item, nil
			}); err != nil {
				return nil, err
			}
			curr, err := store.Get(it.ID)
			if err != nil {
				return nil, err
			}
			result = append(result, curr)
			continue
		}
		if IsAvailableUndone(it, now) {
			result = append(result, it)
		}
	}
	return result, nil
}
