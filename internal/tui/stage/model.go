package stageui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

type pane int

const (
	filePane pane = iota
	diffPane
)

// displayHunk is a single hunk with its rendering and staging state.
type displayHunk struct {
	fd       diff.FileDiff // parent file diff (has Header for Patch)
	hunk     diff.Hunk     // raw hunk for staging/unstaging
	rendered string        // difftastic output
	staged   bool          // whether this hunk is currently staged
}

type Model struct {
	repo    *git.Repo
	engines tui.EngineToggle
	branch  string
	hints   [][2]string // footer keybinding hints (engine hint only when toggleable)

	files         []git.StatusFile
	filteredFiles []git.StatusFile
	selectedIdx   int
	activePane    pane

	displayHunks []displayHunk // combined staged + unstaged hunks, in file order
	hunkOffsets  []int         // viewport line offset where each hunk starts
	hunkIdx      int           // selected hunk index

	// hunkCache memoizes rendered hunk sets by path+width so navigation and filter
	// keystrokes don't re-run the whole difftastic pipeline. Invalidated per path on
	// stage/unstage and editor close, and wholesale on a width change.
	hunkCache  map[string][]displayHunk
	diffReqID  int                // increments per diff load; stale hunkDiffsMsg results are dropped
	cancelDiff context.CancelFunc // stops the superseded in-flight diff load's subprocesses

	viewport  viewport.Model
	filter    textinput.Model
	filtering bool

	diffErr  error
	vim      tui.VimNav
	showHelp bool
	flash    string // transient footer message (e.g. "copied …"), cleared on nav

	width  int
	height int
	ready  bool
}

