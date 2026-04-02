package wn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSettings_missingFile(t *testing.T) {
	// ReadSettings should return empty settings and no error when file does not exist
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("readSettingsFromPath(missing) err = %v", err)
	}
	if got.Sort != "" {
		t.Errorf("Sort = %q, want empty", got.Sort)
	}
}

func TestReadSettings_withSort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"sort":"updated:desc,priority"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Sort != "updated:desc,priority" {
		t.Errorf("Sort = %q, want updated:desc,priority", got.Sort)
	}
}

func TestReadSettings_newStructure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	body := `{
		"sort": "updated:desc",
		"next": { "tag": "agent" },
		"worktree": {
			"base": "./.wn/worktrees",
			"branch_prefix": "keith/",
			"default_branch": "main",
			"claim": "2h"
		},
		"runners": {
			"cursor": {
				"cmd": "cursor agent --print --trust \"{{.Prompt}}\"",
				"prompt": "{{.Description}}"
			}
		},
		"agent": {
			"default": "cursor",
			"default_launch": "tmux-cursor",
			"delay": "5m",
			"poll": "60s"
		},
		"cleanup": { "close_done_items_age": "30d" }
	}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Sort != "updated:desc" {
		t.Errorf("Sort = %q, want updated:desc", got.Sort)
	}
	if got.Next.Tag != "agent" {
		t.Errorf("Next.Tag = %q, want agent", got.Next.Tag)
	}
	wt := got.Worktree
	if wt.Base != "./.wn/worktrees" || wt.BranchPrefix != "keith/" || wt.DefaultBranch != "main" || wt.Claim != "2h" {
		t.Errorf("Worktree = %+v, unexpected values", wt)
	}
	ag := got.Agent
	if ag.Default != "cursor" || ag.DefaultLaunch != "tmux-cursor" || ag.Delay != "5m" || ag.Poll != "60s" {
		t.Errorf("Agent = %+v, unexpected values", ag)
	}
	runner, ok := got.Runners["cursor"]
	if !ok {
		t.Fatal("Runners[cursor] not found")
	}
	if runner.Cmd == "" || runner.Prompt != "{{.Description}}" {
		t.Errorf("Runners[cursor] = %+v, unexpected values", runner)
	}
	if got.Cleanup.CloseDoneItemsAge != "30d" {
		t.Errorf("Cleanup.CloseDoneItemsAge = %q, want 30d", got.Cleanup.CloseDoneItemsAge)
	}
}

func TestMergeSettings_projectOverridesUser(t *testing.T) {
	user := Settings{
		Sort:     "updated:desc",
		Cleanup:  CleanupSettings{CloseDoneItemsAge: "30d"},
		Worktree: WorktreeSettings{Claim: "1h", DefaultBranch: "main"},
		Next:     NextSettings{Tag: "user-tag"},
		Agent:    AgentSettings{Delay: "5m"},
	}
	project := Settings{
		Sort:     "created:asc",
		Cleanup:  CleanupSettings{CloseDoneItemsAge: "7d"},
		Worktree: WorktreeSettings{Claim: "2h"},
	}
	merged := MergeSettings(user, project)
	if merged.Sort != "created:asc" {
		t.Errorf("Sort = %q, want created:asc", merged.Sort)
	}
	if merged.Cleanup.CloseDoneItemsAge != "7d" {
		t.Errorf("Cleanup.CloseDoneItemsAge = %q, want 7d", merged.Cleanup.CloseDoneItemsAge)
	}
	if merged.Worktree.Claim != "2h" {
		t.Errorf("Worktree.Claim = %q, want 2h", merged.Worktree.Claim)
	}
	if merged.Worktree.DefaultBranch != "main" {
		t.Errorf("Worktree.DefaultBranch = %q, want main (from user)", merged.Worktree.DefaultBranch)
	}
	if merged.Agent.Delay != "5m" {
		t.Errorf("Agent.Delay = %q, want 5m (from user)", merged.Agent.Delay)
	}
	if merged.Next.Tag != "user-tag" {
		t.Errorf("Next.Tag = %q, want user-tag (from user)", merged.Next.Tag)
	}
}

func TestMergeSettings_emptyProjectReturnsUser(t *testing.T) {
	user := Settings{Sort: "updated:desc", Worktree: WorktreeSettings{Claim: "1h"}}
	merged := MergeSettings(user, Settings{})
	if merged.Sort != "updated:desc" || merged.Worktree.Claim != "1h" {
		t.Errorf("merged = %+v, want user settings unchanged", merged)
	}
}

func TestReadSettings_withPicker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"picker":"fzf"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Picker != "fzf" {
		t.Errorf("Picker = %q, want fzf", got.Picker)
	}
}

func TestMergeSettings_picker(t *testing.T) {
	user := Settings{Picker: "fzf"}
	project := Settings{Picker: "numbered"}
	merged := MergeSettings(user, project)
	if merged.Picker != "numbered" {
		t.Errorf("Picker = %q, want numbered (project overrides user)", merged.Picker)
	}
	// Empty project leaves user value
	merged2 := MergeSettings(user, Settings{})
	if merged2.Picker != "fzf" {
		t.Errorf("Picker = %q, want fzf (user preserved)", merged2.Picker)
	}
}

func TestMergeSettings_showDefaultFields(t *testing.T) {
	user := Settings{Show: ShowSettings{DefaultFields: "title,body"}}
	project := Settings{Show: ShowSettings{DefaultFields: "title,body,deps"}}
	merged := MergeSettings(user, project)
	if merged.Show.DefaultFields != "title,body,deps" {
		t.Errorf("Show.DefaultFields = %q, want title,body,deps (project overrides user)", merged.Show.DefaultFields)
	}
	// Empty project show leaves user value
	merged2 := MergeSettings(user, Settings{})
	if merged2.Show.DefaultFields != "title,body" {
		t.Errorf("Show.DefaultFields = %q, want title,body (user unchanged)", merged2.Show.DefaultFields)
	}
}

func TestReadSettings_withShow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"show":{"default_fields":"title,body,status"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Show.DefaultFields != "title,body,status" {
		t.Errorf("Show.DefaultFields = %q, want title,body,status", got.Show.DefaultFields)
	}
}

func TestReadSettingsInRoot_withProjectFile_mergesProjectOverUser(t *testing.T) {
	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"updated:desc","worktree":{"claim":"1h"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	projectRoot := t.TempDir()
	wnDir := filepath.Join(projectRoot, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(wnDir, "settings.json")
	if err := os.WriteFile(projectPath, []byte(`{"sort":"created:asc"}`), 0644); err != nil {
		t.Fatal(err)
	}
	userSettings, err := readSettingsFromPath(userPath)
	if err != nil {
		t.Fatal(err)
	}
	projectSettings, err := readSettingsFromPath(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	merged := MergeSettings(userSettings, projectSettings)
	if merged.Sort != "created:asc" {
		t.Errorf("Sort = %q, want created:asc", merged.Sort)
	}
	if merged.Worktree.Claim != "1h" {
		t.Errorf("Worktree.Claim = %q, want 1h (from user)", merged.Worktree.Claim)
	}
}

func TestProjectSettingsPath(t *testing.T) {
	got := ProjectSettingsPath("/foo/bar")
	want := filepath.Join("/foo/bar", ".wn", "settings.json")
	if got != want {
		t.Errorf("ProjectSettingsPath = %q, want %q", got, want)
	}
}

func TestReadSettings_withRunners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	body := `{
		"runners": {
			"claude": {"cmd": "claude --print \"{{.Prompt}}\""},
			"cursor": {"cmd": "cursor agent --print \"{{.Prompt}}\""}
		},
		"agent": {"default": "claude", "default_launch": "cursor"}
	}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got.Runners) != 2 {
		t.Fatalf("len(Runners) = %d, want 2", len(got.Runners))
	}
	claude, ok := got.Runners["claude"]
	if !ok || claude.Cmd == "" {
		t.Errorf("Runners[claude] = %+v, want non-empty cmd", claude)
	}
	cursor, ok := got.Runners["cursor"]
	if !ok || cursor.Cmd == "" {
		t.Errorf("Runners[cursor] = %+v, want non-empty cmd", cursor)
	}
	if got.Agent.Default != "claude" {
		t.Errorf("Agent.Default = %q, want claude", got.Agent.Default)
	}
	if got.Agent.DefaultLaunch != "cursor" {
		t.Errorf("Agent.DefaultLaunch = %q, want cursor", got.Agent.DefaultLaunch)
	}
}

