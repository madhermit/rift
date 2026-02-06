package diffui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhermit/flux/internal/diff"
	"github.com/madhermit/flux/internal/git"
	"github.com/sahilm/fuzzy"
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

	staged bool
	base   string
	target string

	width  int
	height int
	ready  bool
}

type diffLoadedMsg struct {
	content string
	err     error
}

type layout struct {
	headerHeight  int
	contentHeight int
	listWidth     int
	diffWidth     int
}

func (m Model) layout() layout {
	l := layout{headerHeight: 3}
	l.contentHeight = m.height - l.headerHeight

	l.listWidth = m.width / 3
	if l.listWidth < 20 {
		l.listWidth = 20
	}
	if l.listWidth > 60 {
		l.listWidth = 60
	}
	l.diffWidth = m.width - l.listWidth - 4 // borders
	if l.diffWidth < 10 {
		l.diffWidth = 10
	}
	return l
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, staged bool, base, target string) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.PromptStyle = filterPromptStyle
	filter.CharLimit = 256

	return Model{
		repo:          repo,
		engine:        engine,
		files:         files,
		filteredFiles: files,
		viewport:      viewport.New(0, 0),
		filter:        filter,
		staged:        staged,
		base:          base,
		target:        target,
	}
}

func (m Model) Init() tea.Cmd {
	if len(m.filteredFiles) > 0 {
		return m.loadDiff(m.filteredFiles[0].Path)
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		l := m.layout()
		m.viewport.Width = l.diffWidth
		m.viewport.Height = l.contentHeight - 2
		return m, nil
	case diffLoadedMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			m.diffContent = ""
		} else {
			m.diffErr = nil
			m.diffContent = msg.content
		}
		m.viewport.SetContent(m.diffContent)
		m.viewport.GotoTop()
		return m, nil
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
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

	switch msg.Type {
	case tea.KeyTab:
		if m.activePane == filePane {
			m.activePane = diffPane
		} else {
			m.activePane = filePane
		}
		return m, nil
	case tea.KeyEnter:
		if m.activePane == filePane {
			m.activePane = diffPane
			return m, nil
		}
	case tea.KeyUp:
		return m.navigate(-1)
	case tea.KeyDown:
		return m.navigate(1)
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			return m, tea.Quit
		case "/":
			m.filtering = true
			m.filter.Focus()
			return m, nil
		case "j":
			return m.navigate(1)
		case "k":
			return m.navigate(-1)
		}
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
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

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		m.filtering = false
		m.filter.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter()

	if len(m.filteredFiles) > 0 {
		m.selectedIdx = 0
		return m, tea.Batch(cmd, m.loadDiff(m.filteredFiles[0].Path))
	}
	m.selectedIdx = 0
	m.diffContent = ""
	m.viewport.SetContent("")
	return m, cmd
}

func (m *Model) applyFilter() {
	query := m.filter.Value()
	if query == "" {
		m.filteredFiles = m.files
		m.selectedIdx = 0
		return
	}

	paths := make([]string, len(m.files))
	for i, f := range m.files {
		paths[i] = f.Path
	}

	matches := fuzzy.Find(query, paths)
	filtered := make([]git.ChangedFile, len(matches))
	for i, match := range matches {
		filtered[i] = m.files[match.Index]
	}
	m.filteredFiles = filtered
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
	return m, m.loadDiff(m.filteredFiles[m.selectedIdx].Path)
}

func (m Model) loadDiff(file string) tea.Cmd {
	return func() tea.Msg {
		color := os.Getenv("NO_COLOR") == ""
		content, err := m.engine.Diff(context.Background(), m.repo.Root(), file, diff.DiffOpts{
			Staged: m.staged,
			Base:   m.base,
			Target: m.target,
			Color:  color,
		})
		return diffLoadedMsg{content: content, err: err}
	}
}

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	l := m.layout()

	title := titleStyle.Render(fmt.Sprintf("git-flux diff  [%s]", m.engine.Name()))

	// File list with scroll
	var fileList strings.Builder
	listInnerHeight := l.contentHeight - 2
	scrollOffset := 0
	if m.selectedIdx >= listInnerHeight {
		scrollOffset = m.selectedIdx - listInnerHeight + 1
	}
	for i := scrollOffset; i < len(m.filteredFiles) && i-scrollOffset < listInnerHeight; i++ {
		f := m.filteredFiles[i]
		line := truncate(f.Path, l.listWidth-6)
		if i == m.selectedIdx {
			fileList.WriteString(selectedFileStyle.Width(l.listWidth - 4).Render(line))
		} else {
			fileList.WriteString(fileItemStyle.Render(statusIcon(f.Status) + " " + line))
		}
		fileList.WriteString("\n")
	}

	// Pane rendering — active pane gets accent border
	listStyle, vpStyle := paneStyle, paneStyle
	if m.activePane == filePane {
		listStyle = activePaneStyle
	} else {
		vpStyle = activePaneStyle
	}
	listPane := listStyle.Width(l.listWidth - 2).Height(l.contentHeight - 2).Render(fileList.String())
	diffPaneView := vpStyle.Width(l.diffWidth).Height(l.contentHeight - 2).Render(m.viewport.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, listPane, diffPaneView)

	// Status bar
	var status string
	switch {
	case m.filtering:
		status = m.filter.View()
	case m.diffErr != nil:
		status = statusBarStyle.Render(fmt.Sprintf("Error: %v", m.diffErr))
	case len(m.filteredFiles) > 0:
		file := m.filteredFiles[m.selectedIdx].Path
		pct := m.viewport.ScrollPercent() * 100
		status = statusBarStyle.Render(fmt.Sprintf("%s  %.0f%%  [%d/%d files]  q:quit /filter tab:switch j/k:nav", file, pct, m.selectedIdx+1, len(m.filteredFiles)))
	default:
		status = statusBarStyle.Render("No changes found")
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, content, status)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return "..." + s[len(s)-max+3:]
}

func statusIcon(status string) string {
	switch status {
	case "Modified":
		return "M"
	case "Added":
		return "A"
	case "Deleted":
		return "D"
	case "Renamed":
		return "R"
	case "Untracked":
		return "?"
	default:
		return " "
	}
}
