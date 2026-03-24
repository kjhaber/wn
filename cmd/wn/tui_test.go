package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kjhaber/wn/internal/wn"
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
	got := tuiItemDetail(item, false, nil, 80)

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
	got := tuiItemDetail(item, false, nil, 80)
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
	got := tuiItemDetail(item, false, nil, 80)
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
	got := tuiItemDetail(item, false, nil, 80)
	if strings.Contains(got, "deps:") {
		t.Error("should not show deps when none")
	}
}

func TestTUIItemDetail_LongDescriptionWraps(t *testing.T) {
	// A description with no newlines longer than the width should be wrapped.
	longDesc := "This is a very long description that should be wrapped because it exceeds the specified column width by quite a bit and needs to be readable."
	item := &wn.Item{
		ID:          "abc123",
		Description: longDesc,
		Created:     time.Now().UTC(),
		Updated:     time.Now().UTC(),
	}
	width := 40
	got := tuiItemDetail(item, false, nil, width)

	// All content should still be present (no truncation).
	if !strings.Contains(got, "This is a very long description") {
		t.Error("expected beginning of description in output")
	}
	if !strings.Contains(got, "readable.") {
		t.Error("expected end of description in output")
	}

	// Every line of the description portion should fit within the width.
	descPart := strings.Split(got, "\nstatus:")[0]
	for _, line := range strings.Split(descPart, "\n") {
		if len([]rune(line)) > width {
			t.Errorf("description line exceeds width %d: %q (len=%d)", width, line, len([]rune(line)))
		}
	}
}

func TestTUIItemDetail_ExistingNewlinesPreserved(t *testing.T) {
	// Explicit newlines in the description must be preserved after wrapping.
	desc := "First paragraph.\n\nSecond paragraph."
	item := &wn.Item{
		ID:          "abc123",
		Description: desc,
		Created:     time.Now().UTC(),
		Updated:     time.Now().UTC(),
	}
	got := tuiItemDetail(item, false, nil, 80)
	if !strings.Contains(got, "First paragraph.") {
		t.Error("expected first paragraph")
	}
	if !strings.Contains(got, "Second paragraph.") {
		t.Error("expected second paragraph")
	}
	// The blank line between paragraphs should still appear.
	if !strings.Contains(got, "First paragraph.\n\nSecond paragraph.") {
		t.Error("blank line between paragraphs should be preserved")
	}
}

func TestTUIItemDetail_LongNoteBodyWraps(t *testing.T) {
	now := time.Now().UTC()
	longBody := "This note body is intentionally very long so that it will need to be word-wrapped when displayed in a narrow terminal column."
	item := &wn.Item{
		ID:          "abc123",
		Description: "Task",
		Created:     now,
		Updated:     now,
		Notes:       []wn.Note{{Name: "detail", Created: now, Body: longBody}},
	}
	width := 40
	got := tuiItemDetail(item, false, nil, width)

	// Note body content must be present (check words that fit on one line).
	if !strings.Contains(got, "intentionally") {
		t.Error("expected 'intentionally' from note body in output")
	}
	if !strings.Contains(got, "word-wrapped") {
		t.Error("expected 'word-wrapped' from note body in output")
	}

	// Find the notes section and check that no note-body line exceeds width.
	noteIdx := strings.Index(got, "\nnotes:")
	if noteIdx < 0 {
		t.Fatal("notes section not found")
	}
	notesSection := got[noteIdx+1:]
	for _, line := range strings.Split(notesSection, "\n") {
		stripped := strings.TrimLeft(line, " ") // strip leading indent
		if len([]rune(stripped)) > width {
			t.Errorf("note body line exceeds width %d: %q (len=%d)", width, stripped, len([]rune(stripped)))
		}
	}
}

// --- wrapLines ---

func TestWrapLines_ShortLinesUnchanged(t *testing.T) {
	in := "short line"
	got := wrapLines(in, 40)
	if got != in {
		t.Errorf("wrapLines(%q, 40) = %q, want %q", in, got, in)
	}
}

func TestWrapLines_LongLineWraps(t *testing.T) {
	in := "one two three four five six seven eight nine ten"
	got := wrapLines(in, 20)
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 20 {
			t.Errorf("line exceeds width 20: %q", line)
		}
	}
	// All words must still appear.
	if !strings.Contains(got, "one") || !strings.Contains(got, "ten") {
		t.Error("expected all words to be present after wrap")
	}
}

