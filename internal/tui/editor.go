package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// EditorClosedMsg is delivered after the external editor opened by OpenInEditor
// exits, so the screen can refresh (the file may have changed).
type EditorClosedMsg struct{ Err error }

// OpenInEditor suspends the TUI, opens path in $EDITOR (falling back to $VISUAL,
// then vi) with the working directory set to dir, and resumes on exit. When line
// > 0 the editor is asked to open at that line.
func OpenInEditor(dir, path string, line int) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}
	// EDITOR may carry flags (e.g. "code --wait"); the first field is the binary.
	fields := strings.Fields(editor)
	args := append(fields[1:len(fields):len(fields)], editorArgs(fields[0], path, line)...)
	c := exec.Command(fields[0], args...)
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return EditorClosedMsg{Err: err} })
}

// editorArgs builds the editor argument list, adding a line-jump in the syntax
// the chosen editor understands. Unknown editors fall back to the widely
// supported "+N file" form when a line is given.
func editorArgs(editor, path string, line int) []string {
	if line <= 0 {
		return []string{path}
	}
	switch strings.ToLower(filepath.Base(editor)) {
	case "code", "code-insiders", "codium", "vscodium":
		return []string{"-g", fmt.Sprintf("%s:%d", path, line)}
	case "subl", "sublime_text", "hx", "helix":
		return []string{fmt.Sprintf("%s:%d", path, line)}
	case "idea", "goland", "pycharm", "webstorm", "phpstorm", "rubymine", "clion", "rider":
		return []string{"--line", strconv.Itoa(line), path}
	default:
		// vim, nvim, vi, view, nano, emacs, emacsclient, kak, micro, joe, gedit …
		return []string{"+" + strconv.Itoa(line), path}
	}
}