// stageHints builds the footer hints for a model; the engine hint appears only
// when a second engine is available to toggle to.
func stageHints(engines tui.EngineToggle) [][2]string {
	hints := [][2]string{
		{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"},
		{"s", "stage"}, {"u", "unstage"}, {"a", "all"}, {"{/}", "hunk"}, {"o", "open"}, {"y", "yank"},
	}
	if engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	return append(hints, [2]string{"?", "help"}, [2]string{"q", "quit"})
}

// stageNavKeys is the navigation reference for the stage help overlay. Stage is
// not a SplitList, so it lists its own keys (incl. alternates) rather than the
// SplitList PreviewHelpKeys, which advertises keys stage doesn't implement. The
// keys mirror the SplitList screens: gg/G/ctrl+d/u/f/b navigate the file list
// (file pane) or scroll the diff (diff pane), and {/} steps between hunks — the
// same key the SplitList screens use to step between preview sections. esc
// leaves the current mode (filter / help) and otherwise does nothing — it never
// quits (that's q / ctrl+c), matching the diff/log/stash screens.
var stageNavKeys = [][2]string{
	{"j/k  ↑↓", "move / scroll"},
	{"J/K  ⇧↑↓  ]/[", "next / prev file"},
	{"gg/G", "top / bottom"},
	{"{/}", "prev / next hunk"},
	{"ctrl+d/u", "scroll half-page"},
	{"ctrl+f/b", "scroll page"},
	{"esc", "leave mode"},
}

type hunkDiffsMsg struct {
	reqID int    // request token; a result whose token is stale is dropped
	path  string // file the hunks belong to (for the cache key)
	width int    // viewport width the hunks were rendered at (for the cache key)
	hunks []displayHunk
}

type filesLoadedMsg struct {
	files []git.StatusFile
	err   error
}

type stageResultMsg struct {
	err  error
	path string // file whose staged/unstaged split changed (to invalidate its cache)
}

func (m Model) layout() tui.SplitLayout {
	return tui.ComputeSplitLayout(m.width, m.height, m.activePane == diffPane, 20, 60)
}

func New(repo *git.Repo, engine diff.Engine, files []git.StatusFile) Model {
	engines := tui.NewEngineToggle(engine)
	return Model{
		repo:          repo,
		engines:       engines,
		branch:        repo.CurrentBranch(),
		hints:         stageHints(engines),
		files:         files,
		filteredFiles: files,
		hunkCache:     map[string][]displayHunk{},
		viewport:      viewport.New(),
		filter:        tui.NewFilterInput(),
	}
}

func (m Model) Init() tea.Cmd {
	return tui.ThemeInit()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tui.EditorClosedMsg:
		// The edit changed the file's diff, so drop its cached hunks before reloading.
		if m.selectedIdx < len(m.filteredFiles) {
			m.invalidatePath(m.filteredFiles[m.selectedIdx].Path)
		}
		return m, m.reloadFiles()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m.applyLayout()
	case hunkDiffsMsg:
		if msg.reqID != m.diffReqID {
			return m, nil // superseded by a newer selection/layout
		}
		m.hunkCache[hunkCacheKey(msg.path, msg.width)] = msg.hunks
		m.setHunks(msg.hunks)
		return m, nil
	case filesLoadedMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		var selectedPath string
		if m.selectedIdx < len(m.filteredFiles) {
			selectedPath = m.filteredFiles[m.selectedIdx].Path
		}
		m.files = msg.files
		m.applyFilter()
		m.selectedIdx = findFileIndex(m.filteredFiles, selectedPath)
		cmd := m.requestDiff()
		return m, cmd
	case stageResultMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		// The file's staged/unstaged hunk split changed: drop its cache and reload the
		// file list, which re-runs the (now fresh) diff so hunk patches stay valid.
		m.invalidatePath(msg.path)
		return m, m.reloadFiles()
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.showHelp {
		m.showHelp = false // any key closes the help overlay
		return m, nil
	}

	if m.activePane == diffPane && !m.filtering {
		switch msg.String() {
		case "{", "}":
			// handled below as hunk nav
		default:
			if m.vim.HandleKey(&m.viewport, msg) {
				return m, nil
			}
		}
	}

	if msg.String() == "esc" {
		if m.filtering {
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.applyFilter()
			return m, nil
		}
		// esc leaves the current mode (help handled above, filter handled here); at
		// the root there's nowhere to go, so do nothing. Quitting is q / ctrl+c —
		// matching the diff/log/stash screens.
		return m, nil
	}

	if m.filtering {
		return m.handleFilterKey(msg)
	}

	// File pane: the vim jump keys move the selection (gg/G to the ends, ctrl+d/u
	// by half the window, ctrl+f/b by a full window), matching the SplitList
	// screens' list panes. In the diff pane the same keys scroll the viewport
	// (handled above via vim.HandleKey).
	if m.activePane == filePane {
		window := m.layout().ContentHeight - 2
		if next, ok := m.vim.HandleListKey(msg, m.selectedIdx, len(m.filteredFiles), window); ok {
			// selectTo no-ops when next == selectedIdx (e.g. a pending 'g').
			return m.selectTo(next)
		}
	}

	switch msg.String() {
	case "?":
		m.showHelp = true
		return m, nil
	case "o":
		// In the hunk view, open at the selected hunk; in the file list, open the
		// selected file at its top.
		if m.activePane == diffPane && m.hunkIdx < len(m.displayHunks) {
			h := m.displayHunks[m.hunkIdx]
			return m, tui.OpenInEditor(m.repo.Root(), h.fd.Path, h.hunk.NewStart)
		}
		if m.selectedIdx < len(m.filteredFiles) {
			return m, tui.OpenInEditor(m.repo.Root(), m.filteredFiles[m.selectedIdx].Path, 0)
		}
	case "tab":
		if m.activePane == filePane {
			return m.focusPane(diffPane)
		}
		return m.focusPane(filePane)
	case "enter":
		if m.activePane == filePane {
			return m.focusPane(diffPane)
		}
	case "up", "k":
		return m.navigate(-1)
	case "down", "j":
		return m.navigate(1)
	case "J", "shift+down", "]":
		// Step the file selection even while the diff pane is focused, mirroring the
		// SplitList screens' step-while-reading.
		return m.moveSelection(1)
	case "K", "shift+up", "[":
		return m.moveSelection(-1)
	case "q":
		return m, tea.Quit
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, nil
	case "y":
		return m.yank()
	case "e":
		if !m.engines.CanToggle() {
			break // only one engine available; nothing to toggle
		}
		m.engines = m.engines.Toggle()
		m.hunkCache = map[string][]displayHunk{} // the engine changes the rendering
		cmd := m.requestDiff()
		return m, cmd
	case "s":
		return m.stageOrUnstage(true)
	case "u":
		return m.stageOrUnstage(false)
	case "a":
		if m.activePane == filePane {
			return m.stageAll()
		}
	case "}":
		if m.activePane == diffPane {
			return m.navigateHunk(1)
		}
	case "{":
		if m.activePane == diffPane {
			return m.navigateHunk(-1)
		}
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

// yank copies the selected file's path to the clipboard and flashes a
// confirmation (tui.YankToClipboard adds the OSC 52 fallback for headless/SSH).
func (m Model) yank() (tea.Model, tea.Cmd) {
	if len(m.filteredFiles) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.flash, cmd = tui.YankToClipboard(m.filteredFiles[m.selectedIdx].Path)
	return m, cmd
}

func (m Model) navigateHunk(delta int) (tea.Model, tea.Cmd) {
	n := len(m.displayHunks)
	if n == 0 {
		return m, nil
	}
	m.flash = "" // a transient message doesn't survive navigation
	m.hunkIdx += delta
	if m.hunkIdx < 0 {
		m.hunkIdx = 0
	}
	if m.hunkIdx >= n {
		m.hunkIdx = n - 1
	}
	m.renderHunks()
	if m.hunkIdx < len(m.hunkOffsets) {
		m.viewport.SetYOffset(m.hunkOffsets[m.hunkIdx])
	}
	return m, nil
}

func (m Model) stageOrUnstage(stage bool) (tea.Model, tea.Cmd) {
	if len(m.filteredFiles) == 0 {
		return m, nil
	}
	f := m.filteredFiles[m.selectedIdx]

	// Hunk-level: stage/unstage the currently selected hunk
	if m.activePane == diffPane && m.hunkIdx < len(m.displayHunks) {
		dh := m.displayHunks[m.hunkIdx]
		if dh.staged == stage {
			return m, nil // already in desired state
		}
		patch := dh.hunk.Patch(dh.fd.Header)
		repo, path := m.repo, f.Path
		return m, func() tea.Msg {
			var err error
			if stage {
				err = repo.StageHunk(patch)
			} else {
				err = repo.UnstageHunk(patch)
			}
			if err != nil {
				return stageResultMsg{err: err}
			}
			return stageResultMsg{path: path}
		}
	}

	// File-level: unstage requires something staged
	if !stage && (f.StagingStatus == "" || f.StagingStatus == "Untracked") {
		return m, nil
	}
	repo, path := m.repo, f.Path
	return m, func() tea.Msg {
		var err error
		if stage {
			err = repo.Stage(path)
		} else {
			err = repo.Unstage(path)
		}
		if err != nil {
			return stageResultMsg{err: err}
		}
		return stageResultMsg{path: path}
	}
}

func (m Model) stageAll() (tea.Model, tea.Cmd) {
	var paths []string
	for _, f := range m.filteredFiles {
		paths = append(paths, f.Path)
	}
	if len(paths) == 0 {
		return m, nil
	}
	repo := m.repo
	return m, func() tea.Msg {
		if err := repo.Stage(paths...); err != nil {
			return stageResultMsg{err: err}
		}
		return stageResultMsg{} // empty path: every file changed, invalidate all
	}
}

func (m Model) reloadFiles() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		files, err := repo.StatusFiles()
		if err != nil {
			return filesLoadedMsg{err: err}
		}
		return filesLoadedMsg{files: files}
	}
}

// handleMouse routes mouse input by pane: the wheel scrolls the hunk view or
// steps the file selection, a click selects the file row under the pointer and
// focuses the pane it hit. Clicks on an already-focused pane that change
// nothing are no-ops — re-running the layout would snap the diff scroll back
// to the selected hunk. The row is resolved against the layout the user
// clicked on, before any focus change re-widens the panes.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.showHelp || !m.ready {
		return m, nil
	}
	pos := msg.Mouse()
	l := m.layout()
	inContent := pos.Y >= tui.HeaderRows && pos.Y < tui.HeaderRows+l.ContentHeight
	inFiles := inContent && pos.X < l.ListWidth
	inDiff := inContent && !inFiles

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if inFiles {
			// A horizontal wheel maps to delta 0, which selectTo no-ops.
			return m.moveSelection(tui.WheelDelta(msg))
		}
		if inDiff {
			// The viewport handles wheel messages natively (vertical and
			// horizontal), same as the keyboard scrolling fallthrough.
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		switch {
		case inFiles:
			target, ok := tui.ClickedRow(pos.Y-tui.HeaderRows-1, l.ContentHeight-2, m.selectedIdx, len(m.filteredFiles))
			if m.activePane != filePane {
				if ok {
					// Apply the selection before the layout's own diff request,
					// so the pane flip and the new file cost one load, not two.
					m.selectedIdx = target
					m.hunkIdx = 0
				}
				return m.focusPane(filePane)
			}
			if ok {
				return m.selectTo(target)
			}
			return m, nil
		case inDiff:
			return m.focusPane(diffPane)
		}
	}
	return m, nil
}

