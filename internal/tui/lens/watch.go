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
type watchTickMsg struct{}

type watchStateMsg struct {
	fingerprint string
	files       []git.ChangedFile
}

func watchTick() tea.Cmd {
	return tea.Tick(watchInterval, func(time.Time) tea.Msg { return watchTickMsg{} })
}

// pollWatch re-lists the scope's changed files off the event loop and
// fingerprints them, so Update can tell whether anything the lens shows has
// changed.
func (m Model) pollWatch() tea.Cmd {
	repo, scope := m.repo, m.scope
	return func() tea.Msg {
		files := scopeFiles(repo, scope)
		return watchStateMsg{fingerprint: watchFingerprint(repo, files), files: files}
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
		return m, m.pollWatch()
	case watchStateMsg:
		if msg.fingerprint == m.watchFP {
			return m, watchTick()
		}
		m.watchFP = msg.fingerprint
		model, cmd := m.refreshChanged(msg.files)
		return model, tea.Batch(cmd, watchTick())
	}
	return m, nil
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
