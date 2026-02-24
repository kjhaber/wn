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

// listItem is the JSON shape returned by wn_list for each item.
type listItem struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

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
	if items[0].Status != "undone" && items[0].Status != "review-ready" {
		t.Errorf("wn_list items[0].status = %q, want undone or review-ready", items[0].Status)
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
	defer clientSession.Close()

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
		clientSession.Close()
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
	var out map[string]string
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("wn_add must return valid JSON: %v\ncontent: %q", err, text)
	}
	if out["id"] == "" {
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

func TestMCP_wn_item(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// wn_item with id returns full item JSON (for subagents that only have an id)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_item", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_item: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_item IsError true: %s", textContent(res))
	}
	text := textContent(res)
	var out showOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("wn_item result not valid JSON: %v\ncontent: %s", err, text)
	}
	if out.ID != "abc123" {
		t.Errorf("wn_item id = %q, want abc123", out.ID)
	}
	if out.Description != "first line\nbody for prompt" {
		t.Errorf("wn_item description = %q", out.Description)
	}
	if out.Notes == nil {
		t.Error("wn_item notes missing (agents need notes)")
	}
	if out.Log == nil {
		t.Error("wn_item log missing")
	}

	// wn_item without id must not succeed (schema may require id, or handler returns error)
	res2, err2 := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_item", Arguments: map[string]any{}})
	if err2 == nil && !res2.IsError {
		t.Fatalf("wn_item without id: expected validation/handler error or IsError, got success: %s", textContent(res2))
	}
	if err2 == nil && res2.IsError {
		if msg := textContent(res2); !strings.Contains(msg, "id") {
			t.Errorf("wn_item without id: message should mention id, got %q", msg)
		}
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

// TestMCP_wn_undone_clears_review_ready verifies that done→undone clears ReviewReady,
// so the item returns to "undone" status (available for next/claim) rather than "review-ready".
func TestMCP_wn_undone_clears_review_ready(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()
	// release sets review-ready, then done, then undone
	_, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_release", Arguments: map[string]any{"id": "abc123"}})
	_, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_done", Arguments: map[string]any{"id": "abc123"}})
	_, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_undone", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("wn_undone: %v", err)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("wn_list: %v", err)
	}
	text := textContent(res)
	var items []listItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("wn_list result: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("wn_list want 1 item, got %d", len(items))
	}
	if items[0].Status != "undone" {
		t.Errorf("after release→done→undone, status = %q, want undone", items[0].Status)
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

// TestMCP_wn_claim_omitted_for_uses_default verifies that wn_claim with no "for" uses the default duration
// so agents can renew (extend) a claim without passing a duration.
func TestMCP_wn_claim_omitted_for_uses_default(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_claim",
		Arguments: map[string]any{"id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_claim (no for): %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_claim with omitted for should succeed: %s", textContent(res))
	}
	text := textContent(res)
	if !strings.Contains(text, "claimed abc123") {
		t.Errorf("wn_claim content = %q", text)
	}

	// Should have set in_progress_until to approximately now + DefaultClaimDuration
	res2, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var out struct {
		InProgressUntil string `json:"in_progress_until"`
	}
	if err := json.Unmarshal([]byte(textContent(res2)), &out); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if out.InProgressUntil == "" || out.InProgressUntil == "0001-01-01T00:00:00Z" {
		t.Errorf("wn_show: expected in_progress_until set after claim, got %q", out.InProgressUntil)
	}
	until, _ := time.Parse(time.RFC3339, out.InProgressUntil)
	now := time.Now().UTC()
	expectedMin := now.Add(DefaultClaimDuration - 2*time.Minute) // allow clock skew
	expectedMax := now.Add(DefaultClaimDuration + 2*time.Minute)
	if until.Before(expectedMin) || until.After(expectedMax) {
		t.Errorf("wn_claim default: in_progress_until %v expected near now+%v (between %v and %v)", until, DefaultClaimDuration, expectedMin, expectedMax)
	}
}

// TestMCP_wn_claim_renew_extends_claim verifies that calling wn_claim again on an already-claimed item
// (e.g. with omitted "for") renews the claim from now, so agents can extend without losing context.
func TestMCP_wn_claim_renew_extends_claim(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// First claim for 5m
	_, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_claim",
		Arguments: map[string]any{"id": "abc123", "for": "5m"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_claim: %v", err)
	}

	// Simulate time passing: renew with omitted "for" (default duration)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_claim",
		Arguments: map[string]any{"id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_claim renew: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_claim renew: %s", textContent(res))
	}

	// in_progress_until should now be ~now+DefaultClaimDuration (renewed from now), not the old 5m expiry
	res2, _ := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	var out struct {
		InProgressUntil string `json:"in_progress_until"`
	}
	_ = json.Unmarshal([]byte(textContent(res2)), &out)
	until, _ := time.Parse(time.RFC3339, out.InProgressUntil)
	now := time.Now().UTC()
	// Renewed claim should be ~now+DefaultClaimDuration (e.g. 1h), not ~now+5m
	minExpected := now.Add(30 * time.Minute)
	if until.Before(minExpected) {
		t.Errorf("wn_claim renew: in_progress_until %v should be at least ~30m from now (renewed with default), got %v", until, until.Sub(now))
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
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("wn_next must return valid JSON: %v\ncontent: %q", err, text)
	}
	if out["id"] != "abc123" {
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
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("wn_next empty must return valid JSON: %v\ncontent: %q", err, text)
	}
	if out["id"] != nil {
		t.Errorf("wn_next empty = %q, want id:null", text)
	}
}

