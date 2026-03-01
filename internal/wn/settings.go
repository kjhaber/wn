package wn

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the user's wn configuration (e.g. ~/.config/wn/settings.json).
type Settings struct {
	Sort      string          `json:"sort,omitempty"`       // e.g. "updated:desc,priority,tags"
	AgentOrch AgentOrch       `json:"agent_orch,omitempty"` // defaults for wn agent-orch
	Cleanup   CleanupSettings `json:"cleanup,omitempty"`    // options for cleanup subcommands
}

// AgentOrch holds user-level defaults for the agent orchestrator (wn agent-orch).
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type AgentOrch struct {
	Claim         string `json:"claim,omitempty"`          // claim duration per item, e.g. "2h"
	Delay         string `json:"delay,omitempty"`          // delay between runs, e.g. "5m"
	Poll          string `json:"poll,omitempty"`           // poll interval when queue empty, e.g. "60s"
	AgentCmd      string `json:"agent_cmd,omitempty"`      // command template, e.g. "cursor agent --print --trust \"{{.Prompt}}\""
	PromptTpl     string `json:"prompt_tpl,omitempty"`     // prompt template, e.g. "{{.Description}}"
	Worktrees     string `json:"worktrees,omitempty"`      // worktree base path, e.g. "./.wn/worktrees"
	LeaveWorktree bool   `json:"leave_worktree,omitempty"` // true = leave worktree after run (default)
	Branch        string `json:"branch,omitempty"`         // default branch override, e.g. "main"
	BranchPrefix  string `json:"branch_prefix,omitempty"`  // prefix for generated branch names, e.g. "keith/"
	Tag           string `json:"tag,omitempty"`            // only consider items that have this tag
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
	out.Cleanup = mergeCleanup(user.Cleanup, project.Cleanup)
	out.AgentOrch = mergeAgentOrch(user.AgentOrch, project.AgentOrch)
	return out
}

func mergeCleanup(user, project CleanupSettings) CleanupSettings {
	out := user
	if project.CloseDoneItemsAge != "" {
		out.CloseDoneItemsAge = project.CloseDoneItemsAge
	}
	return out
}

func mergeAgentOrch(user, project AgentOrch) AgentOrch {
	out := user
	if project.Claim != "" {
		out.Claim = project.Claim
	}
	if project.Delay != "" {
		out.Delay = project.Delay
	}
	if project.Poll != "" {
		out.Poll = project.Poll
	}
	if project.AgentCmd != "" {
		out.AgentCmd = project.AgentCmd
	}
	if project.PromptTpl != "" {
		out.PromptTpl = project.PromptTpl
	}
	if project.Worktrees != "" {
		out.Worktrees = project.Worktrees
	}
	if project.LeaveWorktree {
		out.LeaveWorktree = project.LeaveWorktree
	}
	if project.Branch != "" {
		out.Branch = project.Branch
	}
	if project.BranchPrefix != "" {
		out.BranchPrefix = project.BranchPrefix
	}
	if project.Tag != "" {
		out.Tag = project.Tag
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
