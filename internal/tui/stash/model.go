package stashui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

type StashAction int

const (
	NoAction StashAction = iota
	Apply
	Pop
	Drop
)

type Model struct {
	repo    *git.Repo
	engines tui.EngineToggle
	list    tui.SplitList[git.StashEntry]
	action  StashAction
	confirm tui.Confirm // pending y/n gate for a destructive action (pop/drop)
	display diff.Display
	stream  *tui.PreviewStream // active stash-diff stream; nil otherwise
}

func New(repo *git.Repo, engine diff.Engine, stashes []git.StashEntry) Model {
	engines := tui.NewEngineToggle(engine)
	branch := repo.CurrentBranch()

	hints := [][2]string{
		{"/", "filter"}, {"⇥", "read"},
		{"a", "apply"}, {"p", "pop"}, {"x", "drop"}, {"\\", "layout"},
	}
	if engines.CanToggle() {
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
			return fmt.Sprintf("stash@{%d} %s %s", s.Index, s.Branch, s.Message)
		},
		PreviewTitle: func(s git.StashEntry) string {
			return fmt.Sprintf("stash@{%d} · %s", s.Index, s.Branch)
		},
		CacheKey: func(s git.StashEntry) string { return fmt.Sprintf("%d", s.Index) },
		Yank:     func(s git.StashEntry) string { return fmt.Sprintf("stash@{%d}", s.Index) },
		Row: func(s git.StashEntry, w int, selected bool) string {
			return tui.TextStyle(selected).Render(tui.Truncate(fmt.Sprintf("stash@{%d}  %s", s.Index, s.Message), w))
		},
	}
	return Model{repo: repo, engines: engines, list: tui.NewSplitList(cfg, stashes)}
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
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		m.stream.Cancel() // stop the previous stash's streaming work
		m.stream = nil
		if msg.CacheHit {
			return m, nil // preview served from cache; no new load
		}
		return m, m.previewCmd(msg.ReqID)
	case tui.StreamReadyMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.ApplyStream(m.list, m.stream, msg)
		return m, cmd
	case tui.ChunkMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.AdvanceStream(m.list, m.stream, msg)
		return m, cmd
	case tea.KeyPressMsg:
		if m.list.Filtering() || m.list.ShowingHelp() {
			break
		}
		if m.confirm.Active() {
			return m.resolveConfirm(msg.String())
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
			m.list = m.list.SetContext(m.engines.Name())
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

func (m Model) View() tea.View { return m.list.TeaView() }

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
	m.confirm = tui.Ask(fmt.Sprintf("%s stash@{%d}", action.verb(), entry.Index))
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

// previewCmd streams the selected stash's diff: its files (between the stash and
// its parent) are diffed in parallel and streamed in order, like the log's commit
// preview. The full result is cached per stash.
func (m Model) previewCmd(reqID int) tea.Cmd {
	entry, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine := m.repo, m.engines.Engine()
	opts := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.display)
	return func() tea.Msg {
		ref := fmt.Sprintf("stash@{%d}", entry.Index)
		opts.Base, opts.Target = ref+"^", ref
		files, err := repo.DiffBetweenCommits(opts.Base, ref)
		if err != nil {
			return tui.PreviewMsg{Err: err, ReqID: reqID}
		}
		if len(files) == 0 {
			return tui.StreamReadyMsg{ReqID: reqID}
		}
		ch, cancel := tui.StreamFiles(engine, repo.Root(), git.Paths(files), opts)
		return tui.StreamReadyMsg{ReqID: reqID, Ch: ch, Cancel: cancel}
	}
}
