package tui

// CollapsedListWidth is the width of the list pane when the diff pane is
// active and the list collapses to a narrow strip.
const CollapsedListWidth = 12

// SplitLayout holds the computed dimensions of a list/diff split-pane view.
type SplitLayout struct {
	HeaderHeight  int
	ContentHeight int
	ListWidth     int
	DiffWidth     int
}

// ComputeSplitLayout lays out a two-pane (list + diff) view for the given
// terminal size. When collapsed (the diff pane is active) the list shrinks to
// CollapsedListWidth; otherwise it takes ~1/3 of the width, clamped to
// [minList, maxList]. The diff pane takes the remainder less 2 columns for its
// border, floored at 10.
func ComputeSplitLayout(width, height int, collapsed bool, minList, maxList int) SplitLayout {
	l := SplitLayout{HeaderHeight: 3}
	l.ContentHeight = height - l.HeaderHeight

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
	if l.DiffWidth < 10 {
		l.DiffWidth = 10
	}
	return l
}
