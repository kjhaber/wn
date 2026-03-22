package wn

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DoneStatus values when Item.Done is true. Empty or "done" = normal completion.
const (
	DoneStatusDone    = "done"
	DoneStatusClosed  = "closed"
	DoneStatusSuspend = "suspend"
)

// Item is a single work item. IDs are 6-character UUID prefixes (lowercase hex).
type Item struct {
	ID              string     `json:"id"`
	Description     string     `json:"description"`
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
	Done            bool       `json:"done"`
	DoneMessage     string     `json:"done_message,omitempty"`
	DoneStatus      string     `json:"done_status,omitempty"`       // when Done: "done" | "closed" | "suspend"; empty = done
	InProgressUntil time.Time  `json:"in_progress_until,omitempty"` // zero = not in progress
	InProgressBy    string     `json:"in_progress_by,omitempty"`    // optional worker id for logging
	ReviewReady     bool       `json:"review_ready,omitempty"`      // undone but excluded from agent next/claim; set on release, cleared when user marks done
	PromptReady     bool       `json:"prompt_ready,omitempty"`      // undone but awaiting human response; excluded from agent next/claim
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

// NoteNameClaudeSession is the note name for storing the Claude Code session ID for resume support.
const NoteNameClaudeSession = "claude-session"

// NoteNameResponse is the note name used by wn respond to store the user's answer on a prompt item.
const NoteNameResponse = "response"

// NoteNameBranch is the special note name for recording which git branch implements a work item.
// Note names starting with "wn:" are reserved for internal use and validated against a known set.
const NoteNameBranch = "wn:branch"

// NoteNameCommit is the special note name for recording the merge commit hash for a work item.
// Used to detect squash merges (where the branch tip is not an ancestor of the merge commit).
const NoteNameCommit = "wn:commit"

// specialNoteNames is the set of known wn: note names. Unknown wn: names are rejected.
var specialNoteNames = map[string]bool{
	NoteNameBranch: true,
	NoteNameCommit: true,
}

// Note is an attachment on an item with a logical name (e.g. "pr-url", "issue-number").
// Item.Notes are listed ordered by Created (oldest first).
type Note struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Body    string    `json:"body"`
}

// ValidNoteName returns true if name is valid. Two forms are accepted:
//   - Standard names: alphanumeric, slash, underscore, or hyphen, 1–32 chars.
//   - Special names: "wn:" prefix followed by one or more lowercase alphanumeric/hyphen chars,
//     total length 1–32 chars. Unknown wn: names pass syntax validation but are rejected
//     by ValidateSpecialNote.
func ValidNoteName(name string) bool {
	if len(name) < 1 || len(name) > 32 {
		return false
	}
	specialName := regexp.MustCompile(`^wn:[a-z][a-z0-9-]*$`)
	if strings.HasPrefix(name, "wn:") {
		return specialName.MatchString(name)
	}
	standardName := regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`)
	return standardName.MatchString(name)
}

// ValidateSpecialNote returns an error if name is a wn: name that is not in the known set.
// Non-wn: names are always accepted.
func ValidateSpecialNote(name string) error {
	if !strings.HasPrefix(name, "wn:") {
		return nil
	}
	if !specialNoteNames[name] {
		return fmt.Errorf("unknown wn: note name %q (known: wn:branch, wn:commit)", name)
	}
	return nil
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
