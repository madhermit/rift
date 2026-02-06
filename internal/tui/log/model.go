package logui

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
	"github.com/sahilm/fuzzy"
)

type pane int

const (
	commitPane pane = iota
	diffPane
)

type Model struct {
	repo   *git.Repo
	engine diff.Engine

	commits         []git.CommitInfo
	filteredCommits []git.CommitInfo
	selectedIdx     int
	activePane      pane

	viewport  viewport.Model
	filter    textinput.Model
	filtering bool

	diffContent string
	diffErr     error

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
	if l.listWidth < 30 {
		l.listWidth = 30
	}
	if l.listWidth > 80 {
		l.listWidth = 80
	}
	// -2 for the diff pane border (list border handled in Width(listWidth-2))
	l.diffWidth = m.width - l.listWidth - 2
	if l.diffWidth < 10 {
		l.diffWidth = 10
	}
	return l
}

func New(repo *git.Repo, engine diff.Engine, commits []git.CommitInfo) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.PromptStyle = filterPromptStyle
	filter.CharLimit = 256

	return Model{
		repo:            repo,
		engine:          engine,
		commits:         commits,
		filteredCommits: commits,
		viewport:        viewport.New(0, 0),
		filter:          filter,
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
		wasReady := m.ready
		m.ready = true
		l := m.layout()
		m.viewport.Width = l.diffWidth
		m.viewport.Height = l.contentHeight - 2
		m.setDiffContent()
		if !wasReady && len(m.filteredCommits) > 0 {
			return m, m.loadCommitDiff(m.filteredCommits[0])
		}
		return m, nil
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
		if m.activePane == commitPane {
			m.activePane = diffPane
		} else {
			m.activePane = commitPane
		}
		return m, nil
	case tea.KeyEnter:
		if m.activePane == commitPane {
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
	if m.activePane == commitPane {
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

	if len(m.filteredCommits) > 0 {
		m.selectedIdx = 0
		return m, tea.Batch(cmd, m.loadCommitDiff(m.filteredCommits[0]))
	}
	m.selectedIdx = 0
	m.diffContent = ""
	m.viewport.SetContent("")
	return m, cmd
}

func (m *Model) applyFilter() {
	query := m.filter.Value()
	if query == "" {
		m.filteredCommits = m.commits
		m.selectedIdx = 0
		return
	}

	targets := make([]string, len(m.commits))
	for i, c := range m.commits {
		targets[i] = c.Hash + " " + c.Message
	}

	matches := fuzzy.Find(query, targets)
	filtered := make([]git.CommitInfo, len(matches))
	for i, match := range matches {
		filtered[i] = m.commits[match.Index]
	}
	m.filteredCommits = filtered
	m.selectedIdx = 0
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if len(m.filteredCommits) == 0 {
		return m, nil
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(m.filteredCommits) {
		m.selectedIdx = len(m.filteredCommits) - 1
	}
	return m, m.loadCommitDiff(m.filteredCommits[m.selectedIdx])
}

func (m Model) loadCommitDiff(commit git.CommitInfo) tea.Cmd {
	width := m.viewport.Width
	return func() tea.Msg {
		color := os.Getenv("NO_COLOR") == ""
		content, err := m.engine.DiffCommit(
			context.Background(), m.repo.Root(),
			commit.Hash+"~1", commit.Hash, color, width,
		)
		if err != nil {
			// First commit has no parent — diff against empty tree
			content, err = m.engine.DiffCommit(
				context.Background(), m.repo.Root(),
				"4b825dc642cb6eb9a060e54bf899d69f82cf7207", commit.Hash, color, width,
			)
		}
		return diffLoadedMsg{content: content, err: err}
	}
}

func (m *Model) setDiffContent() {
	w := m.viewport.Width
	if w <= 0 || m.diffContent == "" {
		m.viewport.SetContent(m.diffContent)
		return
	}
	m.viewport.SetContent(ansi.Hardwrap(m.diffContent, w, true))
}

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	l := m.layout()

	title := titleStyle.Render(fmt.Sprintf("git-flux log  [%s]", m.engine.Name()))

	// Commit list with scroll
	var commitList strings.Builder
	listInnerHeight := l.contentHeight - 2
	scrollOffset := 0
	if m.selectedIdx >= listInnerHeight {
		scrollOffset = m.selectedIdx - listInnerHeight + 1
	}
	for i := scrollOffset; i < len(m.filteredCommits) && i-scrollOffset < listInnerHeight; i++ {
		c := m.filteredCommits[i]
		line := truncate(c.Hash+" "+c.Message, l.listWidth-6)
		if i == m.selectedIdx {
			commitList.WriteString(selectedCommitStyle.Width(l.listWidth - 4).Render(line))
		} else {
			msg := truncate(c.Message, l.listWidth-6-len(c.Hash)-1)
			commitList.WriteString(commitItemStyle.Render(
				hashStyle.Render(c.Hash) + " " + msg,
			))
		}
		commitList.WriteString("\n")
	}

	// Pane rendering
	listStyle, vpStyle := paneStyle, paneStyle
	if m.activePane == commitPane {
		listStyle = activePaneStyle
	} else {
		vpStyle = activePaneStyle
	}
	listPane := listStyle.Width(l.listWidth - 2).Height(l.contentHeight - 2).Render(commitList.String())
	diffPaneView := vpStyle.Width(l.diffWidth).Height(l.contentHeight - 2).Render(m.viewport.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, listPane, diffPaneView)

	// Status bar
	var status string
	switch {
	case m.filtering:
		status = m.filter.View()
	case m.diffErr != nil:
		status = statusBarStyle.Render(fmt.Sprintf("Error: %v", m.diffErr))
	case len(m.filteredCommits) > 0:
		c := m.filteredCommits[m.selectedIdx]
		pct := m.viewport.ScrollPercent() * 100
		status = statusBarStyle.Render(fmt.Sprintf(
			"%s %s  %.0f%%  [%d/%d commits]  q:quit /filter tab:switch j/k:nav",
			c.Hash, c.Date, pct, m.selectedIdx+1, len(m.filteredCommits),
		))
	default:
		status = statusBarStyle.Render("No commits found")
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
	return s[:max-3] + "..."
}
