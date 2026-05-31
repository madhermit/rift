package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// SelectionChangedMsg is emitted by a SplitList when the selected item changes
// and its preview is not already cached. The parent loads a preview and feeds
// the result back as a PreviewMsg carrying the same ReqID. Routing preview
// loading through the parent lets previews depend on parent state (e.g. staged
// vs unstaged). ReqID lets the component discard results from superseded
// requests during fast navigation.
type SelectionChangedMsg struct{ ReqID int }

// PreviewMsg carries loaded preview content into a SplitList's preview pane. The
// parent echoes the ReqID from the SelectionChangedMsg it is answering.
type PreviewMsg struct {
	Content string
	Err     error
	ReqID   int
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
	// CacheKey returns a stable identity for an item's preview, used to cache
	// rendered previews. Return "" to skip caching for an item. May be nil.
	// The cache is keyed by item identity only; the parent clears it (ClearCache
	// / SetItems) when a setting that affects rendering changes.
	CacheKey func(item T) string
	// Yank returns the text to copy to the clipboard for an item when `y` is
	// pressed (e.g. a commit hash or file path). Return "" or leave nil to
	// disable yanking.
	Yank func(item T) string
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

	spinner  spinner.Model
	loading  bool   // a preview is being loaded for the current selection
	showHelp bool   // the keybinding overlay is open
	flash    string // transient footer message (e.g. "copied …"), cleared on nav

	cache map[string]string // rendered previews, keyed by cfg.CacheKey
	reqID int               // increments per preview request; stale results are dropped

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
		spinner:  spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		cache:    map[string]string{},
	}
}

func (m SplitList[T]) cacheKey(it T) string {
	if m.cfg.CacheKey == nil {
		return ""
	}
	k := m.cfg.CacheKey(it)
	if k == "" {
		return ""
	}
	// Width is part of the preview's identity: difftastic bakes the
	// side-by-side/inline layout into its output, so a preview rendered at one
	// pane width must not be served at another (e.g. after a resize or after
	// tab/enter collapses the list pane).
	return fmt.Sprintf("%s\x00%d", k, m.viewport.Width())
}

// requestPreview is called whenever the selection changes. On a cache hit it
// shows the cached preview instantly; on a miss it clears the now-stale preview
// (so the previous item's diff is never shown under the new selection), shows
// the spinner, and asks the parent to load via SelectionChangedMsg.
func (m SplitList[T]) requestPreview() (SplitList[T], tea.Cmd) {
	m.reqID++
	m.prevErr = nil // a new selection clears any previous load error
	m.flash = ""    // and any transient message (e.g. "copied …")
	it, ok := m.Selected()
	if !ok {
		m.clearPreview()
		return m, nil
	}
	if key := m.cacheKey(it); key != "" {
		if cached, hit := m.cache[key]; hit {
			m.loading = false
			m.preview = cached
			m.setPreviewContent()
			m.viewport.GotoTop()
			return m, nil
		}
	}
	m.preview = ""
	m.setPreviewContent()

	reqID := m.reqID
	emit := func() tea.Msg { return SelectionChangedMsg{ReqID: reqID} }
	if m.loading {
		return m, emit
	}
	m.loading = true
	return m, tea.Batch(emit, m.spinner.Tick)
}

// yank copies the selected item's Yank value to the clipboard and flashes a
// confirmation in the footer.
func (m SplitList[T]) yank() (SplitList[T], tea.Cmd) {
	it, ok := m.Selected()
	if !ok || m.cfg.Yank == nil {
		return m, nil
	}
	val := m.cfg.Yank(it)
	if val == "" {
		return m, nil
	}
	if err := clipboard.WriteAll(val); err != nil {
		m.flash = "clipboard unavailable"
	} else {
		m.flash = "copied " + val
	}
	return m, nil
}

// ClearCacheAndReload drops all cached previews and reloads the current
// selection. Parents call this when a setting that affects rendering (e.g. the
// diff layout) changes.
func (m SplitList[T]) ClearCacheAndReload() (SplitList[T], tea.Cmd) {
	m.cache = map[string]string{}
	return m.requestPreview()
}