func TestWrapLines_PreservesExplicitNewlines(t *testing.T) {
	in := "para one\n\npara two"
	got := wrapLines(in, 40)
	if got != in {
		t.Errorf("wrapLines should preserve explicit newlines: got %q", got)
	}
}

func TestWrapLines_MultiParagraphWrap(t *testing.T) {
	in := "a b c d e f g h i j\n\nk l m n o p q r s t"
	got := wrapLines(in, 10)
	// Blank line should be preserved.
	if !strings.Contains(got, "\n\n") {
		t.Error("expected blank line to be preserved in output")
	}
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 10 {
			t.Errorf("line exceeds width 10: %q", line)
		}
	}
}

func TestWrapLines_ZeroWidthNoWrap(t *testing.T) {
	in := "a very long line that would normally wrap"
	got := wrapLines(in, 0)
	// width <= 0 means no wrapping.
	if got != in {
		t.Errorf("wrapLines with width=0 should return unchanged: got %q", got)
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

func TestApplyFilter_ByTagText(t *testing.T) {
	// plain text filter should still match on tags
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

func TestApplyFilter_HashTagOnly(t *testing.T) {
	// "#tag" prefix = tag-only match; should not match description text
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "agent task", Tags: []string{"manual"}},     // desc matches but tag doesn't
			{ID: "b", Description: "some task", Tags: []string{"agent"}},       // tag matches
			{ID: "c", Description: "other task", Tags: []string{"agent", "x"}}, // tag matches
		},
	}
	m.filterText = "#agent"
	m.applyFilter()
	if len(m.items) != 2 {
		t.Errorf("expected 2 items with #agent tag filter, got %d", len(m.items))
	}
	for _, it := range m.items {
		if it.ID == "a" {
			t.Error("item 'a' matched by description should be excluded by #tag filter")
		}
	}
}

func TestApplyFilter_StatusActive(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "undone task", Done: false},
			{ID: "b", Description: "done task", Done: true, DoneStatus: wn.DoneStatusDone},
		},
		statusFilter: tuiFilterActive,
	}
	m.applyFilter()
	if len(m.items) != 1 || m.items[0].ID != "a" {
		t.Errorf("active filter should show only undone items, got %v", tuiItemIDs(m.items))
	}
}

func TestApplyFilter_StatusDone(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "undone task", Done: false},
			{ID: "b", Description: "done task", Done: true, DoneStatus: wn.DoneStatusDone},
			{ID: "c", Description: "suspended", Done: true, DoneStatus: wn.DoneStatusSuspend},
		},
		statusFilter: tuiFilterDone,
	}
	m.applyFilter()
	if len(m.items) != 2 {
		t.Errorf("done filter should show only done items, got %v", tuiItemIDs(m.items))
	}
}

func TestApplyFilter_StatusReview(t *testing.T) {
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "normal task", Done: false, ReviewReady: false},
			{ID: "b", Description: "review task", Done: false, ReviewReady: true},
		},
		statusFilter: tuiFilterReview,
	}
	m.applyFilter()
	if len(m.items) != 1 || m.items[0].ID != "b" {
		t.Errorf("review filter should show only review-ready items, got %v", tuiItemIDs(m.items))
	}
}

func TestApplyFilter_StatusAndTextCombined(t *testing.T) {
	// Both filters must pass
	m := tuiModel{
		allItems: []*wn.Item{
			{ID: "a", Description: "login feature", Done: false},
			{ID: "b", Description: "login bug", Done: true, DoneStatus: wn.DoneStatusDone},
			{ID: "c", Description: "other thing", Done: false},
		},
		statusFilter: tuiFilterActive,
		filterText:   "login",
	}
	m.applyFilter()
	if len(m.items) != 1 || m.items[0].ID != "a" {
		t.Errorf("combined filter: expected [a], got %v", tuiItemIDs(m.items))
	}
}

func tuiItemIDs(items []*wn.Item) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
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
	// cursor tracks row index; build rows first so clampCursor has bounds.
	// 3 undone items → 1 header + 3 items = 4 rows; cursor=2 is item "b".
	m := tuiModel{
		items:  []*wn.Item{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		width:  80,
		height: 24,
	}
	m.buildRows()
	m.cursor = 2 // row 2 = item "b" (rows: header, a, b, c)
	m.clampCursor()
	if m.cursor != 2 {
		t.Errorf("cursor should stay at 2, got %d", m.cursor)
	}
}

