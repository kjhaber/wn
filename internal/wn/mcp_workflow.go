package wn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
