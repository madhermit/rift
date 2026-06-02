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
// parent echoes the ReqID from the SelectionChangedMsg it is answering. When
// Append is set, Content is appended to the existing preview (without resetting
// scroll or caching) — used to stream a multi-file diff in progressively.
type PreviewMsg struct {
	Content string
	Err     error
	ReqID   int
	Append  bool
}

// SplitConfig configures a SplitList for a concrete item type.
type SplitConfig[T any] struct {
	Screen      string      // header screen label, e.g. "log"
	ListTitle   string      // list-pane title, e.g. "commits"
	Context     string      // header right-context, e.g. the diff engine name
	Hints       [][2]string // footer keybinding hints
	EmptyStatus string      // footer status when there are no items

	// NavFraction caps the list strip at this fraction (percent) of the height;
	// the strip still fits its contents below the cap, so a short list takes less.
	// Defaults to ~2/5 when 0.
	NavFraction int

	// Row renders one item's text; the selection marker is added by SplitList.
	// width is the available inner width.
	Row func(item T, width int, selected bool) string
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
	// pane width must not be served at another (e.g. after a resize).
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
	m.viewport.GotoTop() // so a progressive stream's first chunk paints at the top

	reqID := m.reqID
	emit := func() tea.Msg { return SelectionChangedMsg{ReqID: reqID} }
	if m.loading {
		return m, emit
	}
	m.loading = true
	return m, tea.Batch(emit, m.spinner.Tick)
}

// cacheCurrent stores content under the selected item's cache key, skipping
// items without one (e.g. the diff "All changes" entry, whose content depends on
// the filtered set).
func (m SplitList[T]) cacheCurrent(content string) {
	if it, ok := m.Selected(); ok {
		if key := m.cacheKey(it); key != "" {
			m.cache[key] = content
		}
	}
}

