package logui

import (
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/tui"
)

var (
	// Commit-header styles (rendered inside the diff pane).
	hashStyle        = lipgloss.NewStyle().Foreground(tui.Accent)
	headerLabelStyle = lipgloss.NewStyle().Foreground(tui.Subtle)
)
