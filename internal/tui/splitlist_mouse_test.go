package tui

import "testing"

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
	m, _ = m.focusPane(splitPreviewPane)
	if m.active != splitPreviewPane {
		t.Fatal("focusPane did not switch panes")
	}
	_, readNav := m.stackLayout()
	if readNav >= surveyNav {
		t.Errorf("reading layout should collapse the strip: %d -> %d", surveyNav, readNav)
	}
	if same, _ := m.focusPane(splitPreviewPane); same.active != splitPreviewPane {
		t.Error("re-focusing the same pane should be a no-op")
	}
}
