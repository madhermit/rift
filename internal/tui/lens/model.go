// Package lensui presents a diff as either a file list or a tests list, toggled
// live with `t`. It wraps the two SplitList-backed lenses (diffui and reviewui)
// over the same diff scope, keeping each built so a toggle preserves its state.
package lensui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
	diffui "github.com/madhermit/rift/internal/tui/diff"
	reviewui "github.com/madhermit/rift/internal/tui/review"
)

// Model toggles a diff between its file view and its tests view. Each lens is
// built once and kept, so toggling back preserves its scroll/selection. The
// wrapper owns the diff scope — including the `s` staged toggle — so the two
// lenses can't disagree about which side of the index they show.
//
// Only the shown lens runs. Its async preview traffic is tagged with a
// generation so a message from the lens left behind by a toggle is dropped: the
// two lenses are independent SplitLists whose request IDs both start at 0 and
// would otherwise collide. The first switch to tests parses the diff off the
// event loop (collectTests) rather than blocking.
type Model struct {
	repo   *git.Repo
	engine diff.Engine
	scope  review.DiffScope
	files  []git.ChangedFile

	filesLens  tui.PreviewChild // built lazily / at New; nil until first shown
	testsLens  tui.PreviewChild
	showTests  bool
	unreviewed bool // open the file lens with the unreviewed-only filter on
	gen        int
	width      int
	height     int
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, scope review.DiffScope, showTests, unreviewed bool) Model {
	m := Model{repo: repo, engine: engine, scope: scope, files: files, showTests: showTests, unreviewed: unreviewed}
	if showTests {
		m.testsLens = m.buildTests() // sync at startup is fine
	} else {
		m.filesLens = m.buildFiles()
	}
	return m
}

func (m Model) buildFiles() tui.PreviewChild {
	return diffui.New(m.repo, m.engine, m.files, m.scope.Staged, m.scope.Base, m.scope.Target, true, m.scope.Paths, m.unreviewed)
}

func (m Model) buildTests() tui.PreviewChild {
	specs, _ := review.Collect(m.repo, m.scope)
	return reviewui.New(m.repo, m.engine, specs, m.scope, true)
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case lensMsg:
		if msg.gen != m.gen {
			return m, nil // a stale preview message from a lens we toggled away from
		}
		return m.delegate(msg.msg)
	case lensCollectedMsg:
		if msg.gen != m.gen {
			return m, nil // a superseded tests build (toggled again before it landed)
		}
		m.testsLens = reviewui.New(m.repo, m.engine, msg.specs, m.scope, true)
		return m.show(true)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.delegate(msg)
	case tea.KeyPressMsg:
		if m.shown().Filtering() || m.shown().ShowingHelp() {
			break // let the child handle keys while filtering / in help
		}
		switch msg.String() {
		case "t":
			return m.toggle()
		case "s":
			if !m.scope.Committed() {
				return m.toggleStaged()
			}
		}
	}
	return m.delegate(msg)
}

// shownPtr returns a pointer to the field holding the currently-shown lens, so
// the showTests selector lives in one place: the helpers below read and write
// the active lens through it instead of each branching on showTests. The value
// receiver's copy is addressable, so &m.field points into the copy the caller
// returns.
func (m *Model) shownPtr() *tui.PreviewChild {
	if m.showTests {
		return &m.testsLens
	}
	return &m.filesLens
}

func (m Model) shown() tui.PreviewChild { return *m.shownPtr() }

func (m Model) toggle() (tea.Model, tea.Cmd) {
	if m.showTests {
		if m.filesLens == nil {
			m.filesLens = m.buildFiles()
		}
		return m.show(false)
	}
	if m.testsLens == nil {
		// First switch to tests: parse off the event loop, staying on the file
		// view until the specs land. The gen bump invalidates any prior pending
		// build and the file lens's in-flight stream messages.
		m.gen++
		return m, m.collectTests(m.gen)
	}
	return m.show(true)
}

// toggleStaged flips the staged side for both lenses, keeping them in sync: the
// other lens is dropped so it rebuilds with the new staged state on the next
// toggle. The file lens is cheap to rebuild now; the tests lens re-parses off the
// event loop (like the first switch to tests) so `s` doesn't block the UI.
func (m Model) toggleStaged() (tea.Model, tea.Cmd) {
	m = m.cancelShown()
	m.scope.Staged = !m.scope.Staged
	m.gen++
	m.files = worktreeFiles(m.repo, m.scope.Staged, m.scope.Paths)
	if m.showTests {
		// Keep showing the now-stale tests until the re-collect lands.
		m.filesLens = nil
		return m, m.collectTests(m.gen)
	}
	m.filesLens, m.testsLens = m.buildFiles(), nil
	return m.delegate(tea.WindowSizeMsg{Width: m.width, Height: m.height})
}

// show switches the displayed lens and re-syncs its layout. It cancels the lens
// being hidden so its difftastic stream doesn't linger, and the gen bump drops
// that lens's in-flight preview messages.
func (m Model) show(tests bool) (tea.Model, tea.Cmd) {
	m = m.cancelShown()
	m.showTests = tests
	m.gen++
	return m.delegate(tea.WindowSizeMsg{Width: m.width, Height: m.height})
}

// cancelShown stops the currently-shown lens's in-flight diff stream.
func (m Model) cancelShown() Model {
	if p := m.shownPtr(); *p != nil {
		*p = (*p).CancelPreview().(tui.PreviewChild)
	}
	return m
}

func (m Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	p := m.shownPtr()
	nm, cmd := (*p).Update(msg)
	*p = nm.(tui.PreviewChild)
	return m, tagLens(m.gen, cmd)
}

func (m Model) View() tea.View { return m.shown().View() }

func (m Model) collectTests(gen int) tea.Cmd {
	repo, scope := m.repo, m.scope
	return func() tea.Msg {
		specs, _ := review.Collect(repo, scope)
		return lensCollectedMsg{gen: gen, specs: specs}
	}
}

// worktreeFiles lists the changed files for a working-tree staged toggle, sorted
// and pathspec-filtered like the diff command's own listing — so the file lens
// keeps the `-- path` scope that the tests lens (via review.Collect) already does.
func worktreeFiles(repo *git.Repo, staged bool, paths []string) []git.ChangedFile {
	files, err := repo.ChangedFiles(staged)
	if err != nil {
		return nil
	}
	git.SortByPath(files)
	return git.FilterByPaths(files, paths)
}

// lensCollectedMsg delivers the parsed tests for the first switch to the tests
// lens, tagged with the generation that requested it.
type lensCollectedMsg struct {
	gen   int
	specs []review.Spec
}

// lensMsg wraps a shown lens's async preview messages with the generation they
// were emitted under, so a stale message from a toggled-away lens is dropped.
type lensMsg struct {
	gen int
	msg tea.Msg
}

// tagLens tags the shown lens's preview messages with the current generation so
// a stale message from a toggled-away lens (an older gen) is dropped.
func tagLens(gen int, cmd tea.Cmd) tea.Cmd {
	return tui.WrapPreviewCmd(cmd, func(msg tea.Msg) tea.Msg { return lensMsg{gen: gen, msg: msg} })
}