// focusPane switches the focused pane and re-lays-out (the split widths depend
// on focus); a click or key targeting the already-active pane is a no-op, so
// it can't snap the diff scroll back to the selected hunk.
func (m Model) focusPane(p pane) (tea.Model, tea.Cmd) {
	if m.activePane == p {
		return m, nil
	}
	m.activePane = p
	return m.applyLayout()
}

func (m Model) applyLayout() (tea.Model, tea.Cmd) {
	l := m.layout()
	if l.DiffWidth != m.viewport.Width() {
		m.hunkCache = map[string][]displayHunk{} // width-keyed renders are now stale
	}
	// Clamp to >=1 so a very short/narrow terminal doesn't hand the viewport a
	// negative size.
	m.viewport.SetWidth(max(1, l.DiffWidth))
	m.viewport.SetHeight(max(1, l.ContentHeight-2))
	m.renderHunks() // re-render current hunks at the new size until the reload lands
	cmd := m.requestDiff()
	return m, cmd
}

func (m Model) navigate(delta int) (tea.Model, tea.Cmd) {
	if m.activePane == filePane {
		return m.moveSelection(delta)
	}
	return m.navigateHunk(delta)
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "enter" {
		m.filtering = false
		m.filter.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter() // keeps the selection on the current file when it survives
	dcmd := m.requestDiff()
	return m, tea.Batch(cmd, dcmd)
}

func (m *Model) applyFilter() {
	var prevPath string
	if m.selectedIdx < len(m.filteredFiles) {
		prevPath = m.filteredFiles[m.selectedIdx].Path
	}
	m.filteredFiles = tui.FuzzyFilter(m.files, m.filter.Value(), func(f git.StatusFile) string { return f.Path })
	// Keep the selection on the same file when it survives the new filter; reset to
	// the top otherwise.
	m.selectedIdx = 0
	if prevPath != "" {
		for i, f := range m.filteredFiles {
			if f.Path == prevPath {
				m.selectedIdx = i
				break
			}
		}
	}
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	return m.selectTo(m.selectedIdx + delta)
}

// selectTo moves the file selection to idx (clamped), resets the hunk selection,
// and loads the new file's diff. Shared by the arrow/J/K steps and the vim jump
// keys.
func (m Model) selectTo(idx int) (tea.Model, tea.Cmd) {
	if len(m.filteredFiles) == 0 {
		return m, nil
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.filteredFiles) {
		idx = len(m.filteredFiles) - 1
	}
	if idx == m.selectedIdx {
		return m, nil // already there (or clamped back to the boundary); nothing to reload
	}
	m.selectedIdx = idx
	m.hunkIdx = 0 // reset hunk selection on file change
	cmd := m.requestDiff()
	return m, cmd
}

// hunkCacheKey identifies a file's rendered hunk set. Width is part of the key
// because difftastic bakes the pane width into its output.
func hunkCacheKey(path string, width int) string {
	return fmt.Sprintf("%s\x00%d", path, width)
}

// invalidatePath drops a file's cached hunks (all files' when path is ""), so its
// next load re-renders. Called when a stage/unstage or edit changes the file.
func (m *Model) invalidatePath(path string) {
	if path == "" {
		m.hunkCache = map[string][]displayHunk{}
		return
	}
	delete(m.hunkCache, hunkCacheKey(path, m.viewport.Width()))
}

// requestDiff shows the selected file's hunks: instantly from the width-keyed
// cache when present, otherwise via an async load tagged with a fresh request
// token. The token is always bumped (even on a cache hit) so a slow load for a
// previously-selected file can't land over the current one.
func (m *Model) requestDiff() tea.Cmd {
	m.diffReqID++
	m.flash = "" // a new selection / reload clears any transient message
	if m.cancelDiff != nil {
		// A newer selection or layout supersedes the in-flight load: kill its
		// subprocesses instead of letting them run to a discarded result (a
		// wheel flick would otherwise pile up concurrent difftastic runs).
		m.cancelDiff()
		m.cancelDiff = nil
	}
	if len(m.filteredFiles) == 0 {
		m.setHunks(nil)
		return nil
	}
	f := m.filteredFiles[m.selectedIdx]
	if hunks, ok := m.hunkCache[hunkCacheKey(f.Path, m.viewport.Width())]; ok {
		m.setHunks(hunks)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelDiff = cancel
	return m.loadSelectedDiff(ctx, m.diffReqID)
}

// setHunks installs a rendered hunk set and positions the viewport on the
// selected hunk (or the top when there is none).
func (m *Model) setHunks(hunks []displayHunk) {
	m.diffErr = nil
	m.displayHunks = hunks
	if m.hunkIdx >= len(m.displayHunks) {
		m.hunkIdx = max(0, len(m.displayHunks)-1)
	}
	m.renderHunks()
	if m.hunkIdx < len(m.hunkOffsets) {
		m.viewport.SetYOffset(m.hunkOffsets[m.hunkIdx])
	} else {
		m.viewport.GotoTop()
	}
}

func (m Model) loadSelectedDiff(ctx context.Context, reqID int) tea.Cmd {
	if len(m.filteredFiles) == 0 {
		return nil
	}
	f := m.filteredFiles[m.selectedIdx]
	engine := m.engines.Engine()
	repoRoot := m.repo.Root()
	width := m.viewport.Width()
	return func() tea.Msg {
		untracked := f.StagingStatus == "Untracked" || f.WorktreeStatus == "Untracked"
		color := tui.ColorEnabled()
		var result []displayHunk

		if untracked {
			raw, err := diff.RawNewFileDiff(repoRoot, f.Path)
			if err == nil && raw != "" {
				result = append(result, buildDisplayHunks(
					ctx, engine, raw, f.Path, repoRoot, false, color, width-2,
				)...)
			}
		} else {
			raw, err := diff.RawUnifiedDiff(repoRoot, false, f.Path)
			if err == nil && raw != "" {
				result = append(result, buildDisplayHunks(
					ctx, engine, raw, f.Path, repoRoot, false, color, width-2,
				)...)
			}

			raw, err = diff.RawUnifiedDiff(repoRoot, true, f.Path)
			if err == nil && raw != "" {
				result = append(result, buildDisplayHunks(
					ctx, engine, raw, f.Path, repoRoot, true, color, width-2,
				)...)
			}
		}

		if ctx.Err() != nil {
			return nil // superseded mid-load; the result would be dropped anyway
		}

		// Order by new-side position so staged and unstaged hunks interleave in file
		// order rather than grouping by state — staging one then doesn't jump its row.
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].hunk.NewStart < result[j].hunk.NewStart
		})

		return hunkDiffsMsg{reqID: reqID, path: f.Path, width: width, hunks: result}
	}
}

