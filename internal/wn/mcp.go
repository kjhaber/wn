package wn

import (
	"context"
	"fmt"
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_pick",
		Description: "Set the current task (meta CurrentID). Pass a concrete item id to set it directly. Pass \".\" to select the item whose wn:branch note matches the current git branch of the project root (same as CLI wn pick .). Returns JSON {id, description} of the newly selected item.",
	}, handleWnPick)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_root",
		Description: "Return the absolute main git repository root path (worktree-aware). In a linked worktree, resolves to the main repo root. Returns JSON {root: \"/absolute/path\"}. Useful for agents that need to run git commands in the correct repo when operating from a worktree.",
	}, handleWnRoot)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wn_verify",
		Description: "Run the project's configured verify command (settings.verify, e.g. 'make all') from the project root directory. Returns JSON with stdout, stderr, and exit_code. Sets IsError when exit_code is non-zero. Equivalent to 'wn verify --root'. Use this to confirm a build or test suite passes without relying on the MCP server's working directory.",
	}, handleWnVerify)

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