// CacheCurrentPreview memoizes the accumulated preview. Parents call it once a
// progressive stream completes — the per-chunk Append messages deliberately
// don't cache — so the full result is served instantly on revisit.
func (m SplitList[T]) CacheCurrentPreview() SplitList[T] {
	m.cacheCurrent(m.preview)
	return m
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

// Reload re-requests the current selection's preview (a cache hit when it's
// already loaded, otherwise a fresh load). Used to refresh a preview whose
// stream was interrupted — e.g. returning from a drilldown.
func (m SplitList[T]) Reload() (SplitList[T], tea.Cmd) {
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
		if msg.Append {
			// Streamed chunk: grow the preview and keep the scroll position.
			// Streamed chunks are never cached individually (the parent caches the
			// whole result on completion) and never reset scroll — the first chunk
			// appends onto the empty, top-scrolled pane requestPreview left behind.
			m.preview += msg.Content
			m.setPreviewContent()
			return m, nil
		}
		m.preview = msg.Content
		m.cacheCurrent(msg.Content)
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
		// esc means "go back" (e.g. exit a drilldown); at the root there's nowhere
		// to go, so do nothing. Quitting is q / ctrl+c.
		return m, nil
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
	case "J", "shift+down", "]":
		return m.stepList(1)
	case "K", "shift+up", "[":
		return m.stepList(-1)
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

func clampIndex(i, n int) int {
	if i < 0 || n == 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// navigate handles up/down: scrolling the preview when it's focused, otherwise
// stepping the list selection.
func (m SplitList[T]) navigate(delta int) (SplitList[T], tea.Cmd) {
	if m.active == splitPreviewPane {
		if delta > 0 {
			m.viewport.ScrollDown(1)
		} else {
			m.viewport.ScrollUp(1)
		}
		return m, nil
	}
	return m.stepList(delta)
}

// stepList moves the list selection and loads the new item's preview, regardless
// of which pane is focused. Bound to J/K (and shift+↑/↓, with ]/[ as a vim
// alias) so you can step to the next/previous item without leaving the preview
// (e.g. read the next file's diff in place) — a bigger-unit jump than the j/k
// line scroll.
func (m SplitList[T]) stepList(delta int) (SplitList[T], tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	next := clampIndex(m.selected+delta, len(m.filtered))
	if next == m.selected {
		return m, nil // already at the boundary; nothing to reload
	}
	m.selected = next
	return m.requestPreview()
}

func (m SplitList[T]) applyFilter() (SplitList[T], tea.Cmd) {
	m.filtered = FuzzyFilter(m.items, m.filter.Value(), m.cfg.Match)
	m.selected = 0
	// relayout (not just requestPreview): the strip height tracks the item count,
	// so the preview viewport must resize when the filter changes.
	return m.relayout()
}

// panelMin is the minimum outer height of a panel (a 2-row border around at least
// one content row) — the floor for both the list strip and the preview.
const panelMin = 3

func (m SplitList[T]) contentHeight() int {
	return maxInt(panelMin, m.height-HeaderRows-FooterRows)
}

// stackLayout computes the vertical split. While reading (preview focused) the
// strip shrinks to a one-row peek. While surveying the strip fits the list
// (capped at NavFraction of the height); the preview fills the rest below it.
func (m SplitList[T]) stackLayout() (previewH, navH int) {
	contentH := m.contentHeight()
	// Too short to host two panels (strip + preview, each at least panelMin): the
	// preview fills the whole content area and there is no strip.
	if contentH < 2*panelMin {
		return contentH, 0
	}
	// Reading: the strip shrinks to a one-row peek of the current item, the
	// focused preview takes the rest.
	if m.active == splitPreviewPane {
		return contentH - panelMin, panelMin
	}
	capPct := m.cfg.NavFraction
	if capPct <= 0 {
		capPct = 40 // ~2/5
	}
	navH = len(m.filtered) + 2 // +2 panel border; fit to content
	if capH := contentH * capPct / 100; navH > capH {
		navH = capH
	}
	if navH > contentH-panelMin {
		navH = contentH - panelMin // leave the preview at least one panel
	}
	if navH < panelMin {
		navH = panelMin
	}
	return contentH - navH, navH
}

func (m SplitList[T]) relayout() (SplitList[T], tea.Cmd) {
	previewH, _ := m.stackLayout()
	m.viewport.SetWidth(maxInt(1, m.width-2))
	m.viewport.SetHeight(maxInt(1, previewH-2))
	return m.requestPreview()
}

func (m *SplitList[T]) setPreviewContent() {
	content := m.preview
	if w := m.viewport.Width(); w > 0 && content != "" {
		content = ansi.Hardwrap(content, w, true)
	}
	m.vim.SetContent(&m.viewport, content)
}

// currentSection is the file the preview is currently showing, for the panel
// legend — empty only before the first file (e.g. a commit header) or when the
// preview has no file sections.
func (m SplitList[T]) currentSection() string {
	return m.vim.CurrentSection(m.viewport.YOffset())
}

// sectionProgress is the current file's "N/M" position (file of total), for the
// right of the panel legend.
func (m SplitList[T]) sectionProgress() string {
	return m.vim.SectionProgress(m.viewport.YOffset())
}

// clearPreview blanks the preview pane and stops the spinner.
func (m *SplitList[T]) clearPreview() {
	m.loading = false
	m.preview = ""
	m.setPreviewContent()
}

// listView renders the windowed list body (markers + rows) that keeps the
// selection on screen, plus the matching scrollbar. rowWidth is the inner width
// available to each row.
func (m SplitList[T]) listView(innerH, rowWidth int) (string, Scrollbar) {
	if innerH < 1 {
		innerH = 1
	}
	offset, bar := ListWindow(m.selected, len(m.filtered), innerH)
	var b strings.Builder
	for i := offset; i < len(m.filtered) && i-offset < innerH; i++ {
		selected := i == m.selected
		b.WriteString(Marker(selected) + m.cfg.Row(m.filtered[i], rowWidth, selected) + "\n")
	}
	return b.String(), bar
}

// previewTitleView is the preview pane's title, prefixed with the spinner while
// a preview is loading.
func (m SplitList[T]) previewTitleView() string {
	title := ""
	if it, ok := m.Selected(); ok && m.cfg.PreviewTitle != nil {
		title = m.cfg.PreviewTitle(it)
	}
	if m.loading {
		title = m.spinner.View() + " " + title
	}
	return title
}

// previewBodyView is the viewport content plus its scrollbar. The current file
// is shown in the legend, not the body — see currentSection / content.
func (m SplitList[T]) previewBodyView() (string, Scrollbar) {
	return m.viewport.View(), ScrollbarFor(&m.viewport)
}

// footerView renders the bottom bar: the filter prompt while filtering, an error
// or flash message when present, otherwise the position (and scroll % while the
// preview is focused) followed by the keybinding hints.
func (m SplitList[T]) footerView() string {
	switch {
	case m.filtering:
		return FooterContent(m.width, m.filter.View())
	case m.prevErr != nil:
		return Footer(m.width, fmt.Sprintf("Error: %v", m.prevErr), nil)
	case m.flash != "":
		return Footer(m.width, m.flash, m.hints())
	default:
		return Footer(m.width, m.statusText(), m.hints())
	}
}

// hints are the footer keybinding hints, with the ⇥ hint relabeled "list" while
// reading — there tab expands the one-row peek back to the full navigable list.
func (m SplitList[T]) hints() [][2]string {
	if m.active != splitPreviewPane {
		return m.cfg.Hints
	}
	out := make([][2]string, len(m.cfg.Hints))
	copy(out, m.cfg.Hints)
	for i := range out {
		if out[i][0] == "⇥" {
			out[i][1] = "list"
		}
	}
	return out
}

// positionText is the 1-based "N/M" selection position, or "" for ≤1 item.
func (m SplitList[T]) positionText() string {
	if len(m.filtered) <= 1 {
		return ""
	}
	return fmt.Sprintf("%d/%d", m.selected+1, len(m.filtered))
}

// statusText is the footer status segment. The selection position lives in the
// list/peek title now, so this only surfaces the empty-list message.
func (m SplitList[T]) statusText() string {
	if len(m.filtered) == 0 {
		return m.cfg.EmptyStatus
	}
	return ""
}

// View renders the full screen (header + content + footer) as a string. The
// parent wraps it in a tea.View.
func (m SplitList[T]) View() string {
	if !m.ready {
		return "Loading..."
	}
	contentH := m.height - HeaderRows - FooterRows
	if m.showHelp {
		return HelpView(m.cfg.Screen, m.cfg.Context, m.cfg.Hints, PreviewHelpKeys, m.width, contentH)
	}

	header := Header(m.cfg.Screen, m.cfg.Context, m.width)
	return lipgloss.JoinVertical(lipgloss.Left, header, m.content(), m.footerView())
}

// content renders the body: while surveying, the list strip (focused) sits on
// top with the preview below; while reading, the strip collapses to a one-row
// peek above the focused preview. A terminal too short for two panels falls back
// to the preview alone.
func (m SplitList[T]) content() string {
	previewBody, previewBar := m.previewBodyView()
	previewH, navH := m.stackLayout()
	reading := m.active == splitPreviewPane

	if navH == 0 {
		// Too short for two panels: the preview fills the area and is the only
		// orientation, so it keeps the item name (with the spinner while loading,
		// via previewTitleView) plus the file position while reading.
		right := ""
		if reading {
			right = m.sectionProgress()
		}
		return Panel(m.previewTitleView(), right, previewBody, m.width, previewH, reading, previewBar)
	}

	// Two panels. The preview's border legend earns its place: the spinner (with
	// the item) while loading; while reading (focused), the current file on the
	// left and its position (file N/M) on the right; otherwise (unfocused,
	// surveying) a clean border, since the peek already names the item.
	previewTitle, previewRight := "", ""
	switch {
	case m.loading:
		// Just the spinner while loading — including the item name here makes the
		// filename flash in the unfocused preview legend as you navigate the strip.
		previewTitle = m.spinner.View()
	case reading:
		previewTitle = m.currentSection()
		previewRight = m.sectionProgress()
	}
	navTitle := m.cfg.ListTitle
	if pos := m.positionText(); pos != "" {
		navTitle += " · " + pos // the selection position rides with the list title
	}
	listBody, listBar := m.listView(navH-2, m.width-3) // border(2)+marker(1)
	if reading {
		// Peek: one row of the current item, so a scrollbar would mislead.
		listBar = Scrollbar{}
	}
	navPanel := Panel(navTitle, "", listBody, m.width, navH, !reading, listBar)
	previewPanel := Panel(previewTitle, previewRight, previewBody, m.width, previewH, reading, previewBar)
	return lipgloss.JoinVertical(lipgloss.Left, navPanel, previewPanel)
}

// TeaView wraps the rendered screen in a full-screen tea.View. Parent models
// delegate their View() to this.
func (m SplitList[T]) TeaView() tea.View {
	v := tea.NewView(m.View())
	v.AltScreen = true
	return v
}
