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
			PaddingLeft(1).
			PaddingBottom(1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(1)

	filterPromptStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	normalLineStyle = lipgloss.NewStyle().
			Foreground(subtle)

	selectedLineStyle = lipgloss.NewStyle().
				Foreground(white)

	currentLineStyle = lipgloss.NewStyle().
				Foreground(green)

	subtleStyle = lipgloss.NewStyle().
			Foreground(subtle)
)
