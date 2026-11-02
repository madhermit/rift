package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

var filterPromptStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("39")).
	Bold(true)

// NewFilterInput returns a textinput pre-configured for the "/ " prompt shared by
// the list filter and the preview search.
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
// query, ordered by match quality. A term prefixed with `!` is an exclusion:
// items whose target contains it (case-insensitively) are dropped whatever else
// they match, so `model !test` finds the model files without the tests, and a
// bare `!lock` narrows to everything that isn't the lockfile. An empty query, or
// one that is only exclusions, leaves the surviving items in their original
// order rather than reordering them by match quality.
func FuzzyFilter[T any](items []T, query string, format func(T) string) []T {
	if query == "" {
		return items
	}
	include, exclude := splitQuery(query)
	if include == "" && len(exclude) == 0 {
		return items
	}
	targets := make([]string, len(items))
	for i, it := range items {
		targets[i] = format(it)
	}
	kept, keptTargets := items, targets
	if len(exclude) > 0 {
		kept, keptTargets = nil, nil
		for i, t := range targets {
			if !containsAny(strings.ToLower(t), exclude) {
				kept, keptTargets = append(kept, items[i]), append(keptTargets, t)
			}
		}
	}
	if include == "" {
		return kept
	}
	matches := fuzzy.Find(include, keptTargets)
	filtered := make([]T, len(matches))
	for i, match := range matches {
		filtered[i] = kept[match.Index]
	}
	return filtered
}

// splitQuery separates a filter query into its fuzzy part and its `!`-prefixed
// exclusions (lowercased, for case-insensitive matching). A query with no `!` in
// it is handed back untouched — re-splitting one on whitespace would collapse
// runs of spaces the fuzzy matcher takes literally, quietly changing what an
// existing query matches. A lone `!` contributes nothing, so a half-typed
// exclusion filters out nothing rather than everything.
func splitQuery(query string) (include string, exclude []string) {
	if !strings.Contains(query, "!") {
		return query, nil
	}
	var terms []string
	for _, f := range strings.Fields(query) {
		if rest, ok := strings.CutPrefix(f, "!"); ok {
			if rest != "" {
				exclude = append(exclude, strings.ToLower(rest))
			}
			continue
		}
		terms = append(terms, f)
	}
	return strings.Join(terms, " "), exclude
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