func TestClampCursor_BeyondEnd(t *testing.T) {
	// 1 undone item → 1 header + 1 item = 2 rows; cursor beyond end clamps to last row.
	m := tuiModel{
		items:  []*wn.Item{{ID: "a"}},
		width:  80,
		height: 24,
	}
	m.buildRows()
	m.cursor = 10
	m.clampCursor()
	if m.cursor != 1 { // last row (the item row)
		t.Errorf("cursor should clamp to 1 (last row), got %d", m.cursor)
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

// --- respond key ("r") ---

func TestHandleEditor_RespondMarksItemDoneWithNote(t *testing.T) {
	root := t.TempDir()
	if err := wn.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	prompt := &wn.Item{
		ID: "pmt-tui01", Description: "Need answer?",
		Created: now, Updated: now, PromptReady: true,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(prompt); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Write response text to a temp file (simulating editor output).
	f, err := os.CreateTemp("", "wn-tui-resp-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpFile := f.Name()
	_, _ = f.WriteString("Use approach B.")
	_ = f.Close()
	defer func() { _ = os.Remove(tmpFile) }()

	m := tuiModel{store: store, root: root, width: 80, height: 24}
	result, _ := m.handleEditor(tuiEditorMsg{
		action:  tuiEditorRespond,
		tmpFile: tmpFile,
		id:      "pmt-tui01",
	})

	got, err := store.Get("pmt-tui01")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Done {
		t.Error("prompt item should be done after respond")
	}
	if got.PromptReady {
		t.Error("PromptReady should be cleared after respond")
	}
	idx := got.NoteIndexByName(wn.NoteNameResponse)
	if idx < 0 {
		t.Error("response note not found")
	} else if got.Notes[idx].Body != "Use approach B." {
		t.Errorf("response note body = %q", got.Notes[idx].Body)
	}
	if !strings.Contains(result.msg, "pmt-tui01") {
		t.Errorf("msg should mention item ID, got %q", result.msg)
	}
}

// --- renderHints and help toggle ---

func TestRenderHints_CompactFitsIn80Chars(t *testing.T) {
	m := tuiModel{width: 80, height: 24}
	hints := m.renderHints()
	w := lipgloss.Width(hints)
	if w > 80 {
		t.Errorf("renderHints visual width %d exceeds 80 chars", w)
	}
}

func TestRenderHints_ContainsHelpKey(t *testing.T) {
	m := tuiModel{width: 80, height: 24}
	hints := m.renderHints()
	if !strings.Contains(hints, "?") {
		t.Error("renderHints should contain '?' for help toggle")
	}
}

func TestHandleKey_QuestionMark_TogglesHelp(t *testing.T) {
	m := tuiModel{width: 80, height: 24}
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !result.showHelp {
		t.Error("'?' should enable showHelp")
	}
	result2, _ := result.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if result2.showHelp {
		t.Error("'?' again should disable showHelp")
	}
}

func TestTUIHelpContent_ContainsAllShortcuts(t *testing.T) {
	content := tuiHelpContent()
	shortcuts := []string{"a", "e", "x", "u", "-", "D", "r", ">", "/", "#", "f", "g", "q"}
	for _, s := range shortcuts {
		if !strings.Contains(content, "["+s+"]") {
			t.Errorf("help content missing shortcut [%s]", s)
		}
	}
	if !strings.Contains(content, "Space") {
		t.Error("help content missing Space shortcut")
	}
}

// --- default filter ---

func TestNewTUI_DefaultFilterIsActive(t *testing.T) {
	m := newTUI(nil, "", wn.Settings{}, "")
	if m.statusFilter != tuiFilterActive {
		t.Errorf("default statusFilter = %q, want %q", m.statusFilter, tuiFilterActive)
	}
}

// --- tuiGroupKey ---

func TestTUIGroupKey_Prompt(t *testing.T) {
	it := &wn.Item{PromptReady: true}
	if got := tuiGroupKey(it, false, time.Now()); got != tuiGroupPrompt {
		t.Errorf("PromptReady item got group %d, want %d (tuiGroupPrompt)", got, tuiGroupPrompt)
	}
}

func TestTUIGroupKey_Review(t *testing.T) {
	it := &wn.Item{ReviewReady: true}
	if got := tuiGroupKey(it, false, time.Now()); got != tuiGroupReview {
		t.Errorf("ReviewReady item got group %d, want %d (tuiGroupReview)", got, tuiGroupReview)
	}
}

func TestTUIGroupKey_InProgress(t *testing.T) {
	now := time.Now().UTC()
	it := &wn.Item{InProgressUntil: now.Add(time.Hour)}
	if got := tuiGroupKey(it, false, now); got != tuiGroupClaimed {
		t.Errorf("InProgress item got group %d, want %d (tuiGroupClaimed)", got, tuiGroupClaimed)
	}
}

func TestTUIGroupKey_Undone(t *testing.T) {
	it := &wn.Item{}
	if got := tuiGroupKey(it, false, time.Now()); got != tuiGroupUndone {
		t.Errorf("plain undone item got group %d, want %d (tuiGroupUndone)", got, tuiGroupUndone)
	}
}

func TestTUIGroupKey_Blocked(t *testing.T) {
	it := &wn.Item{}
	if got := tuiGroupKey(it, true, time.Now()); got != tuiGroupUndone {
		t.Errorf("blocked item got group %d, want %d (tuiGroupUndone)", got, tuiGroupUndone)
	}
}

func TestTUIGroupKey_Done(t *testing.T) {
	it := &wn.Item{Done: true, DoneStatus: wn.DoneStatusDone}
	if got := tuiGroupKey(it, false, time.Now()); got != tuiGroupDone {
		t.Errorf("done item got group %d, want %d (tuiGroupDone)", got, tuiGroupDone)
	}
}

func TestTUIGroupKey_PromptBeatsReview(t *testing.T) {
	// PromptReady and ReviewReady together → prompt wins (lower group number)
	it := &wn.Item{PromptReady: true, ReviewReady: true}
	if got := tuiGroupKey(it, false, time.Now()); got != tuiGroupPrompt {
		t.Errorf("PromptReady+ReviewReady item got group %d, want %d (tuiGroupPrompt)", got, tuiGroupPrompt)
	}
}

// --- group sort within applyFilter ---

func TestApplyFilter_GroupSortOrder(t *testing.T) {
	now := time.Now().UTC()
	undone := &wn.Item{ID: "u", Description: "undone task"}
	prompt := &wn.Item{ID: "p", Description: "prompt task", PromptReady: true}
	review := &wn.Item{ID: "rv", Description: "review task", ReviewReady: true}
	done := &wn.Item{ID: "d", Description: "done task", Done: true, DoneStatus: wn.DoneStatusDone}
	claimed := &wn.Item{ID: "c", Description: "claimed task", InProgressUntil: now.Add(time.Hour)}

	// Feed in reverse priority order; expect output in priority order.
	m := tuiModel{
		allItems:     []*wn.Item{done, undone, claimed, review, prompt},
		statusFilter: tuiFilterAll,
	}
	m.applyFilter()

	ids := tuiItemIDs(m.items)
	want := []string{"p", "rv", "c", "u", "d"}
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full order: %v)", i, ids[i], want[i], ids)
		}
	}
}

// --- buildRows ---

func TestBuildRows_HasGroupHeaders(t *testing.T) {
	now := time.Now().UTC()
	m := tuiModel{
		items: []*wn.Item{
			{ID: "p", PromptReady: true},
			{ID: "u"},
		},
		allItems: []*wn.Item{
			{ID: "p", PromptReady: true},
			{ID: "u"},
		},
	}
	m.buildRows()

	if len(m.rows) < 4 {
		t.Fatalf("expected at least 4 rows (2 headers + 2 items), got %d", len(m.rows))
	}
	// First row should be the prompt group header
	if m.rows[0].header == "" {
		t.Error("expected first row to be a group header")
	}
	// Second row should be item "p"
	if m.rows[1].itemIdx != 0 {
		t.Errorf("expected row 1 to be item[0] (p), got itemIdx=%d", m.rows[1].itemIdx)
	}
	_ = now // prevent unused import
}

func TestBuildRows_EmptyItems(t *testing.T) {
	m := tuiModel{}
	m.buildRows()
	if len(m.rows) != 0 {
		t.Errorf("expected empty rows for empty items, got %d", len(m.rows))
	}
}

func TestBuildRows_SameGroupNoExtraHeader(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{
			{ID: "a"},
			{ID: "b"},
		},
	}
	m.buildRows()
	// 1 header + 2 items = 3 rows
	if len(m.rows) != 3 {
		t.Errorf("expected 3 rows (1 header + 2 items), got %d: %v", len(m.rows), m.rows)
	}
}

