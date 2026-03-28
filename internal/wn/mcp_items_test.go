package wn

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
