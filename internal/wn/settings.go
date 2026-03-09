package wn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RunnerConfig defines an agent command profile (cmd template, optional prompt override, worktree behavior).
type RunnerConfig struct {
	Cmd           string `json:"cmd"`
	Prompt        string `json:"prompt,omitempty"`
	LeaveWorktree bool   `json:"leave_worktree,omitempty"`
}

// Settings is the user's wn configuration (e.g. ~/.config/wn/settings.json).
type Settings struct {
	Sort     string                  `json:"sort,omitempty"`     // e.g. "updated:desc,priority,tags"
	Picker   string                  `json:"picker,omitempty"`   // interactive picker: "fzf", "numbered", or "" (auto-detect)
	Runners  map[string]RunnerConfig `json:"runners,omitempty"`  // named agent profiles, e.g. "claude", "cursor"
	Next     NextSettings            `json:"next,omitempty"`     // defaults for next-item selection
	Worktree WorktreeSettings        `json:"worktree,omitempty"` // defaults for worktree setup
	Agent    AgentSettings           `json:"agent,omitempty"`    // defaults for agent runs (wn do, wn launch)
	Cleanup  CleanupSettings         `json:"cleanup,omitempty"`  // options for cleanup subcommands
	Show     ShowSettings            `json:"show,omitempty"`     // defaults for wn show / bare wn
}

// NextSettings controls how the next work item is selected.
type NextSettings struct {
	Tag string `json:"tag,omitempty"` // only consider items that have this tag, e.g. "agent"
}

// WorktreeSettings controls worktree creation.
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type WorktreeSettings struct {
	Base          string `json:"base,omitempty"`           // base directory for worktrees, e.g. "../worktrees"
	BranchPrefix  string `json:"branch_prefix,omitempty"`  // prefix for generated branch names, e.g. "keith/"
	DefaultBranch string `json:"default_branch,omitempty"` // override default branch detection, e.g. "main"
	Claim         string `json:"claim,omitempty"`          // how long to claim an item, e.g. "2h"
}

// AgentSettings controls agent execution (wn do, wn launch).
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type AgentSettings struct {
	Default       string `json:"default,omitempty"`        // default runner name for wn do (sync)
	DefaultLaunch string `json:"default_launch,omitempty"` // default runner name for wn launch (async)
	Delay         string `json:"delay,omitempty"`          // delay between runs in loop mode, e.g. "5m"
	Poll          string `json:"poll,omitempty"`           // poll interval when queue empty, e.g. "60s"
}

// ShowSettings holds user-level defaults for the show command and bare 'wn [id]'.
type ShowSettings struct {
	DefaultFields string `json:"default_fields,omitempty"`
}

// CleanupSettings holds user-level defaults for cleanup utilities (wn cleanup ...).
type CleanupSettings struct {
	CloseDoneItemsAge string `json:"close_done_items_age,omitempty"`
}

// ResolveRunner returns the RunnerConfig for the given name. If name is empty, uses agent.default.
// Returns an error if no runner name can be determined or the named runner is not found.
func ResolveRunner(settings Settings, name string) (RunnerConfig, error) {
	resolved := name
	if resolved == "" {
		resolved = settings.Agent.Default
	}
	if resolved == "" {
		return RunnerConfig{}, fmt.Errorf("no runner specified and no agent.default configured in settings")
	}
	r, ok := settings.Runners[resolved]
	if !ok {
		return RunnerConfig{}, fmt.Errorf("runner %q not found in settings.runners", resolved)
	}
	return r, nil
}

// ResolveLaunchRunner is like ResolveRunner but uses agent.default_launch as the fallback.
func ResolveLaunchRunner(settings Settings, name string) (RunnerConfig, error) {
	resolved := name
	if resolved == "" {
		resolved = settings.Agent.DefaultLaunch
	}
	if resolved == "" {
		return RunnerConfig{}, fmt.Errorf("no runner specified and no agent.default_launch configured in settings")
	}
	r, ok := settings.Runners[resolved]
	if !ok {
		return RunnerConfig{}, fmt.Errorf("runner %q not found in settings.runners", resolved)
	}
	return r, nil
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
// Runners are merged by key: project runners override same-named user runners; unique keys from each are kept.
func MergeSettings(user, project Settings) Settings {
	out := user
	if project.Sort != "" {
		out.Sort = project.Sort
	}
	if project.Picker != "" {
		out.Picker = project.Picker
	}
	out.Runners = mergeRunners(user.Runners, project.Runners)
	out.Next = mergeNext(user.Next, project.Next)
	out.Worktree = mergeWorktree(user.Worktree, project.Worktree)
	out.Agent = mergeAgent(user.Agent, project.Agent)
	out.Cleanup = mergeCleanup(user.Cleanup, project.Cleanup)
	out.Show = mergeShow(user.Show, project.Show)
	return out
}

func mergeRunners(user, project map[string]RunnerConfig) map[string]RunnerConfig {
	if len(user) == 0 && len(project) == 0 {
		return nil
	}
	out := make(map[string]RunnerConfig)
	for k, v := range user {
		out[k] = v
	}
	for k, v := range project {
		out[k] = v
	}
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
	if project.Default != "" {
		out.Default = project.Default
	}
	if project.DefaultLaunch != "" {
		out.DefaultLaunch = project.DefaultLaunch
	}
	if project.Delay != "" {
		out.Delay = project.Delay
	}
	if project.Poll != "" {
		out.Poll = project.Poll
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
