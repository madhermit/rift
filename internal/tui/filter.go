package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
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

// FuzzyFilter returns the subset of items whose formatted string fuzzy-matches
// query, ordered by match quality. An empty query returns items unchanged.
func FuzzyFilter[T any](items []T, query string, format func(T) string) []T {
	if query == "" {
		return items
	}
	targets := make([]string, len(items))
	for i, it := range items {
		targets[i] = format(it)
	}
	matches := fuzzy.Find(query, targets)
	filtered := make([]T, len(matches))
	for i, match := range matches {
		filtered[i] = items[match.Index]
	}
	return filtered
}
