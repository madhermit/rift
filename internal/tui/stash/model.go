package stashui

import (
	"context"
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
	engine  diff.Engine
	list    tui.SplitList[git.StashEntry]
	action  StashAction
	display diff.Display
}

func New(repo *git.Repo, engine diff.Engine, stashes []git.StashEntry) Model {
	cfg := tui.SplitConfig[git.StashEntry]{
		Screen:      "stash",
		ListTitle:   "stashes",
		Context:     engine.Name(),
		MinList:     30,
		MaxList:     80,
		EmptyStatus: "No stashes found",
		Hints: [][2]string{
			{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"},
			{"a", "apply"}, {"p", "pop"}, {"x", "drop"}, {"\\", "layout"}, {"q", "quit"},
		},
		Match: func(s git.StashEntry) string {
			return fmt.Sprintf("stash@{%d} %s %s", s.Index, s.Branch, s.Message)
		},
		PreviewTitle: func(s git.StashEntry) string {
			return fmt.Sprintf("stash@{%d} · %s", s.Index, s.Branch)
		},
		CacheKey: func(s git.StashEntry) string { return fmt.Sprintf("%d", s.Index) },
		Row: func(s git.StashEntry, w int, selected, collapsed bool) string {
			style := tui.TextStyle(selected)
			if collapsed {
				return style.Render(fmt.Sprintf("{%d}", s.Index))
			}
			return style.Render(tui.Truncate(fmt.Sprintf("stash@{%d}  %s", s.Index, s.Message), w))
		},
	}
	return Model{repo: repo, engine: engine, list: tui.NewSplitList(cfg, stashes)}
}

func (m Model) Action() StashAction { return m.action }

func (m Model) SelectedIndex() int {
	if s, ok := m.list.Selected(); ok {
		return s.Index
	}
	return -1
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		return m, m.previewCmd(msg.ReqID)
	case tea.KeyPressMsg:
		if msg.String() == "\\" && !m.list.Filtering() {
			m.display = m.display.Next()
			var cmd tea.Cmd
			m.list, cmd = m.list.ClearCacheAndReload()
			return m, cmd
		}
		if action, ok := stashAction(msg.String()); ok && !m.list.Filtering() {
			if _, sel := m.list.Selected(); sel {
				m.action = action
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View { return m.list.TeaView() }

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

func (m Model) previewCmd(reqID int) tea.Cmd {
	entry, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine, width, display := m.repo, m.engine, m.list.PreviewWidth(), m.display
	return func() tea.Msg {
		ref := fmt.Sprintf("stash@{%d}", entry.Index)
		base := ref + "^"
		color := tui.ColorEnabled()
		content, err := engine.DiffCommit(context.Background(), repo.Root(), base, ref, color, width, display)
		if err != nil {
			return tui.PreviewMsg{Err: err, ReqID: reqID}
		}
		return tui.PreviewMsg{Content: content, ReqID: reqID}
	}
}
