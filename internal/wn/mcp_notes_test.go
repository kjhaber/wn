package wn

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_wn_note_add_edit_rm(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// wn_note_add: add a note on current item (abc123)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_add",
		Arguments: map[string]any{"id": "abc123", "name": "pr-url", "body": "https://github.com/org/repo/pull/1"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_add: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_add: %s", textContent(res))
	}

	// wn_show: notes should contain pr-url with that body
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var show struct {
		Notes []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if len(show.Notes) != 1 || show.Notes[0].Name != "pr-url" || show.Notes[0].Body != "https://github.com/org/repo/pull/1" {
		t.Errorf("after note_add: notes = %v", show.Notes)
	}

	// wn_note_edit: update the note body
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_edit",
		Arguments: map[string]any{"id": "abc123", "name": "pr-url", "body": "updated url"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_edit: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_edit: %s", textContent(res))
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	if err := json.Unmarshal([]byte(textContent(res)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if len(show.Notes) != 1 || show.Notes[0].Body != "updated url" {
		t.Errorf("after note_edit: notes = %v", show.Notes)
	}

	// wn_note_rm: remove the note
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_rm",
		Arguments: map[string]any{"id": "abc123", "name": "pr-url"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_rm: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_rm: %s", textContent(res))
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	if err := json.Unmarshal([]byte(textContent(res)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if len(show.Notes) != 0 {
		t.Errorf("after note_rm: notes = %v, want []", show.Notes)
	}
}

func TestMCP_wn_note_search_ByName(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// Add a note on the existing item abc123
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_add",
		Arguments: map[string]any{"id": "abc123", "name": "pr-url", "body": "https://example.com/1"},
	})
	if err != nil || res.IsError {
		t.Fatalf("wn_note_add: err=%v text=%s", err, textContent(res))
	}

	// Search by name — should return abc123
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_search",
		Arguments: map[string]any{"name": "pr-url"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_search: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_search: %s", textContent(res))
	}
	var items []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &items); err != nil {
		t.Fatalf("wn_note_search JSON: %v\nraw: %s", err, textContent(res))
	}
	if len(items) != 1 || items[0].ID != "abc123" {
		t.Errorf("wn_note_search by name: want [{abc123 ...}], got %v", items)
	}
}

func TestMCP_wn_note_search_ByNameAndValue(t *testing.T) {
	ctx, cs, dir, cleanup := setupMCPSessionTwoItems(t, "item1", "item2")
	defer cleanup()

	// Add same note name with different values to both items
	for _, tc := range []struct{ id, val string }{{"item1", "branch-a"}, {"item2", "branch-b"}} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name:      "wn_note_add",
			Arguments: map[string]any{"id": tc.id, "name": "branch", "body": tc.val, "root": dir},
		})
		if err != nil || res.IsError {
			t.Fatalf("wn_note_add %s: err=%v text=%s", tc.id, err, textContent(res))
		}
	}

	// Search by name+value — should return only item1
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_search",
		Arguments: map[string]any{"name": "branch", "value": "branch-a", "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_search: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_search: %s", textContent(res))
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &items); err != nil {
		t.Fatalf("wn_note_search JSON: %v", err)
	}
	if len(items) != 1 || items[0].ID != "item1" {
		t.Errorf("wn_note_search by name+value: want [{item1}], got %v", items)
	}
}

func TestMCP_wn_note_search_NoMatches(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_search",
		Arguments: map[string]any{"name": "nonexistent"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_search: %v", err)
	}
	if !res.IsError {
		t.Fatalf("wn_note_search with no matches should return IsError; got %s", textContent(res))
	}
}

