package logui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
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
	vim         tui.VimNav

	width  int
	height int
	ready  bool
}

type diffLoadedMsg struct {
	content string
	err     error
}

func (m Model) layout() tui.SplitLayout {
	return tui.ComputeSplitLayout(m.width, m.height, m.activePane == diffPane, 30, 80)
}

func New(repo *git.Repo, engine diff.Engine, commits []git.CommitInfo) Model {
	return Model{
		repo:            repo,
		engine:          engine,
		commits:         commits,
		filteredCommits: commits,
		viewport:        viewport.New(),
		filter:          tui.NewFilterInput(),
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

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.activePane == diffPane && !m.filtering && m.vim.HandleKey(&m.viewport, msg) {
		return m, nil
	}

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
	case "tab":
		if m.activePane == commitPane {
			m.activePane = diffPane
		} else {
			m.activePane = commitPane
		}
		return m.applyLayout()
	case "enter":
		if m.activePane == commitPane {
			m.activePane = diffPane
			return m.applyLayout()
		}
	case "up", "k":
		return m.navigate(-1)
	case "down", "j":
		return m.navigate(1)
	case "q":
		return m, tea.Quit
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, nil
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
	m.viewport.SetWidth(l.DiffWidth)
	m.viewport.SetHeight(l.ContentHeight - 2)
	m.setDiffContent()
	if len(m.filteredCommits) > 0 {
		return m, m.loadCommitDiff(m.filteredCommits[m.selectedIdx])
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

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "enter" {
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
	m.filteredCommits = tui.FuzzyFilter(m.commits, m.filter.Value(), func(c git.CommitInfo) string { return c.Hash + " " + c.Message })
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

func commitHeader(commit git.CommitInfo, files []git.ChangedFile, color bool, width int) string {
	hash := commit.Hash
	authorLabel := "Author:"
	dateLabel := "Date:"

	const indent = "    "
	wrapWidth := width - len(indent)

	subject := commit.Message
	if wrapWidth > 0 {
		subject = ansi.Wordwrap(subject, wrapWidth, "")
	}
	subject = indent + strings.ReplaceAll(subject, "\n", "\n"+indent)

	body := ""
	if commit.Body != "" {
		b := commit.Body
		if wrapWidth > 0 && longestLine(b) > wrapWidth {
			b = reflowParagraphs(b)
			b = ansi.Wordwrap(b, wrapWidth, "")
		}
		body = "\n\n" + indent + strings.ReplaceAll(b, "\n", "\n"+indent)
	}
	sep := "─────────────────────"

	if color {
		hash = hashStyle.Render(hash)
		authorLabel = headerLabelStyle.Render(authorLabel)
		dateLabel = headerLabelStyle.Render(dateLabel)
		subject = "\x1b[1m" + subject + "\x1b[22m"
		sep = headerLabelStyle.Render(sep)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "commit %s\n%s %s\n%s   %s\n\n%s%s\n", hash, authorLabel, commit.Author, dateLabel, commit.Date, subject, body)

	if len(files) > 0 {
		b.WriteString("\n")
		for _, f := range files {
			icon := git.StatusChar(f.Status)
			if color {
				icon = tui.StatusStyle(f.Status).Render(icon)
			}
			fmt.Fprintf(&b, "  %s %s %s\n", icon, tui.FileIcon(f.Path), f.Path)
		}
	}

	fmt.Fprintf(&b, "\n%s\n\n", sep)
	return b.String()
}

func longestLine(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if len(line) > max {
			max = len(line)
		}
	}
	return max
}

// reflowParagraphs removes git's hard line wraps (single \n) while
// preserving intentional paragraph breaks (double \n\n).
func reflowParagraphs(s string) string {
	paragraphs := strings.Split(s, "\n\n")
	for i, p := range paragraphs {
		paragraphs[i] = strings.ReplaceAll(strings.TrimSpace(p), "\n", " ")
	}
	return strings.Join(paragraphs, "\n\n")
}

func (m Model) loadCommitDiff(commit git.CommitInfo) tea.Cmd {
	width := m.viewport.Width()
	return func() tea.Msg {
		// Fetch changed file list for the header
		base := commit.Hash + "~1"
		files, err := m.repo.DiffBetweenCommits(base, commit.Hash)
		if err != nil {
			// First commit — diff against empty tree
			files, _ = m.repo.DiffBetweenCommits("4b825dc642cb6eb9a060e54bf899d69f82cf7207", commit.Hash)
		}
		color := os.Getenv("NO_COLOR") == ""
		header := commitHeader(commit, files, color, width)
		content, err := m.engine.DiffCommit(
			context.Background(), m.repo.Root(),
			base, commit.Hash, color, width,
		)
		if err != nil {
			// First commit has no parent — diff against empty tree
			content, err = m.engine.DiffCommit(
				context.Background(), m.repo.Root(),
				"4b825dc642cb6eb9a060e54bf899d69f82cf7207", commit.Hash, color, width,
			)
		}
		if err != nil {
			return diffLoadedMsg{content: content, err: err}
		}
		return diffLoadedMsg{content: header + content}
	}
}

func (m *Model) setDiffContent() {
	content := m.diffContent
	if w := m.viewport.Width(); w > 0 && content != "" {
		content = ansi.Hardwrap(content, w, true)
	}
	m.vim.SetContent(&m.viewport, content)
}

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("Loading...")
	}

	l := m.layout()
	collapsed := m.activePane == diffPane

	// Commit list body.
	var list strings.Builder
	listInnerH := l.ContentHeight - 2
	scrollOffset := 0
	if m.selectedIdx >= listInnerH {
		scrollOffset = m.selectedIdx - listInnerH + 1
	}
	for i := scrollOffset; i < len(m.filteredCommits) && i-scrollOffset < listInnerH; i++ {
		c := m.filteredCommits[i]
		selected := i == m.selectedIdx
		text := c.Hash
		if !collapsed {
			text = tui.Truncate(c.Hash+"  "+c.Message, l.ListWidth-4)
		}
		style := tui.NormalTextStyle
		if selected {
			style = tui.SelectedTextStyle
		}
		list.WriteString(tui.Marker(selected) + style.Render(text) + "\n")
	}

	listTitle, diffTitle := "commits", ""
	if collapsed {
		listTitle = ""
	}
	if len(m.filteredCommits) > 0 {
		diffTitle = m.filteredCommits[m.selectedIdx].Hash
	}
	listPanel := tui.Panel(listTitle, list.String(), l.ListWidth, l.ContentHeight, m.activePane == commitPane)
	diffPanel := tui.Panel(diffTitle, m.viewport.View(), l.DiffWidth+2, l.ContentHeight, m.activePane == diffPane)
	content := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, diffPanel)

	header := tui.Header("log", m.engine.Name(), m.width)

	var footer string
	switch {
	case m.filtering:
		footer = tui.FooterContent(m.width, m.filter.View())
	case m.diffErr != nil:
		footer = tui.Footer(m.width, fmt.Sprintf("Error: %v", m.diffErr), nil)
	default:
		status := "No commits found"
		if len(m.filteredCommits) > 0 {
			status = fmt.Sprintf("%d/%d", m.selectedIdx+1, len(m.filteredCommits))
			if collapsed {
				status += fmt.Sprintf(" · %.0f%%", m.viewport.ScrollPercent()*100)
			}
		}
		footer = tui.Footer(m.width, status, [][2]string{
			{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"},
			{"gg/G", "top/bot"}, {"{/}", "section"}, {"q", "quit"},
		})
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, content, footer))
	v.AltScreen = true
	return v
}
