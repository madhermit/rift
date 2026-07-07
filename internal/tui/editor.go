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

// OpenInEditor suspends the TUI, opens path in $VISUAL (falling back to $EDITOR,
// then vi) with the working directory set to dir, and resumes on exit. When line
// > 0 the editor is asked to open at that line.
func OpenInEditor(dir, path string, line int) tea.Cmd {
	// resolveEditor guarantees a non-blank value, so strings.Fields is non-empty.
	fields := strings.Fields(resolveEditor(os.Getenv("VISUAL"), os.Getenv("EDITOR")))
	// The editor may carry flags (e.g. "code --wait"); the first field is the binary.
	args := append(fields[1:len(fields):len(fields)], editorArgs(fields[0], path, line)...)
	c := exec.Command(fields[0], args...)
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return EditorClosedMsg{Err: err} })
}

// resolveEditor picks the editor command from VISUAL, then EDITOR (the git/POSIX
// convention for interactive editors), treating a blank or whitespace-only value
// as unset, and falls back to vi. Never returns a blank string, so callers can
// safely take the first whitespace-split field as the binary.
func resolveEditor(visual, editor string) string {
	for _, v := range []string{visual, editor} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "vi"
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
