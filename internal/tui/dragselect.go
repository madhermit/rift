package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
)

// cellPos is a position in the preview's display lines: a line index and a
// terminal cell within it.
type cellPos struct {
	Line, Col int
}

func (p cellPos) before(q cellPos) bool {
	return p.Line < q.Line || (p.Line == q.Line && p.Col < q.Col)
}

// dragSelect is a mouse text selection over display lines, running from the cell
// pressed to the cell under the pointer, both inclusive. Columns are cells of the
// rendered (hardwrapped) row, so what is highlighted is exactly what gets copied.
// Where the diff's layout is known the selection keeps to source text: it skips line
// numbers and stays on the side of a split it was pressed on.
type dragSelect struct {
	anchor, end cellPos
	pressed     bool       // the button went down on selectable content
	dragging    bool       // the pointer has left the pressed cell at least once
	layout      []diff.Row // parallel to the display lines
	side        int        // which of a split row's columns the press landed in
}

func (d *dragSelect) press(p cellPos, layout []diff.Row) {
	*d = dragSelect{anchor: p, end: p, pressed: true, layout: layout}
	if p.Line < len(layout) {
		for i, c := range layout[p.Line].Columns {
			if p.Col < c.End {
				d.side = i
				break
			}
			d.side = i
		}
	}
}

// column is the selectable span of a line of the given width: its text on the
// selection's side, or the whole line when its layout isn't known. A one-column row
// inside a selection begun on a split's right side still has its one column.
func (d dragSelect) column(line, width int) diff.Column {
	if line < len(d.layout) {
		if cols := d.layout[line].Columns; len(cols) > 0 {
			return cols[min(d.side, len(cols)-1)]
		}
	}
	return diff.Column{End: width}
}

func (d *dragSelect) extend(p cellPos) {
	if p != d.anchor {
		d.dragging = true
	}
	d.end = p
}

// ordered returns the selection's endpoints in reading order: a drag can run
// backwards.
func (d dragSelect) ordered() (from, to cellPos) {
	if d.end.before(d.anchor) {
		return d.end, d.anchor
	}
	return d.anchor, d.end
}

// span is the half-open cell range the selection covers on a line of the given
// width: the partial ends on its first and last lines, the whole column between.
func (d dragSelect) span(line, width int) (left, right int) {
	from, to := d.ordered()
	c := d.column(line, width)
	left, right = c.Start, c.End
	if line == from.Line {
		left = max(left, from.Col)
	}
	if line == to.Line {
		right = min(right, to.Col+1)
	}
	return left, right
}

// text is the selection as plain text: ANSI-stripped with each row's trailing
// padding trimmed, a wrapped source line back in one piece, and rows where this side
// of a split has nothing left out. Empty for a selection of nothing but blanks.
func (d dragSelect) text(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	from, to := d.ordered()
	first, last := clampIndex(from.Line, len(lines)), clampIndex(to.Line, len(lines))
	rows := make([]string, 0, last-first+1)
	for i := first; i <= last; i++ {
		row := ansi.Strip(lines[i])
		width := ansi.StringWidth(row) + strings.Count(row, "\t")*viewportTabCells
		c := d.column(i, width)
		if c.Missing {
			continue
		}
		left, right := d.span(i, width)
		text := cutCells(row, left, right)
		if c.Continues && len(rows) > 0 {
			rows[len(rows)-1] += text
		} else {
			rows = append(rows, text)
		}
	}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	text := strings.Trim(strings.Join(rows, "\n"), "\n") // an end in a gutter adds nothing
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}

// viewportTabCells is how wide the viewport draws a tab. Display lines keep their
// tabs, and ansi counts one as zero cells, so a column read off the screen is only
// comparable to a line that accounts for them.
const viewportTabCells = 4

// cutCells is ansi.Cut over cells as drawn, for a plain row that may hold tabs. A
// tab the selection touches is copied as a tab, so pasted code keeps its indent.
func cutCells(row string, left, right int) string {
	var b strings.Builder
	at := 0
	for i, segment := range strings.Split(row, "\t") {
		if i > 0 {
			if at < right && at+viewportTabCells > left {
				b.WriteByte('\t')
			}
			at += viewportTabCells
		}
		b.WriteString(ansi.Cut(segment, maxInt(left-at, 0), right-at))
		at += ansi.StringWidth(segment)
	}
	return b.String()
}

// highlight renders the selection into a viewport body in reverse video.
func (d dragSelect) highlight(body string, yOffset, xOffset int) string {
	from, to := d.ordered()
	rows := strings.Split(body, "\n")
	for i, row := range rows {
		line := yOffset + i
		if line < from.Line || line > to.Line {
			continue
		}
		width := ansi.StringWidth(row) + xOffset
		if d.column(line, width).Missing {
			continue
		}
		left, right := d.span(line, width)
		if left < right {
			rows[i] = reverseSpan(row, left-xOffset, right-xOffset)
		}
	}
	return strings.Join(rows, "\n")
}

// reverseSpan puts cells [left, right) of a styled row in reverse video. ansi.Cut
// replays the styles that precede each piece, so color carries across the cuts; the
// resets between pieces stop a piece's later styles leaking into the next one.
func reverseSpan(row string, left, right int) string {
	left = maxInt(left, 0)
	return ansi.Cut(row, 0, left) + "\x1b[m" +
		reverseVideo(ansi.Cut(row, left, right)) + "\x1b[m" +
		ansi.Cut(row, right, ansi.StringWidth(row))
}

// reverseVideo renders s in reverse video, re-asserted after any SGR reset inside
// it so styled diff content stays highlighted end to end.
func reverseVideo(s string) string {
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m\x1b[7m")
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m\x1b[7m")
	return "\x1b[7m" + s + "\x1b[27m"
}
