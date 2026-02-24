package wn

import (
	"regexp"
	"time"
)

// Item is a single work item. IDs are 6-character UUID prefixes (lowercase hex).
type Item struct {
	ID              string     `json:"id"`
	Description     string     `json:"description"`
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
	Done            bool       `json:"done"`
	DoneMessage     string     `json:"done_message,omitempty"`
	InProgressUntil time.Time  `json:"in_progress_until,omitempty"` // zero = not in progress
	InProgressBy    string     `json:"in_progress_by,omitempty"`    // optional worker id for logging
	ReviewReady     bool       `json:"review_ready,omitempty"`      // undone but excluded from agent next/claim; set on release, cleared when user marks done
	Tags            []string   `json:"tags"`
	DependsOn       []string   `json:"depends_on"`
	Order           *int       `json:"order,omitempty"` // optional backlog order when deps don't define it; lower = earlier
	Log             []LogEntry `json:"log"`
	Notes           []Note     `json:"notes,omitempty"` // attachments; listed ordered by Created
}

// LogEntry records one event in an item's history.
type LogEntry struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // e.g. "created", "updated", "tag_added", "done", "undone"
	Msg  string    `json:"msg,omitempty"`
}

// NoteNameDuplicateOf is the standard note name for marking an item as a duplicate of another.
// The note body is the ID of the canonical/original work item.
const NoteNameDuplicateOf = "duplicate-of"

// Note is an attachment on an item with a logical name (e.g. "pr-url", "issue-number").
// Item.Notes are listed ordered by Created (oldest first).
type Note struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Body    string    `json:"body"`
}

// ValidNoteName returns true if name is valid: alphanumeric, slash, underscore, or hyphen, 1–32 chars.
func ValidNoteName(name string) bool {
	if len(name) < 1 || len(name) > 32 {
		return false
	}
	validName := regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`)
	return validName.MatchString(name)
}

// NoteIndexByName returns the 0-based index of the first note with the given name, or -1 if not found.
func (it *Item) NoteIndexByName(name string) int {
	for i, n := range it.Notes {
		if n.Name == name {
			return i
		}
	}
	return -1
}
