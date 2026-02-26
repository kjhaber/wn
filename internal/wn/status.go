package wn

import (
	"fmt"
	"time"
)

// Status values for work items. Used by wn status <state> and ItemListStatus.
const (
	StatusUndone  = "undone"
	StatusClaimed = "claimed"
	StatusReview  = "review"
	StatusDone    = "done"
	StatusClosed  = "closed"
	StatusSuspend = "suspend"
)

// ValidStatuses is the ordered list of valid status values for wn status.
var ValidStatuses = []string{StatusUndone, StatusClaimed, StatusReview, StatusDone, StatusClosed, StatusSuspend}

// ValidStatus returns true if s is one of the valid status values.
func ValidStatus(s string) bool {
	for _, v := range ValidStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// StatusOpts holds optional arguments when setting status (e.g. message for done, duration for claimed).
type StatusOpts struct {
	DoneMessage string
	ClaimFor    time.Duration
	ClaimBy     string
	// DuplicateOf is valid only when setting status to "closed". Sets the standard note duplicate-of (body = original id) and logs duplicate_of.
	DuplicateOf string
}

// SetStatus sets the work item to the given status. Id must exist.
// For "claimed", ClaimFor must be > 0 in opts.
func SetStatus(store Store, id, status string, opts StatusOpts) error {
	if !ValidStatus(status) {
		return fmt.Errorf("invalid status %q; must be one of: undone, claimed, review, done, closed, suspend", status)
	}
	_, err := store.Get(id)
	if err != nil {
		return err
	}
	if status == StatusClosed && opts.DuplicateOf != "" {
		if id == opts.DuplicateOf {
			return fmt.Errorf("cannot mark item as duplicate of itself")
		}
		if _, err := store.Get(opts.DuplicateOf); err != nil {
			return fmt.Errorf("original item %s not found", opts.DuplicateOf)
		}
	}
	now := time.Now().UTC()

	return store.UpdateItem(id, func(it *Item) (*Item, error) {
		switch status {
		case StatusUndone:
			it.Done = false
			it.DoneMessage = ""
			it.DoneStatus = ""
			it.ReviewReady = false
			it.InProgressUntil = time.Time{}
			it.InProgressBy = ""
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "undone"})
		case StatusClaimed:
			if opts.ClaimFor <= 0 {
				opts.ClaimFor = DefaultClaimDuration
			}
			it.Done = false
			it.DoneStatus = ""
			it.ReviewReady = false
			it.InProgressUntil = now.Add(opts.ClaimFor)
			it.InProgressBy = opts.ClaimBy
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: opts.ClaimFor.String()})
		case StatusReview:
			it.Done = false
			it.DoneStatus = ""
			it.InProgressUntil = time.Time{}
			it.InProgressBy = ""
			it.ReviewReady = true
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "review_ready"})
		case StatusDone:
			it.Done = true
			it.DoneMessage = opts.DoneMessage
			it.DoneStatus = DoneStatusDone
			it.ReviewReady = false
			it.InProgressUntil = time.Time{}
			it.InProgressBy = ""
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "done", Msg: opts.DoneMessage})
		case StatusClosed:
			it.Done = true
			it.DoneMessage = opts.DoneMessage
			it.DoneStatus = DoneStatusClosed
			it.ReviewReady = false
			it.InProgressUntil = time.Time{}
			it.InProgressBy = ""
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "closed", Msg: opts.DoneMessage})
			if opts.DuplicateOf != "" {
				if it.Notes == nil {
					it.Notes = []Note{}
				}
				idx := it.NoteIndexByName(NoteNameDuplicateOf)
				if idx >= 0 {
					it.Notes[idx].Body = opts.DuplicateOf
				} else {
					it.Notes = append(it.Notes, Note{Name: NoteNameDuplicateOf, Created: now, Body: opts.DuplicateOf})
				}
				it.Log = append(it.Log, LogEntry{At: now, Kind: "duplicate_of", Msg: opts.DuplicateOf})
			}
		case StatusSuspend:
			it.Done = true
			it.DoneMessage = opts.DoneMessage
			it.DoneStatus = DoneStatusSuspend
			it.ReviewReady = false
			it.InProgressUntil = time.Time{}
			it.InProgressBy = ""
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "suspend", Msg: opts.DoneMessage})
		}
		return it, nil
	})
}
