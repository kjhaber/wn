package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kjhaber/wn/internal/wn"
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
	styleGroupHeader  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true) // group section header
	styleGroupBadge   = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true) // group mode badge
)

// statusFilter values
const (
	tuiFilterAll    = ""
	tuiFilterActive = "active"
	tuiFilterReview = "review"
	tuiFilterDone   = "done"
)

// tuiGroup constants define the display priority order (lower = shown first).
const (
	tuiGroupPrompt  = 0 // PromptReady — needs human response
	tuiGroupReview  = 1 // ReviewReady — needs human review
	tuiGroupClaimed = 2 // In progress / claimed
	tuiGroupUndone  = 3 // Undone (available or blocked)
	tuiGroupDone    = 4 // Done / suspended / closed
)

// tuiGroupMode constants control how the list pane groups items.
const (
	tuiGroupModeStatus = "status" // group by item status (default)
	tuiGroupModeTags   = "tags"   // group by primary tag
	tuiGroupModeNone   = "none"   // flat list, no group headers
)

// tuiListRow is one rendered row in the list pane: either a group header or an item.
type tuiListRow struct {
	header   string // non-empty → group header row (item is nil)
	groupKey string // collapse/expand key; set for header rows
	itemIdx  int    // index into m.items; -1 for header rows
}

// tuiState holds TUI display state that is persisted across sessions.
type tuiState struct {
	StatusFilter    string   `json:"statusFilter"`
	GroupMode       string   `json:"groupMode"`
	CollapsedGroups []string `json:"collapsedGroups"`
}

const tuiStateFile = "tui-state.json"

// loadTUIState reads saved TUI state from .wn/tui-state.json.
// Returns defaults if the file is missing or invalid.
func loadTUIState(root string) tuiState {
	path := filepath.Join(root, ".wn", tuiStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return tuiState{StatusFilter: tuiFilterActive, GroupMode: tuiGroupModeStatus}
	}
	var s tuiState
	if err := json.Unmarshal(data, &s); err != nil {
		return tuiState{StatusFilter: tuiFilterActive, GroupMode: tuiGroupModeStatus}
	}
	if s.StatusFilter == "" {
		s.StatusFilter = tuiFilterActive
	}
	if s.GroupMode == "" {
		s.GroupMode = tuiGroupModeStatus
	}
	return s
}

