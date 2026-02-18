package wn

import "time"

// Item is a single work item. IDs are 6-character UUID prefixes (lowercase hex).
type Item struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Created     time.Time  `json:"created"`
	Updated     time.Time  `json:"updated"`
	Done        bool       `json:"done"`
	DoneMessage string     `json:"done_message,omitempty"`
	Tags        []string   `json:"tags"`
	DependsOn   []string   `json:"depends_on"`
	Log         []LogEntry `json:"log"`
}

// LogEntry records one event in an item's history.
type LogEntry struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // e.g. "created", "updated", "tag_added", "done", "undone"
	Msg  string    `json:"msg,omitempty"`
}
