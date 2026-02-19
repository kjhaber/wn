package wn

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpVersion = "0.1.0"

// NewMCPServer returns an MCP server with wn tools registered (add, list, done, undone, desc, claim, release, next).
// The server uses the current working directory to find the wn root; the client (e.g. Cursor) should spawn the process with cwd set to the project directory.
func NewMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "wn", Version: mcpVersion}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_add",
		Description: "Add a work item. Returns the new item's id.",
	}, handleWnAdd)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_list",
		Description: "List available undone work items (id and first line of description). Optionally filter by tag.",
	}, handleWnList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_done",
		Description: "Mark a work item complete. Optionally provide a completion message.",
	}, handleWnDone)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_undone",
		Description: "Mark a work item not complete.",
	}, handleWnUndone)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_desc",
		Description: "Get the description (prompt-ready body) for a work item. If id is omitted, uses current task.",
	}, handleWnDesc)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_claim",
		Description: "Mark a work item in progress for a duration. Item leaves the undone list until expiry or release.",
	}, handleWnClaim)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_release",
		Description: "Clear in-progress on a work item so it returns to the undone list.",
	}, handleWnRelease)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_next",
		Description: "Set the next available task as current and return its id and description.",
	}, handleWnNext)

	return server
}

func getStore(ctx context.Context) (Store, string, error) {
	root, err := FindRoot()
	if err != nil {
		return nil, "", err
	}
	store, err := NewFileStore(root)
	if err != nil {
		return nil, "", err
	}
	return store, root, nil
}

type wnAddIn struct {
	Description string   `json:"description" jsonschema:"Full description of the work item"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Optional tags"`
}

func handleWnAdd(ctx context.Context, req *mcp.CallToolRequest, in wnAddIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	if in.Description == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "error: description is required"}}, IsError: true}, nil, nil
	}
	id, err := GenerateID(store)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          id,
		Description: in.Description,
		Created:     now,
		Updated:     now,
		Tags:        in.Tags,
		DependsOn:   nil,
		Log:         []LogEntry{{At: now, Kind: "created"}},
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
	text := fmt.Sprintf("added %s", id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, map[string]string{"id": id}, nil
}

type wnListIn struct {
	Tag string `json:"tag,omitempty" jsonschema:"Filter by tag (optional)"`
}

func handleWnList(ctx context.Context, req *mcp.CallToolRequest, in wnListIn) (*mcp.CallToolResult, any, error) {
	store, _, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	items, err := UndoneItems(store)
	if err != nil {
		return nil, nil, err
	}
	if in.Tag != "" {
		filtered := items[:0]
		for _, it := range items {
			for _, t := range it.Tags {
				if t == in.Tag {
					filtered = append(filtered, it)
					break
				}
			}
		}
		items = filtered
	}
	ordered, _ := TopoOrder(items)
	var lines string
	for _, it := range ordered {
		lines += fmt.Sprintf("  %s: %s\n", it.ID, FirstLine(it.Description))
	}
	if lines == "" {
		lines = "No undone tasks.\n"
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: lines}}}, nil, nil
}

type wnDoneIn struct {
	ID      string `json:"id" jsonschema:"Work item id (6-char hex)"`
	Message string `json:"message,omitempty" jsonschema:"Completion message"`
}

func handleWnDone(ctx context.Context, req *mcp.CallToolRequest, in wnDoneIn) (*mcp.CallToolResult, any, error) {
	store, _, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	err = store.UpdateItem(in.ID, func(it *Item) (*Item, error) {
		it.Done = true
		it.DoneMessage = in.Message
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "done", Msg: in.Message})
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("marked %s done", in.ID)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnUndoneIn struct {
	ID string `json:"id" jsonschema:"Work item id"`
}

func handleWnUndone(ctx context.Context, req *mcp.CallToolRequest, in wnUndoneIn) (*mcp.CallToolResult, any, error) {
	store, _, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	err = store.UpdateItem(in.ID, func(it *Item) (*Item, error) {
		it.Done = false
		it.DoneMessage = ""
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "undone"})
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("marked %s undone", in.ID)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnDescIn struct {
	ID string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
}

func handleWnDesc(ctx context.Context, req *mcp.CallToolRequest, in wnDescIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	meta, err := ReadMeta(root)
	if err != nil {
		return nil, nil, err
	}
	id, err := ResolveItemID(meta.CurrentID, in.ID)
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no id provided and no current task"}}, IsError: true}, nil, nil
	}
	item, err := store.Get(id)
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	body := PromptBody(item.Description)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: body}}}, nil, nil
}

type wnClaimIn struct {
	ID  string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	For string `json:"for" jsonschema:"Duration (e.g. 30m, 1h)"`
	By  string `json:"by,omitempty" jsonschema:"Optional worker id for logging"`
}

func handleWnClaim(ctx context.Context, req *mcp.CallToolRequest, in wnClaimIn) (*mcp.CallToolResult, any, error) {
	d, err := time.ParseDuration(in.For)
	if err != nil || d <= 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "invalid or non-positive duration for 'for'"}}, IsError: true}, nil, nil
	}
	store, root, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	meta, err := ReadMeta(root)
	if err != nil {
		return nil, nil, err
	}
	id, err := ResolveItemID(meta.CurrentID, in.ID)
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no id provided and no current task"}}, IsError: true}, nil, nil
	}
	now := time.Now().UTC()
	until := now.Add(d)
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
		it.InProgressUntil = until
		it.InProgressBy = in.By
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: in.For})
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("claimed %s for %s", id, in.For)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnReleaseIn struct {
	ID string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
}

func handleWnRelease(ctx context.Context, req *mcp.CallToolRequest, in wnReleaseIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	meta, err := ReadMeta(root)
	if err != nil {
		return nil, nil, err
	}
	id, err := ResolveItemID(meta.CurrentID, in.ID)
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no id provided and no current task"}}, IsError: true}, nil, nil
	}
	now := time.Now().UTC()
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "released"})
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("released %s", id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnNextIn struct{}

func handleWnNext(ctx context.Context, req *mcp.CallToolRequest, in wnNextIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	undone, err := UndoneItems(store)
	if err != nil {
		return nil, nil, err
	}
	ordered, acyclic := TopoOrder(undone)
	if !acyclic || len(ordered) == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No next task."}}}, nil, nil
	}
	next := ordered[0]
	if err := WithMetaLock(root, func(m Meta) (Meta, error) {
		m.CurrentID = next.ID
		return m, nil
	}); err != nil {
		return nil, nil, err
	}
	text := fmt.Sprintf("%s: %s", next.ID, FirstLine(next.Description))
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, map[string]string{"id": next.ID, "description": FirstLine(next.Description)}, nil
}
