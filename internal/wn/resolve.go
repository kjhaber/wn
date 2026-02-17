package wn

import "errors"

var ErrNoItemID = errors.New("no id provided and no current task")

// ResolveItemID returns the item ID to use: explicitID if non-empty, otherwise currentID.
// Returns ErrNoItemID if both are empty.
func ResolveItemID(currentID, explicitID string) (string, error) {
	if explicitID != "" {
		return explicitID, nil
	}
	if currentID != "" {
		return currentID, nil
	}
	return "", ErrNoItemID
}
