package branchui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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

	width  int
	height int
	ready  bool
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
	h := m.height - 4 // title (with padding) + status + blank line
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("Loading...")
	}

	title := titleStyle.Render("rift branch")
	visible := m.listHeight()

	var list strings.Builder
	for i := m.scrollOff; i < len(m.filtered) && i-m.scrollOff < visible; i++ {
		b := m.filtered[i]
		selected := i == m.selectedIdx

		cursor := "  "
		if selected {
			cursor = "> "
		}

		prefix := "  "
		if b.Current {
			prefix = "* "
		}

		line := cursor + prefix + b.Name
		if b.Remote != "" {
			line += "  [" + b.Remote + "]"
		}
		line += "  " + b.Date + "  " + b.Message

		style := normalLineStyle
		switch {
		case selected:
			style = selectedLineStyle
		case b.Current:
			style = currentLineStyle
		}
		list.WriteString(style.Width(m.width).Render(line) + "\n")
	}

	// Scroll indicator
	var scrollHint string
	if len(m.filtered) > visible {
		scrollHint = subtleStyle.Render(fmt.Sprintf(" (%d more)", len(m.filtered)-visible))
	}

	var status string
	switch {
	case m.filtering:
		status = m.filter.View()
	case len(m.filtered) > 0:
		status = statusBarStyle.Render(fmt.Sprintf(
			"[%d/%d]%s  q:quit  /:filter  j/k:nav  enter:checkout",
			m.selectedIdx+1, len(m.filtered), scrollHint,
		))
	default:
		status = statusBarStyle.Render("No branches found")
	}

	v := tea.NewView(title + "\n" + list.String() + status)
	v.AltScreen = true
	return v
}
