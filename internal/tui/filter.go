package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

var filterPromptStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("39")).
	Bold(true)

// NewFilterInput returns a textinput pre-configured for "/ " fuzzy filtering.
func NewFilterInput() textinput.Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.CharLimit = 256
	styles := filter.Styles()
	styles.Focused.Prompt = filterPromptStyle
	filter.SetStyles(styles)
	return filter
}
