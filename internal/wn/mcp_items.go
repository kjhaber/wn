package wn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type wnAddIn struct {
	Description string   `json:"description" jsonschema:"Full description of the work item"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Optional tags"`
	DependsOn   []string `json:"depends_on,omitempty" jsonschema:"Optional IDs this item will depend on (e.g. current task); preserves agentic queue order when adding follow-up items"`
	Root        string   `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnAdd(ctx context.Context, req *mcp.CallToolRequest, in wnAddIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}
	if in.Description == "" {
		return errResult("error: description is required"), nil, nil
	}
	id, err := GenerateID(store)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	deps := uniqueStrings(in.DependsOn)
	if len(deps) > 0 {
		existing, err := store.List()
		if err != nil {
			return nil, nil, err
		}
		for _, depID := range deps {
			if _, err := store.Get(depID); err != nil {
				return errResult(fmt.Sprintf("depends_on: item %s not found", depID)), nil, nil
			}
		}
		newItemWithDeps := &Item{ID: id, DependsOn: deps}
		itemsWithNew := append(existing, newItemWithDeps)
		for _, depID := range deps {
			if WouldCreateCycle(itemsWithNew, id, depID) {
				return errResult(fmt.Sprintf("circular dependency detected, could not add item depending on %s", depID)), nil, nil
			}
		}
	}
	item := &Item{
		ID:          id,
		Description: in.Description,
		Created:     now,
		Updated:     now,
		Tags:        in.Tags,
		DependsOn:   deps,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	for _, depID := range deps {
		item.Log = append(item.Log, LogEntry{At: now, Kind: "depend_added", Msg: depID})
	}
	if err := store.Put(item); err != nil {
		return nil, nil, err
	}
	if err := WithMetaLock(root, func(m Meta) (Meta, error) {
		m.CurrentID = id
		return m, nil
	}); err != nil {
		return nil, nil, err
	}
	// Structured JSON so agents can parse id without scraping text.
	out := map[string]string{"id": id}
	raw, _ := json.Marshal(out)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, out, nil
}

// uniqueStrings returns a copy of s with duplicate strings removed (order preserved).
func uniqueStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(s))
	for _, v := range s {
		if v == "" {
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

type wnDoneIn struct {
	ID      string `json:"id" jsonschema:"Work item id (6-char hex)"`
	Message string `json:"message,omitempty" jsonschema:"Completion message"`
	Root    string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDone(ctx context.Context, req *mcp.CallToolRequest, in wnDoneIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, _ string) (*mcp.CallToolResult, error) {
		now := time.Now().UTC()
		if err := store.UpdateItem(in.ID, func(it *Item) (*Item, error) {
			it.Done = true
			it.DoneMessage = in.Message
			it.DoneStatus = DoneStatusDone
			it.ReviewReady = false
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "done", Msg: in.Message})
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("marked %s done", in.ID)}}}, nil
	})
}

type wnUndoneIn struct {
	ID   string `json:"id" jsonschema:"Work item id"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnUndone(ctx context.Context, req *mcp.CallToolRequest, in wnUndoneIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, _ string) (*mcp.CallToolResult, error) {
		now := time.Now().UTC()
		if err := store.UpdateItem(in.ID, func(it *Item) (*Item, error) {
			it.Done = false
			it.DoneMessage = ""
			it.DoneStatus = ""
			it.ReviewReady = false
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "undone"})
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("marked %s undone", in.ID)}}}, nil
	})
}

type wnDescIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDesc(ctx context.Context, req *mcp.CallToolRequest, in wnDescIn) (*mcp.CallToolResult, any, error) {
	return withItem(ctx, in.Root, in.ID, func(_ Store, item *Item) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: PromptBody(item.Description)}}}, nil
	})
}

// showOutput is the JSON shape for wn_show; all slice fields have no omitempty so agents always see tags, log, notes, depends_on.
type showOutput struct {
	ID              string     `json:"id"`
	Description     string     `json:"description"`
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
	Done            bool       `json:"done"`
	DoneMessage     string     `json:"done_message,omitempty"`
	ReviewReady     bool       `json:"review_ready,omitempty"`
	PromptReady     bool       `json:"prompt_ready,omitempty"`
	InProgressUntil time.Time  `json:"in_progress_until,omitempty"`
	InProgressBy    string     `json:"in_progress_by,omitempty"`
	Tags            []string   `json:"tags"`
	DependsOn       []string   `json:"depends_on"`
	Order           *int       `json:"order,omitempty"`
	Log             []LogEntry `json:"log"`
	Notes           []Note     `json:"notes"`
}

type wnShowIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

// itemToShowOutput converts an Item to showOutput, ensuring slice fields are non-nil for JSON agents.
func itemToShowOutput(item *Item) showOutput {
	out := showOutput{
		ID:              item.ID,
		Description:     item.Description,
		Created:         item.Created,
		Updated:         item.Updated,
		Done:            item.Done,
		DoneMessage:     item.DoneMessage,
		ReviewReady:     item.ReviewReady,
		PromptReady:     item.PromptReady,
		InProgressUntil: item.InProgressUntil,
		InProgressBy:    item.InProgressBy,
		Tags:            item.Tags,
		DependsOn:       item.DependsOn,
		Order:           item.Order,
		Log:             item.Log,
		Notes:           item.Notes,
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if out.Log == nil {
		out.Log = []LogEntry{}
	}
	if out.Notes == nil {
		out.Notes = []Note{}
	}
	if out.DependsOn == nil {
		out.DependsOn = []string{}
	}
	return out
}

func handleWnShow(ctx context.Context, req *mcp.CallToolRequest, in wnShowIn) (*mcp.CallToolResult, any, error) {
	return withItem(ctx, in.Root, in.ID, func(_ Store, item *Item) (*mcp.CallToolResult, error) {
		out := itemToShowOutput(item)
		raw, err := json.MarshalIndent(&out, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
	})
}

type wnItemIn struct {
	ID   string `json:"id" jsonschema:"Work item id (required)"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

// handleWnItem returns full item JSON by id. Id is required (no current-task fallback), for use by subagents that only have an item id.
func handleWnItem(ctx context.Context, req *mcp.CallToolRequest, in wnItemIn) (*mcp.CallToolResult, any, error) {
	if in.ID == "" {
		return errResult("id is required"), nil, nil
	}
	return withItem(ctx, in.Root, in.ID, func(_ Store, item *Item) (*mcp.CallToolResult, error) {
		out := itemToShowOutput(item)
		raw, err := json.MarshalIndent(&out, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
	})
}
