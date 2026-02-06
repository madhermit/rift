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
			PaddingLeft(2)

	selectedCommitStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(white).
				Background(accent).
				PaddingLeft(1).
				PaddingRight(1)

	filterPromptStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(subtle)

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(accent)
)
