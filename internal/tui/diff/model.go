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
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/flux/internal/diff"
	"github.com/madhermit/flux/internal/git"
	"github.com/madhermit/flux/internal/tui"
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
	vim         tui.VimNav

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

const collapsedListWidth = 12

func (m Model) layout() layout {
	l := layout{headerHeight: 3}
	l.contentHeight = m.height - l.headerHeight

	if m.activePane == diffPane {
		l.listWidth = collapsedListWidth
	} else {
		l.listWidth = m.width / 3
		if l.listWidth < 20 {
			l.listWidth = 20
		}
		if l.listWidth > 60 {
			l.listWidth = 60
		}
	}
	l.diffWidth = m.width - l.listWidth - 2 // diff pane border
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

	allFiles := make([]git.ChangedFile, 0, len(files)+1)
	allFiles = append(allFiles, git.ChangedFile{Path: "", Status: "All"})
	allFiles = append(allFiles, files...)

	return Model{
		repo:          repo,
		engine:        engine,
		files:         allFiles,
		filteredFiles: allFiles,
		viewport:      viewport.New(0, 0),
		filter:        filter,
		staged:        staged,
		base:          base,
		target:        target,
	}
}

func (m Model) Init() tea.Cmd {
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
	}

	if m.activePane == diffPane {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activePane == diffPane && !m.filtering && m.vim.HandleKey(&m.viewport, msg) {
		return m, nil
	}

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
		return m.applyLayout()
	case tea.KeyEnter:
		if m.activePane == filePane {
			m.activePane = diffPane
			return m.applyLayout()
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

func (m Model) applyLayout() (tea.Model, tea.Cmd) {
	l := m.layout()
	m.viewport.Width = l.diffWidth
	m.viewport.Height = l.contentHeight - 2
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
		return m, tea.Batch(cmd, m.loadSelectedDiff())
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
	width := m.viewport.Width
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
	if w := m.viewport.Width; w > 0 && content != "" {
		content = ansi.Hardwrap(content, w, true)
	}
	m.vim.SetContent(&m.viewport, content)
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
	collapsed := m.activePane == diffPane
	for i := scrollOffset; i < len(m.filteredFiles) && i-scrollOffset < listInnerHeight; i++ {
		f := m.filteredFiles[i]
		var line string
		if f.Path == "" {
			if collapsed {
				line = "*"
			} else {
				line = fmt.Sprintf("* All (%d files)", len(m.filteredFiles)-1)
			}
		} else if collapsed {
			line = statusIcon(f.Status) + " " + tui.FileIcon(f.Path)
		} else {
			line = statusIcon(f.Status) + " " + tui.FileIcon(f.Path) + " " + truncate(f.Path, l.listWidth-8)
		}
		if i == m.selectedIdx {
			fileList.WriteString(selectedFileStyle.Render(line))
		} else {
			fileList.WriteString(fileItemStyle.Render(line))
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
		f := m.filteredFiles[m.selectedIdx]
		label := f.Path
		if label == "" {
			label = "All"
		}
		pct := m.viewport.ScrollPercent() * 100
		status = statusBarStyle.Render(fmt.Sprintf("%s  %.0f%%  [%d/%d]  q:quit /filter tab:switch j/k:nav gg/G:top/bot {/}:section", label, pct, m.selectedIdx+1, len(m.filteredFiles)))
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