// --- auto-refresh ---

func TestTUIRefreshInterval_IsReasonable(t *testing.T) {
	if tuiRefreshInterval < 5*time.Second || tuiRefreshInterval > 60*time.Second {
		t.Errorf("tuiRefreshInterval %v is outside reasonable range [5s, 60s]", tuiRefreshInterval)
	}
}

func TestUpdate_WatchMsgReturnsCmd(t *testing.T) {
	root := t.TempDir()
	if err := wn.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	m := tuiModel{store: store, root: root, width: 80, height: 24}
	_, cmd := m.Update(tuiWatchMsg{})
	if cmd == nil {
		t.Error("tuiWatchMsg should return a non-nil cmd to schedule reload and next watch")
	}
}

func TestHandleKey_R_rejectsNonPromptItem(t *testing.T) {
	root := t.TempDir()
	if err := wn.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	now := time.Now().UTC()
	item := &wn.Item{
		ID: "notpmt01", Description: "regular item",
		Created: now, Updated: now,
		Log: []wn.LogEntry{{At: now, Kind: "created"}},
	}
	if err := store.Put(item); err != nil {
		t.Fatalf("Put: %v", err)
	}

	m := tuiModel{
		store:    store,
		root:     root,
		width:    80,
		height:   24,
		allItems: []*wn.Item{item},
		items:    []*wn.Item{item},
	}
	m.buildRows()
	// rows: [header(Undone), item]; cursor on item row (index 1)
	m.cursor = 1
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if result.err == nil {
		t.Error("expected error when pressing 'r' on non-prompt item")
	}
}

