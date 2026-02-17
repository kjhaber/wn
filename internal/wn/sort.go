package wn

// TopoOrder returns items in dependency order: prerequisites first.
// Items with no dependencies come first; then items whose dependencies
// are already in the result. If there is a cycle, the second return
// value is false and order is undefined.
func TopoOrder(items []*Item) ([]*Item, bool) {
	byID := make(map[string]*Item)
	for _, it := range items {
		byID[it.ID] = it
	}
	var result []*Item
	added := make(map[string]bool)
	for len(result) < len(items) {
		progress := false
		for _, it := range items {
			if added[it.ID] {
				continue
			}
			ready := true
			for _, dep := range it.DependsOn {
				if !added[dep] {
					ready = false
					break
				}
			}
			if ready {
				result = append(result, it)
				added[it.ID] = true
				progress = true
			}
		}
		if !progress {
			return result, false
		}
	}
	return result, true
}