func buildDisplayHunks(ctx context.Context, engine diff.Engine, raw, path, repoRoot string, staged, color bool, width int) []displayHunk {
	fileDiffs := diff.ParseUnifiedDiff(raw)
	var allHunks []diff.Hunk
	for _, fd := range fileDiffs {
		allHunks = append(allHunks, fd.Hunks...)
	}
	if len(allHunks) == 0 {
		return nil
	}
	base, _ := diff.BaseContent(repoRoot, staged, path)
	rendered := engine.DiffHunks(ctx, allHunks, path, base, color, width)

	var result []displayHunk
	flatIdx := 0
	for _, fd := range fileDiffs {
		for _, h := range fd.Hunks {
			result = append(result, displayHunk{
				fd:       fd,
				hunk:     h,
				rendered: rendered[flatIdx],
				staged:   staged,
			})
			flatIdx++
		}
	}
	return result
}

func (m *Model) renderHunks() {
	if len(m.displayHunks) == 0 {
		m.hunkOffsets = nil
		m.vim.SetContent(&m.viewport, "")
		return
	}

	n := len(m.displayHunks)
	m.hunkOffsets = make([]int, n)
	w := m.viewport.Width()
	innerW := w - 2 // sidebar (bar + space)
	if innerW < 1 {
		innerW = 1
	}

	var b strings.Builder
	lineCount := 0
	for i, dh := range m.displayHunks {
		active := i == m.hunkIdx
		m.hunkOffsets[i] = lineCount

		var sidebar string
		switch {
		case !active:
			sidebar = sidebarInactive
		case dh.staged:
			sidebar = sidebarStaged
		default:
			sidebar = sidebarUnstaged
		}

		// The active hunk gets a bright separator and a ▶ chevron lead-in ("▶─ "
		// is the same width as the default "── ").
		sep, lead := hunkSepDimStyle, "── "
		if active {
			sep, lead = hunkSepStyle, "▶─ "
		}

		label := fmt.Sprintf("%sHunk %d/%d ", lead, i+1, n)
		if dh.staged {
			label += "[staged] "
		}
		if pad := innerW - utf8.RuneCountInString(label); pad > 0 {
			label += strings.Repeat("─", pad)
		}
		b.WriteString(sidebar)
		b.WriteString(sep.Render(label))
		b.WriteString("\n")
		lineCount++

		// Hunk content
		content := dh.rendered
		if innerW > 0 {
			content = ansi.Hardwrap(content, innerW, true)
		}
		for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
			b.WriteString(sidebar)
			b.WriteString(line)
			b.WriteString("\n")
			lineCount++
		}

		// Bottom separator
		bottom := strings.Repeat("─", innerW)
		b.WriteString(sidebar)
		b.WriteString(sep.Render(bottom))
		b.WriteString("\n")
		lineCount++
	}

	m.vim.SetContent(&m.viewport, b.String())
}

