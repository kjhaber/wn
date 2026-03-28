package wn

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func TestMCP_wn_add_with_depends_on(t *testing.T) {
	ctx, cs, _, cleanup := setupMCPSessionTwoItems(t, "aa1111", "bb2222")
	defer cleanup()

	// Add follow-up item that depends on bb2222 so queue order is maintained
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wn_add",
		Arguments: map[string]any{
			"description": "follow-up task",
			"depends_on":  []string{"bb2222"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool wn_add: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_add with depends_on: %s", textContent(res))
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_add result JSON: %v", err)
	}
	newID := out["id"]
	if newID == "" {
		t.Fatal("wn_add did not return id")
	}

	// New item should have depends_on [bb2222]
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_show",
		Arguments: map[string]any{"id": newID},
	})
	if err != nil {
		t.Fatalf("CallTool wn_show: %v", err)
	}
	var show struct {
		DependsOn []string `json:"depends_on"`
	}
	if err := json.Unmarshal([]byte(textContent(res)), &show); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if len(show.DependsOn) != 1 || show.DependsOn[0] != "bb2222" {
		t.Errorf("wn_show after add with depends_on: depends_on = %v, want [bb2222]", show.DependsOn)
	}
}

func TestMCP_wn_add_with_depends_on_nonexistent_is_error(t *testing.T) {
	ctx, cs, _, cleanup := setupMCPSessionTwoItems(t, "aa1111", "bb2222")
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wn_add",
		Arguments: map[string]any{
			"description": "task",
			"depends_on":  []string{"nonexistent"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool wn_add: %v", err)
	}
	if !res.IsError {
		t.Error("wn_add with depends_on nonexistent id: want IsError true")
	}
	text := textContent(res)
	if text == "" || !strings.Contains(text, "nonexistent") {
		t.Errorf("wn_add error should mention missing id, got %q", text)
	}
}
