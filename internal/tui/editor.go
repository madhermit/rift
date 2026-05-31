package tui

import (
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// EditorClosedMsg is delivered after the external editor opened by OpenInEditor
// exits, so the screen can refresh (the file may have changed).
type EditorClosedMsg struct{ Err error }

// OpenInEditor suspends the TUI, opens path in $EDITOR (falling back to $VISUAL,
// then vi) with the working directory set to dir, and resumes on exit.
func OpenInEditor(dir, path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, path)
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return EditorClosedMsg{Err: err} })
}
