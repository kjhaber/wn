package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kjhaber/wn/internal/wn"
)

// tuiHeaderLayout is precomputed header content for the list/detail split (tuiRenderHeader applies lipgloss).
type tuiHeaderLayout struct {
	itemCount    int
	statusFilter string
	groupMode    string
	filterText   string
	filterMode   bool
	rightItemID  string
	showCurrent  bool
}

// tuiVisibleListRow is one row in the list viewport after scrolling (View applies lipgloss).
type tuiVisibleListRow struct {
	groupHeader string   // non-empty → group header row
	item        *wn.Item // nil when groupHeader is set
	cursorHere  bool
}

// tuiFooterLayout selects which footer line View renders.
type tuiFooterLayout struct {
	kind       string // "error", "filter", "msg", "hints", "help_hints"
	err        error
	filterText string
	msg        string
	showHelp   bool
}

// tuiViewLayout is the full frame layout for one View() call (pure data + lipgloss-free strings).
type tuiViewLayout struct {
	header       tuiHeaderLayout
	listRows     []tuiVisibleListRow
	bodyHeight   int
	detailVPView string // pre-rendered viewport string (bubbletea viewport output)
}

func tuiComputeHeaderLayout(m tuiModel) tuiHeaderLayout {
	it := m.selected()
	h := tuiHeaderLayout{
		itemCount:    len(m.items),
		statusFilter: m.statusFilter,
		groupMode:    m.groupMode,
		filterText:   m.filterText,
		filterMode:   m.filterMode,
	}
	if it != nil {
		h.rightItemID = it.ID
		h.showCurrent = it.ID == m.currentID
	}
	return h
}

func tuiComputeFooterLayout(m tuiModel) tuiFooterLayout {
	if m.err != nil {
		return tuiFooterLayout{kind: "error", err: m.err}
	}
	if m.filterMode {
		return tuiFooterLayout{kind: "filter", filterText: m.filterText}
	}
	if m.msg != "" {
		return tuiFooterLayout{kind: "msg", msg: m.msg}
	}
	return tuiFooterLayout{kind: "hints", showHelp: m.showHelp}
}

// tuiComputeListPaneLayout builds visible list rows for the body viewport (pure from model + geometry).
func tuiComputeListPaneLayout(m tuiModel, bodyHeight int) []tuiVisibleListRow {
	out := make([]tuiVisibleListRow, bodyHeight)
	for i := range out {
		rowIdx := m.listOffset + i
		if rowIdx >= len(m.rows) {
			continue
		}
		row := m.rows[rowIdx]
		cursorHere := rowIdx == m.cursor
		if row.header != "" {
			out[i] = tuiVisibleListRow{groupHeader: row.header, cursorHere: cursorHere}
		} else {
			out[i] = tuiVisibleListRow{item: m.items[row.itemIdx], cursorHere: cursorHere}
		}
	}
	return out
}

// tuiComputeViewLayout derives the full frame from tuiModel (no lipgloss; detail pane uses viewport's last View()).
func tuiComputeViewLayout(m tuiModel) tuiViewLayout {
	bh := m.bodyHeight()
	return tuiViewLayout{
		header:       tuiComputeHeaderLayout(m),
		listRows:     tuiComputeListPaneLayout(m, bh),
		bodyHeight:   bh,
		detailVPView: m.vp.View(),
	}
}

func tuiRenderHeader(h tuiHeaderLayout, vpWidth int) string {
	leftHdr := fmt.Sprintf(" Items (%d)", h.itemCount)
	switch h.statusFilter {
	case tuiFilterActive:
		leftHdr += " " + styleFilterActive.Render("[active]")
	case tuiFilterReview:
		leftHdr += " " + styleFilterReview.Render("[review]")
	case tuiFilterDone:
		leftHdr += " " + styleFilterDone.Render("[done]")
	}
	switch h.groupMode {
	case tuiGroupModeTags:
		leftHdr += " " + styleGroupBadge.Render("[group:tags]")
	case tuiGroupModeNone:
		leftHdr += " " + styleGroupBadge.Render("[group:none]")
	}
	if h.filterText != "" || h.filterMode {
		var badge string
		if h.filterMode {
			badge = fmt.Sprintf("[%s_]", h.filterText)
		} else {
			badge = fmt.Sprintf("[%s]", h.filterText)
		}
		leftHdr += " " + styleFilterActive.Render(badge)
	}
	rightHdr := ""
	if h.rightItemID != "" {
		rightHdr = " " + h.rightItemID
		if h.showCurrent {
			rightHdr += " " + styleCurrent.Render("★ current")
		}
	}
	return styleHeader.Width(tuiLeftWidth).Render(leftHdr) +
		styleDivider.Render("│") +
		styleHeader.Width(vpWidth).Render(rightHdr)
}

