package wn

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the user's wn configuration (e.g. ~/.config/wn/settings.json).
type Settings struct {
	Sort      string    `json:"sort,omitempty"`       // e.g. "updated:desc,priority,tags"
	AgentOrch AgentOrch `json:"agent_orch,omitempty"` // defaults for wn agent-orch
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

// SettingsPath returns the path to the user's wn settings file.
func SettingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "wn", "settings.json"), nil
}

// ReadSettings reads the user's settings. Missing file returns empty Settings, no error.
func ReadSettings() (Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return Settings{}, err
	}
	return readSettingsFromPath(path)
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
