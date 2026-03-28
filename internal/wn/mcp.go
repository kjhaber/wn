package wn

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
		Name:        "wn_note_search",
		Description: "Search all work items for those having a note with the given name, optionally filtered by exact value. Returns a JSON array of {id, description} objects. Use first:true to return only the oldest match (by created time) or latest:true for the most recently updated — useful when deduplicating (e.g. looking up the item for a branch: name=wn:branch value=<branch> first=true).",
	}, handleWnNoteSearch)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_note_add",
		Description: "Add or update a note on a work item by name. Note name: alphanumeric, slash, underscore, hyphen, 1–32 chars (e.g. pr-url, issue-number); or wn:<name> for special notes (e.g. wn:branch). Body is optional — omit to store an empty note, or for wn:branch to auto-detect the current git branch. If id is omitted, uses current task.",
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
		Description: "Mark a work item as a duplicate of another. Sets status to closed and appends the standard note 'duplicate-of' with the original item's id so it leaves the queue. Id is the item to mark (omit for current task); on is the id of the canonical/original work item.",
	}, handleWnDuplicate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_prompt",
		Description: "Create a prompt item (a question for the user) and add it as a dependency of a parent work item. The parent becomes blocked until the user responds with wn_respond. Use this when an agent needs a human decision before it can proceed. parent_id defaults to current task if omitted. Returns the new prompt item id.",
	}, handleWnPrompt)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_respond",
		Description: "Respond to a prompt item: marks it done and stores the answer as a 'response' note, unblocking the parent item. id defaults to current task if omitted.",
	}, handleWnRespond)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_merge",
		Description: "Squash-merge a feature branch into the current HEAD of the main repo, record the commit hash as a wn:commit note, and mark the associated work item done. The work item is found via its wn:branch note. Worktree-aware: git operations run in the main repo root. Use dry_run to preview without making changes.",
	}, handleWnMerge)

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

// errResult builds an IsError MCP tool result with the given message.
func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// withStore resolves the store (and root) for the given project root and calls fn.
// Returns an internal error if the store cannot be opened; otherwise returns fn's result.
func withStore(ctx context.Context, root string, fn func(Store, string) (*mcp.CallToolResult, error)) (*mcp.CallToolResult, any, error) {
	store, r, err := getStoreWithRoot(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	result, err := fn(store, r)
	return result, nil, err
}

// withItem resolves store+item for the given root+requestID and calls fn.
// Returns an error result if the ID cannot be resolved or the item is not found.
func withItem(ctx context.Context, root, requestID string, fn func(Store, *Item) (*mcp.CallToolResult, error)) (*mcp.CallToolResult, any, error) {
	store, r, err := getStoreWithRoot(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	meta, err := ReadMeta(r)
	if err != nil {
		return nil, nil, err
	}
	id, err := ResolveItemID(meta.CurrentID, requestID)
	if err != nil {
		return errResult("no id provided and no current task"), nil, nil
	}
	item, err := store.Get(id)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	result, err := fn(store, item)
	return result, nil, err
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
			return errResult("invalid or non-positive duration for 'for'"), nil, nil
		}
	}
	forMsg := in.For
	if forMsg == "" {
		forMsg = d.String()
	}
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		now := time.Now().UTC()
		until := now.Add(d)
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
			it.InProgressUntil = until
			it.InProgressBy = in.By
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: forMsg})
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("claimed %s for %s", id, forMsg)}}}, nil
	})
}

type wnReleaseIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnRelease(ctx context.Context, req *mcp.CallToolRequest, in wnReleaseIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		now := time.Now().UTC()
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
			it.InProgressUntil = time.Time{}
			it.InProgressBy = ""
			it.ReviewReady = true
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "released"})
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("released %s", id)}}}, nil
	})
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