func tuiRenderGroupHeader(label string, selected bool) string {
	if selected {
		return styleCursor.Width(tuiLeftWidth).Render(" " + label)
	}
	return styleGroupHeader.Width(tuiLeftWidth).Render(" " + label)
}

func tuiRenderItemRow(m tuiModel, it *wn.Item, selected bool, now time.Time) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	star := " "
	if it.ID == m.currentID {
		star = styleCurrent.Render("★")
	}

	indicator := " "
	switch {
	case it.Done && it.DoneStatus == wn.DoneStatusSuspend:
		indicator = "~"
	case it.Done:
		indicator = "✓"
	case wn.IsInProgress(it, now):
		indicator = "●"
	case m.blockedSet[it.ID]:
		indicator = "!"
	}

	tagPart := ""
	if len(it.Tags) > 0 {
		tagPart = " " + styleTag.Render("#"+strings.Join(it.Tags, " #"))
	}

	desc := wn.FirstLine(it.Description)
	tagLen := lipgloss.Width(tagPart)
	available := tuiLeftWidth - 6 - tagLen
	if available < 3 {
		available = 3
	}
	runes := []rune(desc)
	if len(runes) > available {
		desc = string(runes[:available-1]) + "…"
	}

	line := cursor + star + indicator + " " + desc + tagPart
	switch {
	case selected:
		return styleCursor.Width(tuiLeftWidth).Render(line)
	case it.Done:
		return styleDone.Width(tuiLeftWidth).Render(line)
	case wn.IsInProgress(it, now):
		return styleInProgress.Width(tuiLeftWidth).Render(line)
	default:
		return lipgloss.NewStyle().Width(tuiLeftWidth).Render(line)
	}
}

func tuiRenderListLines(m tuiModel, rows []tuiVisibleListRow, now time.Time) []string {
	lines := make([]string, len(rows))
	for i, vr := range rows {
		switch {
		case vr.groupHeader != "":
			lines[i] = tuiRenderGroupHeader(vr.groupHeader, vr.cursorHere)
		case vr.item != nil:
			lines[i] = tuiRenderItemRow(m, vr.item, vr.cursorHere, now)
		default:
			lines[i] = lipgloss.NewStyle().Width(tuiLeftWidth).Render("")
		}
	}
	return lines
}

func tuiRenderFooter(fl tuiFooterLayout) string {
	switch fl.kind {
	case "error":
		return styleErrMsg.Render(" error: " + fl.err.Error())
	case "filter":
		return styleFooter.Render(fmt.Sprintf(" filter: /%s_  (Enter to confirm, Esc to cancel)", fl.filterText))
	case "msg":
		return styleFooter.Render(" " + fl.msg)
	default:
		return tuiRenderHints(fl.showHelp)
	}
}

func tuiRenderHints(showHelp bool) string {
	if showHelp {
		return styleFooter.Render(" [?]close help  [q]quit")
	}
	type hint struct{ k, d string }
	hints := []hint{
		{"↵", "cur"}, {"a", "add"}, {"e", "edit"}, {"x", "done"},
		{"/", "search"}, {"f", "filt"}, {"g", "grp"}, {"q", "quit"}, {"?", "more"},
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, styleKey.Render("["+h.k+"]")+styleFooter.Render(h.d))
	}
	return " " + strings.Join(parts, "  ")
}

func (m tuiModel) View() string {
	if !m.vpReady {
		return "Loading...\n"
	}
	now := time.Now().UTC()
	layout := tuiComputeViewLayout(m)

	header := tuiRenderHeader(layout.header, m.vp.Width)
	listLines := tuiRenderListLines(m, layout.listRows, now)
	listStr := strings.Join(listLines, "\n")

	bh := layout.bodyHeight
	divLines := make([]string, bh)
	for i := range divLines {
		divLines[i] = styleDivider.Render("│")
	}
	divStr := strings.Join(divLines, "\n")

	leftCol := lipgloss.NewStyle().Width(tuiLeftWidth).Height(bh).Render(listStr)
	rightCol := lipgloss.NewStyle().Width(m.vp.Width).Height(bh).Render(layout.detailVPView)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divStr, rightCol)

	footer := tuiRenderFooter(tuiComputeFooterLayout(m))
	return header + "\n" + body + "\n" + footer
}

