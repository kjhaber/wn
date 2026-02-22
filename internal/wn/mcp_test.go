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

// setupMCPSession creates a temp wn root with one item, chdirs into it, and connects
// an MCP client to a wn server over in-memory transport. Returns client session and cleanup.
func setupMCPSession(t *testing.T) (context.Context, *mcp.ClientSession, func()) {
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
	item := &Item{
		ID:          "abc123",
		Description: "first line\nbody for prompt",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := WriteMeta(dir, Meta{CurrentID: "abc123"}); err != nil {
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
		clientSession.Close()
		_ = serverSession.Wait()
		_ = os.Chdir(cwd)
	}
	return ctx, clientSession, cleanup
}

func textContent(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func TestMCP_wn_list(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	text := textContent(res)
	if !strings.Contains(text, "abc123") || !strings.Contains(text, "first line") {
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
	defer clientSession.Close()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := textContent(res)
	if text != "No undone tasks.\n" {
		t.Errorf("wn_list empty = %q, want No undone tasks.\\n", text)
	}
}

func TestMCP_wn_add(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_add",
		Arguments: map[string]any{"description": "new task"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_add: %v", err)
	}
	text := textContent(res)
	if !strings.HasPrefix(text, "added ") {
		t.Errorf("wn_add content = %q", text)
	}
	if res.IsError {
		t.Error("wn_add IsError = true")
	}
}

func TestMCP_wn_add_empty_description_is_error(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_add",
		Arguments: map[string]any{"description": ""},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("wn_add empty description: want IsError true")
	}
	text := textContent(res)
	if !strings.Contains(text, "description") {
		t.Errorf("wn_add error content = %q", text)
	}
}

func TestMCP_wn_desc(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_desc", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_desc: %v", err)
	}
	text := textContent(res)
	// PromptBody("first line\nbody for prompt") = "body for prompt"
	if text != "body for prompt" {
		t.Errorf("wn_desc = %q, want body for prompt", text)
	}
}

func TestMCP_wn_show(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_show IsError true: %s", textContent(res))
	}
	text := textContent(res)
	var item Item
	if err := json.Unmarshal([]byte(text), &item); err != nil {
		t.Fatalf("wn_show result not valid JSON: %v\ncontent: %s", err, text)
	}
	if item.ID != "abc123" {
		t.Errorf("wn_show item.id = %q, want abc123", item.ID)
	}
	if item.Description != "first line\nbody for prompt" {
		t.Errorf("wn_show item.description = %q", item.Description)
	}
	if item.Tags == nil {
		t.Error("wn_show item.tags missing (agents need tags)")
	}
	if item.Log == nil {
		t.Error("wn_show item.log missing (agents need log)")
	}
	if item.Notes == nil {
		t.Error("wn_show item.notes missing (agents need notes)")
	}
	if item.DependsOn == nil {
		t.Error("wn_show item.depends_on missing (agents need depends_on)")
	}
}

func TestMCP_wn_done(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_done",
		Arguments: map[string]any{"id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_done: %v", err)
	}
	text := textContent(res)
	if !strings.Contains(text, "marked abc123 done") {
		t.Errorf("wn_done content = %q", text)
	}
}

func TestMCP_wn_undone(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()
	// mark done first
	_, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_done", Arguments: map[string]any{"id": "abc123"}})

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_undone",
		Arguments: map[string]any{"id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_undone: %v", err)
	}
	text := textContent(res)
	if !strings.Contains(text, "marked abc123 undone") {
		t.Errorf("wn_undone content = %q", text)
	}
}

func TestMCP_wn_claim_and_release(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_claim",
		Arguments: map[string]any{"id": "abc123", "for": "30m"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_claim: %v", err)
	}
	text := textContent(res)
	if !strings.Contains(text, "claimed abc123") {
		t.Errorf("wn_claim content = %q", text)
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_release",
		Arguments: map[string]any{"id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_release: %v", err)
	}
	text = textContent(res)
	if !strings.Contains(text, "released abc123") {
		t.Errorf("wn_release content = %q", text)
	}
}

func TestMCP_wn_claim_invalid_duration(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_claim",
		Arguments: map[string]any{"id": "abc123", "for": "invalid"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("wn_claim invalid duration: want IsError true")
	}
}

func TestMCP_wn_next(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_next", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_next: %v", err)
	}
	text := textContent(res)
	if !strings.HasPrefix(text, "abc123:") {
		t.Errorf("wn_next content = %q", text)
	}
}

func TestMCP_wn_next_empty(t *testing.T) {
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
	defer clientSession.Close()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "wn_next", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := textContent(res)
	if text != "No next task." {
		t.Errorf("wn_next empty = %q, want No next task.", text)
	}
}

func TestMCP_no_wn_root_returns_error(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	// No .wn in dir

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, _ := server.Connect(ctx, serverTransport, nil)
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, _ := client.Connect(ctx, clientTransport, nil)
	defer clientSession.Close()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		return // protocol error is acceptable
	}
	// SDK may pack handler error into result instead of returning err
	if res != nil && res.IsError {
		text := textContent(res)
		if !strings.Contains(text, "root") && !strings.Contains(text, "not found") {
			t.Errorf("wn_list no root: expected error message about root, got %q", text)
		}
		return
	}
	t.Error("wn_list with no wn root: want error or IsError result")
}

// TestMCP_wn_list_with_root verifies that passing "root" to a tool uses that path
// instead of process cwd. We create a wn root in a temp dir but do not chdir there;
// we pass the path in the tool call.
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
	defer clientSession.Close()

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
	if !strings.Contains(text, "x1y2z3") || !strings.Contains(text, "item via root param") {
		t.Errorf("wn_list with root: expected item in output, got %q", text)
	}
}

// TestMCP_fixed_root_guardrail verifies that when SetMCPFixedRoot is set, the server
// uses that path and ignores the "root" parameter in requests.
func TestMCP_fixed_root_guardrail(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := InitRoot(dirA); err != nil {
		t.Fatalf("InitRoot A: %v", err)
	}
	if err := InitRoot(dirB); err != nil {
		t.Fatalf("InitRoot B: %v", err)
	}
	storeA, _ := NewFileStore(dirA)
	storeB, _ := NewFileStore(dirB)
	now := time.Now().UTC()
	itemA := &Item{ID: "aaaaaa", Description: "only in A", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	itemB := &Item{ID: "bbbbbb", Description: "only in B", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}
	if err := storeA.Put(itemA); err != nil {
		t.Fatal(err)
	}
	if err := storeB.Put(itemB); err != nil {
		t.Fatal(err)
	}

	SetMCPFixedRoot(dirA)
	defer SetMCPFixedRoot("")

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, _ := server.Connect(ctx, serverTransport, nil)
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, _ := client.Connect(ctx, clientTransport, nil)
	defer clientSession.Close()

	// Request root=dirB but fixed root is dirA: should see A's item, not B's.
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_list",
		Arguments: map[string]any{"root": dirB},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("wn_list: %s", textContent(res))
	}
	text := textContent(res)
	if !strings.Contains(text, "aaaaaa") || !strings.Contains(text, "only in A") {
		t.Errorf("fixed root guardrail: expected A's item, got %q", text)
	}
	if strings.Contains(text, "bbbbbb") {
		t.Errorf("fixed root guardrail: should not see B's item when fixed to A, got %q", text)
	}
}
