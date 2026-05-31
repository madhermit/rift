package branchui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

const scrollMargin = 3

type Model struct {
	branches    []git.BranchInfo
	filtered    []git.BranchInfo
	selectedIdx int
	scrollOff   int
	checkout    string

	filter    textinput.Model
	filtering bool
	showHelp  bool

	width  int
	height int
	ready  bool
}

var branchHints = [][2]string{
	{"↑↓", "nav"}, {"/", "filter"}, {"⏎", "checkout"}, {"?", "help"}, {"q", "quit"},
}

func (m Model) Checkout() string {
	return m.checkout
}

func New(branches []git.BranchInfo) Model {
	return Model{
		branches: branches,
		filtered: branches,
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
			b := m.filtered[m.selectedIdx]
			if !b.Current {
				m.checkout = b.Name
				return m, tea.Quit
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
	m.filtered = tui.FuzzyFilter(m.branches, m.filter.Value(), func(b git.BranchInfo) string { return b.Name })
	m.selectedIdx = 0
	m.scrollOff = 0
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
	m.clampScroll()
}

func (m *Model) clampScroll() {
	visible := m.listHeight()

	// Keep selection within scroll margin of the viewport edges
	if m.selectedIdx < m.scrollOff+scrollMargin {
		m.scrollOff = m.selectedIdx - scrollMargin
	}
	if m.selectedIdx >= m.scrollOff+visible-scrollMargin {
		m.scrollOff = m.selectedIdx - visible + scrollMargin + 1
	}

	// Hard clamps
	if m.scrollOff < 0 {
		m.scrollOff = 0
	}
	if max := len(m.filtered) - visible; max > 0 && m.scrollOff > max {
		m.scrollOff = max
	}
}

func (m Model) listHeight() int {
	// header + footer chrome, plus the panel's top/bottom border.
	h := m.height - tui.HeaderRows - tui.FooterRows - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("Loading...")
	}

	if m.showHelp {
		contentH := m.height - tui.HeaderRows - tui.FooterRows
		v := tea.NewView(tui.HelpView("branch", "", branchHints, tui.BasicHelpKeys, m.width, contentH))
		v.AltScreen = true
		return v
	}

	visible := m.listHeight()

	var list strings.Builder
	for i := m.scrollOff; i < len(m.filtered) && i-m.scrollOff < visible; i++ {
		b := m.filtered[i]
		selected := i == m.selectedIdx

		flag := " "
		if b.Current {
			flag = currentFlagStyle.Render("●")
		}

		nameStyle := tui.NormalTextStyle
		switch {
		case selected:
			nameStyle = tui.SelectedTextStyle
		case b.Current:
			nameStyle = currentLineStyle
		}

		line := tui.Marker(selected) + flag + " " + nameStyle.Render(b.Name)
		if b.Remote != "" {
			line += "  " + dimStyle.Render("["+b.Remote+"]")
		}
		line += "  " + dimStyle.Render(b.Date+"  "+b.Message)
		list.WriteString(line + "\n")
	}

	contentH := m.height - tui.HeaderRows - tui.FooterRows
	panel := tui.Panel("branches", list.String(), m.width, contentH, true)

	header := tui.Header("branch", "", m.width)

	var footer string
	switch {
	case m.filtering:
		footer = tui.FooterContent(m.width, m.filter.View())
	case len(m.filtered) > 0:
		status := fmt.Sprintf("%d/%d", m.selectedIdx+1, len(m.filtered))
		footer = tui.Footer(m.width, status, branchHints)
	default:
		footer = tui.Footer(m.width, "No branches found", nil)
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, panel, footer))
	v.AltScreen = true
	return v
}
