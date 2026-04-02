package wn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NamedSettingsPath is a settings file path with an associated label (e.g. "user", "user-local").
type NamedSettingsPath struct {
	Name string
	Path string
}

// UserSettingsNamedPaths returns the ordered list of user-level settings file paths with labels.
// The user path comes from WN_SETTINGS_USER (~ expanded), or falls back to SettingsPath().
// An optional user-local path is added when WN_SETTINGS_USER_LOCAL is set (~ expanded).
func UserSettingsNamedPaths() ([]NamedSettingsPath, error) {
	userPath, err := userSettingsPath()
	if err != nil {
		return nil, err
	}
	result := []NamedSettingsPath{{Name: "user", Path: userPath}}
	if local := os.Getenv("WN_SETTINGS_USER_LOCAL"); local != "" {
		expanded, err := expandHomePath(local)
		if err != nil {
			return nil, err
		}
		result = append(result, NamedSettingsPath{Name: "user-local", Path: expanded})
	}
	return result, nil
}

// UserSettingsPaths returns the ordered list of user-level settings file paths.
func UserSettingsPaths() ([]string, error) {
	named, err := UserSettingsNamedPaths()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(named))
	for i, n := range named {
		paths[i] = n.Path
	}
	return paths, nil
}

// userSettingsPath returns the effective user settings path.
// Precedence: WN_SETTINGS_USER > SettingsPath() (which checks WN_CONFIG_DIR).
func userSettingsPath() (string, error) {
	if p := os.Getenv("WN_SETTINGS_USER"); p != "" {
		return expandHomePath(p)
	}
	return SettingsPath()
}

// expandHomePath expands a leading ~/ to the user's home directory.
func expandHomePath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// RunnerConfig defines an agent command profile (cmd template, optional prompt override).
type RunnerConfig struct {
	Cmd    string `json:"cmd"`
	Prompt string `json:"prompt,omitempty"`
}

// Settings is the user's wn configuration (e.g. ~/.config/wn/settings.json).
type Settings struct {
	Sort     string                  `json:"sort,omitempty"`     // e.g. "updated:desc,priority,tags"
	Picker   string                  `json:"picker,omitempty"`   // interactive picker: "fzf", "numbered", or "" (auto-detect)
	Verify   string                  `json:"verify,omitempty"`   // command to run for 'wn verify', e.g. "make all"
	Runners  map[string]RunnerConfig `json:"runners,omitempty"`  // named agent profiles, e.g. "claude", "cursor"
	Next     NextSettings            `json:"next,omitempty"`     // defaults for next-item selection
	Worktree WorktreeSettings        `json:"worktree,omitempty"` // defaults for worktree setup
	Agent    AgentSettings           `json:"agent,omitempty"`    // defaults for agent runs (wn do, wn launch)
	Cleanup  CleanupSettings         `json:"cleanup,omitempty"`  // options for cleanup subcommands
	Show     ShowSettings            `json:"show,omitempty"`     // defaults for wn show / bare wn
	Views    map[string]string       `json:"views,omitempty"`    // named filter+sort+group combos, e.g. "agent" = "--tag agent --sort priority"
}

// NextSettings controls how the next work item is selected.
type NextSettings struct {
	Tag string `json:"tag,omitempty"` // only consider items that have this tag, e.g. "agent"
}

// WorktreeSettings controls worktree creation.
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type WorktreeSettings struct {
	Base           string `json:"base,omitempty"`            // base directory for worktrees, e.g. "../worktrees"
	BranchPrefix   string `json:"branch_prefix,omitempty"`   // prefix for generated branch names, e.g. "keith/"
	BranchTemplate string `json:"branch_template,omitempty"` // Go template for branch base name; vars: {{.ID}}, {{.Slug}}; default: "wn-{{.ID}}-{{.Slug}}"
	DefaultBranch  string `json:"default_branch,omitempty"`  // override default branch detection, e.g. "main"
	Claim          string `json:"claim,omitempty"`           // how long to claim an item, e.g. "2h"
}

