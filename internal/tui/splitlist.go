package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// SelectionChangedMsg is emitted by a SplitList whenever the selected item
// changes (navigation, filtering, resize, or SetItems). The parent model
// responds by loading a preview for the now-selected item and feeding the
// result back as a PreviewMsg. Routing preview loading through the parent lets
// previews depend on parent state (e.g. staged vs unstaged).
type SelectionChangedMsg struct{}

// PreviewMsg carries loaded preview content into a SplitList's preview pane.
type PreviewMsg struct {
	Content string
	Err     error
}

// SplitConfig configures a SplitList for a concrete item type.
type SplitConfig[T any] struct {
	Screen      string // header screen label, e.g. "log"
	ListTitle   string // list-pane title, e.g. "commits"
	Context     string // header right-context, e.g. the diff engine name
	MinList     int    // list-width clamp (expanded)
	MaxList     int
	Hints       [][2]string // footer keybinding hints
	EmptyStatus string      // footer status when there are no items

	// Row renders one item's text; the selection marker is added by SplitList.
	// width is the available inner width; collapsed is true when the list pane
	// is narrowed because the preview pane is focused.
	Row func(item T, width int, selected, collapsed bool) string
	// Match returns an item's fuzzy-match target.
	Match func(item T) string
	// PreviewTitle returns the preview pane's title for an item (may be nil).
	PreviewTitle func(item T) string
}

type splitPane int

const (
	splitListPane splitPane = iota
	splitPreviewPane
)

// SplitList is a reusable list + preview split-pane component. It owns
// selection, scrolling, fuzzy filtering, the preview viewport, and vim-style
// navigation. Embed it in a parent model and delegate Update/View to it.
type SplitList[T any] struct {
	cfg      SplitConfig[T]
	items    []T
	filtered []T
	selected int
	active   splitPane

	filter    textinput.Model
	filtering bool

	viewport viewport.Model
	vim      VimNav
	preview  string
	prevErr  error

	width, height int
	ready         bool
}

// NewSplitList builds a SplitList for the given items.
func NewSplitList[T any](cfg SplitConfig[T], items []T) SplitList[T] {
	return SplitList[T]{
		cfg:      cfg,
		items:    items,
		filtered: items,
		viewport: viewport.New(),
		filter:   NewFilterInput(),
	}
}

// Selected returns the currently selected item, or ok=false when the (filtered)
// list is empty.
func (m SplitList[T]) Selected() (T, bool) {
	var zero T
	if len(m.filtered) == 0 {
		return zero, false
	}
	return m.filtered[m.selected], true
}

// VisibleItems returns the currently filtered items.
func (m SplitList[T]) VisibleItems() []T { return m.filtered }

// Filtering reports whether the filter input is active.
func (m SplitList[T]) Filtering() bool { return m.filtering }

// PreviewWidth is the inner width of the preview pane, for wrapping content.
func (m SplitList[T]) PreviewWidth() int { return m.viewport.Width() }

// SetListTitle updates the list-pane title.
func (m SplitList[T]) SetListTitle(title string) SplitList[T] {
	m.cfg.ListTitle = title
	return m
}

// SetItems replaces the item set, re-applies the active filter, and requests a
// fresh preview.
func (m SplitList[T]) SetItems(items []T) (SplitList[T], tea.Cmd) {
	m.items = items
	return m.applyFilter()
}

