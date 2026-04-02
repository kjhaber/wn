package wn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newSettingsMCPSession(t *testing.T) (context.Context, *mcp.ClientSession, func()) {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := NewMCPServer()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	return ctx, clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	}
}

// initSettingsTestRoot creates a temp wn root, blanks user settings, and optionally writes project settings.
func initSettingsTestRoot(t *testing.T, projectSettings string) string {
	t.Helper()
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	if projectSettings != "" {
		if err := os.WriteFile(filepath.Join(dir, ".wn", "settings.json"), []byte(projectSettings), 0644); err != nil {
			t.Fatalf("WriteFile settings: %v", err)
		}
	}
	cfgDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", cfgDir)
	t.Setenv("WN_SETTINGS_USER", "")
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")
	return dir
}

func TestMCP_wn_settings_get_sort(t *testing.T) {
	dir := initSettingsTestRoot(t, `{"sort":"updated:desc"}`)
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_get",
		Arguments: map[string]any{"key": "sort"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	if textContent(res) != "updated:desc" {
		t.Errorf("got %q, want %q", textContent(res), "updated:desc")
	}
}

func TestMCP_wn_settings_get_nested(t *testing.T) {
	dir := initSettingsTestRoot(t, `{"runners":{"myrunner":{"cmd":"my-cmd"}}}`)
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_get",
		Arguments: map[string]any{"key": "runners.myrunner.cmd"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	if textContent(res) != "my-cmd" {
		t.Errorf("got %q, want %q", textContent(res), "my-cmd")
	}
}

func TestMCP_wn_settings_get_withExplicitRoot(t *testing.T) {
	dir := initSettingsTestRoot(t, `{"sort":"priority"}`)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_get",
		Arguments: map[string]any{"key": "sort", "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	if textContent(res) != "priority" {
		t.Errorf("got %q, want %q", textContent(res), "priority")
	}
}

func TestMCP_wn_settings_get_missingKey(t *testing.T) {
	dir := initSettingsTestRoot(t, "")
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_get",
		Arguments: map[string]any{"key": "nonexistent.key"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError for missing key, got: %s", textContent(res))
	}
}

func TestMCP_wn_settings_set_projectScope(t *testing.T) {
	dir := initSettingsTestRoot(t, "")
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "sort", "value": "priority", "scope": "project"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}

	data, err := os.ReadFile(filepath.Join(dir, ".wn", "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["sort"] != "priority" {
		t.Errorf("sort = %v, want %q", m["sort"], "priority")
	}
}

func TestMCP_wn_settings_set_projectLocalScope(t *testing.T) {
	dir := initSettingsTestRoot(t, "")
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "runners.myrunner.cmd", "value": "my-cmd", "scope": "project-local"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}

	data, err := os.ReadFile(filepath.Join(dir, ".wn", "settings.local.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	runners, _ := m["runners"].(map[string]any)
	runner, _ := runners["myrunner"].(map[string]any)
	if runner["cmd"] != "my-cmd" {
		t.Errorf("runners.myrunner.cmd = %v, want %q", runner["cmd"], "my-cmd")
	}
}

func TestMCP_wn_settings_set_userScope(t *testing.T) {
	dir := initSettingsTestRoot(t, "")
	userPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("WN_SETTINGS_USER", userPath)
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "picker", "value": "numbered", "scope": "user"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}

	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["picker"] != "numbered" {
		t.Errorf("picker = %v, want %q", m["picker"], "numbered")
	}
}

func TestMCP_wn_settings_set_boolValue(t *testing.T) {
	// Tests that setting a runner cmd value via MCP works correctly.
	dir := initSettingsTestRoot(t, "")
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "runners.myrunner.cmd", "value": "echo hello", "scope": "project"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}

	s, err := readSettingsFromPath(filepath.Join(dir, ".wn", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := s.Runners["myrunner"]
	if !ok {
		t.Fatal("runners.myrunner not found")
	}
	if r.Cmd != "echo hello" {
		t.Errorf("cmd = %q, want echo hello", r.Cmd)
	}
}

func TestMCP_wn_settings_set_invalidScope(t *testing.T) {
	dir := initSettingsTestRoot(t, "")
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "sort", "value": "priority", "scope": "invalid-scope"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError for invalid scope, got: %s", textContent(res))
	}
	if !strings.Contains(textContent(res), "scope") {
		t.Errorf("error should mention 'scope', got %q", textContent(res))
	}
}

func TestMCP_wn_settings_set_missingScope(t *testing.T) {
	dir := initSettingsTestRoot(t, "")
	t.Chdir(dir)

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	// scope is required in the schema; the framework rejects the call before the handler runs.
	_, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "sort", "value": "priority"},
	})
	if err == nil {
		t.Error("expected protocol error when scope is missing, got nil")
	}
}

func TestMCP_wn_settings_set_withExplicitRoot(t *testing.T) {
	dir := initSettingsTestRoot(t, "")

	ctx, cs, cleanup := newSettingsMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_settings_set",
		Arguments: map[string]any{"key": "sort", "value": "updated:desc", "scope": "project", "root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}

	s, err := readSettingsFromPath(filepath.Join(dir, ".wn", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Sort != "updated:desc" {
		t.Errorf("sort = %q, want %q", s.Sort, "updated:desc")
	}
}
