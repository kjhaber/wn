package wn

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestErrResult(t *testing.T) {
	result := errResult("test error message")
	if !result.IsError {
		t.Fatal("errResult should set IsError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("errResult content length: want 1, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("errResult content[0] should be *mcp.TextContent")
	}
	if tc.Text != "test error message" {
		t.Errorf("errResult text: want %q, got %q", "test error message", tc.Text)
	}
}

func TestWithStore_success(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	ctx := context.Background()
	called := false
	result, data, err := withStore(ctx, dir, func(store Store, _ string) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	if err != nil {
		t.Fatalf("withStore error: %v", err)
	}
	if data != nil {
		t.Errorf("withStore data should be nil, got %v", data)
	}
	if !called {
		t.Error("withStore fn was not called")
	}
	if textContent(result) != "ok" {
		t.Errorf("withStore result: want %q, got %q", "ok", textContent(result))
	}
}

func TestWithStore_badRoot(t *testing.T) {
	ctx := context.Background()
	_, _, err := withStore(ctx, "/nonexistent/wn-test-path", func(_ Store, _ string) (*mcp.CallToolResult, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("withStore with bad root should return internal error")
	}
}

func TestWithStore_passesRoot(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	ctx := context.Background()
	var gotRoot string
	_, _, err := withStore(ctx, dir, func(_ Store, root string) (*mcp.CallToolResult, error) {
		gotRoot = root
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	if err != nil {
		t.Fatalf("withStore error: %v", err)
	}
	if gotRoot == "" {
		t.Error("withStore should pass root to fn")
	}
}

func TestWithItem_success(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &Item{
		ID:          "abc123",
		Description: "test item",
		Created:     now,
		Updated:     now,
		Log:         []LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ctx := context.Background()
	called := false
	result, _, err := withItem(ctx, dir, "abc123", func(_ Store, it *Item) (*mcp.CallToolResult, error) {
		called = true
		if it.ID != "abc123" {
			return errResult("wrong item"), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "got item"}}}, nil
	})
	if err != nil {
		t.Fatalf("withItem error: %v", err)
	}
	if !called {
		t.Error("withItem fn was not called")
	}
	if textContent(result) != "got item" {
		t.Errorf("withItem result: want %q, got %q", "got item", textContent(result))
	}
}

func TestWithItem_usesCurrentTask(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Put(&Item{ID: "abc123", Description: "task", Created: now, Updated: now, Log: []LogEntry{{At: now, Kind: "created"}}}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := WriteMeta(dir, Meta{CurrentID: "abc123"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	ctx := context.Background()
	result, _, err := withItem(ctx, dir, "", func(_ Store, it *Item) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: it.ID}}}, nil
	})
	if err != nil {
		t.Fatalf("withItem error: %v", err)
	}
	if textContent(result) != "abc123" {
		t.Errorf("withItem should fall back to current task, got %q", textContent(result))
	}
}

func TestWithItem_noID_noCurrentTask(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	ctx := context.Background()
	result, _, err := withItem(ctx, dir, "", func(_ Store, _ *Item) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "should not reach"}}}, nil
	})
	if err != nil {
		t.Fatalf("withItem error: %v", err)
	}
	if !result.IsError {
		t.Error("withItem with no ID and no current task should return error result")
	}
	if !strings.Contains(textContent(result), "no id") {
		t.Errorf("withItem error text should mention 'no id', got %q", textContent(result))
	}
}

func TestWithItem_itemNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	ctx := context.Background()
	result, _, err := withItem(ctx, dir, "notexist", func(_ Store, _ *Item) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "should not reach"}}}, nil
	})
	if err != nil {
		t.Fatalf("withItem error: %v", err)
	}
	if !result.IsError {
		t.Error("withItem with nonexistent item should return error result")
	}
}
