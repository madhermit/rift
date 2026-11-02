package logui

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
	"github.com/madhermit/rift/internal/git"
)

// drillRepo builds a repo with one commit touching a test file.
func drillRepo(t *testing.T) (*git.Repo, []git.CommitInfo) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@b", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@b")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	body := "package a\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "a_test.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "Add a test")
	t.Chdir(dir)
	repo, err := git.OpenRepo()
	if err != nil {
		t.Fatal(err)
	}
	commits, err := repo.Log("HEAD", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	return repo, commits
}

// drive feeds msg to the model and runs every command it produces to quiescence,
// the way the runtime would, so async collects and previews land.
func drive(t *testing.T, m tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			t.Fatal("model did not settle")
		}
		next, cmd := m.Update(queue[0])
		m, queue = next, queue[1:]
		queue = append(queue, runCmd(cmd)...)
	}
	return m
}

func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, runCmd(c)...)
		}
		return out
	case tea.QuitMsg:
		return nil
	case spinner.TickMsg:
		return nil // a spinner tick would re-arm forever
	default:
		return []tea.Msg{msg}
	}
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func screen(m tea.Model) string { return ansi.Strip(m.(Model).View().Content) }

// TestDrillTogglesLens: `t` on a commit opens its tests, `t` flips to its files
// and back, and esc returns to the commit list — the same lens `rift diff` has.
func TestDrillTogglesLens(t *testing.T) {
	repo, commits := drillRepo(t)
	var m tea.Model = New(repo, stubEngine{}, commits, false)
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = drive(t, m, key("t"))
	if s := screen(m); !strings.Contains(s, "TestOne") || !strings.Contains(s, "tests") {
		t.Fatalf("t should open the commit's tests:\n%s", s)
	}
	m = drive(t, m, key("t"))
	if s := screen(m); !strings.Contains(s, "a_test.go") {
		t.Fatalf("t should flip to the commit's files:\n%s", s)
	}
	m = drive(t, m, key("esc"))
	if m.(Model).drill.Open() {
		t.Fatal("esc should close the drilldown")
	}
	if s := screen(m); !strings.Contains(s, "commits") {
		t.Fatalf("esc should return to the commit list:\n%s", s)
	}
}

// TestEnterRespectsTestsFlag: --tests swaps which side each key opens.
func TestEnterRespectsTestsFlag(t *testing.T) {
	repo, commits := drillRepo(t)
	var m tea.Model = New(repo, stubEngine{}, commits, true)
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if s := screen(m); !strings.Contains(s, "⏎ tests") || !strings.Contains(s, "t files") {
		t.Fatalf("hints should say what each key opens:\n%s", s)
	}
	m = drive(t, m, key("enter"))
	if s := screen(m); !strings.Contains(s, "TestOne") {
		t.Fatalf("enter under --tests should open the tests:\n%s", s)
	}
}

// TestDrillSettingsRoundTrip: an engine switched while drilled stays switched back
// on the list, and a drill opened on the alternate can still cycle back — the whole
// ring travels, so the `e` hint and key don't vanish.
func TestDrillSettingsRoundTrip(t *testing.T) {
	repo, commits := drillRepo(t)
	var m tea.Model = New(repo, stubEngine{}, commits, false)
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	primary := m.(Model).settings.Engines.Name()

	m = drive(t, m, key("enter"))
	m = drive(t, m, key("e"))
	m = drive(t, m, key("esc"))
	got := m.(Model).settings
	if got.Engines.Name() == primary {
		t.Fatal("the engine chosen while drilled should survive returning to the list")
	}

	m = drive(t, m, key("enter"))
	if s := screen(m); !strings.Contains(s, "e engine") {
		t.Fatalf("a drill opened on the alternate engine must still offer e:\n%s", s)
	}
	m = drive(t, m, key("e"))
	if s := m.(Model).drill.Settings().Engines.Name(); s != primary {
		t.Fatalf("e should cycle back to %s, got %s", primary, s)
	}
}

// TestDrillHunksResolve guards the routing that used to lose the hunk index in a
// drilldown: the read's answer must reach the embedded view, or {/} silently falls
// back to stepping files while the footer promises hunks.
func TestDrillHunksResolve(t *testing.T) {
	repo, commits := drillRepo(t)
	var m tea.Model = New(repo, diff.NewPlainEngine(), commits, false)
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = drive(t, m, key("enter"))
	m = drive(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = drive(t, m, key("}"))
	if s := screen(m); !strings.Contains(s, "hunk 1/1") {
		t.Fatalf("{/} in a drilldown should step real hunks:\n%s", s)
	}
}