func TestMCP_wn_next_with_claim_for(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_next",
		Arguments: map[string]any{"claim_for": "30m"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_next claim_for: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_next claim_for: %s", textContent(res))
	}
	text := textContent(res)
	var nextOut map[string]any
	if err := json.Unmarshal([]byte(text), &nextOut); err != nil {
		t.Fatalf("wn_next claim_for must return valid JSON: %v", err)
	}
	if nextOut["id"] != "abc123" || nextOut["claimed"] != true {
		t.Errorf("wn_next claim_for content = %q", text)
	}
	// Claimed item should not appear in wn_list (undone list excludes in-progress)
	res2, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool wn_list: %v", err)
	}
	listText := textContent(res2)
	var listItems []listItem
	if err := json.Unmarshal([]byte(listText), &listItems); err != nil {
		t.Fatalf("wn_list after claim must be valid JSON: %v", err)
	}
	for _, it := range listItems {
		if it.ID == "abc123" {
			t.Errorf("claimed item should not be in wn_list; got %q", listText)
			break
		}
	}
	// wn_show should show in_progress_until
	res3, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_show",
		Arguments: map[string]any{"id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var out struct {
		InProgressUntil string `json:"in_progress_until"`
	}
	if err := json.Unmarshal([]byte(textContent(res3)), &out); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if out.InProgressUntil == "" || out.InProgressUntil == "0001-01-01T00:00:00Z" {
		t.Errorf("wn_show: expected in_progress_until set after claim, got %q", out.InProgressUntil)
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
		clientSession.Close()
		_ = serverSession.Wait()
		_ = os.Chdir(cwd)
	}
	return ctx, clientSession, dir, cleanup
}

func TestMCP_wn_depend_and_wn_rmdepend(t *testing.T) {
	ctx, cs, _, cleanup := setupMCPSessionTwoItems(t, "aa1111", "bb2222")
	defer cleanup()

	// wn_depend: aa1111 depends on bb2222
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_depend",
		Arguments: map[string]any{"id": "aa1111", "on": "bb2222"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_depend: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_depend: %s", textContent(res))
	}
	text := textContent(res)
	if !strings.Contains(text, "aa1111") {
		t.Errorf("wn_depend content = %q", text)
	}

	// wn_show aa1111: depends_on should contain bb2222
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_show",
		Arguments: map[string]any{"id": "aa1111"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var out struct {
		DependsOn []string `json:"depends_on"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if len(out.DependsOn) != 1 || out.DependsOn[0] != "bb2222" {
		t.Errorf("wn_show after depend: depends_on = %v, want [bb2222]", out.DependsOn)
	}

	// wn_rmdepend: remove bb2222 from aa1111
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_rmdepend",
		Arguments: map[string]any{"id": "aa1111", "on": "bb2222"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_rmdepend: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_rmdepend: %s", textContent(res))
	}

	// wn_show aa1111: depends_on should be empty
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_show",
		Arguments: map[string]any{"id": "aa1111"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if len(out.DependsOn) != 0 {
		t.Errorf("wn_show after rmdepend: depends_on = %v, want []", out.DependsOn)
	}
}

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
