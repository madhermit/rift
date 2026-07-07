package stageui

import (
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

func newTestModel() Model {
	m := Model{hunkCache: map[string][]displayHunk{}, diffReqID: 5}
	m.viewport = viewport.New()
	m.viewport.SetWidth(40)
	m.viewport.SetHeight(10)
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
