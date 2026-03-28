package wn

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
