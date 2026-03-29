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
