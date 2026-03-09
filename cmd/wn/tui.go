package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/keith/wn/internal/wn"
	"github.com/spf13/cobra"
)

const tuiLeftWidth = 40

var (
	styleCursor       = lipgloss.NewStyle().Background(lipgloss.Color("99")).Foreground(lipgloss.Color("230")).Bold(true)
	styleDone         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleInProgress   = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	styleDivider      = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	styleHeader       = lipgloss.NewStyle().Bold(true)
	styleKey          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	styleFooter       = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	styleErrMsg       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleTag          = lipgloss.NewStyle().Foreground(lipgloss.Color("32"))
	styleCurrent      = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true) // gold star
	styleFilterActive = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)  // green badge
	styleFilterReview = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)  // blue badge
	styleFilterDone   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true) // gray badge
)

// statusFilter values
const (
	tuiFilterAll    = ""
	tuiFilterActive = "active"
	tuiFilterReview = "review"
	tuiFilterDone   = "done"
)

type tuiEditorAction int

const (
	tuiEditorAdd tuiEditorAction = iota + 1
	tuiEditorEdit
)

type tuiLoadedMsg []*wn.Item
type tuiEditorMsg struct {
	action  tuiEditorAction
	tmpFile string
	id      string
	err     error
}
type tuiErrMsg struct{ err error }

type tuiModel struct {
	store    wn.Store
	root     string
	settings wn.Settings

	allItems   []*wn.Item
	items      []*wn.Item
	cursor     int
	listOffset int
	currentID  string // ID of the active work item (from meta)

	vp      viewport.Model
	vpReady bool

	filterMode   bool
	filterText   string
	statusFilter string // tuiFilterAll / tuiFilterActive / tuiFilterReview / tuiFilterDone

	width  int
	height int

	msg string
	err error
}

func newTUI(store wn.Store, root string, settings wn.Settings, currentID string) tuiModel {
	return tuiModel{store: store, root: root, settings: settings, currentID: currentID}
}

func (m tuiModel) Init() tea.Cmd {
	return m.cmdLoad()
}

func (m tuiModel) cmdLoad() tea.Cmd {
	return func() tea.Msg {
		items, err := m.store.List()
		if err != nil {
			return tuiErrMsg{err}
		}
		spec := wn.SortSpecFromSettings(m.settings)
		return tuiLoadedMsg(wn.ApplySort(items, spec))
	}
}