func TestResolveRunner_found(t *testing.T) {
	s := Settings{
		Runners: map[string]RunnerConfig{
			"claude": {Cmd: "claude --print \"{{.Prompt}}\""},
		},
		Agent: AgentSettings{Default: "claude"},
	}
	r, err := ResolveRunner(s, "claude")
	if err != nil {
		t.Fatalf("ResolveRunner: %v", err)
	}
	if r.Cmd == "" {
		t.Error("RunnerConfig.Cmd is empty")
	}
}

func TestResolveRunner_usesDefault(t *testing.T) {
	s := Settings{
		Runners: map[string]RunnerConfig{
			"claude": {Cmd: "claude --print \"{{.Prompt}}\""},
		},
		Agent: AgentSettings{Default: "claude"},
	}
	r, err := ResolveRunner(s, "")
	if err != nil {
		t.Fatalf("ResolveRunner with empty name: %v", err)
	}
	if r.Cmd == "" {
		t.Error("RunnerConfig.Cmd is empty")
	}
}

func TestResolveRunner_notFound(t *testing.T) {
	s := Settings{
		Runners: map[string]RunnerConfig{
			"claude": {Cmd: "claude --print \"{{.Prompt}}\""},
		},
	}
	_, err := ResolveRunner(s, "nonexistent")
	if err == nil {
		t.Error("ResolveRunner(nonexistent) should error")
	}
}

