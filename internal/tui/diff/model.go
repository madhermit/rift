package diffui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

type Model struct {
	repo    *git.Repo
	engines tui.EngineToggle
	list    tui.SplitList[git.ChangedFile]

	staged     bool
	base       string
	target     string
	commitDiff bool
	display    diff.Display
	stream     *tui.PreviewStream // active "all changes" stream; nil otherwise
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

// pathRow renders prefix + a path (dim dir, emphasized basename) truncated to
// fit, right-aligning stat within total width w (reserving a one-column gap).
func pathRow(prefix string, selected bool, path, stat string, w int) string {
	avail := w - lipgloss.Width(prefix)
	if stat != "" {
		avail -= lipgloss.Width(stat) + 1
	}
	if avail < 1 {
		avail = 1
	}
	return rowWithStat(prefix+tui.RenderPath(path, avail, selected), stat, w)
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, staged bool, base, target string, lensToggle bool) Model {
	commitDiff := target != ""
	engines := tui.NewEngineToggle(engine)
	branch := repo.CurrentBranch()

	listTitle := "changes"
	if !commitDiff {
		listTitle = tui.ToggleTitle("unstaged", "staged", staged)
	}

	hints := [][2]string{
		{"/", "filter"}, {"⇥", "read"},
	}
	if lensToggle {
		hints = append(hints, [2]string{"t", "tests"})
	}
	if !commitDiff {
		hints = append(hints, [2]string{"s", "staged"})
	}
	hints = append(hints, [2]string{"\\", "layout"})
	if engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	if !commitDiff {
		// In a commit diff the files are historical; editing the working tree copy
		// would be misleading, so don't offer `o`.
		hints = append(hints, [2]string{"o", "open"})
	}
	hints = append(hints, [2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.ChangedFile]{
		Screen:      "diff",
		ListTitle:   listTitle,
		Branch:      branch,
		Context:     tui.ContextLabel(target, engine.Name()),
		NavFraction: 30,
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
		Row: func(f git.ChangedFile, w int, selected bool) string {
			if f.Path == "" {
				return rowWithStat(tui.TextStyle(selected).Render("∗ All changes"), tui.DiffStat(f.Added, f.Deleted), w)
			}
			prefix := tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status)) + " " + tui.FileIcon(f.Path) + " "
			return pathRow(prefix, selected, f.Path, tui.DiffStat(f.Added, f.Deleted), w)
		},
	}

	return Model{
		repo:       repo,
		engines:    engines,
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
		m.stream.Cancel()
		m.stream = nil
		return m, m.previewCmd(msg.ReqID)
	case tui.StreamReadyMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.ApplyStream(m.list, m.stream, msg)
		return m, cmd
	case tui.ChunkMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.AdvanceStream(m.list, m.stream, msg)
		return m, cmd
	case tui.EditorClosedMsg:
		return m.reloadFresh() // the file may have changed
	case tea.KeyPressMsg:
		if m.list.Filtering() || m.list.ShowingHelp() {
			break
		}
		switch msg.String() {
		case "\\":
			m.display = m.display.Next()
			return m.reloadFresh()
		case "e":
			if !m.engines.CanToggle() {
				break // only one engine available; nothing to toggle
			}
			m.engines = m.engines.Toggle()
			m.list = m.list.SetContext(tui.ContextLabel(m.target, m.engines.Name()))
			return m.reloadFresh()
		case "o":
			// Disabled for a commit diff: the files are historical, so editing the
			// working tree copy would be misleading.
			if sel, ok := m.list.Selected(); ok && !m.commitDiff && sel.Path != "" {
				line := diff.FirstHunkLine(m.repo.Root(), m.staged, sel.Path)
				return m, tui.OpenInEditor(m.repo.Root(), sel.Path, line)
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

// Filtering and ShowingHelp expose the embedded list's modal state, so a parent
// embedding this model (e.g. the log drilldown) knows when not to intercept esc.
func (m Model) Filtering() bool   { return m.list.Filtering() }
func (m Model) ShowingHelp() bool { return m.list.ShowingHelp() }

// CancelPreview stops this model's in-flight diff stream — used by the lens
// wrapper before hiding it, so a backgrounded difftastic run doesn't linger.
func (m Model) CancelPreview() tea.Model {
	m.stream.Cancel()
	m.stream = nil
	return m
}

// previewCmd streams the selected files' diffs: the selected file, or every file
// for the synthetic "All changes" entry, diffed in parallel and emitted in order
// so the first paints as soon as it's ready. An empty changeset streams nothing
// (ApplyStream resolves the loading state and caches the empty result).
func (m Model) previewCmd(reqID int) tea.Cmd {
	sel, ok := m.list.Selected()
	if !ok {
		return nil
	}
	files := previewFiles(sel, m.list.VisibleItems())
	opts := m.previewOpts()
	root, engine := m.repo.Root(), m.engines.Engine()
	return func() tea.Msg {
		if len(files) == 0 {
			return tui.StreamReadyMsg{ReqID: reqID}
		}
		ch, cancel := tui.StreamFiles(engine, root, files, opts)
		return tui.StreamReadyMsg{ReqID: reqID, Ch: ch, Cancel: cancel}
	}
}

func (m Model) previewOpts() diff.DiffOpts {
	o := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.display)
	o.Staged, o.Base, o.Target = m.staged, m.base, m.target
	return o
}

// previewFiles is the list of files a selection previews: just the selected file,
// or every changed file for the synthetic "All changes" entry (empty path).
func previewFiles(sel git.ChangedFile, visible []git.ChangedFile) []string {
	if sel.Path != "" {
		return []string{sel.Path}
	}
	var files []string
	for _, f := range visible {
		if f.Path != "" {
			files = append(files, f.Path)
		}
	}
	return files
}
