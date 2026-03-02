package menu

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/tui"
	"github.com/sahilm/fuzzy"
)

type Command struct {
	Name        string
	Description string
	Available   bool
}

type SelectedMsg struct {
	Command string
}

type Model struct {
	commands    []Command
	filtered    []Command
	selectedIdx int
	selected    string

	filter    textinput.Model
	filtering bool

	width  int
	height int
	ready  bool
}

func (m Model) Selected() string {
	return m.selected
}

func New() Model {
	commands := []Command{
		{Name: "diff", Description: "Browse changes with syntax-aware diffs", Available: true},
		{Name: "log", Description: "Interactive commit log browser", Available: true},
		{Name: "branch", Description: "Fuzzy branch switcher", Available: true},
		{Name: "stash", Description: "Stash manager with preview", Available: true},
		{Name: "stage", Description: "Interactive hunk staging", Available: true},
		{Name: "worktree", Description: "Worktree manager", Available: false},
	}

	return Model{
		commands: commands,
		filtered: commands,
		filter:   tui.NewFilterInput(),
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
		return m, nil
	case SelectedMsg:
		m.selected = msg.Command
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	case "enter":
		if len(m.filtered) > 0 {
			cmd := m.filtered[m.selectedIdx]
			if !cmd.Available {
				return m, nil
			}
			return m, func() tea.Msg {
				return SelectedMsg{Command: cmd.Name}
			}
		}
	case "up", "k":
		m.moveSelection(-1)
		return m, nil
	case "down", "j":
		m.moveSelection(1)
		return m, nil
	case "q":
		return m, tea.Quit
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, nil
	}

	return m, nil
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
	return m, cmd
}

func (m *Model) applyFilter() {
	query := m.filter.Value()
	if query == "" {
		m.filtered = m.commands
		m.selectedIdx = 0
		return
	}

	names := make([]string, len(m.commands))
	for i, c := range m.commands {
		names[i] = c.Name
	}

	matches := fuzzy.Find(query, names)
	filtered := make([]Command, len(matches))
	for i, match := range matches {
		filtered[i] = m.commands[match.Index]
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

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("Loading...")
	}

	title := titleStyle.Render("rift")

	var items strings.Builder
	for i, cmd := range m.filtered {
		name := cmd.Name
		if !cmd.Available {
			name += " (coming soon)"
		}

		if i == m.selectedIdx {
			items.WriteString(selectedItemStyle.Render(name))
		} else {
			items.WriteString(itemStyle.Render(name))
		}
		items.WriteString("\n")
		items.WriteString(descriptionStyle.Render(cmd.Description))
		items.WriteString("\n\n")
	}

	var status string
	if m.filtering {
		status = m.filter.View()
	} else {
		status = statusBarStyle.Render(fmt.Sprintf("[%d/%d]  q:quit  /:filter  j/k:nav  enter:select", m.selectedIdx+1, len(m.filtered)))
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, title, items.String(), status))
	v.AltScreen = true
	return v
}
