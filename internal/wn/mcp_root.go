package wn

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type wnRootIn struct {
	Root string `json:"root,omitempty" jsonschema:"Optional directory path; if omitted, uses process cwd. When mcpFixedRoot is set, this is ignored."`
}

func handleWnRoot(_ context.Context, _ *mcp.CallToolRequest, in wnRootIn) (*mcp.CallToolResult, any, error) {
	dir := in.Root
	if mcpFixedRoot != "" {
		dir = mcpFixedRoot
	}
	root, err := GitRepoRootFromDir(dir)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	data, err := json.Marshal(map[string]string{"root": root})
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}
