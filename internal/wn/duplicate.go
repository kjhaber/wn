package wn

// MarkDuplicateOf marks the work item id as a duplicate of the work item originalID.
// It sets status to closed and adds the standard note NoteNameDuplicateOf with body originalID
// so the item leaves the active queue while preserving it for reference. Returns an error if
// id or originalID do not exist, or if id == originalID.
// Prefer using SetStatus(store, id, StatusClosed, StatusOpts{DuplicateOf: originalID}) directly.
func MarkDuplicateOf(store Store, id, originalID string) error {
	return SetStatus(store, id, StatusClosed, StatusOpts{DuplicateOf: originalID})
}
