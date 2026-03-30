package wn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	defer func() { _ = clientSession.Close() }()

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
	defer func() { _ = clientSession.Close() }()

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
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list fixed root must return valid JSON: %v", err)
	}
	var foundA bool
	for _, it := range items {
		if it.ID == "bbbbbb" {
			t.Errorf("fixed root guardrail: should not see B's item when fixed to A, got %q", text)
			break
		}
		if it.ID == "aaaaaa" && it.Description == "only in A" {
			foundA = true
		}
	}
	if !foundA {
		t.Errorf("fixed root guardrail: expected A's item (aaaaaa, only in A), got %q", text)
	}
}

func TestMCP_wn_root_from_cwd(t *testing.T) {
	ctx, cs, _, dir, cleanup := setupMCPSessionGit(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_root",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_root unexpected error: %s", textContent(res))
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(textContent(res)), &got); err != nil {
		t.Fatalf("wn_root response not valid JSON: %v, raw: %q", err, textContent(res))
	}
	normGot, _ := filepath.EvalSymlinks(got["root"])
	normDir, _ := filepath.EvalSymlinks(dir)
	if normGot != normDir {
		t.Errorf("wn_root from cwd: got %q (norm %q), want %q (norm %q)", got["root"], normGot, dir, normDir)
	}
}

func TestMCP_wn_root_with_explicit_root(t *testing.T) {
	gitDir := t.TempDir()
	setupGitRepo(t, gitDir)

	// Use a plain dir as cwd (no git, no wn); wn_root uses the explicit root param.
	plainDir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(plainDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, _ := server.Connect(ctx, serverTransport, nil)
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, _ := client.Connect(ctx, clientTransport, nil)
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_root",
		Arguments: map[string]any{"root": gitDir},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_root explicit root unexpected error: %s", textContent(res))
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(textContent(res)), &got); err != nil {
		t.Fatalf("wn_root explicit root response not valid JSON: %v", err)
	}
	normGot, _ := filepath.EvalSymlinks(got["root"])
	normDir, _ := filepath.EvalSymlinks(gitDir)
	if normGot != normDir {
		t.Errorf("wn_root explicit root: got %q (norm %q), want %q (norm %q)", got["root"], normGot, gitDir, normDir)
	}
}

func TestMCP_wn_root_non_git_returns_error(t *testing.T) {
	nonGitDir := t.TempDir()

	plainDir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(plainDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, _ := server.Connect(ctx, serverTransport, nil)
	defer func() { _ = serverSession.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, _ := client.Connect(ctx, clientTransport, nil)
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_root",
		Arguments: map[string]any{"root": nonGitDir},
	})
	if err != nil {
		return // protocol error acceptable
	}
	if res == nil || !res.IsError {
		t.Error("wn_root in non-git dir: expected IsError result")
	}
}

// setupMCPSessionTwoItems creates a temp wn root with two items (id1, id2), current=id1.
func setupMCPSessionTwoItems(t *testing.T, id1, id2 string) (context.Context, *mcp.ClientSession, string, func()) {
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
	for _, id := range []string{id1, id2} {
		item := &Item{
			ID:          id,
			Description: "item " + id,
			Created:     now,
			Updated:     now,
			Log:         []LogEntry{{At: now, Kind: "created"}},
		}
		if err := store.Put(item); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}
	if err := WriteMeta(dir, Meta{CurrentID: id1}); err != nil {
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
	return ctx, clientSession, dir, cleanup
}
