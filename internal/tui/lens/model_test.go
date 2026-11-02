package lensui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
	gitpkg "github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
	diffui "github.com/madhermit/rift/internal/tui/diff"
)

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func settle(t *testing.T, m tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			t.Fatal("model did not settle")
		}
		next, cmd := m.Update(queue[0])
		m, queue = next, queue[1:]
		queue = append(queue, run(cmd)...)
	}
	return m
}

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil, spinner.TickMsg:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, run(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// newLens builds a lens over a one-commit repo's root commit.
func newLens(t *testing.T) tea.Model {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@b", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@b")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "init")
	t.Chdir(dir)
	repo, err := gitpkg.OpenRepo()
	if err != nil {
		t.Fatal(err)
	}
	base, files, err := repo.CommitFiles("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return New(repo, diff.NewPlainEngine(), files, review.DiffScope{Base: base, Target: "HEAD"}, false, false, false)
}

func screen(m tea.Model) string { return ansi.Strip(m.(Model).View().Content) }

// showTests stands in for the first toggle's collect, which would need a repo.
func showTests(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	lm := m.(Model)
	next, cmd := lm.Update(lensCollectedMsg{owner: lm.owner, gen: lm.gen, specs: []review.Spec{{File: "a_test.go", Name: "TestA", Status: "added", Line: 1}}})
	for _, msg := range run(cmd) {
		next = settle(t, next, msg)
	}
	return next
}

// TestSettingsCrossToggle: the layout set on one side of `t` holds on the other,
// whichever side it was set on, and the retained lens is brought in line on return.
func TestSettingsCrossToggle(t *testing.T) {
	m := settle(t, newLens(t), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = settle(t, m, key('\\'))
	want := m.(Model).Settings().Display

	m = showTests(t, m)
	if !m.(Model).showTests {
		t.Fatal("expected the tests lens")
	}
	if got := m.(Model).Settings().Display; got != want {
		t.Errorf("tests lens display = %v, want %v", got, want)
	}

	m = settle(t, m, key('\\'))
	want = m.(Model).Settings().Display
	m = settle(t, m, key('t'))
	if m.(Model).showTests {
		t.Fatal("t should return to the files")
	}
	if got := m.(Model).Settings().Display; got != want {
		t.Errorf("file lens display = %v, want %v", got, want)
	}
}

// TestPreviewResumesAfterToggle: a preview hidden before its load answered must
// restart on return, not sit on a spinner forever.
func TestPreviewResumesAfterToggle(t *testing.T) {
	next, cmd := newLens(t).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd == nil {
		t.Fatal("expected a preview request")
	}
	// The request is left unanswered: the toggle's generation bump orphans it.
	m := showTests(t, next)
	m = settle(t, m, key('t'))
	if s := screen(m); !strings.Contains(s, "one") {
		t.Fatalf("the file lens should have resumed its preview:\n%s", s)
	}
}

// TestHunksReachHiddenFileLens: the hunk read only ever starts once, so an answer
// landing while the tests lens shows must still reach the file lens.
func TestHunksReachHiddenFileLens(t *testing.T) {
	var m tea.Model
	next, cmd := newLens(t).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next
	// Answer the selection, holding back the hunk read's result.
	var held []tea.Msg
	queue := run(cmd)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		if _, ok := msg.(diffui.HunksLoadedMsg); ok {
			held = append(held, msg)
			continue
		}
		next, cmd := m.Update(msg)
		m = next
		queue = append(queue, run(cmd)...)
	}
	if len(held) != 1 {
		t.Fatalf("expected one held hunk read, got %d", len(held))
	}

	m = showTests(t, m)
	m = settle(t, m, held[0]) // lands while the tests lens shows
	m = settle(t, m, key('t'))
	m = settle(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = settle(t, m, key('}'))
	if s := screen(m); !strings.Contains(s, "hunk 1/1") {
		t.Fatalf("the file lens lost its hunks:\n%s", s)
	}
}

// TestDiscard: a message from a closed lens is recognized and its stream stopped.
func TestDiscard(t *testing.T) {
	cancelled := false
	msg := lensMsg{msg: tui.StreamReadyMsg{Cancel: func() { cancelled = true }}}
	if !Discard(msg) || !cancelled {
		t.Errorf("Discard = false or stream left running (cancelled=%v)", cancelled)
	}
	if Discard(tea.WindowSizeMsg{}) {
		t.Error("Discard should leave other messages alone")
	}
}
