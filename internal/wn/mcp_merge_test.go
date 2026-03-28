package wn

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_wn_merge_success(t *testing.T) {
	ctx, cs, store, dir, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	now := time.Now().UTC()
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "branch", "wn-abc999-feat")
	execIn(t, dir, "git", "checkout", "wn-abc999-feat")
	writeFile(t, dir+"/feat.txt", "feature")
	execIn(t, dir, "git", "add", "feat.txt")
	execIn(t, dir, "git", "commit", "-m", "feat")
	execIn(t, dir, "git", "checkout", def)

	item := &Item{
		ID:          "abc999",
		Description: "MCP merge test item",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: NoteNameBranch, Body: "wn-abc999-feat", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wn_merge",
		Arguments: map[string]any{
			"branch":  "wn-abc999-feat",
			"message": "Squash: MCP merge test",
		},
	})
	if err != nil {
		t.Fatalf("CallTool wn_merge: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_merge returned error: %s", textContent(res))
	}
	text := textContent(res)
	if !strings.Contains(text, "abc999") {
		t.Errorf("wn_merge response should mention item id; got %q", text)
	}

	got, err := store.Get("abc999")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !got.Done {
		t.Error("item should be marked done after wn_merge")
	}
	if got.NoteIndexByName(NoteNameCommit) < 0 {
		t.Error("item should have wn:commit note after wn_merge")
	}
}

func TestMCP_wn_merge_dryRun(t *testing.T) {
	ctx, cs, store, dir, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	now := time.Now().UTC()
	def, _ := DefaultBranch(dir)
	execIn(t, dir, "git", "branch", "wn-dry999-feat")
	execIn(t, dir, "git", "checkout", "wn-dry999-feat")
	writeFile(t, dir+"/dry.txt", "dry")
	execIn(t, dir, "git", "add", "dry.txt")
	execIn(t, dir, "git", "commit", "-m", "dry")
	execIn(t, dir, "git", "checkout", def)

	item := &Item{
		ID:          "dry999",
		Description: "MCP dry-run item",
		Created:     now,
		Updated:     now,
		ReviewReady: true,
		Tags:        []string{},
		DependsOn:   []string{},
		Log:         []LogEntry{{At: now, Kind: "created"}},
		Notes:       []Note{{Name: NoteNameBranch, Body: "wn-dry999-feat", Created: now}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wn_merge",
		Arguments: map[string]any{
			"branch":  "wn-dry999-feat",
			"message": "would merge",
			"dry_run": true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool wn_merge dry_run: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_merge dry_run returned error: %s", textContent(res))
	}

	got, err := store.Get("dry999")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if got.Done {
		t.Error("item should not be done after dry-run wn_merge")
	}
}

func TestMCP_wn_merge_missingMessage(t *testing.T) {
	ctx, cs, _, _, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	// Missing message: either the SDK schema validation returns an error,
	// or the handler returns IsError. Both are acceptable.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wn_merge",
		Arguments: map[string]any{
			"branch": "some-branch",
		},
	})
	if err == nil && !res.IsError {
		t.Error("wn_merge with no message should return error (got neither err nor IsError)")
	}
}

func TestMCP_wn_merge_noItem(t *testing.T) {
	ctx, cs, _, _, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wn_merge",
		Arguments: map[string]any{
			"branch":  "nonexistent-branch",
			"message": "test msg",
		},
	})
	if err != nil {
		t.Fatalf("CallTool wn_merge: %v", err)
	}
	if !res.IsError {
		t.Error("wn_merge with no associated item should return error")
	}
}

// --- helpers: errResult, withStore, withItem ---
