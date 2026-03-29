package wn

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type wnDependIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id that will depend on another; omit for current task"`
	On   string `json:"on" jsonschema:"ID of the item this one will depend on"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDepend(ctx context.Context, req *mcp.CallToolRequest, in wnDependIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		if in.On == "" {
			return errResult("on (dependency id) is required"), nil
		}
		items, err := store.List()
		if err != nil {
			return nil, err
		}
		if WouldCreateCycle(items, id, in.On) {
			return errResult(fmt.Sprintf("circular dependency detected, could not mark %s dependent on %s", id, in.On)), nil
		}
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
			for _, d := range it.DependsOn {
				if d == in.On {
					return it, nil
				}
			}
			it.DependsOn = append(it.DependsOn, in.On)
			it.Updated = time.Now().UTC()
			it.Log = append(it.Log, LogEntry{At: it.Updated, Kind: "depend_added", Msg: in.On})
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s now depends on %s", id, in.On)}}}, nil
	})
}

type wnRmdependIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id to remove dependency from; omit for current task"`
	On   string `json:"on" jsonschema:"ID of the dependency to remove"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnRmdepend(ctx context.Context, req *mcp.CallToolRequest, in wnRmdependIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		if in.On == "" {
			return errResult("on (dependency id to remove) is required"), nil
		}
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
			var newDeps []string
			for _, d := range it.DependsOn {
				if d != in.On {
					newDeps = append(newDeps, d)
				}
			}
			it.DependsOn = newDeps
			it.Updated = time.Now().UTC()
			it.Log = append(it.Log, LogEntry{At: it.Updated, Kind: "depend_removed", Msg: in.On})
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed dependency %s from %s", in.On, id)}}}, nil
	})
}

type wnDuplicateIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id to mark as duplicate; omit for current task"`
	On   string `json:"on" jsonschema:"ID of the canonical/original work item"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDuplicate(ctx context.Context, req *mcp.CallToolRequest, in wnDuplicateIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		if in.On == "" {
			return errResult("on (original item id) is required"), nil
		}
		if err := SetStatus(store, id, StatusClosed, StatusOpts{DuplicateOf: in.On}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("marked %s as duplicate of %s", id, in.On)}}}, nil
	})
}
