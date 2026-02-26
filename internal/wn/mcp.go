package wn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpVersion = "0.1.0"

// DefaultClaimDuration is used when claim "for" (MCP) or --for (CLI) is omitted so agents can renew without passing a duration.
const DefaultClaimDuration = 1 * time.Hour

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
		Description: "Add a work item. Returns the new item's id. Pass optional depends_on (array of item IDs) to set dependencies when adding follow-up items so agentic queue order is preserved. Use tags (e.g. priority:high) and status suspend for prioritization.",
	}, handleWnAdd)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_list",
		Description: "List undone work items (includes both available-for-claim and review-ready; excludes in-progress). Returns a JSON array of objects with id, description (first line), tags, and status (undone or review-ready). Order: dependency order. Optionally filter by tag (e.g. tag 'priority:high'). Pass limit (max items to return), optional offset (skip N items), or cursor (item id to start after) for pagination and smaller context.",
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
		Name:        "wn_item",
		Description: "Get full work item JSON by id (tags, deps, notes, log, etc.). Id is required—use when you only have an item id (e.g. from wn_next or a subagent). No current-task fallback.",
	}, handleWnItem)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_claim",
		Description: "Mark a work item in progress for a duration. Item leaves the undone list until expiry or release. For is optional—when omitted, uses default (1h) so agents can renew (extend) without losing context.",
	}, handleWnClaim)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_release",
		Description: "Clear in-progress on a work item so it returns to the undone list.",
	}, handleWnRelease)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_next",
		Description: "Set the next available task as current and return its id and description. Next is chosen by dependency order. When tag is provided, return/set current to the next undone item that has that tag (dependency order). Enables getting the next agentic item without listing the full queue. Optionally pass claim_for (e.g. 30m) to atomically claim the item so concurrent workers don't double-assign.",
	}, handleWnNext)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_depend",
		Description: "Mark a work item as depending on another (add to depends_on). If id is omitted, uses current task.",
	}, handleWnDepend)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_rmdepend",
		Description: "Remove a dependency from a work item. If id is omitted, uses current task.",
	}, handleWnRmdepend)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_note_add",
		Description: "Add or update a note on a work item by name. Note name: alphanumeric, slash, underscore, hyphen, 1–32 chars (e.g. pr-url, issue-number). If id is omitted, uses current task.",
	}, handleWnNoteAdd)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_note_edit",
		Description: "Edit an existing note's body on a work item by name. If id is omitted, uses current task.",
	}, handleWnNoteEdit)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_note_rm",
		Description: "Remove a note by name from a work item. If id is omitted, uses current task.",
	}, handleWnNoteRm)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_duplicate",
		Description: "Mark a work item as a duplicate of another. Appends the standard note 'duplicate-of' with the original item's id and marks the item done so it leaves the queue. Id is the item to mark (omit for current task); on is the id of the canonical/original work item.",
	}, handleWnDuplicate)

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
	DependsOn   []string `json:"depends_on,omitempty" jsonschema:"Optional IDs this item will depend on (e.g. current task); preserves agentic queue order when adding follow-up items"`
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
	deps := uniqueStrings(in.DependsOn)
	if len(deps) > 0 {
		existing, err := store.List()
		if err != nil {
			return nil, nil, err
		}
		for _, depID := range deps {
			if _, err := store.Get(depID); err != nil {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("depends_on: item %s not found", depID)}}, IsError: true}, nil, nil
			}
		}
		newItemWithDeps := &Item{ID: id, DependsOn: deps}
		itemsWithNew := append(existing, newItemWithDeps)
		for _, depID := range deps {
			if WouldCreateCycle(itemsWithNew, id, depID) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("circular dependency detected, could not add item depending on %s", depID)}}, IsError: true}, nil, nil
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

type wnListIn struct {
	Tag    string `json:"tag,omitempty" jsonschema:"Filter by tag (optional)"`
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
	store, _, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}
	items, err := ListableUndoneItems(store)
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
			Status:      ItemListStatus(it, now),
		}
	}
	raw, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
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
		it.DoneStatus = DoneStatusDone
		it.ReviewReady = false
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
		it.DoneStatus = ""
		it.ReviewReady = false
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

type wnItemIn struct {
	ID   string `json:"id" jsonschema:"Work item id (required)"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

// handleWnItem returns full item JSON by id. Id is required (no current-task fallback), for use by subagents that only have an item id.
func handleWnItem(ctx context.Context, req *mcp.CallToolRequest, in wnItemIn) (*mcp.CallToolResult, any, error) {
	if in.ID == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "id is required"}}, IsError: true}, nil, nil
	}
	store, _, err := getStoreWithRoot(ctx, in.Root)
	if err != nil {
		return nil, nil, err
	}
	item, err := store.Get(in.ID)
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
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
	For  string `json:"for,omitempty" jsonschema:"Duration (e.g. 30m, 1h). Optional; when omitted, uses default (1h) so agents can renew without losing context"`
	By   string `json:"by,omitempty" jsonschema:"Optional worker id for logging"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnClaim(ctx context.Context, req *mcp.CallToolRequest, in wnClaimIn) (*mcp.CallToolResult, any, error) {
	var d time.Duration
	if in.For == "" {
		d = DefaultClaimDuration
	} else {
		var err error
		d, err = time.ParseDuration(in.For)
		if err != nil || d <= 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "invalid or non-positive duration for 'for'"}}, IsError: true}, nil, nil
		}
	}
	forMsg := in.For
	if forMsg == "" {
		forMsg = d.String()
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
		it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: forMsg})
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("claimed %s for %s", id, forMsg)
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
		it.ReviewReady = true
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
	Root     string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
	Tag      string `json:"tag,omitempty" jsonschema:"Optional tag; when set, return/set current to the next undone item that has this tag (dependency order)"`
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
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "invalid or non-positive claim_for duration"}}, IsError: true}, nil, nil
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
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
		}
		nextOut := map[string]any{"id": next.ID, "description": FirstLine(next.Description), "claimed": true, "claim_for": in.ClaimFor}
		raw, _ := json.Marshal(nextOut)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, map[string]string{"id": next.ID, "description": FirstLine(next.Description)}, nil
	}
	nextOut := map[string]any{"id": next.ID, "description": FirstLine(next.Description)}
	raw, _ := json.Marshal(nextOut)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, map[string]string{"id": next.ID, "description": FirstLine(next.Description)}, nil
}

type wnDependIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id that will depend on another; omit for current task"`
	On   string `json:"on" jsonschema:"ID of the item this one will depend on"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDepend(ctx context.Context, req *mcp.CallToolRequest, in wnDependIn) (*mcp.CallToolResult, any, error) {
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
	if in.On == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "on (dependency id) is required"}}, IsError: true}, nil, nil
	}
	items, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	if WouldCreateCycle(items, id, in.On) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("circular dependency detected, could not mark %s dependent on %s", id, in.On)}}, IsError: true}, nil, nil
	}
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
		for _, d := range it.DependsOn {
			if d == in.On {
				return it, nil
			}
		}
		it.DependsOn = append(it.DependsOn, in.On)
		it.Updated = time.Now().UTC()
		it.Log = append(it.Log, LogEntry{At: it.Updated, Kind: "depend_added", Msg: in.On})
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("%s now depends on %s", id, in.On)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnRmdependIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id to remove dependency from; omit for current task"`
	On   string `json:"on" jsonschema:"ID of the dependency to remove"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnRmdepend(ctx context.Context, req *mcp.CallToolRequest, in wnRmdependIn) (*mcp.CallToolResult, any, error) {
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
	if in.On == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "on (dependency id to remove) is required"}}, IsError: true}, nil, nil
	}
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
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
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("removed dependency %s from %s", in.On, id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnNoteAddIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Name string `json:"name" jsonschema:"Note name (alphanumeric, slash, underscore, hyphen, 1-32 chars)"`
	Body string `json:"body" jsonschema:"Note text (add or update)"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNoteAdd(ctx context.Context, req *mcp.CallToolRequest, in wnNoteAddIn) (*mcp.CallToolResult, any, error) {
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
	if !ValidNoteName(in.Name) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid note name %q (alphanumeric, slash, underscore, hyphen, 1-32 chars)", in.Name)}}, IsError: true}, nil, nil
	}
	trimmed := strings.TrimSpace(in.Body)
	if trimmed == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "body is required and cannot be empty"}}, IsError: true}, nil, nil
	}
	now := time.Now().UTC()
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
		if it.Notes == nil {
			it.Notes = []Note{}
		}
		idx := it.NoteIndexByName(in.Name)
		if idx >= 0 {
			it.Notes[idx].Body = trimmed
		} else {
			it.Notes = append(it.Notes, Note{Name: in.Name, Created: now, Body: trimmed})
		}
		it.Updated = now
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("note %q added/updated on %s", in.Name, id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnNoteEditIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Name string `json:"name" jsonschema:"Note name to edit"`
	Body string `json:"body" jsonschema:"New note text"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNoteEdit(ctx context.Context, req *mcp.CallToolRequest, in wnNoteEditIn) (*mcp.CallToolResult, any, error) {
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
	if in.Name == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}, IsError: true}, nil, nil
	}
	trimmed := strings.TrimSpace(in.Body)
	if trimmed == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "body is required and cannot be empty"}}, IsError: true}, nil, nil
	}
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
		idx := it.NoteIndexByName(in.Name)
		if idx < 0 {
			return nil, fmt.Errorf("no note named %q", in.Name)
		}
		it.Notes[idx].Body = trimmed
		it.Updated = time.Now().UTC()
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("note %q updated on %s", in.Name, id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnNoteRmIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Name string `json:"name" jsonschema:"Note name to remove"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNoteRm(ctx context.Context, req *mcp.CallToolRequest, in wnNoteRmIn) (*mcp.CallToolResult, any, error) {
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
	if in.Name == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}, IsError: true}, nil, nil
	}
	err = store.UpdateItem(id, func(it *Item) (*Item, error) {
		idx := it.NoteIndexByName(in.Name)
		if idx < 0 {
			return nil, fmt.Errorf("no note named %q", in.Name)
		}
		it.Notes = append(it.Notes[:idx], it.Notes[idx+1:]...)
		it.Updated = time.Now().UTC()
		return it, nil
	})
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("note %q removed from %s", in.Name, id)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

type wnDuplicateIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id to mark as duplicate; omit for current task"`
	On   string `json:"on" jsonschema:"ID of the canonical/original work item"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnDuplicate(ctx context.Context, req *mcp.CallToolRequest, in wnDuplicateIn) (*mcp.CallToolResult, any, error) {
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
	if in.On == "" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "on (original item id) is required"}}, IsError: true}, nil, nil
	}
	if err := MarkDuplicateOf(store, id, in.On); err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	text := fmt.Sprintf("marked %s as duplicate of %s", id, in.On)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}
