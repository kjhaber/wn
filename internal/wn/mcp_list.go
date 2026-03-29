package wn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type wnListIn struct {
	Tag    string `json:"tag,omitempty" jsonschema:"Filter by tag (optional); use 'a,b' for AND (must have both) or 'a|b' for OR (has either)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Return at most N items (optional; no limit if 0 or omitted)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Skip first N items (optional)"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Start after this item id (optional; for key-set pagination)"`
	Root   string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

// listItemOut is the JSON shape for each item returned by wn_list (id, description, tags, status).
type listItemOut struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

func handleWnList(ctx context.Context, req *mcp.CallToolRequest, in wnListIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}
	allItems, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	blockedSet := BlockedSet(allItems)
	tags, tagsMatchAll := ParseTagExpr(in.Tag)
	f := false
	items, err := QueryItems(store, ItemQuery{Done: &f, Tags: tags, TagsMatchAll: tagsMatchAll})
	if err != nil {
		return nil, nil, err
	}
	var ordered []*Item
	settings, _ := ReadSettingsInRoot(root)
	if spec := SortSpecFromSettings(settings); len(spec) > 0 {
		ordered = ApplySort(items, spec)
	} else {
		ordered, _ = TopoOrder(items)
	}
	// Apply cursor (start after this id), offset, and limit (bounded window for pagination).
	start := 0
	if in.Cursor != "" {
		for i, it := range ordered {
			if it.ID == in.Cursor {
				start = i + 1
				break
			}
		}
	}
	start += in.Offset
	if start > 0 || in.Limit > 0 {
		if start > len(ordered) {
			ordered = nil
		} else {
			ordered = ordered[start:]
			if in.Limit > 0 && len(ordered) > in.Limit {
				ordered = ordered[:in.Limit]
			}
		}
	}
	now := time.Now().UTC()
	out := make([]listItemOut, len(ordered))
	for i, it := range ordered {
		tags := it.Tags
		if tags == nil {
			tags = []string{}
		}
		out[i] = listItemOut{
			ID:          it.ID,
			Description: FirstLine(it.Description),
			Tags:        tags,
			Status:      ItemListStatus(it, now, blockedSet[it.ID]),
		}
	}
	raw, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

type wnNextIn struct {
	Root     string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
	Tag      string `json:"tag,omitempty" jsonschema:"Optional tag filter (dependency order); use 'a,b' for AND (must have both) or 'a|b' for OR (has either)"`
	ClaimFor string `json:"claim_for,omitempty" jsonschema:"If set, atomically claim the returned item for this duration (e.g. 30m, 1h)"`
	ClaimBy  string `json:"claim_by,omitempty" jsonschema:"Optional worker id when claim_for is set"`
}

func handleWnNext(ctx context.Context, req *mcp.CallToolRequest, in wnNextIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}
	next, err := NextUndoneItem(store, in.Tag)
	if err != nil {
		return nil, nil, err
	}
	if next == nil {
		// Empty: return JSON so agents can distinguish "no task" from "task with empty description".
		emptyOut := map[string]any{"id": nil, "description": nil}
		raw, _ := json.Marshal(emptyOut)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
	}
	if err := WithMetaLock(root, func(m Meta) (Meta, error) {
		m.CurrentID = next.ID
		return m, nil
	}); err != nil {
		return nil, nil, err
	}
	if in.ClaimFor != "" {
		d, err := time.ParseDuration(in.ClaimFor)
		if err != nil || d <= 0 {
			return errResult("invalid or non-positive claim_for duration"), nil, nil
		}
		now := time.Now().UTC()
		until := now.Add(d)
		err = store.UpdateItem(next.ID, func(it *Item) (*Item, error) {
			it.InProgressUntil = until
			it.InProgressBy = in.ClaimBy
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: in.ClaimFor})
			return it, nil
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		nextOut := map[string]any{"id": next.ID, "description": FirstLine(next.Description), "claimed": true, "claim_for": in.ClaimFor}
		raw, _ := json.Marshal(nextOut)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, map[string]string{"id": next.ID, "description": FirstLine(next.Description)}, nil
	}
	nextOut := map[string]any{"id": next.ID, "description": FirstLine(next.Description)}
	raw, _ := json.Marshal(nextOut)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, map[string]string{"id": next.ID, "description": FirstLine(next.Description)}, nil
}

type wnPickIn struct {
	ID   string `json:"id" jsonschema:"Item id to set as current task; use \".\" to select the item whose wn:branch note matches the current git branch of the project root"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnPick(ctx context.Context, req *mcp.CallToolRequest, in wnPickIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}
	if in.ID == "" {
		return errResult("id is required: pass a concrete item id or \".\" to pick by current git branch"), nil, nil
	}

	var item *Item
	if in.ID == "." {
		branch, err := CurrentBranchInDir(root)
		if err != nil {
			return errResult(fmt.Sprintf("could not determine git branch: %s", err)), nil, nil
		}
		item, err = FindItemByBranch(store, branch)
		if err != nil {
			return nil, nil, err
		}
		if item == nil {
			return errResult(fmt.Sprintf("no work item found for branch %q", branch)), nil, nil
		}
	} else {
		item, err = store.Get(in.ID)
		if err != nil {
			return errResult(fmt.Sprintf("item %s not found", in.ID)), nil, nil
		}
	}

	if err := WithMetaLock(root, func(m Meta) (Meta, error) {
		m.CurrentID = item.ID
		return m, nil
	}); err != nil {
		return nil, nil, err
	}
	out := map[string]any{"id": item.ID, "description": FirstLine(item.Description)}
	raw, _ := json.Marshal(out)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}
