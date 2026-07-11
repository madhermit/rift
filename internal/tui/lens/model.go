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
	watch      bool // poll the working tree and refresh on change (see watch.go)
	watchFP    string
	watchGen   int  // invalidates in-flight ticks/polls when the chain or scope changes
	collecting bool // a tests collect for the current gen is in flight
	gen        int
	width      int
	height     int
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, scope review.DiffScope, showTests, unreviewed, watch bool) Model {
	m := Model{repo: repo, engine: engine, scope: scope, files: files, showTests: showTests, unreviewed: unreviewed, watch: watch}
	if watch {
		// Seed the fingerprint from the files the lens opens with, so a change
		// landing before the first poll still registers as a change.
		m.watchFP = fingerprint(files, watchHashes(repo, scope, files))
	}
	if showTests {
		m.testsLens = m.buildTests() // sync at startup is fine
	} else {
		m.filesLens = m.buildFiles()
	}
	return m
}

func (m Model) buildFiles() tui.PreviewChild {
	lens := diffui.New(m.repo, m.engine, m.files, m.scope.Staged, m.scope.Base, m.scope.Target, true, m.scope.Paths, m.unreviewed)
	return lens.SetWatching(m.watch)
}

func (m Model) buildTests() tui.PreviewChild {
	specs, _ := review.Collect(m.repo, m.scope)
	return m.newTests(specs)
}

// newTests builds the tests lens over already-collected specs, tagged with the
// current watch state.
func (m Model) newTests(specs []review.Spec) tui.PreviewChild {
	return reviewui.New(m.repo, m.engine, specs, m.scope, true).SetWatching(m.watch)
}

func (m Model) Init() tea.Cmd {
	if m.watch {
		return tea.Batch(tui.ThemeInit(), watchTick(m.watchGen))
	}
	return tui.ThemeInit()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case watchTickMsg, watchStateMsg:
		return m.watchUpdate(msg)
	case lensMsg:
		if msg.gen != m.gen {
			// A stale message from a superseded generation. A stream that had just
			// started never got its cancel registered — invoke it here, or its
			// difftastic subprocesses would run to completion unmanaged.
			if sr, ok := msg.msg.(tui.StreamReadyMsg); ok && sr.Cancel != nil {
				sr.Cancel()
			}
			return m, nil
		}
		return m.delegate(msg.msg)
	case lensCollectedMsg:
		if msg.gen != m.gen {
			return m, nil // a superseded tests build (toggled again before it landed)
		}
		m.collecting = false
		m = m.clearCollecting() // the specs landed; drop the "collecting…" note
		if msg.apply && m.showTests {
			// A watch refresh: apply the fresh specs to the shown lens in place,
			// preserving its engine/layout/filter and the reviewer's selection.
			if _, ok := m.testsLens.(reviewui.Model); ok {
				return m.delegate(reviewui.SpecsChangedMsg{Specs: msg.specs})
			}
		}
		if m.showTests {
			// Replacing a visible stale lens: kill any stream it started during
			// the collect, or its difftastic runs outlive the discarded model.
			m = m.cancelShown()
		}
		m.testsLens = m.newTests(msg.specs)
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
		case "w":
			if !m.scope.Committed() {
				return m.toggleWatch()
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
		// build and the file lens's in-flight stream messages. A footer note tells
		// the user the collect is running.
		m.gen++
		m = m.noteCollecting()
		m.collecting = true
		return m, m.collectTests(m.gen, false)
	}
	return m.show(true)
}

// noteCollecting shows a "collecting tests…" footer indicator on whichever lens
// stays visible while review.Collect runs off the event loop. It goes through
// the flash mechanism, which sits below the filter/confirm footer and clears on
// the next navigation, so it never fights those.
func (m Model) noteCollecting() Model {
	p := m.shownPtr()
	switch lens := (*p).(type) {
	case diffui.Model:
		*p = lens.SetFlash("collecting tests…")
	case reviewui.Model:
		*p = lens.SetFlash("collecting tests…")
	}
	return m
}

