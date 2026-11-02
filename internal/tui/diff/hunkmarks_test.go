package diffui

import (
	"os"
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
)

func tempRepo(t *testing.T) *git.Repo {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t.co"}, {"config", "user.name", "t"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	repo, err := git.OpenRepo()
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	return repo
}

// markModel is a working-tree diff model over paths, with the hunk index faked:
// spots stand in for a rendered preview's hunks (keyed "<path>#<n>"), which is
// what the marking logic actually addresses. Their display lines are packed one
// per row so they all fit the (empty) preview at once — an empty viewport can't
// scroll, and a spot the reader can't be scrolled to is one the cursor re-syncs
// away from, which would test the viewport rather than the marking.
func markModel(t *testing.T, hunksPer int, paths ...string) Model {
	t.Helper()
	repo := tempRepo(t)
	var files []git.ChangedFile
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("content of "+p+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, git.ChangedFile{Path: p, Status: "Modified", Added: 1})
	}
	m := New(repo, stubEngine{}, files, false, "", "", false, nil, false)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = mm.(Model)

	m.hunk.loaded = true
	m.marks.keys = map[string][]string{}
	line := 0
	for _, p := range paths {
		for i := range hunksPer {
			key := p + "#" + string(rune('a'+i))
			m.marks.keys[p] = append(m.marks.keys[p], key)
			m.hunk.spots = append(m.hunk.spots, hunkSpot{line: line, path: p, key: key})
			line++
		}
	}
	return m
}

func markHunk(t *testing.T, m Model, at int) Model {
	t.Helper()
	m.hunk.at = at
	mm, _ := m.toggleHunkReviewed()
	return mm.(Model)
}

// Marking a file's last open hunk marks the file: a hunk-by-hunk pass has to
// tick files off the list, or it never finishes one.
func TestHunkMarkCompletesFile(t *testing.T) {
	m := markModel(t, 2, "a.go")
	m.list, _ = m.list.SelectKey("a.go")

	m = markHunk(t, m, 1)
	if m.marks.reviewed("a.go") {
		t.Error("one of two hunks marked should not mark the file")
	}
	if !m.marks.partlyReviewed("a.go") {
		t.Error("one marked hunk should read as partly reviewed")
	}

	m = markHunk(t, m, 2)
	if !m.marks.reviewed("a.go") {
		t.Error("marking the last hunk should mark the file")
	}
}

// And reopening any hunk reopens its file, so a ✓ never sits over code the
// reviewer just put back on the pile.
func TestHunkUnmarkReopensFile(t *testing.T) {
	m := markModel(t, 2, "a.go")
	m.list, _ = m.list.SelectKey("a.go")
	m = markHunk(t, m, 1)
	m = markHunk(t, m, 2)
	if !m.marks.reviewed("a.go") {
		t.Fatal("setup: the file should be reviewed")
	}

	m = markHunk(t, m, 2) // toggle the same hunk back off
	if m.marks.reviewed("a.go") {
		t.Error("reopening a hunk should reopen its file")
	}
	if !m.marks.partlyReviewed("a.go") {
		t.Error("the other hunk's mark should survive")
	}
	if !m.marks.hunks.IsReviewed("a.go", "a.go#a") {
		t.Error("hunk 1 should still be marked")
	}
}

// Marking advances to the next hunk that still needs eyes, skipping ones already
// done, so a pass is R-R-R.
func TestHunkMarkAdvances(t *testing.T) {
	m := markModel(t, 4, "a.go") // the All row stays selected: one continuous preview

	m = markHunk(t, m, 1)
	if m.hunk.at != 2 {
		t.Errorf("cursor should advance to hunk 2, got %d", m.hunk.at)
	}
	// Mark 3 out of order, then finish 2: the cursor should skip over 3 to 4.
	m = markHunk(t, m, 3)
	m = markHunk(t, m, 2)
	if m.hunk.at != 4 {
		t.Errorf("cursor should skip the already-marked hunk 3 and land on 4, got %d", m.hunk.at)
	}
	// Nothing open below the last hunk: the cursor holds rather than wrapping.
	m = markHunk(t, m, 4)
	if m.hunk.at != 4 {
		t.Errorf("cursor should hold at the last hunk, got %d", m.hunk.at)
	}
}

// A hunk rift couldn't identify is navigable but not markable — marking it would
// key the store on nothing.
func TestHunkMarkNeedsKey(t *testing.T) {
	m := markModel(t, 1, "a.go")
	m.hunk.spots[0].key = ""

	m = markHunk(t, m, 1)
	if m.marks.partlyReviewed("a.go") || m.marks.reviewed("a.go") {
		t.Error("an unidentifiable hunk should mark nothing")
	}
}

// `r` is a claim about the file, not about each hunk in it: it must not
// pre-mark the hunks, or stepping in afterwards would find nothing left to read.
func TestFileMarkLeavesHunksAlone(t *testing.T) {
	m := markModel(t, 2, "a.go", "b.go")
	m.list, _ = m.list.SelectKey("a.go")

	mm, _ := m.toggleReviewed()
	m = mm.(Model)
	if !m.marks.reviewed("a.go") {
		t.Fatal("r should mark the file")
	}
	if m.marks.hunks.IsReviewed("a.go", "a.go#a") {
		t.Error("marking a file should not mark its hunks")
	}
}

