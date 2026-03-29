package wn

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_wn_pick_byID(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// abc123 is the current task; change to a new item
	store, err := NewFileStore(".")
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	other := &Item{
		ID:          "def456",
		Description: "second item",
		Created:     now,
		Updated:     now,
		Tags:        []string{},
		DependsOn:   []string{},
		Notes:       []Note{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(other); err != nil {
		t.Fatalf("Put: %v", err)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_pick",
		Arguments: map[string]any{"id": "def456"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_pick: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_pick returned error: %s", textContent(res))
	}
	var out struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_pick response not valid JSON: %v\ncontent: %s", err, textContent(res))
	}
	if out.ID != "def456" {
		t.Errorf("wn_pick id = %q, want def456", out.ID)
	}

	// Verify meta was updated
	meta, err := ReadMeta(".")
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "def456" {
		t.Errorf("CurrentID = %q, want def456", meta.CurrentID)
	}
}

func TestMCP_wn_pick_invalidID(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_pick",
		Arguments: map[string]any{"id": "notexist"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_pick: %v", err)
	}
	if !res.IsError {
		t.Fatalf("wn_pick with invalid id: expected IsError, got success: %s", textContent(res))
	}
	if !strings.Contains(textContent(res), "notexist") {
		t.Errorf("error should mention the id; got %q", textContent(res))
	}
}

func TestMCP_wn_pick_noID(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_pick",
		Arguments: map[string]any{},
	})
	if err != nil {
		// Schema validation error is also acceptable
		return
	}
	if !res.IsError {
		t.Fatalf("wn_pick without id: expected IsError or schema error, got success: %s", textContent(res))
	}
}

func TestMCP_wn_pick_dot(t *testing.T) {
	ctx, cs, store, dir, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	// Create a feature branch and switch to it
	execIn(t, dir, "git", "checkout", "-b", "feat/my-feature")

	// Add an item with a wn:branch note matching the current branch
	now := time.Now().UTC()
	item := &Item{
		ID:          "feat01",
		Description: "feature item",
		Created:     now,
		Updated:     now,
		Tags:        []string{},
		DependsOn:   []string{},
		Notes:       []Note{{Name: NoteNameBranch, Body: "feat/my-feature", Created: now}},
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_pick",
		Arguments: map[string]any{"id": ".", "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_pick .: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_pick . returned error: %s", textContent(res))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_pick . response not valid JSON: %v\ncontent: %s", err, textContent(res))
	}
	if out.ID != "feat01" {
		t.Errorf("wn_pick . id = %q, want feat01", out.ID)
	}

	// Verify meta was updated
	meta, err := ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.CurrentID != "feat01" {
		t.Errorf("CurrentID = %q, want feat01", meta.CurrentID)
	}
}

func TestMCP_wn_pick_dot_noMatch(t *testing.T) {
	ctx, cs, _, dir, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	// Switch to a branch with no matching wn:branch note
	execIn(t, dir, "git", "checkout", "-b", "orphan-branch")

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_pick",
		Arguments: map[string]any{"id": ".", "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_pick .: %v", err)
	}
	if !res.IsError {
		t.Fatalf("wn_pick . with no match: expected IsError, got success: %s", textContent(res))
	}
	if !strings.Contains(textContent(res), "orphan-branch") {
		t.Errorf("error should mention branch name; got %q", textContent(res))
	}
}
