package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Truncate shortens s to at most max display cells, appending a trailing
// ellipsis when it overflows. Use for text where the start is most significant
// (commit messages, stash descriptions).
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= max {
		return s
	}
	if max <= 3 {
		return headWidth(s, max)
	}
	return headWidth(s, max-3) + "..."
}

// TruncatePath shortens a file path to at most max display cells, keeping the
// filename intact: it first abbreviates leading directory components to a single
// rune each (internal/tui/diff/x.go → i/t/diff/x.go), and only if the filename
// alone still overflows does it elide its head behind a leading ellipsis.
func TruncatePath(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= max {
		return s
	}

	parts := strings.Split(s, "/")
	for i := 0; i < len(parts)-1; i++ {
		if ansi.StringWidth(strings.Join(parts, "/")) <= max {
			break
		}
		parts[i] = firstRune(parts[i])
	}
	out := strings.Join(parts, "/")
	if ansi.StringWidth(out) <= max {
		return out
	}
	// Filename alone still overflows: keep its tail behind a leading ellipsis.
	return "…" + tailWidth(out, max-1)
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return s
}

// headWidth returns the longest prefix of s whose display width is at most w.
func headWidth(s string, w int) string {
	width := 0
	for i, r := range s {
		rw := ansi.StringWidth(string(r))
		if width+rw > w {
			return s[:i]
		}
		width += rw
	}
	return s
}

// tailWidth returns the longest suffix of s whose display width is at most w.
func tailWidth(s string, w int) string {
	r := []rune(s)
	width := 0
	for i := len(r) - 1; i >= 0; i-- {
		rw := ansi.StringWidth(string(r[i]))
		if width+rw > w {
			return string(r[i+1:])
		}
		width += rw
	}
	return s
}
