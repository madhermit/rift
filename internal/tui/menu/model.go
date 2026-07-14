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
	vim       tui.VimNav // gg/G/ctrl+d/u jumps over the command list

	width  int
	height int
	ready  bool
}

var menuHints = [][2]string{
	{"↑↓", "nav"}, {"/", "filter"}, {"⏎", "select"}, {"?", "help"}, {"q", "quit"},
}

// menuNavKeys is the navigation reference for the menu's help overlay. The menu
// is a plain list (no preview pane), so it lists the list-nav keys it actually
// handles rather than the SplitList PreviewHelpKeys. Unlike a work screen, the
// menu's esc quits (there's no mode to leave and nowhere to go back to).
var menuNavKeys = [][2]string{
	{"j/k", "move"},
	{"gg/G", "top / bottom"},
	{"ctrl+d/u", "half-page"},
	{"esc", "quit"},
}

func (m Model) Selected() string {
	return m.selected
}

func New() Model {
	commands := []Command{
		{Name: "diff", Description: "Browse changes with syntax-aware diffs"},
		{Name: "log", Description: "Interactive commit log browser"},
		{Name: "stash", Description: "Stash manager with preview"},
		{Name: "stage", Description: "Interactive hunk staging"},
	}

	return Model{
		commands: commands,
		filtered: commands,
		filter:   tui.NewFilterInput(),
	}
}

func (m Model) Init() tea.Cmd {
	return tui.ThemeInit()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handleMouse(msg)
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

// handleMouse maps mouse input onto the menu list: the wheel moves the
// selection, a click selects the row under the pointer, and a click on the
// already-selected row activates it (the menu is a launcher, so a click means
// "choose this").
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.showHelp || !m.ready {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		m.moveSelection(tui.WheelDelta(msg)) // horizontal wheel maps to 0: a no-op
		return m, nil
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		// Rows render un-windowed from the panel's first inner line — and the
		// panel clips at its inner height, so a click must land on a row that
		// is both real and visible (a border/footer click on a short terminal
		// must not select, let alone launch, a clipped-away entry).
		idx := msg.Y - tui.HeaderRows - 1
		if idx < 0 || idx >= len(m.filtered) || idx >= m.contentHeight()-2 {
			return m, nil
		}
		if idx == m.selectedIdx {
			return m, m.activate(idx)
		}
		m.selectedIdx = idx
		return m, nil
	}
	return m, nil
}

// contentHeight is the vertical space between header and footer — the panel's
// outer height. The render and the click hit-test both derive from it, so the
// two can't desync.
func (m Model) contentHeight() int {
	return m.height - tui.HeaderRows - tui.FooterRows
}

// activate launches the command at idx — the single path shared by the enter
// key and a click on the selected row.
func (m Model) activate(idx int) tea.Cmd {
	cmd := m.filtered[idx]
	return func() tea.Msg { return SelectedMsg{Command: cmd.Name} }
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

	// gg/G and ctrl+d/u jump the selection (menu has no preview pane, so the vim
	// keys move the list directly).
	if next, ok := m.vim.HandleListKey(msg, m.selectedIdx, len(m.filtered), len(m.filtered)); ok {
		m.selectedIdx = next
		return m, nil
	}

	switch msg.String() {
	case "enter":
		if len(m.filtered) > 0 {
			return m, m.activate(m.selectedIdx)
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
	var prevName string
	if m.selectedIdx < len(m.filtered) {
		prevName = m.filtered[m.selectedIdx].Name
	}
	m.filtered = tui.FuzzyFilter(m.commands, m.filter.Value(), func(c Command) string { return c.Name })
	// Keep the selection on the same command when it survives the new filter.
	m.selectedIdx = 0
	if prevName != "" {
		for i, c := range m.filtered {
			if c.Name == prevName {
				m.selectedIdx = i
				break
			}
		}
	}
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
		return tui.ScreenView("Loading...")
	}

	if m.showHelp {
		contentH := m.contentHeight()
		return tui.ScreenView(tui.HelpView("", "a composable fuzzy git tool", menuHints, menuNavKeys, m.width, contentH))
	}

	var items strings.Builder
	for i, cmd := range m.filtered {
		selected := i == m.selectedIdx
		nameStyle := tui.NormalTextStyle
		if selected {
			nameStyle = tui.SelectedTextStyle
		}
		row := tui.Marker(selected) + nameStyle.Render(fmt.Sprintf("%-9s", cmd.Name)) +
			"  " + dimStyle.Render(cmd.Description)
		items.WriteString(row + "\n")
	}

	contentH := m.contentHeight()
	panel := tui.Panel("commands", "", items.String(), m.width, contentH, true, tui.Scrollbar{})

	header := tui.Header("", "a composable fuzzy git tool", m.width)

	var footer string
	switch {
	case m.filtering:
		footer = tui.FooterContent(m.width, m.filter.View())
	case len(m.filtered) == 0:
		footer = tui.Footer(m.width, "no matches", menuHints)
	default:
		footer = tui.Footer(m.width, fmt.Sprintf("%d/%d", m.selectedIdx+1, len(m.filtered)), menuHints)
	}

	return tui.ScreenView(lipgloss.JoinVertical(lipgloss.Left, header, panel, footer))
}