func TestResolveRunner_noDefault(t *testing.T) {
	s := Settings{}
	_, err := ResolveRunner(s, "")
	if err == nil {
		t.Error("ResolveRunner with empty name and no agent.default should error")
	}
}

func TestResolveLaunchRunner_usesDefaultLaunch(t *testing.T) {
	s := Settings{
		Runners: map[string]RunnerConfig{
			"tmux-claude": {Cmd: "tmux new-window -c {{.Worktree}} 'claude -p \"{{.Prompt}}\"'"},
		},
		Agent: AgentSettings{DefaultLaunch: "tmux-claude"},
	}
	r, err := ResolveLaunchRunner(s, "")
	if err != nil {
		t.Fatalf("ResolveLaunchRunner: %v", err)
	}
	if r.Cmd == "" {
		t.Error("RunnerConfig.Cmd should be set for tmux-claude")
	}
}

func TestResolveLaunchRunner_noDefaultLaunch(t *testing.T) {
	s := Settings{}
	_, err := ResolveLaunchRunner(s, "")
	if err == nil {
		t.Error("ResolveLaunchRunner with no agent.default_launch should error")
	}
}

func TestMergeSettings_runnersMergeByKey(t *testing.T) {
	user := Settings{
		Runners: map[string]RunnerConfig{
			"claude": {Cmd: "claude-user-cmd"},
			"cursor": {Cmd: "cursor-user-cmd"},
		},
	}
	project := Settings{
		Runners: map[string]RunnerConfig{
			"claude":      {Cmd: "claude-project-cmd"}, // overrides user
			"tmux-claude": {Cmd: "tmux-cmd"},           // new runner
		},
	}
	merged := MergeSettings(user, project)
	if merged.Runners["claude"].Cmd != "claude-project-cmd" {
		t.Errorf("claude runner = %q, want project override", merged.Runners["claude"].Cmd)
	}
	if merged.Runners["cursor"].Cmd != "cursor-user-cmd" {
		t.Errorf("cursor runner = %q, want user value preserved", merged.Runners["cursor"].Cmd)
	}
	if merged.Runners["tmux-claude"].Cmd != "tmux-cmd" {
		t.Errorf("tmux-claude runner = %q, want project addition", merged.Runners["tmux-claude"].Cmd)
	}
}

func TestMergeSettings_runnersEmptyProjectPreservesUser(t *testing.T) {
	user := Settings{
		Runners: map[string]RunnerConfig{
			"claude": {Cmd: "claude-cmd"},
		},
	}
	merged := MergeSettings(user, Settings{})
	if merged.Runners["claude"].Cmd != "claude-cmd" {
		t.Errorf("claude runner = %q, want user value preserved", merged.Runners["claude"].Cmd)
	}
}

