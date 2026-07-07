package stageui

import (
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

func newTestModel() Model {
	m := Model{hunkCache: map[string][]displayHunk{}, diffReqID: 5}
	m.viewport = viewport.New()
	m.viewport.SetWidth(40)
	m.viewport.SetHeight(10)
	m.filter = tui.NewFilterInput()
	return m
}

// TestStaleHunkDiffsDropped guards against a slow difftastic render for a
// previously-selected file landing over the current file's hunks (which would let
// `s` stage the wrong hunk). Only the result carrying the current request token is
// applied; an older token is dropped.
func TestStaleHunkDiffsDropped(t *testing.T) {
	fresh := []displayHunk{{rendered: "HELLO", hunk: diff.Hunk{NewStart: 1}}}

	var m tea.Model = newTestModel()
	m, _ = m.Update(hunkDiffsMsg{reqID: 5, path: "a.go", width: 40, hunks: fresh})
	sm := m.(Model)
	if len(sm.displayHunks) != 1 || sm.displayHunks[0].rendered != "HELLO" {
		t.Fatalf("current-token result should be applied, got %d hunks", len(sm.displayHunks))
	}
	if _, ok := sm.hunkCache[hunkCacheKey("a.go", 40)]; !ok {
		t.Error("result should be cached by path+width")
	}

	// A late result from an older token must be ignored, keeping the current hunks.
	stale := []displayHunk{{rendered: "STALE", hunk: diff.Hunk{NewStart: 1}}}
	m, _ = m.Update(hunkDiffsMsg{reqID: 3, path: "b.go", width: 40, hunks: stale})
	if sm = m.(Model); len(sm.displayHunks) != 1 || sm.displayHunks[0].rendered != "HELLO" {
		t.Error("stale-token result should be dropped, keeping the current hunks")
	}
}

// TestEscLeavesModeNeverQuits verifies stage's esc matches the SplitList screens:
// it exits filter mode, closes the help overlay, and no-ops at the root — never
// quitting (that's q / ctrl+c).
func TestEscLeavesModeNeverQuits(t *testing.T) {
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}

	// At the root, esc does nothing and, crucially, does not quit (nil command).
	root := newTestModel()
	root.ready = true
	if _, cmd := root.Update(esc); cmd != nil {
		t.Error("esc at the root should be a no-op, not quit")
	}

	// esc exits filter mode.
	filtering := newTestModel()
	filtering.filtering = true
	filtering.filter.Focus()
	if m, _ := filtering.Update(esc); m.(Model).filtering {
		t.Error("esc should exit filter mode")
	}

	// esc closes the help overlay.
	helping := newTestModel()
	helping.showHelp = true
	if m, _ := helping.Update(esc); m.(Model).showHelp {
		t.Error("esc should close the help overlay")
	}
}

// TestFilterKeepsSelection verifies stage keeps the selection on the same file
// when it survives the new filter, and resets to the top otherwise.
func TestFilterKeepsSelection(t *testing.T) {
	files := []git.StatusFile{{Path: "alpha.go"}, {Path: "alpaca.go"}, {Path: "beta.go"}}

	kept := newTestModel()
	kept.files = files
	kept.filteredFiles = files
	kept.selectedIdx = 1 // alpaca.go
	kept.filter.SetValue("alp")
	kept.applyFilter()
	if got := kept.filteredFiles[kept.selectedIdx].Path; got != "alpaca.go" {
		t.Errorf("filter should keep selection on alpaca.go, got %q", got)
	}

	reset := newTestModel()
	reset.files = files
	reset.filteredFiles = files
	reset.selectedIdx = 1 // alpaca.go
	reset.filter.SetValue("beta")
	reset.applyFilter()
	if got := reset.filteredFiles[reset.selectedIdx].Path; got != "beta.go" {
		t.Errorf("when the selection doesn't survive, reset to the first survivor; got %q", got)
	}
}

// TestHunkCacheHitAdvancesToken verifies a cache hit still bumps the request
// token, so a pending load for the file we just left can't land over the cached
// selection.
func TestHunkCacheHitAdvancesToken(t *testing.T) {
	m := newTestModel()
	m.hunkCache[hunkCacheKey("a.go", 40)] = []displayHunk{{rendered: "CACHED", hunk: diff.Hunk{NewStart: 1}}}
	m.filteredFiles = []git.StatusFile{{Path: "a.go"}}
	before := m.diffReqID
	cmd := m.requestDiff()
	if cmd != nil {
		t.Error("a cache hit should not dispatch a load")
	}
	if m.diffReqID == before {
		t.Error("a cache hit should still advance the request token")
	}
	if len(m.displayHunks) != 1 || m.displayHunks[0].rendered != "CACHED" {
		t.Error("a cache hit should show the cached hunks")
	}
}

// TestFilePaneVimNav verifies the file list supports the same vim jump keys as
// the SplitList screens' list panes: gg/G to the ends and ctrl+d to step by half
// the window.
func TestFilePaneVimNav(t *testing.T) {
	key := func(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(s[0]), Text: s} }
	files := []git.StatusFile{{Path: "a"}, {Path: "b"}, {Path: "c"}, {Path: "d"}, {Path: "e"}}

	m := newTestModel()
	m.ready = true
	m.width, m.height = 80, 24
	m.files, m.filteredFiles = files, files
	// Pre-cache each file's (empty) hunks so navigation is a cache hit and doesn't
	// dispatch a difftastic load against the nil test repo.
	for _, f := range files {
		m.hunkCache[hunkCacheKey(f.Path, m.viewport.Width())] = nil
	}

	var tm tea.Model = m
	tm, _ = tm.Update(key("G"))
	if got := tm.(Model).selectedIdx; got != 4 {
		t.Errorf("G should select the last file, got %d", got)
	}
	tm, _ = tm.Update(key("g"))
	tm, _ = tm.Update(key("g"))
	if got := tm.(Model).selectedIdx; got != 0 {
		t.Errorf("gg should select the first file, got %d", got)
	}
	tm, _ = tm.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if got := tm.(Model).selectedIdx; got == 0 {
		t.Error("ctrl+d should step the file selection down by half a window")
	}
}

// TestHunkNavCanonicalKey verifies {/} steps between hunks in the diff pane and
// that the former n/p aliases no longer navigate (they were dropped so `p` means
// only stash's pop across screens).
func TestHunkNavCanonicalKey(t *testing.T) {
	key := func(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(s[0]), Text: s} }
	hunks := []displayHunk{
		{rendered: "h0", hunk: diff.Hunk{NewStart: 1}},
		{rendered: "h1", hunk: diff.Hunk{NewStart: 10}},
		{rendered: "h2", hunk: diff.Hunk{NewStart: 20}},
	}

	m := newTestModel()
	m.ready = true
	m.activePane = diffPane
	m.displayHunks = hunks
	m.renderHunks()

	var tm tea.Model = m
	tm, _ = tm.Update(key("}"))
	if got := tm.(Model).hunkIdx; got != 1 {
		t.Errorf("} should advance to the next hunk, got %d", got)
	}
	tm, _ = tm.Update(key("{"))
	if got := tm.(Model).hunkIdx; got != 0 {
		t.Errorf("{ should step to the previous hunk, got %d", got)
	}
	tm, _ = tm.Update(key("n"))
	if got := tm.(Model).hunkIdx; got != 0 {
		t.Errorf("n should no longer navigate hunks, got %d", got)
	}
}
