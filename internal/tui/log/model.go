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
	diffui "github.com/madhermit/rift/internal/tui/diff"
)

// Action is a git operation the user chose to run on the selected commit; the
// program quits with it and the command layer performs it.
type Action int

const (
	NoAction Action = iota
	CherryPick
	Revert
)

type Model struct {
	repo            *git.Repo
	engine          diff.Engine
	altEngine       diff.Engine // the other engine, swapped in by the `e` toggle
	canToggleEngine bool        // true when the two engines actually differ
	list            tui.SplitList[git.CommitInfo]
	display         diff.Display
	action          Action

	drilled  diffui.Model // file-level view of the selected commit (when drilling)
	drilling bool
	width    int
	height   int
}

// Action reports the git operation chosen for the selected commit (NoAction if
// the user just quit).
func (m Model) Action() Action { return m.action }

// SelectedHash is the hash of the commit the user acted on, or "" if none.
func (m Model) SelectedHash() string {
	if c, ok := m.list.Selected(); ok {
		return c.Hash
	}
	return ""
}

func New(repo *git.Repo, engine diff.Engine, commits []git.CommitInfo) Model {
	altEngine := diff.NewPlainEngine()
	canToggleEngine := engine.Name() != altEngine.Name()

	hints := [][2]string{
		{"↑↓", "nav"}, {"⏎", "files"}, {"/", "filter"}, {"⇥", "switch"}, {"gg/G", "top/bot"}, {"\\", "layout"},
	}
	if canToggleEngine {
		hints = append(hints, [2]string{"e", "engine"})
	}
	hints = append(hints,
		[2]string{"c", "cherry-pick"}, [2]string{"r", "revert"},
		[2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.CommitInfo]{
		Screen:       "log",
		ListTitle:    "commits",
		Context:      engine.Name(),
		MinList:      30,
		MaxList:      80,
		EmptyStatus:  "No commits found",
		Hints:        hints,
		Match:        func(c git.CommitInfo) string { return c.Hash + " " + c.Message },
		PreviewTitle: func(c git.CommitInfo) string { return c.Hash },
		CacheKey:     func(c git.CommitInfo) string { return c.Hash },
		Yank:         func(c git.CommitInfo) string { return c.Hash },
		Row: func(c git.CommitInfo, w int, selected, collapsed bool) string {
			style := tui.TextStyle(selected)
			if collapsed {
				return style.Render(c.Hash)
			}
			return style.Render(tui.Truncate(c.Hash+"  "+c.Message, w))
		},
	}
	return Model{repo: repo, engine: engine, altEngine: altEngine, canToggleEngine: canToggleEngine, list: tui.NewSplitList(cfg, commits)}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The drilled diff view runs its own SplitList with an independent preview
	// request counter; its async traffic is tagged (drilledMsg) so it routes back
	// to the drilled view and is never mistaken for the commit list's own
	// SelectionChangedMsg/PreviewMsg (whose ReqIDs would otherwise collide).
	if dm, ok := msg.(drilledMsg); ok {
		if !m.drilling {
			return m, nil // a stale message from a closed drilldown
		}
		return m.updateDrilled(dm.msg)
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && m.drilling {
		return m.updateDrilled(key)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Resizes go to both the commit list (so it's current when we return) and
		// the drilled view.
		m.width, m.height = msg.Width, msg.Height
		var lc tea.Cmd
		m.list, lc = m.list.Update(msg)
		if !m.drilling {
			return m, lc
		}
		var dc tea.Cmd
		m, dc = m.sendDrilled(msg)
		return m, tea.Batch(lc, dc)
	case tui.SelectionChangedMsg:
		return m, m.previewCmd(msg.ReqID)
	case tea.KeyPressMsg:
		if m.list.Filtering() || m.list.ShowingHelp() {
			break
		}
		switch msg.String() {
		case "enter":
			if c, ok := m.list.Selected(); ok {
				return m.drillInto(c)
			}
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
		case "c":
			if _, ok := m.list.Selected(); ok {
				m.action = CherryPick
				return m, tea.Quit
			}
		case "r":
			if _, ok := m.list.Selected(); ok {
				m.action = Revert
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// drilledMsg wraps a message produced by the drilled diff view so the parent can
// route it back without confusing it with the commit list's own preview traffic.
type drilledMsg struct{ msg tea.Msg }

// tagDrilled wraps the preview messages a command emits as drilledMsg, recursing
// into batches. Control messages (quit, editor exec) and spinner ticks (already
// scoped by a spinner ID) pass through untouched so the runtime still sees them.
func tagDrilled(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			tagged := make(tea.BatchMsg, len(msg))
			for i, c := range msg {
				tagged[i] = tagDrilled(c)
			}
			return tagged
		case tui.SelectionChangedMsg, tui.PreviewMsg:
			return drilledMsg{msg}
		default:
			return msg
		}
	}
}

// sendDrilled forwards a message to the drilled diff view and tags the commands
// it returns. Centralizes the one diffui.Model type assertion.
func (m Model) sendDrilled(msg tea.Msg) (Model, tea.Cmd) {
	dm, cmd := m.drilled.Update(msg)
	m.drilled = dm.(diffui.Model)
	return m, tagDrilled(cmd)
}

// updateDrilled delegates to the embedded diff view, intercepting esc as "back
// to the commit list" (but letting esc clear a filter / close help there).
func (m Model) updateDrilled(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "esc" && !m.drilled.Filtering() && !m.drilled.ShowingHelp() {
			m.drilling = false
			return m, nil
		}
	}
	m, cmd := m.sendDrilled(msg)
	return m, cmd
}

// commitFiles returns the files a commit changed, plus the base ref to diff it
// against — the commit's parent, or the empty tree for a root commit.
func commitFiles(repo *git.Repo, hash string) (base string, files []git.ChangedFile) {
	base = hash + "~1"
	files, err := repo.DiffBetweenCommits(base, hash)
	if err != nil {
		base = git.EmptyTree // root commit: diff against the empty tree
		files, _ = repo.DiffBetweenCommits(base, hash)
	}
	return base, files
}

// drillInto opens the file-level diff view for a commit.
func (m Model) drillInto(commit git.CommitInfo) (tea.Model, tea.Cmd) {
	base, files := commitFiles(m.repo, commit.Hash)
	m.drilled = diffui.New(m.repo, m.engine, files, false, base, commit.Hash)
	m.drilling = true
	m, cmd := m.sendDrilled(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, cmd
}

// reloadFresh drops the preview cache and reloads the current selection, after a
// setting (layout/engine) that affects rendering has changed.
func (m Model) reloadFresh() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.ClearCacheAndReload()
	return m, cmd
}

func (m Model) View() tea.View {
	if m.drilling {
		return m.drilled.View()
	}
	return m.list.TeaView()
}

func (m Model) previewCmd(reqID int) tea.Cmd {
	commit, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine, width, display := m.repo, m.engine, m.list.PreviewWidth(), m.display
	return func() tea.Msg {
		base, files := commitFiles(repo, commit.Hash)
		color := tui.ColorEnabled()
		header := commitHeader(commit, files, color, width)
		content, err := engine.DiffCommit(context.Background(), repo.Root(), base, commit.Hash, color, width, display)
		if err != nil {
			return tui.PreviewMsg{Content: content, Err: err, ReqID: reqID}
		}
		return tui.PreviewMsg{Content: header + content, ReqID: reqID}
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
			status := git.StatusChar(f.Status)
			fileIcon := tui.FileIcon(f.Path)
			// Reserve room for the "  <status> <icon> " prefix; fall back to the
			// full path when the width isn't known yet (unsized preview).
			pathWidth := width - 6
			if pathWidth < 1 {
				pathWidth = len(f.Path)
			}
			path := tui.RenderPath(f.Path, pathWidth, false)
			if color {
				status = tui.StatusStyle(f.Status).Render(status)
			} else {
				fileIcon = ansi.Strip(fileIcon)
				path = ansi.Strip(path)
			}
			fmt.Fprintf(&b, "  %s %s %s\n", status, fileIcon, path)
		}
	}

	fmt.Fprintf(&b, "\n%s\n\n", sep)
	return b.String()
}