func TestMergeSettings_agentDefaultMerged(t *testing.T) {
	user := Settings{Agent: AgentSettings{Default: "claude", Delay: "5m"}}
	project := Settings{Agent: AgentSettings{Default: "cursor"}}
	merged := MergeSettings(user, project)
	if merged.Agent.Default != "cursor" {
		t.Errorf("Agent.Default = %q, want cursor (project overrides)", merged.Agent.Default)
	}
	if merged.Agent.Delay != "5m" {
		t.Errorf("Agent.Delay = %q, want 5m (from user)", merged.Agent.Delay)
	}
}

func TestReadSettings_withViews(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	body := `{
		"views": {
			"agent": "--tag agent --sort priority",
			"myview": "--all --group status"
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got.Views) != 2 {
		t.Fatalf("len(Views) = %d, want 2", len(got.Views))
	}
	if got.Views["agent"] != "--tag agent --sort priority" {
		t.Errorf("Views[agent] = %q, want --tag agent --sort priority", got.Views["agent"])
	}
	if got.Views["myview"] != "--all --group status" {
		t.Errorf("Views[myview] = %q, want --all --group status", got.Views["myview"])
	}
}

func TestMergeSettings_viewsMergeByKey(t *testing.T) {
	user := Settings{
		Views: map[string]string{
			"agent":   "--tag agent",
			"backlog": "--all --sort priority",
		},
	}
	project := Settings{
		Views: map[string]string{
			"agent":   "--tag agent --sort updated:desc", // overrides user
			"project": "--tag myproject",                 // new view
		},
	}
	merged := MergeSettings(user, project)
	if merged.Views["agent"] != "--tag agent --sort updated:desc" {
		t.Errorf("Views[agent] = %q, want project override", merged.Views["agent"])
	}
	if merged.Views["backlog"] != "--all --sort priority" {
		t.Errorf("Views[backlog] = %q, want user value preserved", merged.Views["backlog"])
	}
	if merged.Views["project"] != "--tag myproject" {
		t.Errorf("Views[project] = %q, want project addition", merged.Views["project"])
	}
}

func TestMergeSettings_viewsEmptyProjectPreservesUser(t *testing.T) {
	user := Settings{
		Views: map[string]string{
			"agent": "--tag agent",
		},
	}
	merged := MergeSettings(user, Settings{})
	if merged.Views["agent"] != "--tag agent" {
		t.Errorf("Views[agent] = %q, want user value preserved", merged.Views["agent"])
	}
}

func TestResolveView_found(t *testing.T) {
	s := Settings{
		Views: map[string]string{
			"agent": "--tag agent --sort priority",
		},
	}
	args, err := ResolveView(s, "agent")
	if err != nil {
		t.Fatalf("ResolveView: %v", err)
	}
	if len(args) == 0 {
		t.Error("ResolveView returned empty args")
	}
	// Should parse to ["--tag", "agent", "--sort", "priority"]
	found := false
	for _, a := range args {
		if a == "agent" {
			found = true
		}
	}
	if !found {
		t.Errorf("ResolveView args = %v, expected to contain 'agent'", args)
	}
}

func TestResolveView_notFound(t *testing.T) {
	s := Settings{
		Views: map[string]string{
			"agent": "--tag agent",
		},
	}
	_, err := ResolveView(s, "nonexistent")
	if err == nil {
		t.Error("ResolveView(nonexistent) should error")
	}
}

func TestResolveView_emptyViews(t *testing.T) {
	s := Settings{}
	_, err := ResolveView(s, "agent")
	if err == nil {
		t.Error("ResolveView with no views should error")
	}
}

// --- WN_SETTINGS_USER / WN_SETTINGS_USER_LOCAL tests ---

func TestUserSettingsNamedPaths_default(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", dir)
	t.Setenv("WN_SETTINGS_USER", "")
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")
	named, err := UserSettingsNamedPaths()
	if err != nil {
		t.Fatalf("UserSettingsNamedPaths: %v", err)
	}
	if len(named) != 1 {
		t.Fatalf("len(named) = %d, want 1", len(named))
	}
	if named[0].Name != "user" {
		t.Errorf("Name = %q, want user", named[0].Name)
	}
	want := filepath.Join(dir, "settings.json")
	if named[0].Path != want {
		t.Errorf("Path = %q, want %q", named[0].Path, want)
	}
}

func TestUserSettingsNamedPaths_WN_SETTINGS_USER(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom.json")
	t.Setenv("WN_SETTINGS_USER", customPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")
	named, err := UserSettingsNamedPaths()
	if err != nil {
		t.Fatalf("UserSettingsNamedPaths: %v", err)
	}
	if len(named) != 1 {
		t.Fatalf("len(named) = %d, want 1", len(named))
	}
	if named[0].Name != "user" || named[0].Path != customPath {
		t.Errorf("named[0] = %+v, want {user, %s}", named[0], customPath)
	}
}

func TestUserSettingsNamedPaths_WN_SETTINGS_USER_LOCAL(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user.json")
	localPath := filepath.Join(dir, "local.json")
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", localPath)
	named, err := UserSettingsNamedPaths()
	if err != nil {
		t.Fatalf("UserSettingsNamedPaths: %v", err)
	}
	if len(named) != 2 {
		t.Fatalf("len(named) = %d, want 2", len(named))
	}
	if named[0].Name != "user" || named[0].Path != userPath {
		t.Errorf("named[0] = %+v, want {user, %s}", named[0], userPath)
	}
	if named[1].Name != "user-local" || named[1].Path != localPath {
		t.Errorf("named[1] = %+v, want {user-local, %s}", named[1], localPath)
	}
}

func TestReadSettings_WN_SETTINGS_USER(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"sort":"wn-settings-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", path)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")
	got, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if got.Sort != "wn-settings-sort" {
		t.Errorf("Sort = %q, want wn-settings-sort", got.Sort)
	}
}

func TestReadSettings_userLocalOverridesUser(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user.json")
	localPath := filepath.Join(dir, "local.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"user-sort","picker":"fzf"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte(`{"sort":"local-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", localPath)
	got, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if got.Sort != "local-sort" {
		t.Errorf("Sort = %q, want local-sort (local overrides user)", got.Sort)
	}
	if got.Picker != "fzf" {
		t.Errorf("Picker = %q, want fzf (from user, not overridden by local)", got.Picker)
	}
}

