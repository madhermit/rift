package diffui

import (
	"context"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

type Model struct {
	repo            *git.Repo
	engine          diff.Engine
	altEngine       diff.Engine // the other engine, swapped in by the `e` toggle
	canToggleEngine bool        // true when the two engines actually differ
	list            tui.SplitList[git.ChangedFile]

	staged     bool
	base       string
	target     string
	commitDiff bool
	display    diff.Display
}

// filesLoadedMsg carries a refreshed file list after toggling staged/unstaged.
type filesLoadedMsg struct {
	files []git.ChangedFile
	err   error
}

// prependAllEntry adds a synthetic "All" row (carrying the totals) that previews
// every file at once.
func prependAllEntry(files []git.ChangedFile) []git.ChangedFile {
	all := git.ChangedFile{Path: "", Status: "All"}
	for _, f := range files {
		all.Added += f.Added
		all.Deleted += f.Deleted
	}
	out := make([]git.ChangedFile, 0, len(files)+1)
	return append(append(out, all), files...)
}

// rowWithStat right-aligns stat (e.g. "+3 -1") at the end of a row of width w,
// dropping it if there's no room.
func rowWithStat(left, stat string, w int) string {
	if stat == "" {
		return left
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(stat)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + stat
}

// pathRow renders prefix + a styled path truncated to fit, right-aligning stat
// within total width w (reserving a one-column gap before it).
func pathRow(prefix string, style lipgloss.Style, path, stat string, w int) string {
	avail := w - lipgloss.Width(prefix)
	if stat != "" {
		avail -= lipgloss.Width(stat) + 1
	}
	if avail < 1 {
		avail = 1
	}
	return rowWithStat(prefix+style.Render(tui.TruncatePath(path, avail)), stat, w)
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, staged bool, base, target string) Model {
	commitDiff := target != ""
	altEngine := diff.NewPlainEngine()
	canToggleEngine := engine.Name() != altEngine.Name()

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
	hints = append(hints, [2]string{"\\", "layout"})
	if canToggleEngine {
		hints = append(hints, [2]string{"e", "engine"})
	}
	hints = append(hints, [2]string{"o", "open"}, [2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

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
		// Cache per file; the All entry depends on the filtered set, so skip it.
		// Staged/display changes clear the cache (SetItems / ClearCacheAndReload).
		CacheKey: func(f git.ChangedFile) string { return f.Path },
		Yank:     func(f git.ChangedFile) string { return f.Path },
		Row: func(f git.ChangedFile, w int, selected, collapsed bool) string {
			style := tui.TextStyle(selected)
			switch {
			case f.Path == "" && collapsed:
				return style.Render("∗")
			case f.Path == "":
				return rowWithStat(style.Render("∗ All changes"), tui.DiffStat(f.Added, f.Deleted), w)
			case collapsed:
				return tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " + tui.FileIcon(f.Path)
			default:
				prefix := tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " + tui.FileIcon(f.Path) + " "
				return pathRow(prefix, style, f.Path, tui.DiffStat(f.Added, f.Deleted), w)
			}
		},
	}

	return Model{
		repo:            repo,
		engine:          engine,
		staged:          staged,
		base:            base,
		target:          target,
		commitDiff:      commitDiff,
		list:            tui.NewSplitList(cfg, prependAllEntry(files)),
		altEngine:       altEngine,
		canToggleEngine: canToggleEngine,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		return m, m.previewCmd(msg.ReqID)
	case tui.EditorClosedMsg:
		return m.reloadFresh() // the file may have changed
	case filesLoadedMsg:
		if msg.err != nil {
			m.list = m.list.SetError(msg.err)
			return m, nil
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.SetItems(prependAllEntry(msg.files))
		return m, cmd
	case tea.KeyPressMsg:
		if m.list.Filtering() || m.list.ShowingHelp() {
			break
		}
		switch msg.String() {
		case "\\":
			m.display = m.display.Next()
			return m.reloadFresh()
		case "e":
			if !m.canToggleEngine {
				break // only one engine available; nothing to toggle
			}
			m.engine, m.altEngine = m.altEngine, m.engine
			m.list = m.list.SetContext(m.engine.Name())
			return m.reloadFresh()
		case "s":
			if !m.commitDiff {
				return m.toggleStaged()
			}
		case "o":
			if sel, ok := m.list.Selected(); ok && sel.Path != "" {
				return m, tui.OpenInEditor(m.repo.Root(), sel.Path)
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// reloadFresh drops the preview cache and reloads the current selection, after a
// setting (layout/engine) that affects rendering has changed.
func (m Model) reloadFresh() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.ClearCacheAndReload()
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

func (m Model) previewCmd(reqID int) tea.Cmd {
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
		Staged:  m.staged,
		Base:    m.base,
		Target:  m.target,
		Color:   tui.ColorEnabled(),
		Width:   m.list.PreviewWidth(),
		Display: m.display,
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
		return tui.PreviewMsg{Content: result.String(), ReqID: reqID}
	}
}
