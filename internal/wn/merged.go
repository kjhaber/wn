package wn

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MarkMergedResult reports one review-ready item's outcome from MarkMergedItems.
type MarkMergedResult struct {
	ID     string
	Status string // "marked", "skipped_no_branch", "skipped_not_merged", "skipped_error"
	Reason string
}

var commitHashRe = regexp.MustCompile("^[0-9a-fA-F]{7,40}$")

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
			// If the branch no longer exists (e.g. cleaned up after merge), fall back to a commit hash
			// from a commit/commit-info note when available.
			if strings.Contains(err.Error(), "does not exist") {
				commitRef := commitRefFromNotes(it)
				if commitRef == "" {
					results = append(results, MarkMergedResult{
						ID:     it.ID,
						Status: "skipped_error",
						Reason: fmt.Sprintf("branch %s does not exist and no commit note found", branch),
					})
					continue
				}
				merged, err = CommitMergedInto(repoRoot, commitRef, intoRef)
				if err != nil {
					results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_error", Reason: err.Error()})
					continue
				}
			} else {
				results = append(results, MarkMergedResult{ID: it.ID, Status: "skipped_error", Reason: err.Error()})
				continue
			}
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
			item.DoneStatus = DoneStatusDone
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

// commitRefFromNotes attempts to extract a commit hash from well-known notes ("commit" or "commit-info").
// It returns the first token of the note body when it looks like a hex SHA (7-40 chars), or "" otherwise.
func commitRefFromNotes(it *Item) string {
	for _, name := range []string{"commit", "commit-info"} {
		idx := it.NoteIndexByName(name)
		if idx < 0 {
			continue
		}
		body := strings.TrimSpace(it.Notes[idx].Body)
		if body == "" {
			continue
		}
		fields := strings.Fields(body)
		if len(fields) == 0 {
			continue
		}
		cand := fields[0]
		if commitHashRe.MatchString(cand) {
			return cand
		}
	}
	return ""
}