// --- group-by mode ---

func TestBuildRows_GroupModeNone_NoHeaders(t *testing.T) {
	m := tuiModel{
		items:     []*wn.Item{{ID: "a"}, {ID: "b"}},
		groupMode: tuiGroupModeNone,
	}
	m.buildRows()
	for _, row := range m.rows {
		if row.header != "" {
			t.Errorf("expected no header rows in 'none' group mode, got header %q", row.header)
		}
	}
	if len(m.rows) != 2 {
		t.Errorf("expected 2 item rows in 'none' mode, got %d", len(m.rows))
	}
}

func TestBuildRows_GroupModeTags_HeadersPerTag(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{
			{ID: "a", Tags: []string{"backend"}},
			{ID: "b", Tags: []string{"frontend"}},
			{ID: "c", Tags: []string{"backend", "auth"}}, // first tag = backend → same group as a
		},
		groupMode: tuiGroupModeTags,
	}
	m.buildRows()
	headers := 0
	for _, row := range m.rows {
		if row.header != "" {
			headers++
		}
	}
	if headers != 2 {
		t.Errorf("expected 2 tag group headers (backend, frontend), got %d", headers)
	}
}

func TestBuildRows_GroupModeTags_NoTagsItem(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{
			{ID: "a", Tags: []string{"backend"}},
			{ID: "b"}, // no tags
		},
		groupMode: tuiGroupModeTags,
	}
	m.buildRows()
	headers := 0
	noTagsFound := false
	for _, row := range m.rows {
		if row.header != "" {
			headers++
			if strings.Contains(row.header, "no tags") {
				noTagsFound = true
			}
		}
	}
	if headers != 2 {
		t.Errorf("expected 2 headers (backend, no tags), got %d", headers)
	}
	if !noTagsFound {
		t.Error("expected a '(no tags)' group header for untagged items")
	}
}

// --- collapsible groups ---

func TestBuildRows_HeaderContainsCount(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{{ID: "a"}, {ID: "b"}},
	}
	m.buildRows()
	found := false
	for _, row := range m.rows {
		if row.header != "" && strings.Contains(row.header, "(2)") {
			found = true
		}
	}
	if !found {
		t.Error("group header should contain item count like '(2)'")
	}
}

