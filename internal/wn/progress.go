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

// ReviewReadyItems returns all items that are undone and review-ready (excluded from next/claim).
func ReviewReadyItems(store Store) ([]*Item, error) {
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	var result []*Item
	for _, it := range items {
		if !it.Done && it.ReviewReady {
			result = append(result, it)
		}
	}
	return result, nil
}

// FilterByTag returns items that have the given tag. If tag is empty, returns items unchanged.
func FilterByTag(items []*Item, tag string) []*Item {
	if tag == "" {
		return items
	}
	filtered := make([]*Item, 0, len(items))
	for _, it := range items {
		for _, t := range it.Tags {
			if t == tag {
				filtered = append(filtered, it)
				break
			}
		}
	}
	return filtered
}

// NextUndoneItem returns the first undone item in dependency order, optionally filtered by tag.
// If tag is non-empty, only items with that tag are considered. Returns nil if none.
func NextUndoneItem(store Store, tag string) (*Item, error) {
	undone, err := UndoneItems(store)
	if err != nil {
		return nil, err
	}
	undone = FilterByTag(undone, tag)
	ordered, acyclic := TopoOrder(undone)
	if !acyclic || len(ordered) == 0 {
		return nil, nil
	}
	return ordered[0], nil
}

// ListableUndoneItems returns all undone items (including review-ready) for list/export.
// Clears expired in-progress lazily. Used by wn list (default/--undone), export --undone, and MCP wn_list. For pick/next/claim use UndoneItems (available only); for list --review-ready use ReviewReadyItems.
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
