package stashui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
	lensui "github.com/madhermit/rift/internal/tui/lens"
)

type StashAction int

const (
	NoAction StashAction = iota
	Apply
	Pop
	Drop
)

type Model struct {
	repo     *git.Repo
	settings tui.ViewSettings
	list     tui.SplitList[git.StashEntry]
	action   StashAction
	confirm  tui.Confirm        // pending y/n gate for a destructive action (pop/drop)
	stream   *tui.PreviewStream // active stash-diff stream; nil otherwise
	drill    lensui.Drill       // the selected stash's files↔tests view, once opened
	width    int
	height   int
}

func New(repo *git.Repo, engine diff.Engine, stashes []git.StashEntry) Model {
	settings := tui.ViewSettings{Engines: tui.NewEngineToggle(engine)}
	branch := repo.CurrentBranch()

	hints := [][2]string{
		{"⏎", "files"}, {"t", "tests"}, {"/", "filter"}, {"⇥", "read"},
		{"a", "apply"}, {"p", "pop"}, {"x", "drop"}, {"\\", "layout"},
	}
	if settings.Engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	hints = append(hints, [2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.StashEntry]{
		Screen:      "stash",
		ListTitle:   "stashes",
		Branch:      branch,
		Context:     engine.Name(),
		NavFraction: 30,
		EmptyStatus: "No stashes found",
		Hints:       hints,
		Match: func(s git.StashEntry) string {
			return stashRef(s.Index) + " " + s.Branch + " " + s.Message
		},
		PreviewTitle: func(s git.StashEntry) string {
			return stashRef(s.Index) + " · " + s.Branch
		},
		CacheKey: func(s git.StashEntry) string { return fmt.Sprintf("%d", s.Index) },
		Yank:     func(s git.StashEntry) string { return stashRef(s.Index) },
		Row: func(s git.StashEntry, w int, selected bool) string {
			return tui.TextStyle(selected).Render(tui.Truncate(stashRef(s.Index)+"  "+s.Message, w))
		},
	}
	return Model{repo: repo, settings: settings, list: tui.NewSplitList(cfg, stashes)}
}

func (m Model) Action() StashAction { return m.action }

func (m Model) SelectedIndex() int {
	if s, ok := m.list.Selected(); ok {
		return s.Index
	}
	return -1
}

func (m Model) Init() tea.Cmd { return tui.ThemeInit() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// See logui.Model.Update: an open drilldown takes everything but the stash
	// list's own traffic, so no input can drift the entry an action would hit.
	if m.drill.Owns(msg) {
		next, cmd, closed := m.drill.Update(msg)
		m.drill = next
		if closed {
			m.list, m.settings, cmd = lensui.BackToList(m.drill, m.list, m.settings)
		}
		return m, cmd
	}
	if lensui.Discard(msg) {
		return m, nil // a late message from a closed drilldown
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var lc, dc tea.Cmd
		m.list, lc = m.list.Update(msg)
		m.drill, dc = m.drill.Resize(msg)
		return m, tea.Batch(lc, dc)
	case tui.SelectionChangedMsg:
		m.stream.Cancel() // stop the previous stash's streaming work
		m.stream = nil
		if m.drill.Open() || msg.CacheHit {
			return m, nil // hidden, or served from cache; no new load
		}
		return m, m.previewCmd(msg.ReqID)
	case tui.StreamReadyMsg:
		if m.drill.Open() {
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
	case tea.KeyPressMsg:
		if m.list.Capturing() || m.list.ShowingHelp() {
			break
		}
		if m.confirm.Active() {
			return m.resolveConfirm(msg.String())
		}
		switch msg.String() {
		case "enter":
			return m.openDrill(false)
		case "t":
			return m.openDrill(true)
		case "\\":
			m.settings.Display = m.settings.Display.Next()
			return m.reloadFresh()
		case "e":
			if !m.settings.Engines.CanToggle() {
				break // only one engine available; nothing to toggle
			}
			m.settings.Engines = m.settings.Engines.Toggle()
			m.list = m.list.SetContext(m.settings.Engines.Name())
			return m.reloadFresh()
		}
		if action, ok := stashAction(msg.String()); ok {
			return m.startAction(action)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// openDrill opens the selected stash's files↔tests view, on its tests side when
// tests is set. The list's own preview stream is cancelled: it's hidden now.
func (m Model) openDrill(tests bool) (tea.Model, tea.Cmd) {
	entry, ok := m.list.Selected()
	if !ok {
		return m, nil
	}
	m.stream.Cancel()
	m.stream = nil
	base, target := m.repo.StashRange(entry.Index)
	files, err := m.repo.DiffBetweenCommits(base, target)
	scope := review.DiffScope{Base: base, Target: target}
	lens := lensui.NewDrill(m.repo, m.settings, files, scope, tui.Breadcrumb("stash", stashRef(entry.Index)))
	if err != nil {
		lens = lens.SetEmptyStatus(fmt.Sprintf("could not load stash diff: %v", err))
	}
	var cmd tea.Cmd
	m.drill, cmd = lensui.OpenDrill(lens, m.width, m.height, tests)
	return m, cmd
}

func stashRef(index int) string { return fmt.Sprintf("stash@{%d}", index) }

func (m Model) View() tea.View {
	if m.drill.Open() {
		return m.drill.View()
	}
	return m.list.TeaView()
}

// reloadFresh drops the preview cache and reloads the current selection, after a
// setting (layout/engine) that affects rendering has changed.
func (m Model) reloadFresh() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.ClearCacheAndReload()
	return m, cmd
}

func stashAction(key string) (StashAction, bool) {
	switch key {
	case "a":
		return Apply, true
	case "p":
		return Pop, true
	case "x":
		return Drop, true
	}
	return NoAction, false
}

func (a StashAction) verb() string {
	switch a {
	case Apply:
		return "apply"
	case Pop:
		return "pop"
	case Drop:
		return "drop"
	}
	return ""
}

// startAction runs a chosen action on the selected stash. Action keys fire only
// from the list pane (not while reading the preview). Apply is non-destructive and
// quits immediately; pop and drop destroy the stash entry, so they first arm an
// inline y/n confirmation (resolveConfirm quits on "y").
func (m Model) startAction(action StashAction) (tea.Model, tea.Cmd) {
	entry, ok := m.list.Selected()
	if !ok || m.list.Reading() {
		return m, nil
	}
	m.action = action
	if action == Apply {
		return m, tea.Quit
	}
	m.confirm = tui.Ask(action.verb() + " " + stashRef(entry.Index))
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

// previewCmd streams the selected stash's diff: its files (see StashRange) are
// diffed in parallel and streamed in order, like the log's commit
// preview. The full result is cached per stash.
func (m Model) previewCmd(reqID int) tea.Cmd {
	entry, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine := m.repo, m.settings.Engines.Engine()
	opts := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.settings.Display)
	return func() tea.Msg {
		opts.Base, opts.Target = repo.StashRange(entry.Index)
		files, err := repo.DiffBetweenCommits(opts.Base, opts.Target)
		if err != nil {
			return tui.PreviewMsg{Err: err, ReqID: reqID}
		}
		if len(files) == 0 {
			return tui.StreamReadyMsg{ReqID: reqID}
		}
		ch, cancel := tui.StreamFiles(engine, repo.Root(), files, opts)
		return tui.StreamReadyMsg{ReqID: reqID, Ch: ch, Cancel: cancel}
	}
}