// SetError shows err in the footer and clears the preview. Used for failures
// not tied to a specific preview request (e.g. reloading the item list); unlike
// a PreviewMsg it is not gated by the request token.
func (m SplitList[T]) SetError(err error) SplitList[T] {
	m.prevErr = err
	m.clearPreview()
	return m
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

// ShowingHelp reports whether the keybinding overlay is open. Parents check this
// (like Filtering) to stop intercepting keys while it's up.
func (m SplitList[T]) ShowingHelp() bool { return m.showHelp }

// PreviewWidth is the inner width of the preview pane, for wrapping content.
func (m SplitList[T]) PreviewWidth() int { return m.viewport.Width() }

// SetListTitle updates the list-pane title.
func (m SplitList[T]) SetListTitle(title string) SplitList[T] {
	m.cfg.ListTitle = title
	return m
}

// SetContext updates the header right-context (e.g. the active diff engine).
func (m SplitList[T]) SetContext(ctx string) SplitList[T] {
	m.cfg.Context = ctx
	return m
}

// SetItems replaces the item set, re-applies the active filter, and requests a
// fresh preview. The cache is cleared because the item set (and the state it
// reflects, e.g. staged vs unstaged) has changed.
func (m SplitList[T]) SetItems(items []T) (SplitList[T], tea.Cmd) {
	m.items = items
	m.cache = map[string]string{}
	return m.applyFilter()
}

func (m SplitList[T]) Update(msg tea.Msg) (SplitList[T], tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		return m.relayout()
	case PreviewMsg:
		if msg.ReqID != m.reqID {
			return m, nil // superseded by a newer selection
		}
		m.prevErr = msg.Err
		if msg.Err != nil {
			m.clearPreview()
			return m, nil
		}
		m.loading = false
		m.preview = msg.Content
		if it, ok := m.Selected(); ok {
			if key := m.cacheKey(it); key != "" {
				m.cache[key] = msg.Content
			}
		}
		m.setPreviewContent()
		m.viewport.GotoTop()
		return m, nil
	case spinner.TickMsg:
		if !m.loading {
			return m, nil // let the tick chain die once loading finishes
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
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
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.showHelp {
		m.showHelp = false // any key dismisses the help overlay
		return m, nil
	}

	if m.active == splitPreviewPane && !m.filtering && m.vim.HandleKey(&m.viewport, msg) {
		return m, nil
	}

	if msg.String() == "esc" {
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
	case "y":
		return m.yank()
	case "?":
		m.showHelp = true
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
	prev := m.selected
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
	if m.selected == prev {
		return m, nil // already at the boundary; nothing to reload
	}
	return m.requestPreview()
}

func (m SplitList[T]) applyFilter() (SplitList[T], tea.Cmd) {
	m.filtered = FuzzyFilter(m.items, m.filter.Value(), m.cfg.Match)
	m.selected = 0
	return m.requestPreview()
}

func (m SplitList[T]) layout() SplitLayout {
	return ComputeSplitLayout(m.width, m.height, m.active == splitPreviewPane, m.cfg.MinList, m.cfg.MaxList)
}

func (m SplitList[T]) relayout() (SplitList[T], tea.Cmd) {
	l := m.layout()
	m.viewport.SetWidth(l.DiffWidth)
	m.viewport.SetHeight(l.ContentHeight - 2)
	return m.requestPreview()
}

func (m *SplitList[T]) setPreviewContent() {
	content := m.preview
	if w := m.viewport.Width(); w > 0 && content != "" {
		content = ansi.Hardwrap(content, w, true)
	}
	m.vim.SetContent(&m.viewport, content)
}

// clearPreview blanks the preview pane and stops the spinner.
func (m *SplitList[T]) clearPreview() {
	m.loading = false
	m.preview = ""
	m.setPreviewContent()
}

// View renders the full screen (header + split panes + footer) as a string. The
// parent wraps it in a tea.View.
func (m SplitList[T]) View() string {
	if !m.ready {
		return "Loading..."
	}

	l := m.layout()
	if m.showHelp {
		return HelpView(m.cfg.Screen, m.cfg.Context, m.cfg.Hints, PreviewHelpKeys, m.width, l.ContentHeight)
	}
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
	if m.loading {
		previewTitle = m.spinner.View() + " " + previewTitle
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
	case m.flash != "":
		footer = Footer(m.width, m.flash, m.cfg.Hints)
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