func (m *tuiModel) applyFilter() {
	var out []*wn.Item

	// Determine text search mode: "#tag" prefix = tag-only match.
	tagOnly := strings.HasPrefix(m.filterText, "#")
	search := strings.ToLower(strings.TrimPrefix(m.filterText, "#"))

	for _, it := range m.allItems {
		// Status filter
		switch m.statusFilter {
		case tuiFilterActive:
			if it.Done {
				continue
			}
		case tuiFilterReview:
			if !it.ReviewReady {
				continue
			}
		case tuiFilterDone:
			if !it.Done {
				continue
			}
		}

		// Text / tag filter
		if search != "" {
			if tagOnly {
				matched := false
				for _, t := range it.Tags {
					if strings.Contains(strings.ToLower(t), search) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			} else {
				matchDesc := strings.Contains(strings.ToLower(it.Description), search)
				matchTag := false
				for _, t := range it.Tags {
					if strings.Contains(strings.ToLower(t), search) {
						matchTag = true
						break
					}
				}
				if !matchDesc && !matchTag {
					continue
				}
			}
		}
		out = append(out, it)
	}
	m.items = out
}

func (m *tuiModel) clampCursor() {
	n := len(m.items)
	if n == 0 {
		m.cursor = 0
		m.listOffset = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	bh := m.bodyHeight()
	if m.cursor < m.listOffset {
		m.listOffset = m.cursor
	}
	if m.cursor >= m.listOffset+bh {
		m.listOffset = m.cursor - bh + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}

func (m tuiModel) bodyHeight() int {
	h := m.height - 2 // header row + footer row
	if h < 1 {
		return 1
	}
	return h
}

func (m tuiModel) selected() *wn.Item {
	if len(m.items) == 0 || m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return m.items[m.cursor]
}

func (m *tuiModel) refreshViewport() {
	if !m.vpReady {
		return
	}
	it := m.selected()
	if it == nil {
		m.vp.SetContent("(no items)")
		return
	}
	m.vp.SetContent(tuiItemDetail(it, m.store))
	m.vp.GotoTop()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		dw := msg.Width - tuiLeftWidth - 1
		if dw < 10 {
			dw = 10
		}
		bh := m.bodyHeight()
		if !m.vpReady {
			m.vp = viewport.New(dw, bh)
			m.vpReady = true
		} else {
			m.vp.Width = dw
			m.vp.Height = bh
		}
		m.refreshViewport()
		return m, nil

	case tuiLoadedMsg:
		m.allItems = []*wn.Item(msg)
		m.applyFilter()
		m.clampCursor()
		m.refreshViewport()
		return m, nil

	case tuiErrMsg:
		m.err = msg.err
		return m, nil

	case tuiEditorMsg:
		return m.handleEditor(msg)

	case tuiLaunchMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.msg = "launched: " + msg.id
		}
		return m, nil

	case tea.KeyMsg:
		if m.filterMode {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)
	}

	if m.vpReady {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	m.msg = ""
	m.err = nil
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.clampCursor()
			m.refreshViewport()
		}

	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
			m.clampCursor()
			m.refreshViewport()
		}

	case "pgup", "pgdown":
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case "enter":
		it := m.selected()
		if it != nil {
			if err := wn.WithMetaLock(m.root, func(meta wn.Meta) (wn.Meta, error) {
				meta.CurrentID = it.ID
				return meta, nil
			}); err != nil {
				m.err = err
			} else {
				m.currentID = it.ID
				m.msg = "current: " + it.ID
			}
		}

	case "a":
		return m.openEditor(tuiEditorAdd, "")

	case "e":
		if it := m.selected(); it != nil {
			return m.openEditor(tuiEditorEdit, it.ID)
		}

	case "x":
		if it := m.selected(); it != nil {
			if err := wn.SetStatus(m.store, it.ID, wn.StatusDone, wn.StatusOpts{}); err != nil {
				m.err = err
			} else {
				m.msg = "done: " + it.ID
				return m, m.cmdLoad()
			}
		}

	case "-":
		if it := m.selected(); it != nil {
			if err := wn.SetStatus(m.store, it.ID, wn.StatusSuspend, wn.StatusOpts{}); err != nil {
				m.err = err
			} else {
				m.msg = "suspended: " + it.ID
				return m, m.cmdLoad()
			}
		}

	case "u":
		if it := m.selected(); it != nil {
			if err := wn.SetStatus(m.store, it.ID, wn.StatusUndone, wn.StatusOpts{}); err != nil {
				m.err = err
			} else {
				m.msg = "undone: " + it.ID
				return m, m.cmdLoad()
			}
		}

	case "D":
		if it := m.selected(); it != nil {
			id := it.ID
			if err := m.store.Delete(id); err != nil {
				m.err = err
			} else {
				_ = wn.WithMetaLock(m.root, func(meta wn.Meta) (wn.Meta, error) {
					if meta.CurrentID == id {
						meta.CurrentID = ""
					}
					return meta, nil
				})
				m.msg = "deleted: " + id
				return m, m.cmdLoad()
			}
		}

	case "/":
		m.filterMode = true
		m.filterText = ""

	case "#":
		m.filterMode = true
		m.filterText = "#"

	case "f":
		switch m.statusFilter {
		case tuiFilterAll:
			m.statusFilter = tuiFilterActive
		case tuiFilterActive:
			m.statusFilter = tuiFilterReview
		case tuiFilterReview:
			m.statusFilter = tuiFilterDone
		default:
			m.statusFilter = tuiFilterAll
		}
		m.applyFilter()
		m.cursor = 0
		m.listOffset = 0
		m.refreshViewport()

	case "esc":
		m.filterMode = false
		m.filterText = ""
		m.statusFilter = tuiFilterAll
		m.applyFilter()
		m.clampCursor()
		m.refreshViewport()

	case ">":
		if it := m.selected(); it != nil {
			return m, m.cmdLaunch(it.ID)
		}
	}
	return m, nil
}

type tuiLaunchMsg struct {
	id  string
	err error
}

func (m tuiModel) cmdLaunch(id string) tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			exe = "wn"
		}
		cmd := exec.Command(exe, "launch", id)
		err = cmd.Run()
		return tuiLaunchMsg{id: id, err: err}
	}
}

