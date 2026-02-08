package stashui

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
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
	"github.com/sahilm/fuzzy"
)

type StashAction int

const (
	NoAction StashAction = iota
	Apply
	Pop
	Drop
)

type pane int

const (
	listPane pane = iota
	diffPane
)

type Model struct {
	repo   *git.Repo
	engine diff.Engine

	stashes         []git.StashEntry
	filteredStashes []git.StashEntry
	selectedIdx     int
	activePane      pane

	viewport  viewport.Model
	filter    textinput.Model
	filtering bool

	diffContent string
	diffErr     error
	vim         tui.VimNav

	action StashAction

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
		if l.listWidth < 30 {
			l.listWidth = 30
		}
		if l.listWidth > 80 {
			l.listWidth = 80
		}
	}
	l.diffWidth = m.width - l.listWidth - 2
	if l.diffWidth < 10 {
		l.diffWidth = 10
	}
	return l
}

func New(repo *git.Repo, engine diff.Engine, stashes []git.StashEntry) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.PromptStyle = filterPromptStyle
	filter.CharLimit = 256

	return Model{
		repo:            repo,
		engine:          engine,
		stashes:         stashes,
		filteredStashes: stashes,
		viewport:        viewport.New(0, 0),
		filter:          filter,
	}
}

func (m Model) Action() StashAction { return m.action }

func (m Model) SelectedIndex() int {
	if len(m.filteredStashes) == 0 {
		return -1
	}
	return m.filteredStashes[m.selectedIdx].Index
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
		if m.activePane == listPane {
			m.activePane = diffPane
		} else {
			m.activePane = listPane
		}
		return m.applyLayout()
	case tea.KeyEnter:
		if m.activePane == listPane {
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
		case "a":
			if len(m.filteredStashes) > 0 {
				m.action = Apply
				return m, tea.Quit
			}
		case "p":
			if len(m.filteredStashes) > 0 {
				m.action = Pop
				return m, tea.Quit
			}
		case "x":
			if len(m.filteredStashes) > 0 {
				m.action = Drop
				return m, tea.Quit
			}
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
	if len(m.filteredStashes) > 0 {
		return m, m.loadStashDiff(m.filteredStashes[m.selectedIdx])
	}
	return m, nil
}

func (m Model) navigate(delta int) (tea.Model, tea.Cmd) {
	if m.activePane == listPane {
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

	if len(m.filteredStashes) > 0 {
		m.selectedIdx = 0
		return m, tea.Batch(cmd, m.loadStashDiff(m.filteredStashes[0]))
	}
	m.selectedIdx = 0
	m.diffContent = ""
	m.viewport.SetContent("")
	return m, cmd
}

func (m *Model) applyFilter() {
	query := m.filter.Value()
	if query == "" {
		m.filteredStashes = m.stashes
		m.selectedIdx = 0
		return
	}

	targets := make([]string, len(m.stashes))
	for i, s := range m.stashes {
		targets[i] = fmt.Sprintf("stash@{%d} %s %s", s.Index, s.Branch, s.Message)
	}

	matches := fuzzy.Find(query, targets)
	filtered := make([]git.StashEntry, len(matches))
	for i, match := range matches {
		filtered[i] = m.stashes[match.Index]
	}
	m.filteredStashes = filtered
	m.selectedIdx = 0
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if len(m.filteredStashes) == 0 {
		return m, nil
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(m.filteredStashes) {
		m.selectedIdx = len(m.filteredStashes) - 1
	}
	return m, m.loadStashDiff(m.filteredStashes[m.selectedIdx])
}

func (m Model) loadStashDiff(entry git.StashEntry) tea.Cmd {
	width := m.viewport.Width
	return func() tea.Msg {
		ref := fmt.Sprintf("stash@{%d}", entry.Index)
		base := ref + "^"
		color := os.Getenv("NO_COLOR") == ""
		content, err := m.engine.DiffCommit(
			context.Background(), m.repo.Root(),
			base, ref, color, width,
		)
		if err != nil {
			return diffLoadedMsg{err: err}
		}
		return diffLoadedMsg{content: content}
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

	title := titleStyle.Render(fmt.Sprintf("rift stash  [%s]", m.engine.Name()))

	// Stash list with scroll
	var stashList strings.Builder
	collapsed := m.activePane == diffPane
	listInnerHeight := l.contentHeight - 2
	scrollOffset := 0
	if m.selectedIdx >= listInnerHeight {
		scrollOffset = m.selectedIdx - listInnerHeight + 1
	}
	for i := scrollOffset; i < len(m.filteredStashes) && i-scrollOffset < listInnerHeight; i++ {
		s := m.filteredStashes[i]
		var line string
		if collapsed {
			line = fmt.Sprintf("{%d}", s.Index)
		} else {
			line = truncate(fmt.Sprintf("stash@{%d} %s", s.Index, s.Message), l.listWidth-6)
		}
		if i == m.selectedIdx {
			stashList.WriteString(selectedItemStyle.Render(line))
		} else {
			stashList.WriteString(itemStyle.Render(line))
		}
		stashList.WriteString("\n")
	}

	// Pane rendering
	listStyle, vpStyle := paneStyle, paneStyle
	if m.activePane == listPane {
		listStyle = activePaneStyle
	} else {
		vpStyle = activePaneStyle
	}
	listPaneView := listStyle.Width(l.listWidth - 2).Height(l.contentHeight - 2).Render(stashList.String())
	diffPaneView := vpStyle.Width(l.diffWidth).Height(l.contentHeight - 2).Render(m.viewport.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, listPaneView, diffPaneView)

	// Status bar
	var status string
	switch {
	case m.filtering:
		status = m.filter.View()
	case m.diffErr != nil:
		status = statusBarStyle.Render(fmt.Sprintf("Error: %v", m.diffErr))
	case len(m.filteredStashes) > 0:
		s := m.filteredStashes[m.selectedIdx]
		pct := m.viewport.ScrollPercent() * 100
		status = statusBarStyle.Render(fmt.Sprintf(
			"stash@{%d} %s  %.0f%%  [%d/%d]  q:quit /filter tab:switch j/k:nav a:apply p:pop x:drop gg/G:top/bot",
			s.Index, s.Branch, pct, m.selectedIdx+1, len(m.filteredStashes),
		))
	default:
		status = statusBarStyle.Render("No stashes found")
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
