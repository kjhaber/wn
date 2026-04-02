package wn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsGetValue_topLevel(t *testing.T) {
	s := Settings{Sort: "updated:desc"}
	got, err := SettingsGetValue(s, "sort")
	if err != nil {
		t.Fatalf("SettingsGetValue: %v", err)
	}
	if got != "updated:desc" {
		t.Errorf("got %q, want %q", got, "updated:desc")
	}
}

func TestSettingsGetValue_nestedRunnerCmd(t *testing.T) {
	s := Settings{
		Runners: map[string]RunnerConfig{
			"tmux-claude": {Cmd: "wn-tmux-claude {{.Worktree}} {{.ItemID}}"},
		},
	}
	got, err := SettingsGetValue(s, "runners.tmux-claude.cmd")
	if err != nil {
		t.Fatalf("SettingsGetValue: %v", err)
	}
	if got != "wn-tmux-claude {{.Worktree}} {{.ItemID}}" {
		t.Errorf("got %q, want runner cmd string", got)
	}
}

func TestSettingsGetValue_runnerObject(t *testing.T) {
	s := Settings{
		Runners: map[string]RunnerConfig{
			"myrunner": {Cmd: "some-cmd"},
		},
	}
	got, err := SettingsGetValue(s, "runners.myrunner")
	if err != nil {
		t.Fatalf("SettingsGetValue: %v", err)
	}
	// Should return JSON object string
	if got == "" {
		t.Error("expected non-empty JSON object for runner")
	}
	if got[0] != '{' {
		t.Errorf("expected JSON object, got %q", got)
	}
}

func TestSettingsGetValue_missingKey(t *testing.T) {
	s := Settings{Sort: "updated:desc"}
	_, err := SettingsGetValue(s, "nonexistent")
	if err == nil {
		t.Error("expected error for missing key, got nil")
	}
}

func TestSettingsGetValue_missingNestedKey(t *testing.T) {
	s := Settings{}
	_, err := SettingsGetValue(s, "runners.norunner.cmd")
	if err == nil {
		t.Error("expected error for missing nested key, got nil")
	}
}

func TestSettingsGetValue_emptyKey(t *testing.T) {
	s := Settings{}
	_, err := SettingsGetValue(s, "")
	if err == nil {
		t.Error("expected error for empty key, got nil")
	}
}

func TestSettingsSetValue_newFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := SettingsSetValue(path, "sort", "created:asc"); err != nil {
		t.Fatalf("SettingsSetValue: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("readSettingsFromPath: %v", err)
	}
	_ = data
	if s.Sort != "created:asc" {
		t.Errorf("Sort = %q, want %q", s.Sort, "created:asc")
	}
}

func TestSettingsSetValue_topLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"picker":"fzf"}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SettingsSetValue(path, "sort", "priority"); err != nil {
		t.Fatalf("SettingsSetValue: %v", err)
	}

	s, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Sort != "priority" {
		t.Errorf("Sort = %q, want %q", s.Sort, "priority")
	}
	// Other fields preserved
	if s.Picker != "fzf" {
		t.Errorf("Picker = %q, want %q (should be preserved)", s.Picker, "fzf")
	}
}

func TestSettingsSetValue_nestedKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := SettingsSetValue(path, "runners.tmux-claude.cmd", "my-launcher {{.ItemID}}"); err != nil {
		t.Fatalf("SettingsSetValue: %v", err)
	}

	s, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := s.Runners["tmux-claude"]
	if !ok {
		t.Fatal("runners.tmux-claude not found after set")
	}
	if r.Cmd != "my-launcher {{.ItemID}}" {
		t.Errorf("cmd = %q, want %q", r.Cmd, "my-launcher {{.ItemID}}")
	}
}

func TestSettingsSetValue_runnerCmd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := SettingsSetValue(path, "runners.myrunner.cmd", "echo hello"); err != nil {
		t.Fatalf("SettingsSetValue: %v", err)
	}

	s, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := s.Runners["myrunner"]
	if !ok {
		t.Fatal("runners.myrunner not found after set")
	}
	if r.Cmd != "echo hello" {
		t.Errorf("cmd = %q, want echo hello", r.Cmd)
	}
}

func TestSettingsSetValue_preservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"sort":"updated:desc","runners":{"existing":{"cmd":"old-cmd"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SettingsSetValue(path, "runners.new.cmd", "new-cmd"); err != nil {
		t.Fatalf("SettingsSetValue: %v", err)
	}

	s, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Sort != "updated:desc" {
		t.Errorf("Sort = %q, want %q (should be preserved)", s.Sort, "updated:desc")
	}
	if existing, ok := s.Runners["existing"]; !ok || existing.Cmd != "old-cmd" {
		t.Error("existing runner should be preserved")
	}
	if newRunner, ok := s.Runners["new"]; !ok || newRunner.Cmd != "new-cmd" {
		t.Error("new runner should be set")
	}
}

func TestSettingsSetValue_emptyKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := SettingsSetValue(path, "", "value"); err == nil {
		t.Error("expected error for empty key")
	}
}
