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

// verifyOut is the expected JSON shape returned by wn_verify.
type verifyOut struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// newVerifyMCPSession creates a minimal MCP client+server without chdir (root is passed explicitly in calls).
func newVerifyMCPSession(t *testing.T) (context.Context, *mcp.ClientSession, func()) {
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

// initVerifyTestRoot creates a temp wn root and optionally writes a verify command to project settings.
func initVerifyTestRoot(t *testing.T, verifyCmd string) string {
	t.Helper()
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	if verifyCmd != "" {
		settings := map[string]string{"verify": verifyCmd}
		data, _ := json.Marshal(settings)
		if err := os.WriteFile(filepath.Join(dir, ".wn", "settings.json"), data, 0644); err != nil {
			t.Fatalf("WriteFile settings: %v", err)
		}
	}
	return dir
}

func TestMCP_wn_verify_success(t *testing.T) {
	dir := initVerifyTestRoot(t, "echo hello")
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_verify returned unexpected error: %s", textContent(res))
	}

	var out verifyOut
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_verify response not valid JSON: %v\ncontent: %s", err, textContent(res))
	}
	if out.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", out.ExitCode)
	}
	if !strings.Contains(out.Stdout, "hello") {
		t.Errorf("stdout should contain 'hello', got %q", out.Stdout)
	}
}

func TestMCP_wn_verify_failure(t *testing.T) {
	dir := initVerifyTestRoot(t, "echo build-error >&2; exit 2")
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if !res.IsError {
		t.Fatalf("wn_verify with failing command: expected IsError, got success: %s", textContent(res))
	}

	var out verifyOut
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("wn_verify failure response not valid JSON: %v\ncontent: %s", err, textContent(res))
	}
	if out.ExitCode == 0 {
		t.Errorf("exit_code = 0, want non-zero")
	}
}

func TestMCP_wn_verify_no_verify_configured(t *testing.T) {
	dir := initVerifyTestRoot(t, "") // no verify command in project settings
	// Blank user settings so the real user's verify setting doesn't interfere.
	cfgDir := t.TempDir()
	t.Setenv("WN_CONFIG_DIR", cfgDir)
	t.Setenv("WN_SETTINGS_USER", "")
	t.Setenv("WN_SETTINGS_USER_LOCAL", "")

	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError when no verify configured, got: %s", textContent(res))
	}
	if !strings.Contains(textContent(res), "verify") {
		t.Errorf("error should mention 'verify', got %q", textContent(res))
	}
}

func TestMCP_wn_verify_uses_root_dir(t *testing.T) {
	dir := initVerifyTestRoot(t, "pwd")
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_verify pwd returned error: %s", textContent(res))
	}

	var out verifyOut
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, textContent(res))
	}
	if !strings.Contains(strings.TrimSpace(out.Stdout), dir) {
		t.Errorf("stdout (pwd) should contain root dir %q, got %q", dir, out.Stdout)
	}
}

func TestMCP_wn_verify_captures_stderr(t *testing.T) {
	dir := initVerifyTestRoot(t, "echo errout >&2")
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"root": dir},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}

	var out verifyOut
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, textContent(res))
	}
	if !strings.Contains(out.Stderr, "errout") {
		t.Errorf("stderr should contain 'errout', got %q", out.Stderr)
	}
}

func TestMCP_wn_verify_invalid_timeout(t *testing.T) {
	dir := initVerifyTestRoot(t, "echo ok")
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"root": dir, "timeout": "notaduration"},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for invalid timeout, got: %s", textContent(res))
	}
	if !strings.Contains(textContent(res), "timeout") {
		t.Errorf("error should mention 'timeout', got %q", textContent(res))
	}
}
