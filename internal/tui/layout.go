package tui

import tea "charm.land/bubbletea/v2"

// CollapsedListWidth is the width of the list pane when the diff pane is
// active and the list collapses to a narrow strip.
const CollapsedListWidth = 12

// HeaderRows and FooterRows are the vertical chrome reserved above and below
// the split-pane content (see Header and Footer).
const (
	HeaderRows = 2
	FooterRows = 2
)

// SplitLayout holds the computed dimensions of a list/diff split-pane view.
// ContentHeight is the height of each pane (border included), with the header
// and footer rows already subtracted.
type SplitLayout struct {
	ContentHeight int
	ListWidth     int
	DiffWidth     int
}

// minDiffWidth is the narrowest the diff pane is allowed to get before the list
// pane is shrunk to make room.
const minDiffWidth = 10

// ComputeSplitLayout lays out a two-pane (list + diff) view for the given
// terminal size. When collapsed (the diff pane is active) the list shrinks to
// CollapsedListWidth; otherwise it takes ~1/3 of the width, clamped to
// [minList, maxList]. The diff pane takes the remainder less 2 columns for its
// border; on a narrow terminal the list yields so the diff keeps minDiffWidth
// and the panes fit within the screen (down to ~13 columns, below which a
// split view isn't usable anyway).
func ComputeSplitLayout(width, height int, collapsed bool, minList, maxList int) SplitLayout {
	l := SplitLayout{}
	l.ContentHeight = height - HeaderRows - FooterRows

	if collapsed {
		l.ListWidth = CollapsedListWidth
	} else {
		l.ListWidth = width / 3
		if l.ListWidth < minList {
			l.ListWidth = minList
		}
		if l.ListWidth > maxList {
			l.ListWidth = maxList
		}
	}

	l.DiffWidth = width - l.ListWidth - 2
	if l.DiffWidth < minDiffWidth {
		// Not enough room: shrink the list so the diff keeps its floor and the
		// panes still fit within width (rather than overflowing it).
		l.DiffWidth = minDiffWidth
		l.ListWidth = width - l.DiffWidth - 2
		if l.ListWidth < 1 {
			l.ListWidth = 1
		}
	}
	return l
}

// ScreenView wraps rendered screen content in the standard full-screen
// tea.View: alt-screen on, and mouse tracking requested unless the user
// opted out (see MouseEnabled). Every top-level screen builds its view here
// so terminal modes stay consistent across them.
func ScreenView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	if MouseEnabled() {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// WheelDelta maps a wheel message to a vertical scroll direction: -1 up,
// 1 down, 0 for horizontal wheel events (which the screens don't use).
func WheelDelta(msg tea.MouseWheelMsg) int {
	switch msg.Button {
	case tea.MouseWheelUp:
		return -1
	case tea.MouseWheelDown:
		return 1
	}
	return 0
}

// ClickedRow maps a click on a list's inner row (0-based, panel borders
// already subtracted) to the item index it lands on, through the same window
// the list renders with (see ListWindow). ok is false when the click misses —
// a border, past the last item — or lands on the already-selected item, so
// callers treat it as "selection changed".
func ClickedRow(row, innerH, selected, total int) (int, bool) {
	if row < 0 || row >= innerH || total == 0 {
		return 0, false
	}
	offset, _ := ListWindow(selected, total, innerH)
	idx := offset + row
	if idx >= total || idx == selected {
		return 0, false
	}
	return idx, true
}