func (m tuiModel) handleFilterKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterMode = false
		m.filterText = ""
		m.applyFilter()
		m.clampCursor()
		m.refreshViewport()
	case "enter":
		m.filterMode = false
		m.clampCursor()
		m.refreshViewport()
	case "backspace", "ctrl+h":
		runes := []rune(m.filterText)
		if len(runes) > 0 {
			m.filterText = string(runes[:len(runes)-1])
			m.applyFilter()
			m.cursor = 0
			m.listOffset = 0
			m.refreshViewport()
		}
	default:
		if len(msg.Runes) > 0 {
			m.filterText += string(msg.Runes)
			m.applyFilter()
			m.cursor = 0
			m.listOffset = 0
			m.refreshViewport()
		}
	}
	return m, nil
}

func (m tuiModel) openEditor(action tuiEditorAction, id string) (tuiModel, tea.Cmd) {
	initial := ""
	if action == tuiEditorEdit && id != "" {
		if it, err := m.store.Get(id); err == nil {
			initial = it.Description
		}
	}
	f, err := os.CreateTemp("", "wn-tui-*.txt")
	if err != nil {
		m.err = err
		return m, nil
	}
	tmpFile := f.Name()
	if initial != "" {
		_, _ = f.WriteString(initial)
	}
	_ = f.Close()

	edStr := strings.TrimSpace(os.Getenv("EDITOR"))
	if edStr == "" {
		_ = os.Remove(tmpFile)
		m.err = wn.ErrEditorUnset
		return m, nil
	}
	parts := tuiSplitArgs(edStr)
	args := append(parts[1:], tmpFile)
	cmd := exec.Command(parts[0], args...)
	return m, tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return tuiEditorMsg{action: action, tmpFile: tmpFile, id: id, err: execErr}
	})
}

func (m tuiModel) handleEditor(msg tuiEditorMsg) (tuiModel, tea.Cmd) {
	defer os.Remove(msg.tmpFile)
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	data, err := os.ReadFile(msg.tmpFile)
	if err != nil {
		m.err = err
		return m, nil
	}
	content := strings.TrimSuffix(string(data), "\n")
	if strings.TrimSpace(content) == "" {
		m.msg = "cancelled (empty)"
		return m, nil
	}
	switch msg.action {
	case tuiEditorAdd:
		id, err := wn.GenerateID(m.store)
		if err != nil {
			m.err = err
			return m, nil
		}
		now := time.Now().UTC()
		item := &wn.Item{
			ID:          id,
			Description: content,
			Created:     now,
			Updated:     now,
			Log:         []wn.LogEntry{{At: now, Kind: "created"}},
		}
		if err := m.store.Put(item); err != nil {
			m.err = err
			return m, nil
		}
		_ = wn.WithMetaLock(m.root, func(meta wn.Meta) (wn.Meta, error) {
			meta.CurrentID = id
			return meta, nil
		})
		m.currentID = id
		m.msg = "added: " + id
	case tuiEditorEdit:
		err := m.store.UpdateItem(msg.id, func(it *wn.Item) (*wn.Item, error) {
			it.Description = content
			it.Updated = time.Now().UTC()
			it.Log = append(it.Log, wn.LogEntry{At: it.Updated, Kind: "updated"})
			return it, nil
		})
		if err != nil {
			m.err = err
			return m, nil
		}
		m.msg = "updated: " + msg.id
	}
	return m, m.cmdLoad()
}

