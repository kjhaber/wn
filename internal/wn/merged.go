package wn

import (
	"fmt"
	"strings"
	"time"
)

// MarkMergedResult reports one review-ready item's outcome from MarkMergedItems.
type MarkMergedResult struct {
	ID     string
	Status string // "marked", "skipped_no_branch", "skipped_not_merged", "skipped_error"
	Reason string
}

// MarkMergedItems checks all review-ready items, finds their "branch" note, and
// marks done those whose branch has been merged into intoRef (empty = HEAD).
// If dryRun is true, no changes are made. Returns results for each item checked.
func MarkMergedItems(store Store, repoRoot, intoRef string, dryRun bool) ([]MarkMergedResult, error) {
	items, err := ReviewReadyItems(store)
	if err != nil {
		return nil, err
	}
	var results []MarkMergedResult
	for _, it := range items {
		idx := it.NoteIndexByName("branch")
		if idx < 0 {
			results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_no_branch", Reason: "no branch note"})
			continue
		}
		branch := strings.TrimSpace(it.Notes[idx].Body)
		if branch == "" {
			results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_no_branch", Reason: "branch note empty"})
			continue
		}
		merged, err := BranchMergedInto(repoRoot, branch, intoRef)
		if err != nil {
			results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_error", Reason: err.Error()})
			continue
		}
		if !merged {
			results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_not_merged", Reason: fmt.Sprintf("branch %s not merged", branch)})
			continue
		}
		if dryRun {
			results = append(results, MarkMergedResult{ID: it.ID, Status: "marked", Reason: fmt.Sprintf("would mark done (branch %s merged)", branch)})
			continue
		}
		now := time.Now().UTC()
		msg := "merged to current branch"
		if intoRef != "" && intoRef != "HEAD" {
			msg = fmt.Sprintf("merged to %s", intoRef)
		}
		if err := store.UpdateItem(it.ID, func(item *Item) (*Item, error) {
			item.Done = true
			item.DoneMessage = msg
			item.ReviewReady = false
			item.Updated = now
			item.Log = append(item.Log, LogEntry{At: now, Kind: "done", Msg: msg})
			return item, nil
		}); err != nil {
			results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_error", Reason: err.Error()})
			continue
		}
		results = append(results, MarkMergedResult{ID: it.ID, Status: "marked", Reason: msg})
	}
	return results, nil
}
