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
		"next": {
			"tag": "agent"
		},
		"worktree": {
			"base": "./.wn/worktrees",
			"branch_prefix": "keith/",
			"default_branch": "main",
			"claim": "2h"
		},
		"agent": {
			"cmd": "cursor agent --print --trust \"{{.Prompt}}\"",
			"prompt": "{{.Description}}",
			"delay": "5m",
			"poll": "60s",
			"leave_worktree": true
		},
		"cleanup": {
			"close_done_items_age": "30d"
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
	if got.Next.Tag != "agent" {
		t.Errorf("Next.Tag = %q, want agent", got.Next.Tag)
	}
	wt := got.Worktree
	if wt.Base != "./.wn/worktrees" || wt.BranchPrefix != "keith/" || wt.DefaultBranch != "main" || wt.Claim != "2h" {
		t.Errorf("Worktree = %+v, unexpected values", wt)
	}
	ag := got.Agent
	if ag.Cmd == "" || ag.Prompt != "{{.Description}}" || ag.Delay != "5m" || ag.Poll != "60s" || !ag.LeaveWorktree {
		t.Errorf("Agent = %+v, unexpected values", ag)
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