func TestReadSettings_WN_SETTINGS_USER_LOCAL_missingFile(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"user-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", filepath.Join(dir, "nonexistent.json"))
	got, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if got.Sort != "user-sort" {
		t.Errorf("Sort = %q, want user-sort (missing local file skipped)", got.Sort)
	}
}

// --- project settings.local.json tests ---

func TestProjectLocalSettingsPath(t *testing.T) {
	got := ProjectLocalSettingsPath("/foo/bar")
	want := filepath.Join("/foo/bar", ".wn", "settings.local.json")
	if got != want {
		t.Errorf("ProjectLocalSettingsPath = %q, want %q", got, want)
	}
}

func TestReadSettingsInRoot_localOverridesProject(t *testing.T) {
	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"user-sort","picker":"fzf"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	projectRoot := t.TempDir()
	wnDir := filepath.Join(projectRoot, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(wnDir, "settings.json")
	if err := os.WriteFile(projectPath, []byte(`{"sort":"project-sort","verify":"make all"}`), 0644); err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(wnDir, "settings.local.json")
	if err := os.WriteFile(localPath, []byte(`{"sort":"local-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadSettingsInRoot(projectRoot)
	if err != nil {
		t.Fatalf("ReadSettingsInRoot: %v", err)
	}
	// local overrides project
	if got.Sort != "local-sort" {
		t.Errorf("Sort = %q, want local-sort (settings.local.json overrides settings.json)", got.Sort)
	}
	// project value preserved when local doesn't set it
	if got.Verify != "make all" {
		t.Errorf("Verify = %q, want make all (from project settings.json)", got.Verify)
	}
	// user value preserved when neither project nor local sets it
	if got.Picker != "fzf" {
		t.Errorf("Picker = %q, want fzf (from user settings)", got.Picker)
	}
}

func TestReadSettingsInRoot_missingLocalFileIgnored(t *testing.T) {
	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"user-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	projectRoot := t.TempDir()
	wnDir := filepath.Join(projectRoot, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(wnDir, "settings.json")
	if err := os.WriteFile(projectPath, []byte(`{"sort":"project-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// No settings.local.json created

	got, err := ReadSettingsInRoot(projectRoot)
	if err != nil {
		t.Fatalf("ReadSettingsInRoot: %v", err)
	}
	if got.Sort != "project-sort" {
		t.Errorf("Sort = %q, want project-sort (local missing is fine)", got.Sort)
	}
}

func TestReadSettingsInRoot_localWithoutProjectFile(t *testing.T) {
	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"user-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	projectRoot := t.TempDir()
	wnDir := filepath.Join(projectRoot, ".wn")
	if err := os.MkdirAll(wnDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Only local file, no settings.json
	localPath := filepath.Join(wnDir, "settings.local.json")
	if err := os.WriteFile(localPath, []byte(`{"sort":"local-sort"}`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadSettingsInRoot(projectRoot)
	if err != nil {
		t.Fatalf("ReadSettingsInRoot: %v", err)
	}
	if got.Sort != "local-sort" {
		t.Errorf("Sort = %q, want local-sort (local file without settings.json)", got.Sort)
	}
}

func TestReadSettings_withVerify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"verify":"make all"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Verify != "make all" {
		t.Errorf("Verify = %q, want make all", got.Verify)
	}
}

func TestMergeSettings_verifyProjectOverridesUser(t *testing.T) {
	user := Settings{Verify: "make test"}
	project := Settings{Verify: "make all"}
	merged := MergeSettings(user, project)
	if merged.Verify != "make all" {
		t.Errorf("Verify = %q, want make all (project overrides user)", merged.Verify)
	}
}

func TestMergeSettings_verifyEmptyProjectPreservesUser(t *testing.T) {
	user := Settings{Verify: "make test"}
	merged := MergeSettings(user, Settings{})
	if merged.Verify != "make test" {
		t.Errorf("Verify = %q, want make test (user preserved)", merged.Verify)
	}
}

func TestMergeSettings_BranchTemplate(t *testing.T) {
	user := Settings{Worktree: WorktreeSettings{BranchTemplate: "{{.Slug}}"}}
	merged := MergeSettings(user, Settings{})
	if merged.Worktree.BranchTemplate != "{{.Slug}}" {
		t.Errorf("BranchTemplate = %q, want {{.Slug}} (user preserved)", merged.Worktree.BranchTemplate)
	}
	project := Settings{Worktree: WorktreeSettings{BranchTemplate: "{{.ID}}-{{.Slug}}"}}
	merged = MergeSettings(user, project)
	if merged.Worktree.BranchTemplate != "{{.ID}}-{{.Slug}}" {
		t.Errorf("BranchTemplate = %q, want {{.ID}}-{{.Slug}} (project overrides)", merged.Worktree.BranchTemplate)
	}
}

func TestMergeSettings_CommitTemplate(t *testing.T) {
	user := Settings{Agent: AgentSettings{CommitTemplate: "feat: {{.FirstLine}}"}}
	merged := MergeSettings(user, Settings{})
	if merged.Agent.CommitTemplate != "feat: {{.FirstLine}}" {
		t.Errorf("CommitTemplate = %q, want feat: {{.FirstLine}} (user preserved)", merged.Agent.CommitTemplate)
	}
	project := Settings{Agent: AgentSettings{CommitTemplate: "fix: {{.FirstLine}}"}}
	merged = MergeSettings(user, project)
	if merged.Agent.CommitTemplate != "fix: {{.FirstLine}}" {
		t.Errorf("CommitTemplate = %q, want fix: {{.FirstLine}} (project overrides)", merged.Agent.CommitTemplate)
	}
}

func TestReadSettings_BranchTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	body := `{"worktree":{"branch_template":"{{.Slug}}"},"agent":{"commit_template":"feat: {{.FirstLine}}"}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("readSettingsFromPath: %v", err)
	}
	if got.Worktree.BranchTemplate != "{{.Slug}}" {
		t.Errorf("BranchTemplate = %q, want {{.Slug}}", got.Worktree.BranchTemplate)
	}
	if got.Agent.CommitTemplate != "feat: {{.FirstLine}}" {
		t.Errorf("CommitTemplate = %q, want feat: {{.FirstLine}}", got.Agent.CommitTemplate)
	}
}
