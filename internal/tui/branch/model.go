package branchui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhermit/flux/internal/git"
	"github.com/sahilm/fuzzy"
)

type Model struct {
	branches    []git.BranchInfo
	filtered    []git.BranchInfo
	selectedIdx int
	checkout    string

	filter    textinput.Model
	filtering bool

	width  int
	height int
	ready  bool
}

func (m Model) Checkout() string {
	return m.checkout
}

func New(branches []git.BranchInfo) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.PromptStyle = filterPromptStyle
	filter.CharLimit = 256

	return Model{
		branches: branches,
		filtered: branches,
		filter:   filter,
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
		return m, nil
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
	case tea.KeyEnter:
		if len(m.filtered) > 0 {
			b := m.filtered[m.selectedIdx]
			if !b.Current {
				m.checkout = b.Name
				return m, tea.Quit
			}
		}
	case tea.KeyUp:
		m.moveSelection(-1)
		return m, nil
	case tea.KeyDown:
		m.moveSelection(1)
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			return m, tea.Quit
		case "/":
			m.filtering = true
			m.filter.Focus()
			return m, nil
		case "j":
			m.moveSelection(1)
			return m, nil
		case "k":
			m.moveSelection(-1)
			return m, nil
		}
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
	return m, cmd
}

func (m *Model) applyFilter() {
	query := m.filter.Value()
	if query == "" {
		m.filtered = m.branches
		m.selectedIdx = 0
		return
	}

	names := make([]string, len(m.branches))
	for i, b := range m.branches {
		names[i] = b.Name
	}

	matches := fuzzy.Find(query, names)
	filtered := make([]git.BranchInfo, len(matches))
	for i, match := range matches {
		filtered[i] = m.branches[match.Index]
	}
	m.filtered = filtered
	m.selectedIdx = 0
}

func (m *Model) moveSelection(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(m.filtered) {
		m.selectedIdx = len(m.filtered) - 1
	}
}

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	title := titleStyle.Render("git-flux branch")

	var list strings.Builder
	contentHeight := m.height - 3 // title + status + padding
	scrollOffset := 0
	if m.selectedIdx >= contentHeight {
		scrollOffset = m.selectedIdx - contentHeight + 1
	}
	for i := scrollOffset; i < len(m.filtered) && i-scrollOffset < contentHeight; i++ {
		b := m.filtered[i]
		name, info := formatBranchParts(b)

		if i == m.selectedIdx {
			list.WriteString(branchItemStyle.Render(selectedBranchStyle.Render(name) + "  " + info))
		} else if b.Current {
			list.WriteString(branchItemStyle.Render(currentBranchStyle.Render(name) + "  " + info))
		} else {
			list.WriteString(branchItemStyle.Render(name + "  " + info))
		}
		list.WriteString("\n")
	}

	var status string
	switch {
	case m.filtering:
		status = m.filter.View()
	case len(m.filtered) > 0:
		status = statusBarStyle.Render(fmt.Sprintf(
			"[%d/%d]  q:quit  /:filter  j/k:nav  enter:checkout",
			m.selectedIdx+1, len(m.filtered),
		))
	default:
		status = statusBarStyle.Render("No branches found")
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, list.String(), status)
}

func formatBranchParts(b git.BranchInfo) (name, info string) {
	prefix := "  "
	if b.Current {
		prefix = "* "
	}
	name = prefix + b.Name

	info = subtleStyle.Render(b.Date) + "  " + b.Message
	if b.Remote != "" {
		info = subtleStyle.Render("["+b.Remote+"]") + " " + info
	}

	return name, info
}
