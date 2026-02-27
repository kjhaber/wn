package wn

import (
	"fmt"
	"time"
)

// CloseDoneItemResult reports one item's outcome from CloseDoneItems.
type CloseDoneItemResult struct {
	ID     string
	Status string // "closed", "skipped_not_done", "skipped_not_old_enough"
	Reason string
}

// CloseDoneItems finds items that are in "done" state and have been done longer
// than cutoff (done_at < cutoff) and sets them to "closed". When dryRun is
// true, no changes are written; only results are returned.
//
// An item is considered "done" for this purpose when Done is true and
// DoneStatus is "", DoneStatusDone, or any other non-terminal done variant
// (i.e. not closed or suspend). The done-at timestamp is the At time of the
// most recent log entry with Kind "done"; if none is found, Updated is used.
func CloseDoneItems(store Store, cutoff time.Time, dryRun bool) ([]CloseDoneItemResult, error) {
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	var results []CloseDoneItemResult
	for _, it := range items {
		if !it.Done {
			results = append(results, CloseDoneItemResult{
				ID:     it.ID,
				Status: "skipped_not_done",
				Reason: "item is not done",
			})
			continue
		}
		if it.DoneStatus == DoneStatusClosed || it.DoneStatus == DoneStatusSuspend {
			results = append(results, CloseDoneItemResult{
				ID:     it.ID,
				Status: "skipped_not_done",
				Reason: fmt.Sprintf("item has terminal status %q", it.DoneStatus),
			})
			continue
		}
		doneAt := mostRecentDoneTime(it)
		if doneAt.IsZero() {
			doneAt = it.Updated
		}
		if doneAt.IsZero() || !doneAt.Before(cutoff) {
			results = append(results, CloseDoneItemResult{
				ID:     it.ID,
				Status: "skipped_not_old_enough",
				Reason: "done time is not older than cutoff",
			})
			continue
		}
		if dryRun {
			results = append(results, CloseDoneItemResult{
				ID:     it.ID,
				Status: "closed",
				Reason: fmt.Sprintf("would close (done at %s, cutoff %s)", doneAt.Format(time.RFC3339), cutoff.Format(time.RFC3339)),
			})
			continue
		}
		if err := SetStatus(store, it.ID, StatusClosed, StatusOpts{}); err != nil {
			return nil, err
		}
		results = append(results, CloseDoneItemResult{
			ID:     it.ID,
			Status: "closed",
			Reason: fmt.Sprintf("closed (done at %s, cutoff %s)", doneAt.Format(time.RFC3339), cutoff.Format(time.RFC3339)),
		})
	}
	return results, nil
}

// mostRecentDoneTime returns the At time of the most recent "done" log entry,
// or zero if none exists.
func mostRecentDoneTime(it *Item) time.Time {
	for i := len(it.Log) - 1; i >= 0; i-- {
		if it.Log[i].Kind == "done" {
			return it.Log[i].At
		}
	}
	return time.Time{}
}
