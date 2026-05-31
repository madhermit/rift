package stageui

import (
	"context"
	"fmt"
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
	repo   *git.Repo
	engine diff.Engine

	files         []git.StatusFile
	filteredFiles []git.StatusFile
	selectedIdx   int
	activePane    pane

	displayHunks []displayHunk // combined staged + unstaged hunks
	hunkOffsets  []int         // viewport line offset where each hunk starts
	hunkIdx      int           // selected hunk index

	viewport  viewport.Model
	filter    textinput.Model
	filtering bool

	diffErr        error
	vim            tui.VimNav
	skipDiffReload bool // after hunk stage/unstage, only reload file list
	showHelp       bool

	width  int
	height int
	ready  bool
}

var stageHints = [][2]string{
	{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"},
	{"s", "stage"}, {"u", "unstage"}, {"a", "all"}, {"n/p", "hunk"}, {"?", "help"}, {"q", "quit"},
}

type hunkDiffsMsg struct {
	hunks []displayHunk
}

type filesLoadedMsg struct {
	files []git.StatusFile
	err   error
}

type stageResultMsg struct {
	err     error
	hunkIdx int  // which hunk was staged/unstaged (-1 for file-level)
	staged  bool // true if staged, false if unstaged
}

func (m Model) layout() tui.SplitLayout {
	return tui.ComputeSplitLayout(m.width, m.height, m.activePane == diffPane, 20, 60)
}

func New(repo *git.Repo, engine diff.Engine, files []git.StatusFile) Model {
	return Model{
		repo:          repo,
		engine:        engine,
		files:         files,
		filteredFiles: files,
		viewport:      viewport.New(),
		filter:        tui.NewFilterInput(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m.applyLayout()
	case hunkDiffsMsg:
		m.diffErr = nil
		m.displayHunks = msg.hunks
		if m.hunkIdx >= len(m.displayHunks) {
			m.hunkIdx = max(0, len(m.displayHunks)-1)
		}
		m.renderHunks()
		if m.hunkIdx < len(m.hunkOffsets) {
			m.viewport.SetYOffset(m.hunkOffsets[m.hunkIdx])
		} else {
			m.viewport.GotoTop()
		}
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
		if m.skipDiffReload {
			m.skipDiffReload = false
			return m, nil
		}
		if len(m.filteredFiles) > 0 {
			return m, m.loadSelectedDiff()
		}
		m.diffErr = nil
		m.displayHunks = nil
		m.viewport.SetContent("")
		return m, nil
	case stageResultMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		if msg.hunkIdx >= 0 && msg.hunkIdx < len(m.displayHunks) {
			// Hunk-level: toggle in-place, skip diff reload
			m.displayHunks[msg.hunkIdx].staged = msg.staged
			m.renderHunks()
			m.skipDiffReload = true
		}
		// File-level (hunkIdx == -1): full reload
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
		return m, tea.Quit
	}

	if m.filtering {
		return m.handleFilterKey(msg)
	}

	switch msg.String() {
	case "?":
		m.showHelp = true
		return m, nil
	case "tab":
		if m.activePane == filePane {
			m.activePane = diffPane
		} else {
			m.activePane = filePane
		}
		return m.applyLayout()
	case "enter":
		if m.activePane == filePane {
			m.activePane = diffPane
			return m.applyLayout()
		}
	case "up", "k":
		return m.navigate(-1)
	case "down", "j":
		return m.navigate(1)
	case "q":
		return m, tea.Quit
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, nil
	case "s":
		return m.stageOrUnstage(true)
	case "u":
		return m.stageOrUnstage(false)
	case "a":
		if m.activePane == filePane {
			return m.stageAll()
		}
	case "}", "n":
		if m.activePane == diffPane {
			return m.navigateHunk(1)
		}
	case "{", "p":
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

func (m Model) navigateHunk(delta int) (tea.Model, tea.Cmd) {
	n := len(m.displayHunks)
	if n == 0 {
		return m, nil
	}
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
		repo := m.repo
		idx := m.hunkIdx
		return m, func() tea.Msg {
			var err error
			if stage {
				err = repo.StageHunk(patch)
			} else {
				err = repo.UnstageHunk(patch)
			}
			if err != nil {
				return stageResultMsg{err: err, hunkIdx: -1}
			}
			return stageResultMsg{hunkIdx: idx, staged: stage}
		}
	}

	// File-level: unstage requires something staged
	if !stage && (f.StagingStatus == "" || f.StagingStatus == "Untracked") {
		return m, nil
	}
	repo := m.repo
	path := f.Path
	return m, func() tea.Msg {
		var err error
		if stage {
			err = repo.Stage(path)
		} else {
			err = repo.Unstage(path)
		}
		if err != nil {
			return stageResultMsg{err: err, hunkIdx: -1}
		}
		return stageResultMsg{hunkIdx: -1, staged: stage}
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
			return stageResultMsg{err: err, hunkIdx: -1}
		}
		return stageResultMsg{hunkIdx: -1, staged: true}
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

func (m Model) applyLayout() (tea.Model, tea.Cmd) {
	l := m.layout()
	m.viewport.SetWidth(l.DiffWidth)
	m.viewport.SetHeight(l.ContentHeight - 2)
	m.renderHunks()
	if len(m.filteredFiles) > 0 {
		return m, m.loadSelectedDiff()
	}
	return m, nil
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
	m.applyFilter()
	m.selectedIdx = 0

	if len(m.filteredFiles) > 0 {
		return m, tea.Batch(cmd, m.loadSelectedDiff())
	}
	m.viewport.SetContent("")
	return m, cmd
}

func (m *Model) applyFilter() {
	m.filteredFiles = tui.FuzzyFilter(m.files, m.filter.Value(), func(f git.StatusFile) string { return f.Path })
	m.selectedIdx = 0
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if len(m.filteredFiles) == 0 {
		return m, nil
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(m.filteredFiles) {
		m.selectedIdx = len(m.filteredFiles) - 1
	}
	m.hunkIdx = 0 // reset hunk selection on file change
	return m, m.loadSelectedDiff()
}

func (m Model) loadSelectedDiff() tea.Cmd {
	if len(m.filteredFiles) == 0 {
		return nil
	}
	f := m.filteredFiles[m.selectedIdx]
	engine := m.engine
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
					engine, raw, f.Path, repoRoot, false, color, width-2,
				)...)
			}
		} else {
			raw, err := diff.RawUnifiedDiff(repoRoot, false, f.Path)
			if err == nil && raw != "" {
				result = append(result, buildDisplayHunks(
					engine, raw, f.Path, repoRoot, false, color, width-2,
				)...)
			}

			raw, err = diff.RawUnifiedDiff(repoRoot, true, f.Path)
			if err == nil && raw != "" {
				result = append(result, buildDisplayHunks(
					engine, raw, f.Path, repoRoot, true, color, width-2,
				)...)
			}
		}

		return hunkDiffsMsg{hunks: result}
	}
}

