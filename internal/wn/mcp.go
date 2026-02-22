package wn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpVersion = "0.1.0"

// mcpFixedRoot, when set by SetMCPFixedRoot, locks the server to that project root:
// all tools use it and the per-request "root" parameter is ignored (guardrail).
var mcpFixedRoot string

// SetMCPFixedRoot sets the project root used by all MCP tools for this process.
// When non-empty, tools use this path instead of the request "root" or process cwd.
// Call before Run to lock the server to a specific workspace (e.g. from a spawn-time arg or env).
func SetMCPFixedRoot(root string) {
	mcpFixedRoot = root
}

// NewMCPServer returns an MCP server with wn tools registered (add, list, done, undone, desc, claim, release, next).
// Each tool accepts an optional "root" argument (used only when no fixed root is set). If the server was started with a fixed root (wn mcp /path or WN_ROOT), that path is used and request "root" is ignored.
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
		Name:        "wn_show",
		Description: "Fetch full work item as JSON by id (tags, deps, notes, log, etc.). If id is omitted, uses current task.",
	}, handleWnShow)
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_order",
		Description: "Set or clear optional backlog order for an item (lower = earlier when dependencies don't define order).",
	}, handleWnOrder)

	return server
}

// getStoreWithRoot returns the store and root for the given project. When mcpFixedRoot is set (spawn-time guardrail), it is used and projectRoot from the request is ignored. Otherwise, if projectRoot is non-empty it is used (via FindRootFromDir); else FindRoot() (process cwd).
func getStoreWithRoot(ctx context.Context, projectRoot string) (Store, string, error) {
	var root string
	var err error
	if mcpFixedRoot != "" {
		root, err = FindRootFromDir(mcpFixedRoot)
	} else if projectRoot != "" {
		root, err = FindRootFromDir(projectRoot)
	} else {
		root, err = FindRoot()
	}
	if err != nil {
		// Include what we tried so MCP callers can debug config (e.g. ${workspaceFolder})
		msg := err.Error()
		if mcpFixedRoot != "" {
			msg = fmt.Sprintf("%s (mcp fixed root was %q)", msg, mcpFixedRoot)
		} else if projectRoot != "" {
			msg = fmt.Sprintf("%s (request root was %q)", msg, projectRoot)
		}
		return nil, "", fmt.Errorf("%s", msg)
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
	Order       *int     `json:"order,omitempty" jsonschema:"Optional backlog order (lower = earlier)"`
	Root        string   `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnAdd(ctx context.Context, req *mcp.CallToolRequest, in wnAddIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
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
		Order:       in.Order,
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
	Tag  string `json:"tag,omitempty" jsonschema:"Filter by tag (optional)"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnList(ctx context.Context, req *mcp.CallToolRequest, in wnListIn) (*mcp.CallToolResult, any, error) {
	store, _, err := getStoreWithRoot(ctx, in.Root)
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
	var ordered []*Item
	settings, _ := ReadSettings()
	if spec := SortSpecFromSettings(settings); len(spec) > 0 {
		ordered = ApplySort(items, spec)
	} else {
		ordered, _ = TopoOrder(items)
	}
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
	Root    string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDone(ctx context.Context, req *mcp.CallToolRequest, in wnDoneIn) (*mcp.CallToolResult, any, error) {
	store, _, err := getStoreWithRoot(ctx, in.Root)
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
	ID   string `json:"id" jsonschema:"Work item id"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnUndone(ctx context.Context, req *mcp.CallToolRequest, in wnUndoneIn) (*mcp.CallToolResult, any, error) {
	store, _, err := getStoreWithRoot(ctx, in.Root)
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
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDesc(ctx context.Context, req *mcp.CallToolRequest, in wnDescIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
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

// showOutput is the JSON shape for wn_show; all slice fields have no omitempty so agents always see tags, log, notes, depends_on.
type showOutput struct {
	ID              string     `json:"id"`
	Description     string     `json:"description"`
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
	Done            bool       `json:"done"`
	DoneMessage     string     `json:"done_message,omitempty"`
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

func handleWnShow(ctx context.Context, req *mcp.CallToolRequest, in wnShowIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
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
	// Use showOutput so tags, log, notes, depends_on are always present in JSON for agents
	out := showOutput{
		ID:              item.ID,
		Description:     item.Description,
		Created:         item.Created,
		Updated:         item.Updated,
		Done:            item.Done,
		DoneMessage:     item.DoneMessage,
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
	raw, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

type wnClaimIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	For  string `json:"for" jsonschema:"Duration (e.g. 30m, 1h)"`
	By   string `json:"by,omitempty" jsonschema:"Optional worker id for logging"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnClaim(ctx context.Context, req *mcp.CallToolRequest, in wnClaimIn) (*mcp.CallToolResult, any, error) {
	d, err := time.ParseDuration(in.For)
	if err != nil || d <= 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "invalid or non-positive duration for 'for'"}}, IsError: true}, nil, nil
	}
	store, root, err := getStoreWithRoot(ctx, in.Root)
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
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnRelease(ctx context.Context, req *mcp.CallToolRequest, in wnReleaseIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
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

type wnNextIn struct {
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNext(ctx context.Context, req *mcp.CallToolRequest, in wnNextIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
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

type wnOrderIn struct {
	ID    string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Order *int   `json:"order,omitempty" jsonschema:"Set order to this number (lower = earlier)"`
	Unset bool   `json:"unset,omitempty" jsonschema:"If true, clear the order field"`
	Root  string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnOrder(ctx context.Context, req *mcp.CallToolRequest, in wnOrderIn) (*mcp.CallToolResult, any, error) {
	store, root, err := getStoreWithRoot(ctx, in.Root)
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
	if in.Unset {
		err = store.UpdateItem(id, func(it *Item) (*Item, error) {
			it.Order = nil
			it.Updated = time.Now().UTC()
			it.Log = append(it.Log, LogEntry{At: it.Updated, Kind: "order_cleared"})
			return it, nil
		})
	} else if in.Order != nil {
		n := *in.Order
		err = store.UpdateItem(id, func(it *Item) (*Item, error) {
			it.Order = &n
			it.Updated = time.Now().UTC()
			it.Log = append(it.Log, LogEntry{At: it.Updated, Kind: "order_set", Msg: fmt.Sprintf("%d", n)})
			return it, nil
		})
	} else {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "provide order (number) or unset: true"}}, IsError: true}, nil, nil
	}
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("order updated for %s", id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}