func TestBuildRows_CollapseIndicatorExpanded(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{{ID: "a"}},
	}
	m.buildRows()
	for _, row := range m.rows {
		if row.header != "" && !strings.Contains(row.header, "▼") {
			t.Errorf("expanded group header should contain ▼, got %q", row.header)
		}
	}
}

func TestBuildRows_CollapseIndicatorCollapsed(t *testing.T) {
	m := tuiModel{
		items:           []*wn.Item{{ID: "a"}},
		collapsedGroups: map[string]bool{"3": true}, // tuiGroupUndone = 3
	}
	m.buildRows()
	allCollapsed := true
	for _, row := range m.rows {
		if row.header != "" && !strings.Contains(row.header, "▶") {
			allCollapsed = false
		}
	}
	if !allCollapsed {
		t.Error("collapsed group header should contain ▶")
	}
}

func TestBuildRows_CollapsedGroupHidesItems(t *testing.T) {
	m := tuiModel{
		items:           []*wn.Item{{ID: "a"}, {ID: "b"}},
		collapsedGroups: map[string]bool{"3": true}, // tuiGroupUndone = 3 (both items)
	}
	m.buildRows()
	itemRows := 0
	for _, row := range m.rows {
		if row.itemIdx >= 0 {
			itemRows++
		}
	}
	if itemRows != 0 {
		t.Errorf("collapsed group should hide items, got %d item rows", itemRows)
	}
}

func TestBuildRows_GroupKeySetOnHeader(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{{ID: "a"}},
	}
	m.buildRows()
	for _, row := range m.rows {
		if row.header != "" && row.groupKey == "" {
			t.Error("header row should have a non-empty groupKey for collapse lookup")
		}
	}
}

// --- cursor model: selected() on header row ---

func TestSelected_OnHeaderRow_ReturnsNil(t *testing.T) {
	m := tuiModel{
		items: []*wn.Item{{ID: "a"}},
		rows: []tuiListRow{
			{header: "▼ Undone (1)", groupKey: "3", itemIdx: -1},
			{itemIdx: 0},
		},
		cursor: 0, // cursor is on the header row
	}
	if it := m.selected(); it != nil {
		t.Errorf("selected() should return nil when cursor is on header row, got %v", it.ID)
	}
}

func TestSelected_OnItemRow_ReturnsItem(t *testing.T) {
	item := &wn.Item{ID: "a"}
	m := tuiModel{
		items: []*wn.Item{item},
		rows: []tuiListRow{
			{header: "▼ Undone (1)", groupKey: "3", itemIdx: -1},
			{itemIdx: 0},
		},
		cursor: 1, // cursor is on the item row
	}
	if got := m.selected(); got == nil || got.ID != "a" {
		t.Errorf("selected() should return item 'a' when cursor is on item row, got %v", got)
	}
}

// --- space bar: collapse group ---

func TestHandleKey_Space_OnHeader_TogglesCollapse(t *testing.T) {
	m := tuiModel{
		width:  80,
		height: 24,
		items:  []*wn.Item{{ID: "a"}},
		rows: []tuiListRow{
			{header: "▼ Undone (1)", groupKey: "3", itemIdx: -1},
			{itemIdx: 0},
		},
		cursor: 0, // on header
	}
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !result.collapsedGroups["3"] {
		t.Error("space on header row should collapse the group")
	}
}

func TestHandleKey_Space_OnItem_CollapsesGroup(t *testing.T) {
	m := tuiModel{
		width:  80,
		height: 24,
		items:  []*wn.Item{{ID: "a"}},
		rows: []tuiListRow{
			{header: "▼ Undone (1)", groupKey: "3", itemIdx: -1},
			{itemIdx: 0},
		},
		cursor: 1, // on item
	}
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !result.collapsedGroups["3"] {
		t.Error("space on item row should collapse the item's group")
	}
	// After collapse, cursor should be on the header row.
	if len(result.rows) == 0 || result.rows[result.cursor].header == "" {
		t.Error("after collapsing, cursor should be on the header row")
	}
}

// --- g key: cycle group mode ---

func TestHandleKey_G_CyclesGroupMode_StatusToTags(t *testing.T) {
	m := tuiModel{width: 80, height: 24, groupMode: tuiGroupModeStatus}
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if result.groupMode != tuiGroupModeTags {
		t.Errorf("g from status mode should cycle to tags, got %q", result.groupMode)
	}
}

