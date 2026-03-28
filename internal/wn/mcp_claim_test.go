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
	defer func() { _ = clientSession.Close() }()

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

// TestMCP_wn_next_with_tag verifies that wn_next with tag returns/sets current to the next undone item that has that tag (dependency order).

func TestMCP_wn_next_with_tag(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i, id := range []string{"aa1111", "bb2222"} {
		ord := i
		tags := []string{}
		if id == "bb2222" {
			tags = []string{"agent"}
		}
		item := &Item{
			ID:          id,
			Description: "item " + id,
			Created:     now,
			Updated:     now,
			Order:       &ord,
			Tags:        tags,
			Log:         []LogEntry{{At: now, Kind: "created"}},
		}
		if err := store.Put(item); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}
	if err := WriteMeta(dir, Meta{CurrentID: "aa1111"}); err != nil {
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

	// wn_next with tag "agent" should return bb2222 (only undone item with that tag)
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_next",
		Arguments: map[string]any{"tag": "agent"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_next tag=agent: %v", err)
	}
	text := textContent(res)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("wn_next tag=agent must return valid JSON: %v\ncontent: %q", err, text)
	}
	if out["id"] != "bb2222" {
		t.Errorf("wn_next tag=agent = %q, want id bb2222", text)
	}
	meta, _ := ReadMeta(dir)
	if meta.CurrentID != "bb2222" {
		t.Errorf("after wn_next tag=agent: CurrentID = %q, want bb2222", meta.CurrentID)
	}

	// wn_next with tag "nonexistent" should return id:null
	res2, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_next",
		Arguments: map[string]any{"tag": "nonexistent"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_next tag=nonexistent: %v", err)
	}
	text2 := textContent(res2)
	var out2 map[string]any
	if err := json.Unmarshal([]byte(text2), &out2); err != nil {
		t.Fatalf("wn_next tag=nonexistent must return valid JSON: %v", err)
	}
	if out2["id"] != nil {
		t.Errorf("wn_next tag=nonexistent = %q, want id:null", text2)
	}
}
