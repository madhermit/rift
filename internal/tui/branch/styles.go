package branchui

import (
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/tui"
)

var (
	currentLineStyle = lipgloss.NewStyle().Foreground(tui.Green)
	currentFlagStyle = lipgloss.NewStyle().Foreground(tui.Green)
	dimStyle         = lipgloss.NewStyle().Foreground(tui.Subtle)
)
