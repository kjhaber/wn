package wn

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_wn_prompt_createsItemAndDep(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// wn_prompt with parent_id abc123 and a question
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_prompt",
		Arguments: map[string]any{"parent_id": "abc123", "question": "Should we use postgres or sqlite?"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_prompt: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_prompt error: %s", textContent(res))
	}

	// Response should include the new prompt item id
	var out map[string]string
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_prompt result not JSON: %v\ncontent: %s", err, textContent(res))
	}
	promptID := out["id"]
	if promptID == "" {
		t.Fatal("wn_prompt: expected id in response")
	}

	// Verify prompt item exists and is prompt-ready
	showRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": promptID}})
	if err != nil {
		t.Fatalf("wn_show prompt item: %v", err)
	}
	var promptItem struct {
		PromptReady bool   `json:"prompt_ready"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(textContent(showRes)), &promptItem); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if !promptItem.PromptReady {
		t.Error("prompt item should have prompt_ready=true")
	}
	if promptItem.Description != "Should we use postgres or sqlite?" {
		t.Errorf("prompt item description = %q", promptItem.Description)
	}

	// Verify parent depends on prompt item
	parentRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": "abc123"}})
	if err != nil {
		t.Fatalf("wn_show parent: %v", err)
	}
	var parent struct {
		DependsOn []string `json:"depends_on"`
	}
	if err := json.Unmarshal([]byte(textContent(parentRes)), &parent); err != nil {
		t.Fatalf("wn_show parent JSON: %v", err)
	}
	var depFound bool
	for _, d := range parent.DependsOn {
		if d == promptID {
			depFound = true
			break
		}
	}
	if !depFound {
		t.Errorf("parent abc123 should depend on prompt item %s, DependsOn=%v", promptID, parent.DependsOn)
	}
}

func TestMCP_wn_prompt_fallsBackToCurrentItem(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// No parent_id: should use current task (abc123)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_prompt",
		Arguments: map[string]any{"question": "Is this the right approach?"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_prompt: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_prompt error: %s", textContent(res))
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_prompt result not JSON: %v", err)
	}
	if out["id"] == "" {
		t.Error("expected id in response")
	}
}

func TestMCP_wn_prompt_requiresQuestion(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_prompt",
		Arguments: map[string]any{"parent_id": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_prompt: %v", err)
	}
	if !res.IsError {
		t.Error("expected error when question is empty")
	}
}

func TestMCP_wn_respond_marksPromptDone(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	// Create a prompt item first
	promptRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_prompt",
		Arguments: map[string]any{"parent_id": "abc123", "question": "Which approach?"},
	})
	if err != nil || promptRes.IsError {
		t.Fatalf("wn_prompt setup: %v / %s", err, textContent(promptRes))
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(textContent(promptRes)), &out)
	promptID := out["id"]

	// Respond to it
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_respond",
		Arguments: map[string]any{"id": promptID, "answer": "Use approach B."},
	})
	if err != nil {
		t.Fatalf("CallTool wn_respond: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_respond error: %s", textContent(res))
	}

	// Verify prompt item is done and has response note
	showRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wn_show", Arguments: map[string]any{"id": promptID}})
	if err != nil {
		t.Fatalf("wn_show: %v", err)
	}
	var item struct {
		Done  bool `json:"done"`
		Notes []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(showRes)), &item); err != nil {
		t.Fatalf("wn_show JSON: %v", err)
	}
	if !item.Done {
		t.Error("prompt item should be done after wn_respond")
	}
	var noteFound bool
	for _, n := range item.Notes {
		if n.Name == NoteNameResponse && n.Body == "Use approach B." {
			noteFound = true
			break
		}
	}
	if !noteFound {
		t.Errorf("response note not found, notes=%v", item.Notes)
	}
}

func TestMCP_wn_respond_rejectsNonPromptItem(t *testing.T) {
	ctx, cs, cleanup := setupMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_respond",
		Arguments: map[string]any{"id": "abc123", "answer": "This should fail."},
	})
	if err != nil {
		t.Fatalf("CallTool wn_respond: %v", err)
	}
	if !res.IsError {
		t.Error("expected error when responding to non-prompt item")
	}
}

// TestMCP_wn_list_GitWorktree verifies that MCP tools auto-detect the correct root
// when the server process cwd is a linked git worktree (no .wn there; .wn is in main repo).
