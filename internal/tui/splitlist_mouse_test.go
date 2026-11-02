package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestDragSelectText(t *testing.T) {
	lines := []string{
		"\x1b[31mhello\x1b[0m world   ",
		"",
		"世界 wide",
		"   indented",
		"\t\treturn x",
	}
	tests := []struct {
		name       string
		anchor, to cellPos
		want       string
	}{
		{"part of a line", cellPos{0, 1}, cellPos{0, 3}, "ell"},
		{"crossing a style boundary", cellPos{0, 3}, cellPos{0, 7}, "lo wo"},
		{"backwards", cellPos{0, 3}, cellPos{0, 1}, "ell"},
		{"trailing padding trimmed", cellPos{0, 6}, cellPos{0, 40}, "world"},
		{"across lines keeps partial ends", cellPos{0, 6}, cellPos{3, 4}, "world\n\n世界 wide\n   in"},
		{"backwards across lines", cellPos{3, 4}, cellPos{0, 6}, "world\n\n世界 wide\n   in"},
		{"wide characters", cellPos{2, 0}, cellPos{2, 3}, "世界"},
		{"only blanks copies nothing", cellPos{3, 0}, cellPos{3, 2}, ""},
		{"past the end of a line", cellPos{1, 5}, cellPos{1, 9}, ""},
		{"past the last line", cellPos{4, 8}, cellPos{9, 0}, "return x"},
		{"columns count tabs as drawn", cellPos{4, 8}, cellPos{4, 13}, "return"},
		{"a selected tab stays a tab", cellPos{4, 0}, cellPos{4, 13}, "\t\treturn"},
		{"a tab touched from inside is kept", cellPos{4, 6}, cellPos{4, 8}, "\tr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := dragSelect{anchor: tt.anchor, end: tt.to, pressed: true, dragging: true}
			if got := d.text(lines); got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDragSelectHighlight(t *testing.T) {
	body := "one\n\x1b[31mtwo three\x1b[0m tail\nfour\nfive"
	d := dragSelect{anchor: cellPos{11, 4}, end: cellPos{12, 1}, pressed: true, dragging: true}
	rows := strings.Split(d.highlight(body, 10, 0), "\n")

	if strings.Contains(rows[0], "\x1b[7m") || strings.Contains(rows[3], "\x1b[7m") {
		t.Errorf("rows outside the selection must not be highlighted: %q", rows)
	}
	for i, want := range []string{"one", "two three tail", "four", "five"} {
		if got := ansi.Strip(rows[i]); got != want {
			t.Errorf("row %d text = %q, want %q: highlighting must not change content", i, got, want)
		}
	}
	// The reverse video starts mid-row, with what precedes it left alone.
	if !strings.HasPrefix(rows[1], "\x1b[31mtwo ") || !strings.Contains(rows[1], "\x1b[7m") {
		t.Errorf("selection should start at the anchor's cell: %q", rows[1])
	}
	// The reset inside the selection re-asserts reverse, and the unselected tail
	// of a partially selected row is not reversed.
	if !strings.Contains(rows[1], "\x1b[0m\x1b[7m") {
		t.Errorf("reset inside the selection not re-asserted: %q", rows[1])
	}
	// A partial last row: "fo" selected, and color state survives past the cut.
	if got := ansi.Strip(ansi.Cut(rows[2], 0, 2)); got != "fo" || !strings.Contains(rows[2], "\x1b[7mfo") {
		t.Errorf("last row should reverse exactly through the end cell: %q", rows[2])
	}

	styled := reverseSpan("\x1b[31mabcdef\x1b[0m", 1, 3)
	if !strings.Contains(styled, "\x1b[m\x1b[31mdef") {
		t.Errorf("the tail after a selection should stay colored: %q", styled)
	}
}

// previewAt builds a list showing lines in its preview, returning the screen row of
// the preview's first content line.
func previewAt(t *testing.T, lines []string) (SplitList[string], int) {
	t.Helper()
	m := testList([]string{"a"})
	m.resizeViewport()
	m.preview = strings.Join(lines, "\n")
	m.setPreviewContent()
	_, navH := m.stackLayout()
	return m, HeaderRows + navH + 1
}

func press(x, y int) tea.Msg { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft} }
func move(x, y int) tea.Msg  { return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft} }
func lift(x, y int) tea.Msg  { return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft} }

