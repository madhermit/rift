package branchui

import "github.com/charmbracelet/lipgloss"

var (
	subtle = lipgloss.Color("241")
	accent = lipgloss.Color("39")
	white  = lipgloss.Color("15")
	green  = lipgloss.Color("35")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			PaddingLeft(1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(1)

	filterPromptStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	branchItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedBranchStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(white).
				Background(accent)

	currentBranchStyle = lipgloss.NewStyle().
				Foreground(green).
				Bold(true)

	subtleStyle = lipgloss.NewStyle().
			Foreground(subtle)
)
