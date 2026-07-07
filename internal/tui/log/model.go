package logui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
	diffui "github.com/madhermit/rift/internal/tui/diff"
	reviewui "github.com/madhermit/rift/internal/tui/review"
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
	repo    *git.Repo
	engines tui.EngineToggle
	list    tui.SplitList[git.CommitInfo]
	display diff.Display
	action  Action
	confirm tui.Confirm        // pending y/n gate for cherry-pick / revert
	stream  *tui.PreviewStream // active commit-diff stream; nil otherwise

	drilled    tui.PreviewChild // per-commit view (file diff, or tests) when drilling
	drilling   bool
	collecting bool // a tests-lens collect is in flight (between enter and open)
	tests      bool // the tests lens: drill into a commit's tests, not its files
	width      int
	height     int
}

// Action reports the git operation chosen for the selected commit (NoAction if
// the user just quit).
func (m Model) Action() Action { return m.action }

func (a Action) verb() string {
	switch a {
	case CherryPick:
		return "cherry-pick"
	case Revert:
		return "revert"
	}
	return ""
}

// startAction arms cherry-pick / revert on the selected commit behind an inline
// y/n confirmation (both rewrite history / the working tree). resolveConfirm quits
// with the action on "y".
func (m Model) startAction(action Action) (tea.Model, tea.Cmd) {
	commit, ok := m.list.Selected()
	if !ok {
		return m, nil
	}
	m.action = action
	m.confirm = tui.Ask(fmt.Sprintf("%s %s", action.verb(), commit.Hash))
	m.list = m.list.SetPrompt(m.confirm.Prompt())
	return m, nil
}

// resolveConfirm answers a pending y/n confirmation: "y" runs the armed action
// (quit), anything else cancels it.
func (m Model) resolveConfirm(key string) (tea.Model, tea.Cmd) {
	confirmed, cleared := m.confirm.Answer(key)
	m.confirm = cleared
	m.list = m.list.SetPrompt("")
	if confirmed {
		return m, tea.Quit
	}
	m.action = NoAction
	return m, nil
}

// SelectedHash is the hash of the commit the user acted on, or "" if none.
func (m Model) SelectedHash() string {
	if c, ok := m.list.Selected(); ok {
		return c.Hash
	}
	return ""
}

