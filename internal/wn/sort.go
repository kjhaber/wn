package wn

import "sort"

// DefaultOrder is the effective priority when Item.Order is nil (lower = higher priority).
// Use order values > DefaultOrder (e.g. 100+) to place items below default-priority items.
const DefaultOrder = 99

// MaxOrder is the maximum allowed order value (0..MaxOrder). Values above this are rejected when setting order.
const MaxOrder = 255

// ValidOrder reports whether n is a valid order value (0 <= n <= MaxOrder).
func ValidOrder(n int) bool {
	return n >= 0 && n <= MaxOrder
}

// orderKey returns the effective sort key for an item: lower = earlier in backlog.
// Nil Order uses DefaultOrder so items can use order > DefaultOrder to sort lower than default.
func orderKey(it *Item) int {
	if it.Order == nil {
		return DefaultOrder
	}
	return *it.Order
}

// TopoOrder returns items in dependency order: prerequisites first.
// Among items that are ready in the same round, Order is used as tiebreaker
// (lower Order = earlier). Items with no Order use DefaultOrder (99); use order > DefaultOrder to place items lower.
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
