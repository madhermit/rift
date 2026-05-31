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
	repo            *git.Repo
	engine          diff.Engine
	altEngine       diff.Engine // the other engine, swapped in by the `e` toggle
	canToggleEngine bool        // true when the two engines actually differ
	list            tui.SplitList[git.StashEntry]
	action          StashAction
	display         diff.Display
}

func New(repo *git.Repo, engine diff.Engine, stashes []git.StashEntry) Model {
	altEngine := diff.NewPlainEngine()
	canToggleEngine := engine.Name() != altEngine.Name()

	hints := [][2]string{
		{"↑↓", "nav"}, {"/", "filter"}, {"⇥", "switch"},
		{"a", "apply"}, {"p", "pop"}, {"x", "drop"}, {"\\", "layout"},
	}
	if canToggleEngine {
		hints = append(hints, [2]string{"e", "engine"})
	}
	hints = append(hints, [2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.StashEntry]{
		Screen:      "stash",
		ListTitle:   "stashes",
		Context:     engine.Name(),
		MinList:     30,
		MaxList:     80,
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
		Row: func(s git.StashEntry, w int, selected, collapsed bool) string {
			style := tui.TextStyle(selected)
			if collapsed {
				return style.Render(fmt.Sprintf("{%d}", s.Index))
			}
			return style.Render(tui.Truncate(fmt.Sprintf("stash@{%d}  %s", s.Index, s.Message), w))
		},
	}
	return Model{repo: repo, engine: engine, altEngine: altEngine, canToggleEngine: canToggleEngine, list: tui.NewSplitList(cfg, stashes)}
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
		}
		if action, ok := stashAction(msg.String()); ok {
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