func New(repo *git.Repo, engine diff.Engine, commits []git.CommitInfo, tests bool) Model {
	engines := tui.NewEngineToggle(engine)
	branch := repo.CurrentBranch()

	drillLabel := "files"
	if tests {
		drillLabel = "tests"
	}
	hints := [][2]string{
		{"⏎", drillLabel}, {"/", "filter"}, {"⇥", "read"}, {"\\", "layout"},
	}
	if engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	hints = append(hints,
		[2]string{"c", "cherry-pick"}, [2]string{"r", "revert"},
		[2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.CommitInfo]{
		Screen:       "log",
		ListTitle:    "commits",
		Branch:       branch,
		Context:      engine.Name(),
		NavFraction:  30,
		EmptyStatus:  "No commits found",
		Hints:        hints,
		Match:        func(c git.CommitInfo) string { return c.Hash + " " + c.Message },
		PreviewTitle: func(c git.CommitInfo) string { return c.Hash },
		CacheKey:     func(c git.CommitInfo) string { return c.Hash },
		Yank:         func(c git.CommitInfo) string { return c.Hash },
		Row: func(c git.CommitInfo, w int, selected bool) string {
			return tui.TextStyle(selected).Render(tui.Truncate(c.Hash+"  "+c.Message, w))
		},
	}
	return Model{repo: repo, engines: engines, tests: tests, list: tui.NewSplitList(cfg, commits)}
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
	// The drilled tests view can open an editor (`o`); its close message must reach
	// that view (to re-read the edited file), not the hidden commit list below.
	if _, ok := msg.(tui.EditorClosedMsg); ok && m.drilling {
		return m.updateDrilled(msg)
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
		m.stream.Cancel() // stop the previous commit's streaming work
		m.stream = nil
		if m.drilling || m.collecting || msg.CacheHit {
			return m, nil // hidden/about-to-be, or served from cache; don't stream
		}
		return m, m.previewCmd(msg.ReqID)
	case tui.StreamReadyMsg:
		if m.drilling || m.collecting {
			if msg.Cancel != nil {
				msg.Cancel() // a load that landed after we left the list; drop it
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.ApplyStream(m.list, m.stream, msg)
		return m, cmd
	case tui.ChunkMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.AdvanceStream(m.list, m.stream, msg)
		return m, cmd
	case testsCollectedMsg:
		// Collected off the event loop (see collectTests); open the tests
		// drilldown. A collection error surfaces as an empty drilldown, matching
		// the file drilldown's handling.
		m.collecting = false
		m.drilled = reviewui.New(m.repo, m.engines.Engine(), msg.specs, msg.scope, false)
		m.drilling = true
		var cmd tea.Cmd
		m, cmd = m.sendDrilled(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, cmd
	case tea.KeyPressMsg:
		if m.list.Filtering() || m.list.ShowingHelp() {
			break
		}
		if m.confirm.Active() {
			return m.resolveConfirm(msg.String())
		}
		switch msg.String() {
		case "enter":
			if c, ok := m.list.Selected(); ok {
				if m.tests && m.collecting {
					break // a tests collect is already in flight; ignore re-enter
				}
				m.stream.Cancel() // the list preview is hidden once a commit opens
				m.stream = nil
				if m.tests {
					// Collecting a commit's tests parses every changed file and
					// shells git per file; run it off the event loop, then open.
					// `collecting` keeps the list from streaming or re-entering
					// until the specs land.
					m.collecting = true
					return m, m.collectTests(c)
				}
				return m.drillInto(c)
			}
		case "\\":
			m.display = m.display.Next()
			return m.reloadFresh()
		case "e":
			if !m.engines.CanToggle() {
				break // only one engine available; nothing to toggle
			}
			m.engines = m.engines.Toggle()
			m.list = m.list.SetContext(m.engines.Name())
			return m.reloadFresh()
		case "c":
			return m.startAction(CherryPick)
		case "r":
			return m.startAction(Revert)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// drilledMsg wraps a message produced by the drilled diff view so the parent can
// route it back without confusing it with the commit list's own preview traffic.
type drilledMsg struct{ msg tea.Msg }

// tagDrilled wraps the drilled view's preview messages as drilledMsg so the
// parent routes them back to it rather than confusing them with the commit
// list's own preview traffic (their ReqIDs would collide).
func tagDrilled(cmd tea.Cmd) tea.Cmd {
	return tui.WrapPreviewCmd(cmd, func(msg tea.Msg) tea.Msg { return drilledMsg{msg} })
}

// sendDrilled forwards a message to the drilled diff view and tags the commands
// it returns. Centralizes the one diffui.Model type assertion.
func (m Model) sendDrilled(msg tea.Msg) (Model, tea.Cmd) {
	dm, cmd := m.drilled.Update(msg)
	m.drilled = dm.(tui.PreviewChild)
	return m, tagDrilled(cmd)
}

// updateDrilled delegates to the embedded diff view, intercepting esc as "back
// to the commit list" (but letting esc clear a filter / close help there).
func (m Model) updateDrilled(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "esc" && !m.drilled.Filtering() && !m.drilled.ShowingHelp() {
			m.drilling = false
			// Refresh the commit preview, which was frozen (and possibly cut off
			// mid-stream) while drilled — a cache hit if it had finished.
			var cmd tea.Cmd
			m.list, cmd = m.list.Reload()
			return m, cmd
		}
	}
	m, cmd := m.sendDrilled(msg)
	return m, cmd
}

// commitFiles returns the files a commit changed, plus the base ref to diff it
// against — the commit's parent, or the empty tree for a root commit.
func commitFiles(repo *git.Repo, hash string) (base string, files []git.ChangedFile, err error) {
	base = hash + "~1"
	files, err = repo.DiffBetweenCommits(base, hash)
	if err != nil {
		base = git.EmptyTree // root commit: diff against the empty tree
		files, err = repo.DiffBetweenCommits(base, hash)
	}
	return base, files, err
}

// drillInto opens the file-level diff view for a commit. The commit-list's own
// preview stream is cancelled — the drilled view does its own (streamed) diff,
// and the hidden list preview shouldn't keep difftastic busy.
func (m Model) drillInto(commit git.CommitInfo) (tea.Model, tea.Cmd) {
	base, files, err := commitFiles(m.repo, commit.Hash)
	dv := diffui.New(m.repo, m.engines.Engine(), files, false, base, commit.Hash, false, nil)
	if err != nil {
		// Surface the load failure instead of an indistinguishable "No changes found".
		dv = dv.SetEmptyStatus(fmt.Sprintf("could not load commit diff: %v", err))
	}
	m.drilled = dv
	m.drilling = true
	m, cmd := m.sendDrilled(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, cmd
}

// testsCollectedMsg carries a commit's collected test specs back to the event
// loop so the tests drilldown can open without blocking on parsing + git. A
// collection error surfaces as an empty drilldown (like the file drilldown), so
// it isn't carried here.
type testsCollectedMsg struct {
	scope review.DiffScope
	specs []review.Spec
}

// collectTests parses the selected commit's touched tests off the event loop.
func (m Model) collectTests(commit git.CommitInfo) tea.Cmd {
	repo, hash := m.repo, commit.Hash
	return func() tea.Msg {
		base, _, _ := commitFiles(repo, hash)
		scope := review.DiffScope{Base: base, Target: hash}
		specs, _ := review.Collect(repo, scope)
		return testsCollectedMsg{scope: scope, specs: specs}
	}
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

// previewCmd builds the selected commit's preview. The commit header (metadata +
// file list) is shown as soon as the file list is computed; the per-file diffs
// then stream in beneath it, in order, diffed in parallel — so a large commit
// paints its header and first file immediately instead of blocking on the whole
// diff. The accumulated result is cached (per commit) when the stream completes.
func (m Model) previewCmd(reqID int) tea.Cmd {
	commit, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine := m.repo, m.engines.Engine()
	opts := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.display)
	return func() tea.Msg {
		base, files, err := commitFiles(repo, commit.Hash)
		if err != nil {
			return tui.PreviewMsg{Err: err, ReqID: reqID}
		}
		header := commitHeader(commit, files, opts.Color, opts.Width)
		if len(files) == 0 {
			return tui.StreamReadyMsg{ReqID: reqID, Header: header}
		}
		opts.Base, opts.Target = base, commit.Hash
		ch, cancel := tui.StreamFiles(engine, repo.Root(), git.Paths(files), opts)
		return tui.StreamReadyMsg{ReqID: reqID, Header: header, Ch: ch, Cancel: cancel}
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
