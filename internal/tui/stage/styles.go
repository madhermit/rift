package stageui

import (
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/tui"
)

var (
	// Per-file staged/unstaged status chars.
	stagedStyle   = lipgloss.NewStyle().Foreground(tui.Green)
	unstagedStyle = lipgloss.NewStyle().Foreground(tui.Red)

	// Hunk separators and gutter markers in the diff pane.
	hunkSepStyle    = lipgloss.NewStyle().Foreground(tui.Green).Bold(true)
	hunkSepDimStyle = lipgloss.NewStyle().Foreground(tui.Subtle)

	// The active hunk gets a heavy colored bar (┃) in the gutter — distinct from
	// the list selection marker (▌) — while inactive hunks leave the gutter blank.
	sidebarUnstaged = lipgloss.NewStyle().Foreground(tui.Magenta).Render("┃") + " "
	sidebarStaged   = lipgloss.NewStyle().Foreground(tui.Green).Render("┃") + " "
	sidebarInactive = "  "
)