func (m tuiModel) View() string {
	if !m.vpReady {
		return "Loading...\n"
	}
	bh := m.bodyHeight()

	// Header
	it := m.selected()
	leftHdr := fmt.Sprintf(" Items (%d)", len(m.items))
	// Status filter badge
	switch m.statusFilter {
	case tuiFilterActive:
		leftHdr += " " + styleFilterActive.Render("[active]")
	case tuiFilterReview:
		leftHdr += " " + styleFilterReview.Render("[review]")
	case tuiFilterDone:
		leftHdr += " " + styleFilterDone.Render("[done]")
	}
	// Text filter badge
	if m.filterText != "" || m.filterMode {
		var badge string
		if m.filterMode {
			badge = fmt.Sprintf("[%s_]", m.filterText)
		} else {
			badge = fmt.Sprintf("[%s]", m.filterText)
		}
		leftHdr += " " + styleFilterActive.Render(badge)
	}
	rightHdr := ""
	if it != nil {
		rightHdr = " " + it.ID
		if it.ID == m.currentID {
			rightHdr += " " + styleCurrent.Render("★ current")
		}
	}
	header := styleHeader.Width(tuiLeftWidth).Render(leftHdr) +
		styleDivider.Render("│") +
		styleHeader.Width(m.vp.Width).Render(rightHdr)

	// List column
	listLines := m.renderList(bh)
	listStr := strings.Join(listLines, "\n")

	// Divider column
	divLines := make([]string, bh)
	for i := range divLines {
		divLines[i] = styleDivider.Render("│")
	}
	divStr := strings.Join(divLines, "\n")

	// Detail column (viewport)
	leftCol := lipgloss.NewStyle().Width(tuiLeftWidth).Height(bh).Render(listStr)
	rightCol := lipgloss.NewStyle().Width(m.vp.Width).Height(bh).Render(m.vp.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divStr, rightCol)

	return header + "\n" + body + "\n" + m.renderFooter()
}

func (m tuiModel) renderList(height int) []string {
	lines := make([]string, height)
	for i := range lines {
		idx := m.listOffset + i
		if idx >= len(m.items) {
			lines[i] = lipgloss.NewStyle().Width(tuiLeftWidth).Render("")
			continue
		}
		lines[i] = m.renderRow(m.items[idx], idx == m.cursor)
	}
	return lines
}

func (m tuiModel) renderRow(it *wn.Item, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	// Current item star (gold ★ if this is the active work item)
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
	case wn.IsInProgress(it, time.Now().UTC()):
		indicator = "●"
	}

	tagPart := ""
	if len(it.Tags) > 0 {
		tagPart = " " + styleTag.Render("#"+strings.Join(it.Tags, " #"))
	}

	desc := wn.FirstLine(it.Description)
	// available = leftWidth minus cursor(2) + star(1) + indicator(1) + space(1) + tags + margin(1)
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
	case wn.IsInProgress(it, time.Now().UTC()):
		return styleInProgress.Width(tuiLeftWidth).Render(line)
	default:
		return lipgloss.NewStyle().Width(tuiLeftWidth).Render(line)
	}
}

func (m tuiModel) renderFooter() string {
	if m.err != nil {
		return styleErrMsg.Render(" error: " + m.err.Error())
	}
	if m.filterMode {
		return styleFooter.Render(fmt.Sprintf(" filter: /%s_  (Enter to confirm, Esc to cancel)", m.filterText))
	}
	if m.msg != "" {
		return styleFooter.Render(" " + m.msg)
	}
	return m.renderHints()
}

func (m tuiModel) renderHints() string {
	type hint struct{ k, d string }
	hints := []hint{
		{"a", "add"}, {"e", "edit"}, {"x", "done"},
		{"-", "suspend"}, {"u", "undone"}, {"D", "delete"},
		{"↵", "set current"}, {">", "launch"}, {"/", "search"}, {"#", "tag filter"},
		{"f", "cycle filter"}, {"PgUp/Dn", "scroll"}, {"q", "quit"},
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, styleKey.Render("["+h.k+"]")+styleFooter.Render(h.d))
	}
	return " " + strings.Join(parts, "  ")
}

// tuiItemDetail renders a work item as a multi-line string for the detail pane.
func tuiItemDetail(item *wn.Item, store wn.Store) string {
	const timeFmt = "2006-01-02 15:04"
	var b strings.Builder

	b.WriteString(item.Description)
	b.WriteString("\n")

	b.WriteString("\nstatus: ")
	b.WriteString(wn.ItemListStatus(item, time.Now().UTC()))
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
			b.WriteString(fmt.Sprintf("  %-20s  %s\n  %s\n", n.Name, n.Created.Format(timeFmt), n.Body))
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

// tuiSplitArgs splits an editor command string, handling simple quoting.
func tuiSplitArgs(s string) []string {
	var parts []string
	var b strings.Builder
	quote := false
	for _, r := range s {
		if r == '"' || r == '\'' {
			quote = !quote
			continue
		}
		if !quote && (r == ' ' || r == '\t') {
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for managing work items",
	Args:  cobra.NoArgs,
	RunE:  runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	settings, _ := wn.ReadSettingsInRoot(root)
	meta, _ := wn.ReadMeta(root)
	m := newTUI(store, root, settings, meta.CurrentID)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
