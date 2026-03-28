package wn

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_wn_duplicate(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// Add a second item to use as the duplicate
	addRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_add",
		Arguments: map[string]any{"description": "duplicate task"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_add: %v", err)
	}
	if addRes.IsError {
		t.Fatalf("wn_add: %s", textContent(addRes))
	}
	var addOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(textContent(addRes)), &addOut); err != nil {
		t.Fatalf("wn_add result: %v", err)
	}
	dupID := addOut.ID
	if dupID == "" {
		t.Fatal("wn_add did not return id")
	}

	// Mark the new item as duplicate of abc123
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_duplicate",
		Arguments: map[string]any{"id": dupID, "on": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_duplicate: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_duplicate: %s", textContent(res))
	}
	text := textContent(res)
	if !strings.Contains(text, "duplicate of abc123") {
		t.Errorf("wn_duplicate content = %q", text)
	}

	// Verify: item is done and has note duplicate-of with body abc123
	showRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": dupID}})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var show struct {
		Done  bool `json:"done"`
		Notes []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(showRes)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if !show.Done {
		t.Error("item should be done after wn_duplicate")
	}
	var found bool
	for _, n := range show.Notes {
		if n.Name == NoteNameDuplicateOf && n.Body == "abc123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("notes should contain duplicate-of: abc123, got %v", show.Notes)
	}
}
