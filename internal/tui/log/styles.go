package logui

import "github.com/charmbracelet/lipgloss"

var (
	subtle = lipgloss.Color("241")
	accent = lipgloss.Color("39")
	white  = lipgloss.Color("15")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			PaddingLeft(1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(1)

	hashStyle = lipgloss.NewStyle().
			Foreground(accent)

	commitItemStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(2)

	selectedCommitStyle = lipgloss.NewStyle().
				Foreground(white).
				PaddingLeft(2)

	filterPromptStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(subtle)

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(accent)

	// Commit header styles
	headerLabelStyle = lipgloss.NewStyle().Foreground(subtle)

	// File status colors for header
	statusAddedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	statusDeletedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusModifiedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	statusRenamedStyle  = lipgloss.NewStyle().Foreground(accent)
)