// The cursor readout ticks the hunk the reader is on once it's marked.
func TestHunkCursorReadout(t *testing.T) {
	m := markModel(t, 3, "a.go")
	m.hunk.at = 2
	if got := m.hunkStatus(); got != "hunk 2/3" {
		t.Errorf("readout = %q, want %q", got, "hunk 2/3")
	}

	m.marks.hunks.Toggle("a.go", "a.go#b")
	if got := m.hunkStatus(); got != "hunk 2/3 ✓" {
		t.Errorf("readout = %q, want %q", got, "hunk 2/3 ✓")
	}
}

// The unit tests above poke the model's state directly. This one drives the real
// key path — focus the preview, resolve hunks against rendered content, press the
// key — so a binding that never reaches the handler shows up as a failure.
func TestHunkMarkThroughKeys(t *testing.T) {
	repo := tempRepo(t)
	if err := os.WriteFile("a.go", []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []git.ChangedFile{{Path: "a.go", Status: "Modified", Added: 2, Deleted: 2}}
	hunks := []diff.Hunk{
		{Header: "@@ -1,3 +1,3 @@", NewStart: 1, Lines: []string{"-old one", "+new one", " ctx"}},
		{Header: "@@ -10,3 +10,3 @@", NewStart: 10, Lines: []string{"-old two", "+new two", " ctx"}},
	}
	preview := "diff --git a/a.go b/a.go\n" +
		"@@ -1,3 +1,3 @@\n-old one\n+new one\n ctx\n" +
		"@@ -10,3 +10,3 @@\n-old two\n+new two\n ctx\n"

	send := func(m Model, msg tea.Msg) Model {
		t.Helper()
		mm, _ := m.Update(msg)
		return mm.(Model)
	}
	press := func(m Model, r rune) Model {
		t.Helper()
		return send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	m := New(repo, stubEngine{}, files, false, "", "", false, nil, false)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 30})
	var selCmd tea.Cmd
	m.list, selCmd = m.list.SelectKey("a.go")
	reqID, ok := reqIDFrom(selCmd)
	if !ok {
		t.Fatal("selecting a file should ask the parent for its preview")
	}
	m = send(m, tea.KeyPressMsg{Code: tea.KeyTab}) // focus the preview
	m = send(m, tui.PreviewMsg{Content: preview, ReqID: reqID})
	m = send(m, HunksLoadedMsg{
		hunks: map[string][]diff.Hunk{"a.go": hunks},
		keys:  map[string][]string{"a.go": {review.HunkKey(hunks[0]), review.HunkKey(hunks[1])}},
	})

	if len(m.hunk.spots) != 2 {
		t.Fatalf("hunks should resolve against the rendered diff, got %d spots", len(m.hunk.spots))
	}
	// The preview opens on the file header, above the first hunk, so the first R
	// has nothing under the cursor: it starts the pass rather than doing nothing.
	if m.hunk.at != 0 {
		t.Fatalf("the cursor should start above the first hunk, got %d", m.hunk.at)
	}
	m = press(m, 'R')
	if m.hunk.at != 1 {
		t.Fatalf("R above the first hunk should land on it, got %d", m.hunk.at)
	}
	if m.marks.partlyReviewed("a.go") {
		t.Error("...and mark nothing on the way")
	}

	m = press(m, 'R')
	if !m.marks.hunks.IsReviewed("a.go", review.HunkKey(hunks[0])) {
		t.Error("R should mark the hunk under the cursor")
	}
	if m.marks.reviewed("a.go") {
		t.Error("one of two hunks marked should not mark the file")
	}

	m = press(m, 'R')
	if !m.marks.reviewed("a.go") {
		t.Error("marking the last hunk should mark the file")
	}

	// And `/` over the preview searches the diff rather than filtering the list —
	// the screen's own keys must not swallow the query as it is typed.
	m = press(m, '/')
	for _, r := range "two" {
		m = press(m, r)
	}
	if !m.Capturing() {
		t.Fatal("/ over the preview should open the search")
	}
	if got := m.list.VisibleItems(); len(got) != 2 {
		t.Errorf("searching must not filter the file list, got %d rows", len(got))
	}
}

// reqIDFrom runs cmd and digs out the ReqID of the SelectionChangedMsg it
// carries — the token a preview has to echo back to be accepted.
func reqIDFrom(cmd tea.Cmd) (int, bool) {
	if cmd == nil {
		return 0, false
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			if id, ok := reqIDFrom(c); ok {
				return id, true
			}
		}
	case tui.SelectionChangedMsg:
		return msg.ReqID, true
	}
	return 0, false
}

// Completing a file auto-advances to the next one, which swaps the preview out
// from under the hunk cursor. The cursor has to go with it: a cached preview
// paints in the same frame, so a stale readout would land on the wrong diff.
func TestHunkCursorClearedOnAdvance(t *testing.T) {
	m := markModel(t, 1, "a.go", "b.go")
	m.list, _ = m.list.SelectKey("a.go")

	m = markHunk(t, m, 1) // a.go's only hunk: completes the file and advances
	if !m.marks.reviewed("a.go") {
		t.Fatal("setup: a.go should be reviewed")
	}
	if m.list.SelectedKey() != "b.go" {
		t.Fatalf("should have advanced to b.go, on %q", m.list.SelectedKey())
	}
	if len(m.hunk.spots) != 0 || m.hunk.at != 0 {
		t.Errorf("the cursor should not survive the reselect (spots=%d at=%d)", len(m.hunk.spots), m.hunk.at)
	}
	if got := m.hunkStatus(); got != "" {
		t.Errorf("readout = %q, want empty until the new preview resolves", got)
	}
}
