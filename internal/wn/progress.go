package wn

import (
	"slices"
	"strings"
	"time"
)

// ItemListStatus returns the display status for list/JSON output.
// Possible values: "undone", "blocked", "claimed", "review", "done", "closed", "suspend".
// blocked should be true when the item has unresolved dependencies (see BlockedSet).
func ItemListStatus(it *Item, now time.Time, blocked bool) string {
	if it.Done {
		switch it.DoneStatus {
		case DoneStatusClosed:
			return "closed"
		case DoneStatusSuspend:
			return "suspend"
		default:
			return "done"
		}
	}
	if it.ReviewReady {
		return "review"
	}
	if it.PromptReady {
		return "prompt"
	}
	if blocked {
		return "blocked"
	}
	if IsInProgress(it, now) {
		return "claimed"
	}
	return "undone"
}

// BlockedSet returns the set of item IDs that are blocked: non-done, non-review items
// that have at least one dependency that is not done. allItems should be the complete
// list of items from the store for accurate results.
func BlockedSet(allItems []*Item) map[string]bool {
	doneByID := make(map[string]bool, len(allItems))
	for _, it := range allItems {
		doneByID[it.ID] = it.Done
	}
	blocked := make(map[string]bool)
	for _, it := range allItems {
		if it.Done || it.ReviewReady || it.PromptReady {
			continue
		}
		for _, depID := range it.DependsOn {
			if isDone, exists := doneByID[depID]; exists && !isDone {
				blocked[it.ID] = true
				break
			}
		}
	}
	return blocked
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
	if it.PromptReady {
		return false
	}
	if it.InProgressUntil.IsZero() {
		return true
	}
	return !now.Before(it.InProgressUntil)
}

// UndoneItems returns all items that are undone and available for agent (next/claim).
// Excludes review-ready, prompt-ready, and in-progress; clears expired in-progress lazily.
func UndoneItems(store Store) ([]*Item, error) {
	f := false
	return QueryItems(store, ItemQuery{Done: &f, ReviewReady: &f})
}

// ReviewReadyItems returns all items that are undone and review-ready (excluded from next/claim).
func ReviewReadyItems(store Store) ([]*Item, error) {
	f, t := false, true
	return QueryItems(store, ItemQuery{Done: &f, ReviewReady: &t})
}

// FilterByTag returns items matching the tag expression. If tag is empty, returns items unchanged.
// Compound expressions: "a,b" means AND (item must have all tags); "a|b" means OR (item has any).
// Plain "a" is a single-tag exact match (backward-compatible).
func FilterByTag(items []*Item, tag string) []*Item {
	if tag == "" {
		return items
	}
	var match func(*Item) bool
	if strings.Contains(tag, ",") {
		required := strings.Split(tag, ",")
		match = func(it *Item) bool {
			tagSet := make(map[string]bool, len(it.Tags))
			for _, t := range it.Tags {
				tagSet[t] = true
			}
			for _, r := range required {
				if !tagSet[r] {
					return false
				}
			}
			return true
		}
	} else if strings.Contains(tag, "|") {
		any := strings.Split(tag, "|")
		match = func(it *Item) bool {
			for _, t := range it.Tags {
				if slices.Contains(any, t) {
					return true
				}
			}
			return false
		}
	} else {
		match = func(it *Item) bool {
			return slices.Contains(it.Tags, tag)
		}
	}
	filtered := make([]*Item, 0, len(items))
	for _, it := range items {
		if match(it) {
			filtered = append(filtered, it)
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
// Clears expired in-progress lazily. Used by wn list (default/--undone), export --undone, and MCP wn_list.
// For pick/next/claim use UndoneItems (available only); for list --review-ready use ReviewReadyItems.
func ListableUndoneItems(store Store) ([]*Item, error) {
	f := false
	return QueryItems(store, ItemQuery{Done: &f})
}
