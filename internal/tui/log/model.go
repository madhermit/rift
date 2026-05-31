package logui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

// emptyTree is git's well-known empty-tree object, used to diff a root commit
// that has no parent.
const emptyTree = "4b825dc642cb6eb9a060e54bf899d69f82cf7207"

type Model struct {
	repo    *git.Repo
	engine  diff.Engine
	list    tui.SplitList[git.CommitInfo]
	display diff.Display
}

func New(repo *git.Repo, engine diff.Engine, commits []git.CommitInfo) Model {
	cfg := tui.SplitConfig[git.CommitInfo]{
		Screen:      "log",
		ListTitle:   "commits",
		Context:     engine.Name(),
		MinList:     30,
		MaxList:     80,
		EmptyStatus: "No commits found",
		Hints: [][2]string{
			{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"},
			{"gg/G", "top/bot"}, {"\\", "layout"}, {"q", "quit"},
		},
		Match:        func(c git.CommitInfo) string { return c.Hash + " " + c.Message },
		PreviewTitle: func(c git.CommitInfo) string { return c.Hash },
		Row: func(c git.CommitInfo, w int, selected, collapsed bool) string {
			style := tui.TextStyle(selected)
			if collapsed {
				return style.Render(c.Hash)
			}
			return style.Render(tui.Truncate(c.Hash+"  "+c.Message, w))
		},
	}
	return Model{repo: repo, engine: engine, list: tui.NewSplitList(cfg, commits)}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		return m, m.previewCmd()
	case tea.KeyPressMsg:
		if msg.String() == "\\" && !m.list.Filtering() {
			m.display = m.display.Next()
			return m, m.previewCmd()
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View { return m.list.TeaView() }

func (m Model) previewCmd() tea.Cmd {
	commit, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine, width, display := m.repo, m.engine, m.list.PreviewWidth(), m.display
	return func() tea.Msg {
		base := commit.Hash + "~1"
		files, err := repo.DiffBetweenCommits(base, commit.Hash)
		if err != nil {
			base = emptyTree // root commit: diff against the empty tree
			files, _ = repo.DiffBetweenCommits(base, commit.Hash)
		}
		color := tui.ColorEnabled()
		header := commitHeader(commit, files, color, width)
		content, err := engine.DiffCommit(context.Background(), repo.Root(), base, commit.Hash, color, width, display)
		if err != nil {
			return tui.PreviewMsg{Content: content, Err: err}
		}
		return tui.PreviewMsg{Content: header + content}
	}
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
		body = "\n" + tui.Markdown(commit.Body, width, color)
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
