package diffui

import (
	"context"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

type Model struct {
	repo   *git.Repo
	engine diff.Engine
	list   tui.SplitList[git.ChangedFile]

	staged     bool
	base       string
	target     string
	commitDiff bool
}

// filesLoadedMsg carries a refreshed file list after toggling staged/unstaged.
type filesLoadedMsg struct {
	files []git.ChangedFile
	err   error
}

// prependAllEntry adds a synthetic "All" row that previews every file at once.
func prependAllEntry(files []git.ChangedFile) []git.ChangedFile {
	all := make([]git.ChangedFile, 0, len(files)+1)
	all = append(all, git.ChangedFile{Path: "", Status: "All"})
	return append(all, files...)
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, staged bool, base, target string) Model {
	commitDiff := target != ""

	listTitle := "changes"
	if !commitDiff {
		listTitle = "unstaged"
		if staged {
			listTitle = "staged"
		}
	}

	hints := [][2]string{
		{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"}, {"gg/G", "top/bot"}, {"{/}", "section"},
	}
	if !commitDiff {
		hints = append(hints, [2]string{"s", "staged"})
	}
	hints = append(hints, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.ChangedFile]{
		Screen:      "diff",
		ListTitle:   listTitle,
		Context:     engine.Name(),
		MinList:     20,
		MaxList:     60,
		EmptyStatus: "No changes found",
		Hints:       hints,
		Match:       func(f git.ChangedFile) string { return f.Path },
		PreviewTitle: func(f git.ChangedFile) string {
			if f.Path == "" {
				return "all changes"
			}
			return f.Path
		},
		Row: func(f git.ChangedFile, w int, selected, collapsed bool) string {
			style := tui.TextStyle(selected)
			switch {
			case f.Path == "" && collapsed:
				return style.Render("∗")
			case f.Path == "":
				return style.Render("∗ All changes")
			case collapsed:
				return tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " + tui.FileIcon(f.Path)
			default:
				return tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " +
					tui.FileIcon(f.Path) + " " + style.Render(tui.TruncatePath(f.Path, w-4))
			}
		},
	}

	return Model{
		repo:       repo,
		engine:     engine,
		staged:     staged,
		base:       base,
		target:     target,
		commitDiff: commitDiff,
		list:       tui.NewSplitList(cfg, prependAllEntry(files)),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		return m, m.previewCmd()
	case filesLoadedMsg:
		if msg.err != nil {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(tui.PreviewMsg{Err: msg.err})
			return m, cmd
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.SetItems(prependAllEntry(msg.files))
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "s" && !m.commitDiff && !m.list.Filtering() {
			return m.toggleStaged()
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View { return m.list.TeaView() }

func (m Model) toggleStaged() (tea.Model, tea.Cmd) {
	m.staged = !m.staged
	title := "unstaged"
	if m.staged {
		title = "staged"
	}
	m.list = m.list.SetListTitle(title)

	repo, staged := m.repo, m.staged
	return m, func() tea.Msg {
		files, err := repo.ChangedFiles(staged)
		if err != nil {
			return filesLoadedMsg{err: err}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		return filesLoadedMsg{files: files}
	}
}

func (m Model) previewCmd() tea.Cmd {
	sel, ok := m.list.Selected()
	if !ok {
		return nil
	}
	var files []string
	if sel.Path == "" {
		for _, f := range m.list.VisibleItems() {
			if f.Path != "" {
				files = append(files, f.Path)
			}
		}
	} else {
		files = []string{sel.Path}
	}

	repo, engine := m.repo, m.engine
	opts := diff.DiffOpts{
		Staged: m.staged,
		Base:   m.base,
		Target: m.target,
		Color:  tui.ColorEnabled(),
		Width:  m.list.PreviewWidth(),
	}
	return func() tea.Msg {
		var result strings.Builder
		for _, file := range files {
			content, err := engine.Diff(context.Background(), repo.Root(), file, opts)
			if err != nil {
				continue
			}
			if content != "" {
				result.WriteString(content)
				result.WriteString("\n")
			}
		}
		return tui.PreviewMsg{Content: result.String()}
	}
}
