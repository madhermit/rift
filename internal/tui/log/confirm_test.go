package logui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/git"
)

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestCherryPickConfirmation covers the inline y/n gate on the destructive log
// actions: c arms it (no immediate quit), y confirms and quits with the action,
// and any other key cancels without acting.
func TestCherryPickConfirmation(t *testing.T) {
	commits := []git.CommitInfo{{Hash: "a1b2c3d", Message: "Fix a bug"}}
	base := New(nil, stubEngine{}, commits, false)
	var m tea.Model = base
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// c arms the confirmation and must not quit or act yet.
	armed, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	am := armed.(Model)
	if !am.confirm.Active() {
		t.Fatal("c should arm a confirmation")
	}
	if isQuit(cmd) {
		t.Error("c should not quit immediately")
	}
	if am.Action() != CherryPick {
		t.Error("c should arm the cherry-pick action")
	}
	if v := ansi.Strip(am.View().Content); !strings.Contains(v, "cherry-pick a1b2c3d? (y/n)") {
		t.Errorf("footer should show the confirmation prompt, got:\n%s", v)
	}

	// y confirms: quit with the action still set.
	confirmed, cmd := armed.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cm := confirmed.(Model); cm.Action() != CherryPick || cm.confirm.Active() {
		t.Errorf("y should confirm cherry-pick and clear the gate (action=%v active=%v)", cm.Action(), cm.confirm.Active())
	}
	if !isQuit(cmd) {
		t.Error("y should quit")
	}

	// Any other key cancels: no action, gate cleared, no quit.
	cancelled, cmd := armed.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if xm := cancelled.(Model); xm.Action() != NoAction || xm.confirm.Active() {
		t.Errorf("n should cancel (action=%v active=%v)", xm.Action(), xm.confirm.Active())
	}
	if isQuit(cmd) {
		t.Error("n should not quit")
	}
}
