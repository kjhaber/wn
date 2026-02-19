package wn

import (
	"context"
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
