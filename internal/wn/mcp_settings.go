package wn

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type wnSettingsGetIn struct {
	Key  string `json:"key" jsonschema:"Dot-notation settings key to read (e.g. sort, runners.tmux-claude.cmd)"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnSettingsGet(_ context.Context, _ *mcp.CallToolRequest, in wnSettingsGetIn) (*mcp.CallToolResult, any, error) {
	if in.Key == "" {
		return errResult("key is required"), nil, nil
	}

	root := resolveSettingsRoot(in.Root)
	settings, err := ReadSettingsInRoot(root)
	if err != nil {
		return nil, nil, err
	}

	val, err := SettingsGetValue(settings, in.Key)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: val}}}, nil, nil
}

type wnSettingsSetIn struct {
	Key   string `json:"key" jsonschema:"Dot-notation settings key to write (e.g. sort, runners.tmux-claude.cmd)"`
	Value string `json:"value" jsonschema:"Value to set. JSON values (true, 42, {\"cmd\":\"...\"}) are stored as their JSON type; plain strings are stored as strings."`
	Scope string `json:"scope" jsonschema:"Settings scope: user, user-local, project, or project-local"`
	Root  string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnSettingsSet(_ context.Context, _ *mcp.CallToolRequest, in wnSettingsSetIn) (*mcp.CallToolResult, any, error) {
	if in.Key == "" {
		return errResult("key is required"), nil, nil
	}

	root := resolveSettingsRoot(in.Root)

	path, err := resolveSettingsScopePath(in.Scope, root)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}

	if err := SettingsSetValue(path, in.Key, in.Value); err != nil {
		return errResult(err.Error()), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("set %s in %s settings", in.Key, in.Scope)}}}, nil, nil
}

// resolveSettingsRoot returns the wn project root for settings operations.
// It tries to find the root via mcpFixedRoot, explicit root, or process cwd;
// if none is found, returns "" (callers fall back to user-only settings).
func resolveSettingsRoot(requestRoot string) string {
	dir := requestRoot
	if mcpFixedRoot != "" {
		dir = mcpFixedRoot
	}
	var root string
	var err error
	if dir != "" {
		root, err = FindRootFromDir(dir)
	} else {
		root, err = FindRoot()
	}
	if err != nil {
		return ""
	}
	return root
}

// resolveSettingsScopePath returns the file path for the given scope label.
func resolveSettingsScopePath(scope string, root string) (string, error) {
	named, err := UserSettingsNamedPaths()
	if err != nil {
		return "", err
	}
	for _, n := range named {
		if n.Name == scope {
			return n.Path, nil
		}
	}
	if root != "" {
		switch scope {
		case "project":
			return ProjectSettingsPath(root), nil
		case "project-local":
			return ProjectLocalSettingsPath(root), nil
		}
	} else if scope == "project" || scope == "project-local" {
		return "", fmt.Errorf("scope %q requires a wn project root (run from a wn project or pass root)", scope)
	}
	return "", fmt.Errorf("unknown scope %q; valid scopes: user, user-local, project, project-local", scope)
}
