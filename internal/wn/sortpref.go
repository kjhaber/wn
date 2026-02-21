package wn

import (
	"fmt"
	"sort"
	"strings"
)

// SortOption is one key in a sort specification (e.g. "updated:desc").
type SortOption struct {
	Key  string // created, updated, priority, alpha, tags
	Desc bool   // descending when true
}

// ParseSortSpec parses a comma-separated sort spec like "updated:desc,priority,tags".
// Each term may be "key" (asc) or "key:asc" or "key:desc". Valid keys: created, updated, priority, alpha, tags.
// Returns nil, nil for empty string.
func ParseSortSpec(s string) ([]SortOption, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []SortOption
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, dir, _ := strings.Cut(part, ":")
		key = strings.TrimSpace(strings.ToLower(key))
		dir = strings.TrimSpace(strings.ToLower(dir))
		desc := false
		switch dir {
		case "", "asc":
		case "desc":
			desc = true
		default:
			return nil, fmt.Errorf("invalid sort direction %q", dir)
		}
		switch key {
		case "created", "updated", "priority", "alpha", "tags":
			out = append(out, SortOption{Key: key, Desc: desc})
		default:
			return nil, fmt.Errorf("invalid sort key %q (use created, updated, priority, alpha, tags)", key)
		}
	}
	return out, nil
}

// ApplySort sorts items in place by the given spec (primary key, then tiebreakers).
// Nil or empty spec returns items unchanged. "priority" uses Item.Order (lower = earlier when asc).
// "tags" sorts by a canonical tag string so items with same tags are adjacent (group by tags).
func ApplySort(items []*Item, spec []SortOption) []*Item {
	if len(spec) == 0 || len(items) == 0 {
		return items
	}
	// Copy so we don't mutate caller's slice order in-place
	result := make([]*Item, len(items))
	copy(result, items)
	sort.Slice(result, func(i, j int) bool {
		for _, opt := range spec {
			less := compareByKey(result[i], result[j], opt.Key, opt.Desc)
			if less {
				return true
			}
			if compareByKey(result[j], result[i], opt.Key, opt.Desc) {
				return false
			}
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func compareByKey(a, b *Item, key string, desc bool) bool {
	var less bool
	switch key {
	case "created":
		less = a.Created.Before(b.Created)
	case "updated":
		less = a.Updated.Before(b.Updated)
	case "priority":
		less = orderLess(a.Order, b.Order)
	case "alpha":
		less = FirstLine(a.Description) < FirstLine(b.Description)
	case "tags":
		less = tagsKey(a.Tags) < tagsKey(b.Tags)
	default:
		less = a.ID < b.ID
	}
	if desc {
		return !less
	}
	return less
}

func orderLess(a, b *int) bool {
	va := orderKeyFromPtr(a)
	vb := orderKeyFromPtr(b)
	return va < vb
}

func orderKeyFromPtr(p *int) int {
	if p == nil {
		return 1<<31 - 1
	}
	return *p
}

func tagsKey(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	// Copy and sort for stable grouping
	c := make([]string, len(tags))
	copy(c, tags)
	sort.Strings(c)
	return strings.Join(c, ",")
}