func TestMCP_wn_note_search_First(t *testing.T) {
	ctx, cs, dir, cleanup := setupMCPSessionTwoItems(t, "older1", "newer2")
	defer cleanup()

	// Set created times differently by writing items directly
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC()
	for _, tc := range []struct {
		id string
		ts time.Time
	}{{"older1", t1}, {"newer2", t2}} {
		if err := store.UpdateItem(tc.id, func(it *Item) (*Item, error) {
			it.Created = tc.ts
			it.Updated = tc.ts
			it.Notes = []Note{{Name: "shared", Created: tc.ts, Body: "val"}}
			return it, nil
		}); err != nil {
			t.Fatalf("UpdateItem %s: %v", tc.id, err)
		}
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_search",
		Arguments: map[string]any{"name": "shared", "first": true, "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_search --first: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_search --first: %s", textContent(res))
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &items); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(items) != 1 || items[0].ID != "older1" {
		t.Errorf("--first: want [{older1}], got %v", items)
	}
}

func TestMCP_wn_note_search_Latest(t *testing.T) {
	ctx, cs, dir, cleanup := setupMCPSessionTwoItems(t, "older1", "newer2")
	defer cleanup()

	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC()
	for _, tc := range []struct {
		id string
		ts time.Time
	}{{"older1", t1}, {"newer2", t2}} {
		if err := store.UpdateItem(tc.id, func(it *Item) (*Item, error) {
			it.Created = tc.ts
			it.Updated = tc.ts
			it.Notes = []Note{{Name: "shared", Created: tc.ts, Body: "val"}}
			return it, nil
		}); err != nil {
			t.Fatalf("UpdateItem %s: %v", tc.id, err)
		}
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_search",
		Arguments: map[string]any{"name": "shared", "latest": true, "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_search --latest: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_search --latest: %s", textContent(res))
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &items); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(items) != 1 || items[0].ID != "newer2" {
		t.Errorf("--latest: want [{newer2}], got %v", items)
	}
}

func TestMCP_wn_note_add_empty_body(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// Empty body should be allowed and stored as empty string
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_add",
		Arguments: map[string]any{"id": "abc123", "name": "my-tag"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_add: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_add with empty body should succeed; got: %s", textContent(res))
	}

	showRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var show struct {
		Notes []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(showRes)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	found := false
	for _, n := range show.Notes {
		if n.Name == "my-tag" && n.Body == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("empty-body note not found; notes = %v", show.Notes)
	}
}

func TestMCP_wn_note_add_unknown_special_name(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_add",
		Arguments: map[string]any{"id": "abc123", "name": "wn:unknown", "body": "value"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_add: %v", err)
	}
	if !res.IsError {
		t.Fatal("wn_note_add with unknown wn: name should return error")
	}
	if !strings.Contains(textContent(res), "wn:unknown") {
		t.Errorf("error should mention the unknown name; got: %s", textContent(res))
	}
}

func TestMCP_wn_note_add_wn_branch(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_note_add",
		Arguments: map[string]any{"id": "abc123", "name": "wn:branch", "body": "feat/my-branch"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_note_add: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_note_add wn:branch: %s", textContent(res))
	}

	showRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var show struct {
		Notes []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(showRes)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	found := false
	for _, n := range show.Notes {
		if n.Name == NoteNameBranch && n.Body == "feat/my-branch" {
			found = true
		}
	}
	if !found {
		t.Errorf("wn:branch note not found after add; notes = %v", show.Notes)
	}
}

// setupMCPSessionGit creates a temp dir with both a git repo and a wn store,
// chdirs into it, and connects an MCP client to a wn server over in-memory
// transport. Returns client session and cleanup.
func setupMCPSessionGit(t *testing.T) (context.Context, *mcp.ClientSession, Store, string, func()) {
	t.Helper()
	dir := t.TempDir()
	setupGitRepo(t, dir)
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		_ = os.Chdir(cwd)
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = os.Chdir(cwd)
		t.Fatalf("client.Connect: %v", err)
	}
	cleanup := func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
		_ = os.Chdir(cwd)
	}
	return ctx, clientSession, store, dir, cleanup
}
