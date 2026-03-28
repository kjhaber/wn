package wn

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_wn_list_returns_structured_json(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list result must be valid JSON: %v\ncontent: %s", err, text)
	}
	if len(items) != 1 {
		t.Fatalf("wn_list want 1 item, got %d", len(items))
	}
	if items[0].ID != "abc123" {
		t.Errorf("wn_list items[0].id = %q, want abc123", items[0].ID)
	}
	if items[0].Description != "first line" {
		t.Errorf("wn_list items[0].description = %q, want first line", items[0].Description)
	}
	if items[0].Tags == nil {
		t.Error("wn_list items[0].tags must be present (array)")
	}
	if items[0].Status != "undone" {
		t.Errorf("wn_list items[0].status = %q, want undone", items[0].Status)
	}
}

func TestMCP_wn_list(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list must return valid JSON: %v\ncontent: %q", err, text)
	}
	if len(items) < 1 || items[0].ID != "abc123" || items[0].Description != "first line" {
		t.Errorf("wn_list content = %q", text)
	}
}

func TestMCP_wn_list_empty(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, _ := server.Connect(ctx, serverTransport, nil)
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, _ := client.Connect(ctx, clientTransport, nil)
	defer func() { _ = clientSession.Close() }()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list empty must return valid JSON: %v\ncontent: %q", err, text)
	}
	if len(items) != 0 {
		t.Errorf("wn_list empty = %d items, want 0", len(items))
	}
}

func TestMCP_wn_list_includes_review_ready(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, item := range []*Item{
		{ID: "u1", Description: "undone", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}},
		{ID: "rr1", Description: "review-ready", Created: now, Updated: now, ReviewReady: true, Log: []LogEntry{{At: now, Kind: "created"}}},
	} {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, _ := server.Connect(ctx, serverTransport, nil)
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, _ := client.Connect(ctx, clientTransport, nil)
	defer func() { _ = clientSession.Close() }()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list must return valid JSON: %v\ncontent: %q", err, text)
	}
	if len(items) != 2 {
		t.Fatalf("wn_list want 2 items (undone + review-ready), got %d", len(items))
	}
	byID := make(map[string]listItem)
	for _, it := range items {
		byID[it.ID] = it
	}
	if byID["u1"].Status != "undone" {
		t.Errorf("wn_list u1 status = %q, want undone", byID["u1"].Status)
	}
	if byID["rr1"].Status != "review" {
		t.Errorf("wn_list rr1 status = %q, want review", byID["rr1"].Status)
	}
}

// setupMCPSessionThreeItems creates a temp wn root with three items (aaa, bbb, ccc) in dependency order (Order 0,1,2).
func setupMCPSessionThreeItems(t *testing.T) (context.Context, *mcp.ClientSession, func()) {
	t.Helper()
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	for i, id := range []string{"aaa", "bbb", "ccc"} {
		ord := i
		item := &Item{
			ID:          id,
			Description: "item " + id,
			Created:     now,
			Updated:     now,
			Order:       &ord,
			Log:         []LogEntry{{At: now, Kind: "created"}},
		}
		if err := store.Put(item); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}
	if err := WriteMeta(dir, Meta{CurrentID: "aaa"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
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
	return ctx, clientSession, cleanup
}

func TestMCP_wn_list_limit(t *testing.T) {
	ctx, cs, cleanup := setupMCPSessionThreeItems(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_list",
		Arguments: map[string]any{"limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list limit 2 must return valid JSON: %v\ncontent: %s", err, text)
	}
	if len(items) != 2 {
		t.Fatalf("wn_list limit 2: got %d items, want 2", len(items))
	}
	if items[0].ID != "aaa" || items[1].ID != "bbb" {
		t.Errorf("wn_list limit 2 = %v, %v; want aaa, bbb", items[0].ID, items[1].ID)
	}
}

func TestMCP_wn_list_limit_offset(t *testing.T) {
	ctx, cs, cleanup := setupMCPSessionThreeItems(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_list",
		Arguments: map[string]any{"limit": 1, "offset": 1},
	})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list limit 1 offset 1 must return valid JSON: %v\ncontent: %s", err, text)
	}
	if len(items) != 1 {
		t.Fatalf("wn_list limit 1 offset 1: got %d items, want 1", len(items))
	}
	if items[0].ID != "bbb" {
		t.Errorf("wn_list limit 1 offset 1 = %v; want bbb", items[0].ID)
	}
}

func TestMCP_wn_list_cursor(t *testing.T) {
	ctx, cs, cleanup := setupMCPSessionThreeItems(t)
	defer cleanup()

	// Start after aaa: should return bbb, ccc; with limit 1 just bbb
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_list",
		Arguments: map[string]any{"cursor": "aaa", "limit": 1},
	})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list cursor aaa limit 1: %v\ncontent: %s", err, text)
	}
	if len(items) != 1 {
		t.Fatalf("wn_list cursor aaa limit 1: got %d items, want 1", len(items))
	}
	if items[0].ID != "bbb" {
		t.Errorf("wn_list cursor aaa limit 1 = %v; want bbb", items[0].ID)
	}
}

func TestMCP_wn_list_with_root(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "x1y2z3",
		Description: "item via root param",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Do not chdir; stay in current (possibly non-wn) directory.
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() { _ = clientSession.Close() }()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_list",
		Arguments: map[string]any{"root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_list with root: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("wn_list with root: %s", textContent(res))
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list with root must return valid JSON: %v\ncontent: %q", err, text)
	}
	if len(items) != 1 || items[0].ID != "x1y2z3" || items[0].Description != "item via root param" {
		t.Errorf("wn_list with root: expected one item x1y2z3, got %q", text)
	}
}

// TestMCP_fixed_root_guardrail verifies that when SetMCPFixedRoot is set, the server
// uses that path and ignores the "root" parameter in requests.

func TestMCP_wn_list_GitWorktree(t *testing.T) {
	// Set up a main repo with .wn and a linked worktree.
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)
	if err := InitRoot(mainRepo); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(mainRepo)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "wt1234",
		Description: "worktree item",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	worktreeDir := t.TempDir()
	execIn(t, mainRepo, "git", "worktree", "add", worktreeDir, "-b", "wn-mcp-worktree-test")

	origWd, _ := os.Getwd()
	origEnv := os.Getenv("WN_ROOT")
	_ = os.Unsetenv("WN_ROOT")
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		if origEnv == "" {
			_ = os.Unsetenv("WN_ROOT")
		} else {
			_ = os.Setenv("WN_ROOT", origEnv)
		}
	})

	// Change to the worktree directory (no .wn here) before starting the MCP server.
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}

	// Reset mcpFixedRoot so the server uses cwd-based detection.
	oldFixed := mcpFixedRoot
	mcpFixedRoot = ""
	t.Cleanup(func() { mcpFixedRoot = oldFixed })

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	}()

	// wn_list without root param should auto-detect via git worktree.
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_list from worktree: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_list from worktree returned error: %s", textContent(res))
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list result must be valid JSON: %v\ncontent: %s", err, text)
	}
	if len(items) != 1 || items[0].ID != "wt1234" {
		t.Errorf("wn_list from worktree: want [{id: wt1234}], got %s", text)
	}
}
