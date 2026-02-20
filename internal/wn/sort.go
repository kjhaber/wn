package wn

import "sort"

// orderVal returns the effective sort key for an item: lower = earlier in backlog.
// Nil Order is treated as "no preference" and sorts after any set value (stable).
func orderKey(it *Item) int {
	if it.Order == nil {
		return 1<<31 - 1
	}
	return *it.Order
}

// TopoOrder returns items in dependency order: prerequisites first.
// Among items that are ready in the same round, Order is used as tiebreaker
// (lower Order = earlier). Items with no Order sort after those with Order.
// If there is a cycle, the second return value is false and order is undefined.
func TopoOrder(items []*Item) ([]*Item, bool) {
	byID := make(map[string]*Item)
	for _, it := range items {
		byID[it.ID] = it
	}
	var result []*Item
	added := make(map[string]bool)
	for len(result) < len(items) {
		var ready []*Item
		for _, it := range items {
			if added[it.ID] {
				continue
			}
			ok := true
			for _, dep := range it.DependsOn {
				if !added[dep] {
					ok = false
					break
				}
			}
			if ok {
				ready = append(ready, it)
			}
		}
		if len(ready) == 0 {
			return result, false
		}
		sort.Slice(ready, func(i, j int) bool {
			return orderKey(ready[i]) < orderKey(ready[j])
		})
		for _, it := range ready {
			result = append(result, it)
			added[it.ID] = true
		}
	}
	return result, true
}