type wnNoteSearchIn struct {
	Name   string `json:"name" jsonschema:"Note name to search for"`
	Value  string `json:"value,omitempty" jsonschema:"Optional exact note body to match"`
	First  bool   `json:"first,omitempty" jsonschema:"Return only the oldest matching item (by created time)"`
	Latest bool   `json:"latest,omitempty" jsonschema:"Return only the most recently updated matching item"`
	Root   string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

type noteSearchOut struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

func handleWnNoteSearch(ctx context.Context, req *mcp.CallToolRequest, in wnNoteSearchIn) (*mcp.CallToolResult, any, error) {
	if in.First && in.Latest {
		return errResult("first and latest are mutually exclusive"), nil, nil
	}
	return withStore(ctx, in.Root, func(store Store, _ string) (*mcp.CallToolResult, error) {
		items, err := store.List()
		if err != nil {
			return nil, err
		}
		var matches []*Item
		for _, it := range items {
			idx := it.NoteIndexByName(in.Name)
			if idx < 0 {
				continue
			}
			if in.Value != "" && it.Notes[idx].Body != in.Value {
				continue
			}
			matches = append(matches, it)
		}
		if len(matches) == 0 {
			return errResult(fmt.Sprintf("no items found with note %q", in.Name)), nil
		}
		if in.First {
			oldest := matches[0]
			for _, it := range matches[1:] {
				if it.Created.Before(oldest.Created) {
					oldest = it
				}
			}
			matches = []*Item{oldest}
		} else if in.Latest {
			newest := matches[0]
			for _, it := range matches[1:] {
				if it.Updated.After(newest.Updated) {
					newest = it
				}
			}
			matches = []*Item{newest}
		}
		out := make([]noteSearchOut, len(matches))
		for i, it := range matches {
			out[i] = noteSearchOut{ID: it.ID, Description: FirstLine(it.Description)}
		}
		raw, err := json.Marshal(&out)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
	})
}

type wnNoteAddIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Name string `json:"name" jsonschema:"Note name (alphanumeric, slash, underscore, hyphen, 1-32 chars; or wn:<name> for special notes e.g. wn:branch)"`
	Body string `json:"body,omitempty" jsonschema:"Note text; omit or leave empty to store an empty note. For wn:branch specifically, omitting body auto-detects the current git branch from the process cwd."`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNoteAdd(ctx context.Context, req *mcp.CallToolRequest, in wnNoteAddIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		if !ValidNoteName(in.Name) {
			return errResult(fmt.Sprintf("invalid note name %q (alphanumeric, slash, underscore, hyphen, 1-32 chars; or wn:<name> for special notes)", in.Name)), nil
		}
		if err := ValidateSpecialNote(in.Name); err != nil {
			return errResult(err.Error()), nil
		}
		trimmed := strings.TrimSpace(in.Body)
		switch {
		case trimmed == "" && in.Name == NoteNameBranch:
			cwd, err := os.Getwd()
			if err != nil {
				return errResult(fmt.Sprintf("wn:branch: %v", err)), nil
			}
			branch, err := CurrentBranchInDir(cwd)
			if err != nil {
				return errResult(fmt.Sprintf("wn:branch: could not detect current git branch: %v", err)), nil
			}
			trimmed = branch
		case trimmed == "" && in.Name == NoteNameCommit:
			return errResult("wn:commit requires a commit hash"), nil
		case in.Name == NoteNameCommit:
			exists, err := CommitExists(root, trimmed)
			if err != nil {
				return errResult(fmt.Sprintf("wn:commit: %v", err)), nil
			}
			if !exists {
				return errResult(fmt.Sprintf("wn:commit: commit %q not found in this repository", trimmed)), nil
			}
		}
		now := time.Now().UTC()
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
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
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("note %q added/updated on %s", in.Name, id)}}}, nil
	})
}

type wnNoteEditIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Name string `json:"name" jsonschema:"Note name to edit"`
	Body string `json:"body" jsonschema:"New note text"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNoteEdit(ctx context.Context, req *mcp.CallToolRequest, in wnNoteEditIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		if in.Name == "" {
			return errResult("name is required"), nil
		}
		trimmed := strings.TrimSpace(in.Body)
		if trimmed == "" {
			return errResult("body is required and cannot be empty"), nil
		}
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
			idx := it.NoteIndexByName(in.Name)
			if idx < 0 {
				return nil, fmt.Errorf("no note named %q", in.Name)
			}
			it.Notes[idx].Body = trimmed
			it.Updated = time.Now().UTC()
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("note %q updated on %s", in.Name, id)}}}, nil
	})
}

type wnNoteRmIn struct {
	ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
	Name string `json:"name" jsonschema:"Note name to remove"`
	Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnNoteRm(ctx context.Context, req *mcp.CallToolRequest, in wnNoteRmIn) (*mcp.CallToolResult, any, error) {
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		id, err := ResolveItemID(meta.CurrentID, in.ID)
		if err != nil {
			return errResult("no id provided and no current task"), nil
		}
		if in.Name == "" {
			return errResult("name is required"), nil
		}
		if err := store.UpdateItem(id, func(it *Item) (*Item, error) {
			idx := it.NoteIndexByName(in.Name)
			if idx < 0 {
				return nil, fmt.Errorf("no note named %q", in.Name)
			}
			it.Notes = append(it.Notes[:idx], it.Notes[idx+1:]...)
			it.Updated = time.Now().UTC()
			return it, nil
		}); err != nil {
			return errResult(err.Error()), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("note %q removed from %s", in.Name, id)}}}, nil
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

type wnPromptIn struct {
	ParentID string `json:"parent_id,omitempty" jsonschema:"ID of the parent work item that will be blocked; defaults to current task"`
	Question string `json:"question,omitempty" jsonschema:"The question or decision needed from the user"`
	Root     string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnPrompt(ctx context.Context, req *mcp.CallToolRequest, in wnPromptIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Question) == "" {
		return errResult("question is required"), nil, nil
	}
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		meta, err := ReadMeta(root)
		if err != nil {
			return nil, err
		}
		parentID, err := ResolveItemID(meta.CurrentID, in.ParentID)
		if err != nil {
			return errResult("no parent_id provided and no current task"), nil
		}
		if _, err := store.Get(parentID); err != nil {
			return errResult(fmt.Sprintf("parent item %s not found", parentID)), nil
		}
		promptID, err := GenerateID(store)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		promptItem := &Item{
			ID:          promptID,
			Description: strings.TrimSpace(in.Question),
			Created:     now,
			Updated:     now,
			PromptReady: true,
			Log:         []LogEntry{{At: now, Kind: "created"}, {At: now, Kind: "prompt_ready"}},
		}
		if err := store.Put(promptItem); err != nil {
			return nil, err
		}
		// Add prompt item as dependency of parent
		allItems, err := store.List()
		if err != nil {
			_ = store.Delete(promptID)
			return nil, err
		}
		if WouldCreateCycle(allItems, parentID, promptID) {
			_ = store.Delete(promptID)
			return errResult("circular dependency would result"), nil
		}
		if err := store.UpdateItem(parentID, func(it *Item) (*Item, error) {
			it.DependsOn = append(it.DependsOn, promptID)
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "depend_added", Msg: promptID})
			return it, nil
		}); err != nil {
			return nil, err
		}
		out := map[string]string{"id": promptID}
		raw, _ := json.Marshal(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
	})
}

type wnRespondIn struct {
	ID     string `json:"id,omitempty" jsonschema:"ID of the prompt item to respond to; defaults to current task"`
	Answer string `json:"answer" jsonschema:"The response to the question"`
	Root   string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnRespond(ctx context.Context, req *mcp.CallToolRequest, in wnRespondIn) (*mcp.CallToolResult, any, error) {
	return withItem(ctx, in.Root, in.ID, func(store Store, item *Item) (*mcp.CallToolResult, error) {
		if !item.PromptReady {
			return errResult(fmt.Sprintf("item %s is not in prompt state", item.ID)), nil
		}
		now := time.Now().UTC()
		answer := strings.TrimSpace(in.Answer)
		if err := store.UpdateItem(item.ID, func(it *Item) (*Item, error) {
			it.Done = true
			it.DoneStatus = DoneStatusDone
			it.PromptReady = false
			it.Updated = now
			it.Log = append(it.Log, LogEntry{At: now, Kind: "done", Msg: answer})
			if it.Notes == nil {
				it.Notes = []Note{}
			}
			idx := it.NoteIndexByName(NoteNameResponse)
			if idx >= 0 {
				it.Notes[idx].Body = answer
			} else {
				it.Notes = append(it.Notes, Note{Name: NoteNameResponse, Created: now, Body: answer})
			}
			return it, nil
		}); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("responded to %s; prompt marked done", item.ID)}}}, nil
	})
}

type wnMergeIn struct {
	Branch  string `json:"branch" jsonschema:"Feature branch name to squash-merge"`
	Message string `json:"message" jsonschema:"Commit message for the squash merge commit"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"If true, report what would happen without making changes"`
	Root    string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
}

func handleWnMerge(ctx context.Context, req *mcp.CallToolRequest, in wnMergeIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Branch) == "" {
		return errResult("error: branch is required"), nil, nil
	}
	if strings.TrimSpace(in.Message) == "" {
		return errResult("error: message is required"), nil, nil
	}
	return withStore(ctx, in.Root, func(store Store, root string) (*mcp.CallToolResult, error) {
		result, err := SquashMerge(store, root, in.Branch, in.Message, in.DryRun)
		if err != nil {
			return errResult(err.Error()), nil
		}
		var text string
		if in.DryRun {
			text = fmt.Sprintf("would squash-merge %s (item %s) into current HEAD", result.Branch, result.ItemID)
		} else {
			text = fmt.Sprintf("merged %s → %s (item %s marked done)", result.Branch, result.CommitHash[:7], result.ItemID)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	})
}
