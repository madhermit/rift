package diff

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Column is the span of one row that holds source text, in cells: the part after
// the line numbers, and on one side of a side-by-side split.
type Column struct {
	Start, End int
	Missing    bool // this side has no text on this row
	Continues  bool // this row wraps the source line of the row above
}

// Row is a display row's text columns: two for a side-by-side row, one for any
// other row of a difftastic hunk, none for a row with no known layout.
type Row struct {
	Columns []Column
}

// Layout finds where the source text sits in difftastic's rendered output, given
// ANSI-stripped display rows and the width they were rendered for. The result is
// parallel to rows. It reads the text rather than asking the engine because one
// file's hunks can each take a different shape, and rows from anything else (the
// git engine, a commit message) must stay plain: a hunk is only trusted when all of
// its rows fit one shape, so a misread loses precision rather than content.
func Layout(rows []string, width int) []Row {
	out := make([]Row, len(rows))
	for i := 0; i < len(rows); i++ {
		if !isHunkHeader(rows[i]) {
			continue
		}
		end := i + 1
		for end < len(rows) && strings.TrimSpace(rows[end]) != "" {
			end++
		}
		first := i + 1
		if first < end && strings.HasPrefix(rows[first], "File permissions changed") {
			first++ // difftastic's one note between a header and its rows
		}
		layoutHunk(rows[first:end], width, out[first:end])
		i = end
	}
	return out
}

// isHunkHeader matches "path --- Lang" and "path --- 2/7 --- Lang". Git's own
// "--- a/path" has nothing before the dashes.
func isHunkHeader(row string) bool {
	before, after, ok := strings.Cut(row, " --- ")
	return ok && strings.TrimSpace(before) != "" && strings.TrimSpace(after) != "" && before[0] != ' '
}

// leadingNumber finds the line-number field a row opens with: digits, or the dots
// difftastic prints where a side has no number. end is the cell after it.
func leadingNumber(row string) (start, end int, ok bool) {
	start = len(row) - len(strings.TrimLeft(row, " "))
	field := row[start:]
	n := len(field) - len(strings.TrimLeft(field, "0123456789"))
	if n == 0 {
		n = len(field) - len(strings.TrimLeft(field, "."))
	}
	if n == 0 || (n < len(field) && field[n] != ' ') {
		return 0, 0, false
	}
	return start, start + n, true
}

func layoutHunk(rows []string, width int, out []Row) {
	if len(rows) == 0 {
		return
	}
	if sideBySide(rows, width, out) {
		return
	}
	for _, gutter := range []func() func(string) (int, bool){doubleNumberGutter, singleNumberGutter, inlineGutter} {
		if singleColumn(rows, width, gutter(), out) {
			return
		}
	}
}

// sideBySide lays out a true two-column hunk:
//
//	59 func f56() int { return 56 }      60 func f56() int { return 56 }
//	..                                   62
//
// difftastic gives the left side half the width less a one-cell spacer, so the
// right side's number field starts at a known cell, and every row must have one.
func sideBySide(rows []string, width int, out []Row) bool {
	split := (width-1)/2 + 1
	leftEnd, rightEnd := -1, -1
	parsed := make([]Row, len(rows))
	for i, row := range rows {
		_, lEnd, lok := leadingNumber(row)
		right := ansi.Cut(row, split, width)
		_, rEnd, rok := leadingNumber(right)
		if !lok || !rok || strings.TrimSpace(ansi.Cut(row, split-1, split)) != "" {
			return false
		}
		if (leftEnd >= 0 && lEnd != leftEnd) || (rightEnd >= 0 && rEnd != rightEnd) {
			return false
		}
		leftEnd, rightEnd = lEnd, rEnd
		parsed[i].Columns = []Column{
			sideColumn(row, lEnd, lEnd+1, split-1, 0),
			sideColumn(right, rEnd, rEnd+1, width-split, split),
		}
	}
	copy(out, parsed)
	return true
}

// sideColumn is one side's column. Dots in the number field mean there's no new
// source line here: with text on the row it wraps the line above, without it the
// side simply has nothing. offset places a right side's cells within the full row.
func sideColumn(side string, numberEnd, start, end, offset int) Column {
	dots := side[numberEnd-1] == '.'
	blank := strings.TrimSpace(ansi.Cut(side, start, end)) == ""
	return Column{
		Start:     start + offset,
		End:       end + offset,
		Missing:   dots && blank,
		Continues: dots && !blank,
	}
}

// singleColumn lays out a hunk with one text column, given how to find a row's
// gutter. difftastic doesn't wrap these (rift hard-wraps them), so a row without a
// number is only accepted as the tail of a row that filled the width.
func singleColumn(rows []string, width int, gutter func(string) (int, bool), out []Row) bool {
	parsed := make([]Row, len(rows))
	full := false
	for i, row := range rows {
		column := Column{End: width}
		if end, ok := gutter(row); ok {
			column.Start = end
		} else if full {
			column.Continues = true
		} else {
			return false
		}
		parsed[i].Columns = []Column{column}
		full = ansi.StringWidth(row) >= width-1 // a wide rune can leave one cell spare
	}
	copy(out, parsed)
	return true
}

// doubleNumberGutter is for a hunk with a number field per side ahead of one shared
// text column: " 10  10 func f7()", " ..  11 func onlyRight()".
func doubleNumberGutter() func(string) (int, bool) {
	firstEnd, secondEnd := -1, -1
	return func(row string) (int, bool) {
		_, first, ok := leadingNumber(row)
		if !ok {
			return 0, false
		}
		_, second, ok := leadingNumber(row[first:])
		second += first
		if !ok || (firstEnd >= 0 && (first != firstEnd || second != secondEnd)) {
			return 0, false
		}
		firstEnd, secondEnd = first, second
		return second + 1, true
	}
}

// singleNumberGutter is for one right-aligned number field: "  5 func f2()".
func singleNumberGutter() func(string) (int, bool) {
	numberEnd := -1
	return func(row string) (int, bool) {
		_, end, ok := leadingNumber(row)
		if !ok || (numberEnd >= 0 && end != numberEnd) {
			return 0, false
		}
		numberEnd = end
		return end + 1, true
	}
}

// inlineGutter is for the inline display, whose old and new rows pad differently and
// don't align when numbers change length: "10    func" and "   11 func".
func inlineGutter() func(string) (int, bool) {
	return func(row string) (int, bool) {
		start, end, ok := leadingNumber(row)
		switch {
		case ok && start == 3:
			return end + 1, true
		case ok && start == 0 && (strings.HasPrefix(row[end:], "    ") || strings.TrimSpace(row[end:]) == ""):
			return end + 4, true
		}
		return 0, false
	}
}