func (m tuiModel) renderHints() string {
	return tuiRenderHints(m.showHelp)
}

// tuiHelpContent returns a formatted string of all keyboard shortcuts for display in the help overlay.
func tuiHelpContent() string {
	var b strings.Builder
	b.WriteString("Keyboard Shortcuts\n\n")
	b.WriteString("Item Actions:\n")
	b.WriteString("  [a]  add new item          [e]  edit selected\n")
	b.WriteString("  [x]  mark done             [u]  mark undone\n")
	b.WriteString("  [-]  suspend               [D]  delete\n")
	b.WriteString("  [r]  respond to prompt     [↵]  set as current\n")
	b.WriteString("  [>]  launch in worktree\n")
	b.WriteString("\nNavigation & Filter:\n")
	b.WriteString("  [/]  search by text        [#]  search by tag\n")
	b.WriteString("  [f]  cycle status filter   [Esc]  reset to defaults\n")
	b.WriteString("  [↑/k]  move up             [↓/j]  move down\n")
	b.WriteString("  [PgUp/PgDn]  scroll detail pane\n")
	b.WriteString("\nGrouping:\n")
	b.WriteString("  [g]  cycle group mode (status → tags → none)\n")
	b.WriteString("  [Space]  collapse/expand group\n")
	b.WriteString("\n  [?]  toggle this help     [q]  quit\n")
	return b.String()
}

// wrapLines word-wraps text to fit within width columns, preserving explicit newlines.
// If width <= 0, text is returned unchanged.
func wrapLines(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		if len([]rune(line)) <= width {
			out = append(out, line)
			continue
		}
		words := strings.Fields(line)
		if len(words) == 0 {
			out = append(out, line)
			continue
		}
		var current strings.Builder
		for _, word := range words {
			wlen := len([]rune(word))
			if current.Len() == 0 {
				current.WriteString(word)
			} else if current.Len()+1+wlen <= width {
				current.WriteByte(' ')
				current.WriteString(word)
			} else {
				out = append(out, current.String())
				current.Reset()
				current.WriteString(word)
			}
		}
		if current.Len() > 0 {
			out = append(out, current.String())
		}
	}
	return strings.Join(out, "\n")
}

// tuiItemDetail renders a work item as a multi-line string for the detail pane.
// width is the column width for word-wrapping; pass 0 to disable wrapping.
func tuiItemDetail(item *wn.Item, blocked bool, store wn.Store, width int) string {
	const timeFmt = "2006-01-02 15:04"
	var b strings.Builder

	b.WriteString(wrapLines(item.Description, width))
	b.WriteString("\n")

	b.WriteString("\nstatus: ")
	b.WriteString(wn.ItemListStatus(item, time.Now().UTC(), blocked))
	b.WriteString("\n")

	if len(item.Tags) > 0 {
		b.WriteString("tags:   " + strings.Join(item.Tags, ", ") + "\n")
	}
	if len(item.DependsOn) > 0 {
		b.WriteString("deps:   " + strings.Join(item.DependsOn, ", ") + "\n")
	}
	if store != nil {
		if deps, err := wn.Dependents(store, item.ID); err == nil && len(deps) > 0 {
			b.WriteString("needed-by: " + strings.Join(deps, ", ") + "\n")
		}
	}

	if len(item.Notes) > 0 {
		b.WriteString("\nnotes:\n")
		for _, n := range item.Notes {
			// Wrap the note body; subtract 2 for the leading "  " indent.
			noteWidth := width - 2
			body := wrapLines(n.Body, noteWidth)
			// Re-indent wrapped lines.
			indentedBody := strings.ReplaceAll(body, "\n", "\n  ")
			_, _ = fmt.Fprintf(&b, "  %-20s  %s\n  %s\n", n.Name, n.Created.Format(timeFmt), indentedBody)
		}
	}

	if len(item.Log) > 0 {
		b.WriteString("\nlog:\n")
		for _, e := range item.Log {
			line := fmt.Sprintf("  %s  %s", e.At.Format(timeFmt), e.Kind)
			if e.Msg != "" {
				line += "  " + e.Msg
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}