// saveTUIState writes the current TUI state to .wn/tui-state.json.
func saveTUIState(root string, m tuiModel) {
	collapsed := make([]string, 0, len(m.collapsedGroups))
	for k := range m.collapsedGroups {
		collapsed = append(collapsed, k)
	}
	sort.Strings(collapsed)
	s := tuiState{
		StatusFilter:    m.statusFilter,
		GroupMode:       m.groupMode,
		CollapsedGroups: collapsed,
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	path := filepath.Join(root, ".wn", tuiStateFile)
	_ = os.WriteFile(path, data, 0o644)
}

type tuiModel struct {
	store    wn.Store
	root     string
	settings wn.Settings

	allItems   []*wn.Item
	blockedSet map[string]bool
	items      []*wn.Item
	rows       []tuiListRow // flat list of group headers + item rows for rendering
	cursor     int
	listOffset int
	currentID  string // ID of the active work item (from meta)

	vp      viewport.Model
	vpReady bool

	filterMode   bool
	filterText   string
	statusFilter string // tuiFilterAll / tuiFilterActive / tuiFilterReview / tuiFilterDone

	groupMode       string          // tuiGroupModeStatus / tuiGroupModeTags / tuiGroupModeNone
	collapsedGroups map[string]bool // group keys that are currently collapsed

	width  int
	height int

	showHelp bool

	watchCh <-chan struct{} // non-nil when fsnotify watcher is active; nil falls back to polling

	msg string
	err error
}

func newTUI(store wn.Store, root string, settings wn.Settings, currentID string, stateArgs ...tuiState) tuiModel {
	groupMode := tuiGroupModeStatus
	statusFilter := tuiFilterActive
	var collapsedGroups map[string]bool
	if len(stateArgs) > 0 {
		state := stateArgs[0]
		if state.GroupMode != "" {
			groupMode = state.GroupMode
		}
		if state.StatusFilter != "" {
			statusFilter = state.StatusFilter
		}
		if len(state.CollapsedGroups) > 0 {
			collapsedGroups = make(map[string]bool)
			for _, k := range state.CollapsedGroups {
				collapsedGroups[k] = true
			}
		}
	}
	itemsDir := filepath.Join(root, ".wn", "items")
	return tuiModel{
		store:           store,
		root:            root,
		settings:        settings,
		currentID:       currentID,
		statusFilter:    statusFilter,
		groupMode:       groupMode,
		collapsedGroups: collapsedGroups,
		watchCh:         tuiStartWatcher(itemsDir),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.cmdLoad(), tuiWatchCmd(m.watchCh))
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

// tuiFilteredSortedItems returns items matching status/text filters, sorted for the given group mode.
func tuiFilteredSortedItems(allItems []*wn.Item, blockedSet map[string]bool, filterText, statusFilter, groupMode string, now time.Time) []*wn.Item {
	var out []*wn.Item

	tagOnly := strings.HasPrefix(filterText, "#")
	search := strings.ToLower(strings.TrimPrefix(filterText, "#"))

	for _, it := range allItems {
		switch statusFilter {
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

	mode := groupMode
	if mode == "" {
		mode = tuiGroupModeStatus
	}
	switch mode {
	case tuiGroupModeStatus:
		sort.SliceStable(out, func(i, j int) bool {
			gi := tuiGroupKey(out[i], blockedSet[out[i].ID], now)
			gj := tuiGroupKey(out[j], blockedSet[out[j].ID], now)
			return gi < gj
		})
	case tuiGroupModeTags:
		sort.SliceStable(out, func(i, j int) bool {
			ti, tj := "", ""
			if len(out[i].Tags) > 0 {
				ti = out[i].Tags[0]
			}
			if len(out[j].Tags) > 0 {
				tj = out[j].Tags[0]
			}
			return ti < tj
		})
	}
	return out
}

func (m *tuiModel) applyFilter() {
	now := time.Now().UTC()
	m.items = tuiFilteredSortedItems(m.allItems, m.blockedSet, m.filterText, m.statusFilter, m.groupMode, now)
	m.buildRows()
}

// tuiGroupKey returns the display-priority group for an item (lower = shown first).
func tuiGroupKey(it *wn.Item, blocked bool, now time.Time) int {
	if it.Done {
		return tuiGroupDone
	}
	if it.PromptReady {
		return tuiGroupPrompt
	}
	if it.ReviewReady {
		return tuiGroupReview
	}
	if wn.IsInProgress(it, now) {
		return tuiGroupClaimed
	}
	return tuiGroupUndone // includes blocked items
}

// tuiGroupLabel returns the display label for a group.
func tuiGroupLabel(g int) string {
	switch g {
	case tuiGroupPrompt:
		return "Needs Response"
	case tuiGroupReview:
		return "Review Ready"
	case tuiGroupClaimed:
		return "In Progress"
	case tuiGroupUndone:
		return "Undone"
	case tuiGroupDone:
		return "Done"
	default:
		return ""
	}
}

// tuiBuildRows builds the flat display-row list (group headers + item rows) from filtered items.
func tuiBuildRows(items []*wn.Item, groupMode string, collapsedGroups map[string]bool, blockedSet map[string]bool, now time.Time) []tuiListRow {
	if len(items) == 0 {
		return nil
	}

	mode := groupMode
	if mode == "" {
		mode = tuiGroupModeStatus
	}

	if mode == tuiGroupModeNone {
		rows := make([]tuiListRow, len(items))
		for i := range items {
			rows[i] = tuiListRow{itemIdx: i}
		}
		return rows
	}

	type section struct {
		key   string
		label string
		items []int
	}
	var sections []section
	sectionIdx := map[string]int{}

	for i, it := range items {
		var key, label string
		switch mode {
		case tuiGroupModeStatus:
			g := tuiGroupKey(it, blockedSet[it.ID], now)
			key = strconv.Itoa(g)
			label = tuiGroupLabel(g)
		default: // tuiGroupModeTags
			if len(it.Tags) == 0 {
				key = ""
				label = "(no tags)"
			} else {
				key = it.Tags[0]
				label = "#" + it.Tags[0]
			}
		}
		if idx, ok := sectionIdx[key]; ok {
			sections[idx].items = append(sections[idx].items, i)
		} else {
			sectionIdx[key] = len(sections)
			sections = append(sections, section{key: key, label: label, items: []int{i}})
		}
	}

	var rows []tuiListRow
	for _, sec := range sections {
		collapsed := collapsedGroups != nil && collapsedGroups[sec.key]
		indicator := "▼"
		if collapsed {
			indicator = "▶"
		}
		header := fmt.Sprintf("%s %s (%d)", indicator, sec.label, len(sec.items))
		rows = append(rows, tuiListRow{header: header, groupKey: sec.key, itemIdx: -1})
		if !collapsed {
			for _, idx := range sec.items {
				rows = append(rows, tuiListRow{itemIdx: idx})
			}
		}
	}
	return rows
}

func (m *tuiModel) buildRows() {
	now := time.Now().UTC()
	m.rows = tuiBuildRows(m.items, m.groupMode, m.collapsedGroups, m.blockedSet, now)
}

// clampCursor ensures m.cursor is within m.rows bounds and adjusts m.listOffset for scrolling.
func (m *tuiModel) clampCursor() {
	n := len(m.rows)
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
		// Keep the preceding group header visible if cursor is the first item in a group.
		if m.listOffset > 0 && m.rows[m.listOffset-1].header != "" {
			m.listOffset--
		}
	}
	if m.cursor >= m.listOffset+bh {
		m.listOffset = m.cursor - bh + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}

// restoreCursor moves m.cursor to the row for the item with the given ID, or clamps if not found.
func (m *tuiModel) restoreCursor(id string) {
	if id != "" {
		for i, row := range m.rows {
			if row.itemIdx >= 0 && m.items[row.itemIdx].ID == id {
				m.cursor = i
				m.clampCursor()
				return
			}
		}
	}
	m.clampCursor()
}

// toggleGroupCollapse toggles the collapsed state of the given group key and rebuilds rows.
func (m *tuiModel) toggleGroupCollapse(key string) {
	if m.collapsedGroups == nil {
		m.collapsedGroups = make(map[string]bool)
	}
	if m.collapsedGroups[key] {
		delete(m.collapsedGroups, key)
	} else {
		m.collapsedGroups[key] = true
	}
	m.buildRows()
}

// collapseItemGroup collapses the group containing the currently selected item
// and moves the cursor to that group's header row.
func (m *tuiModel) collapseItemGroup() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	row := m.rows[m.cursor]
	if row.itemIdx < 0 {
		return // already on a header row
	}
	// Walk back to find the preceding header row.
	headerKey := ""
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].header != "" {
			headerKey = m.rows[i].groupKey
			break
		}
	}
	if headerKey == "" {
		return
	}
	if m.collapsedGroups == nil {
		m.collapsedGroups = make(map[string]bool)
	}
	m.collapsedGroups[headerKey] = true
	m.buildRows()
	// Move cursor to the header row for the collapsed group.
	for i, r := range m.rows {
		if r.groupKey == headerKey {
			m.cursor = i
			break
		}
	}
	m.clampCursor()
}

func (m tuiModel) bodyHeight() int {
	h := m.height - 2 // header row + footer row
	if h < 1 {
		return 1
	}
	return h
}

// selected returns the currently highlighted item, or nil if the cursor is on a header row.
func (m tuiModel) selected() *wn.Item {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	row := m.rows[m.cursor]
	if row.itemIdx < 0 {
		return nil // cursor is on a header row
	}
	return m.items[row.itemIdx]
}

func (m *tuiModel) refreshViewport() {
	if !m.vpReady {
		return
	}
	if m.showHelp {
		m.vp.SetContent(tuiHelpContent())
		m.vp.GotoTop()
		return
	}
	it := m.selected()
	if it == nil {
		m.vp.SetContent("(no items)")
		return
	}
	m.vp.SetContent(tuiItemDetail(it, m.blockedSet[it.ID], m.store, m.vp.Width))
	m.vp.GotoTop()
}

type tuiFlags struct {
	resetState bool
}

func newTUICmd() *cobra.Command {
	flags := &tuiFlags{}
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Interactive TUI for managing work items",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd, args, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.resetState, "reset-state", false, "Start with default TUI state (ignore saved state)")
	return cmd
}

func runTUI(cmd *cobra.Command, args []string, flags *tuiFlags) error {
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
	var state tuiState
	if flags.resetState {
		state = tuiState{StatusFilter: tuiFilterActive, GroupMode: tuiGroupModeStatus}
	} else {
		state = loadTUIState(root)
	}
	m := newTUI(store, root, settings, meta.CurrentID, state)
	finalModel, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if finalM, ok := finalModel.(tuiModel); ok {
		saveTUIState(root, finalM)
	}
	return err
}
