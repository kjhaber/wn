package wn

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultVerifyTimeout = 10 * time.Minute

type wnVerifyIn struct {
	Root    string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
	Timeout string `json:"timeout,omitempty" jsonschema:"Maximum time to wait for the verify command (e.g. 30s, 5m); default is 10m"`
}

func handleWnVerify(ctx context.Context, req *mcp.CallToolRequest, in wnVerifyIn) (*mcp.CallToolResult, any, error) {
	_, root, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}

	settings, err := ReadSettingsInRoot(root)
	if err != nil {
		return nil, nil, err
	}
	if settings.Verify == "" {
		return errResult("no verify command configured; set 'verify' in .wn/settings.json or ~/.config/wn/settings.json"), nil, nil
	}

	timeout := defaultVerifyTimeout
	if in.Timeout != "" {
		d, parseErr := time.ParseDuration(in.Timeout)
		if parseErr != nil || d <= 0 {
			return errResult("invalid or non-positive timeout"), nil, nil
		}
		timeout = d
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", settings.Verify)
	cmd.Dir = root

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = devNull.Close() }()
	cmd.Stdin = devNull

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		exitErr, isExitError := runErr.(*exec.ExitError)
		switch {
		case isExitError:
			exitCode = exitErr.ExitCode()
		case cmdCtx.Err() == context.DeadlineExceeded:
			exitCode = -1
		default:
			return nil, nil, runErr
		}
	}

	out := map[string]any{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
	}
	raw, _ := json.Marshal(out)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		IsError: exitCode != 0,
	}, nil, nil
}
