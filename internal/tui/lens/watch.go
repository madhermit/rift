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
// polls rather than watching the filesystem: the poll is a handful of fast git
// subprocesses, and polling can't hit inotify watch limits or miss events on
// network filesystems. Each poll fingerprints the scope's changed files; a
// changed fingerprint refreshes the shown lens in place, invalidating only the
// previews whose content actually changed — so the selection and the reader's
// scroll position survive.

const watchInterval = time.Second

// watchTickMsg schedules the next poll; watchStateMsg carries a poll's result.
// Both carry the watch generation they were started under: the `w` toggle and
// the `s` scope flip bump it, so a tick or poll left in flight is dropped
// instead of applying a stale scope or running a second, parallel polling
// chain.
type watchTickMsg struct{ gen int }

type watchStateMsg struct {
	gen         int
	failed      bool // the listing errored; skip this poll rather than treat it as empty
	fingerprint string
	files       []git.ChangedFile
	hashes      map[string]string
}

func watchTick(gen int) tea.Cmd {
	return tea.Tick(watchInterval, func(time.Time) tea.Msg { return watchTickMsg{gen: gen} })
}

// pollWatch re-lists the scope's changed files off the event loop and
// fingerprints them, so Update can tell whether anything the lens shows has
// changed. A listing error is reported as failed — never as an empty set,
// which would wipe the lens.
func (m Model) pollWatch() tea.Cmd {
	repo, scope, gen := m.repo, m.scope, m.watchGen
	return func() tea.Msg {
		files, err := scopeFiles(repo, scope)
		if err != nil {
			return watchStateMsg{gen: gen, failed: true}
		}
		hashes := watchHashes(repo, scope, files)
		return watchStateMsg{gen: gen, fingerprint: fingerprint(files, hashes), files: files, hashes: hashes}
	}
}

// watchHashes is each file's content identity for the fingerprint: index blob
// hashes for the staged scope (a re-stage must register even when the worktree
// and the stat line don't change), worktree content hashes otherwise.
func watchHashes(repo *git.Repo, scope review.DiffScope, files []git.ChangedFile) map[string]string {
	if scope.Staged {
		return repo.StagedBlobHashes(git.Paths(files))
	}
	return review.ContentHashes(repo, files)
}

// fingerprint reduces a changed-file set to a comparable identity: the listing
// itself plus each file's content hash, so an edit that leaves the stat line
// unchanged (`x := 1` → `x := 2`) still registers.
func fingerprint(files []git.ChangedFile, hashes map[string]string) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\x00%d\x00%s\x00", f.Path, f.OldPath, f.Status, f.Added, f.Deleted, hashes[f.Path])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// watchUpdate handles the watch loop's messages: a tick polls, and a poll
// result either re-arms the tick (nothing changed, or the listing failed) or
// refreshes the shown lens before re-arming.
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
		if msg.failed || msg.fingerprint == m.watchFP {
			return m, watchTick(m.watchGen)
		}
		m.watchFP = msg.fingerprint
		model, cmd := m.refreshChanged(msg.files, msg.hashes)
		return model, tea.Batch(cmd, watchTick(m.watchGen))
	}
	return m, nil
}

// toggleWatch flips watch mode live (`w`). Enabling polls immediately, off the
// event loop: watchFP survives from before the last disable, so re-enabling
// over an unchanged tree refreshes nothing, while any drift that accumulated
// while watch was off is caught by that first poll.
func (m Model) toggleWatch() (tea.Model, tea.Cmd) {
	m.watch = !m.watch
	m.watchGen++ // orphan any tick/poll left in flight by the previous chain
	m = m.setWatching(m.watch)
	if !m.watch {
		return m, nil
	}
	return m, m.pollWatch()
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
// lens refreshes in place — the file lens keeps its selection and the cached
// previews of untouched files, the tests lens re-collects and applies the
// specs in place — and the hidden lens is dropped so its next toggle rebuilds
// against the new state.
func (m Model) refreshChanged(files []git.ChangedFile, hashes map[string]string) (tea.Model, tea.Cmd) {
	m.files = files
	if m.showTests {
		return m.recollect(true)
	}
	m.testsLens = nil
	// Invalidate in-flight messages for the pre-change tree. A `t` press racing
	// this refresh had its collect dropped by the bump, so relaunch it against
	// the new tree — the switch to tests then happens when the fresh specs land.
	m.gen++
	var relaunch tea.Cmd
	if m.collecting {
		relaunch = m.collectTests(m.gen, false)
	}
	if m.scope.Staged {
		hashes = nil // poll hashes are index identities; reviewed marks need worktree hashes
	}
	model, cmd := m.delegate(diffui.FilesChangedMsg{Files: files, Hashes: hashes})
	return model, tea.Batch(cmd, relaunch)
}
