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

	sidebarUnstaged = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("▎") + " "
	sidebarStaged   = lipgloss.NewStyle().Foreground(tui.Green).Render("▎") + " "
	sidebarInactive = "  "
)
