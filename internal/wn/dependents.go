package wn

// Dependents returns the IDs of work items that depend on the given id
// (i.e. items whose DependsOn contains id). Order is undefined.
func Dependents(store Store, id string) ([]string, error) {
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range items {
		for _, dep := range it.DependsOn {
			if dep == id {
				out = append(out, it.ID)
				break
			}
		}
	}
	return out, nil
}
