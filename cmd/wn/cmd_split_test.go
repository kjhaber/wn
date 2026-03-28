package main

import (
	"bytes"
	"os"
	"testing"
)

// TestCommandFileStructure verifies that main.go has been split into per-command files.
// These are structural tests enforcing the desired file organization.
func TestCommandFileStructure(t *testing.T) {
	expectedFiles := []string{
		"cmd_add.go",
		"cmd_done.go",
		"cmd_status.go",
		"cmd_list.go",
		"cmd_note.go",
		"cmd_depend.go",
		"cmd_agent.go",
		"cmd_merge.go",
		"cmd_util.go",
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected per-command file %s to exist: %v", f, err)
		}
	}
}

func TestMainGoIsSmall(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}
	const maxLines = 100
	lines := bytes.Count(data, []byte("\n")) + 1
	if lines > maxLines {
		t.Errorf("main.go has %d lines; expected <= %d after split into per-command files", lines, maxLines)
	}
}
