package reviewui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
)

var (
	pathStyle   = lipgloss.NewStyle().Foreground(tui.Subtle)
	ticketStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))

	// Glyph colors by change kind. The hues match the app's add/modify/rename
	// semantics (green/yellow/accent), but the glyphs are the tests lens's own
	// (+/→/~) so they don't read as git's A/R/M file markers.
	addedGlyph    = lipgloss.NewStyle().Foreground(tui.Green)
	renamedGlyph  = lipgloss.NewStyle().Foreground(tui.Accent)
	modifiedGlyph = lipgloss.NewStyle().Foreground(tui.Yellow)
)

type Model struct {
	repo    *git.Repo
	engines tui.EngineToggle
	list    tui.SplitList[review.Spec]

	scope    review.DiffScope
	display  diff.Display
	stream   *tui.PreviewStream // active spec-diff stream; nil otherwise
	watching bool               // show the live-watch indicator in the header context
}

// New builds the tests lens for a diff. The scope decides whether it reads the
// working tree (with an `s` staged toggle) or a committed range (e.g. a commit
// drilled into from the log).
func New(repo *git.Repo, engine diff.Engine, specs []review.Spec, scope review.DiffScope, lensToggle bool) Model {
	engines := tui.NewEngineToggle(engine)

	hints := [][2]string{{"/", "filter"}, {"⇥", "read"}}
	if lensToggle {
		hints = append(hints, [2]string{"t", "files"})
	}
	if !scope.Committed() {
		hints = append(hints, [2]string{"s", "staged"}, [2]string{"w", "watch"})
	}
	hints = append(hints, [2]string{"\\", "layout"})
	if engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	// `o` jumps to the test in the working-tree file. Unlike the diff view it's
	// offered even for a committed range (e.g. log --tests): the intent is to go
	// edit the test, and its line is a fine best-effort target.
	hints = append(hints, [2]string{"o", "open"})
	hints = append(hints, [2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[review.Spec]{
		Screen:      tui.Breadcrumb("diff", "tests"),
		ListTitle:   listTitle(scope),
		Branch:      repo.CurrentBranch(),
		Context:     tui.ContextLabel(scope.Target, engine.Name()),
		NavFraction: 40,
		EmptyStatus: "No test changes in scope",
		Hints:       hints,
		Match: func(s review.Spec) string {
			return s.File + " " + strings.Join(s.Path, " ") + " " + s.Name + " " + s.Ticket
		},
		PreviewTitle:  func(s review.Spec) string { return s.File },
		PreviewHeader: specHeader,
		CacheKey:      func(s review.Spec) string { return s.File },
		Yank:          func(s review.Spec) string { return s.Name },
		PreviewLine:   func(s review.Spec) int { return s.Line },
		Row:           specRow,
	}

	return Model{
		repo:    repo,
		engines: engines,
		scope:   scope,
		list:    tui.NewSplitList(cfg, specs),
	}
}

func listTitle(scope review.DiffScope) string {
	if scope.Committed() {
		return "tests"
	}
	return tui.ToggleTitle("unstaged", "staged", scope.Staged)
}

// glyphStyle is the color for a spec's change-kind glyph (see Spec.Glyph): green
// added, accent renamed, yellow modified.
func glyphStyle(status string) lipgloss.Style {
	switch status {
	case "added":
		return addedGlyph
	case "renamed":
		return renamedGlyph
	default:
		return modifiedGlyph
	}
}

// specHeader is the sticky preview label: the change-kind glyph and the test's
// "describe › path › name". It names the change in the preview pane even when the
// diff itself doesn't show the declaration line (a body-only modification), so
// you never lose track of which test — or what kind of change — you're looking at.
func specHeader(s review.Spec) string {
	return glyphStyle(s.Status).Render(s.Glyph()) + " " + s.PathPrefix() + s.Name
}

// specRow renders a spec as "<glyph> <describe › path ›> name  TICKET", with the
// nesting path dimmed and the ticket pinned to the right when it fits. The glyph
// (+/→/~) marks how the diff changed the test: added / renamed / modified.
func specRow(s review.Spec, w int, selected bool) string {
	name := tui.TextStyle(selected).Render(s.Name)
	if p := s.PathPrefix(); p != "" {
		name = pathStyle.Render(p) + name
	}
	left := glyphStyle(s.Status).Render(s.Glyph()) + " " + name

	ticket := ""
	if s.Ticket != "" {
		ticket = ticketStyle.Render(s.Ticket)
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(ticket)
	if ticket == "" || gap < 1 {
		return tui.Truncate(left, w)
	}
	return left + strings.Repeat(" ", gap) + ticket
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		m.stream.Cancel()
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
				break
			}
			m.engines = m.engines.Toggle()
			m.list = m.list.SetContext(m.contextLabel())
			return m.reloadFresh()
		case "o":
			if sel, ok := m.list.Selected(); ok {
				return m, tui.OpenInEditor(m.repo.Root(), sel.File, sel.Line)
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View { return m.list.TeaView() }

// Filtering and ShowingHelp expose the embedded list's modal state so a parent
// embedding this model (the log drilldown) knows when not to intercept esc.
func (m Model) Filtering() bool   { return m.list.Filtering() }
func (m Model) ShowingHelp() bool { return m.list.ShowingHelp() }

// SetWatching sets the "watching" tag in the header context, marking the lens
// as live-reloading. The flag persists on the model because the engine toggle
// rebuilds the label.
func (m Model) SetWatching(on bool) Model {
	m.watching = on
	m.list = m.list.SetContext(m.contextLabel())
	return m
}

func (m Model) contextLabel() string {
	label := tui.ContextLabel(m.scope.Target, m.engines.Name())
	if m.watching {
		label += " · watching"
	}
	return label
}

// SetBreadcrumb marks this view as an embedded drilldown: it sets the header
// breadcrumb (e.g. "log ❯ a1b2c3d ❯ tests") and adds an "esc back" footer hint.
func (m Model) SetBreadcrumb(screen string) Model {
	m.list = m.list.WithBreadcrumb(screen)
	return m
}

// SetFlash sets the embedded list's transient footer message (see
// SplitList.SetFlash), used to surface "collecting tests…" during a re-collect.
func (m Model) SetFlash(s string) Model {
	m.list = m.list.SetFlash(s)
	return m
}

// CancelPreview stops this model's in-flight diff stream — used by the lens
// wrapper before hiding it, so a backgrounded difftastic run doesn't linger.
func (m Model) CancelPreview() tea.Model {
	m.stream.Cancel()
	m.stream = nil
	return m
}

func (m Model) reloadFresh() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.ClearCacheAndReload()
	return m, cmd
}

// previewCmd streams the structural diff of the selected spec's file, so the
// right pane shows the change that added or modified the test.
func (m Model) previewCmd(reqID int) tea.Cmd {
	sel, ok := m.list.Selected()
	if !ok {
		return nil
	}
	opts := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.display)
	opts.Staged, opts.Base, opts.Target = m.scope.Staged, m.scope.Base, m.scope.Target
	root, engine := m.repo.Root(), m.engines.Engine()
	return func() tea.Msg {
		ch, cancel := tui.StreamFiles(engine, root, []git.ChangedFile{{Path: sel.File}}, opts)
		return tui.StreamReadyMsg{ReqID: reqID, Ch: ch, Cancel: cancel}
	}
}