func TestDragMapsScreenCellsToText(t *testing.T) {
	m, top := previewAt(t, []string{"alpha beta gamma", "second line"})
	// Column 0 is the panel border, so screen x=7 is cell 6, the b of "beta".
	m, _ = m.Update(press(7, top))
	m, _ = m.Update(move(10, top))
	if got := m.drag.text(m.vim.Lines()); got != "beta" {
		t.Errorf("selected %q, want %q", got, "beta")
	}
	m, _ = m.Update(lift(10, top))
	if m.drag.pressed {
		t.Error("release should end the drag")
	}
	if m.flash != "copied 4 characters" {
		t.Errorf("flash = %q", m.flash)
	}
}

func TestDragThatReturnsIsAClick(t *testing.T) {
	m, top := previewAt(t, []string{"alpha beta"})
	m, _ = m.Update(press(3, top))
	m, _ = m.Update(move(4, top))
	m, _ = m.Update(move(3, top))
	m, _ = m.Update(lift(3, top))
	if m.flash != "" {
		t.Errorf("a wobble back to the pressed cell must not copy: flash %q", m.flash)
	}
	if m.active != splitPreviewPane {
		t.Error("a click should focus the preview")
	}
}

func TestDragPastEdgeScrolls(t *testing.T) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	m, top := previewAt(t, lines)

	m, _ = m.Update(press(2, top))
	m, cmd := m.Update(move(2, top+m.viewport.Height()+1))
	if cmd == nil {
		t.Fatal("dragging past the bottom edge should start the scroll tick")
	}
	before := m.drag.end.Line
	m, cmd = m.Update(dragScrollMsg{})
	if m.viewport.YOffset() != 1 || m.drag.end.Line != before+1 {
		t.Errorf("tick should scroll a line and extend: offset %d, end %d (was %d)", m.viewport.YOffset(), m.drag.end.Line, before)
	}
	if cmd == nil {
		t.Error("the tick should keep going while the pointer is past the edge")
	}

	// Back inside the pane: the chain dies.
	m, cmd = m.Update(move(2, top+3))
	if cmd != nil {
		t.Error("motion inside the pane should not start a tick")
	}
	offset := m.viewport.YOffset()
	m, cmd = m.Update(dragScrollMsg{})
	if m.viewport.YOffset() != offset || cmd != nil {
		t.Errorf("a pointer back inside must stop scrolling: offset %d -> %d", offset, m.viewport.YOffset())
	}

	// A late tick after release does nothing.
	m, _ = m.Update(lift(2, top+3))
	m, cmd = m.Update(dragScrollMsg{})
	if m.viewport.YOffset() != offset || cmd != nil {
		t.Error("a tick after release must not scroll")
	}
}

func TestDragHighlightShownOnlyWhileMoving(t *testing.T) {
	m, top := previewAt(t, []string{"alpha beta"})
	m, _ = m.Update(press(2, top))
	if body, _ := m.previewBodyView(); strings.Contains(body, "\x1b[7m") {
		t.Error("a press alone must not highlight")
	}
	m, _ = m.Update(move(4, top))
	if body, _ := m.previewBodyView(); !strings.Contains(body, "\x1b[7m") {
		t.Error("a drag should highlight")
	}
}

// Selection columns are read off the screen, so viewportTabCells has to match what
// the viewport really draws, or the copy drifts from the highlight.
func TestViewportDrawsTabsAtAssumedWidth(t *testing.T) {
	m, top := previewAt(t, []string{"\tx"})
	if got := ansi.Strip(m.viewport.View()); !strings.HasPrefix(got, strings.Repeat(" ", viewportTabCells)+"x") {
		t.Fatalf("viewport drew a tab differently than assumed: %q", got)
	}
	m, _ = m.Update(press(1+viewportTabCells, top))
	m, _ = m.Update(move(2+viewportTabCells, top))
	if got := m.drag.text(m.vim.Lines()); got != "x" {
		t.Errorf("selected %q, want %q", got, "x")
	}
}

func TestWheelExtendsDrag(t *testing.T) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	m, top := previewAt(t, lines)
	m, _ = m.Update(press(2, top))
	m, _ = m.Update(tea.MouseWheelMsg{X: 2, Y: top, Button: tea.MouseWheelDown})
	if m.viewport.YOffset() == 0 {
		t.Fatal("the wheel should scroll the preview")
	}
	if !m.drag.dragging || m.drag.end.Line != m.viewport.YOffset() {
		t.Errorf("the selection should follow the scroll: end line %d, offset %d", m.drag.end.Line, m.viewport.YOffset())
	}
}
