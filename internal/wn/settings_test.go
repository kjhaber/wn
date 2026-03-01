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

func TestReadSettings_withAgentOrch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	body := `{
		"sort": "updated:desc",
		"cleanup": {
			"close_done_items_age": "30d"
		},
		"agent_orch": {
			"claim": "2h",
			"delay": "5m",
			"poll": "60s",
			"agent_cmd": "cursor agent --print --trust \"{{.Prompt}}\"",
			"prompt_tpl": "{{.Description}}",
			"worktrees": "./.wn/worktrees",
			"leave_worktree": true,
			"branch": "main",
			"branch_prefix": "keith/"
		}
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
	if got.Cleanup.CloseDoneItemsAge != "30d" {
		t.Errorf("Cleanup.CloseDoneItemsAge = %q, want 30d", got.Cleanup.CloseDoneItemsAge)
	}
	ao := got.AgentOrch
	if ao.Claim != "2h" || ao.Delay != "5m" || ao.Poll != "60s" {
		t.Errorf("AgentOrch claim/delay/poll = %q / %q / %q", ao.Claim, ao.Delay, ao.Poll)
	}
	if ao.AgentCmd == "" || ao.PromptTpl != "{{.Description}}" {
		t.Errorf("AgentOrch agent_cmd or prompt_tpl wrong: %q, %q", ao.AgentCmd, ao.PromptTpl)
	}
	if ao.Worktrees != "./.wn/worktrees" || !ao.LeaveWorktree || ao.Branch != "main" {
		t.Errorf("AgentOrch worktrees/leave_worktree/branch = %q / %v / %q", ao.Worktrees, ao.LeaveWorktree, ao.Branch)
	}
	if ao.BranchPrefix != "keith/" {
		t.Errorf("AgentOrch branch_prefix = %q, want keith/", ao.BranchPrefix)
	}
}

func TestMergeSettings_projectOverridesUser(t *testing.T) {
	user := Settings{
		Sort:      "updated:desc",
		Cleanup:   CleanupSettings{CloseDoneItemsAge: "30d"},
		AgentOrch: AgentOrch{Claim: "1h", Delay: "5m", Tag: "user-tag"},
	}
	project := Settings{
		Sort:      "created:asc",
		Cleanup:   CleanupSettings{CloseDoneItemsAge: "7d"},
		AgentOrch: AgentOrch{Claim: "2h"},
	}
	merged := MergeSettings(user, project)
	if merged.Sort != "created:asc" {
		t.Errorf("Sort = %q, want created:asc", merged.Sort)
	}
	if merged.Cleanup.CloseDoneItemsAge != "7d" {
		t.Errorf("Cleanup.CloseDoneItemsAge = %q, want 7d", merged.Cleanup.CloseDoneItemsAge)
	}
	if merged.AgentOrch.Claim != "2h" {
		t.Errorf("AgentOrch.Claim = %q, want 2h", merged.AgentOrch.Claim)
	}
	if merged.AgentOrch.Delay != "5m" {
		t.Errorf("AgentOrch.Delay = %q, want 5m (from user)", merged.AgentOrch.Delay)
	}
	if merged.AgentOrch.Tag != "user-tag" {
		t.Errorf("AgentOrch.Tag = %q, want user-tag (from user)", merged.AgentOrch.Tag)
	}
}

func TestMergeSettings_emptyProjectReturnsUser(t *testing.T) {
	user := Settings{Sort: "updated:desc", AgentOrch: AgentOrch{Claim: "1h"}}
	merged := MergeSettings(user, Settings{})
	if merged.Sort != "updated:desc" || merged.AgentOrch.Claim != "1h" {
		t.Errorf("merged = %+v, want user settings unchanged", merged)
	}
}

func TestReadSettingsInRoot_withProjectFile_mergesProjectOverUser(t *testing.T) {
	userDir := t.TempDir()
	userPath := filepath.Join(userDir, "settings.json")
	if err := os.WriteFile(userPath, []byte(`{"sort":"updated:desc","agent_orch":{"claim":"1h"}}`), 0644); err != nil {
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
	if merged.AgentOrch.Claim != "1h" {
		t.Errorf("AgentOrch.Claim = %q, want 1h (from user)", merged.AgentOrch.Claim)
	}
}

func TestProjectSettingsPath(t *testing.T) {
	got := ProjectSettingsPath("/foo/bar")
	want := filepath.Join("/foo/bar", ".wn", "settings.json")
	if got != want {
		t.Errorf("ProjectSettingsPath = %q, want %q", got, want)
	}
}
