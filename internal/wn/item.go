package wn

import "time"

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

// Note is an attachment on an item (e.g. "I wrote this in file X" or a link), without editing the description.
// Item.Notes are listed ordered by Created (oldest first).
type Note struct {
	Created time.Time `json:"created"`
	Body    string    `json:"body"`
}
