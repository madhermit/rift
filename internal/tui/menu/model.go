package menu

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/tui"
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
	showHelp  bool

	width  int
	height int
	ready  bool
}

var menuHints = [][2]string{
	{"↑↓", "nav"}, {"/", "filter"}, {"⏎", "select"}, {"?", "help"}, {"q", "quit"},
}

func (m Model) Selected() string {
	return m.selected
}

func New() Model {
	commands := []Command{
		{Name: "diff", Description: "Browse changes with syntax-aware diffs", Available: true},
		{Name: "log", Description: "Interactive commit log browser", Available: true},
		{Name: "stash", Description: "Stash manager with preview", Available: true},
		{Name: "stage", Description: "Interactive hunk staging", Available: true},
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
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.showHelp {
		m.showHelp = false // any key closes the help overlay
		return m, nil
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
	case "?":
		m.showHelp = true
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
	m.filtered = tui.FuzzyFilter(m.commands, m.filter.Value(), func(c Command) string { return c.Name })
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

	if m.showHelp {
		contentH := m.height - tui.HeaderRows - tui.FooterRows
		v := tea.NewView(tui.HelpView("", "a composable fuzzy git tool", menuHints, tui.BasicHelpKeys, m.width, contentH))
		v.AltScreen = true
		return v
	}

	var items strings.Builder
	for i, cmd := range m.filtered {
		selected := i == m.selectedIdx
		nameStyle := tui.NormalTextStyle
		switch {
		case !cmd.Available:
			nameStyle = dimStyle
		case selected:
			nameStyle = tui.SelectedTextStyle
		}
		desc := cmd.Description
		if !cmd.Available {
			desc += " (soon)"
		}
		row := tui.Marker(selected) + nameStyle.Render(fmt.Sprintf("%-9s", cmd.Name)) +
			"  " + dimStyle.Render(desc)
		items.WriteString(row + "\n")
	}

	contentH := m.height - tui.HeaderRows - tui.FooterRows
	panel := tui.Panel("commands", items.String(), m.width, contentH, true, tui.Scrollbar{})

	header := tui.Header("", "a composable fuzzy git tool", m.width)

	var footer string
	if m.filtering {
		footer = tui.FooterContent(m.width, m.filter.View())
	} else {
		footer = tui.Footer(m.width, fmt.Sprintf("%d/%d", m.selectedIdx+1, len(m.filtered)), menuHints)
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, panel, footer))
	v.AltScreen = true
	return v
}
