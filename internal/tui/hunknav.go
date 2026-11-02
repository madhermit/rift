package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
)

// Locating source positions inside a *rendered* diff. The preview pane shows an
// engine's output, not the unified diff it came from, so anything that wants to
// address the source — anchor on a test's line, put a cursor on a hunk — has to
// map a line number back onto a display row. Two independent readings do it,
// because the two engines print different things:
//
//   - the git engine renders the unified diff verbatim, `@@` headers included,
//     so a hunk's own header line locates it exactly;
//   - difftastic prints no headers at all, only a new-side line-number gutter,
//     so the position is the first row whose new-side number reaches the target.
//
// Both are read from the same stripped rows, and neither is engine-specific
// knowledge that leaks outward: a caller asks for a position and gets the best
// available answer, or none.

// HunkOffsets maps each of a file's hunks to the display line where it begins
// within lines — one file's rendered diff, ANSI included. Offsets are
// non-decreasing and every hunk gets one: a hunk neither reading can place
// falls back to just after the previous hunk, so the cursor stays usable on
// output that carries neither headers nor a gutter (a binary or mode-only
// change) rather than dropping hunks out of the sequence.
func HunkOffsets(lines []string, hunks []diff.Hunk) []int {
	offsets := make([]int, len(hunks))
	stripped := stripAll(lines)
	g := findGutter(stripped)
	from := 0
	for i, h := range hunks {
		at := hunkStart(stripped, from, g, h)
		if at < 0 {
			at = from
		}
		offsets[i] = at
		from = at + 1 // the next hunk starts strictly after this one
	}
	return offsets
}

// SectionRange is the display-line range [start, end) of the section labelled
// label. A preview with no sections at all is one unlabelled span (a diff
// rendered without a banner), so it yields everything; an unmatched label in a
// sectioned preview yields an empty range, so a caller resolving positions in a
// file that isn't there finds nothing rather than searching another file's rows.
func SectionRange(sections []PreviewSection, label string, total int) (start, end int) {
	for i, s := range sections {
		if s.Label != label {
			continue
		}
		if i+1 < len(sections) {
			return s.Offset, sections[i+1].Offset
		}
		return s.Offset, total
	}
	if len(sections) == 0 {
		return 0, total
	}
	return 0, 0
}

// hunkStart locates h's first display row at or after from, preferring its
// literal `@@` header (exact, when the engine printed one) and falling back to
// the line-number gutter. -1 when neither places it.
func hunkStart(stripped []string, from int, g gutter, h diff.Hunk) int {
	if h.Header != "" {
		for i := from; i < len(stripped); i++ {
			if strings.HasPrefix(stripped[i], h.Header) {
				return i
			}
		}
	}
	return firstRowAtOrPast(stripped, from, g, h.NewStart)
}

// firstRowAtOrPast returns the index of the first row at or after from whose
// new-side line number reaches target, or -1 when the rows carry no gutter, the
// target is meaningless (<= 0), or nothing reaches it.
func firstRowAtOrPast(stripped []string, from int, g gutter, target int) int {
	if !g.ok || target <= 0 {
		return -1
	}
	for i := from; i < len(stripped); i++ {
		if n := g.lineNum(stripped[i]); n >= target {
			return i
		}
	}
	return -1
}

func stripAll(lines []string) []string {
	stripped := make([]string, len(lines))
	for i, line := range lines {
		stripped[i] = ansi.Strip(line)
	}
	return stripped
}

// gutter describes where a rendered diff prints its new-side line numbers: the
// column the digits are anchored to, and whether they anchor by their start or
// their end. Both exist in practice — difftastic's inline layout left-aligns
// the number at a fixed start column (so its end moves as the number widens
// from 9 to 10), while side-by-side right-aligns it to a fixed end column — and
// reading the wrong one silently skips every row whose number has the less
// common digit width. ok is false when the render carries no line numbers at
// all (the git engine prints none; nor does a binary file).
type gutter struct {
	col     int
	byStart bool
	ok      bool
}

// findGutter locates the new-side line-number gutter. A gutter number is a
// digit run past the margin (column 0 belongs to the old-side gutter) with a
// space before it and padding after, so both layouts' new-side numbers qualify
// and their content doesn't. Whichever anchoring — start column or end column —
// accounts for more of those runs is the one the render uses; ties keep the end,
// which is what a uniform-width gutter reduces to either way.
func findGutter(stripped []string) gutter {
	starts, ends := map[int]int{}, map[int]int{}
	for _, s := range stripped {
		for i := 0; i < len(s); {
			if !isDigit(s[i]) {
				i++
				continue
			}
			start := i
			for i < len(s) && isDigit(s[i]) {
				i++
			}
			if start > 0 && s[start-1] == ' ' && i < len(s) && s[i] == ' ' {
				starts[start]++
				ends[i]++ // i is the exclusive end: the first padding space
			}
		}
	}
	startCol, startN := columnMode(starts)
	endCol, endN := columnMode(ends)
	switch {
	case startN > endN:
		return gutter{col: startCol, byStart: true, ok: true}
	case endN > 0:
		return gutter{col: endCol, ok: true}
	}
	return gutter{}
}

// columnMode returns the most frequent column and how often it occurred,
// tie-breaking on the larger column so that randomized map iteration can't
// change the answer (and the scroll position) between runs of the same diff.
func columnMode(counts map[int]int) (col, n int) {
	col = -1
	for c, k := range counts {
		if k > n || (k == n && c > col) {
			col, n = c, k
		}
	}
	return col, n
}

// lineNum reads the new-side line number a row carries, or -1 when it carries
// none: an old-side/removed row, a wrapped continuation, or a header. The digits
// must sit exactly on the gutter's anchor and be fenced by spaces, so content
// that merely contains a number isn't mistaken for a line number.
func (g gutter) lineNum(s string) int {
	if !g.ok || g.col <= 0 {
		return -1
	}
	start, end := g.col, g.col
	if g.byStart {
		if start >= len(s) || s[start-1] != ' ' || !isDigit(s[start]) {
			return -1
		}
		for end < len(s) && isDigit(s[end]) {
			end++
		}
	} else {
		if end > len(s) || !isDigit(s[end-1]) {
			return -1
		}
		for start > 0 && isDigit(s[start-1]) {
			start--
		}
		if start > 0 && s[start-1] != ' ' {
			return -1
		}
	}
	if end < len(s) && s[end] != ' ' {
		return -1
	}
	n, _ := strconv.Atoi(s[start:end])
	return n
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
