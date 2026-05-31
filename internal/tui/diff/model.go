package diffui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

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

type Model struct {
	repo   *git.Repo
	engine diff.Engine

	files         []git.ChangedFile
	filteredFiles []git.ChangedFile
	selectedIdx   int
	activePane    pane

	viewport  viewport.Model
	filter    textinput.Model
	filtering bool

	diffContent string
	diffErr     error
	vim         tui.VimNav

	staged     bool
	base       string
	target     string
	commitDiff bool

	width  int
	height int
	ready  bool
}

type diffLoadedMsg struct {
	content string
	err     error
}

type filesLoadedMsg struct {
	files []git.ChangedFile
	err   error
}

func (m Model) layout() tui.SplitLayout {
	return tui.ComputeSplitLayout(m.width, m.height, m.activePane == diffPane, 20, 60)
}

func prependAllEntry(files []git.ChangedFile) []git.ChangedFile {
	all := make([]git.ChangedFile, 0, len(files)+1)
	all = append(all, git.ChangedFile{Path: "", Status: "All"})
	return append(all, files...)
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, staged bool, base, target string) Model {
	allFiles := prependAllEntry(files)

	return Model{
		repo:          repo,
		engine:        engine,
		files:         allFiles,
		filteredFiles: allFiles,
		viewport:      viewport.New(),
		filter:        tui.NewFilterInput(),
		staged:        staged,
		base:          base,
		target:        target,
		commitDiff:    target != "",
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
	case diffLoadedMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			m.diffContent = ""
		} else {
			m.diffErr = nil
			m.diffContent = msg.content
		}
		m.setDiffContent()
		m.viewport.GotoTop()
		return m, nil
	case filesLoadedMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		m.files = prependAllEntry(msg.files)
		m.applyFilter()
		m.diffContent = ""
		m.diffErr = nil
		m.setDiffContent()
		if len(m.filteredFiles) > 0 {
			return m, m.loadSelectedDiff()
		}
		return m, nil
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.activePane == diffPane && !m.filtering && m.vim.HandleKey(&m.viewport, msg) {
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
			m.applyFilter()
			return m, nil
		}
		return m, tea.Quit
	}

	if m.filtering {
		return m.handleFilterKey(msg)
	}

	switch msg.String() {
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
		if !m.commitDiff {
			return m.toggleStaged()
		}
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) applyLayout() (tea.Model, tea.Cmd) {
	l := m.layout()
	m.viewport.SetWidth(l.DiffWidth)
	m.viewport.SetHeight(l.ContentHeight - 2)
	m.setDiffContent()
	if len(m.filteredFiles) > 0 {
		return m, m.loadSelectedDiff()
	}
	return m, nil
}

func (m Model) navigate(delta int) (tea.Model, tea.Cmd) {
	if m.activePane == filePane {
		return m.moveSelection(delta)
	}
	if delta > 0 {
		m.viewport.ScrollDown(1)
	} else {
		m.viewport.ScrollUp(1)
	}
	return m, nil
}

func (m Model) toggleStaged() (tea.Model, tea.Cmd) {
	m.staged = !m.staged
	repo := m.repo
	staged := m.staged
	return m, func() tea.Msg {
		files, err := repo.ChangedFiles(staged)
		if err != nil {
			return filesLoadedMsg{err: err}
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
		return filesLoadedMsg{files: files}
	}
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

	if len(m.filteredFiles) > 0 {
		m.selectedIdx = 0
		return m, tea.Batch(cmd, m.loadSelectedDiff())
	}
	m.selectedIdx = 0
	m.diffContent = ""
	m.viewport.SetContent("")
	return m, cmd
}

func (m *Model) applyFilter() {
	m.filteredFiles = tui.FuzzyFilter(m.files, m.filter.Value(), func(f git.ChangedFile) string { return f.Path })
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
	return m, m.loadSelectedDiff()
}

func (m Model) loadSelectedDiff() tea.Cmd {
	selected := m.filteredFiles[m.selectedIdx]
	if selected.Path == "" {
		var files []string
		for _, f := range m.filteredFiles {
			if f.Path != "" {
				files = append(files, f.Path)
			}
		}
		return m.loadDiff(files...)
	}
	return m.loadDiff(selected.Path)
}

func (m Model) loadDiff(files ...string) tea.Cmd {
	width := m.viewport.Width()
	return func() tea.Msg {
		color := os.Getenv("NO_COLOR") == ""
		opts := diff.DiffOpts{
			Staged: m.staged,
			Base:   m.base,
			Target: m.target,
			Color:  color,
			Width:  width,
		}
		var result strings.Builder
		for _, file := range files {
			content, err := m.engine.Diff(context.Background(), m.repo.Root(), file, opts)
			if err != nil {
				continue
			}
			if content != "" {
				result.WriteString(content)
				result.WriteString("\n")
			}
		}
		return diffLoadedMsg{content: result.String()}
	}
}

func (m *Model) setDiffContent() {
	content := m.diffContent
	if w := m.viewport.Width(); w > 0 && content != "" {
		content = ansi.Hardwrap(content, w, true)
	}
	m.vim.SetContent(&m.viewport, content)
}

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("Loading...")
	}

	l := m.layout()
	collapsed := m.activePane == diffPane

	// File list body.
	var list strings.Builder
	listInnerH := l.ContentHeight - 2
	scrollOffset := 0
	if m.selectedIdx >= listInnerH {
		scrollOffset = m.selectedIdx - listInnerH + 1
	}
	for i := scrollOffset; i < len(m.filteredFiles) && i-scrollOffset < listInnerH; i++ {
		f := m.filteredFiles[i]
		selected := i == m.selectedIdx
		textStyle := tui.NormalTextStyle
		if selected {
			textStyle = tui.SelectedTextStyle
		}
		var line string
		switch {
		case f.Path == "" && collapsed:
			line = textStyle.Render("∗")
		case f.Path == "":
			line = textStyle.Render(fmt.Sprintf("∗ All (%d files)", len(m.filteredFiles)-1))
		case collapsed:
			line = tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " + tui.FileIcon(f.Path)
		default:
			line = tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " +
				tui.FileIcon(f.Path) + " " + textStyle.Render(tui.TruncatePath(f.Path, l.ListWidth-8))
		}
		list.WriteString(tui.Marker(selected) + line + "\n")
	}

	listTitle := "changes"
	if !m.commitDiff {
		listTitle = "unstaged"
		if m.staged {
			listTitle = "staged"
		}
	}
	diffTitle := ""
	if collapsed {
		listTitle = ""
	}
	if len(m.filteredFiles) > 0 {
		if diffTitle = m.filteredFiles[m.selectedIdx].Path; diffTitle == "" {
			diffTitle = "All"
		}
	}
	listPanel := tui.Panel(listTitle, list.String(), l.ListWidth, l.ContentHeight, m.activePane == filePane)
	diffPanel := tui.Panel(diffTitle, m.viewport.View(), l.DiffWidth+2, l.ContentHeight, m.activePane == diffPane)
	content := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, diffPanel)

	header := tui.Header("diff", m.engine.Name(), m.width)

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
			if collapsed {
				status += fmt.Sprintf(" · %.0f%%", m.viewport.ScrollPercent()*100)
			}
		}
		hints := [][2]string{
			{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"}, {"gg/G", "top/bot"}, {"{/}", "section"},
		}
		if !m.commitDiff {
			hints = append(hints, [2]string{"s", "staged"})
		}
		hints = append(hints, [2]string{"q", "quit"})
		footer = tui.Footer(m.width, status, hints)
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, content, footer))
	v.AltScreen = true
	return v
}