// AgentSettings controls agent execution (wn do, wn launch).
// Durations are strings parseable by time.ParseDuration (e.g. "2h", "30m").
type AgentSettings struct {
	Default        string `json:"default,omitempty"`         // default runner name for wn do (sync)
	DefaultLaunch  string `json:"default_launch,omitempty"`  // default runner name for wn launch (async)
	Delay          string `json:"delay,omitempty"`           // delay between runs in loop mode, e.g. "5m"
	Poll           string `json:"poll,omitempty"`            // poll interval when queue empty, e.g. "60s"
	CommitTemplate string `json:"commit_template,omitempty"` // Go template for auto-commit messages; vars: {{.ID}}, {{.FirstLine}}; default: "wn {{.ID}}: {{.FirstLine}}"
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
// If WN_CONFIG_DIR is set, it is used instead of the OS config dir (useful for tests).
func SettingsPath() (string, error) {
	if dir := os.Getenv("WN_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json"), nil
	}
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

// ProjectLocalSettingsPath returns the path to the user-local project settings file under root (.wn/settings.local.json).
// This file is intended to be gitignored and overrides .wn/settings.json on a field-by-field basis.
func ProjectLocalSettingsPath(root string) string {
	return filepath.Join(root, ".wn", "settings.local.json")
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
	if project.Verify != "" {
		out.Verify = project.Verify
	}
	out.Runners = mergeRunners(user.Runners, project.Runners)
	out.Next = mergeNext(user.Next, project.Next)
	out.Worktree = mergeWorktree(user.Worktree, project.Worktree)
	out.Agent = mergeAgent(user.Agent, project.Agent)
	out.Cleanup = mergeCleanup(user.Cleanup, project.Cleanup)
	out.Show = mergeShow(user.Show, project.Show)
	out.Views = mergeViews(user.Views, project.Views)
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
	if project.BranchTemplate != "" {
		out.BranchTemplate = project.BranchTemplate
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
	if project.CommitTemplate != "" {
		out.CommitTemplate = project.CommitTemplate
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

func mergeViews(user, project map[string]string) map[string]string {
	if len(user) == 0 && len(project) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range user {
		out[k] = v
	}
	for k, v := range project {
		out[k] = v
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

// ReadSettings reads the user's settings. Supports multiple files via WN_SETTINGS (comma-separated paths, last wins).
// Missing files are silently skipped. Returns empty Settings with no error if all files are missing.
func ReadSettings() (Settings, error) {
	paths, err := UserSettingsPaths()
	if err != nil {
		return Settings{}, err
	}
	var merged Settings
	for _, p := range paths {
		s, err := readSettingsFromPath(p)
		if err != nil {
			return Settings{}, err
		}
		merged = MergeSettings(merged, s)
	}
	return merged, nil
}

// ReadSettingsInRoot returns effective settings for the given project root: user settings with optional project overrides from root/.wn/settings.json, further overridden by root/.wn/settings.local.json. When root is empty, returns user settings only. Missing project files are ignored.
func ReadSettingsInRoot(root string) (Settings, error) {
	user, err := ReadSettings()
	if err != nil {
		return Settings{}, err
	}
	if root == "" {
		return user, nil
	}
	project, err := readSettingsFromPath(ProjectSettingsPath(root))
	if err != nil {
		return user, nil
	}
	merged := MergeSettings(user, project)
	local, err := readSettingsFromPath(ProjectLocalSettingsPath(root))
	if err != nil {
		return merged, nil
	}
	return MergeSettings(merged, local), nil
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

// ResolveView looks up a named view in settings and returns its flags as a slice of argument strings.
// Returns an error if the view name is not found.
func ResolveView(settings Settings, name string) ([]string, error) {
	flagsStr, ok := settings.Views[name]
	if !ok {
		return nil, fmt.Errorf("view %q not found in settings.views", name)
	}
	return splitShellArgs(flagsStr), nil
}

// splitShellArgs splits a flags string into individual arguments, respecting quoted strings.
// Supports single and double quotes. Does not handle escape sequences beyond simple quoting.
func splitShellArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	for _, r := range s {
		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				cur.WriteRune(r)
			}
		case inDouble:
			if r == '"' {
				inDouble = false
			} else {
				cur.WriteRune(r)
			}
		case r == '\'':
			inSingle = true
		case r == '"':
			inDouble = true
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
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
