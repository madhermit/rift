package tui

import (
	"strings"
	"testing"
)

func testList(items []string) SplitList[string] {
	cfg := SplitConfig[string]{
		Row:   func(s string, _ int, _ bool) string { return s },
		Match: func(s string) string { return s },
	}
	m := NewSplitList(cfg, items)
	m.width, m.height = 80, 24
	m.ready = true
	return m
}

func TestSelectRow(t *testing.T) {
	m := testList([]string{"a", "b", "c", "d", "e", "f", "g", "h"})
	_, navH := m.stackLayout()
	innerH := navH - 2
	if innerH < 3 {
		t.Fatalf("test needs a strip of at least 3 rows, got %d", innerH)
	}

	// Un-scrolled window: row r is item r.
	got, _ := m.selectRow(2, innerH)
	if got.selected != 2 {
		t.Errorf("selectRow(2) selected %d, want 2", got.selected)
	}

	// Selection past the window scrolls the list; a click maps through the
	// same offset the rows rendered with.
	m.selected = len(m.items) - 1
	offset, _ := ListWindow(m.selected, len(m.items), innerH)
	if offset == 0 {
		t.Fatalf("expected a scrolled window for selected=%d innerH=%d", m.selected, innerH)
	}
	got, _ = m.selectRow(0, innerH)
	if got.selected != offset {
		t.Errorf("scrolled selectRow(0) selected %d, want %d", got.selected, offset)
	}

	// Border and past-the-end clicks keep the selection.
	for _, row := range []int{-1, innerH, len(m.items) + 5} {
		got, _ = m.selectRow(row, innerH)
		if got.selected != m.selected {
			t.Errorf("selectRow(%d) moved selection to %d, want unchanged %d", row, got.selected, m.selected)
		}
	}
}

func TestFocusPaneRelayouts(t *testing.T) {
	m := testList([]string{"a", "b", "c"})
	_, surveyNav := m.stackLayout()

	// A mouse click focuses the preview without entering the peek layout —
	// re-layouting the screen under the pointer is jarring.
	m, _ = m.focusPane(splitPreviewPane)
	if m.active != splitPreviewPane {
		t.Fatal("focusPane did not switch panes")
	}
	if _, nav := m.stackLayout(); nav != surveyNav {
		t.Errorf("click focus must not collapse the strip: %d -> %d", surveyNav, nav)
	}

	// The keyboard gesture (⇥/⏎) sets peek before focusing: that collapses.
	m.peek = true
	if _, nav := m.stackLayout(); nav >= surveyNav {
		t.Errorf("peek layout should collapse the strip: %d -> %d", surveyNav, nav)
	}

	// Focusing the list (click on the peek row) leaves the peek layout.
	m, _ = m.focusPane(splitListPane)
	if m.peek {
		t.Error("focusing the list should clear the peek layout")
	}
	if _, nav := m.stackLayout(); nav != surveyNav {
		t.Errorf("strip should expand when the list takes focus: got %d, want %d", surveyNav, nav)
	}
}

func TestHighlightRows(t *testing.T) {
	body := "one\n\x1b[31mtwo\x1b[0m tail\nthree"
	got := highlightRows(body, 10, 11, 11) // only the middle visible row (line 11)
	lines := strings.Split(got, "\n")
	if strings.Contains(lines[0], "\x1b[7m") || strings.Contains(lines[2], "\x1b[7m") {
		t.Errorf("unselected rows must not be highlighted: %q", got)
	}
	if !strings.HasPrefix(lines[1], "\x1b[7m") {
		t.Errorf("selected row missing reverse video: %q", lines[1])
	}
	// The reset inside the styled segment must re-assert reverse video.
	if !strings.Contains(lines[1], "\x1b[0m\x1b[7m") {
		t.Errorf("reset inside selected row not re-asserted: %q", lines[1])
	}
}
