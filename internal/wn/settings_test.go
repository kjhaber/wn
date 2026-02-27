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