func (m SplitList[T]) Update(msg tea.Msg) (SplitList[T], tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		return m.relayout()
	case PreviewMsg:
		m.prevErr = msg.Err
		if msg.Err != nil {
			m.preview = ""
		} else {
			m.preview = msg.Content
		}
		m.setPreviewContent()
		m.viewport.GotoTop()
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.active == splitPreviewPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m SplitList[T]) handleKey(msg tea.KeyPressMsg) (SplitList[T], tea.Cmd) {
	if m.active == splitPreviewPane && !m.filtering && m.vim.HandleKey(&m.viewport, msg) {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filtering {
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			return m.applyFilter()
		}
		return m, tea.Quit
	}

	if m.filtering {
		return m.handleFilterKey(msg)
	}

	switch msg.String() {
	case "tab":
		m.active = 1 - m.active
		return m.relayout()
	case "enter":
		if m.active == splitListPane {
			m.active = splitPreviewPane
			return m.relayout()
		}
	case "up", "k":
		return m.navigate(-1)
	case "down", "j":
		return m.navigate(1)
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, nil
	case "q":
		return m, tea.Quit
	}

	if m.active == splitPreviewPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m SplitList[T]) handleFilterKey(msg tea.KeyPressMsg) (SplitList[T], tea.Cmd) {
	if msg.String() == "enter" {
		m.filtering = false
		m.filter.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m, fcmd := m.applyFilter()
	return m, tea.Batch(cmd, fcmd)
}

func (m SplitList[T]) navigate(delta int) (SplitList[T], tea.Cmd) {
	if m.active == splitPreviewPane {
		if delta > 0 {
			m.viewport.ScrollDown(1)
		} else {
			m.viewport.ScrollUp(1)
		}
		return m, nil
	}
	if len(m.filtered) == 0 {
		return m, nil
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
	return m, selectionChanged
}

func (m SplitList[T]) applyFilter() (SplitList[T], tea.Cmd) {
	m.filtered = FuzzyFilter(m.items, m.filter.Value(), m.cfg.Match)
	m.selected = 0
	if len(m.filtered) == 0 {
		m.preview = ""
		m.viewport.SetContent("")
		return m, nil
	}
	return m, selectionChanged
}

func (m SplitList[T]) layout() SplitLayout {
	return ComputeSplitLayout(m.width, m.height, m.active == splitPreviewPane, m.cfg.MinList, m.cfg.MaxList)
}

func (m SplitList[T]) relayout() (SplitList[T], tea.Cmd) {
	l := m.layout()
	m.viewport.SetWidth(l.DiffWidth)
	m.viewport.SetHeight(l.ContentHeight - 2)
	m.setPreviewContent()
	if len(m.filtered) == 0 {
		return m, nil
	}
	return m, selectionChanged
}

func (m *SplitList[T]) setPreviewContent() {
	content := m.preview
	if w := m.viewport.Width(); w > 0 && content != "" {
		content = ansi.Hardwrap(content, w, true)
	}
	m.vim.SetContent(&m.viewport, content)
}

func selectionChanged() tea.Msg { return SelectionChangedMsg{} }

// View renders the full screen (header + split panes + footer) as a string. The
// parent wraps it in a tea.View.
func (m SplitList[T]) View() string {
	if !m.ready {
		return "Loading..."
	}

	l := m.layout()
	collapsed := m.active == splitPreviewPane

	innerH := l.ContentHeight - 2
	if innerH < 1 {
		innerH = 1
	}
	scrollOffset := 0
	if m.selected >= innerH {
		scrollOffset = m.selected - innerH + 1
	}
	rowWidth := l.ListWidth - 3 // border (2) + marker (1)
	var list strings.Builder
	for i := scrollOffset; i < len(m.filtered) && i-scrollOffset < innerH; i++ {
		selected := i == m.selected
		list.WriteString(Marker(selected) + m.cfg.Row(m.filtered[i], rowWidth, selected, collapsed) + "\n")
	}

	listTitle := m.cfg.ListTitle
	if collapsed {
		listTitle = ""
	}
	previewTitle := ""
	if it, ok := m.Selected(); ok && m.cfg.PreviewTitle != nil {
		previewTitle = m.cfg.PreviewTitle(it)
	}

	listPanel := Panel(listTitle, list.String(), l.ListWidth, l.ContentHeight, m.active == splitListPane)
	previewPanel := Panel(previewTitle, m.viewport.View(), l.DiffWidth+2, l.ContentHeight, m.active == splitPreviewPane)
	content := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, previewPanel)

	header := Header(m.cfg.Screen, m.cfg.Context, m.width)

	var footer string
	switch {
	case m.filtering:
		footer = FooterContent(m.width, m.filter.View())
	case m.prevErr != nil:
		footer = Footer(m.width, fmt.Sprintf("Error: %v", m.prevErr), nil)
	default:
		status := m.cfg.EmptyStatus
		if len(m.filtered) > 0 {
			status = fmt.Sprintf("%d/%d", m.selected+1, len(m.filtered))
			if collapsed {
				status += fmt.Sprintf(" · %.0f%%", m.viewport.ScrollPercent()*100)
			}
		}
		footer = Footer(m.width, status, m.cfg.Hints)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// TeaView wraps the rendered screen in a full-screen tea.View. Parent models
// delegate their View() to this.
func (m SplitList[T]) TeaView() tea.View {
	v := tea.NewView(m.View())
	v.AltScreen = true
	return v
}