// clearCollecting drops the "collecting…" note once the specs land; only the
// lens that was visible during the collect holds it.
func (m Model) clearCollecting() Model {
	if d, ok := m.filesLens.(diffui.Model); ok {
		m.filesLens = d.SetFlash("")
	}
	if r, ok := m.testsLens.(reviewui.Model); ok {
		m.testsLens = r.SetFlash("")
	}
	return m
}

// toggleStaged flips the staged side for both lenses, keeping them in sync: the
// other lens is dropped so it rebuilds with the new staged state on the next
// toggle. The file lens is cheap to rebuild now; the tests lens re-parses off the
// event loop (like the first switch to tests) so `s` doesn't block the UI.
func (m Model) toggleStaged() (tea.Model, tea.Cmd) {
	m.scope.Staged = !m.scope.Staged
	m.files, _ = scopeFiles(m.repo, m.scope) // best-effort, like the reload paths
	var rearm tea.Cmd
	if m.watch {
		// A poll in flight captured the old scope: orphan it (gen bump) and re-arm
		// a fresh chain, re-seeding the fingerprint so the flip itself doesn't
		// read as a tree change on the next poll.
		m.watchGen++
		m.watchFP = fingerprint(m.files, watchHashes(m.repo, m.scope, m.files))
		rearm = watchTick(m.watchGen)
	}
	if m.showTests {
		model, cmd := m.recollect(false)
		return model, tea.Batch(cmd, rearm)
	}
	m = m.cancelShown()
	m.gen++
	m.collecting = false // a pending `t` collect targeted the old scope; let it drop
	m.filesLens, m.testsLens = m.buildFiles(), nil
	model, cmd := m.delegate(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return model, tea.Batch(cmd, rearm)
}

// recollect invalidates the current specs and re-collects for the tests view:
// the stale tests lens stays visible (with a "collecting" note) until the
// fresh specs land. apply=true applies them to the shown lens in place (a
// watch refresh — the reviewer's view state survives); apply=false rebuilds
// the lens (a scope flip).
func (m Model) recollect(apply bool) (tea.Model, tea.Cmd) {
	m = m.cancelShown()
	m.gen++
	m = m.noteCollecting()
	m.filesLens = nil
	m.collecting = true
	return m, m.collectTests(m.gen, apply)
}

// show switches the displayed lens and re-syncs its layout. It cancels the lens
// being hidden so its difftastic stream doesn't linger, and the gen bump drops
// that lens's in-flight preview messages.
func (m Model) show(tests bool) (tea.Model, tea.Cmd) {
	m = m.cancelShown()
	if !tests && m.collecting {
		// The gen bump below drops the pending collect, and its fingerprint has
		// already been consumed — the retained tests lens predates the tree that
		// collect targeted, so drop it too; the next `t` re-collects instead of
		// showing stale specs indefinitely.
		m.testsLens = nil
		m.collecting = false
	}
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

func (m Model) collectTests(gen int, apply bool) tea.Cmd {
	repo, scope := m.repo, m.scope
	return func() tea.Msg {
		specs, _ := review.Collect(repo, scope)
		return lensCollectedMsg{gen: gen, apply: apply, specs: specs}
	}
}

// scopeFiles lists the changed files for a working-tree scope (staged side,
// base ref, pathspec), sorted and filtered like the diff command's own listing
// — so the file lens keeps the `-- path` scope that the tests lens (via
// review.Collect) already does.
func scopeFiles(repo *git.Repo, scope review.DiffScope) ([]git.ChangedFile, error) {
	return repo.ListChanged(scope.Staged, scope.Base, "", scope.Paths...)
}

// lensCollectedMsg delivers the parsed tests for the first switch to the tests
// lens, tagged with the generation that requested it.
type lensCollectedMsg struct {
	gen   int
	apply bool // apply the specs to the shown lens in place (a watch refresh)
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