func TestHandleKey_G_CyclesGroupMode_TagsToNone(t *testing.T) {
	m := tuiModel{width: 80, height: 24, groupMode: tuiGroupModeTags}
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if result.groupMode != tuiGroupModeNone {
		t.Errorf("g from tags mode should cycle to none, got %q", result.groupMode)
	}
}

func TestHandleKey_G_CyclesGroupMode_NoneToStatus(t *testing.T) {
	m := tuiModel{width: 80, height: 24, groupMode: tuiGroupModeNone}
	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if result.groupMode != tuiGroupModeStatus {
		t.Errorf("g from none mode should cycle to status, got %q", result.groupMode)
	}
}

// --- state persistence ---

func TestSaveTUIState_CreatesFile(t *testing.T) {
	root := t.TempDir()
	if err := wn.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	m := tuiModel{
		statusFilter: tuiFilterActive,
		groupMode:    tuiGroupModeStatus,
	}
	saveTUIState(root, m)
	path := filepath.Join(root, ".wn", "tui-state.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected state file at %s, got: %v", path, err)
	}
}

func TestLoadTUIState_RestoresState(t *testing.T) {
	root := t.TempDir()
	if err := wn.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	m := tuiModel{
		statusFilter:    tuiFilterDone,
		groupMode:       tuiGroupModeTags,
		collapsedGroups: map[string]bool{"3": true},
	}
	saveTUIState(root, m)
	state := loadTUIState(root)
	if state.StatusFilter != tuiFilterDone {
		t.Errorf("expected StatusFilter=%q, got %q", tuiFilterDone, state.StatusFilter)
	}
	if state.GroupMode != tuiGroupModeTags {
		t.Errorf("expected GroupMode=%q, got %q", tuiGroupModeTags, state.GroupMode)
	}
	if len(state.CollapsedGroups) != 1 || state.CollapsedGroups[0] != "3" {
		t.Errorf("expected CollapsedGroups=[\"3\"], got %v", state.CollapsedGroups)
	}
}

func TestLoadTUIState_MissingFile_ReturnsDefaults(t *testing.T) {
	root := t.TempDir()
	state := loadTUIState(root)
	if state.StatusFilter != tuiFilterActive {
		t.Errorf("expected default StatusFilter=%q, got %q", tuiFilterActive, state.StatusFilter)
	}
	if state.GroupMode != tuiGroupModeStatus {
		t.Errorf("expected default GroupMode=%q, got %q", tuiGroupModeStatus, state.GroupMode)
	}
}

// --- filesystem watcher ---

func TestTUIStartWatcher_ValidDir_ReturnsChannel(t *testing.T) {
	root := t.TempDir()
	itemsDir := filepath.Join(root, ".wn", "items")
	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ch := tuiStartWatcher(itemsDir)
	if ch == nil {
		t.Error("expected non-nil channel for valid directory")
	}
}

func TestTUIStartWatcher_InvalidDir_ReturnsNil(t *testing.T) {
	ch := tuiStartWatcher("/nonexistent/path/that/definitely/does/not/exist")
	if ch != nil {
		t.Error("expected nil channel for non-existent directory")
	}
}

func TestTUIStartWatcher_DetectsFileCreation(t *testing.T) {
	root := t.TempDir()
	itemsDir := filepath.Join(root, ".wn", "items")
	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ch := tuiStartWatcher(itemsDir)
	if ch == nil {
		t.Skip("filesystem watching not available on this platform")
	}

	// Give the watcher goroutine time to start.
	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(itemsDir, "abc123.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case <-ch:
		// success: watcher detected the change
	case <-time.After(2 * time.Second):
		t.Error("expected filesystem event within 2 seconds after file creation")
	}
}

func TestTUIWatchCmd_NilChannel_ReturnsCmd(t *testing.T) {
	cmd := tuiWatchCmd(nil)
	if cmd == nil {
		t.Error("tuiWatchCmd(nil) should return a non-nil fallback polling cmd")
	}
}

func TestTUIUpdate_WatchMsgTriggersReload(t *testing.T) {
	root := t.TempDir()
	if err := wn.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	m := tuiModel{store: store, root: root, width: 80, height: 24}
	_, cmd := m.Update(tuiWatchMsg{})
	if cmd == nil {
		t.Error("tuiWatchMsg should return a non-nil cmd to schedule reload and reschedule watch")
	}
}
