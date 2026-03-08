package wn

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the user's wn configuration (e.g. ~/.config/wn/settings.json).
type Settings struct {
	Sort     string           `json:"sort,omitempty"`     // e.g. "updated:desc,priority,tags"
	Picker   string           `json:"picker,omitempty"`   // interactive picker: "fzf", "numbered", or "" (auto-detect)
	Next     NextSettings     `json:"next,omitempty"`     // defaults for next-item selection
	Worktree WorktreeSettings `json:"worktree,omitempty"` // defaults for worktree setup (wn worktree, wn do, wn agent-orch)
	Agent    AgentSettings    `json:"agent,omitempty"`    // defaults for headless agent runs (wn do, wn agent-orch)
	Cleanup  CleanupSettings  `json:"cleanup,omitempty"`  // options for cleanup subcommands
	Show     ShowSettings     `json:"show,omitempty"`     // defaults for wn show / bare wn
}

// NextSettings controls how the next work item is selected (wn next, wn worktree --next, wn agent-orch).
type NextSettings struct {
	Tag string `json:"tag,omitempty"` // only consider items that have this tag, e.g. "agent"
}

// WorktreeSettings controls worktree creation (wn worktree, wn do, wn agent-orch).
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type WorktreeSettings struct {
	Base          string `json:"base,omitempty"`           // base directory for worktrees, e.g. "../worktrees"
	BranchPrefix  string `json:"branch_prefix,omitempty"`  // prefix for generated branch names, e.g. "keith/"
	DefaultBranch string `json:"default_branch,omitempty"` // override default branch detection, e.g. "main"
	Claim         string `json:"claim,omitempty"`          // how long to claim an item, e.g. "2h"
}

// AgentSettings controls headless agent execution (wn do, wn agent-orch).
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type AgentSettings struct {
	Cmd           string `json:"cmd,omitempty"`            // command template, e.g. "cursor agent --print --trust \"{{.Prompt}}\""
	Prompt        string `json:"prompt,omitempty"`         // prompt template, e.g. "{{.Description}}"
	Delay         string `json:"delay,omitempty"`          // delay between items, e.g. "5m"
	Poll          string `json:"poll,omitempty"`           // poll interval when queue empty, e.g. "60s"
	LeaveWorktree bool   `json:"leave_worktree,omitempty"` // if true, keep worktree after agent finishes
}

// ShowSettings holds user-level defaults for the show command and bare 'wn [id]'.
type ShowSettings struct {
	// DefaultFields is a comma-separated list of fields to display.
	// Valid fields: title, body, status, deps, notes, log.
	// Example: "title,body,deps,notes"
	DefaultFields string `json:"default_fields,omitempty"`
}

// CleanupSettings holds user-level defaults for cleanup utilities (wn cleanup ...).
type CleanupSettings struct {
	// CloseDoneItemsAge is the default age threshold for "wn cleanup close-done-items"
	// when --age is not provided (e.g. "30d", "7d", "48h").
	CloseDoneItemsAge string `json:"close_done_items_age,omitempty"`
}

// SettingsPath returns the path to the user's wn settings file.
func SettingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "wn", "settings.json"), nil
}

// ProjectSettingsPath returns the path to the project-level settings file under root (.wn/settings.json).
func ProjectSettingsPath(root string) string {
	return filepath.Join(root, ".wn", "settings.json")
}

// MergeSettings overlays project onto user. Non-empty project fields override user; empty project fields leave user values.
func MergeSettings(user, project Settings) Settings {
	out := user
	if project.Sort != "" {
		out.Sort = project.Sort
	}
	if project.Picker != "" {
		out.Picker = project.Picker
	}
	out.Next = mergeNext(user.Next, project.Next)
	out.Worktree = mergeWorktree(user.Worktree, project.Worktree)
	out.Agent = mergeAgent(user.Agent, project.Agent)
	out.Cleanup = mergeCleanup(user.Cleanup, project.Cleanup)
	out.Show = mergeShow(user.Show, project.Show)
	return out
}

func mergeNext(user, project NextSettings) NextSettings {
	out := user
	if project.Tag != "" {
		out.Tag = project.Tag
	}
	return out
}

func mergeWorktree(user, project WorktreeSettings) WorktreeSettings {
	out := user
	if project.Base != "" {
		out.Base = project.Base
	}
	if project.BranchPrefix != "" {
		out.BranchPrefix = project.BranchPrefix
	}
	if project.DefaultBranch != "" {
		out.DefaultBranch = project.DefaultBranch
	}
	if project.Claim != "" {
		out.Claim = project.Claim
	}
	return out
}

func mergeAgent(user, project AgentSettings) AgentSettings {
	out := user
	if project.Cmd != "" {
		out.Cmd = project.Cmd
	}
	if project.Prompt != "" {
		out.Prompt = project.Prompt
	}
	if project.Delay != "" {
		out.Delay = project.Delay
	}
	if project.Poll != "" {
		out.Poll = project.Poll
	}
	if project.LeaveWorktree {
		out.LeaveWorktree = project.LeaveWorktree
	}
	return out
}

func mergeShow(user, project ShowSettings) ShowSettings {
	out := user
	if project.DefaultFields != "" {
		out.DefaultFields = project.DefaultFields
	}
	return out
}

func mergeCleanup(user, project CleanupSettings) CleanupSettings {
	out := user
	if project.CloseDoneItemsAge != "" {
		out.CloseDoneItemsAge = project.CloseDoneItemsAge
	}
	return out
}

// ReadSettings reads the user's settings. Missing file returns empty Settings, no error.
func ReadSettings() (Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return Settings{}, err
	}
	return readSettingsFromPath(path)
}

// ReadSettingsInRoot returns effective settings for the given project root: user settings with optional project overrides from root/.wn/settings.json. When root is empty, returns user settings only. Missing project file is ignored (user settings only).
func ReadSettingsInRoot(root string) (Settings, error) {
	user, err := ReadSettings()
	if err != nil {
		return Settings{}, err
	}
	if root == "" {
		return user, nil
	}
	projectPath := ProjectSettingsPath(root)
	project, err := readSettingsFromPath(projectPath)
	if err != nil {
		return user, nil
	}
	return MergeSettings(user, project), nil
}

// readSettingsFromPath reads settings from a specific path (for tests).
func readSettingsFromPath(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SortSpecFromSettings returns parsed sort options from settings, or nil if empty/invalid.
// Invalid spec is ignored (returns nil).
func SortSpecFromSettings(settings Settings) []SortOption {
	if settings.Sort == "" {
		return nil
	}
	spec, err := ParseSortSpec(settings.Sort)
	if err != nil {
		return nil
	}
	return spec
}
