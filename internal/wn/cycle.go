package wn

// WouldCreateCycle returns true if adding an edge from fromID to toID
// would create a cycle in the graph of items.
func WouldCreateCycle(items []*Item, fromID, toID string) bool {
	if fromID == toID {
		return true
	}
	// Build adjacency list (who depends on whom: item -> deps)
	adj := make(map[string][]string)
	for _, it := range items {
		adj[it.ID] = it.DependsOn
	}
	// If we add fromID -> toID, is there a path from toID to fromID?
	adj[fromID] = append(append([]string(nil), adj[fromID]...), toID)
	return pathExists(adj, toID, fromID, nil)
}

func pathExists(adj map[string][]string, from, to string, seen map[string]bool) bool {
	if seen == nil {
		seen = make(map[string]bool)
	}
	if from == to {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, n := range adj[from] {
		if pathExists(adj, n, to, seen) {
			return true
		}
	}
	return false
}
