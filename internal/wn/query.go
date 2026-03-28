package wn

import (
	"strings"
	"time"
)

// ItemQuery specifies filter criteria for QueryItems.
// Nil pointer fields mean "no filter on this attribute".
type ItemQuery struct {
	// Done filters by done status. nil = include all; true = done only; false = undone only.
	Done *bool
	// ReviewReady filters by review-ready status. nil = include all;
	// true = only review-ready items; false = exclude review-ready and prompt-ready items.
	ReviewReady *bool
	// Tags filters items by tag. When empty, no tag filter is applied.
	Tags []string
	// TagsMatchAll controls multi-tag semantics. true = AND (item must have all tags);
	// false = OR (item must have at least one tag). Ignored when Tags is empty.
	TagsMatchAll bool
	// Limit caps the number of results. 0 means no limit.
	Limit int
}

// QueryItems returns items from store that match all criteria in q.
//
// When Done=false, expired in-progress claims are lazily cleared and the items
// are included in results; non-expired in-progress items are excluded. This
// mirrors the behaviour of UndoneItems and ListableUndoneItems.
func QueryItems(store Store, q ItemQuery) ([]*Item, error) {
	now := time.Now().UTC()
	all, err := store.List()
	if err != nil {
		return nil, err
	}

	queryingUndone := q.Done != nil && !*q.Done

	var result []*Item
	for _, it := range all {
		// Filter by Done.
		if q.Done != nil && it.Done != *q.Done {
			continue
		}

		if queryingUndone {
			// Lazily clear expired in-progress claims; skip non-expired ones.
			if !it.InProgressUntil.IsZero() {
				if now.After(it.InProgressUntil) {
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
					it = curr
				} else {
					// Currently claimed; skip.
					continue
				}
			}
		}

		// Filter by ReviewReady / PromptReady.
		if q.ReviewReady != nil {
			if *q.ReviewReady {
				if !it.ReviewReady {
					continue
				}
			} else {
				if it.ReviewReady || it.PromptReady {
					continue
				}
			}
		}

		result = append(result, it)
	}

	// Filter by tags.
	if len(q.Tags) > 0 {
		filtered := make([]*Item, 0, len(result))
		for _, it := range result {
			if itemMatchesTags(it, q.Tags, q.TagsMatchAll) {
				filtered = append(filtered, it)
			}
		}
		result = filtered
	}

	// Apply limit.
	if q.Limit > 0 && len(result) > q.Limit {
		result = result[:q.Limit]
	}

	return result, nil
}

// itemMatchesTags reports whether it has the required tags.
// When matchAll is true, the item must have every tag in tags (AND).
// When matchAll is false, the item must have at least one tag in tags (OR).
func itemMatchesTags(it *Item, tags []string, matchAll bool) bool {
	if matchAll {
		tagSet := make(map[string]bool, len(it.Tags))
		for _, t := range it.Tags {
			tagSet[t] = true
		}
		for _, required := range tags {
			if !tagSet[required] {
				return false
			}
		}
		return true
	}
	// OR: any tag in the item matches any required tag.
	for _, t := range it.Tags {
		for _, required := range tags {
			if t == required {
				return true
			}
		}
	}
	return false
}

// ParseTagExpr converts a CLI-style tag expression into Tags/TagsMatchAll values for ItemQuery.
// "a,b" → (["a","b"], true) AND; "a|b" → (["a","b"], false) OR; "a" → (["a"], false); "" → (nil, false).
func ParseTagExpr(expr string) (tags []string, matchAll bool) {
	if expr == "" {
		return nil, false
	}
	if strings.Contains(expr, ",") {
		return strings.Split(expr, ","), true
	}
	if strings.Contains(expr, "|") {
		return strings.Split(expr, "|"), false
	}
	return []string{expr}, false
}
