package lensui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	diffui "github.com/madhermit/rift/internal/tui/diff"
	reviewui "github.com/madhermit/rift/internal/tui/review"
)

// Watch mode (`rift diff --watch`) keeps the lens current while something else
// edits the working tree — typically an agent writing code in another pane. It
// polls rather than watching the filesystem: the poll is three fast git
// subprocesses (status, numstat, hash-object), and polling can't hit inotify
// watch limits or miss events on network filesystems. Each poll fingerprints
// the scope's changed files; a changed fingerprint refreshes the shown lens in
// place, so the selection (and the reviewer's place) survives.

const watchInterval = time.Second

// watchTickMsg schedules the next poll; watchStateMsg carries a poll's result.
// Both carry the watch generation they were started under: the `w` toggle bumps
// it, so a tick or poll left in flight by a disable (or a rapid disable/enable)
// is dropped instead of running a second, parallel polling chain.
type watchTickMsg struct{ gen int }

type watchStateMsg struct {
	gen         int
	fingerprint string
	files       []git.ChangedFile
}

func watchTick(gen int) tea.Cmd {
	return tea.Tick(watchInterval, func(time.Time) tea.Msg { return watchTickMsg{gen: gen} })
}

// pollWatch re-lists the scope's changed files off the event loop and
// fingerprints them, so Update can tell whether anything the lens shows has
// changed.
func (m Model) pollWatch() tea.Cmd {
	repo, scope, gen := m.repo, m.scope, m.watchGen
	return func() tea.Msg {
		files := scopeFiles(repo, scope)
		return watchStateMsg{gen: gen, fingerprint: watchFingerprint(repo, files), files: files}
	}
}

// watchFingerprint reduces a changed-file set to a comparable identity: the
// listing itself plus each file's content hash, so an edit that leaves the
// stat line unchanged (`x := 1` → `x := 2`) still registers.
func watchFingerprint(repo *git.Repo, files []git.ChangedFile) string {
	return fingerprint(files, review.ContentHashes(repo, files))
}

func fingerprint(files []git.ChangedFile, hashes map[string]string) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d\x00%s\x00", f.Path, f.Status, f.Added, f.Deleted, hashes[f.Path])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// watchUpdate handles the watch loop's messages: a tick polls, and a poll
// result either re-arms the tick (nothing changed) or refreshes the shown
// lens before re-arming.
func (m Model) watchUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case watchTickMsg:
		if !m.watch || msg.gen != m.watchGen {
			return m, nil // watch toggled off, or a superseded chain
		}
		return m, m.pollWatch()
	case watchStateMsg:
		if !m.watch || msg.gen != m.watchGen {
			return m, nil
		}
		if msg.fingerprint == m.watchFP {
			return m, watchTick(m.watchGen)
		}
		m.watchFP = msg.fingerprint
		model, cmd := m.refreshChanged(msg.files)
		return model, tea.Batch(cmd, watchTick(m.watchGen))
	}
	return m, nil
}

// toggleWatch flips watch mode live (`w`). Enabling treats the moment as a
// change: the tree may have drifted while watch was off, so seed the
// fingerprint from fresh state and refresh the shown lens against it before
// starting the polling chain.
func (m Model) toggleWatch() (tea.Model, tea.Cmd) {
	m.watch = !m.watch
	m.watchGen++
	m = m.setWatching(m.watch)
	if !m.watch {
		return m, nil
	}
	files := scopeFiles(m.repo, m.scope)
	m.watchFP = watchFingerprint(m.repo, files)
	model, cmd := m.refreshChanged(files)
	return model, tea.Batch(cmd, watchTick(m.watchGen))
}

// setWatching re-tags both built lenses' header context with the current watch
// state; buildFiles/newTests tag lenses built later.
func (m Model) setWatching(on bool) Model {
	if d, ok := m.filesLens.(diffui.Model); ok {
		m.filesLens = d.SetWatching(on)
	}
	if r, ok := m.testsLens.(reviewui.Model); ok {
		m.testsLens = r.SetWatching(on)
	}
	return m
}

// refreshChanged applies an externally observed working-tree change. The shown
// lens refreshes in place — the file lens keeps its selection, the tests lens
// re-collects like a staged toggle — and the hidden lens is dropped so its
// next toggle rebuilds against the new state.
func (m Model) refreshChanged(files []git.ChangedFile) (tea.Model, tea.Cmd) {
	m.files = files
	if m.showTests {
		m = m.cancelShown()
		m.gen++
		m = m.noteCollecting()
		m.filesLens = nil
		return m, m.collectTests(m.gen)
	}
	m.testsLens = nil
	// Invalidate any in-flight tests collect (a `t` press racing this refresh):
	// it parsed the pre-change tree, so drop it rather than show stale tests.
	// The delegate below tags the file lens's fresh preview with the new gen.
	m.gen++
	return m.delegate(diffui.FilesChangedMsg{Files: files})
}
