package wn

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
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
	Dir      string `json:"dir"`
	Cmd      string `json:"cmd"`
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

// initVerifyGitRepo initializes a minimal git repo in dir for at_root tests.
func initVerifyGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "init")
}

func TestMCP_wn_verify_success(t *testing.T) {
	dir := initVerifyTestRoot(t, "echo hello")
	t.Chdir(dir)
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{},
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
	t.Chdir(dir)
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{},
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
	t.Chdir(dir)

	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{},
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

func TestMCP_wn_verify_cwd_runs_in_process_cwd(t *testing.T) {
	// Without at_root, the command should run in the process's cwd, not the wn root.
	// Use a subdirectory so cwd ≠ wn root, distinguishing the two behaviors.
	dir := initVerifyTestRoot(t, "pwd")
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Chdir(subdir)
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{},
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
	// pwd should output subdir (process cwd), not dir (wn root)
	if !strings.Contains(strings.TrimSpace(out.Stdout), subdir) {
		t.Errorf("stdout (pwd) should contain cwd %q, got %q", subdir, out.Stdout)
	}
}

func TestMCP_wn_verify_at_root_runs_in_git_root(t *testing.T) {
	// With at_root=true, the command should run in the main git worktree root.
	dir := initVerifyTestRoot(t, "pwd")
	initVerifyGitRepo(t, dir)

	// cd into a subdirectory so cwd != git root
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Chdir(subdir)

	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"at_root": true},
	})
	if err != nil {
		t.Fatalf("CallTool wn_verify: %v", err)
	}
	if res.IsError {
		t.Fatalf("wn_verify at_root returned error: %s", textContent(res))
	}

	var out verifyOut
	if err := json.Unmarshal([]byte(textContent(res)), &out); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, textContent(res))
	}
	// pwd should contain the git root (dir), not the subdir
	if !strings.Contains(strings.TrimSpace(out.Stdout), dir) {
		t.Errorf("stdout (pwd) should contain git root %q, got %q", dir, out.Stdout)
	}
	if strings.Contains(strings.TrimSpace(out.Stdout), subdir) {
		t.Errorf("stdout (pwd) should not contain subdir %q, got %q", subdir, out.Stdout)
	}
}

func TestMCP_wn_verify_includes_dir_and_cmd_in_output(t *testing.T) {
	verifyCmd := "echo ok"
	dir := initVerifyTestRoot(t, verifyCmd)
	t.Chdir(dir)
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{},
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
	if out.Dir == "" {
		t.Errorf("dir field should be set in output, got empty string; full output: %s", textContent(res))
	}
	if out.Cmd != verifyCmd {
		t.Errorf("cmd = %q, want %q", out.Cmd, verifyCmd)
	}
}

func TestMCP_wn_verify_captures_stderr(t *testing.T) {
	dir := initVerifyTestRoot(t, "echo errout >&2")
	t.Chdir(dir)
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{},
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
	t.Chdir(dir)
	ctx, cs, cleanup := newVerifyMCPSession(t)
	defer cleanup()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wn_verify",
		Arguments: map[string]any{"timeout": "notaduration"},
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