func buildDisplayHunks(engine diff.Engine, raw, path, repoRoot string, staged, color bool, width int) []displayHunk {
	fileDiffs := diff.ParseUnifiedDiff(raw)
	var allHunks []diff.Hunk
	for _, fd := range fileDiffs {
		allHunks = append(allHunks, fd.Hunks...)
	}
	if len(allHunks) == 0 {
		return nil
	}
	base, _ := diff.BaseContent(repoRoot, staged, path)
	rendered := engine.DiffHunks(context.Background(), allHunks, path, base, color, width)

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
	innerW := w - 2 // sidebar (▎ + space)
	if innerW < 1 {
		innerW = 1
	}

	var b strings.Builder
	lineCount := 0
	for i, dh := range m.displayHunks {
		active := i == m.hunkIdx
		m.hunkOffsets[i] = lineCount

		sidebar := sidebarUnstaged
		if dh.staged {
			sidebar = sidebarStaged
		}
		if !active {
			sidebar = sidebarInactive
		}

		sep := hunkSepDimStyle
		if active {
			sep = hunkSepStyle
		}

		// Top separator
		label := fmt.Sprintf("── Hunk %d/%d ", i+1, n)
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
		return tea.NewView("Loading...")
	}

	l := m.layout()

	if m.showHelp {
		v := tea.NewView(tui.HelpView("stage", m.engine.Name(), stageHints, tui.PreviewHelpKeys, m.width, l.ContentHeight))
		v.AltScreen = true
		return v
	}

	collapsed := m.activePane == diffPane

	// File list body.
	var list strings.Builder
	listInnerH := l.ContentHeight - 2
	if listInnerH < 1 {
		listInnerH = 1
	}
	scrollOffset := 0
	if m.selectedIdx >= listInnerH {
		scrollOffset = m.selectedIdx - listInnerH + 1
	}
	for i := scrollOffset; i < len(m.filteredFiles) && i-scrollOffset < listInnerH; i++ {
		f := m.filteredFiles[i]
		selected := i == m.selectedIdx
		status := formatStatusShort(f)
		textStyle := tui.NormalTextStyle
		if selected {
			textStyle = tui.SelectedTextStyle
		}
		var line string
		if collapsed {
			line = status + " " + tui.FileIcon(f.Path)
		} else {
			line = status + " " + tui.FileIcon(f.Path) + " " + textStyle.Render(tui.TruncatePath(f.Path, l.ListWidth-9))
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
	listPanel := tui.Panel(listTitle, list.String(), l.ListWidth, l.ContentHeight, m.activePane == filePane)
	diffPanel := tui.Panel(diffTitle, m.viewport.View(), l.DiffWidth+2, l.ContentHeight, m.activePane == diffPane)
	content := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, diffPanel)

	header := tui.Header("stage", m.engine.Name(), m.width)

	var footer string
	switch {
	case m.filtering:
		footer = tui.FooterContent(m.width, m.filter.View())
	case m.diffErr != nil:
		footer = tui.Footer(m.width, fmt.Sprintf("Error: %v", m.diffErr), nil)
	default:
		status := "No changes found"
		if len(m.filteredFiles) > 0 {
			status = fmt.Sprintf("%d/%d", m.selectedIdx+1, len(m.filteredFiles))
		}
		footer = tui.Footer(m.width, status, stageHints)
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, content, footer))
	v.AltScreen = true
	return v
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
