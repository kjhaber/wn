package main

import (
	"strings"
	"testing"
	"time"

	"github.com/keith/wn/internal/wn"
)

// --- tuiSplitArgs ---

func TestTUISplitArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"vim", []string{"vim"}},
		{"vim -f", []string{"vim", "-f"}},
		{"code --wait", []string{"code", "--wait"}},
		{`"my editor" --flag`, []string{"my editor", "--flag"}},
		{"  vim  ", []string{"vim"}},
		{"", nil},
	}
	for _, tc := range tests {
		got := tuiSplitArgs(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("tuiSplitArgs(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("tuiSplitArgs(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// --- tuiItemDetail ---

func TestTUIItemDetail_BasicFields(t *testing.T) {
	item := &wn.Item{
		ID:          "abc123",
		Description: "My task\n\nMore details here.",
		Created:     time.Now().UTC(),
		Updated:     time.Now().UTC(),
		Tags:        []string{"feature", "urgent"},
		Log:         []wn.LogEntry{{At: time.Now().UTC(), Kind: "created"}},
	}
	got := tuiItemDetail(item, nil)

	if !strings.Contains(got, "My task") {
		t.Error("expected description in detail")
	}
	if !strings.Contains(got, "More details here.") {
		t.Error("expected body in detail")
	}
	if !strings.Contains(got, "tags:") {
		t.Error("expected tags label in detail")
	}
	if !strings.Contains(got, "feature") {
		t.Error("expected tag value in detail")
	}
	if !strings.Contains(got, "log:") {
		t.Error("expected log label in detail")
	}
	if !strings.Contains(got, "created") {
		t.Error("expected log entry kind in detail")
	}
}

func TestTUIItemDetail_DoneStatus(t *testing.T) {
	item := &wn.Item{
		ID:          "abc123",
		Description: "A done task",
		Done:        true,
		DoneStatus:  wn.DoneStatusDone,
		Created:     time.Now().UTC(),
		Updated:     time.Now().UTC(),
	}
	got := tuiItemDetail(item, nil)
	if !strings.Contains(got, "status:") {
		t.Error("expected status in detail")
	}
}

func TestTUIItemDetail_WithNotes(t *testing.T) {
	now := time.Now().UTC()
	item := &wn.Item{
		ID:          "abc123",
		Description: "Task with notes",
		Created:     now,
		Updated:     now,
		Notes: []wn.Note{
			{Name: "pr-url", Created: now, Body: "https://example.com/pr/1"},
		},
	}
	got := tuiItemDetail(item, nil)
	if !strings.Contains(got, "notes:") {
		t.Error("expected notes section")
	}
	if !strings.Contains(got, "pr-url") {
		t.Error("expected note name")
	}
	if !strings.Contains(got, "https://example.com/pr/1") {
		t.Error("expected note body")
	}
}

func TestTUIItemDetail_NoDepsWhenEmpty(t *testing.T) {
	item := &wn.Item{
		ID:          "abc123",
		Description: "Simple task",
		Created:     time.Now().UTC(),
		Updated:     time.Now().UTC(),
	}
	got := tuiItemDetail(item, nil)
	if strings.Contains(got, "deps:") {
		t.Error("should not show deps when none")
	}
}

// --- applyFilter ---

func TestApplyFilter_Empty(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "foo"},
			{ID: "b", Description: "bar"},
		},
	}
	m.filterText = ""
	m.applyFilter()
	if len(m.items) != 2 {
		t.Errorf("empty filter should include all items, got %d", len(m.items))
	}
}

func TestApplyFilter_ByDescription(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "fix the login bug"},
			{ID: "b", Description: "add new feature"},
			{ID: "c", Description: "update login page"},
		},
	}
	m.filterText = "login"
	m.applyFilter()
	if len(m.items) != 2 {
		t.Errorf("expected 2 items matching 'login', got %d", len(m.items))
	}
	for _, it := range m.items {
		if !strings.Contains(strings.ToLower(it.Description), "login") {
			t.Errorf("item %s does not match filter", it.ID)
		}
	}
}

func TestApplyFilter_ByTag(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "task A", Tags: []string{"agent"}},
			{ID: "b", Description: "task B", Tags: []string{"manual"}},
			{ID: "c", Description: "task C", Tags: []string{"agent", "urgent"}},
		},
	}
	m.filterText = "agent"
	m.applyFilter()
	if len(m.items) != 2 {
		t.Errorf("expected 2 items with 'agent' tag, got %d", len(m.items))
	}
}

func TestApplyFilter_CaseInsensitive(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "Fix the Login Bug"},
		},
	}
	m.filterText = "login"
	m.applyFilter()
	if len(m.items) != 1 {
		t.Error("filter should be case insensitive")
	}
}

func TestApplyFilter_NoMatch(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "foo"},
		},
	}
	m.filterText = "zzznomatch"
	m.applyFilter()
	if len(m.items) != 0 {
		t.Errorf("expected 0 items, got %d", len(m.items))
	}
}

// --- clampCursor ---

func TestClampCursor_WithinBounds(t *testing.T) {
	m := tuiModel{
		items:  []*wn.Item{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		cursor: 1,
		width:  80,
		height: 24,
	}
	m.clampCursor()
	if m.cursor != 1 {
		t.Errorf("cursor should stay at 1, got %d", m.cursor)
	}
}

func TestClampCursor_BeyondEnd(t *testing.T) {
	m := tuiModel{
		items:  []*wn.Item{{ID: "a"}},
		cursor: 5,
		width:  80,
		height: 24,
	}
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("cursor should clamp to 0, got %d", m.cursor)
	}
}

func TestClampCursor_EmptyList(t *testing.T) {
	m := tuiModel{
		items:  nil,
		cursor: 3,
		width:  80,
		height: 24,
	}
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("cursor should be 0 for empty list, got %d", m.cursor)
	}
	if m.listOffset != 0 {
		t.Errorf("listOffset should be 0 for empty list, got %d", m.listOffset)
	}
}
