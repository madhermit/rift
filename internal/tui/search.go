package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// PreviewSearch is `/` over a rendered diff: a live query over the display lines,
// the lines it matched, and which of them the reader is on. The screen owns the
// viewport and the lines and passes them in. searchOrigin is the offset the box
// opened at — the fixed point incremental search steps forward from as you type
// (so the target doesn't drift under its own jumps), and where esc puts the reader
// back if they cancel.
type PreviewSearch struct {
	search       textinput.Model
	searching    bool
	query        string
	matches      []int
	matchAt      int
	searchOrigin int
}

// NewPreviewSearch builds an inactive search.
func NewPreviewSearch() PreviewSearch {
	return PreviewSearch{search: NewFilterInput(), matchAt: -1}
}

// OpenSearch focuses the query box, measuring matches from the reader's position.
func (s *PreviewSearch) OpenSearch(vp *viewport.Model) {
	s.searching = true
	s.searchOrigin = vp.YOffset()
	s.search.Focus()
}

// Searching reports whether the query box has the keyboard.
func (s PreviewSearch) Searching() bool { return s.searching }

// SearchLive reports whether a query is in effect, typed or accepted: esc has
// something to clear and n/N have something to step.
func (s PreviewSearch) SearchLive() bool { return s.searching || s.query != "" }

// SearchInput renders the query box for the footer.
func (s PreviewSearch) SearchInput() string { return s.search.View() }

// HandleSearchKey feeds a key to the open query box.
func (s *PreviewSearch) HandleSearchKey(msg tea.KeyPressMsg, vp *viewport.Model, lines []string) tea.Cmd {
	if msg.String() == "enter" {
		// Accept: the box closes but the query stays live, so the highlight holds
		// and n/N keep stepping. Where the reader ended up becomes the anchor the
		// next search measures from.
		s.searching = false
		s.search.Blur()
		s.searchOrigin = vp.YOffset()
		return nil
	}
	var cmd tea.Cmd
	s.search, cmd = s.search.Update(msg)
	if q := s.search.Value(); q != s.query {
		s.query = q
		s.runSearch(vp, lines)
	}
	return cmd
}

// runSearch re-matches the whole preview and moves to the first match at or
// after the offset the box opened at, wrapping to the top when there is none
// below. Measuring from that fixed origin rather than the live scroll position
// is what keeps an incremental search from walking away from the reader as each
// keystroke narrows the query.
func (s *PreviewSearch) runSearch(vp *viewport.Model, lines []string) {
	s.Rescan(lines)
	if len(s.matches) == 0 {
		return
	}
	s.matchAt = 0
	for i, line := range s.matches {
		if line >= s.searchOrigin {
			s.matchAt = i
			break
		}
	}
	s.scrollToMatch(vp)
}

// Rescan recomputes the active search's matches from scratch — for content that
// was replaced rather than grown.
func (s *PreviewSearch) Rescan(lines []string) {
	s.matches, s.matchAt = nil, -1
	s.ScanFrom(lines, 0)
}

// ScanFrom appends the display lines from index from onward that contain the
// query. Matching is on the ANSI-stripped row, case-insensitively unless the
// query carries an uppercase letter (vim's smartcase). It searches *rendered
// rows*, so a match is only findable where it is legible: a source line the
// hardwrap broke in two won't match across the break, the same bargain as
// grepping what's on screen.
func (s *PreviewSearch) ScanFrom(lines []string, from int) {
	if s.query == "" {
		return
	}
	needle, fold := s.query, s.query == strings.ToLower(s.query)
	for i := from; i < len(lines); i++ {
		row := ansi.Strip(lines[i])
		if fold {
			row = strings.ToLower(row)
		}
		if strings.Contains(row, needle) {
			s.matches = append(s.matches, i)
		}
	}
}

// StepMatch moves to the next (dir > 0) or previous match, wrapping at both
// ends. From "no match selected" — a query typed while the preview was still
// streaming its first hit in — either direction lands on the first.
func (s *PreviewSearch) StepMatch(dir int, vp *viewport.Model) {
	if len(s.matches) == 0 {
		return
	}
	if s.matchAt < 0 {
		s.matchAt = 0
	} else {
		s.matchAt = (s.matchAt + dir + len(s.matches)) % len(s.matches)
	}
	s.scrollToMatch(vp)
}

// scrollToMatch puts the current match on the preview's top row — the same
// landing every other jump in the pane uses (a hunk, a test anchor), so a match
// is always found in the same place on screen.
func (s *PreviewSearch) scrollToMatch(vp *viewport.Model) {
	if line := s.MatchLine(); line >= 0 {
		vp.SetYOffset(line)
	}
}

// MatchLine is the display line of the current match, or -1 when there is none.
func (s PreviewSearch) MatchLine() int {
	if s.matchAt >= 0 && s.matchAt < len(s.matches) {
		return s.matches[s.matchAt]
	}
	return -1
}

// SearchStatus is the preview legend's right-hand text while a search is live.
func (s PreviewSearch) SearchStatus() string {
	switch {
	case s.query == "":
		return ""
	case len(s.matches) == 0:
		return "no match"
	case s.matchAt < 0:
		return fmt.Sprintf("%d matches", len(s.matches))
	default:
		return fmt.Sprintf("match %d/%d", s.matchAt+1, len(s.matches))
	}
}

// ResetSearch drops the search without moving the reader — for a preview that
// has been replaced, where the offset it started from names nothing any more.
func (s *PreviewSearch) ResetSearch() {
	s.searching = false
	s.search.Blur()
	s.search.SetValue("")
	s.query, s.matches, s.matchAt, s.searchOrigin = "", nil, -1, 0
}

// ClearSearch ends the search from the keyboard (esc). Cancelling while still
// typing returns the reader to where the box opened — incremental search moved
// them and they just asked for that to be undone; clearing an accepted search
// leaves them wherever they navigated to.
func (s *PreviewSearch) ClearSearch(vp *viewport.Model) {
	if s.searching {
		vp.SetYOffset(s.searchOrigin)
	}
	s.ResetSearch()
}

// HighlightMatch renders the current match in a viewport body in reverse video.
func (s PreviewSearch) HighlightMatch(body string, yOffset int) string {
	if line := s.MatchLine(); line >= 0 {
		return highlightRows(body, yOffset, line, line)
	}
	return body
}
