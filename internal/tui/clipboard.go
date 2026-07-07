package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// YankToClipboard copies val to the clipboard, returning the footer flash to
// show and any command to run. It tries the OS clipboard first (atotto) and,
// when that has no backend — a headless or SSH session — falls back to an OSC 52
// escape emitted through the terminal (tea.SetClipboard). OSC 52 can't report
// whether the terminal accepted it, so the fallback still reports "copied".
func YankToClipboard(val string) (flash string, cmd tea.Cmd) {
	flash = "copied " + val
	if clipboard.WriteAll(val) == nil {
		return flash, nil
	}
	return flash, tea.SetClipboard(val)
}
