package menu

import "charm.land/lipgloss/v2"

var (
	subtle = lipgloss.Color("241")
	accent = lipgloss.Color("39")
	white  = lipgloss.Color("15")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			PaddingLeft(1).
			PaddingBottom(1)

	itemStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(white).
				PaddingLeft(4)

	descriptionStyle = lipgloss.NewStyle().
				Foreground(subtle).
				PaddingLeft(6)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(subtle).
			PaddingLeft(1)
)
