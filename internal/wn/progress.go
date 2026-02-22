package wn

import "time"

// ItemListStatus returns "done", "undone", "claimed", or "review-ready" for list/JSON output.
func ItemListStatus(it *Item, now time.Time) string {
	if it.Done {
		return "done"
	}
	if IsInProgress(it, now) {
		return "claimed"
	}
	if it.ReviewReady {
		return "review-ready"
	}
	return "undone"
}

// IsInProgress returns true if the item is currently in progress (has a future InProgressUntil).
func IsInProgress(it *Item, now time.Time) bool {
	if it.InProgressUntil.IsZero() {
		return false
	}
	return now.Before(it.InProgressUntil)
}

// IsAvailableUndone returns true if the item is undone and available to be claimed
// (not done, not review-ready, and either not in progress or in-progress has expired).
func IsAvailableUndone(it *Item, now time.Time) bool {
	if it.Done {
		return false
	}
	if it.ReviewReady {
		return false
	}
	if it.InProgressUntil.IsZero() {
		return true
	}
	return !now.Before(it.InProgressUntil)
}

// UndoneItems returns all items that are undone and available for agent (next/claim).
// Excludes review-ready and in-progress; clears expired in-progress lazily.
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
			// Expired: clear in-progress and include in result (unless review-ready)
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
			if !curr.ReviewReady {
				result = append(result, curr)
			}
			continue
		}
		if IsAvailableUndone(it, now) {
			result = append(result, it)
		}
	}
	return result, nil
}

// ListableUndoneItems returns all undone items (including review-ready) for human list/filters.
// Clears expired in-progress lazily. Used by list and wn_list so review-ready items are visible.
func ListableUndoneItems(store Store) ([]*Item, error) {
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
		if it.InProgressUntil.IsZero() {
			result = append(result, it)
		}
	}
	return result, nil
}
