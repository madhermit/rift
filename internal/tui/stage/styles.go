package stageui

import "github.com/charmbracelet/lipgloss"

var (
	subtle = lipgloss.Color("241")
	accent = lipgloss.Color("39")
	white  = lipgloss.Color("15")
	green  = lipgloss.Color("2")
	red    = lipgloss.Color("1")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			PaddingLeft(1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(1)

	fileItemStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(1)

	selectedFileStyle = lipgloss.NewStyle().
				Foreground(white).
				Background(lipgloss.Color("236")).
				PaddingLeft(1)

	filterPromptStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(subtle)

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(accent)

	stagedStyle = lipgloss.NewStyle().
			Foreground(green)

	unstagedStyle = lipgloss.NewStyle().
			Foreground(red)

	hunkSepStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	hunkSepDimStyle = lipgloss.NewStyle().
				Foreground(subtle)

	sidebarUnstaged = lipgloss.NewStyle().
				Foreground(lipgloss.Color("5")).
				Render("▎") + " "

	sidebarStaged = lipgloss.NewStyle().
			Foreground(green).
			Render("▎") + " "

	sidebarInactive = "  "
)
