package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kjhaber/wn/internal/wn"
)

type tuiEditorAction int

const (
	tuiEditorAdd tuiEditorAction = iota + 1
	tuiEditorEdit
	tuiEditorRespond
)

type tuiLoadedMsg []*wn.Item
type tuiEditorMsg struct {
	action  tuiEditorAction
	tmpFile string
	id      string
	err     error
}
type tuiErrMsg struct{ err error }

type tuiLaunchMsg struct {
	id  string
	err error
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

	case tuiWatchMsg:
		return m, tea.Batch(m.cmdLoad(), tuiWatchCmd(m.watchCh))

	case tuiLoadedMsg:
		selectedID := ""
		if it := m.selected(); it != nil {
			selectedID = it.ID
		}
		m.allItems = []*wn.Item(msg)
		m.blockedSet = wn.BlockedSet(m.allItems)
		m.applyFilter()
		m.restoreCursor(selectedID)
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
		if m.cursor < len(m.rows)-1 {
			m.cursor++
			m.clampCursor()
			m.refreshViewport()
		}

	case "pgup", "pgdown":
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case "enter":
		// On a header row: expand if collapsed, no-op if already expanded.
		if len(m.rows) > 0 && m.cursor < len(m.rows) && m.rows[m.cursor].header != "" {
			key := m.rows[m.cursor].groupKey
			if m.collapsedGroups != nil && m.collapsedGroups[key] {
				delete(m.collapsedGroups, key)
				m.buildRows()
				m.clampCursor()
				m.refreshViewport()
			}
			break
		}
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

	case " ":
		if len(m.rows) > 0 && m.cursor < len(m.rows) {
			row := m.rows[m.cursor]
			if row.header != "" {
				m.toggleGroupCollapse(row.groupKey)
			} else {
				m.collapseItemGroup()
			}
			m.refreshViewport()
		}

	case "g":
		selectedID := ""
		if it := m.selected(); it != nil {
			selectedID = it.ID
		}
		switch m.groupMode {
		case "", tuiGroupModeStatus:
			m.groupMode = tuiGroupModeTags
		case tuiGroupModeTags:
			m.groupMode = tuiGroupModeNone
		default:
			m.groupMode = tuiGroupModeStatus
		}
		m.applyFilter()
		m.restoreCursor(selectedID)
		m.refreshViewport()

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

	case "?":
		m.showHelp = !m.showHelp
		m.refreshViewport()

	case "esc":
		m.filterMode = false
		m.filterText = ""
		m.statusFilter = tuiFilterActive
		m.groupMode = tuiGroupModeStatus
		m.collapsedGroups = nil
		m.showHelp = false
		m.applyFilter()
		m.cursor = 0
		m.listOffset = 0
		m.refreshViewport()

	case "r":
		it := m.selected()
		if it == nil || !it.PromptReady {
			m.err = fmt.Errorf("selected item is not awaiting a response")
			return m, nil
		}
		return m.openEditor(tuiEditorRespond, it.ID)

	case ">":
		if it := m.selected(); it != nil {
			return m, m.cmdLaunch(it.ID)
		}
	}
	return m, nil
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
	defer func() { _ = os.Remove(msg.tmpFile) }()
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

	case tuiEditorRespond:
		now := time.Now().UTC()
		trimmed := strings.TrimSpace(content)
		err := m.store.UpdateItem(msg.id, func(it *wn.Item) (*wn.Item, error) {
			it.Done = true
			it.DoneStatus = wn.DoneStatusDone
			it.PromptReady = false
			it.Updated = now
			it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: trimmed})
			if it.Notes == nil {
				it.Notes = []wn.Note{}
			}
			idx := it.NoteIndexByName(wn.NoteNameResponse)
			if idx >= 0 {
				it.Notes[idx].Body = trimmed
			} else {
				it.Notes = append(it.Notes, wn.Note{Name: wn.NoteNameResponse, Created: now, Body: trimmed})
			}
			return it, nil
		})
		if err != nil {
			m.err = err
			return m, nil
		}
		m.msg = "responded: " + msg.id
	}
	return m, m.cmdLoad()
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