func (m Model) View() tea.View {
	if !m.ready {
		return tui.ScreenView("Loading...")
	}

	l := m.layout()

	if m.showHelp {
		return tui.ScreenView(tui.HelpView("stage", tui.HeaderContext(m.branch, m.engines.Name()), m.hints, stageNavKeys, m.width, l.ContentHeight))
	}

	collapsed := m.activePane == diffPane

	// File list body.
	var list strings.Builder
	listInnerH := l.ContentHeight - 2
	if listInnerH < 1 {
		listInnerH = 1
	}
	scrollOffset, listBar := tui.ListWindow(m.selectedIdx, len(m.filteredFiles), listInnerH)
	for i := scrollOffset; i < len(m.filteredFiles) && i-scrollOffset < listInnerH; i++ {
		f := m.filteredFiles[i]
		selected := i == m.selectedIdx
		status := formatStatusShort(f)
		var line string
		if collapsed {
			line = status
			if ic := tui.FileIcon(f.Path); ic != "" {
				line += " " + ic
			}
		} else {
			// IconField is "" (and drops the column) when icons are disabled; the
			// path width tracks its actual width so the layout stays aligned.
			iconField := tui.IconField(f.Path)
			pathW := l.ListWidth - 7 - lipgloss.Width(iconField)
			line = status + " " + iconField + tui.RenderPath(f.Path, pathW, selected)
		}
		list.WriteString(tui.Marker(selected) + line + "\n")
	}

	listTitle, diffTitle := "changes", ""
	if collapsed {
		listTitle = ""
	}
	if len(m.filteredFiles) > 0 {
		diffTitle = m.filteredFiles[m.selectedIdx].Path
	}
	diffBar := tui.ScrollbarFor(&m.viewport)
	listPanel := tui.Panel(listTitle, "", list.String(), l.ListWidth, l.ContentHeight, m.activePane == filePane, listBar)
	diffPanel := tui.Panel(diffTitle, "", m.viewport.View(), l.DiffWidth+2, l.ContentHeight, m.activePane == diffPane, diffBar)
	content := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, diffPanel)

	header := tui.Header("stage", tui.HeaderContext(m.branch, m.engines.Name()), m.width)

	var footer string
	switch {
	case m.filtering:
		footer = tui.FooterContent(m.width, m.filter.View())
	case m.diffErr != nil:
		footer = tui.Footer(m.width, fmt.Sprintf("Error: %v", m.diffErr), nil)
	case m.flash != "":
		footer = tui.Footer(m.width, m.flash, m.hints)
	default:
		status := "No changes found"
		if len(m.filteredFiles) > 0 {
			status = fmt.Sprintf("%d/%d", m.selectedIdx+1, len(m.filteredFiles))
		}
		footer = tui.Footer(m.width, status, m.hints)
	}

	return tui.ScreenView(lipgloss.JoinVertical(lipgloss.Left, header, content, footer))
}

func findFileIndex(files []git.StatusFile, path string) int {
	for i, f := range files {
		if f.Path == path {
			return i
		}
	}
	if len(files) == 0 {
		return 0
	}
	return len(files) - 1
}

func formatStatusShort(f git.StatusFile) string {
	return stagedStyle.Render(git.StatusChar(f.StagingStatus)) + unstagedStyle.Render(git.StatusChar(f.WorktreeStatus))
}
